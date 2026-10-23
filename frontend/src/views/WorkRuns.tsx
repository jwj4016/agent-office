import {Check, ExternalLink, FileText, MessageSquareWarning, Play, Plus, RotateCcw, Save, Sparkles, Trash2} from 'lucide-react';
import {useEffect, useState} from 'react';
import {api} from '../api';
import {t} from '../i18n';
import {useLoad} from '../live';
import type {Assignment, OutputType, PlanJobView, Project, RunDetail, WorkflowOutput, WorkItem, WorkPlan} from '../types';
import {Banner, Button, ErrorBox, Field, IssueList, Loading, RunBadge, Section, StatusBadge, useAction, useToast} from '../ui';
import {ArtifactViewer} from './ArtifactViewer';

const outputTypes: OutputType[] = ['markdown', 'json', 'report', 'file', 'code_change', 'diagram'];
const activeRun = (r: RunDetail) => r.status === 'running' || r.status === 'waiting' || r.status === 'paused';

// WorkRun is how a work item gets done: a small plan the person writes,
// runs of that plan, their results, and change requests. A finished run
// never completes the work; the person checks the result and completes it.
export function WorkRun({project, item, assignments, onChanged, onOpenRun}: {
    project: Project; item: WorkItem; assignments: Assignment[]; onChanged: () => void; onOpenRun: (runId: string) => void;
}) {
    const view = useLoad(() => api.workPlan(project.id, item.id), [project.id, item.id, item.updatedAt], project.id);
    const runs = useLoad(() => api.workRuns(project.id, item.id), [project.id, item.id], project.id);
    const archived = project.status === 'archived';
    const closed = item.status === 'done' || item.status === 'cancelled';
    if (view.error || runs.error) return <ErrorBox error={view.error || runs.error}/>;
    if (!view.data || !runs.data) return <Loading/>;
    const busy = runs.data.some(activeRun);
    const reload = () => { view.reload(); runs.reload(); onChanged(); };
    return (
        <Section title="작업 계획과 실행">
            <PlanProposal project={project} itemId={item.id} assignments={assignments} disabled={archived || closed || busy} onApplied={reload}/>
            <PlanForm key={view.data.plan.updatedAt || 'new'} project={project} itemId={item.id} initial={view.data.plan}
                      assignments={assignments} disabled={archived || busy} onSaved={reload}/>
            {view.data.saved && view.data.preview ? (
                <details className="card">
                    <summary className="small">실행에 전달되는 내용 보기</summary>
                    <div className="pre small">{view.data.preview}</div>
                </details>
            ) : null}
            {view.data.saved && view.data.problem ? <Banner tone="warn">{view.data.problem}</Banner> : null}
            {closed ? <p className="muted small">닫힌 업무입니다. 다시 열면 실행할 수 있습니다.</p> : null}
            <RunControls project={project} itemId={item.id} canRun={view.data.saved && !view.data.problem && !closed && !archived}
                         busy={busy} hasResult={runs.data.some((r) => r.status === 'succeeded')} onStarted={reload}/>
            <RunList project={project} runs={runs.data} onOpenRun={onOpenRun}/>
        </Section>
    );
}

function PlanForm({project, itemId, initial, assignments, disabled, onSaved}: {
    project: Project; itemId: string; initial: WorkPlan; assignments: Assignment[]; disabled: boolean; onSaved: () => void;
}) {
    const [plan, setPlan] = useState<WorkPlan>(initial);
    useEffect(() => setPlan(initial), [initial]);
    const action = useAction();
    const toast = useToast();
    const set = (patch: Partial<WorkPlan>) => setPlan({...plan, ...patch});
    const setOutput = (i: number, patch: Partial<WorkflowOutput>) => set({outputs: plan.outputs.map((o, j) => (j === i ? {...o, ...patch} : o))});
    const save = () => action.run(async () => {
        await api.saveWorkPlan({...plan, projectId: project.id, workItemId: itemId});
        toast('작업 계획을 저장했습니다');
        onSaved();
    });
    return (
        <form className="card stack" aria-label="작업 계획" onSubmit={(e) => { e.preventDefault(); save(); }}>
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
                <legend className="field-label">산출물</legend>
                {plan.outputs.map((o, i) => (
                    <div key={i} className="row nowrap">
                        <input value={o.key} disabled={disabled} aria-label={`산출물 ${i + 1} 이름`} placeholder="이름 (예: result)"
                               onChange={(e) => setOutput(i, {key: e.target.value})}/>
                        <select value={o.type} disabled={disabled} aria-label={`산출물 ${i + 1} 형식`}
                                onChange={(e) => setOutput(i, {type: e.target.value as OutputType})}>
                            {outputTypes.map((ty) => <option key={ty} value={ty}>{t.outputType[ty]}</option>)}
                        </select>
                        <Button size="sm" variant="ghost" iconOnly icon={Trash2} aria-label={`산출물 ${i + 1} 빼기`}
                                disabled={disabled || plan.outputs.length === 1} onClick={() => set({outputs: plan.outputs.filter((_, j) => j !== i)})}/>
                    </div>
                ))}
                <div><Button size="sm" variant="ghost" icon={Plus} disabled={disabled}
                             onClick={() => set({outputs: [...plan.outputs, {key: '', type: 'markdown'}]})}>산출물 추가</Button></div>
            </fieldset>
            <ErrorBox error={action.error}/>
            <div className="form-actions">
                <Button type="submit" icon={Save} busy={action.busy} disabled={disabled || !plan.assignmentId}>계획 저장</Button>
                {disabled ? <span className="muted small">실행 중에는 계획을 바꿀 수 없습니다.</span> : null}
            </div>
        </form>
    );
}

function RunControls({project, itemId, canRun, busy, hasResult, onStarted}: {
    project: Project; itemId: string; canRun: boolean; busy: boolean; hasResult: boolean; onStarted: () => void;
}) {
    const [feedback, setFeedback] = useState('');
    const action = useAction();
    const toast = useToast();
    const [issues, setIssues] = useState<Parameters<typeof IssueList>[0]['issues']>([]);
    const start = (fb: string) => action.run(async () => {
        const res = await api.startWork(project.id, itemId, fb);
        if (res.issues.length && !res.runId) {
            setIssues(res.issues);
            return;
        }
        setIssues([]);
        setFeedback('');
        toast(fb ? '수정 요청을 담아 다시 실행했습니다' : '실행을 시작했습니다');
        onStarted();
    });
    return (
        <div className="stack tight">
            <div className="row">
                <Button variant="primary" icon={Play} busy={action.busy} disabled={!canRun || busy} onClick={() => start('')}>
                    {hasResult ? '다시 실행' : '계획대로 실행'}</Button>
                {busy ? <span className="muted small">실행 중입니다. 끝나거나 취소된 뒤에 다시 실행할 수 있습니다.</span> : null}
            </div>
            {hasResult ? (
                <form className="stack tight" aria-label="수정 요청" onSubmit={(e) => { e.preventDefault(); if (feedback.trim()) start(feedback); }}>
                    <textarea value={feedback} rows={2} aria-label="수정 요청 내용" placeholder="결과에서 고칠 점을 적으면 이전 결과와 함께 다시 실행합니다"
                              disabled={!canRun || busy} onChange={(e) => setFeedback(e.target.value)}/>
                    <div><Button type="submit" size="sm" icon={MessageSquareWarning} busy={action.busy}
                                 disabled={!canRun || busy || !feedback.trim()}>수정 요청하고 다시 실행</Button></div>
                </form>
            ) : null}
            {issues.length ? <IssueList issues={issues}/> : null}
            <ErrorBox error={action.error}/>
        </div>
    );
}

function RunList({project, runs, onOpenRun}: { project: Project; runs: RunDetail[]; onOpenRun: (runId: string) => void }) {
    const [open, setOpen] = useState<string | null>(null);
    if (runs.length === 0) return <p className="muted small">아직 실행하지 않았습니다.</p>;
    return (
        <ul className="stack tight" aria-label="이 업무의 실행" style={{listStyle: 'none', padding: 0, margin: 0}}>
            {runs.map((r) => {
                const arts = r.steps.flatMap((s) => s.artifacts.filter((a) => a.validity === 'valid'));
                return (
                    <li key={r.id} className="card stack tight">
                        <div className="row between nowrap">
                            <span className="row nowrap small"><strong>v{r.versionNumber}</strong><RunBadge status={r.status}/></span>
                            <Button size="sm" variant="ghost" icon={ExternalLink} aria-label={`v${r.versionNumber} 실행 화면에서 보기`}
                                    onClick={() => onOpenRun(r.id)}>실행 화면</Button>
                        </div>
                        {arts.length ? (
                            <div className="row">
                                {arts.map((a) => (
                                    <Button key={a.id} size="sm" icon={FileText} aria-pressed={open === a.id}
                                            onClick={() => setOpen(open === a.id ? null : a.id)}>{a.outputKey} 결과</Button>
                                ))}
                            </div>
                        ) : null}
                        {arts.some((a) => a.id === open) && open ? (
                            <ArtifactViewer projectId={project.id} artifactId={open} onClose={() => setOpen(null)}/>
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
                            <dt>산출물</dt><dd>{p.outputs.map((o) => `${o.key} (${t.outputType[o.type] ?? o.type})`).join(', ')}</dd>
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
