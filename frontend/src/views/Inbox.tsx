import {Check, CheckCircle2, ExternalLink, FileText, FolderGit2, MessagesSquare, PauseCircle, Send, Undo2} from 'lucide-react';
import {useState} from 'react';
import {api} from '../api';
import {t} from '../i18n';
import {useLoad} from '../live';
import type {ActionResult, InboxItem} from '../types';
import {
    Banner, Button, EmptyState, ErrorBox, Field, Icon, inboxKindIcon, Loading, PageHeader, relativeTime, Section, shortTime, StatusBadge, useAction,
    useDetail, useToast,
} from '../ui';
import {ArtifactViewer} from './ArtifactViewer';
import {decisionKindCounts, decisionKindLabel, decisionKinds, type DecisionKind, InboxGroups} from './InboxGroups';
import {pendingCount} from './Judgments';
import {groupToolApprovals, ToolApprovalCard} from './ToolApprovals';

const itemKey = (it: InboxItem) => `${it.kind}:${it.runId}:${it.attemptId}:${it.approvalId ?? ''}:${it.messageId ?? ''}`;

export function Inbox({onOpenRun, onOpenWork}: {
    onOpenRun: (projectId: string, runId: string) => void; onOpenWork?: (projectId: string, workItemId: string) => void;
}) {
    const {data, error, reload} = useLoad(() => api.inbox(), [], '*');
    const decisions = useLoad(() => api.pendingDecisions(), [], '*');
    const decisionCount = pendingCount(decisions.data);
    // Oldest first: what has waited longest is handled first.
    const items = [...(data ?? [])].sort((a, b) => a.since.localeCompare(b.since));
    const groups = new Map<string, InboxItem[]>();
    for (const it of items) groups.set(it.projectName, [...(groups.get(it.projectName) ?? []), it]);
    // An AI step's tool requests share one card.
    const cards = (list: InboxItem[]) => groupToolApprovals(list).map((g) => Array.isArray(g)
        ? <ToolApprovalCard key={`tools:${g[0].attemptId}`} items={g} onDone={reload} onOpenRun={() => onOpenRun(g[0].projectId, g[0].runId)}/>
        : <InboxCard key={itemKey(g)} item={g} onDone={reload} onOpenRun={() => onOpenRun(g.projectId, g.runId)}/>);
    // One kind at a time, or everything ('all'); 'run' is what runs wait for.
    const [show, setShow] = useState<'all' | 'run' | DecisionKind>('all');
    const counts = decisions.data ? decisionKindCounts(decisions.data) : undefined;
    const kinds = decisionKinds.filter((k) => counts && counts[k] > 0);
    const total = items.length + decisionCount;
    return (
        <div className="page narrow" style={{maxWidth: 880}}>
            <PageHeader title={t.nav.inbox} badges={items.length + decisionCount ? <span className="badge warn count">{items.length + decisionCount}</span> : null}
                        desc="부서의 결과 검토·협업·제안·질문과 실행이 기다리는 승인·리뷰입니다. 공동 목표별로 묶어 보여 주고, 처리하지 않으면 자동으로 승인되지 않습니다."/>
            <ErrorBox error={error || decisions.error}/>
            {kinds.length + (items.length ? 1 : 0) > 1 ? (
                <div className="segmented wrap" role="group" aria-label="할 일 종류">
                    <button type="button" className="btn small" aria-pressed={show === 'all'} onClick={() => setShow('all')}>전체 {total}</button>
                    {kinds.map((k) => (
                        <button key={k} type="button" className="btn small" aria-pressed={show === k} onClick={() => setShow(k)}>
                            {decisionKindLabel[k]} {counts![k]}</button>
                    ))}
                    {items.length ? <button type="button" className="btn small" aria-pressed={show === 'run'} onClick={() => setShow('run')}>
                        실행 확인 {items.length}</button> : null}
                </div>
            ) : null}
            {decisions.data && show !== 'run' ? (
                <InboxGroups data={decisions.data} kind={show === 'all' ? undefined : show} onDone={decisions.reload} onOpenWork={onOpenWork} onOpenRun={onOpenRun}/>
            ) : null}
            {!data && !error ? <Loading/> : null}
            {data && items.length === 0 && decisionCount === 0 ? (
                <EmptyState icon={CheckCircle2} title="지금 처리할 일이 없습니다.">
                    AI가 승인·리뷰·답변을 기다리면 여기에 나타나고, 사이드바에 개수가 표시됩니다.
                </EmptyState>
            ) : null}
            {items.length && (show === 'all' || show === 'run') ? (
                <Section title="실행 확인" count={items.length}>
                    {groups.size > 1
                        ? [...groups].map(([name, list]) => (
                            <div key={name} className="stack" role="group" aria-label={name}>
                                <span className="kind-head">{name} <span className="muted">{list.length}</span></span>{cards(list)}</div>
                        ))
                        : <div className="stack">{cards(items)}</div>}
                </Section>
            ) : null}
        </div>
    );
}

function InboxCard({item, onDone, onOpenRun}: { item: InboxItem; onDone: () => void; onOpenRun: () => void }) {
    const action = useAction();
    const setDetail = useDetail();
    const toast = useToast();
    const [problem, setProblem] = useState<string | null>(null);
    const [done, setDone] = useState<string | null>(null);
    const [note, setNote] = useState<string | null>(null);

    // submit returns true when the item was finished (the form can go).
    const submit = async (fn: () => Promise<ActionResult>): Promise<boolean> => {
        setProblem(null);
        setNote(null);
        const res = await action.run(fn);
        if (!res) return false;
        if (res.problem) {
            setProblem(res.problem);
            return false;
        }
        if (res.outcome.decision === 'held') {
            // A hold is recorded but decides nothing: keep the controls.
            setNote('보류를 기록했습니다. 승인 또는 반려를 계속 기다립니다.');
            return true;
        }
        setDone(res.outcome.already ? '이미 처리된 항목입니다.' : '처리했습니다.');
        if (!res.outcome.already) toast(`‘${item.stepTitle}’ 처리 완료`);
        onDone();
        return true;
    };

    const inputs = item.inputs ?? [];
    return (
        <article className="card stack" aria-label={`${item.projectName} ${item.stepTitle}`}>
            <div className="row nowrap" style={{alignItems: 'flex-start'}}>
                <span className="icon-wrap warn"><Icon icon={inboxKindIcon[item.kind]} size={18}/></span>
                <div className="stack tight" style={{flex: 1, minWidth: 0}}>
                    <div className="row">
                        <StatusBadge tone="warn">{t.inboxKind[item.kind]}</StatusBadge>
                        {item.round > 0 ? <StatusBadge>수정 {item.round}회차</StatusBadge> : null}
                        <span className="spacer"/>
                        <span className="muted xsmall" title={shortTime(item.since)}>{relativeTime(item.since)}</span>
                    </div>
                    <h3 style={{fontSize: 'var(--fs-lg)'}}>{item.stepTitle}</h3>
                    <div className="row small muted">
                        <span>서비스 <strong style={{color: 'var(--text)'}}>{item.projectName}</strong></span>
                        <span>· {item.runTitle} v{item.versionNumber}</span>
                        <span>· 시도 {item.generation}-{item.attempt}</span>
                        <Button size="sm" variant="ghost" icon={ExternalLink} onClick={onOpenRun}>실행 보기</Button>
                    </div>
                </div>
            </div>

            {item.escalation ? <Banner tone="warn" title={t.messageKind.escalation}>{item.escalation}</Banner> : null}
            {item.instructions ? <p className="pre">{item.instructions}</p> : null}
            {item.detail ? <div className="card subtle tight pre">{item.detail}</div> : null}
            <WorkspaceNote ws={item.workspace}/>
            {item.context?.length ? (
                <details open={item.context.some((c) => c.label.startsWith('회의 의견'))}>
                    <summary className="small"><Icon icon={MessagesSquare} size={14}/> 관련 대화·결정 {item.context.length}건</summary>
                    <ul className="stack small" style={{listStyle: 'none', padding: 0, marginTop: 8}}>
                        {item.context.map((c, i) => (
                            <li key={i} className="card subtle tight">
                                <strong>{c.label}</strong> <span className="muted">({c.ref})</span>
                                <div className="pre">{c.text}</div>
                            </li>
                        ))}
                    </ul>
                </details>
            ) : null}
            {inputs.length ? (
                <div className="row small">
                    <span className="muted">확인할 자료</span>
                    {inputs.map((m, i) => m.artifactId ? (
                        <Button key={i} size="sm" icon={FileText} onClick={() => setDetail(
                            <ArtifactViewer projectId={item.projectId} artifactId={m.artifactId!} onClose={() => setDetail(null)}/>)}>
                            {m.name}</Button>
                    ) : <StatusBadge key={i}>{m.name}: 없음</StatusBadge>)}
                </div>
            ) : null}

            <hr className="divider"/>
            {done ? <Banner tone="ok">{done}</Banner> : (
                <>
                    {item.kind === 'approval' ? <ApprovalForm item={item} busy={action.busy} onSubmit={submit}/> : null}
                    {item.kind === 'review' ? <ReviewForm item={item} busy={action.busy} onSubmit={submit}/> : null}
                    {item.kind === 'task' ? <TaskForm item={item} busy={action.busy} onSubmit={submit}/> : null}
                    {item.kind === 'question' ? <AnswerForm item={item} busy={action.busy} onSubmit={submit}/> : null}
                    {item.kind === 'budget' ? (
                        <p className="muted small">‘{item.stepTitle}’ 등 새 AI 업무가 시작되지 않고 있습니다. 해당 서비스의 ‘설정’ 탭에서 예산을 올리면 이어서 진행합니다.</p>
                    ) : null}
                </>
            )}
            {note ? <Banner tone="info">{note}</Banner> : null}
            {problem ? <Banner tone="bad" title="제출이 완료 기준을 충족하지 않습니다">{problem}</Banner> : null}
            <ErrorBox error={action.error}/>
        </article>
    );
}

type FormProps = { item: InboxItem; busy: boolean; onSubmit: (fn: () => Promise<ActionResult>) => Promise<boolean> };

function TargetPicker({item, targets, setTargets}: { item: InboxItem; targets: string[]; setTargets: (t: string[]) => void }) {
    if (!item.reworkTargets.length) return <p className="muted small">이 업무에는 돌려보낼 대상이 지정되어 있지 않습니다.</p>;
    return (
        <fieldset className="stack tight">
            <legend className="field-label" style={{marginBottom: 6}}>돌려보낼 업무</legend>
            <div className="row">
                {item.reworkTargets.map((r) => (
                    <label key={r.id} className="check">
                        <input type="checkbox" checked={targets.includes(r.id)}
                               onChange={(e) => setTargets(e.target.checked ? [...targets, r.id] : targets.filter((x) => x !== r.id))}/>{r.title}
                    </label>
                ))}
            </div>
        </fieldset>
    );
}

function ApprovalForm({item, busy, onSubmit}: FormProps) {
    const [mode, setMode] = useState<'none' | 'reject' | 'hold'>('none');
    const [reason, setReason] = useState('');
    const [targets, setTargets] = useState<string[]>(item.reworkTargets.length === 1 ? [item.reworkTargets[0].id] : []);
    const decide = async (decision: string) => {
        const ok = await onSubmit(() =>
            api.decideApproval(item.projectId, item.approvalId!, item.generation, decision, reason, decision === 'rejected' ? targets : []));
        if (ok && decision === 'held') {
            setMode('none');
            setReason('');
        }
    };
    return (
        <div className="stack">
            <div className="form-actions">
                <Button variant="primary" icon={Check} busy={busy && mode === 'none'} disabled={busy} onClick={() => decide('approved')}>승인</Button>
                <Button icon={PauseCircle} disabled={busy} aria-pressed={mode === 'hold'} onClick={() => setMode('hold')}>보류</Button>
                <Button variant="danger" icon={Undo2} disabled={busy} aria-pressed={mode === 'reject'} onClick={() => setMode('reject')}>반려</Button>
            </div>
            {mode !== 'none' ? (
                <div className="card subtle stack">
                    {mode === 'reject' ? <TargetPicker item={item} targets={targets} setTargets={setTargets}/> : null}
                    <Field label={mode === 'reject' ? '반려 이유' : '보류 이유'} hint={mode === 'reject' ? '담당 AI에게 그대로 전달됩니다.' : '보류는 결정을 미룰 뿐, 승인이나 반려를 대신하지 않습니다.'}>
                        <textarea value={reason} autoFocus onChange={(e) => setReason(e.target.value)}/>
                    </Field>
                    <div className="form-actions">
                        <Button variant={mode === 'reject' ? 'danger-solid' : 'primary'} busy={busy}
                                disabled={!reason.trim() || (mode === 'reject' && !targets.length)}
                                onClick={() => decide(mode === 'reject' ? 'rejected' : 'held')}>{mode === 'reject' ? '반려하기' : '보류 기록'}</Button>
                        <Button onClick={() => setMode('none')}>{t.common.cancel}</Button>
                    </div>
                </div>
            ) : null}
        </div>
    );
}

function ReviewForm({item, busy, onSubmit}: FormProps) {
    const [comment, setComment] = useState('');
    const [changes, setChanges] = useState(false);
    const [targets, setTargets] = useState<string[]>([]);
    const [values, setValues] = useState<Record<string, string>>({});
    // The app writes the review report; other outputs come from the reviewer.
    const reportKey = item.outputs.find((o) => o.type === 'report')?.key;
    const extra = item.outputs.filter((o) => o.key !== reportKey);
    const send = (decision: string) => onSubmit(() =>
        api.submitReview(item.projectId, item.attemptId, item.generation, decision, comment,
            decision === 'changes_requested' ? targets : [], decision === 'pass' ? values : {}));
    return (
        <div className="stack">
            <Field label="리뷰 의견" optional={!changes} hint={changes ? '수정 요청에는 의견이 필요합니다.' : undefined}>
                <textarea value={comment} onChange={(e) => setComment(e.target.value)} placeholder="확인한 내용, 수정이 필요한 부분"/>
            </Field>
            {!changes && extra.map((o) => (
                <Field key={o.key} label={`${o.key} · ${t.outputType[o.type]}`} optional={o.required === false} hint="통과할 때 함께 제출됩니다.">
                    <textarea value={values[o.key] ?? ''} onChange={(e) => setValues({...values, [o.key]: e.target.value})}/>
                </Field>
            ))}
            {changes ? <TargetPicker item={item} targets={targets} setTargets={setTargets}/> : null}
            <div className="form-actions">
                {!changes ? (
                    <>
                        <Button variant="primary" icon={Check} busy={busy} onClick={() => send('pass')}>통과</Button>
                        <Button variant="danger" icon={Undo2} disabled={busy} onClick={() => setChanges(true)}>수정 요청…</Button>
                    </>
                ) : (
                    <>
                        <Button variant="danger-solid" busy={busy} disabled={!comment.trim() || !targets.length} onClick={() => send('changes_requested')}>수정 요청 보내기</Button>
                        <Button onClick={() => setChanges(false)}>{t.common.cancel}</Button>
                    </>
                )}
            </div>
        </div>
    );
}

function TaskForm({item, busy, onSubmit}: FormProps) {
    const [values, setValues] = useState<Record<string, string>>({});
    // Code changes are made in the workspace; the app commits them.
    const code = item.outputs.some((o) => o.type === 'code_change');
    const gitRun = item.workspace?.kind === 'code';
    return (
        <form className="stack" aria-label="작업 결과 제출" onSubmit={(e) => {
            e.preventDefault();
            onSubmit(() => api.submitHuman(item.projectId, item.attemptId, item.generation, values));
        }}>
            {code && gitRun ? <Banner tone="info" icon={FolderGit2}>작업 공간의 파일을 직접 고친 뒤 제출하세요. 앱이 이 브랜치에 커밋하고 변경 목록을 기록합니다.</Banner> : null}
            {item.outputs.filter((o) => o.type !== 'code_change' || !gitRun).map((o) => (
                <Field key={o.key} label={`${o.key} · ${t.outputType[o.type]}`} optional={o.required === false}>
                    <textarea value={values[o.key] ?? ''} rows={6} onChange={(e) => setValues({...values, [o.key]: e.target.value})}
                              placeholder={o.type === 'markdown' ? '작업 결과를 작성하세요' : 'JSON'}/>
                </Field>
            ))}
            <div className="form-actions"><Button variant="primary" type="submit" icon={Send} busy={busy}>제출</Button></div>
        </form>
    );
}

// WorkspaceNote shows where a person's task or review works.
function WorkspaceNote({ws}: { ws: InboxItem['workspace'] }) {
    if (!ws || ws.kind === 'shared') return null;
    return (
        <div className="card subtle tight stack tight small">
            <div className="row nowrap"><Icon icon={FolderGit2} size={14}/>
                <span className="muted">{ws.kind === 'code' ? '작업 공간' : '확인용 작업 공간(읽기 전용)'}</span>
                <span className="mono">{ws.path}</span></div>
            {ws.branch ? <div className="muted">브랜치 <span className="mono">{ws.branch}</span></div> : null}
            {ws.conflicts.length ? <Banner tone="bad">병합 충돌을 해결하세요: {ws.conflicts.join(', ')}</Banner> : null}
            {ws.note && !ws.conflicts.length ? <div className="muted">{ws.note}</div> : null}
        </div>
    );
}

function AnswerForm({item, busy, onSubmit}: FormProps) {
    const [answer, setAnswer] = useState('');
    return (
        <form className="stack" aria-label="답변" onSubmit={(e) => { e.preventDefault(); onSubmit(() => api.answer(item.projectId, item.messageId!, answer)); }}>
            <Field label="답변" hint="질문한 AI에게 그대로 전달됩니다."><textarea value={answer} onChange={(e) => setAnswer(e.target.value)}/></Field>
            <div className="form-actions"><Button variant="primary" type="submit" icon={Send} busy={busy} disabled={!answer.trim()}>답변 보내기</Button></div>
        </form>
    );
}
