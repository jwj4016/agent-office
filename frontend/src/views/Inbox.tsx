import {useState} from 'react';
import {api} from '../api';
import {t} from '../i18n';
import {useLoad} from '../live';
import type {ActionResult, InboxItem} from '../types';
import {ErrorBox, shortTime, useAction, useDetail} from '../ui';
import {ArtifactViewer} from './ArtifactViewer';

export function Inbox({onOpenRun}: { onOpenRun: (projectId: string, runId: string) => void }) {
    const {data, error, reload} = useLoad(() => api.inbox(), [], '*');
    const items = data ?? [];
    return (
        <div className="stack">
            <h1>{t.nav.inbox}</h1>
            <p className="muted">내가 맡은 작업·리뷰·승인과 AI의 질문입니다. 처리하지 않으면 자동으로 승인되지 않습니다.</p>
            <ErrorBox error={error}/>
            {data && items.length === 0 ? <p className="notice">지금 처리할 일이 없습니다.</p> : null}
            {items.map((it) => (
                <InboxCard key={`${it.kind}:${it.runId}:${it.attemptId}:${it.approvalId ?? ''}:${it.messageId ?? ''}`} item={it}
                           onDone={reload} onOpenRun={() => onOpenRun(it.projectId, it.runId)}/>
            ))}
        </div>
    );
}

function InboxCard({item, onDone, onOpenRun}: { item: InboxItem; onDone: () => void; onOpenRun: () => void }) {
    const action = useAction();
    const setDetail = useDetail();
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
        onDone();
        return true;
    };

    const inputs = item.inputs ?? [];
    return (
        <article className="card stack" aria-label={`${item.projectName} ${item.stepTitle}`}>
            <div className="row between">
                <div className="row">
                    <span className="badge warn">{t.inboxKind[item.kind]}</span>
                    <strong>{item.stepTitle}</strong>
                    {item.round > 0 ? <span className="badge">수정 {item.round}회차</span> : null}
                </div>
                <span className="muted small">{shortTime(item.since)}</span>
            </div>
            <div className="small muted">
                서비스 <strong>{item.projectName}</strong> · {item.runTitle} v{item.versionNumber} · 시도 {item.generation}-{item.attempt}
                {' '}<button className="btn small" onClick={onOpenRun}>실행 보기</button>
            </div>
            {item.instructions ? <p style={{whiteSpace: 'pre-wrap'}}>{item.instructions}</p> : null}
            {item.detail ? <div className="card" style={{background: 'var(--surface-2)'}}>{item.detail}</div> : null}
            {inputs.length ? (
                <div className="row small">
                    <span className="muted">확인할 자료:</span>
                    {inputs.map((m, i) => m.artifactId ? (
                        <button key={i} className="btn small" onClick={() => setDetail(
                            <ArtifactViewer projectId={item.projectId} artifactId={m.artifactId!} onClose={() => setDetail(null)}/>)}>
                            {m.name}</button>
                    ) : <span key={i} className="badge">{m.name}: 없음</span>)}
                </div>
            ) : null}
            {done ? <p className="notice">{done}</p> : (
                <>
                    {item.kind === 'approval' ? <ApprovalForm item={item} busy={action.busy} onSubmit={submit}/> : null}
                    {item.kind === 'review' ? <ReviewForm item={item} busy={action.busy} onSubmit={submit}/> : null}
                    {item.kind === 'task' ? <TaskForm item={item} busy={action.busy} onSubmit={submit}/> : null}
                    {item.kind === 'tool_approval' ? (
                        <div className="row">
                            <button className="btn primary" disabled={action.busy} onClick={() => submit(() => api.decideTool(item.projectId, item.approvalId!, true))}>허용</button>
                            <button className="btn danger" disabled={action.busy} onClick={() => submit(() => api.decideTool(item.projectId, item.approvalId!, false))}>거절</button>
                        </div>
                    ) : null}
                    {item.kind === 'question' ? <AnswerForm item={item} busy={action.busy} onSubmit={submit}/> : null}
                    {item.kind === 'budget' ? <p className="muted small">‘{item.stepTitle}’ 등 새 AI 업무가 시작되지 않고 있습니다. 해당 서비스의 개요에서 예산을 올리면 이어서 진행합니다.</p> : null}
                </>
            )}
            {note ? <p className="notice">{note}</p> : null}
            {problem ? <div className="alert" role="alert">제출이 완료 기준을 충족하지 않습니다: {problem}</div> : null}
            <ErrorBox error={action.error}/>
        </article>
    );
}

type FormProps = { item: InboxItem; busy: boolean; onSubmit: (fn: () => Promise<ActionResult>) => Promise<boolean> };

function TargetPicker({item, targets, setTargets}: { item: InboxItem; targets: string[]; setTargets: (t: string[]) => void }) {
    if (!item.reworkTargets.length) return <p className="muted small">이 업무에는 돌려보낼 대상이 지정되어 있지 않습니다.</p>;
    return (
        <fieldset className="row" style={{border: 'none', padding: 0}}>
            <legend className="small muted">돌려보낼 업무</legend>
            {item.reworkTargets.map((r) => (
                <label key={r.id} className="check">
                    <input type="checkbox" checked={targets.includes(r.id)}
                           onChange={(e) => setTargets(e.target.checked ? [...targets, r.id] : targets.filter((x) => x !== r.id))}/>{r.title}
                </label>
            ))}
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
            <div className="row">
                <button className="btn primary" disabled={busy} onClick={() => decide('approved')}>승인</button>
                <button className="btn" disabled={busy} onClick={() => setMode('hold')}>보류</button>
                <button className="btn danger" disabled={busy} onClick={() => setMode('reject')}>반려</button>
            </div>
            {mode !== 'none' ? (
                <div className="stack">
                    {mode === 'reject' ? <TargetPicker item={item} targets={targets} setTargets={setTargets}/> : null}
                    <label className="field"><span>{mode === 'reject' ? '반려 이유 (담당 AI에게 전달됩니다)' : '보류 이유'}</span>
                        <textarea value={reason} onChange={(e) => setReason(e.target.value)}/></label>
                    <div className="row">
                        <button className="btn primary" disabled={busy || !reason.trim() || (mode === 'reject' && !targets.length)}
                                onClick={() => decide(mode === 'reject' ? 'rejected' : 'held')}>{mode === 'reject' ? '반려하기' : '보류 기록'}</button>
                        <button className="btn" onClick={() => setMode('none')}>{t.common.cancel}</button>
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
            <label className="field"><span>리뷰 의견</span>
                <textarea value={comment} onChange={(e) => setComment(e.target.value)} placeholder="확인한 내용, 수정이 필요한 부분"/></label>
            {!changes && extra.map((o) => (
                <label key={o.key} className="field">
                    <span>{o.key} · {t.outputType[o.type]} · {o.required === false ? t.common.optional : t.common.required} (통과 시 제출)</span>
                    <textarea value={values[o.key] ?? ''} onChange={(e) => setValues({...values, [o.key]: e.target.value})}/>
                </label>
            ))}
            {changes ? <TargetPicker item={item} targets={targets} setTargets={setTargets}/> : null}
            <div className="row">
                {!changes ? (
                    <>
                        <button className="btn primary" disabled={busy} onClick={() => send('pass')}>통과</button>
                        <button className="btn danger" disabled={busy} onClick={() => setChanges(true)}>수정 요청…</button>
                    </>
                ) : (
                    <>
                        <button className="btn danger" disabled={busy || !comment.trim() || !targets.length} onClick={() => send('changes_requested')}>수정 요청 보내기</button>
                        <button className="btn" onClick={() => setChanges(false)}>{t.common.cancel}</button>
                    </>
                )}
            </div>
        </div>
    );
}

function TaskForm({item, busy, onSubmit}: FormProps) {
    const [values, setValues] = useState<Record<string, string>>({});
    return (
        <form className="stack" onSubmit={(e) => {
            e.preventDefault();
            onSubmit(() => api.submitHuman(item.projectId, item.attemptId, item.generation, values));
        }}>
            {item.outputs.map((o) => (
                <label key={o.key} className="field">
                    <span>{o.key} · {t.outputType[o.type]} · {o.required === false ? t.common.optional : t.common.required}</span>
                    <textarea value={values[o.key] ?? ''} onChange={(e) => setValues({...values, [o.key]: e.target.value})}
                              placeholder={o.type === 'markdown' ? '작업 결과를 작성하세요' : 'JSON'}/>
                </label>
            ))}
            <div className="row"><button className="btn primary" type="submit" disabled={busy}>제출</button></div>
        </form>
    );
}

function AnswerForm({item, busy, onSubmit}: FormProps) {
    const [answer, setAnswer] = useState('');
    return (
        <form className="stack" onSubmit={(e) => { e.preventDefault(); onSubmit(() => api.answer(item.projectId, item.messageId!, answer)); }}>
            <label className="field"><span>답변</span><textarea value={answer} onChange={(e) => setAnswer(e.target.value)}/></label>
            <div className="row"><button className="btn primary" type="submit" disabled={busy || !answer.trim()}>답변 보내기</button></div>
        </form>
    );
}
