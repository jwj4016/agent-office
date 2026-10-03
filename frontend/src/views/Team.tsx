import {useState} from 'react';
import {api} from '../api';
import {t} from '../i18n';
import {useLoad} from '../live';
import type {ActorKind, Assignment, Project} from '../types';
import {ActorBadge, ErrorBox, useAction, useDetail} from '../ui';

type Form = {
    id: string; roleId: string; actorKind: ActorKind; displayName: string;
    connectionId: string; model: string; instructions: string;
};

const empty: Form = {id: '', roleId: '', actorKind: 'ai', displayName: '', connectionId: '', model: 'auto', instructions: ''};

export function Team({project}: { project: Project }) {
    const list = useLoad(() => api.assignments(project.id), [project.id], project.id);
    const roles = useLoad(() => api.roles(), []);
    const conns = useLoad(() => api.connections(), []);
    const [form, setForm] = useState<Form | null>(null);
    const action = useAction();
    const setDetail = useDetail();
    const archived = project.status === 'archived';
    const roleName = (id: string) => roles.data?.find((r) => r.id === id)?.name ?? '(삭제된 역할)';
    const conn = (id: string) => conns.data?.find((c) => c.id === id);

    const showPreview = async (a: Assignment) => {
        const p = await action.run(() => api.instructionPreview(project.id, a.id));
        if (!p) return;
        setDetail(
            <div className="stack">
                <div className="row between"><h2>{a.displayName}의 최종 지침</h2>
                    <button className="btn small" onClick={() => setDetail(null)}>{t.common.close}</button></div>
                <p className="muted small">회사 → 상위 역할 → 역할 → 서비스 → 담당자 순서로 합쳐집니다. 업무 지시와 수정 의견은 실행 때 덧붙습니다.</p>
                {p.layers.map((l, i) => (
                    <div key={i} className="card">
                        <div className="small muted">{l.name}</div>
                        <div style={{whiteSpace: 'pre-wrap'}}>{l.text || <span className="muted">(비어 있음)</span>}</div>
                    </div>
                ))}
            </div>,
        );
    };

    const save = async () => {
        if (!form) return;
        const saved = await action.run(() => api.saveAssignment({
            id: form.id, projectId: project.id, roleId: form.roleId, actorKind: form.actorKind, displayName: form.displayName,
            connectionId: form.actorKind === 'ai' ? form.connectionId : '', model: form.actorKind === 'ai' ? form.model : '',
            overrides: {instructions: form.instructions},
        }));
        if (saved) {
            setForm(null);
            list.reload();
        }
    };

    return (
        <div className="stack">
            <div className="row between">
                <p className="muted" style={{margin: 0}}>회사 역할을 이 서비스에서 누가 맡을지 정합니다. 배정만으로 AI가 실행되지는 않습니다.</p>
                <button className="btn" disabled={archived} onClick={() => setForm({...empty})}>담당자 추가</button>
            </div>
            <ErrorBox error={list.error || action.error}/>
            <table className="list">
                <thead><tr><th>담당자</th><th>역할</th><th>종류</th><th>AI 연결</th><th/></tr></thead>
                <tbody>
                {(list.data ?? []).map((a) => {
                    const c = conn(a.connectionId);
                    return (
                        <tr key={a.id}>
                            <td>{a.displayName}</td>
                            <td>{roleName(a.roleId)}</td>
                            <td><ActorBadge kind={a.actorKind}/></td>
                            <td className="small">
                                {a.actorKind === 'human' ? <span className="muted">나 (local-owner)</span>
                                    : c ? <>{c.name} <span className={`badge ${c.usable ? 'ok' : 'bad'}`}>{c.usable ? '사용 가능' : '확인 필요'}</span>
                                        {a.model ? <span className="muted"> · {a.model}</span> : null}</>
                                        : <span className="badge bad">연결 없음</span>}
                            </td>
                            <td><div className="row">
                                <button className="btn small" onClick={() => showPreview(a)}>지침 보기</button>
                                <button className="btn small" disabled={archived} onClick={() => setForm({
                                    id: a.id, roleId: a.roleId, actorKind: a.actorKind, displayName: a.displayName,
                                    connectionId: a.connectionId, model: a.model, instructions: a.overrides?.instructions ?? '',
                                })}>{t.common.edit}</button>
                            </div></td>
                        </tr>
                    );
                })}
                </tbody>
            </table>
            {list.data?.length === 0 ? <p className="muted small">아직 담당자가 없습니다.</p> : null}

            {form ? (
                <form className="card stack" aria-label="담당자 편집" onSubmit={(e) => { e.preventDefault(); save(); }} style={{maxWidth: 560}}>
                    <h2>{form.id ? '담당자 편집' : '새 담당자'}</h2>
                    <label className="field"><span>역할</span>
                        <select value={form.roleId} onChange={(e) => {
                            const name = roles.data?.find((r) => r.id === e.target.value)?.name ?? '';
                            setForm({...form, roleId: e.target.value, displayName: form.displayName || (name ? `${name} ${form.actorKind === 'ai' ? 'AI' : ''}`.trim() : '')});
                        }}>
                            <option value="">역할 선택</option>
                            {(roles.data ?? []).map((r) => <option key={r.id} value={r.id}>{r.name}</option>)}
                        </select></label>
                    {roles.data?.length === 0 ? <p className="muted small">먼저 ‘회사 조직’에서 역할을 만드세요.</p> : null}
                    <fieldset className="row" style={{border: 'none', padding: 0}}>
                        <legend className="small muted">맡는 사람</legend>
                        <label className="check"><input type="radio" checked={form.actorKind === 'ai'} onChange={() => setForm({...form, actorKind: 'ai'})}/> AI</label>
                        <label className="check"><input type="radio" checked={form.actorKind === 'human'} onChange={() => setForm({...form, actorKind: 'human'})}/> 나 (사람)</label>
                    </fieldset>
                    <label className="field"><span>표시 이름</span>
                        <input value={form.displayName} onChange={(e) => setForm({...form, displayName: e.target.value})}/></label>
                    {form.actorKind === 'ai' ? (
                        <>
                            <label className="field"><span>AI 연결</span>
                                <select value={form.connectionId} onChange={(e) => setForm({...form, connectionId: e.target.value})}>
                                    <option value="">연결 선택</option>
                                    {(conns.data ?? []).map((c) => <option key={c.id} value={c.id}>{c.name}{c.usable ? '' : ' (확인 필요)'}</option>)}
                                </select></label>
                            {conn(form.connectionId) ? <p className="muted small">{conn(form.connectionId)!.note}</p> : null}
                            {!(conns.data ?? []).some((c) => c.provider === 'test') ? (
                                <button type="button" className="btn small" onClick={async () => {
                                    const c = await action.run(() => api.ensureTestConnection());
                                    if (c) { conns.reload(); setForm({...form, connectionId: c.id, model: 'auto'}); }
                                }}>테스트 AI 연결 만들기 (가짜 응답)</button>
                            ) : null}
                            <label className="field"><span>모델 / 시나리오</span>
                                <input value={form.model} onChange={(e) => setForm({...form, model: e.target.value})}
                                       placeholder="테스트 연결은 auto"/></label>
                        </>
                    ) : <p className="muted small">사람 담당자는 이 앱의 사용자 본인입니다.</p>}
                    <label className="field"><span>이 서비스에서만 추가할 지침</span>
                        <textarea value={form.instructions} onChange={(e) => setForm({...form, instructions: e.target.value})}/></label>
                    <div className="row">
                        <button className="btn primary" type="submit" disabled={action.busy || !form.roleId || !form.displayName.trim()}>{t.common.save}</button>
                        <button className="btn" type="button" onClick={() => setForm(null)}>{t.common.cancel}</button>
                    </div>
                </form>
            ) : null}
        </div>
    );
}
