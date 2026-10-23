import {CalendarClock, Flag, ListTodo, MessageSquarePlus, Pencil, Plus, Target} from 'lucide-react';
import {useState} from 'react';
import {api} from '../api';
import {t} from '../i18n';
import {useLoad} from '../live';
import type {Assignment, Project, Department, WorkFilter, WorkItem, WorkKind, WorkNote, WorkPriority, WorkStatus} from '../types';
import {
    Banner, Button, DetailHeader, EmptyState, ErrorBox, Field, Icon, Loading, Section, StatusBadge, type Tone, shortTime, useAction, useDetail,
    useToast,
} from '../ui';
import {DeliveryInbox, WorkEvents} from './Deliveries';
import {WorkDocuments} from './Documents';
import {WorkRun} from './WorkRuns';

export const workStatuses: WorkStatus[] = ['received', 'ready', 'in_progress', 'waiting', 'blocked', 'done', 'cancelled'];
export const workPriorities: WorkPriority[] = ['urgent', 'high', 'normal', 'low'];

export const workTone: Record<WorkStatus, Tone> = {
    received: '', ready: 'info', in_progress: 'info', waiting: 'warn', blocked: 'bad', done: 'ok', cancelled: '',
};
const priorityTone: Record<WorkPriority, Tone> = {urgent: 'bad', high: 'warn', normal: '', low: ''};

export const isClosed = (s: WorkStatus) => s === 'done' || s === 'cancelled';
export const needsReason = (s: WorkStatus) => s === 'waiting' || s === 'blocked';

// today is the local date in the YYYY-MM-DD form due dates use.
export function today(now = new Date()) {
    const p = (n: number) => String(n).padStart(2, '0');
    return `${now.getFullYear()}-${p(now.getMonth() + 1)}-${p(now.getDate())}`;
}

export const overdue = (w: WorkItem, day = today()) => !!w.dueAt && !isClosed(w.status) && w.dueAt < day;

// goalProgress counts a goal's work by whether it is closed.
export function goalProgress(goalId: string, all: WorkItem[]) {
    const children = all.filter((w) => w.parentId === goalId);
    return {total: children.length, done: children.filter((w) => w.status === 'done').length, open: children.filter((w) => !isClosed(w.status)).length};
}

export function WorkStatusBadge({status, reason}: { status: WorkStatus; reason?: string }) {
    return <StatusBadge tone={workTone[status]}>{t.workStatus[status]}{reason ? ` · ${reason}` : ''}</StatusBadge>;
}

type Context = {
    project: Project; departments: Department[]; categories: string[]; assignments: Assignment[]; goals: WorkItem[];
    onOpenRun: (runId: string) => void;
};

const deptName = (ctx: Context, id: string) => ctx.departments.find((d) => d.id === id)?.name ?? '';
const personName = (ctx: Context, id: string) => ctx.assignments.find((a) => a.id === id)?.displayName ?? '';
const goalTitle = (ctx: Context, id: string) => ctx.goals.find((g) => g.id === id)?.title ?? '';

const defaultFilter: WorkFilter = {status: 'open'};

// Work is the service's work list: shared goals and each department's
// work, filtered by status, department, priority, category and due date.
export function Work({project, onTeam, onOpenRun}: { project: Project; onTeam: () => void; onOpenRun: (runId: string) => void }) {
    const [filter, setFilter] = useState<WorkFilter>(defaultFilter);
    const departments = useLoad(() => api.departments(project.id), [project.id], project.id);
    const categories = useLoad(() => api.categories(project.id), [project.id], project.id);
    const assignments = useLoad(() => api.assignments(project.id), [project.id], project.id);
    const all = useLoad(() => api.workItems(project.id), [project.id], project.id);
    const list = useLoad(() => api.workItems(project.id, {...filter, kind: 'work'}), [project.id, JSON.stringify(filter)], project.id);
    const setDetail = useDetail();
    const archived = project.status === 'archived';

    const error = departments.error || categories.error || assignments.error || all.error || list.error;
    if (error) return <ErrorBox error={error}/>;
    if (!departments.data || !categories.data || !assignments.data || !all.data) return <Loading/>;

    const goals = all.data.filter((w) => w.kind === 'goal');
    const ctx: Context = {project, departments: departments.data, categories: categories.data, assignments: assignments.data, goals, onOpenRun};
    const refresh = () => { all.reload(); list.reload(); };
    const open = (id: string) => setDetail(<WorkPanel key={id} ctx={ctx} id={id} onChanged={refresh}/>);
    const create = (kind: WorkKind, parentId = '') => setDetail(
        <WorkForm key={`new-${kind}-${parentId}`} ctx={ctx} initial={{...emptyWork, kind, parentId}}
                  onSaved={(w) => { refresh(); open(w.id); }}/>);
    const activeDepts = departments.data.filter((d) => d.status === 'active');
    const openGoals = goals.filter((g) => !isClosed(g.status));
    const set = (patch: Partial<WorkFilter>) => setFilter({...filter, ...patch});
    const filtered = JSON.stringify(filter) !== JSON.stringify(defaultFilter);

    return (
        <div className="stack loose">
            <div className="row between">
                <p className="muted">부서가 맡은 일과 여러 부서의 일을 묶는 공동 목표입니다. 업무는 사람이 완료를 확인할 때까지 남습니다.</p>
                <span className="row nowrap">
                    <Button icon={Target} disabled={archived || activeDepts.length === 0} onClick={() => create('goal')}>공동 목표 추가</Button>
                    <Button variant="primary" icon={Plus} disabled={archived} onClick={() => create('work')}>업무 추가</Button>
                </span>
            </div>
            {activeDepts.length === 0 ? (
                <Banner tone="info" title="아직 이 서비스에 부서가 없습니다"
                        actions={<Button size="sm" onClick={onTeam}>부서 만들기</Button>}>
                    부서를 만들거나 회사 부서 템플릿에서 가져오면 업무를 부서에 맡기고 공동 목표의 책임 부서를 정할 수 있습니다.
                </Banner>
            ) : null}

            <Section title={<><Icon icon={Target} size={18}/>공동 목표</>} label="공동 목표" count={openGoals.length}>
                {openGoals.length === 0 ? <p className="muted small">진행 중인 공동 목표가 없습니다.</p> : (
                    <div className="grid-cards">
                        {openGoals.map((g) => {
                            const p = goalProgress(g.id, all.data ?? []);
                            return (
                                <button key={g.id} className="card clickable stack tight" style={{textAlign: 'left'}} onClick={() => open(g.id)}
                                        aria-label={`${g.title} 공동 목표 보기`}>
                                    <span className="row between nowrap"><strong className="truncate">{g.title}</strong><WorkStatusBadge status={g.status}/></span>
                                    <span className="muted small">책임: {deptName(ctx, g.departmentId) || '없음'}{g.dueAt ? ` · 기한 ${g.dueAt}` : ''}</span>
                                    <span className="small">하위 업무 {p.total}건 · 완료 {p.done} · 진행 중 {p.open}</span>
                                </button>
                            );
                        })}
                    </div>
                )}
            </Section>

            <Section title={<><Icon icon={ListTodo} size={18}/>업무</>} label="업무" count={list.data?.length}>
                <div className="toolbar work-filters" role="group" aria-label="업무 조회 조건">
                    <label className="row nowrap small">상태
                        <select value={filter.status ?? ''} onChange={(e) => set({status: e.target.value})}>
                            <option value="open">진행 중인 업무</option>
                            <option value="closed">닫힌 업무</option>
                            <option value="">전체</option>
                            {workStatuses.map((s) => <option key={s} value={s}>{t.workStatus[s]}</option>)}
                        </select></label>
                    <label className="row nowrap small">부서
                        <select value={filter.departmentId ?? ''} onChange={(e) => set({departmentId: e.target.value})}>
                            <option value="">모든 부서</option>
                            {ctx.departments.map((d) => <option key={d.id} value={d.id}>{d.name}</option>)}
                        </select></label>
                    <label className="row nowrap small">우선순위
                        <select value={filter.priority ?? ''} onChange={(e) => set({priority: e.target.value})}>
                            <option value="">전체</option>
                            {workPriorities.map((p) => <option key={p} value={p}>{t.workPriority[p]}</option>)}
                        </select></label>
                    <label className="row nowrap small">분류
                        <select value={filter.category ?? ''} onChange={(e) => set({category: e.target.value})}>
                            <option value="">전체</option>
                            {ctx.categories.map((c) => <option key={c} value={c}>{c}</option>)}
                        </select></label>
                    <label className="row nowrap small">기한
                        <input type="date" value={filter.dueBefore ?? ''} aria-label="이 날짜까지 기한인 업무"
                               onChange={(e) => set({dueBefore: e.target.value})}/></label>
                    {filtered ? <Button size="sm" variant="ghost" onClick={() => setFilter(defaultFilter)}>조건 초기화</Button> : null}
                </div>
                <ErrorBox error={list.error}/>
                {!list.data ? <Loading/> : list.data.length === 0 ? (
                    <EmptyState compact icon={ListTodo} title={filtered ? '조건에 맞는 업무가 없습니다' : '아직 업무가 없습니다'}
                                actions={!filtered && !archived ? <Button variant="primary" icon={Plus} onClick={() => create('work')}>업무 추가</Button> : null}>
                        {filtered ? '조회 조건을 바꿔 보세요.' : '부서가 맡을 일을 등록하세요. 공동 목표 아래에 둘 수도 있습니다.'}
                    </EmptyState>
                ) : (
                    <div className="table-wrap">
                        <table className="list">
                            <thead><tr><th>업무</th><th>부서</th><th>상태</th><th>우선순위</th><th>기한</th><th>분류</th></tr></thead>
                            <tbody>
                            {list.data.map((w) => (
                                <tr key={w.id}>
                                    <td>
                                        <button className="btn ghost small" onClick={() => open(w.id)} aria-label={`${w.title} 업무 보기`}>
                                            <strong>{w.title}</strong></button>
                                        {w.parentId ? <div className="muted xsmall">목표: {goalTitle(ctx, w.parentId)}</div> : null}
                                    </td>
                                    <td className="small">{deptName(ctx, w.departmentId) || <span className="muted">미배정</span>}</td>
                                    <td><WorkStatusBadge status={w.status} reason={w.statusReason}/></td>
                                    <td><StatusBadge tone={priorityTone[w.priority]}>{t.workPriority[w.priority]}</StatusBadge></td>
                                    <td className="small nowrap">{w.dueAt ? (
                                        overdue(w) ? <StatusBadge tone="bad" icon={CalendarClock}>{w.dueAt} 지남</StatusBadge> : w.dueAt
                                    ) : <span className="muted">—</span>}</td>
                                    <td className="small">{w.categories.join(', ') || <span className="muted">—</span>}</td>
                                </tr>
                            ))}
                            </tbody>
                        </table>
                    </div>
                )}
            </Section>

            <DeliveryInbox project={project} departments={departments.data}/>
        </div>
    );
}

// WorkPanel shows one work item or goal in the detail pane: its fields,
// status, the work under a goal, and the history.
function WorkPanel({ctx, id, onChanged}: { ctx: Context; id: string; onChanged: () => void }) {
    const d = useLoad(() => api.workItem(ctx.project.id, id), [ctx.project.id, id]);
    const setDetail = useDetail();
    const archived = ctx.project.status === 'archived';
    if (d.error) return <><DetailHeader title="업무" onClose={() => setDetail(null)}/><ErrorBox error={d.error}/></>;
    if (!d.data) return <Loading/>;
    const w = d.data.item;
    const changed = () => { d.reload(); onChanged(); };
    const reopen = () => setDetail(<WorkPanel key={id} ctx={ctx} id={id} onChanged={onChanged}/>);
    const edit = () => setDetail(<WorkForm key={`edit-${id}`} ctx={ctx} initial={w} onSaved={() => { onChanged(); reopen(); }}/>);
    const addChild = () => setDetail(<WorkForm key={`new-under-${id}`} ctx={ctx} initial={{...emptyWork, kind: 'work', parentId: id}}
                                               onSaved={(c) => { onChanged(); setDetail(<WorkPanel key={c.id} ctx={ctx} id={c.id} onChanged={onChanged}/>); }}/>);
    return (
        <div className="stack">
            <DetailHeader title={<>{w.kind === 'goal' ? <Icon icon={Target} size={18}/> : null}{w.title}</>} onClose={() => setDetail(null)}>
                <div className="row">
                    <StatusBadge tone="info">{w.kind === 'goal' ? '공동 목표' : '업무'}</StatusBadge>
                    <WorkStatusBadge status={w.status} reason={w.statusReason}/>
                    <StatusBadge tone={priorityTone[w.priority]} icon={Flag}>{t.workPriority[w.priority]}</StatusBadge>
                </div>
            </DetailHeader>
            <dl className="kv">
                <dt>{w.kind === 'goal' ? '책임 부서' : '담당 부서'}</dt><dd>{deptName(ctx, w.departmentId) || '미배정'}</dd>
                <dt>담당자</dt><dd>{personName(ctx, w.assigneeAssignmentId) || '—'}</dd>
                {w.kind === 'work' ? <><dt>공동 목표</dt><dd>{w.parentId ? (
                    <button className="btn ghost small" onClick={() => setDetail(<WorkPanel key={w.parentId} ctx={ctx} id={w.parentId} onChanged={onChanged}/>)}>
                        {goalTitle(ctx, w.parentId) || '목표 보기'}</button>) : '—'}</dd></> : null}
                <dt>기한</dt><dd>{w.dueAt ? <>{w.dueAt}{overdue(w) ? <> <StatusBadge tone="bad">지남</StatusBadge></> : null}</> : '—'}</dd>
                <dt>분류</dt><dd>{w.categories.join(', ') || '—'}</dd>
                <dt>완료 조건</dt><dd className="pre">{w.doneCriteria || <span className="muted">적지 않음</span>}</dd>
                <dt>설명</dt><dd className="pre">{w.description || <span className="muted">적지 않음</span>}</dd>
            </dl>
            <div><Button size="sm" icon={Pencil} disabled={archived} onClick={edit}>{t.common.edit}</Button></div>

            <StatusChanger key={`${w.status}-${w.updatedAt}`} ctx={ctx} item={w} disabled={archived} onChanged={changed}/>

            {w.kind === 'work' ? (
                <WorkRun project={ctx.project} item={w} assignments={ctx.assignments} departments={ctx.departments} categories={ctx.categories}
                         onChanged={changed} onOpenRun={ctx.onOpenRun}/>
            ) : null}
            {w.kind === 'work' ? (
                <WorkDocuments project={ctx.project} item={w}/>
            ) : null}

            {w.kind === 'goal' ? (
                <Section title="이 목표의 업무" count={d.data.children.length}
                         actions={<Button size="sm" icon={Plus} disabled={archived} onClick={addChild}>업무 추가</Button>}>
                    {d.data.children.length === 0 ? <p className="muted small">아직 이 목표에 연결된 업무가 없습니다.</p> : (
                        <ul className="stack tight" style={{listStyle: 'none', padding: 0, margin: 0}}>
                            {d.data.children.map((c) => (
                                <li key={c.id} className="row between nowrap">
                                    <button className="btn ghost small truncate"
                                            onClick={() => setDetail(<WorkPanel key={c.id} ctx={ctx} id={c.id} onChanged={onChanged}/>)}>{c.title}</button>
                                    <span className="row nowrap"><span className="muted small">{deptName(ctx, c.departmentId)}</span>
                                        <WorkStatusBadge status={c.status}/></span>
                                </li>
                            ))}
                        </ul>
                    )}
                    {d.data.children.some((c) => !isClosed(c.status)) && !isClosed(w.status) ? (
                        <p className="muted small">진행 중인 업무가 남아 있습니다. 목표를 완료하기 전에 결과를 확인하세요.</p>
                    ) : null}
                </Section>
            ) : null}

            <WorkEvents project={ctx.project} workItemId={w.id} departments={ctx.departments}/>

            <History ctx={ctx} itemId={w.id} notes={d.data.notes} disabled={archived} onAdded={d.reload}/>
        </div>
    );
}

// StatusChanger moves the item to another status. Waiting and blocked
// need a reason; closing and reopening keep the history.
function StatusChanger({ctx, item, disabled, onChanged}: { ctx: Context; item: WorkItem; disabled: boolean; onChanged: () => void }) {
    const [status, setStatus] = useState<WorkStatus>(item.status);
    const [reason, setReason] = useState('');
    const action = useAction();
    const toast = useToast();
    const closed = isClosed(item.status);
    const missing = needsReason(status) && !reason.trim();
    const change = (to: WorkStatus, why: string) => action.run(async () => {
        await api.setWorkStatus(ctx.project.id, item.id, to, why);
        toast(`‘${item.title}’을(를) ${t.workStatus[to]} 상태로 바꿨습니다`);
        setReason('');
        onChanged();
    });
    return (
        <form className="card stack" aria-label="상태 변경" onSubmit={(e) => { e.preventDefault(); if (!missing) change(status, reason); }}>
            <div className="grid-2">
                <Field label="상태">
                    <select value={status} disabled={disabled} onChange={(e) => setStatus(e.target.value as WorkStatus)}>
                        {workStatuses.map((s) => <option key={s} value={s}>{t.workStatus[s]}</option>)}
                    </select></Field>
                <Field label="이유" optional={!needsReason(status)} error={missing ? '대기나 막힘에는 이유가 필요합니다' : undefined}>
                    <input value={reason} disabled={disabled} placeholder={needsReason(status) ? '무엇을 기다리거나 왜 막혔는지' : '변경 이유 (기록에 남습니다)'}
                           onChange={(e) => setReason(e.target.value)}/></Field>
            </div>
            <ErrorBox error={action.error}/>
            <div className="form-actions">
                <Button type="submit" busy={action.busy} disabled={disabled || missing || (status === item.status && !reason.trim())}>상태 변경</Button>
                {!closed && item.status !== 'done' ? (
                    <Button variant="primary" busy={action.busy} disabled={disabled}
                            onClick={() => change('done', reason)}>완료 조건 확인 후 완료</Button>
                ) : null}
                {closed ? <Button busy={action.busy} disabled={disabled} onClick={() => change('in_progress', reason)}>다시 열기</Button> : null}
            </div>
        </form>
    );
}

const statusLine = (n: WorkNote) => n.fromStatus
    ? `${t.workStatus[n.fromStatus as WorkStatus] ?? n.fromStatus} → ${t.workStatus[n.toStatus as WorkStatus] ?? n.toStatus}`
    : `${t.workStatus[n.toStatus as WorkStatus] ?? n.toStatus} 상태로 등록`;

function History({ctx, itemId, notes, disabled, onAdded}: {
    ctx: Context; itemId: string; notes: WorkNote[]; disabled: boolean; onAdded: () => void;
}) {
    const [body, setBody] = useState('');
    const action = useAction();
    const add = () => action.run(async () => {
        await api.addWorkNote(ctx.project.id, itemId, body);
        setBody('');
        onAdded();
    });
    return (
        <Section title="기록" count={notes.length}>
            <ol className="timeline">
                {notes.map((n) => (
                    <li key={n.id} className="timeline-item">
                        <span className={`timeline-marker ${n.kind === 'status' ? 'info' : ''}`}>
                            <Icon icon={n.kind === 'status' ? Flag : MessageSquarePlus} size={14}/></span>
                        <div className="timeline-body">
                            <span className="timeline-title"><strong className="small">{n.kind === 'status' ? statusLine(n) : '메모'}</strong>
                                {n.author.startsWith('run:') ? <StatusBadge tone="info">실행에 따라</StatusBadge> : null}
                                <span className="muted xsmall">{shortTime(n.createdAt)}</span></span>
                            {n.body ? <div className="pre small">{n.body}</div> : null}
                        </div>
                    </li>
                ))}
            </ol>
            <form className="stack" aria-label="메모 남기기" onSubmit={(e) => { e.preventDefault(); add(); }}>
                <textarea value={body} disabled={disabled} rows={3} aria-label="메모" placeholder="진행 상황이나 결정을 남기세요"
                          onChange={(e) => setBody(e.target.value)}/>
                <ErrorBox error={action.error}/>
                <div><Button type="submit" size="sm" icon={MessageSquarePlus} busy={action.busy} disabled={disabled || !body.trim()}>메모 남기기</Button></div>
            </form>
        </Section>
    );
}

type WorkDraft = Pick<WorkItem, 'kind' | 'parentId' | 'departmentId' | 'assigneeAssignmentId' | 'title' | 'description'
    | 'doneCriteria' | 'priority' | 'categories' | 'dueAt'> & { id?: string };

const emptyWork: WorkDraft = {
    kind: 'work', parentId: '', departmentId: '', assigneeAssignmentId: '', title: '', description: '', doneCriteria: '',
    priority: 'normal', categories: [], dueAt: '',
};

// WorkForm creates or edits a work item or goal in the detail pane.
function WorkForm({ctx, initial, onSaved}: { ctx: Context; initial: WorkDraft; onSaved: (w: WorkItem) => void }) {
    const [form, setForm] = useState<WorkDraft>(initial);
    const action = useAction();
    const toast = useToast();
    const setDetail = useDetail();
    const goal = form.kind === 'goal';
    const editing = !!form.id;
    // Inactive departments keep their work but take no new work.
    const depts = ctx.departments.filter((d) => d.status === 'active' || d.id === initial.departmentId);
    const goals = ctx.goals.filter((g) => !isClosed(g.status) || g.id === initial.parentId);
    const set = (patch: Partial<WorkDraft>) => setForm({...form, ...patch});
    const save = async () => {
        const payload = {...form, projectId: ctx.project.id, parentId: goal ? '' : form.parentId};
        const saved = await action.run(() => (editing ? api.updateWorkItem(payload) : api.createWorkItem(payload)));
        if (saved) {
            toast(`‘${saved.title}’을(를) 저장했습니다`);
            onSaved(saved);
        }
    };
    const kindLabel = goal ? '공동 목표' : '업무';
    return (
        <form className="stack" aria-label={`${kindLabel} 편집`} onSubmit={(e) => { e.preventDefault(); save(); }}>
            <DetailHeader title={editing ? `${kindLabel} 편집` : `새 ${kindLabel}`} onClose={() => setDetail(null)}>
                {goal ? <p className="muted small">공동 목표는 여러 부서의 업무를 묶고, 결과를 책임질 부서 하나를 정합니다.</p> : null}
            </DetailHeader>
            <Field label="제목"><input value={form.title} autoFocus onChange={(e) => set({title: e.target.value})}/></Field>
            <div className="grid-2">
                <Field label={goal ? '책임 부서' : '담당 부서'} optional={!goal}>
                    <select value={form.departmentId} onChange={(e) => set({departmentId: e.target.value})}>
                        <option value="">{goal ? '부서 선택' : '미배정'}</option>
                        {depts.map((d) => <option key={d.id} value={d.id}>{d.name}{d.status === 'active' ? '' : ' (사용 안 함)'}</option>)}
                    </select></Field>
                <Field label="담당자" optional>
                    <select value={form.assigneeAssignmentId} onChange={(e) => set({assigneeAssignmentId: e.target.value})}>
                        <option value="">지정 안 함</option>
                        {ctx.assignments.map((a) => <option key={a.id} value={a.id}>{a.displayName}</option>)}
                    </select></Field>
            </div>
            {!goal ? (
                <Field label="공동 목표" optional>
                    <select value={form.parentId} onChange={(e) => set({parentId: e.target.value})}>
                        <option value="">없음</option>
                        {goals.map((g) => <option key={g.id} value={g.id}>{g.title}</option>)}
                    </select></Field>
            ) : null}
            <div className="grid-2">
                <Field label="우선순위">
                    <select value={form.priority} onChange={(e) => set({priority: e.target.value as WorkPriority})}>
                        {workPriorities.map((p) => <option key={p} value={p}>{t.workPriority[p]}</option>)}
                    </select></Field>
                <Field label="기한" optional>
                    <input type="date" value={form.dueAt} onChange={(e) => set({dueAt: e.target.value})}/></Field>
            </div>
            <fieldset className="stack tight">
                <legend className="field-label">분류 <span className="optional">(선택)</span></legend>
                <div className="row">
                    {ctx.categories.map((c) => (
                        <label key={c} className="check">
                            <input type="checkbox" checked={form.categories.includes(c)}
                                   onChange={(e) => set({categories: e.target.checked ? [...form.categories, c] : form.categories.filter((x) => x !== c)})}/>{c}
                        </label>
                    ))}
                    {ctx.categories.length === 0 ? <span className="muted small">서비스 설정에서 분류를 추가하세요.</span> : null}
                </div>
            </fieldset>
            <Field label="완료 조건" optional hint="사람이 업무를 완료할 때 확인하는 기준입니다.">
                <textarea value={form.doneCriteria} rows={3} onChange={(e) => set({doneCriteria: e.target.value})}/></Field>
            <Field label="설명" optional><textarea value={form.description} rows={4} onChange={(e) => set({description: e.target.value})}/></Field>
            <ErrorBox error={action.error}/>
            <div className="form-actions">
                <Button variant="primary" type="submit" busy={action.busy} disabled={!form.title.trim() || (goal && !form.departmentId)}>
                    {t.common.save}</Button>
                <Button onClick={() => setDetail(null)}>{t.common.cancel}</Button>
            </div>
        </form>
    );
}
