import {ArrowLeft, ChevronRight, FileText, GitBranch, Hand, MessageSquare, Pause, Play, RotateCcw, Terminal, XCircle} from 'lucide-react';
import {useState} from 'react';
import {api} from '../api';
import {t} from '../i18n';
import {useLive, useLoad} from '../live';
import {blockedSteps, blockReason, canRetry, currentSteps, progress, runControls, runEnded} from '../runInsight';
import type {MessageKind, Project, RunDetail, StepView} from '../types';
import {
    Banner, Button, ConfirmButton, EmptyState, ErrorBox, Field, Icon, Loading, nodeKindIcon, ProgressBar, RunBadge, Section, shortTime,
    StatusBadge, StepBadge, stepIcon, stepTone, type Tone, runTone, useAction, useDetail, useToast,
} from '../ui';
import {ArtifactViewer} from './ArtifactViewer';

export function Runs({project, runId, onOpen}: { project: Project; runId?: string; onOpen: (runId?: string) => void }) {
    const list = useLoad(() => api.runs(project.id), [project.id], project.id);
    if (runId) return <RunView project={project} runId={runId} onBack={() => onOpen(undefined)}/>;
    if (list.error) return <ErrorBox error={list.error}/>;
    if (!list.data) return <Loading/>;
    if (list.data.length === 0) {
        return (
            <EmptyState icon={Play} title="아직 실행 기록이 없습니다">
                ‘업무 흐름’ 탭에서 흐름을 열고 버전을 확정한 뒤 실행하면 여기에 기록됩니다.
            </EmptyState>
        );
    }
    return (
        <ul className="stack" style={{listStyle: 'none', padding: 0, margin: 0}} aria-label="실행 목록">
            {list.data.map((r) => {
                const p = progress(r);
                return (
                    <li key={r.id}>
                        <button className="card clickable row nowrap" onClick={() => onOpen(r.id)} aria-label={`${r.title} v${r.versionNumber}, ${t.runStatus[r.status]}`}>
                            <span className="stack tight" style={{flex: 1, minWidth: 0}}>
                                <span className="row nowrap">
                                    <strong className="truncate">{r.title}</strong>
                                    <span className="muted small">v{r.versionNumber}</span>
                                </span>
                                <ProgressBar done={p.done} total={p.total} tone={runTone[r.status]}/>
                            </span>
                            <RunBadge status={r.status}/>
                            <Icon icon={ChevronRight}/>
                        </button>
                    </li>
                );
            })}
        </ul>
    );
}

const markerTone = (s: StepView): Tone => stepTone[s.status] || '';

function RunView({project, runId, onBack}: { project: Project; runId: string; onBack: () => void }) {
    const {data, error, reload} = useLoad(() => api.run(project.id, runId), [project.id, runId], project.id);
    const live = useLive();
    const action = useAction();
    const toast = useToast();
    const setDetail = useDetail();
    if (error) return <ErrorBox error={error}/>;
    if (!data) return <Loading/>;
    const run: RunDetail = data;
    const finished = runEnded(run.status);
    const controls = runControls(run);
    const p = progress(run);
    const blocked = blockedSteps(run);
    const waiting = currentSteps(run).filter((s) => s.status.startsWith('waiting'));
    const act = (fn: () => Promise<void>, done: string) => action.run(async () => { await fn(); reload(); toast(done); });
    const retry = (s: StepView) => act(() => api.retryStep(project.id, runId, s.id), `‘${s.title}’을 다시 시도합니다`);
    const openArtifact = (artifactId: string) =>
        setDetail(<ArtifactViewer projectId={project.id} artifactId={artifactId} onClose={() => setDetail(null)}/>);

    return (
        <div className="stack loose">
            <div><Button variant="ghost" size="sm" icon={ArrowLeft} onClick={onBack}>실행 목록</Button></div>

            <div className="card stack">
                <div className="row between">
                    <div className="row" style={{minWidth: 0}}>
                        <h2>{run.title} <span className="muted small">v{run.versionNumber}</span></h2>
                        <RunBadge status={run.status}/>
                    </div>
                    {!finished ? (
                        <div className="row">
                            {controls.resume ? <Button icon={Play} busy={action.busy} onClick={() => act(() => api.resumeRun(project.id, runId), '실행을 재개했습니다')}>재개</Button> : null}
                            {controls.pause ? <Button icon={Pause} busy={action.busy} onClick={() => act(() => api.pauseRun(project.id, runId), '일시 정지했습니다')}>일시 정지</Button> : null}
                            {controls.cancel ? <ConfirmButton label="실행 취소" confirmLabel="취소하기" className="btn danger" icon={XCircle}
                                                              onConfirm={() => act(() => api.cancelRun(project.id, runId), '실행을 취소했습니다')}/> : null}
                        </div>
                    ) : null}
                </div>
                <ProgressBar done={p.done} total={p.total} tone={runTone[run.status]} label={`${p.total}개 중 ${p.done}개 완료`}/>
                {run.repo?.note ? <p className="small muted" role="note"><Icon icon={GitBranch} size={14}/> {run.repo.note}</p> : null}
            </div>

            <ErrorBox error={action.error}/>
            {run.paused ? <Banner tone="warn" icon={Pause} title="일시 정지 중">진행 중인 업무는 계속되고, 새 업무는 시작하지 않습니다.</Banner> : null}
            {run.status === 'cancelled' ? <Banner tone="info" title="취소된 실행">이미 만들어진 파일이나 외부 조치는 되돌려지지 않았습니다.</Banner> : null}
            {run.status === 'succeeded' ? <Banner tone="ok" title="모든 업무를 마쳤습니다">결과물은 각 업무 아래에서 열어 볼 수 있습니다.</Banner> : null}

            {blocked.length ? (
                <Section title="실행이 멈춘 이유" label="실행이 멈춘 이유">
                    {blocked.map((s) => {
                        const why = blockReason(s);
                        return (
                            <Banner key={s.id} tone="bad" title={s.title}
                                    actions={canRetry(run, s) ? <Button variant="primary" size="sm" icon={RotateCcw} busy={action.busy} onClick={() => retry(s)}>다시 시도</Button> : null}>
                                <div>{why.text}</div>
                                {!why.recorded ? (
                                    <div className="small" style={{marginTop: 4}}>
                                        다시 시도하면 같은 입력으로 새 시도를 시작합니다. 원인을 알려 주거나 방향을 바꾸려면 아래 ‘대화·결정’에 메모를 남긴 뒤 다시 시도하세요.
                                    </div>
                                ) : null}
                            </Banner>
                        );
                    })}
                </Section>
            ) : null}

            {waiting.length ? (
                <Banner tone="warn" icon={Hand} title="사람을 기다리는 업무가 있습니다">
                    {waiting.map((s) => s.title).join(', ')} — ‘내 할 일’에서 처리하세요.
                </Banner>
            ) : null}

            <Section title="업무 진행" label="업무 진행">
                <ol className="timeline card">
                    {run.steps.map((s) => (
                        <li key={s.id} className={`timeline-item ${s.status === 'pending' ? 'is-pending' : ''}`}>
                            <span className={`timeline-marker ${markerTone(s)}`}>
                                <Icon icon={stepIcon[s.status]} size={14} className={s.status === 'running' ? 'spin' : undefined}/>
                            </span>
                            <div className="timeline-body">
                                <div className="timeline-title">
                                    <strong>{s.title}</strong>
                                    <StatusBadge icon={nodeKindIcon[s.kind]}>{t.nodeKind[s.kind]}</StatusBadge>
                                    <StepBadge status={s.status}/>
                                    {s.round > 0 ? <span className="muted small">수정 {s.round}회</span> : null}
                                    {s.attempt > 1 ? <span className="muted small">시도 {s.attempt}</span> : null}
                                    <span className="spacer"/>
                                    {canRetry(run, s) && !blocked.includes(s)
                                        ? <Button size="sm" icon={RotateCcw} busy={action.busy} onClick={() => retry(s)}>다시 시도</Button> : null}
                                </div>
                                {s.error ? <p className="small" style={{color: 'var(--danger-text)'}}>{s.error}</p> : null}
                                {s.workspace?.kind === 'code' ? (
                                    <p className="small muted mono" title={s.workspace.path}>
                                        <Icon icon={GitBranch} size={12}/> {s.workspace.branch}{s.workspace.commit ? ` @ ${s.workspace.commit.slice(0, 8)}` : ''}
                                    </p>
                                ) : null}
                                {s.workspace?.conflicts.length ? <p className="small" style={{color: 'var(--danger-text)'}}>병합 충돌: {s.workspace.conflicts.join(', ')}</p> : null}
                                {s.attemptId && live.deltas[s.attemptId] ? (
                                    <details open>
                                        <summary className="small"><Icon icon={Terminal} size={14}/> 실시간 출력</summary>
                                        <pre className="content" aria-live="polite">{live.deltas[s.attemptId].slice(-600)}</pre>
                                    </details>
                                ) : null}
                                {s.artifacts.length ? (
                                    <div className="row">
                                        {s.artifacts.map((a) => (
                                            <Button key={a.id} size="sm" icon={FileText} onClick={() => openArtifact(a.id)}>{a.outputKey} v{a.version}</Button>
                                        ))}
                                    </div>
                                ) : null}
                            </div>
                        </li>
                    ))}
                </ol>
            </Section>
            <Conversation project={project} run={run} finished={finished}/>
        </div>
    );
}

const messageTone: Partial<Record<MessageKind, Tone>> = {question: 'warn', escalation: 'bad', decision: 'ok', review_request: 'info'};

// Conversation is the run's record of questions, answers, handoffs,
// review requests, proposals, decisions and escalations, plus a form to
// leave a note for a step's next attempt.
export function Conversation({project, run, finished}: { project: Project; run: RunDetail; finished: boolean }) {
    const list = useLoad(() => api.runMessages(project.id, run.id), [project.id, run.id], project.id);
    const notable = run.steps.filter((s) => s.kind !== 'condition' && s.kind !== 'join');
    const [step, setStep] = useState('');
    const [kind, setKind] = useState<'proposal' | 'decision'>('decision');
    const [body, setBody] = useState('');
    const action = useAction();
    const toast = useToast();
    const post = () => action.run(async () => {
        await api.postNote(project.id, run.id, step, kind, body);
        setBody('');
        list.reload();
        toast('메모를 남겼습니다');
    });
    const msgs = list.data ?? [];
    return (
        <Section title={<><Icon icon={MessageSquare} size={18}/>대화·결정</>} label="대화·결정 기록">
            <ErrorBox error={list.error}/>
            {msgs.length === 0 ? <p className="muted small">아직 기록된 대화가 없습니다.</p> : (
                <ul className="stack" style={{listStyle: 'none', padding: 0, margin: 0}}>
                    {msgs.map((m) => (
                        <li key={m.id} className="card tight stack" style={{gap: 'var(--s-2)'}}>
                            <div className="row small">
                                <StatusBadge tone={messageTone[m.kind] ?? ''}>{t.messageKind[m.kind] ?? m.kind}</StatusBadge>
                                <strong>{m.senderName}</strong><span className="muted">→ {m.recipientName}</span>
                                {m.refs.round ? <span className="muted">회의 {m.refs.round}라운드{m.refs.agree ? ' · 동의' : ''}</span> : null}
                                <span className="spacer"/>
                                <span className="muted xsmall">{shortTime(m.createdAt)}</span>
                            </div>
                            <div className="pre">{m.body}</div>
                            {m.refs.artifacts?.length ? (
                                <div className="muted xsmall">근거: {m.refs.artifacts.map((r) => `${r.stepId ?? ''}.${r.outputKey} v${r.version}`).join(', ')}</div>
                            ) : null}
                        </li>
                    ))}
                </ul>
            )}
            {!finished ? (
                <form className="card stack" aria-label="메모 남기기" onSubmit={(e) => { e.preventDefault(); post(); }}>
                    <div className="grid-2">
                        <Field label="받을 업무">
                            <select aria-label="메모 받을 업무" value={step} onChange={(e) => setStep(e.target.value)}>
                                <option value="">업무 선택…</option>
                                {notable.map((s) => <option key={s.id} value={s.id}>{s.title}</option>)}
                            </select>
                        </Field>
                        <Field label="종류">
                            <select aria-label="메모 종류" value={kind} onChange={(e) => setKind(e.target.value as 'proposal' | 'decision')}>
                                <option value="decision">{t.messageKind.decision}</option>
                                <option value="proposal">{t.messageKind.proposal}</option>
                            </select>
                        </Field>
                    </div>
                    <Field label="내용" hint="이미 작업 중인 시도에는 전달되지 않고, 다음 시도(시작·재시도·수정)부터 반영됩니다.">
                        <textarea aria-label="메모 내용" value={body} onChange={(e) => setBody(e.target.value)}
                                  placeholder="이 업무의 다음 시도에 전달할 결정이나 제안"/>
                    </Field>
                    <ErrorBox error={action.error}/>
                    <div className="form-actions">
                        <Button type="submit" busy={action.busy} disabled={!step || !body.trim()}>메모 남기기</Button>
                    </div>
                </form>
            ) : null}
        </Section>
    );
}
