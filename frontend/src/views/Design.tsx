import {ArrowLeft, Bot, Check, ChevronRight, Lightbulb, Loader2, PenLine, Play, RefreshCw, Sparkles, Trash2, Users, Workflow} from 'lucide-react';
import {useEffect, useState} from 'react';
import {api} from '../api';
import {t} from '../i18n';
import {useLoad} from '../live';
import type {DesignView} from '../types';
import {
    ActorBadge, Banner, Button, ConfirmButton, EmptyState, ErrorBox, Field, Icon, IssueList, Loading, nodeKindIcon, PageHeader, relativeTime, Section,
    StatusBadge, type Tone, useAction, useToast,
} from '../ui';

const statusLabel: Record<DesignView['status'], [string, Tone]> = {
    drafting: ['설계 중', 'info'], ready: ['검토 대기', 'warn'], failed: ['실패', 'bad'], applied: ['적용됨', 'ok'], discarded: ['버림', ''],
};

const examples = [
    '작은 웹 게임을 출시하고 싶어. 코드 리뷰는 내가 할게.',
    '우리 회사 소개 사이트를 새로 만들고 싶어. 문구는 내가 승인할게.',
    '매주 업계 뉴스를 조사해서 요약 보고서를 받고 싶어.',
];

export function Design({onOpenWorkflow, onOpenRun}: {
    onOpenWorkflow: (projectId: string, workflowId: string) => void; onOpenRun: (projectId: string, runId: string) => void;
}) {
    const list = useLoad(() => api.designs(), [], '*');
    const [selected, setSelected] = useState<string | null>(null);
    const designs = list.data ?? [];
    return (
        <div className="page narrow" style={{maxWidth: 880}}>
            <PageHeader title={t.nav.design}
                        desc="이루고 싶은 목표를 적으면 설계 AI가 필요한 역할·담당자·업무 흐름을 제안합니다. 제안은 검토한 뒤 적용하며, 도구 권한이나 키를 새로 만들지 않습니다."/>
            {selected
                ? <DesignDetail id={selected} onBack={() => setSelected(null)} onOpenWorkflow={onOpenWorkflow} onOpenRun={onOpenRun} onChanged={list.reload}/>
                : <RequestForm onCreated={(id) => { setSelected(id); list.reload(); }}/>}
            <ErrorBox error={list.error}/>
            {designs.length ? (
                <Section title="요청 기록" count={designs.length} label="설계 요청">
                    <ul className="attention" style={{listStyle: 'none', padding: 0, margin: 0}}>
                        {designs.map((d) => (
                            <li key={d.id}>
                                <button className="row-button" aria-current={selected === d.id ? 'true' : undefined} onClick={() => setSelected(d.id)}>
                                    <span className={`icon-wrap ${statusLabel[d.status][1]}`}>
                                        <Icon icon={d.status === 'drafting' ? Loader2 : Sparkles} size={18} className={d.status === 'drafting' ? 'spin' : undefined}/>
                                    </span>
                                    <span className="body">
                                        <span className="title truncate">{d.goal.length > 80 ? d.goal.slice(0, 80) + '…' : d.goal}</span>
                                        <span className="meta">{d.mode === 'auto' ? t.project.modeAuto : t.project.modeReview} · {relativeTime(d.createdAt)}</span>
                                    </span>
                                    <StatusBadge tone={statusLabel[d.status][1]}>{statusLabel[d.status][0]}</StatusBadge>
                                    <Icon icon={ChevronRight}/>
                                </button>
                            </li>
                        ))}
                    </ul>
                </Section>
            ) : null}
        </div>
    );
}

const splitTasks = (s: string) => s.split(/[\n,]/).map((x) => x.trim()).filter(Boolean);
const optNum = (v: string) => (v.trim() === '' ? undefined : Number(v));

function RequestForm({onCreated}: { onCreated: (id: string) => void }) {
    const projects = useLoad(() => api.dashboard(false), []);
    const conns = useLoad(() => api.connections(), []);
    const usable = (conns.data ?? []).filter((c) => c.usable);
    const [goal, setGoal] = useState('');
    const [target, setTarget] = useState('');
    const [projectName, setProjectName] = useState('');
    const [tasks, setTasks] = useState('');
    const [mode, setMode] = useState<'review' | 'auto'>('review');
    const [connectionId, setConnectionId] = useState('');
    const [model, setModel] = useState('');
    const [maxTokens, setMaxTokens] = useState('');
    const [allowed, setAllowed] = useState<string[]>([]);
    const action = useAction();
    useEffect(() => {
        if (!connectionId && usable.length) setConnectionId(usable.find((c) => c.provider !== 'test')?.id ?? usable[0].id);
    }, [usable, connectionId]);
    const conn = usable.find((c) => c.id === connectionId);

    const submit = () => action.run(async () => {
        const d = await api.startDesign({
            goal, projectId: target, projectName, humanTasks: splitTasks(tasks), mode, connectionId, model,
            budget: {maxTokens: optNum(maxTokens)}, allowedConnectionIds: allowed,
        });
        setGoal('');
        onCreated(d.id);
    });

    return (
        <form className="card stack loose" aria-label="목표 요청" onSubmit={(e) => e.preventDefault()}>
            <fieldset className="stack">
                <legend className="row" style={{marginBottom: 'var(--s-3)'}}><span className="badge info count">1</span><h2>무엇을 이루고 싶나요?</h2></legend>
                <Field label="목표" hint="결과물, 기한, 꼭 지킬 조건을 적을수록 설계가 정확해집니다.">
                    <textarea value={goal} rows={4} onChange={(e) => setGoal(e.target.value)} placeholder="예: 작은 웹 게임을 출시하고 싶어. 코드 리뷰는 내가 할게."/>
                </Field>
                {!goal.trim() ? (
                    <div className="row small">
                        <span className="muted"><Icon icon={Lightbulb} size={14}/> 예시</span>
                        {examples.map((x) => <Button key={x} size="sm" variant="ghost" onClick={() => setGoal(x)}>{x.split('.')[0]}</Button>)}
                    </div>
                ) : null}
            </fieldset>

            <fieldset className="stack">
                <legend className="row" style={{marginBottom: 'var(--s-3)'}}><span className="badge info count">2</span><h2>어디에, 누가 맡나요?</h2></legend>
                <div className="grid-2">
                    <Field label="대상 서비스">
                        <select value={target} onChange={(e) => setTarget(e.target.value)}>
                            <option value="">새 서비스 만들기</option>
                            {(projects.data ?? []).map((s) => <option key={s.project.id} value={s.project.id}>{s.project.name} (업무 흐름 추가)</option>)}
                        </select></Field>
                    {target === '' ? (
                        <Field label="새 서비스 이름" optional hint="비우면 AI가 제안합니다.">
                            <input value={projectName} onChange={(e) => setProjectName(e.target.value)}/></Field>
                    ) : <span/>}
                </div>
                <Field label="내가 직접 맡을 업무" optional hint="쉼표나 줄로 구분합니다. 적은 업무가 설계안에서 나에게 배정되지 않으면 적용할 수 없습니다.">
                    <input value={tasks} onChange={(e) => setTasks(e.target.value)} placeholder="예: 코드 리뷰, 출시 승인"/></Field>
            </fieldset>

            <fieldset className="stack">
                <legend className="row" style={{marginBottom: 'var(--s-3)'}}><span className="badge info count">3</span><h2>어떻게 실행할까요?</h2></legend>
                <div className="grid-2">
                    <label className="choice">
                        <input type="radio" name="design-mode" checked={mode === 'review'} onChange={() => setMode('review')}/>
                        <span><span className="choice-title">{t.project.modeReview} (기본)</span>
                            <span className="choice-desc" style={{display: 'block'}}>설계안을 확인하고 고친 뒤 직접 실행합니다.</span></span>
                    </label>
                    <label className="choice">
                        <input type="radio" name="design-mode" checked={mode === 'auto'} onChange={() => setMode('auto')}/>
                        <span><span className="choice-title">{t.project.modeAuto}</span>
                            <span className="choice-desc" style={{display: 'block'}}>허용한 범위 안에서 바로 적용·시작합니다. 사람 업무와 승인은 기다립니다.</span></span>
                    </label>
                </div>
                {mode === 'auto' ? (
                    <div className="card subtle stack">
                        {target === '' ? (
                            <>
                                <Field label="토큰 예산" hint="자동 실행에는 예산이 필요합니다.">
                                    <input inputMode="numeric" value={maxTokens} onChange={(e) => setMaxTokens(e.target.value)}/></Field>
                                <fieldset className="stack tight">
                                    <legend className="field-label" style={{marginBottom: 6}}>자동 실행에 쓸 수 있는 연결</legend>
                                    {usable.map((c) => (
                                        <label key={c.id} className="check">
                                            <input type="checkbox" checked={allowed.includes(c.id)}
                                                   onChange={(e) => setAllowed(e.target.checked ? [...allowed, c.id] : allowed.filter((x) => x !== c.id))}/>{c.name}
                                        </label>
                                    ))}
                                </fieldset>
                            </>
                        ) : <p className="muted small">기존 서비스는 그 서비스의 ‘설정’ 탭에서 정한 예산과 허용 연결을 사용합니다.</p>}
                    </div>
                ) : null}
                <details>
                    <summary className="small">설계에 쓸 AI: {conn ? conn.name : '없음'}{model ? ` · ${model}` : ''} <span className="muted">(바꾸기)</span></summary>
                    <div className="grid-2" style={{marginTop: 'var(--s-3)'}}>
                        <Field label="설계에 쓸 AI 연결">
                            <select value={connectionId} onChange={(e) => setConnectionId(e.target.value)}>
                                {usable.length === 0 ? <option value="">사용 가능한 연결 없음</option> : null}
                                {usable.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}
                            </select></Field>
                        <Field label="모델" optional hint="비우면 연결의 기본 모델"><input value={model} onChange={(e) => setModel(e.target.value)}/></Field>
                    </div>
                </details>
                {conns.data && usable.length === 0
                    ? <Banner tone="warn" title="사용 가능한 AI 연결이 없습니다">설정 → AI 연결에서 연결을 추가하고 ‘실제 호출 시험’을 통과시키세요.</Banner> : null}
            </fieldset>

            <ErrorBox error={action.error}/>
            <div className="form-actions">
                <ConfirmButton label="설계안 만들기" confirmLabel="AI 사용량을 써서 설계" className="btn primary" icon={Sparkles}
                               disabled={action.busy || !goal.trim() || !connectionId} onConfirm={submit}/>
                <span className="muted small">설계에는 선택한 AI의 사용량이 듭니다.</span>
            </div>
        </form>
    );
}

function DesignDetail({id, onBack, onOpenWorkflow, onOpenRun, onChanged}: {
    id: string; onBack: () => void; onOpenWorkflow: (projectId: string, workflowId: string) => void;
    onOpenRun: (projectId: string, runId: string) => void; onChanged: () => void;
}) {
    const {data, error, reload} = useLoad(() => api.design(id), [id]);
    const action = useAction();
    const toast = useToast();
    const [issues, setIssues] = useState<DesignView['autoIssues']>([]);
    // Drafting runs in the background: check again every 2 seconds.
    useEffect(() => {
        if (data?.status !== 'drafting') return;
        const timer = window.setInterval(reload, 2000);
        return () => window.clearInterval(timer);
    }, [data?.status, reload]);
    // The request list shows the status too: refresh it when this one changes.
    const status = data?.status;
    useEffect(() => {
        if (status) onChanged();
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [status]);
    if (error) return <ErrorBox error={error}/>;
    if (!data) return <Loading/>;
    const r = data.result;
    const apply = (auto: boolean) => action.run(async () => {
        const res = await api.applyDesign(id, auto);
        setIssues(res.issues);
        onChanged();
        if (res.applied) {
            toast(auto ? '적용하고 실행을 시작했습니다' : '설계안을 적용했습니다');
            if (res.runId) onOpenRun(res.applied.projectId, res.runId);
            else if (!auto) onOpenWorkflow(res.applied.projectId, res.applied.workflowId);
        }
        reload();
    });
    return (
        <section className="card stack" aria-label="설계안">
            <div className="row between">
                <Button variant="ghost" size="sm" icon={ArrowLeft} onClick={onBack}>새 요청 쓰기</Button>
                <StatusBadge tone={statusLabel[data.status][1]}>{statusLabel[data.status][0]}</StatusBadge>
            </div>
            <div className="stack tight">
                <h2>설계안</h2>
                <p className="pre muted">{data.goal}</p>
                {data.request.humanTasks?.length ? <p className="small">내가 맡을 업무: {data.request.humanTasks.join(', ')}</p> : null}
            </div>
            {data.status === 'drafting' ? (
                <Banner tone="info" icon={Loader2} title="설계 AI가 작업 중입니다…">
                    역할·담당자·업무 흐름을 구성하고 있습니다. 몇 분 걸릴 수 있고, 이 화면을 떠나도 계속 진행됩니다.
                </Banner>
            ) : null}
            {data.status === 'failed' ? <Banner tone="bad" title="설계하지 못했습니다">{data.error}</Banner> : null}
            {data.status === 'failed' && data.proposal && typeof data.proposal === 'object' && Object.keys(data.proposal).length > 0 ? (
                <div><Button icon={RefreshCw} busy={action.busy}
                             onClick={() => action.run(async () => { await api.reparseDesign(id); onChanged(); reload(); })}>
                    저장된 답변 다시 검사 (AI 호출 없음)</Button></div>
            ) : null}
            {data.status === 'applied' && data.applied?.projectId && data.applied.workflowId ? (
                <div><Button icon={Workflow} onClick={() => onOpenWorkflow(data.applied!.projectId!, data.applied!.workflowId!)}>적용된 업무 흐름 열기</Button></div>
            ) : null}
            {r && data.status === 'ready' ? <ProposalView view={data}/> : null}
            <IssueList issues={issues}/>
            <ErrorBox error={action.error}/>
            {data.status === 'ready' ? (
                <div className="form-actions">
                    <Button variant="primary" icon={PenLine} busy={action.busy} disabled={!r?.canApply} onClick={() => apply(false)}>적용하고 편집하기</Button>
                    {data.mode === 'auto' ? (
                        <Button variant="primary" icon={Play} disabled={action.busy || !r?.canApply || data.autoIssues.length > 0}
                                onClick={() => apply(true)}>적용하고 자동 시작</Button>
                    ) : null}
                    <span className="spacer"/>
                    <ConfirmButton label="버리기" confirmLabel="버리기" className="btn danger" icon={Trash2}
                                   onConfirm={() => action.run(async () => { await api.discardDesign(id); onChanged(); reload(); })}/>
                </div>
            ) : null}
        </section>
    );
}

function ProposalView({view}: { view: DesignView }) {
    const r = view.result!;
    const d = r.diff;
    const missing = d.missing ?? [];
    return (
        <div className="stack loose">
            <div className="card subtle stack tight">
                <div className="row">
                    <strong>{r.proposal.project.name || '(이름 없음)'}</strong>
                    {view.projectId ? <StatusBadge>기존 서비스에 추가</StatusBadge> : <StatusBadge tone="info">새 서비스</StatusBadge>}
                </div>
                <p className="small muted">{r.proposal.project.goal}</p>
            </div>

            {missing.length ? (
                <Banner tone="warn" title="부족한 항목">
                    <ul style={{margin: 0, paddingLeft: 18}}>{missing.map((m, i) => <li key={i}>{m.detail}</li>)}</ul>
                </Banner>
            ) : null}

            <Section title={<><Icon icon={Users} size={18}/>역할</>} label="역할">
                <div className="row">
                    {(d.reuseRoles ?? []).map((x) => <StatusBadge key={x.ref} icon={Check}>{x.name} · 재사용</StatusBadge>)}
                    {(d.newRoles ?? []).map((x) => <StatusBadge key={x.ref} tone="info" icon={Sparkles}>{x.name} · 새 역할</StatusBadge>)}
                </div>
                {(d.roleChanges ?? []).length ? (
                    <div className="card subtle stack tight">
                        <strong className="small">기존 역할 수정 제안 (자동으로 적용되지 않음)</strong>
                        {(d.roleChanges ?? []).map((c) => (
                            <div key={c.roleId} className="small">{c.roleName}: {c.reason}<div className="muted">제안 지침: {c.instructions}</div></div>
                        ))}
                    </div>
                ) : null}
            </Section>

            <Section title={<><Icon icon={Bot} size={18}/>담당자</>} label="담당자">
                <div className="table-wrap">
                    <table className="list">
                        <thead><tr><th>이름</th><th>역할</th><th>종류</th><th>연결</th></tr></thead>
                        <tbody>
                        {(d.assignments ?? []).map((a) => (
                            <tr key={a.ref}>
                                <td>{a.displayName}</td><td>{a.roleName}</td><td><ActorBadge kind={a.actorKind}/></td>
                                <td>{a.actorKind === 'human' ? '나' : a.connected ? a.connectionName : <StatusBadge tone="bad">연결 필요</StatusBadge>}</td>
                            </tr>
                        ))}
                        </tbody>
                    </table>
                </div>
            </Section>

            <Section title={<><Icon icon={Workflow} size={18}/>업무 흐름: {r.proposal.workflow.title}</>} label="업무 흐름">
                <ol className="timeline card">
                    {(d.steps ?? []).map((s) => (
                        <li key={s.id} className="timeline-item">
                            <span className={`timeline-marker ${s.human ? 'warn' : 'info'}`}><Icon icon={nodeKindIcon[s.kind] ?? Workflow} size={14}/></span>
                            <div className="timeline-body">
                                <div className="timeline-title">
                                    <strong>{s.title}</strong>
                                    <StatusBadge>{t.nodeKind[s.kind] ?? s.kind}</StatusBadge>
                                    <span className="muted small">{s.assignee}</span>
                                    {s.human ? <StatusBadge tone="human">사람</StatusBadge> : null}
                                </div>
                            </div>
                        </li>
                    ))}
                </ol>
            </Section>

            {r.proposal.notes ? <p className="muted small">설계 메모: {r.proposal.notes}</p> : null}
            <IssueList issues={r.issues}/>
            {view.mode === 'auto' && view.autoIssues.length ? (
                <div className="stack tight"><strong className="small">자동 시작할 수 없는 이유</strong><IssueList issues={view.autoIssues}/></div>
            ) : null}
            {!(d.steps ?? []).length && !(d.assignments ?? []).length ? <EmptyState compact title="제안된 내용이 없습니다"/> : null}
        </div>
    );
}
