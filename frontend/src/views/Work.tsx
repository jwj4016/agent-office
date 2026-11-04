import {ArrowRight, CalendarClock, Flag, GitBranch, ListTodo, MessageSquarePlus, Pencil, Plus, Target} from 'lucide-react';
import {type ReactNode, useEffect, useState} from 'react';
import {api} from '../api';
import {t} from '../i18n';
import {useLoad} from '../live';
import type {Assignment, AutoStart, Project, Department, WorkCause, WorkFilter, WorkItem, WorkKind, WorkNote, WorkPriority, WorkStatus} from '../types';
import {
    Banner, Button, DetailHeader, EmptyState, ErrorBox, Field, Icon, Loading, Section, StatusBadge, type Tone, shortTime, useAction, useDetail,
    useToast,
} from '../ui';
import {AutomationPanel} from './Automation';
import {WorkCollaboration} from './Collab';
import {DeliveryInbox, eventLabel, WorkEvents} from './Deliveries';
import {DepartmentDecisions} from './Judgments';
import {VersionBadge, WorkDocuments} from './Documents';
import {Expandable, WorkTabs} from './WorkRuns';

export const workStatuses: WorkStatus[] = ['received', 'ready', 'in_progress', 'waiting', 'blocked', 'done', 'cancelled'];
export const workPriorities: WorkPriority[] = ['urgent', 'high', 'normal', 'low'];

export const workTone: Record<WorkStatus, Tone> = {
    received: '', ready: 'info', in_progress: 'info', waiting: 'warn', blocked: 'bad', done: 'ok', cancelled: '',
};
const priorityTone: Record<WorkPriority, Tone> = {urgent: 'bad', high: 'warn', normal: '', low: ''};

export const isClosed = (s: WorkStatus) => s === 'done' || s === 'cancelled';
export const needsReason = (s: WorkStatus) => s === 'waiting' || s === 'blocked';

// WorkGroup is who the work waits for, which is how the list is grouped.
export type WorkGroup = 'you' | 'working' | 'after' | 'closed';

// workNext says who a work item waits for and what happens next. The run
// reasons are the ones the app writes when a run moves the work on.
export function workNext(w: WorkItem, auto: AutoStart | undefined, all: WorkItem[]): { group: WorkGroup; label: string } {
    if (isClosed(w.status)) return {group: 'closed', label: t.workStatus[w.status]};
    if (auto?.status === 'review') return {group: 'you', label: '시작할지 정하기'};
    if (w.status === 'blocked') return {group: 'you', label: '막힌 이유 확인'};
    if (w.status === 'waiting') {
        if (w.statusReason.startsWith('실행이 끝났습니다')) return {group: 'you', label: '결과 보고 완료하기'};
        if (w.statusReason.startsWith('실행이 사람의')) return {group: 'you', label: '승인·입력하기'};
        return {group: 'you', label: '내용 확인하기'};
    }
    if (w.status === 'in_progress') return {group: 'working', label: '작업 중'};
    const waiting = (w.afterIds ?? []).some((id) => { const x = all.find((o) => o.id === id); return !!x && !isClosed(x.status); });
    if (waiting) return {group: 'after', label: '앞 업무 대기'};
    if (auto && ['pending', 'planning', 'approved'].includes(auto.status)) return {group: 'working', label: auto.status === 'planning' ? 'AI가 계획 작성 중' : '자동 시작 준비 중'};
    return {group: 'you', label: '시작하기'};
}

const groupTitle: Record<WorkGroup, string> = {you: '사장님 확인 필요', working: '작업 중', after: '앞 업무 대기', closed: '닫힌 업무'};

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

// OpenOnce opens a detail panel when it first shows (keyed by what it opens).
function OpenOnce({open}: { open: () => void }) {
    useEffect(open, []);
    return null;
}

export function WorkStatusBadge({status, reason}: { status: WorkStatus; reason?: string }) {
    return <StatusBadge tone={workTone[status]}>{t.workStatus[status]}{reason ? ` · ${reason}` : ''}</StatusBadge>;
}

// GoalResults lists what the goal and its work shared: drafts while the
// departments work, final versions once confirmed.
function GoalResults({project, goal, items, onOpenWork}: {
    project: Project; goal: WorkItem; items: WorkItem[]; onOpenWork: (id: string) => void;
}) {
    const docs = useLoad(() => api.documents(project.id), [project.id], project.id);
    if (docs.error) return <ErrorBox error={docs.error}/>;
    if (!docs.data) return null;
    const works = new Map([goal, ...items].map((w) => [w.id, w]));
    const shared = docs.data.filter((doc) => works.has(doc.workItemId) && doc.status === 'active');
    return (
        <Section title="공유 자료와 결과" count={shared.length}>
            {shared.length === 0 ? <p className="muted small">이 목표의 업무가 공유한 자료가 아직 없습니다.</p> : (
                <ul className="stack tight" style={{listStyle: 'none', padding: 0, margin: 0}}>
                    {shared.map((doc) => {
                        const v = doc.versions.filter((x) => !x.withdrawnAt).at(-1);
                        const from = works.get(doc.workItemId)!;
                        return (
                            <li key={doc.id} className="row between nowrap">
                                <span className="truncate">{doc.title}</span>
                                <span className="row nowrap">
                                    {from.id !== goal.id ? <button className="btn ghost small" onClick={() => onOpenWork(from.id)}>{from.title}</button> : null}
                                    {v ? <VersionBadge v={v}/> : null}
                                </span>
                            </li>
                        );
                    })}
                </ul>
            )}
        </Section>
    );
}

// WorkLineageSection says why the work started (the event a department
// judged, or the request it took on) and which work came of it.
function WorkLineageSection({project, item, onOpenWork}: { project: Project; item: WorkItem; onOpenWork: (id: string) => void }) {
    const l = useLoad(() => api.workLineage(project.id, item.id), [project.id, item.id, item.updatedAt], project.id);
    if (l.error) return <ErrorBox error={l.error}/>;
    if (!l.data) return null;
    const {causes, next} = l.data;
    const link = (id: string | undefined, title: string | undefined) =>
        id ? <button className="btn ghost small wrap" onClick={() => onOpenWork(id)}>{title || '업무'}</button> : null;
    return (
        <Section title={<><Icon icon={GitBranch} size={18}/>시작한 이유와 이어진 업무</>} label="시작한 이유와 이어진 업무">
            {causes.length === 0 ? <p className="muted small">사람이 직접 만든 업무입니다.</p> : null}
            {causes.map((c, i) => <CauseCard key={i} c={c} link={link}/>)}
            {next.length ? (
                <div className="stack tight" role="group" aria-label="이어진 업무">
                    <span className="kind-head">이 업무에서 이어진 업무 <span className="muted">{next.length}</span></span>
                    {next.map((n) => (
                        <span key={`${n.via}-${n.id}`} className="row small">
                            <Icon icon={ArrowRight} size={14}/>{link(n.id, n.title)}
                            <span className="muted">{n.departmentName || '부서 없음'} · {n.via === 'collab' ? '협업 요청' : '부서 제안'}</span>
                            <WorkStatusBadge status={n.status}/>
                        </span>
                    ))}
                </div>
            ) : null}
        </Section>
    );
}

function CauseCard({c, link}: { c: WorkCause; link: (id: string | undefined, title: string | undefined) => ReactNode }) {
    if (c.via === 'request') {
        return (
            <div className="card subtle tight stack tight small">
                <strong>사람의 요청으로 등록한 업무</strong>
                <span className="pre">요청: {c.requestBody}</span>
                {c.reason ? <span className="pre">접수 담당의 판단: {c.reason}</span> : null}
                <span className="muted">등록: {c.decidedBy === 'ai' ? '접수 담당(자동 모드)' : '사람'}</span>
            </div>
        );
    }
    if (c.via === 'collab') {
        return (
            <div className="card subtle tight stack tight small">
                <strong>{c.fromDepartmentName} → {c.departmentName} 협업 요청으로 맡은 업무</strong>
                <span>요청한 업무: {link(c.fromWorkItemId, c.fromWorkTitle)}</span>
                <span className="pre">요청: {c.reason}</span>
                <span className="muted">결정: {c.decidedBy === 'ai' ? `${c.departmentName} 대표 AI` : '사람'}</span>
            </div>
        );
    }
    return (
        <div className="card subtle tight stack tight small">
            <strong>{c.departmentName} 제안으로 등록한 업무</strong>
            <span>사건: {eventLabel(c.eventKind)}{c.eventTitle ? ` · ‘${c.eventTitle}’` : ''} <span className="muted">#{c.eventSequence}</span>
                {c.scheduleSlot ? <StatusBadge tone="info" icon={CalendarClock}>정기 점검</StatusBadge> : null}</span>
            {c.eventNote ? <span className="pre">요청 내용: {c.eventNote}</span> : null}
            {c.documentTitle ? <span>자료: {c.documentTitle}</span> : null}
            {c.fromWorkItemId ? <span>관련 업무: {link(c.fromWorkItemId, c.fromWorkTitle)}</span> : null}
            <span className="pre">판단 이유: {c.reason}</span>
        </div>
    );
}

type Context = {
    project: Project; departments: Department[]; categories: string[]; assignments: Assignment[]; goals: WorkItem[];
    onOpenRun: (runId: string) => void; onSelect: (id: string) => void;
};

const deptName = (ctx: Context, id: string) => ctx.departments.find((d) => d.id === id)?.name ?? '';
const personName = (ctx: Context, id: string) => ctx.assignments.find((a) => a.id === id)?.displayName ?? '';
const goalTitle = (ctx: Context, id: string) => ctx.goals.find((g) => g.id === id)?.title ?? '';

const defaultFilter: WorkFilter = {status: 'open'};

// Work is the service's work list: shared goals and each department's
// work, filtered by status, department, priority, category and due date.
// workItemId opens that item's panel, e.g. when coming from the inbox.
export function Work({project, workItemId, onTeam, onRequest, onOpenRun}: {
    project: Project; workItemId?: string; onTeam: () => void; onRequest: () => void; onOpenRun: (runId: string) => void;
}) {
    const [filter, setFilter] = useState<WorkFilter>(defaultFilter);
    const [showFilter, setShowFilter] = useState(false);
    const [showDept, setShowDept] = useState<boolean | null>(null);
    const [selected, setSelected] = useState('');
    const departments = useLoad(() => api.departments(project.id), [project.id], project.id);
    const categories = useLoad(() => api.categories(project.id), [project.id], project.id);
    const assignments = useLoad(() => api.assignments(project.id), [project.id], project.id);
    const all = useLoad(() => api.workItems(project.id), [project.id], project.id);
    const list = useLoad(() => api.workItems(project.id, {...filter, kind: 'work'}), [project.id, JSON.stringify(filter)], project.id);
    const autos = useLoad(() => api.autoStarts(project.id), [project.id], project.id);
    const questions = useLoad(() => api.judgments(project.id, 'open,manual'), [project.id], project.id);
    const proposals = useLoad(() => api.proposals(project.id, 'proposed'), [project.id], project.id);
    const deliveries = useLoad(() => api.deliveries(project.id, {status: 'open'}), [project.id], project.id);
    const setDetail = useDetail();
    const archived = project.status === 'archived';

    const error = departments.error || categories.error || assignments.error || all.error || list.error;
    if (error) return <ErrorBox error={error}/>;
    if (!departments.data || !categories.data || !assignments.data || !all.data) return <Loading/>;

    const goals = all.data.filter((w) => w.kind === 'goal');
    const ctx: Context = {project, departments: departments.data, categories: categories.data, assignments: assignments.data, goals, onOpenRun, onSelect: setSelected};
    const refresh = () => { all.reload(); list.reload(); autos.reload(); };
    const open = (id: string) => setDetail(<WorkPanel key={id} ctx={ctx} id={id} onChanged={refresh}/>);
    const create = (kind: WorkKind, parentId = '') => setDetail(
        <WorkForm key={`new-${kind}-${parentId}`} ctx={ctx} initial={{...emptyWork, kind, parentId}}
                  onSaved={(w) => { refresh(); open(w.id); }}/>);
    const activeDepts = departments.data.filter((d) => d.status === 'active');
    const openGoals = goals.filter((g) => !isClosed(g.status));
    const set = (patch: Partial<WorkFilter>) => setFilter({...filter, ...patch});
    const filtered = JSON.stringify(filter) !== JSON.stringify(defaultFilter);
    const closedView = filter.status === 'closed';
    const everything = all.data;
    const autoOf = (id: string) => (autos.data ?? []).find((a) => a.workItemId === id);
    const rows = (list.data ?? []).map((w) => ({w, next: workNext(w, autoOf(w.id), everything)}));
    const deptCount = (questions.data?.length ?? 0) + (proposals.data?.length ?? 0) + (deliveries.data?.length ?? 0);
    const deptOpen = showDept ?? deptCount > 0;

    return (
        <div className="stack loose">
            {workItemId ? <OpenOnce key={workItemId} open={() => open(workItemId)}/> : null}
            <div className="row between">
                <p className="muted" style={{flex: '1 1 280px'}}>요청에서 생긴 일이 어디까지 왔는지 보는 곳입니다. 사장님이 볼 일이 맨 위에 옵니다.</p>
                <span className="row nowrap">
                    <Button icon={Plus} disabled={archived} onClick={() => create('work')}>직접 추가</Button>
                    <Button variant="primary" icon={MessageSquarePlus} disabled={archived} onClick={onRequest}>요청하기</Button>
                </span>
            </div>
            {activeDepts.length === 0 ? (
                <Banner tone="info" title="아직 이 서비스에 부서가 없습니다"
                        actions={<Button size="sm" onClick={onTeam}>부서 만들기</Button>}>
                    부서를 만들거나 회사 부서 템플릿에서 가져오면 업무를 부서에 맡기고 공동 목표의 책임 부서를 정할 수 있습니다.
                </Banner>
            ) : null}

            <ErrorBox error={list.error}/>
            {!list.data ? <Loading/> : rows.length === 0 ? (
                <EmptyState compact icon={ListTodo} title={filtered ? '조건에 맞는 업무가 없습니다' : '진행 중인 업무가 없습니다'}
                            actions={!filtered && !archived ? <Button variant="primary" icon={MessageSquarePlus} onClick={onRequest}>요청하기</Button> : null}>
                    {filtered ? '조회 조건을 바꿔 보세요.' : '회사에 요청하면 담당 부서가 일을 맡습니다.'}
                </EmptyState>
            ) : (['you', 'working', 'after', 'closed'] as WorkGroup[]).map((g) => {
                const inGroup = rows.filter((r) => r.next.group === g);
                if (inGroup.length === 0) return null;
                return (
                    <section key={g} className={`work-group ${g}`} aria-label={groupTitle[g]}>
                        <h2 className="work-group-title">{groupTitle[g]} <span className="work-group-count">{inGroup.length}</span></h2>
                        <ul className="work-rows">
                            {inGroup.map(({w, next}) => (
                                <li key={w.id}>
                                    <button type="button" className={`work-row${selected === w.id ? ' selected' : ''}`} onClick={() => open(w.id)}
                                            aria-label={`${w.title} 업무 보기`} title={w.title} aria-current={selected === w.id ? 'true' : undefined}>
                                        <span className="work-row-main">
                                            <span className="work-row-title">{w.title}</span>
                                            {g === 'you' && w.statusReason && !w.statusReason.startsWith('실행이 끝났습니다') ? <span className="muted xsmall">{w.statusReason}</span> : null}
                                            {w.parentId ? <span className="muted xsmall">목표: {goalTitle(ctx, w.parentId)}</span> : null}
                                            {w.priority === 'urgent' || overdue(w) ? (
                                                <span className="row">
                                                    {w.priority === 'urgent' ? <StatusBadge tone="bad">{t.workPriority.urgent}</StatusBadge> : null}
                                                    {overdue(w) ? <StatusBadge tone="bad" icon={CalendarClock}>{w.dueAt} 지남</StatusBadge> : null}
                                                </span>
                                            ) : null}
                                        </span>
                                        <span className="work-row-dept">{deptName(ctx, w.departmentId) || '미배정'}</span>
                                        <span className={`work-row-next ${g}`}>{next.label}</span>
                                    </button>
                                </li>
                            ))}
                        </ul>
                    </section>
                );
            })}

            {openGoals.length ? (
                <Section title={<><Icon icon={Target} size={18}/>공동 목표</>} label="공동 목표" count={openGoals.length}>
                    <div className="grid-cards">
                        {openGoals.map((g) => {
                            const p = goalProgress(g.id, everything);
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
                </Section>
            ) : null}

            <div className="row" role="group" aria-label="더 보기">
                <Button size="sm" onClick={() => set({status: closedView ? 'open' : 'closed'})}>{closedView ? '진행 중인 업무 보기' : '끝난 업무 보기'}</Button>
                <Button size="sm" aria-expanded={deptOpen} onClick={() => setShowDept(!deptOpen)}>부서 판단·전달 {deptCount}</Button>
                <Button size="sm" aria-expanded={showFilter || filtered} onClick={() => setShowFilter(!showFilter)}>찾기·거르기</Button>
                <Button size="sm" variant="ghost" icon={Target} disabled={archived || activeDepts.length === 0} onClick={() => create('goal')}>공동 목표 추가</Button>
            </div>

            {showFilter || filtered ? (
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
            ) : null}

            {deptOpen ? <>
                <DepartmentDecisions project={project}/>
                <DeliveryInbox project={project} departments={departments.data}/>
            </> : null}
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
    if (w.kind === 'work') {
        const dept = ctx.departments.find((x) => x.id === w.departmentId);
        const openWork = (other: string) => setDetail(<WorkPanel key={other} ctx={ctx} id={other} onChanged={onChanged}/>);
        return (
            <div className="stack">
                <SelectRow ctx={ctx} id={id}/>
                <DetailHeader title={w.title} onClose={() => setDetail(null)}>
                    <div className="small muted">{deptName(ctx, w.departmentId) || '담당 부서 없음'} · <WorkAssignee ctx={ctx} item={w}/></div>
                </DetailHeader>
                <WorkTabs project={ctx.project} item={w} assignments={ctx.assignments} departments={ctx.departments} categories={ctx.categories}
                          onChanged={changed} onOpenRun={ctx.onOpenRun}
                          onOpenRules={dept ? () => setDetail(<AutomationPanel key={`auto-${dept.id}`} project={ctx.project} department={dept}/>) : undefined}
                          summary={<WorkSummary ctx={ctx} item={w} onEdit={edit} onChanged={changed} onOpenWork={openWork}/>}
                          documents={<WorkDocuments project={ctx.project} item={w}/>}
                          record={<>
                              <WorkLineageSection project={ctx.project} item={w} onOpenWork={openWork}/>
                              <History ctx={ctx} itemId={w.id} notes={d.data.notes} disabled={archived} onAdded={d.reload}/>
                              <WorkEvents project={ctx.project} workItemId={w.id} departments={ctx.departments}/>
                          </>}/>
            </div>
        );
    }
    return (
        <div className="stack">
            <DetailHeader title={<><Icon icon={Target} size={18}/>{w.title}</>} onClose={() => setDetail(null)}>
                <div className="row">
                    <StatusBadge tone="info">공동 목표</StatusBadge>
                    <WorkStatusBadge status={w.status} reason={w.statusReason}/>
                    <StatusBadge tone={priorityTone[w.priority]} icon={Flag}>{t.workPriority[w.priority]}</StatusBadge>
                </div>
            </DetailHeader>
            <dl className="kv">
                <dt>책임 부서</dt><dd>{deptName(ctx, w.departmentId) || '미배정'}</dd>
                <dt>담당자</dt><dd>{personName(ctx, w.assigneeAssignmentId) || '—'}</dd>
                {w.afterIds?.length ? <><dt>앞 업무</dt><dd><AfterWork projectId={ctx.project.id} ids={w.afterIds}
                    open={(other) => setDetail(<WorkPanel key={other} ctx={ctx} id={other} onChanged={onChanged}/>)}/></dd></> : null}
                <dt>기한</dt><dd>{w.dueAt ? <>{w.dueAt}{overdue(w) ? <> <StatusBadge tone="bad">지남</StatusBadge></> : null}</> : '—'}</dd>
                <dt>분류</dt><dd>{w.categories.join(', ') || '—'}</dd>
                <dt>완료 조건</dt><dd className="pre">{w.doneCriteria || <span className="muted">적지 않음</span>}</dd>
                <dt>설명</dt><dd className="pre">{w.description || <span className="muted">적지 않음</span>}</dd>
            </dl>
            <div><Button size="sm" icon={Pencil} disabled={archived} onClick={edit}>{t.common.edit}</Button></div>
            <StatusChanger key={`${w.status}-${w.updatedAt}`} ctx={ctx} item={w} disabled={archived} onChanged={changed}/>

            <WorkCollaboration project={ctx.project} item={w} departments={ctx.departments} onChanged={changed}
                               onOpenWork={(other) => setDetail(<WorkPanel key={other} ctx={ctx} id={other} onChanged={onChanged}/>)}/>

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
            {w.kind === 'goal' ? (
                <GoalResults project={ctx.project} goal={w} items={d.data.children}
                             onOpenWork={(other) => setDetail(<WorkPanel key={other} ctx={ctx} id={other} onChanged={onChanged}/>)}/>
            ) : null}

            <WorkEvents project={ctx.project} workItemId={w.id} departments={ctx.departments}/>

            <History ctx={ctx} itemId={w.id} notes={d.data.notes} disabled={archived} onAdded={d.reload}/>
        </div>
    );
}

// SelectRow marks the open work in the list while its detail is shown.
function SelectRow({ctx, id}: { ctx: Context; id: string }) {
    useEffect(() => {
        ctx.onSelect(id);
        return () => ctx.onSelect('');
    }, [id]);
    return null;
}

// causeLine says in one line why the work started.
function causeLine(c: WorkCause) {
    if (c.via === 'request') return '사람의 요청으로 등록했습니다.';
    if (c.via === 'collab') return `${c.fromDepartmentName}의 협업 요청으로 맡았습니다.`;
    return `${c.departmentName}에서 제안해 등록했습니다.`;
}

// WorkSummary is the first thing to read about a work item: what it is,
// why it started and when it counts as done. Changing it is under ‘더보기’.
function WorkSummary({ctx, item, onEdit, onChanged, onOpenWork}: {
    ctx: Context; item: WorkItem; onEdit: () => void; onChanged: () => void; onOpenWork: (id: string) => void;
}) {
    const lineage = useLoad(() => api.workLineage(ctx.project.id, item.id), [ctx.project.id, item.id, item.updatedAt], ctx.project.id);
    const collab = useLoad(() => api.workCollab(ctx.project.id, item.id), [ctx.project.id, item.id, item.updatedAt], ctx.project.id);
    const [more, setMore] = useState<boolean | null>(null);
    const archived = ctx.project.status === 'archived';
    const setDetail = useDetail();
    const c = collab.data;
    const moreOpen = more ?? (!!c && c.requests.length + c.incoming.length + c.checks.length > 0);
    const cause = lineage.data?.causes[0];
    const hasFacts = item.parentId || item.afterIds?.length || item.dueAt || item.categories.length || item.priority === 'urgent';
    return (
        <>
            <div className="stack tight">
                <h3 className="detail-label">무슨 일인가요</h3>
                {item.description ? <Expandable text={item.description}/> : <span className="muted">적지 않음</span>}
            </div>
            {cause ? (
                <div className="stack tight">
                    <h3 className="detail-label">왜 시작했나요</h3>
                    <span>{causeLine(cause)}</span>
                </div>
            ) : null}
            <div className="stack tight">
                <h3 className="detail-label">끝났다고 볼 조건</h3>
                {item.doneCriteria ? <Expandable text={item.doneCriteria}/> : <span className="muted">적지 않음</span>}
            </div>
            {hasFacts ? (
                <dl className="kv">
                    {item.priority === 'urgent' ? <><dt>우선순위</dt><dd><StatusBadge tone="bad" icon={Flag}>{t.workPriority.urgent}</StatusBadge></dd></> : null}
                    {item.parentId ? <><dt>공동 목표</dt><dd>
                        <button className="btn ghost small wrap" onClick={() => setDetail(<WorkPanel key={item.parentId} ctx={ctx} id={item.parentId} onChanged={onChanged}/>)}>
                            {goalTitle(ctx, item.parentId) || '목표 보기'}</button></dd></> : null}
                    {item.afterIds?.length ? <><dt>앞 업무</dt><dd><AfterWork projectId={ctx.project.id} ids={item.afterIds} open={onOpenWork}/></dd></> : null}
                    {item.dueAt ? <><dt>기한</dt><dd>{item.dueAt}{overdue(item) ? <> <StatusBadge tone="bad">지남</StatusBadge></> : null}</dd></> : null}
                    {item.categories.length ? <><dt>분류</dt><dd>{item.categories.join(', ')}</dd></> : null}
                </dl>
            ) : null}
            <div><Button size="sm" variant="ghost" aria-expanded={moreOpen} onClick={() => setMore(!moreOpen)}>더보기: 편집·상태 변경·다른 부서에 요청</Button></div>
            {moreOpen ? <>
                <div><Button size="sm" icon={Pencil} disabled={archived} onClick={onEdit}>{t.common.edit}</Button></div>
                <StatusChanger key={`${item.status}-${item.updatedAt}`} ctx={ctx} item={item} disabled={archived} onChanged={onChanged}/>
                <WorkCollaboration project={ctx.project} item={item} departments={ctx.departments} onChanged={onChanged} onOpenWork={onOpenWork}/>
            </> : null}
        </>
    );
}

// WorkAssignee names who does the work: the person set on it, or else
// the one its saved plan gives it to.
function WorkAssignee({ctx, item}: { ctx: Context; item: WorkItem }) {
    const plan = useLoad(() => api.workPlan(ctx.project.id, item.id), [ctx.project.id, item.id, item.updatedAt], ctx.project.id);
    const set = personName(ctx, item.assigneeAssignmentId);
    if (set) return <>{set}</>;
    if (plan.data?.plan.workflowVersionId) return <>업무 흐름대로 실행</>;
    return <>{personName(ctx, plan.data?.plan.assignmentId ?? '') || '—'}</>;
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
    | 'doneCriteria' | 'priority' | 'categories' | 'dueAt' | 'reviewBeforeStart'> & { id?: string };

const emptyWork: WorkDraft = {
    kind: 'work', parentId: '', departmentId: '', assigneeAssignmentId: '', title: '', description: '', doneCriteria: '',
    priority: 'normal', categories: [], dueAt: '', reviewBeforeStart: false,
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

// AfterWork lists the work an item waits for; automatic start holds it
// until they are all closed.
function AfterWork({projectId, ids, open}: { projectId: string; ids: string[]; open: (id: string) => void }) {
    const list = useLoad(() => Promise.all(ids.map((id) => api.workItem(projectId, id).then((d) => d.item))), [projectId, ids.join(',')], projectId);
    if (!list.data) return <span className="muted small">불러오는 중…</span>;
    return (
        <span className="stack tight">
            {list.data.map((x) => (
                <span key={x.id} className="row small">
                    <button className="btn ghost small wrap" onClick={() => open(x.id)}>{x.title}</button><WorkStatusBadge status={x.status}/>
                </span>
            ))}
            <span className="muted xsmall">앞 업무가 모두 끝나야 자동으로 시작합니다.</span>
        </span>
    );
}
