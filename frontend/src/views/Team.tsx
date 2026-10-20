import {FileText, FlaskConical, Pencil, UserPlus, Users} from 'lucide-react';
import {useState} from 'react';
import {api} from '../api';
import {t} from '../i18n';
import {useLoad} from '../live';
import type {ActorKind, Assignment, Project} from '../types';
import {ActorBadge, Button, DetailHeader, EmptyState, ErrorBox, Field, Loading, StatusBadge, useAction, useDetail, useToast} from '../ui';

type Form = {
    id: string; roleId: string; actorKind: ActorKind; displayName: string;
    connectionId: string; model: string; instructions: string;
};

const empty: Form = {id: '', roleId: '', actorKind: 'ai', displayName: '', connectionId: '', model: 'auto', instructions: ''};

export function Team({project}: { project: Project }) {
    const list = useLoad(() => api.assignments(project.id), [project.id], project.id);
    const roles = useLoad(() => api.roles(), []);
    const conns = useLoad(() => api.connections(), []);
    const action = useAction();
    const setDetail = useDetail();
    const archived = project.status === 'archived';
    const roleName = (id: string) => roles.data?.find((r) => r.id === id)?.name ?? '(삭제된 역할)';
    const conn = (id: string) => conns.data?.find((c) => c.id === id);

    const showPreview = async (a: Assignment) => {
        const p = await action.run(() => api.instructionPreview(project.id, a.id));
        if (!p) return;
        setDetail(
            <>
                <DetailHeader title={`${a.displayName}의 최종 지침`} onClose={() => setDetail(null)}>
                    <p className="muted small">회사 → 상위 역할 → 역할 → 서비스 → 담당자 순서로 합쳐집니다. 업무 지시와 수정 의견은 실행 때 덧붙습니다.</p>
                </DetailHeader>
                <ol className="timeline">
                    {p.layers.map((l, i) => (
                        <li key={i} className="timeline-item">
                            <span className="timeline-marker info">{i + 1}</span>
                            <div className="timeline-body">
                                <strong className="small">{l.name}</strong>
                                <div className="pre small">{l.text || <span className="muted">(비어 있음)</span>}</div>
                            </div>
                        </li>
                    ))}
                </ol>
            </>,
        );
    };

    const edit = (f: Form) => setDetail(<AssignmentForm key={f.id || 'new'} project={project} initial={f}
                                                        onDone={() => { setDetail(null); list.reload(); }}/>);

    if (list.error) return <ErrorBox error={list.error}/>;
    if (!list.data) return <Loading/>;
    return (
        <div className="stack">
            <div className="row between">
                <p className="muted">회사 역할을 이 서비스에서 누가 맡을지 정합니다. 배정만으로 AI가 실행되지는 않습니다.</p>
                <Button variant="primary" icon={UserPlus} disabled={archived} onClick={() => edit({...empty})}>담당자 추가</Button>
            </div>
            <ErrorBox error={action.error}/>
            {list.data.length === 0 ? (
                <EmptyState icon={Users} title="아직 담당자가 없습니다"
                            actions={!archived ? <Button variant="primary" icon={UserPlus} onClick={() => edit({...empty})}>담당자 추가</Button> : null}>
                    역할마다 AI 또는 나를 배정하면 업무 흐름에서 업무를 맡길 수 있습니다.
                </EmptyState>
            ) : (
                <div className="table-wrap">
                    <table className="list">
                        <thead><tr><th>담당자</th><th>역할</th><th>종류</th><th>AI 연결</th><th/></tr></thead>
                        <tbody>
                        {list.data.map((a) => {
                            const c = conn(a.connectionId);
                            return (
                                <tr key={a.id}>
                                    <td><strong>{a.displayName}</strong></td>
                                    <td>{roleName(a.roleId)}</td>
                                    <td><ActorBadge kind={a.actorKind}/></td>
                                    <td className="small">
                                        {a.actorKind === 'human' ? <span className="muted">나 (이 앱 사용자)</span>
                                            : c ? <span className="row nowrap">{c.name}<StatusBadge tone={c.usable ? 'ok' : 'bad'}>{c.usable ? '사용 가능' : '확인 필요'}</StatusBadge>
                                                {a.model ? <span className="muted">{a.model}</span> : null}</span>
                                                : <StatusBadge tone="bad">연결 없음</StatusBadge>}
                                    </td>
                                    <td className="actions">
                                        <span className="row nowrap" style={{justifyContent: 'flex-end'}}>
                                            <Button size="sm" variant="ghost" icon={FileText} onClick={() => showPreview(a)}>지침 보기</Button>
                                            <Button size="sm" icon={Pencil} disabled={archived} onClick={() => edit({
                                                id: a.id, roleId: a.roleId, actorKind: a.actorKind, displayName: a.displayName,
                                                connectionId: a.connectionId, model: a.model, instructions: a.overrides?.instructions ?? '',
                                            })}>{t.common.edit}</Button>
                                        </span>
                                    </td>
                                </tr>
                            );
                        })}
                        </tbody>
                    </table>
                </div>
            )}
        </div>
    );
}

// AssignmentForm edits one assignment in the detail pane.
function AssignmentForm({project, initial, onDone}: { project: Project; initial: Form; onDone: () => void }) {
    const roles = useLoad(() => api.roles(), []);
    const conns = useLoad(() => api.connections(), []);
    const [form, setForm] = useState<Form>(initial);
    const action = useAction();
    const toast = useToast();
    const setDetail = useDetail();
    const conn = (id: string) => conns.data?.find((c) => c.id === id);
    const save = async () => {
        const saved = await action.run(() => api.saveAssignment({
            id: form.id, projectId: project.id, roleId: form.roleId, actorKind: form.actorKind, displayName: form.displayName,
            connectionId: form.actorKind === 'ai' ? form.connectionId : '', model: form.actorKind === 'ai' ? form.model : '',
            overrides: {instructions: form.instructions},
        }));
        if (saved) {
            toast(`‘${form.displayName}’을 저장했습니다`);
            onDone();
        }
    };
    return (
        <form className="stack" aria-label="담당자 편집" onSubmit={(e) => { e.preventDefault(); save(); }}>
            <DetailHeader title={form.id ? '담당자 편집' : '새 담당자'} onClose={() => setDetail(null)}/>
            <Field label="역할" hint={roles.data?.length === 0 ? '먼저 ‘회사 조직’에서 역할을 만드세요.' : undefined}>
                <select value={form.roleId} onChange={(e) => {
                    const name = roles.data?.find((r) => r.id === e.target.value)?.name ?? '';
                    setForm({...form, roleId: e.target.value, displayName: form.displayName || (name ? `${name} ${form.actorKind === 'ai' ? 'AI' : ''}`.trim() : '')});
                }}>
                    <option value="">역할 선택</option>
                    {(roles.data ?? []).map((r) => <option key={r.id} value={r.id}>{r.name}</option>)}
                </select></Field>
            <fieldset className="choice-group">
                <legend>맡는 사람</legend>
                <div className="grid-2">
                    <label className="choice"><input type="radio" name="actor" checked={form.actorKind === 'ai'} onChange={() => setForm({...form, actorKind: 'ai'})}/>
                        <span className="choice-title">AI</span></label>
                    <label className="choice"><input type="radio" name="actor" checked={form.actorKind === 'human'} onChange={() => setForm({...form, actorKind: 'human'})}/>
                        <span className="choice-title">나 (사람)</span></label>
                </div>
            </fieldset>
            <Field label="표시 이름"><input value={form.displayName} onChange={(e) => setForm({...form, displayName: e.target.value})}/></Field>
            {form.actorKind === 'ai' ? (
                <>
                    <Field label="AI 연결" hint={conn(form.connectionId)?.note}>
                        <select value={form.connectionId} onChange={(e) => setForm({...form, connectionId: e.target.value})}>
                            <option value="">연결 선택</option>
                            {(conns.data ?? []).map((c) => <option key={c.id} value={c.id}>{c.name}{c.usable ? '' : ' (확인 필요)'}</option>)}
                        </select></Field>
                    {conns.data && !conns.data.some((c) => c.provider === 'test') ? (
                        <div><Button size="sm" variant="ghost" icon={FlaskConical} onClick={async () => {
                            const c = await action.run(() => api.ensureTestConnection());
                            if (c) { conns.reload(); setForm({...form, connectionId: c.id, model: 'auto'}); }
                        }}>테스트 AI 연결 만들기 (가짜 응답)</Button></div>
                    ) : null}
                    <Field label="모델 / 시나리오" hint="테스트 연결은 auto">
                        <input value={form.model} onChange={(e) => setForm({...form, model: e.target.value})}/></Field>
                </>
            ) : <p className="muted small">사람 담당자는 이 앱의 사용자 본인입니다.</p>}
            <Field label="이 서비스에서만 추가할 지침" optional>
                <textarea value={form.instructions} onChange={(e) => setForm({...form, instructions: e.target.value})}/></Field>
            <ErrorBox error={action.error}/>
            <div className="form-actions">
                <Button variant="primary" type="submit" busy={action.busy} disabled={!form.roleId || !form.displayName.trim()}>{t.common.save}</Button>
                <Button onClick={() => setDetail(null)}>{t.common.cancel}</Button>
            </div>
        </form>
    );
}
