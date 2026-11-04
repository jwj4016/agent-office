import {Check, ExternalLink, FileText, MessageSquareWarning, Pencil, Play, Plus, RotateCcw, Save, Share2, Sparkles, Trash2} from 'lucide-react';
import {type ReactNode, useState} from 'react';
import {api} from '../api';
import {t} from '../i18n';
import {useLoad} from '../live';
import type {
    Assignment, Department, OutputType, PlanJobView, Project, RunDetail, StartApproval, WorkflowChoice, WorkflowOutput, WorkItem, WorkPlan, WorkPlanView, WorkStatus,
} from '../types';
import {Banner, Button, ErrorBox, Field, IssueList, Loading, RunBadge, StatusBadge, Tabs, useAction, useToast} from '../ui';
import {ArtifactViewer} from './ArtifactViewer';
import {AutoStartBanner} from './Automation';
import {ShareResult} from './Documents';

const outputTypes: OutputType[] = ['markdown', 'json', 'report', 'file', 'code_change', 'diagram'];
const activeRun = (r: RunDetail) => r.status === 'running' || r.status === 'waiting' || r.status === 'paused';

export type WorkTab = 'summary' | 'plan' | 'result' | 'record';

// NextKind is what a work item needs now, the first thing its detail says.
export type NextKind = 'running' | 'run_waiting' | 'result' | 'stopped' | 'preparing' | 'trouble' | 'ready' | 'no_plan' | 'manual' | 'closed';

// nextKind reads it from the work, its plan and its runs (newest first).
// A finished run never completes the work: the person checks the result.
export function nextKind(item: WorkItem, view: WorkPlanView, runs: RunDetail[]): NextKind {
    if (item.status === 'done' || item.status === 'cancelled') return 'closed';
    const active = runs.find(activeRun);
    if (active) return active.status === 'running' && !active.paused ? 'running' : 'run_waiting';
    const auto = view.autoStart?.status;
    if (auto === 'review') return 'stopped';
    if (auto === 'pending' || auto === 'planning' || auto === 'approved') return 'preparing';
    if (item.status === 'blocked') return 'trouble';
    const last = runs[0];
    if (last?.status === 'succeeded') return 'result';
    if (last) return 'trouble';
    if (item.status === 'waiting' || item.status === 'in_progress') return 'manual';
    return view.saved && !view.problem ? 'ready' : 'no_plan';
}

const nextTone: Record<NextKind, string> = {
    running: 'ai', preparing: 'ai', run_waiting: 'you', result: 'you', stopped: 'you', no_plan: 'you', ready: 'you', manual: 'you', trouble: 'bad', closed: 'ok',
};

// Expandable keeps long text to a few lines until it is asked for.
export function Expandable({text}: { text: string }) {
    const [open, setOpen] = useState(false);
    const long = text.length > 160;
    return (
        <div className="stack tight">
            <div className={`pre${long && !open ? ' clamp-4' : ''}`}>{text}</div>
            {long ? <div><button type="button" className="link-btn" aria-expanded={open} onClick={() => setOpen(!open)}>{open ? '접기' : '전체 보기'}</button></div> : null}
        </div>
    );
}

// useStart starts the saved plan, with a change request when given one.
function useStart(project: Project, itemId: string, onStarted: () => void) {
    const action = useAction();
    const toast = useToast();
    const [issues, setIssues] = useState<Parameters<typeof IssueList>[0]['issues']>([]);
    const start = (fb: string, done = fb ? '수정 요청을 담아 다시 실행했습니다' : '실행을 시작했습니다') => action.run(async () => {
        const res = await api.startWork(project.id, itemId, fb);
        if (res.issues.length && !res.runId) {
            setIssues(res.issues);
            return;
        }
        setIssues([]);
        toast(done);
        onStarted();
    });
    return {start, issues, action};
}

// NextStep says where the work stands and offers only what fits now.
function NextStep({project, item, view, runs, kind, onChanged, onOpenRun, onTab, onOpenRules}: {
    project: Project; item: WorkItem; view: WorkPlanView; runs: RunDetail[]; kind: NextKind;
    onChanged: () => void; onOpenRun: (runId: string) => void; onTab: (t: WorkTab) => void; onOpenRules?: () => void;
}) {
    const {start, issues, action} = useStart(project, item.id, onChanged);
    const toast = useToast();
    const archived = project.status === 'archived';
    const a = view.autoStart;
    const run = runs.find(activeRun) ?? runs[0];
    const seeRun = run ? <button type="button" className="link-btn" onClick={() => onOpenRun(run.id)}>실행 보기</button> : null;
    const setStatus = (to: WorkStatus, done: string) => action.run(async () => { await api.setWorkStatus(project.id, item.id, to, ''); toast(done); onChanged(); });
    const canRun = view.saved && !view.problem && !archived;
    const complete = <Button variant="primary" icon={Check} busy={action.busy} disabled={archived} onClick={() => setStatus('done', `‘${item.title}’을(를) 완료했습니다`)}>완료</Button>;
    const codeStop = kind === 'stopped' && a?.reason.includes('자동 작업 범위 밖') && view.plan.outputs.some((o) => o.type === 'code_change');
    const parts: Record<NextKind, { head: string; title: string; body?: ReactNode; actions?: ReactNode }> = {
        running: {head: '지금 상태', title: 'AI가 작업 중입니다', body: '사장님이 하실 일은 없습니다. 끝나면 내 할 일에 알려 드립니다.', actions: seeRun},
        run_waiting: {head: '사장님 확인 필요', title: '실행이 사장님을 기다립니다', body: item.statusReason || '실행 보기에서 승인이나 입력을 확인하세요.', actions: seeRun},
        result: {head: '사장님 확인 필요', title: '작업이 끝났습니다. 결과를 보고 완료해 주세요', actions: <>{complete}
            <Button icon={MessageSquareWarning} onClick={() => onTab('result')}>수정 요청</Button></>},
        stopped: {head: '사장님 확인 필요', title: codeStop ? '코드를 고치는 일이라 자동으로 시작하지 않았습니다' : '자동으로 시작하지 않았습니다',
            body: <>{a?.reason}. 이 업무는 아직 시작하지 않았습니다.</>,
            actions: <>
                {view.saved && !view.problem
                    ? <Button variant="primary" icon={Play} busy={action.busy} disabled={archived} onClick={() => start('', '이번 업무만 시작했습니다. 부서 규칙은 그대로입니다')}>이번만 시작</Button>
                    : <Button variant="primary" onClick={() => onTab('plan')}>계획 세우기</Button>}
                {onOpenRules ? <Button onClick={onOpenRules}>부서 자동화 규칙 열기</Button> : null}
                {a ? <Button variant="ghost" icon={RotateCcw} busy={action.busy} onClick={() => action.run(async () => { await api.retryAutoStart(project.id, a.id); toast('자동 시작 조건을 다시 확인합니다'); onChanged(); })}>다시 확인</Button> : null}
                {a ? <Button variant="ghost" busy={action.busy} onClick={() => action.run(async () => { await api.dismissAutoStart(project.id, a.id, '사람이 직접 진행'); onChanged(); })}>자동 시작 닫기</Button> : null}
            </>},
        preparing: {head: '지금 상태', title: a?.status === 'planning' ? 'AI가 계획을 쓰고 있습니다' : '자동 시작을 준비하고 있습니다', body: a?.note || '사장님이 하실 일은 없습니다.'},
        trouble: {head: '사장님 확인 필요', title: '일이 멈췄습니다', body: item.statusReason || view.problem || '실행이 끝나지 못했습니다.', actions: <>
            <Button variant="primary" icon={Play} busy={action.busy} disabled={!canRun} onClick={() => start('')}>다시 실행</Button>{seeRun}</>},
        ready: {head: '사장님 확인 필요', title: '시작 전입니다', body: '계획을 확인하고 시작하세요.', actions: <>
            <Button variant="primary" icon={Play} busy={action.busy} disabled={!canRun} onClick={() => start('')}>계획대로 실행</Button>
            <Button onClick={() => onTab('plan')}>계획 보기</Button></>},
        no_plan: {head: '사장님 확인 필요', title: '계획을 세우면 시작할 수 있습니다', body: view.saved ? view.problem : undefined, actions: <Button variant="primary" onClick={() => onTab('plan')}>계획 세우기</Button>},
        manual: {head: '지금 상태', title: item.status === 'waiting' ? '기다리는 중입니다' : '진행 중입니다', body: item.statusReason || undefined, actions: complete},
        closed: {head: '지금 상태', title: item.status === 'done' ? '완료된 업무입니다' : '취소된 업무입니다', actions:
            <Button busy={action.busy} disabled={archived} onClick={() => setStatus('in_progress', '다시 열었습니다')}>다시 열기</Button>},
    };
    const p = parts[kind];
    return (
        <div className={`next-step ${nextTone[kind]}`} role="status" aria-label="지금 할 일">
            <div className="next-step-head">{p.head}</div>
            <div className="next-step-title">{p.title}</div>
            {p.body ? <div className="next-step-body">{p.body}</div> : null}
            {p.actions ? <div className="next-step-actions">{p.actions}</div> : null}
            {issues.length ? <IssueList issues={issues}/> : null}
            <ErrorBox error={action.error}/>
        </div>
    );
}

// WorkTabs is a work item's detail: what it needs now, then its summary,
// plan, results and record.
export function WorkTabs({project, item, assignments, departments = [], categories = [], summary, documents, record, onChanged, onOpenRun, onOpenRules}: {
    project: Project; item: WorkItem; assignments: Assignment[]; departments?: Department[]; categories?: string[];
    summary?: ReactNode; documents?: ReactNode; record?: ReactNode;
    onChanged: () => void; onOpenRun: (runId: string) => void; onOpenRules?: () => void;
}) {
    const view = useLoad(() => api.workPlan(project.id, item.id), [project.id, item.id, item.updatedAt], project.id);
    const runs = useLoad(() => api.workRuns(project.id, item.id), [project.id, item.id], project.id);
    const [chosen, setChosen] = useState<WorkTab | null>(null);
    if (view.error || runs.error) return <ErrorBox error={view.error || runs.error}/>;
    if (!view.data || !runs.data) return <Loading/>;
    const reload = () => { view.reload(); runs.reload(); onChanged(); };
    const kind = nextKind(item, view.data, runs.data);
    const tab = chosen ?? (kind === 'result' ? 'result' : 'summary');
    const tabs: { id: WorkTab; label: string }[] = [{id: 'summary', label: '요약'}, {id: 'plan', label: '계획'}, {id: 'result', label: '결과'}, {id: 'record', label: '기록'}];
    return (
        <>
            <NextStep project={project} item={item} view={view.data} runs={runs.data} kind={kind} onChanged={reload} onOpenRun={onOpenRun} onTab={setChosen} onOpenRules={onOpenRules}/>
            <Tabs label="업무 상세 메뉴" items={tabs} value={tab} onChange={setChosen}/>
            <div role="tabpanel" aria-label={tabs.find((x) => x.id === tab)?.label} className="stack">
                {tab === 'summary' ? summary : null}
                {tab === 'plan' ? (
                    <WorkPlanTab project={project} item={item} view={view.data} busy={runs.data.some(activeRun)} assignments={assignments} onChanged={reload}>{documents}</WorkPlanTab>
                ) : null}
                {tab === 'result' ? (
                    <WorkResultTab project={project} item={item} view={view.data} runs={runs.data} departments={departments} categories={categories} onChanged={reload} onOpenRun={onOpenRun}/>
                ) : null}
                {tab === 'record' ? record : null}
            </div>
        </>
    );
}

// WorkPlanTab shows the plan as it is and opens the form only to change it.
function WorkPlanTab({project, item, view, busy, assignments, onChanged, children}: {
    project: Project; item: WorkItem; view: WorkPlanView; busy: boolean; assignments: Assignment[]; onChanged: () => void; children?: ReactNode;
}) {
    const choices = useLoad(() => api.workflowChoices(project.id), [project.id], project.id);
    const [editing, setEditing] = useState(false);
    const archived = project.status === 'archived';
    const closed = item.status === 'done' || item.status === 'cancelled';
    const plan = view.plan;
    const linked = !!plan.workflowVersionId;
    const who = assignments.find((a) => a.id === plan.assignmentId)?.displayName ?? '—';
    const flow = (choices.data ?? []).find((c) => c.versionId === plan.workflowVersionId);
    return (
        <>
            {view.autoStart && view.autoStart.status !== 'review' ? <AutoStartBanner a={view.autoStart} onDone={onChanged}/> : null}
            {editing || !view.saved ? (
                <>
                    {linked ? null : <PlanProposal project={project} itemId={item.id} assignments={assignments} disabled={archived || closed || busy} onApplied={onChanged}/>}
                    <PlanForm key={plan.updatedAt || 'new'} project={project} itemId={item.id} initial={plan}
                              assignments={assignments} choices={choices.data ?? []} disabled={archived || busy}
                              onSaved={() => { setEditing(false); onChanged(); }}/>
                    {editing ? <div><Button size="sm" variant="ghost" onClick={() => setEditing(false)}>{t.common.cancel}</Button></div> : null}
                </>
            ) : (
                <>
                    <dl className="kv" aria-label="저장된 계획">
                        {linked ? <><dt>실행 방식</dt><dd>업무 흐름: {flow ? `${flow.title} ${flow.number}판` : '확정한 버전'}</dd></> : <>
                            <dt>담당자</dt><dd>{who}</dd>
                            <dt>결과물</dt><dd>{plan.outputs.map((o) => t.outputType[o.type]).join(', ') || '—'}</dd>
                        </>}
                    </dl>
                    {!linked && plan.instructions ? <div className="stack tight"><h3 className="detail-label">작업 지침</h3><Expandable text={plan.instructions}/></div> : null}
                    {!linked && plan.materials ? <div className="stack tight"><h3 className="detail-label">참고 자료</h3><Expandable text={plan.materials}/></div> : null}
                    {view.problem ? <Banner tone="warn">{view.problem}</Banner> : null}
                    <div className="row">
                        <Button size="sm" icon={Pencil} disabled={archived || busy} onClick={() => setEditing(true)}>계획 편집</Button>
                        {busy ? <span className="muted small">실행 중에는 계획을 바꿀 수 없습니다.</span> : null}
                    </div>
                    {view.preview ? (
                        <details className="card">
                            <summary className="small">실행에 전달되는 내용 보기</summary>
                            <div className="pre small">{view.preview}</div>
                        </details>
                    ) : null}
                </>
            )}
            <StartGate project={project} item={item} approval={view.saved ? view.approval : undefined} onChanged={onChanged}/>
            {children}
        </>
    );
}

// WorkResultTab holds what came out, next to what the work had to reach.
function WorkResultTab({project, item, view, runs, departments, categories, onChanged, onOpenRun}: {
    project: Project; item: WorkItem; view: WorkPlanView; runs: RunDetail[]; departments: Department[]; categories: string[];
    onChanged: () => void; onOpenRun: (runId: string) => void;
}) {
    const closed = item.status === 'done' || item.status === 'cancelled';
    const busy = runs.some(activeRun);
    return (
        <>
            {item.doneCriteria ? <div className="stack tight"><h3 className="detail-label">끝났다고 볼 조건</h3><Expandable text={item.doneCriteria}/></div> : null}
            <RunList project={project} runs={runs} item={item} departments={departments} categories={categories} onOpenRun={onOpenRun}/>
            {runs.length ? <RunControls project={project} itemId={item.id} canRun={view.saved && !view.problem && !closed && project.status !== 'archived'}
                                        busy={busy} canRework={!view.plan.workflowVersionId} onStarted={onChanged}/> : null}
            {closed ? <p className="muted small">닫힌 업무입니다. 다시 열면 실행할 수 있습니다.</p> : null}
        </>
    );
}

// StartGate says under which approval the work last started and lets
// the person keep a review before every start, even in auto mode.
function StartGate({project, item, approval, onChanged}: {
    project: Project; item: WorkItem; approval?: StartApproval; onChanged: () => void;
}) {
    const action = useAction();
    const toggle = (on: boolean) => action.run(async () => { await api.updateWorkItem({...item, reviewBeforeStart: on}); onChanged(); });
    return (
        <div className="row small">
            <label className="check">
                <input type="checkbox" checked={item.reviewBeforeStart} disabled={project.status === 'archived' || action.busy}
                       onChange={(e) => toggle(e.target.checked)}/>자동 모드에서도 시작 전에 직접 검토
            </label>
            {approval?.by ? (
                <StatusBadge tone={approval.valid ? 'ok' : 'warn'}>
                    {approval.by === 'auto' ? '자동 시작 승인' : '사람이 시작'}{approval.valid ? '' : ' · 이후 계획·조건·자료·담당자가 바뀜'}
                </StatusBadge>
            ) : null}
            <ErrorBox error={action.error}/>
        </div>
    );
}

function PlanForm({project, itemId, initial, assignments, choices, disabled, onSaved}: {
    project: Project; itemId: string; initial: WorkPlan; assignments: Assignment[]; choices: WorkflowChoice[]; disabled: boolean; onSaved: () => void;
}) {
    // The form starts from the stored plan and is remounted (keyed by its
    // save time) when a saved plan changes; reloads of the same plan, which
    // any event of the service causes, keep what the person is typing.
    const [plan, setPlan] = useState<WorkPlan>(initial);
    const action = useAction();
    const toast = useToast();
    const set = (patch: Partial<WorkPlan>) => setPlan({...plan, ...patch});
    const setOutput = (i: number, patch: Partial<WorkflowOutput>) => set({outputs: plan.outputs.map((o, j) => (j === i ? {...o, ...patch} : o))});
    const save = () => action.run(async () => {
        await api.saveWorkPlan({...plan, projectId: project.id, workItemId: itemId});
        toast('작업 계획을 저장했습니다');
        onSaved();
    });
    const linked = !!plan.workflowVersionId;
    return (
        <form className="card stack" aria-label="작업 계획" onSubmit={(e) => { e.preventDefault(); save(); }}>
            <Field label="실행 방식" hint="반복하는 일은 확정한 업무 흐름 버전을 그대로 실행할 수 있습니다.">
                <select value={plan.workflowVersionId} disabled={disabled} onChange={(e) => set({workflowVersionId: e.target.value})}>
                    <option value="">이 업무의 작업 계획</option>
                    {choices.map((c) => <option key={c.versionId} value={c.versionId}>업무 흐름: {c.title} {c.number}판</option>)}
                </select></Field>
            {linked ? null : <>
            <Field label="담당자" hint="부서의 대표 담당자가 기본입니다. 사람을 고르면 내 할 일로 들어옵니다.">
                <select value={plan.assignmentId} disabled={disabled} onChange={(e) => set({assignmentId: e.target.value})}>
                    <option value="">담당자 선택</option>
                    {assignments.map((a) => <option key={a.id} value={a.id}>{a.displayName}{a.actorKind === 'human' ? ' (나)' : ''}</option>)}
                </select></Field>
            <Field label="작업 지침" optional hint="업무 설명과 완료 조건은 자동으로 함께 전달됩니다.">
                <textarea value={plan.instructions} rows={4} disabled={disabled} onChange={(e) => set({instructions: e.target.value})}/></Field>
            <Field label="참고 자료" optional hint="읽어 볼 문서나 경로, 링크를 적으세요.">
                <textarea value={plan.materials} rows={2} disabled={disabled} onChange={(e) => set({materials: e.target.value})}/></Field>
            <fieldset className="stack tight">
                <legend className="field-label">결과물</legend>
                {plan.outputs.map((o, i) => (
                    <div key={i} className="row nowrap">
                        <input value={o.key} disabled={disabled} aria-label={`결과물 ${i + 1} 이름`} placeholder="이름 (예: result)"
                               style={{flex: 1, minWidth: 0}} onChange={(e) => setOutput(i, {key: e.target.value})}/>
                        <select value={o.type} disabled={disabled} aria-label={`결과물 ${i + 1} 형식`} style={{flex: 1, minWidth: 0}}
                                onChange={(e) => setOutput(i, {type: e.target.value as OutputType})}>
                            {outputTypes.map((ty) => <option key={ty} value={ty}>{t.outputType[ty]}</option>)}
                        </select>
                        <Button size="sm" variant="ghost" iconOnly icon={Trash2} aria-label={`결과물 ${i + 1} 빼기`}
                                disabled={disabled || plan.outputs.length === 1} onClick={() => set({outputs: plan.outputs.filter((_, j) => j !== i)})}/>
                    </div>
                ))}
                <div><Button size="sm" variant="ghost" icon={Plus} disabled={disabled}
                             onClick={() => set({outputs: [...plan.outputs, {key: '', type: 'markdown'}]})}>결과물 추가</Button></div>
            </fieldset>
            </>}
            <ErrorBox error={action.error}/>
            <div className="form-actions">
                <Button type="submit" icon={Save} busy={action.busy} disabled={disabled || (!linked && !plan.assignmentId)}>계획 저장</Button>
                {disabled ? <span className="muted small">실행 중에는 계획을 바꿀 수 없습니다.</span> : null}
            </div>
        </form>
    );
}

function RunControls({project, itemId, canRun, busy, canRework = true, onStarted}: {
    project: Project; itemId: string; canRun: boolean; busy: boolean; canRework?: boolean; onStarted: () => void;
}) {
    const [feedback, setFeedback] = useState('');
    const {start, issues, action} = useStart(project, itemId, () => { setFeedback(''); onStarted(); });
    return (
        <div className="stack tight">
            {canRework ? (
                <form className="stack tight" aria-label="수정 요청" onSubmit={(e) => { e.preventDefault(); if (feedback.trim()) start(feedback); }}>
                    <textarea value={feedback} rows={2} aria-label="수정 요청 내용" placeholder="결과에서 고칠 점을 적으면 이전 결과와 함께 다시 실행합니다"
                              disabled={!canRun || busy} onChange={(e) => setFeedback(e.target.value)}/>
                    <div><Button type="submit" size="sm" icon={MessageSquareWarning} busy={action.busy}
                                 disabled={!canRun || busy || !feedback.trim()}>수정 요청하고 다시 실행</Button></div>
                </form>
            ) : null}
            <div className="row">
                <Button size="sm" variant="ghost" icon={Play} busy={action.busy} disabled={!canRun || busy} onClick={() => start('')}>그대로 다시 실행</Button>
                {busy ? <span className="muted small">실행 중입니다. 끝나거나 취소된 뒤에 다시 실행할 수 있습니다.</span> : null}
            </div>
            {issues.length ? <IssueList issues={issues}/> : null}
            <ErrorBox error={action.error}/>
        </div>
    );
}

function RunList({project, runs, item, departments, categories, onOpenRun}: {
    project: Project; runs: RunDetail[]; item: WorkItem; departments: Department[]; categories: string[]; onOpenRun: (runId: string) => void;
}) {
    const [open, setOpen] = useState<string | null>(null);
    const [sharing, setSharing] = useState<string | null>(null);
    if (runs.length === 0) return <p className="muted small">아직 실행하지 않았습니다.</p>;
    return (
        <ul className="stack tight" aria-label="이 업무의 실행" style={{listStyle: 'none', padding: 0, margin: 0}}>
            {runs.map((r) => {
                const arts = r.steps.flatMap((s) => s.artifacts.filter((a) => a.validity === 'valid'));
                return (
                    <li key={r.id} className="card stack tight">
                        <div className="row between nowrap">
                            <span className="row nowrap small"><strong>v{r.versionNumber}</strong><RunBadge status={r.status}/></span>
                            <Button size="sm" variant="ghost" icon={ExternalLink} aria-label={`v${r.versionNumber} 실행 보기`}
                                    onClick={() => onOpenRun(r.id)}>실행 보기</Button>
                        </div>
                        {arts.length ? (
                            <div className="row">
                                {arts.map((a) => (
                                    <span key={a.id} className="row nowrap">
                                        <Button size="sm" icon={FileText} aria-pressed={open === a.id}
                                                onClick={() => setOpen(open === a.id ? null : a.id)}>{a.outputKey} 결과</Button>
                                        <Button size="sm" variant="ghost" icon={Share2} aria-label={`${a.outputKey} 결과 공유`} disabled={project.status === 'archived'}
                                                onClick={() => setSharing(sharing === a.id ? null : a.id)}>공유</Button>
                                    </span>
                                ))}
                            </div>
                        ) : null}
                        {arts.some((a) => a.id === open) && open ? (
                            <ArtifactViewer projectId={project.id} artifactId={open} onClose={() => setOpen(null)}/>
                        ) : null}
                        {arts.some((a) => a.id === sharing) && sharing ? (
                            <ShareResult project={project} artifactId={sharing} item={item} departments={departments} categories={categories}
                                         onDone={() => setSharing(null)}/>
                        ) : null}
                    </li>
                );
            })}
        </ul>
    );
}

// newRequestId names one click, so the same click sent twice is one AI
// request while every new click asks again.
export function newRequestId() {
    return typeof crypto !== 'undefined' && 'randomUUID' in crypto ? crypto.randomUUID() : `${Date.now()}-${Math.random()}`;
}

// PlanProposal asks the work's AI for a plan and shows the latest
// proposal. Applying it fills the plan below; it never starts a run.
function PlanProposal({project, itemId, assignments, disabled, onApplied}: {
    project: Project; itemId: string; assignments: Assignment[]; disabled: boolean; onApplied: () => void;
}) {
    const jobs = useLoad(() => api.planJobs(project.id, itemId), [project.id, itemId], project.id);
    const action = useAction();
    const toast = useToast();
    const latest: PlanJobView | undefined = jobs.data?.[0];
    const working = latest && (latest.job.status === 'pending' || latest.job.status === 'working');
    const who = (id: string) => assignments.find((a) => a.id === id)?.displayName ?? '(알 수 없는 담당자)';
    const ask = () => action.run(async () => { await api.requestWorkPlan(project.id, itemId, newRequestId()); jobs.reload(); });
    return (
        <div className="stack tight">
            <div className="row">
                <Button icon={Sparkles} busy={action.busy || !!working} disabled={disabled || !!working} onClick={ask}>
                    {working ? 'AI가 계획을 쓰는 중…' : 'AI에게 계획 제안 받기'}</Button>
                <span className="muted xsmall">부서의 AI가 아래 계획을 대신 써 줍니다. 적용해도 바로 실행하지 않습니다.</span>
            </div>
            <ErrorBox error={jobs.error || action.error}/>
            {latest && !latest.job.appliedAt ? (() => {
                const j = latest.job;
                if (j.status === 'failed' || j.status === 'held') {
                    return (
                        <Banner tone={j.status === 'failed' ? 'bad' : 'warn'} title={j.status === 'failed' ? '계획 제안에 실패했습니다' : '계획 제안이 보류되었습니다'}
                                actions={<Button size="sm" icon={RotateCcw} disabled={disabled} busy={action.busy}
                                                 onClick={() => action.run(async () => { await api.retryAIJob(project.id, j.id); jobs.reload(); })}>다시 시도</Button>}>
                            {j.error || j.holdReason}{j.status === 'failed' ? ` (시도 ${j.attempts}회)` : ''}
                        </Banner>
                    );
                }
                if (j.status !== 'ready' || !latest.proposal) return null;
                const p = latest.proposal;
                return (
                    <div className="card stack tight" aria-label="AI 계획 제안">
                        <span className="row nowrap"><strong className="small">AI 계획 제안</strong>
                            {latest.stale ? <StatusBadge tone="warn">업무가 바뀜</StatusBadge> : null}</span>
                        <dl className="kv">
                            <dt>담당자</dt><dd>{who(p.assignmentId)}</dd>
                            <dt>작업 지침</dt><dd className="pre">{p.instructions}</dd>
                            {p.materials ? <><dt>참고 자료</dt><dd className="pre">{p.materials}</dd></> : null}
                            <dt>결과물</dt><dd>{p.outputs.map((o) => `${o.key} (${t.outputType[o.type] ?? o.type})`).join(', ')}</dd>
                            {p.notes ? <><dt>근거·가정</dt><dd className="pre">{p.notes}</dd></> : null}
                        </dl>
                        {latest.stale ? <p className="muted small">제안을 만든 뒤 업무 내용이 바뀌어 적용할 수 없습니다. 다시 요청하세요.</p> : null}
                        <div><Button size="sm" variant="primary" icon={Check} busy={action.busy} disabled={disabled || latest.stale}
                                     onClick={() => action.run(async () => {
                                         await api.applyPlanJob(project.id, j.id);
                                         toast('제안을 작업 계획에 적용했습니다. 확인한 뒤 실행하세요');
                                         jobs.reload();
                                         onApplied();
                                     })}>이 계획 적용</Button></div>
                    </div>
                );
            })() : null}
        </div>
    );
}
