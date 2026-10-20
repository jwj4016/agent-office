import {ArrowDown, ArrowLeft, MessageSquare, RefreshCw, Send, Terminal} from 'lucide-react';
import {useEffect, useRef, useState} from 'react';
import {api} from '../api';
import {t} from '../i18n';
import {useLoad} from '../live';
import {runEnded} from '../runInsight';
import type {MessageView, Project, StoredEvent} from '../types';
import {Banner, Button, DetailHeader, EmptyState, ErrorBox, Field, Icon, Loading, RunBadge, shortTime, StatusBadge, Tabs, useAction, useDetail, useToast} from '../ui';

type Source = {projectId: string; runId?: string; assistId?: string};
type Entry = {id: string; kind: string; sequence: number; time: string; title: string; body: string; stepId?: string; assignmentId?: string; attempt?: number; attemptId?: string};

const eventTitles: Record<string, string> = {
    'activity.output': 'AI 출력', 'provider.message': 'AI 응답', 'provider.completed': 'AI 응답', 'side.completed': '보조 AI 응답',
    'provider.tool': '도구 실행', 'provider.error': 'AI 오류', 'provider.question': 'AI 질문', 'provider.approval_request': '도구 사용 요청',
    'assist.request': '내 수정 요청', 'assist.ready': '검토 준비 완료', 'assist.failed': '요청 실패',
    'step.running': '업무 시작', 'step.verifying': '결과 확인', 'step.succeeded': '업무 완료', 'step.failed': '업무 실패',
    'step.cancelled': '업무 취소', 'step.interrupted': '업무 중단', 'step.waiting_input': '답변 대기', 'step.waiting_approval': '승인 대기',
    'meeting.started': '회의 시작', 'meeting.finished': '회의 종료', 'side.tool_declined': '보조 AI 도구 요청 거절',
};

// Only normalized public text is displayed; raw provider/session payloads stay out of the UI.
export function activityEntry(ev: StoredEvent): Entry | null {
    const p = (ev.payload ?? {}) as Record<string, unknown>;
    const title = eventTitles[ev.kind];
    if (!title) return null;
    const str = (key: string) => typeof p[key] === 'string' ? p[key] as string : '';
    const body = str('text') || str('detail') || str('message') || str('error');
    if (['provider.message', 'provider.completed', 'side.completed', 'activity.output'].includes(ev.kind) && !body) return null;
    const tool = ev.kind === 'provider.tool' ? [str('name'), str('status')].filter(Boolean).join(' · ') : '';
    return {id: ev.id, kind: ev.kind, sequence: ev.sequence, time: ev.createdAt, title: [title, tool, str('phase')].filter(Boolean).join(' · '), body,
        stepId: ev.stepId, assignmentId: ev.assignmentId, attempt: ev.attempt, attemptId: ev.stepAttemptId};
}

// Shared by workflow requests and run/desk activity. New entries append without
// moving the reader; the explicit button acknowledges and jumps to the latest.
export function ActivityFeed({projectId, runId = '', assistId = '', stepId = '', assignmentId = '', stepNames = {}, actorNames = {}}: Source & {
    stepId?: string; assignmentId?: string; stepNames?: Record<string, string>; actorNames?: Record<string, string>;
}) {
    const page = useLoad(() => api.aiActivity(projectId, runId, assistId), [projectId, runId, assistId], projectId);
    const conversation = useLoad(() => runId ? api.runMessages(projectId, runId) : Promise.resolve([]), [projectId, runId], projectId);
    const [tab, setTab] = useState<'conversation' | 'logs'>('conversation');
    const [events, setEvents] = useState<StoredEvent[]>([]);
    const [seen, setSeen] = useState<number | null>(null);
    const [seenMessages, setSeenMessages] = useState<Set<string> | null>(null);
    const [more, setMore] = useState<boolean | undefined>(undefined);
    const action = useAction();
    const end = useRef<HTMLDivElement>(null);
    useEffect(() => {
        if (!page.data) return;
        setEvents((old) => [...new Map([...old, ...page.data!.events].map((e) => [e.sequence, e])).values()].sort((a, b) => a.sequence - b.sequence));
        setSeen((old) => old ?? Math.max(0, ...page.data!.events.map((e) => e.sequence)));
    }, [page.data]);
    useEffect(() => {
        if (conversation.data) setSeenMessages((old) => old ?? new Set(conversation.data!.map((m) => m.id)));
    }, [conversation.data]);

    const entries = events.filter((e) => !stepId || e.stepId === stepId).map(activityEntry).filter((e): e is Entry => !!e);
    const messages = (conversation.data ?? []).filter((m) => (!stepId || m.stepId === stepId || m.recipient === `step:${stepId}`)
        && (!assignmentId || m.sender === assignmentId || m.recipient === assignmentId || !!stepId));
    // Side sessions already have attributed answer/proposal messages. Their
    // transport response (often JSON) belongs in logs, not a second speaker.
    const responses = entries.filter((e) => ['provider.message', 'provider.completed', 'assist.request', 'assist.ready', 'assist.failed'].includes(e.kind));
    // Some providers repeat the final message in their completion event.
    const unique = responses.filter((e) => e.kind !== 'provider.completed' || !responses.some((p) => p.kind === 'provider.message' && p.body === e.body && p.attemptId === e.attemptId));
    const displayed = tab === 'logs' ? entries : [...unique, ...messages.map(messageEntry)].sort((a, b) => a.time.localeCompare(b.time) || a.sequence - b.sequence);
    const fresh = entries.filter((e) => e.sequence > (seen ?? Infinity)).length
        + messages.filter((m) => seenMessages && !seenMessages.has(m.id)).length;
    const showLatest = () => {
        setSeen(Math.max(0, ...events.map((e) => e.sequence)));
        setSeenMessages(new Set((conversation.data ?? []).map((m) => m.id)));
        end.current?.scrollIntoView?.({block: 'end'});
    };
    const older = () => action.run(async () => {
        const next = await api.aiActivity(projectId, runId, assistId, events[0]?.sequence ?? 0);
        setEvents((old) => [...new Map([...next.events, ...old].map((e) => [e.sequence, e])).values()].sort((a, b) => a.sequence - b.sequence));
        setMore(next.hasMore);
    });
    return <div className="stack activity-feed">
        <Tabs label="AI 활동 종류" items={[{id: 'conversation', label: '대화·결정', icon: MessageSquare}, {id: 'logs', label: '실행 로그', icon: Terminal}]} value={tab} onChange={setTab}/>
        <div className="activity-updates" role="status" aria-atomic="true">
            {fresh > 0 ? <Button size="sm" icon={ArrowDown} onClick={showLatest}>새 기록 {fresh}개 · 최신으로 이동</Button> : <span className="muted small">새 기록은 읽던 위치를 유지하며 추가됩니다.</span>}
        </div>
        <ErrorBox error={page.error ?? conversation.error ?? action.error}/>
        {page.error ? <Button size="sm" icon={RefreshCw} onClick={page.reload}>기록 다시 불러오기</Button> : null}
        {page.data === undefined && !page.error ? <Loading rows={2}/> : <>
            {(more ?? page.data?.hasMore) ? <Button size="sm" busy={action.busy} onClick={older}>이전 기록 불러오기</Button> : null}
            {!displayed.length ? <EmptyState compact title={tab === 'logs' ? '아직 실행 로그가 없습니다' : '아직 기록된 대화가 없습니다'}>
                {tab === 'conversation' ? '진행 중인 공개 출력은 실행 로그에서 확인할 수 있습니다.' : '이전에 저장되지 않은 실시간 출력은 복원할 수 없습니다.'}
            </EmptyState> : <ol className="activity-entries stack" aria-label={tab === 'logs' ? 'AI 실행 로그' : 'AI 대화·결정'}>
                {displayed.map((e) => <li key={e.id} className="card tight stack">
                    <div className="row small between"><strong>{e.title}</strong><time dateTime={e.time} className="muted xsmall">{shortTime(e.time)}</time></div>
                    {e.stepId ? <div className="muted xsmall">{stepNames[e.stepId] ?? e.stepId}{e.assignmentId ? ` · ${actorNames[e.assignmentId] ?? e.assignmentId}` : ''}{e.attempt ? ` · 시도 ${e.attempt}` : ''}</div> : null}
                    {e.body ? (e.body.length > 2000 ? <details><summary className="small">긴 내용 펼치기 · {e.body.length.toLocaleString()}자</summary><div className="pre activity-text">{e.body}</div></details>
                        : <div className={tab === 'logs' ? 'pre mono small activity-text' : 'pre activity-text'}>{e.body}</div>) : null}
                </li>)}
            </ol>}
        </>}
        <div ref={end}/>
    </div>;
}

function messageEntry(m: MessageView): Entry {
    return {id: m.id, kind: m.kind, sequence: 0, time: m.createdAt, title: `${t.messageKind[m.kind]} · ${m.senderName} → ${m.recipientName}${m.refs.round ? ` · 회의 ${m.refs.round}라운드` : ''}`,
        body: m.body + (m.refs.artifacts?.length ? `\n\n근거: ${m.refs.artifacts.map((a) => `${a.stepId ?? ''}.${a.outputKey} v${a.version}`).join(', ')}` : ''),
        stepId: m.stepId || (m.recipient.startsWith('step:') ? m.recipient.slice(5) : undefined)};
}

export function AIActivityPanel({project, runId, stepId: initialStep = '', assignmentId = '', onBack, onInbox}: {
    project: Project; runId: string; stepId?: string; assignmentId?: string; onBack?: () => void; onInbox: () => void;
}) {
    const run = useLoad(() => api.run(project.id, runId), [project.id, runId], project.id);
    const team = useLoad(() => api.assignments(project.id), [project.id], project.id);
    const inbox = useLoad(() => api.inbox(), [], project.id);
    const [stepId, setStepId] = useState(initialStep);
    const [body, setBody] = useState('');
    const action = useAction();
    const toast = useToast();
    const setDetail = useDetail();
    const step = run.data?.steps.find((s) => s.id === stepId);
    const relevantAssignment = stepId === initialStep ? assignmentId : '';
    const who = team.data?.find((a) => a.id === (relevantAssignment || step?.assignmentId));
    const pending = (inbox.data ?? []).filter((i) => i.projectId === project.id && i.runId === runId && (!stepId || i.stepId === stepId));
    return <div className="stack">
        <DetailHeader title="AI 활동" onClose={() => setDetail(null)}>
            <span className="small">{project.name}</span>
        </DetailHeader>
        {onBack ? <Button variant="ghost" size="sm" icon={ArrowLeft} onClick={onBack}>담당자 상세로</Button> : null}
        <ErrorBox error={run.error ?? team.error}/>
        {!run.data ? (run.error ? null : <Loading/>) : <>
            <div className="card tight stack"><strong>{run.data.title} · v{run.data.versionNumber}</strong><RunBadge status={run.data.status}/>
                <span className="small">{step?.title ?? '전체 업무'} · {who?.displayName ?? (step?.assignmentId || '전체 담당자')}</span>
            </div>
            <Field label="확인할 업무"><select value={stepId} onChange={(e) => setStepId(e.target.value)}>
                <option value="">전체 업무</option>{run.data.steps.map((s) => <option key={s.id} value={s.id}>{s.title}</option>)}
            </select></Field>
            {pending.length ? <Banner tone="warn" title={`내 응답이 필요한 항목 ${pending.length}건`} actions={<Button size="sm" onClick={onInbox}>내 할 일에서 처리</Button>}/> : null}
            <ActivityFeed key={`${runId}/${stepId}`} projectId={project.id} runId={runId} stepId={stepId} assignmentId={relevantAssignment}
                stepNames={Object.fromEntries(run.data.steps.map((s) => [s.id, s.title]))}
                actorNames={Object.fromEntries((team.data ?? []).map((a) => [a.id, a.displayName]))}/>
            {!runEnded(run.data.status) && project.status !== 'archived' ? <form className="card stack" onSubmit={(e) => {
                e.preventDefault(); action.run(async () => { await api.postNote(project.id, runId, stepId, 'decision', body); setBody(''); toast('다음 시도에 전달할 메모를 남겼습니다'); });
            }}>
                <Field label="다음 시도에 전달할 메모" hint="이미 진행 중인 작업에는 즉시 전달되지 않습니다. 업무를 선택한 뒤 남겨 주세요.">
                    <textarea value={body} onChange={(e) => setBody(e.target.value)}/>
                </Field>
                <ErrorBox error={action.error}/>
                <Button type="submit" icon={Send} busy={action.busy} disabled={!body.trim() || !step || step.kind === 'condition' || step.kind === 'join'}>메모 남기기</Button>
            </form> : <StatusBadge>기록 보기</StatusBadge>}
        </>}
    </div>;
}
