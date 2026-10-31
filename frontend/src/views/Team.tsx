import {Bot, Building, CalendarClock, Copy, FileText, FlaskConical, GripVertical, LayoutTemplate, Pencil, Plus, Radio, UserPlus, Users} from 'lucide-react';
import {type DragEvent, useState} from 'react';
import {api} from '../api';
import {t} from '../i18n';
import {useLoad} from '../live';
import type {ActiveStatus, ActorKind, Assignment, Department, DepartmentTemplate, Project, Role} from '../types';
import {ServiceDeskSettings} from './IntakeDesk';
import {OrganizeService} from './Organize';
import {AutomationPanel, rulesOf} from './Automation';
import {SubscriptionPanel} from './Judgments';
import {scheduleLabel} from './Schedule';
import {ActorBadge, Banner, Button, DetailHeader, EmptyState, ErrorBox, Field, Icon, Loading, Section, StatusBadge, useAction, useDetail, useToast} from '../ui';

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
                    <p className="muted small">회사 → 상위 역할 → 역할 → 서비스 → 부서 → 담당자 순서로 합쳐집니다. 업무 지시와 수정 의견은 실행 때 덧붙습니다.</p>
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
        <div className="stack loose">
            <OrganizeService project={project} onApplied={list.reload}/>
            <ServiceDepartments project={project} assignments={list.data} roles={roles.data ?? []} onChanged={list.reload}/>
            <ServiceDeskSettings project={project}/>
            <Section title={<><Icon icon={Users} size={18}/>담당자</>} label="담당자" count={list.data.length}>
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
            </Section>
        </div>
    );
}

// TEMPLATE_MIME carries a department template id while it is dragged.
export const TEMPLATE_MIME = 'application/x-agent-office-department-template';

// ServiceDepartments lists this service's departments. A service is one
// company, so its departments are its own; company templates can be
// copied in with a button or by dragging them onto the list.
function ServiceDepartments({project, assignments, roles, onChanged}: {
    project: Project; assignments: Assignment[]; roles: Role[]; onChanged: () => void;
}) {
    const list = useLoad(() => api.departments(project.id), [project.id], project.id);
    const schedules = useLoad(() => api.schedules(project.id), [project.id], project.id);
    const templates = useLoad(() => api.departmentTemplates(), []);
    const setDetail = useDetail();
    const action = useAction();
    const toast = useToast();
    const [over, setOver] = useState(false);
    const archived = project.status === 'archived';
    const person = (id: string) => assignments.find((a) => a.id === id)?.displayName;
    const reload = () => { list.reload(); onChanged(); };
    const edit = (d: Partial<Department>) => setDetail(
        <DepartmentForm key={d.id || 'new'} project={project} initial={d} departments={list.data ?? []} assignments={assignments}
                        onDone={() => { setDetail(null); reload(); }} onTemplate={() => templates.reload()}/>);
    const importTemplate = (tpl: DepartmentTemplate) => action.run(async () => {
        const d = await api.importDepartmentTemplate(project.id, tpl.id);
        reload();
        toast(`‘${tpl.name}’ 템플릿으로 ‘${d.name}’ 부서를 만들었습니다${d.memberIds.length ? ` (구성원 ${d.memberIds.length}명)` : ''}`);
    });
    const drop = (e: DragEvent<HTMLElement>) => {
        e.preventDefault();
        setOver(false);
        const tpl = (templates.data ?? []).find((x) => x.id === e.dataTransfer.getData(TEMPLATE_MIME));
        if (tpl && !archived) importTemplate(tpl);
    };
    const dragging = (e: DragEvent<HTMLElement>) => {
        if (!archived && e.dataTransfer.types.includes(TEMPLATE_MIME)) {
            e.preventDefault();
            e.dataTransfer.dropEffect = 'copy';
            setOver(true);
        }
    };
    const roleNames = (ids: string[]) => ids.map((id) => roles.find((r) => r.id === id)?.name).filter(Boolean).join(', ');
    return (
        <Section title={<><Icon icon={Building} size={18}/>부서</>} label="부서" count={list.data?.length}
                 actions={<Button icon={Plus} disabled={archived || !list.data} onClick={() => edit({})}>부서 만들기</Button>}>
            <p className="muted small">서비스는 하나의 회사입니다. 부서와 구성원은 이 서비스에만 속합니다.</p>
            <ErrorBox error={list.error || templates.error || action.error}/>
            <div className={`dept-drop ${over ? 'over' : ''}`} onDragOver={dragging} onDragEnter={dragging}
                 onDragLeave={() => setOver(false)} onDrop={drop} aria-label="부서 목록 (템플릿을 끌어다 놓으면 부서를 만듭니다)">
                {list.data && list.data.length === 0 ? (
                    <p className="muted small">아직 부서가 없습니다. 새로 만들거나 아래 템플릿을 끌어다 놓으세요.</p>
                ) : null}
                {list.data && list.data.length ? (
                    <div className="table-wrap">
                        <table className="list">
                            <thead><tr><th>부서</th><th>책임</th><th>구성원</th><th>대표 담당자</th><th>상태</th><th/></tr></thead>
                            <tbody>
                            {list.data.map((d) => (
                                <tr key={d.id}>
                                    <td><strong>{d.name}</strong></td>
                                    <td className="small pre">{d.responsibility || <span className="muted">적지 않음</span>}</td>
                                    <td className="small">{d.memberIds.map(person).filter(Boolean).join(', ') || <span className="muted">없음</span>}</td>
                                    <td className="small">{person(d.leadAssignmentId) ?? <span className="muted">지정 안 함</span>}</td>
                                    <td><span className="row nowrap">
                                        <StatusBadge tone={d.status === 'active' ? 'ok' : ''}>{d.status === 'active' ? '사용' : '사용 안 함'}</StatusBadge>
                                        {rulesOf(d).autoStart || rulesOf(d).autoCollab ? <StatusBadge tone="ai" icon={Bot}>자동화</StatusBadge> : null}
                                        {(schedules.data ?? []).filter((s) => s.departmentId === d.id && s.enabled).map((s) => (
                                            <StatusBadge key={s.id} tone="info" icon={CalendarClock}>{scheduleLabel(s)}</StatusBadge>
                                        ))}
                                    </span></td>
                                    <td className="actions"><span className="row nowrap" style={{justifyContent: 'flex-end'}}>
                                        <Button size="sm" icon={Radio} aria-label={`${d.name} 판단 설정`}
                                                onClick={() => setDetail(<SubscriptionPanel key={`sub-${d.id}`} project={project} department={d}/>)}>판단 설정</Button>
                                        <Button size="sm" icon={Bot} aria-label={`${d.name} 자동화`}
                                                onClick={() => setDetail(<AutomationPanel key={`auto-${d.id}-${d.updatedAt}`} project={project} department={d}/>)}>자동화</Button>
                                        <Button size="sm" icon={Pencil} disabled={archived}
                                                aria-label={`${d.name} 부서 편집`} onClick={() => edit(d)}>{t.common.edit}</Button>
                                    </span></td>
                                </tr>
                            ))}
                            </tbody>
                        </table>
                    </div>
                ) : null}
            </div>
            <div className="stack tight">
                <span className="field-label row nowrap"><Icon icon={LayoutTemplate} size={16}/>회사 부서 템플릿</span>
                {templates.data && templates.data.length === 0 ? (
                    <p className="muted small">회사 조직에서 부서 템플릿을 만들면 여기서 가져올 수 있습니다.</p>
                ) : null}
                <ul className="row" aria-label="부서 템플릿" style={{listStyle: 'none', padding: 0, margin: 0}}>
                    {(templates.data ?? []).map((tpl) => (
                        <li key={tpl.id} className="card row nowrap dept-template" draggable={!archived}
                            onDragStart={(e) => { e.dataTransfer.setData(TEMPLATE_MIME, tpl.id); e.dataTransfer.effectAllowed = 'copy'; }}
                            title={[tpl.responsibility, roleNames(tpl.roleIds) && `추천 역할: ${roleNames(tpl.roleIds)}`].filter(Boolean).join('\n')}>
                            <Icon icon={GripVertical} size={14} className="muted"/>
                            <span className="stack tight" style={{minWidth: 0}}>
                                <strong className="small truncate">{tpl.name}</strong>
                                {tpl.roleIds.length ? <span className="muted xsmall truncate">{roleNames(tpl.roleIds)}</span> : null}
                            </span>
                            <Button size="sm" variant="ghost" icon={Copy} busy={action.busy} disabled={archived}
                                    aria-label={`${tpl.name} 템플릿 가져오기`} onClick={() => importTemplate(tpl)}>가져오기</Button>
                        </li>
                    ))}
                </ul>
                {templates.data?.length ? (
                    <p className="muted xsmall">가져오면 이 서비스의 부서가 새로 만들어집니다. 템플릿의 추천 역할을 맡은 담당자 중 아직 부서가 없는 사람이 구성원이 됩니다.
                        템플릿을 나중에 고쳐도 가져온 부서는 바뀌지 않습니다.</p>
                ) : null}
            </div>
        </Section>
    );
}

function DepartmentForm({project, initial, departments, assignments, onDone, onTemplate}: {
    project: Project; initial: Partial<Department>; departments: Department[]; assignments: Assignment[];
    onDone: () => void; onTemplate: () => void;
}) {
    const [form, setForm] = useState({
        name: initial.name ?? '', responsibility: initial.responsibility ?? '', leadAssignmentId: initial.leadAssignmentId ?? '',
        memberIds: initial.memberIds ?? [] as string[], status: (initial.status ?? 'active') as ActiveStatus,
    });
    const action = useAction();
    const toast = useToast();
    const setDetail = useDetail();
    const editing = !!initial.id;
    const elsewhere = (assignmentId: string) => departments.find((d) => d.id !== initial.id && d.memberIds.includes(assignmentId))?.name;
    const toggle = (id: string, on: boolean) => {
        const memberIds = on ? [...form.memberIds, id] : form.memberIds.filter((x) => x !== id);
        setForm({...form, memberIds, leadAssignmentId: memberIds.includes(form.leadAssignmentId) ? form.leadAssignmentId : ''});
    };
    const save = async () => {
        const saved = await action.run(() => api.saveDepartment({id: initial.id, projectId: project.id, ...form}));
        if (saved) {
            toast(`‘${saved.name}’ 부서를 저장했습니다`);
            onDone();
        }
    };
    const asTemplate = () => action.run(async () => {
        const tpl = await api.saveDepartmentAsTemplate(project.id, initial.id ?? '', form.name);
        onTemplate();
        toast(`‘${tpl.name}’ 부서 템플릿으로 저장했습니다`);
    });
    return (
        <form className="stack" aria-label="부서 편집" onSubmit={(e) => { e.preventDefault(); save(); }}>
            <DetailHeader title={editing ? `${initial.name} 부서` : '새 부서'} onClose={() => setDetail(null)}/>
            <Field label="이름"><input value={form.name} autoFocus onChange={(e) => setForm({...form, name: e.target.value})}/></Field>
            <Field label="책임" optional hint="이 부서 구성원의 지침에 ‘부서’ 층으로 들어갑니다.">
                <textarea value={form.responsibility} rows={3} onChange={(e) => setForm({...form, responsibility: e.target.value})}/></Field>
            <fieldset className="stack tight">
                <legend className="field-label">구성원 <span className="optional">(선택)</span></legend>
                {assignments.length === 0 ? <span className="muted small">아래 ‘담당자’에서 담당자를 먼저 추가하세요.</span> : null}
                {assignments.map((a) => {
                    const other = elsewhere(a.id);
                    const checked = form.memberIds.includes(a.id);
                    return (
                        <label key={a.id} className="check block">
                            <input type="checkbox" checked={checked} onChange={(e) => toggle(a.id, e.target.checked)}/>
                            {a.displayName}
                            {other ? <span className="muted xsmall">{checked ? `(${other}에서 옮겨집니다)` : `(현재 ${other})`}</span> : null}
                        </label>
                    );
                })}
            </fieldset>
            <Field label="대표 담당자" optional hint="구성원 중 한 명. 부서의 판단과 기본 업무 실행을 맡습니다.">
                <select value={form.leadAssignmentId} onChange={(e) => setForm({...form, leadAssignmentId: e.target.value})}>
                    <option value="">지정 안 함</option>
                    {assignments.filter((a) => form.memberIds.includes(a.id)).map((a) => <option key={a.id} value={a.id}>{a.displayName}</option>)}
                </select></Field>
            {editing ? (
                <label className="check">
                    <input type="checkbox" checked={form.status === 'active'}
                           onChange={(e) => setForm({...form, status: e.target.checked ? 'active' : 'inactive'})}/>
                    사용 (끄면 새 업무를 맡지 않고, 맡은 업무는 그대로 남습니다)
                </label>
            ) : null}
            <ErrorBox error={action.error}/>
            <div className="form-actions">
                <Button variant="primary" type="submit" busy={action.busy} disabled={!form.name.trim()}>{t.common.save}</Button>
                {editing ? <Button icon={LayoutTemplate} busy={action.busy} onClick={asTemplate}>템플릿으로 저장</Button> : null}
                <Button onClick={() => setDetail(null)}>{t.common.cancel}</Button>
            </div>
        </form>
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
