import {
    Ban, Check, ClipboardList, Sparkles, CornerDownRight, FileDown, FolderOutput, Inbox as InboxIcon, Play, RotateCcw, Send, Settings2, Undo2, X,
} from 'lucide-react';
import {useState} from 'react';
import {api} from '../api';
import {useLoad} from '../live';
import type {
    ActionPayload, Assignment, Department, DeliveryMethod, Project, RequestItem, RequestStatus, RoutePayload, ServiceRequest,
    WorkProposalPayload,
} from '../types';
import {Banner, Button, EmptyState, ErrorBox, Field, Icon, Loading, PageHeader, relativeTime, Section, StatusBadge, type Tone, useAction, useToast} from '../ui';
import {FolderInput} from './FolderInput';

// Requests to the company or a service (docs/plan/company-requests): the
// person writes what they want; the intake desk answers, asks back,
// passes it on, proposes work, or proposes an action the person approves.

export const requestStatusLabel: Record<RequestStatus, string> = {
    received: '접수 대기', waiting_desk: '접수 담당 필요', held: '보류', triaging: '접수 담당이 보는 중', question: '질문에 답해 주세요',
    manual: '직접 처리 필요', proposed: '결정 필요', routed: '서비스로 넘김', in_progress: '조치 실행 중', done: '끝남',
    declined: '처리 불가', cancelled: '취소됨', failed: '접수 실패',
};

const requestTone: Record<RequestStatus, Tone> = {
    received: 'info', waiting_desk: 'warn', held: 'warn', triaging: 'info', question: 'warn', manual: 'warn', proposed: 'warn',
    routed: 'info', in_progress: 'info', done: 'ok', declined: '', cancelled: '', failed: 'bad',
};

const closed = (s: RequestStatus) => s === 'done' || s === 'declined' || s === 'cancelled' || s === 'failed';

export const actionLabel: Record<string, string> = {
    deliver_code: '코드 결과 전달', save_result: '결과 파일 저장', export_service: '서비스 내보내기', start_workflow: '업무 흐름 실행',
};

// RequestComposer writes a new request. With services it asks the company
// and may name the service directly (skipping the company desk).
export function RequestComposer({projectId, services, previous, onSent}: {
    projectId: string; services?: Project[]; previous?: ServiceRequest; onSent?: () => void;
}) {
    const [body, setBody] = useState('');
    const [target, setTarget] = useState('');
    const action = useAction();
    const toast = useToast();
    const where = services ? target : projectId;
    const send = () => action.run(async () => {
        await api.submitRequest(where, body, previous?.id ?? '');
        setBody('');
        toast('요청을 보냈습니다. 접수 담당이 보고 처리합니다');
        onSent?.();
    });
    return (
        <div className="card stack">
            {previous ? <p className="small muted"><Icon icon={CornerDownRight} size={14}/> 이어서 요청: {previous.body}</p> : null}
            <Field label={previous ? '이어서 할 요청' : '요청'} hint="하고 싶은 일을 말하듯 적으세요. 접수 담당이 답하거나, 업무·조치로 만들거나, 되묻습니다.">
                <textarea rows={3} value={body} onChange={(e) => setBody(e.target.value)} maxLength={4000}
                          placeholder={services ? '예: 빌라노트 코드 내 폴더로 보내 줘' : '예: QA·전문가 검토에서 나온 문제 고쳐 줘'}/>
            </Field>
            {services ? (
                <Field label="받을 곳" hint="비워 두면 회사 접수 담당이 어느 서비스의 일인지 가려 넘깁니다.">
                    <select value={target} onChange={(e) => setTarget(e.target.value)}>
                        <option value="">회사 접수 담당이 정하기</option>
                        {services.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
                    </select>
                </Field>
            ) : null}
            <div className="row">
                <Button variant="primary" icon={Send} disabled={!body.trim()} busy={action.busy} onClick={send}>요청 보내기</Button>
            </div>
            <ErrorBox error={action.error}/>
        </div>
    );
}

type Lookups = { departments: Department[]; assignments: Assignment[] };

// RequestCard shows one request, what its desk decided, and what waits
// on the person.
export function RequestCard({req, onChanged, onOpenWork, nested}: {
    req: ServiceRequest; onChanged: () => void; onOpenWork?: (projectId: string, workItemId: string) => void; nested?: boolean;
}) {
    const action = useAction();
    const toast = useToast();
    const [reply, setReply] = useState('');
    const [following, setFollowing] = useState(false);
    const hasWork = req.items.some((i) => i.kind === 'work');
    const look = useLoad(async (): Promise<Lookups> => req.projectId && hasWork
        ? {departments: await api.departments(req.projectId), assignments: await api.assignments(req.projectId)}
        : {departments: [], assignments: []}, [req.projectId, hasWork]);
    const act = (fn: () => Promise<unknown>, done: string) => action.run(async () => { await fn(); toast(done); onChanged(); });
    const where = req.projectName || '회사';
    return (
        <article className={`card stack ${nested ? 'subtle tight' : ''}`} aria-label={`${where} 요청: ${req.body}`}>
            <div className="row between" style={{alignItems: 'flex-start'}}>
                <div className="stack tight" style={{minWidth: 0}}>
                    <span className="small muted">{nested ? `→ ${where}` : where} · {relativeTime(req.createdAt)}{req.round > 1 ? ' · 2회차' : ''}</span>
                    <strong className="pre">{req.body}</strong>
                </div>
                <StatusBadge tone={requestTone[req.status]}>{requestStatusLabel[req.status]}</StatusBadge>
            </div>

            {req.staffedDesk ? <p className="small muted"><Icon icon={Sparkles} size={14}/> 접수 담당이 없어 AI가 서비스를 분석해 ‘{req.staffedDesk}’를 새로 배치했습니다. 지침은 부서·담당자 화면에서 보고 바꿀 수 있습니다.</p> : null}
            {req.status === 'received' || req.status === 'triaging'
                ? <p className="small muted">접수 담당이 요청을 보고 있습니다.</p> : null}
            {req.status === 'waiting_desk' || req.status === 'held' ? <Banner tone="warn">{req.holdReason}</Banner> : null}
            {req.answer ? <div className="card subtle tight pre">{req.answer}</div> : null}
            {req.reply ? <p className="small muted pre">앞선 질문: {req.question} · 답: {req.reply}</p> : null}
            {req.status === 'question' ? (
                <div className="stack tight">
                    <Banner tone="warn" title="접수 담당의 질문">{req.question}</Banner>
                    <Field label="답">
                        <textarea rows={2} value={reply} onChange={(e) => setReply(e.target.value)}/>
                    </Field>
                    <div className="row">
                        <Button variant="primary" size="sm" icon={Send} disabled={!reply.trim()} busy={action.busy}
                                onClick={() => act(() => api.answerRequest(req.projectId, req.id, reply), '답을 보냈습니다')}>답 보내기</Button>
                    </div>
                </div>
            ) : null}
            {req.status === 'manual' ? (
                <Banner tone="warn" title="접수 담당이 두 번 되물었습니다" actions={
                    <Button size="sm" icon={Check} busy={action.busy} onClick={() => act(() => api.closeRequest(req.projectId, req.id, ''), '요청을 닫았습니다')}>직접 처리하고 닫기</Button>}>
                    {req.question} — 업무 화면에서 직접 업무를 만들거나, 다른 말로 새로 요청하세요.
                </Banner>
            ) : null}
            {req.status === 'declined' ? <Banner tone="info" title="이 앱으로 처리할 수 없는 요청입니다">{req.reason}</Banner> : null}
            {req.status === 'failed' ? (
                <Banner tone="bad" title="접수하지 못했습니다" actions={
                    <Button size="sm" icon={RotateCcw} busy={action.busy} onClick={() => act(() => api.retryRequest(req.projectId, req.id), '다시 접수합니다')}>다시 시도</Button>}>
                    {req.error}
                </Banner>
            ) : null}
            {req.reason && req.status !== 'declined' ? <p className="small muted pre">접수 담당의 판단: {req.reason}</p> : null}

            {req.items.filter((i) => i.kind === 'action').map((i) => <ActionItem key={i.id} req={req} item={i} onChanged={onChanged}/>)}
            {req.items.filter((i) => i.kind === 'work').map((i) => (
                <WorkProposalItem key={i.id} req={req} item={i} look={look.data} onChanged={onChanged} onOpenWork={onOpenWork}/>
            ))}
            {req.children.map((c) => <RequestCard key={c.id} req={c} onChanged={onChanged} onOpenWork={onOpenWork} nested/>)}
            {req.items.filter((i) => i.kind === 'route' && !req.children.some((c) => c.id === i.childRequestId)).map((i) => (
                <p key={i.id} className="small muted">→ {(i.payload as RoutePayload).body}</p>
            ))}

            <div className="row">
                {!closed(req.status) && req.status !== 'triaging' && !nested ? (
                    <Button size="sm" variant="ghost" icon={Ban} busy={action.busy} onClick={() => act(() => api.cancelRequest(req.projectId, req.id), '요청을 취소했습니다')}>요청 취소</Button>
                ) : null}
                {closed(req.status) && !nested && !following ? (
                    <Button size="sm" variant="ghost" icon={CornerDownRight} onClick={() => setFollowing(true)}>이어서 요청</Button>
                ) : null}
            </div>
            {following ? <RequestComposer projectId={req.projectId} previous={req} onSent={() => { setFollowing(false); onChanged(); }}/> : null}
            <ErrorBox error={action.error}/>
        </article>
    );
}

function WorkProposalItem({req, item, look, onChanged, onOpenWork}: {
    req: ServiceRequest; item: RequestItem; look?: Lookups; onChanged: () => void; onOpenWork?: (projectId: string, workItemId: string) => void;
}) {
    const p = item.payload as WorkProposalPayload;
    const [title, setTitle] = useState(p.title);
    const action = useAction();
    const toast = useToast();
    const dept = look?.departments.find((d) => d.id === p.departmentId)?.name;
    const person = look?.assignments.find((a) => a.id === p.assigneeAssignmentId)?.displayName;
    const act = (fn: () => Promise<unknown>, done: string) => action.run(async () => { await fn(); toast(done); onChanged(); });
    return (
        <div className="card subtle tight stack tight" role="group" aria-label={`업무 제안: ${p.title}`}>
            <span className="row small"><Icon icon={ClipboardList} size={14}/><strong>업무 제안</strong>
                <span className="muted">{dept ? `부서 ${dept}` : '부서 없음'}{person ? ` · 담당 ${person}` : ''}{p.priority && p.priority !== 'normal' ? ` · ${p.priority}` : ''}</span>
                {item.decidedBy === 'auto' ? <StatusBadge tone="info">자동 등록</StatusBadge> : null}</span>
            {item.status === 'proposed' ? (
                <>
                    <Field label="업무 제목"><input value={title} onChange={(e) => setTitle(e.target.value)}/></Field>
                    {p.description ? <span className="small pre">{p.description}</span> : null}
                    {p.doneCriteria ? <span className="small muted pre">완료 조건: {p.doneCriteria}</span> : null}
                    <div className="row">
                        <Button size="sm" variant="primary" icon={Check} disabled={!title.trim()} busy={action.busy}
                                onClick={() => act(() => api.acceptRequestWork(req.projectId, item.id, {title}), '업무를 등록했습니다')}>업무 등록</Button>
                        <Button size="sm" variant="ghost" icon={X} busy={action.busy}
                                onClick={() => act(() => api.rejectRequestItem(req.projectId, item.id, ''), '제안을 거절했습니다')}>거절</Button>
                    </div>
                </>
            ) : item.workItemId ? (
                <span className="row small">
                    {onOpenWork ? <button className="btn ghost small" onClick={() => onOpenWork(req.projectId, item.workItemId)}>{item.workTitle || p.title}</button>
                        : <span>{item.workTitle || p.title}</span>}
                    <span className="muted">등록됨{item.workStatus ? ` · ${item.workStatus}` : ''}</span>
                </span>
            ) : <span className="small muted">{p.title} · 거절함</span>}
            <ErrorBox error={action.error}/>
        </div>
    );
}

// ActionItem is an action the desk proposed. It runs only when the person
// approves; code delivery asks how and where first.
function ActionItem({req, item, onChanged}: { req: ServiceRequest; item: RequestItem; onChanged: () => void }) {
    const p = item.payload as ActionPayload;
    const action = useAction();
    const toast = useToast();
    const [folder, setFolder] = useState(p.folder ?? '');
    const [useAsWorkspace, setUseAsWorkspace] = useState(true);
    const code = useLoad(() => p.action === 'deliver_code' && p.runId ? api.finalCode(req.projectId, p.runId) : Promise.resolve(undefined),
        [req.projectId, p.runId, item.status]);
    const open = item.status === 'proposed' || item.status === 'failed' || item.status === 'check';
    const run = (method?: DeliveryMethod) => action.run(async () => {
        try {
            const out = await api.runRequestAction({projectId: req.projectId, itemId: item.id, method, folder, useAsWorkspace});
            if (out.cancelled) return;
            toast(`${actionLabel[p.action]}을 실행했습니다`);
        } finally {
            onChanged();
        }
    });
    const act = (fn: () => Promise<unknown>, done: string) => action.run(async () => { await fn(); toast(done); onChanged(); });
    const fc = code.data;
    return (
        <div className="card subtle tight stack tight" role="group" aria-label={`조치 제안: ${actionLabel[p.action]}`}>
            <span className="row small"><Icon icon={p.action === 'deliver_code' ? FolderOutput : p.action === 'start_workflow' ? Play : FileDown} size={14}/>
                <strong>{actionLabel[p.action]}</strong>{p.title ? <span className="muted">‘{p.title}’</span> : null}
                {item.status === 'done' ? <StatusBadge tone="ok">실행함</StatusBadge> : null}
                {item.status === 'running' ? <StatusBadge tone="info">실행 중</StatusBadge> : null}
                {item.status === 'rejected' ? <StatusBadge>거절함</StatusBadge> : null}</span>
            {p.reason ? <span className="small pre">{p.reason}</span> : null}
            {item.status === 'failed' ? <Banner tone="bad" title="조치가 실패했습니다">{item.error}</Banner> : null}
            {item.status === 'check' ? (
                <Banner tone="warn" title="조치를 실행하는 동안 앱이 꺼졌습니다" actions={
                    <Button size="sm" icon={Check} busy={action.busy} onClick={() => act(() => api.confirmRequestAction(req.projectId, item.id, ''), '확인했습니다')}>
                        끝난 것으로 확인</Button>}>
                    결과를 확인하고, 끝났으면 확인을 누르고 아니면 다시 실행하세요.
                </Banner>
            ) : null}
            {item.status === 'done' && item.result ? <ActionResultLine p={p} result={item.result}/> : null}
            {open && p.action === 'deliver_code' ? (
                code.error ? <ErrorBox error={code.error}/> : !fc ? <Loading rows={1}/> : fc.appRepo ? (
                    <>
                        <Field label="꺼낼 폴더" hint={p.folder && folder === p.folder
                            ? '접수 담당이 요청을 보고 제안한 폴더입니다. 바꿀 수 있습니다. 비어 있거나 아직 없는 폴더만 됩니다.'
                            : '비어 있거나 아직 없는 폴더만 됩니다. 비워 두면 기준 코드만 갱신합니다.'}>
                            <FolderInput value={folder} onChange={setFolder} title="코드를 꺼낼 폴더 선택"/>
                        </Field>
                        <label className="check block">
                            <input type="checkbox" checked={useAsWorkspace} onChange={(e) => setUseAsWorkspace(e.target.checked)}/>
                            이 폴더를 서비스 작업 폴더로 쓰기<span className="muted xsmall"> · 새로 확정하는 업무 흐름 버전과 새 업무부터 이 폴더에서 작업합니다. 이미 확정한 버전은 그대로 앱 저장소에서 작업하고, 그 기준 코드도 이 결과로 갱신됩니다</span>
                        </label>
                        <div className="row">
                            <Button size="sm" variant="primary" icon={Check} busy={action.busy} onClick={() => run(folder ? 'folder' : 'base')}>
                                {folder ? '승인: 폴더로 꺼내기' : '승인: 기준 코드만 갱신'}</Button>
                            <Button size="sm" variant="ghost" icon={X} busy={action.busy}
                                    onClick={() => act(() => api.rejectRequestItem(req.projectId, item.id, ''), '조치를 거절했습니다')}>거절</Button>
                        </div>
                    </>
                ) : (
                    <div className="row">
                        <Button size="sm" variant="primary" icon={Check} busy={action.busy} onClick={() => run('branch')}>승인: 결과 브랜치 만들기</Button>
                        <Button size="sm" icon={Check} busy={action.busy} onClick={() => run('advance')}>승인: 현재 브랜치에도 반영</Button>
                        <Button size="sm" variant="ghost" icon={X} busy={action.busy}
                                onClick={() => act(() => api.rejectRequestItem(req.projectId, item.id, ''), '조치를 거절했습니다')}>거절</Button>
                    </div>
                )
            ) : open ? (
                <div className="row">
                    <Button size="sm" variant="primary" icon={Check} busy={action.busy} onClick={() => run()}>
                        {p.action === 'start_workflow' ? '승인: 실행 시작 (AI 비용이 듭니다)' : '승인: 저장 위치 고르기'}</Button>
                    <Button size="sm" variant="ghost" icon={X} busy={action.busy}
                            onClick={() => act(() => api.rejectRequestItem(req.projectId, item.id, ''), '조치를 거절했습니다')}>거절</Button>
                </div>
            ) : null}
            <ErrorBox error={action.error}/>
        </div>
    );
}

function ActionResultLine({p, result}: { p: ActionPayload; result: Record<string, unknown> }) {
    const s = (k: string) => (typeof result[k] === 'string' ? result[k] as string : '');
    switch (p.action) {
    case 'deliver_code':
        return <span className="small mono">{[s('folder'), s('branch'), s('commit').slice(0, 8)].filter(Boolean).join(' · ')}{s('note') ? ` — ${s('note')}` : ''}</span>;
    case 'start_workflow':
        return <span className="small">실행을 시작했습니다{s('runId') ? ` (${s('runId')})` : ''}</span>;
    default:
        return <span className="small mono">{s('path')}</span>;
    }
}

// RequestList shows a service's requests (projectId) or the company's.
export function RequestList({projectId, onOpenWork, empty}: {
    projectId: string; onOpenWork?: (projectId: string, workItemId: string) => void; empty: string;
}) {
    const list = useLoad(() => api.requests({projectId}), [projectId], '*');
    if (list.error) return <ErrorBox error={list.error}/>;
    if (!list.data) return <Loading/>;
    if (list.data.length === 0) return <EmptyState icon={InboxIcon} title="아직 요청이 없습니다" compact>{empty}</EmptyState>;
    return (
        <div className="stack" aria-label="요청 목록">
            {list.data.map((r) => <RequestCard key={r.id} req={r} onChanged={list.reload} onOpenWork={onOpenWork}/>)}
        </div>
    );
}

// DeskNotice says when a desk is missing or off, with where to set it up.
function DeskNotice({projectId, onSetup}: { projectId: string; onSetup?: () => void }) {
    const desk = useLoad(() => api.deskView(projectId), [projectId], '*');
    if (!desk.data || (desk.data.exists && desk.data.desk.enabled)) return null;
    return (
        <Banner tone={desk.data.exists ? 'warn' : 'info'} title={desk.data.exists ? '접수 담당이 ‘사용 안 함’입니다' : '아직 접수 담당이 없습니다'}
                actions={onSetup ? <Button size="sm" icon={Settings2} onClick={onSetup}>접수 담당 정하기</Button> : null}>
            {desk.data.exists ? '요청은 받아 두고, 다시 켜면 이어서 처리합니다.' : '요청을 보내면 AI가 분석해 알맞은 접수 담당을 배치하고 이어서 처리합니다. 직접 정할 수도 있습니다.'}
        </Banner>
    );
}

// ServiceRequests is a service's ‘요청’ tab.
export function ServiceRequests({project, onOpenWork, onTeam}: {
    project: Project; onOpenWork?: (projectId: string, workItemId: string) => void; onTeam?: () => void;
}) {
    return (
        <div className="stack loose">
            <DeskNotice projectId={project.id} onSetup={onTeam}/>
            {project.status === 'archived' ? null : <RequestComposer projectId={project.id}/>}
            <Section title="요청 기록" label="요청 기록">
                <RequestList projectId={project.id} onOpenWork={onOpenWork} empty="서비스에 요청하면 접수 담당이 처리하고, 그 결과가 여기에 남습니다."/>
            </Section>
        </div>
    );
}

// CompanyRequests is the ‘회사에 요청’ screen.
export function CompanyRequests({services, onOpenWork, onOrg}: {
    services: Project[]; onOpenWork?: (projectId: string, workItemId: string) => void; onOrg?: () => void;
}) {
    return (
        <div className="page narrow" style={{maxWidth: 880}}>
            <PageHeader title="회사에 요청" desc="회사에 말로 맡기면 회사 접수 담당이 어느 서비스의 일인지 가려 그 서비스의 접수 담당에게 넘깁니다."/>
            <div className="stack loose">
                <DeskNotice projectId="" onSetup={onOrg}/>
                <RequestComposer projectId="" services={services}/>
                <Section title="회사 요청 기록" label="회사 요청 기록">
                    <RequestList projectId="" onOpenWork={onOpenWork} empty="회사에 한 요청과 각 서비스가 처리한 결과가 여기에 남습니다."/>
                </Section>
            </div>
        </div>
    );
}

// RequestTodos lists requests waiting on the person, for the inbox.
export function RequestTodos({requests, onChanged, onOpenWork}: {
    requests: ServiceRequest[]; onChanged: () => void; onOpenWork?: (projectId: string, workItemId: string) => void;
}) {
    if (requests.length === 0) return null;
    return (
        <Section title={<><Icon icon={Undo2} size={18}/>요청</>} count={requests.length} label="요청">
            <div className="stack">
                {requests.map((r) => <RequestCard key={r.id} req={r} onChanged={onChanged} onOpenWork={onOpenWork}/>)}
            </div>
        </Section>
    );
}
