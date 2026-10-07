import {useEffect, useState} from 'react';
import {api} from '../api';
import {t} from '../i18n';
import {useLoad} from '../live';
import type {DesignView} from '../types';
import {ActorBadge, ConfirmButton, ErrorBox, IssueList, shortTime, useAction} from '../ui';

const statusLabel: Record<DesignView['status'], [string, string]> = {
    drafting: ['설계 중', 'info'], ready: ['검토 대기', 'warn'], failed: ['실패', 'bad'], applied: ['적용됨', 'ok'], discarded: ['버림', ''],
};

export function Design({onOpenWorkflow, onOpenRun}: {
    onOpenWorkflow: (projectId: string, workflowId: string) => void; onOpenRun: (projectId: string, runId: string) => void;
}) {
    const list = useLoad(() => api.designs(), [], '*');
    const [selected, setSelected] = useState<string | null>(null);
    const designs = list.data ?? [];
    return (
        <div className="stack">
            <h1>{t.nav.design}</h1>
            <p className="muted">목표를 말하면 설계 AI가 필요한 역할·담당자·업무 흐름을 제안합니다. 제안은 검토한 뒤 적용하며, 도구 권한이나 키를 새로 만들지 않습니다.</p>
            <RequestForm onCreated={(id) => { setSelected(id); list.reload(); }}/>
            <ErrorBox error={list.error}/>
            {designs.length ? (
                <section className="stack" aria-label="설계 요청">
                    <h2>요청 기록</h2>
                    {designs.map((d) => (
                        <button key={d.id} className={`card clickable ${selected === d.id ? 'selected' : ''}`} onClick={() => setSelected(d.id)}>
                            <div className="row between">
                                <span>{d.goal.length > 80 ? d.goal.slice(0, 80) + '…' : d.goal}</span>
                                <span className={`badge ${statusLabel[d.status][1]}`}>{statusLabel[d.status][0]}</span>
                            </div>
                            <div className="muted small">{d.mode === 'auto' ? '자동 구성·실행' : '검토 후 실행'} · {shortTime(d.createdAt)}</div>
                        </button>
                    ))}
                </section>
            ) : null}
            {selected ? <DesignDetail id={selected} onOpenWorkflow={onOpenWorkflow} onOpenRun={onOpenRun} onChanged={list.reload}/> : null}
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

    const submit = () => action.run(async () => {
        const d = await api.startDesign({
            goal, projectId: target, projectName, humanTasks: splitTasks(tasks), mode, connectionId, model,
            budget: {maxTokens: optNum(maxTokens)}, allowedConnectionIds: allowed,
        });
        setGoal('');
        onCreated(d.id);
    });

    return (
        <form className="card stack" aria-label="목표 요청" onSubmit={(e) => e.preventDefault()}>
            <label className="field"><span>목표</span>
                <textarea value={goal} onChange={(e) => setGoal(e.target.value)} placeholder="예: 작은 웹 게임을 출시하고 싶어. 코드 리뷰는 내가 할게."/></label>
            <label className="field"><span>대상 서비스</span>
                <select value={target} onChange={(e) => setTarget(e.target.value)}>
                    <option value="">새 서비스 만들기</option>
                    {(projects.data ?? []).map((s) => <option key={s.project.id} value={s.project.id}>{s.project.name} (업무 흐름 추가)</option>)}
                </select></label>
            {target === '' ? (
                <label className="field"><span>새 서비스 이름 (비우면 AI가 제안)</span>
                    <input value={projectName} onChange={(e) => setProjectName(e.target.value)}/></label>
            ) : null}
            <label className="field"><span>내가 직접 맡을 업무 (쉼표나 줄로 구분)</span>
                <input value={tasks} onChange={(e) => setTasks(e.target.value)} placeholder="예: 코드 리뷰, 출시 승인"/></label>
            <p className="muted small" style={{marginTop: -4}}>적은 업무가 설계안에서 실제로 나에게 배정되지 않으면 적용할 수 없습니다.</p>
            <fieldset className="row" style={{border: 'none', padding: 0}}>
                <legend className="small muted">실행 방식</legend>
                <label className="check"><input type="radio" checked={mode === 'review'} onChange={() => setMode('review')}/> 검토 후 실행 (기본)</label>
                <label className="check"><input type="radio" checked={mode === 'auto'} onChange={() => setMode('auto')}/> 자동 구성·실행</label>
            </fieldset>
            {mode === 'auto' ? (
                <div className="card stack" style={{background: 'var(--surface-2)'}}>
                    <p className="small" style={{margin: 0}}>자동 모드는 미리 허용한 범위 안에서만 적용·시작합니다. 사람 업무와 승인은 그래도 기다립니다.</p>
                    {target === '' ? (
                        <>
                            <label className="field"><span>토큰 예산 (필수)</span>
                                <input inputMode="numeric" value={maxTokens} onChange={(e) => setMaxTokens(e.target.value)}/></label>
                            <fieldset className="stack" style={{border: 'none', padding: 0}}>
                                <legend className="small muted">자동 실행에 쓸 수 있는 연결</legend>
                                {usable.map((c) => (
                                    <label key={c.id} className="check">
                                        <input type="checkbox" checked={allowed.includes(c.id)}
                                               onChange={(e) => setAllowed(e.target.checked ? [...allowed, c.id] : allowed.filter((x) => x !== c.id))}/>{c.name}
                                    </label>
                                ))}
                            </fieldset>
                        </>
                    ) : <p className="muted small" style={{margin: 0}}>기존 서비스는 그 서비스의 개요에서 정한 예산과 허용 연결을 사용합니다.</p>}
                </div>
            ) : null}
            <div className="row">
                <label className="field"><span>설계에 쓸 AI 연결</span>
                    <select value={connectionId} onChange={(e) => setConnectionId(e.target.value)}>
                        {usable.length === 0 ? <option value="">사용 가능한 연결 없음 (설정에서 확인)</option> : null}
                        {usable.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}
                    </select></label>
                <label className="field"><span>모델 (선택)</span><input value={model} onChange={(e) => setModel(e.target.value)}/></label>
            </div>
            <ErrorBox error={action.error}/>
            <div className="row">
                <ConfirmButton label="설계안 만들기" confirmLabel="AI 사용량을 써서 설계" className="btn primary"
                               disabled={action.busy || !goal.trim() || !connectionId} onConfirm={submit}/>
            </div>
        </form>
    );
}

function DesignDetail({id, onOpenWorkflow, onOpenRun, onChanged}: {
    id: string; onOpenWorkflow: (projectId: string, workflowId: string) => void;
    onOpenRun: (projectId: string, runId: string) => void; onChanged: () => void;
}) {
    const {data, error, reload} = useLoad(() => api.design(id), [id]);
    const action = useAction();
    const [issues, setIssues] = useState<DesignView['autoIssues']>([]);
    // Drafting runs in the background: check again every 2 seconds.
    useEffect(() => {
        if (data?.status !== 'drafting') return;
        const t = window.setInterval(reload, 2000);
        return () => window.clearInterval(t);
    }, [data?.status, reload]);
    if (error) return <ErrorBox error={error}/>;
    if (!data) return <p className="muted">{t.common.loading}</p>;
    const r = data.result;
    const apply = (auto: boolean) => action.run(async () => {
        const res = await api.applyDesign(id, auto);
        setIssues(res.issues);
        onChanged();
        if (res.applied) {
            if (res.runId) onOpenRun(res.applied.projectId, res.runId);
            else if (!auto) onOpenWorkflow(res.applied.projectId, res.applied.workflowId);
        }
        reload();
    });
    return (
        <section className="card stack" aria-label="설계안">
            <div className="row between">
                <h2 style={{margin: 0}}>설계안</h2>
                <span className={`badge ${statusLabel[data.status][1]}`}>{statusLabel[data.status][0]}</span>
            </div>
            <p style={{whiteSpace: 'pre-wrap'}}>{data.goal}</p>
            {data.request.humanTasks?.length ? <p className="small">내가 맡을 업무: {data.request.humanTasks.join(', ')}</p> : null}
            {data.status === 'drafting' ? <p className="muted">설계 AI가 작업 중입니다…</p> : null}
            {data.status === 'failed' ? <div className="alert" role="alert">{data.error}</div> : null}
            {data.status === 'applied' && data.applied?.projectId && data.applied.workflowId ? (
                <button className="btn" onClick={() => onOpenWorkflow(data.applied!.projectId!, data.applied!.workflowId!)}>적용된 업무 흐름 열기</button>
            ) : null}
            {r && data.status === 'ready' ? <ProposalView view={data}/> : null}
            <IssueList issues={issues}/>
            <ErrorBox error={action.error}/>
            {data.status === 'ready' ? (
                <div className="row">
                    <button className="btn primary" disabled={action.busy || !r?.canApply} onClick={() => apply(false)}>적용하고 편집하기</button>
                    {data.mode === 'auto' ? (
                        <button className="btn primary" disabled={action.busy || !r?.canApply || data.autoIssues.length > 0}
                                onClick={() => apply(true)}>적용하고 자동 시작</button>
                    ) : null}
                    <ConfirmButton label="버리기" confirmLabel="버리기" className="btn danger"
                                   onConfirm={() => action.run(async () => { await api.discardDesign(id); onChanged(); reload(); })}/>
                </div>
            ) : null}
        </section>
    );
}

function ProposalView({view}: { view: DesignView }) {
    const r = view.result!;
    const d = r.diff;
    return (
        <div className="stack">
            <div className="small">
                <strong>{r.proposal.project.name || '(이름 없음)'}</strong> · {r.proposal.project.goal}
                {view.projectId ? <span className="badge" style={{marginLeft: 6}}>기존 서비스에 추가</span> : <span className="badge info" style={{marginLeft: 6}}>새 서비스</span>}
            </div>
            <h3>역할</h3>
            <div className="row small">
                {(d.reuseRoles ?? []).map((x) => <span key={x.ref} className="badge">{x.name} · 재사용</span>)}
                {(d.newRoles ?? []).map((x) => <span key={x.ref} className="badge info">{x.name} · 새 역할</span>)}
            </div>
            {(d.roleChanges ?? []).length ? (
                <div className="card stack" style={{background: 'var(--surface-2)'}}>
                    <strong className="small">기존 역할 수정 제안 (자동으로 적용되지 않음)</strong>
                    {(d.roleChanges ?? []).map((c) => (
                        <div key={c.roleId} className="small">{c.roleName}: {c.reason}<div className="muted">제안 지침: {c.instructions}</div></div>
                    ))}
                </div>
            ) : null}
            <h3>담당자</h3>
            <table className="list small">
                <thead><tr><th>이름</th><th>역할</th><th>종류</th><th>연결</th></tr></thead>
                <tbody>
                {(d.assignments ?? []).map((a) => (
                    <tr key={a.ref}>
                        <td>{a.displayName}</td><td>{a.roleName}</td><td><ActorBadge kind={a.actorKind}/></td>
                        <td>{a.actorKind === 'human' ? '나' : a.connected ? a.connectionName : <span className="badge bad">연결 필요</span>}</td>
                    </tr>
                ))}
                </tbody>
            </table>
            <h3>업무 흐름: {r.proposal.workflow.title}</h3>
            <table className="list small">
                <thead><tr><th>업무</th><th>종류</th><th>담당</th></tr></thead>
                <tbody>
                {(d.steps ?? []).map((s) => (
                    <tr key={s.id}>
                        <td>{s.title}</td><td>{t.nodeKind[s.kind] ?? s.kind}</td>
                        <td>{s.assignee}{s.human ? <span className="badge human" style={{marginLeft: 6}}>사람</span> : null}</td>
                    </tr>
                ))}
                </tbody>
            </table>
            {(d.missing ?? []).length ? (
                <div className="stack small"><strong>부족한 항목</strong>
                    {(d.missing ?? []).map((m, i) => <span key={i}>· {m.detail}</span>)}</div>
            ) : null}
            {r.proposal.notes ? <p className="muted small">설계 메모: {r.proposal.notes}</p> : null}
            <IssueList issues={r.issues}/>
            {view.mode === 'auto' && view.autoIssues.length ? (
                <div className="stack"><strong className="small">자동 시작할 수 없는 이유</strong><IssueList issues={view.autoIssues}/></div>
            ) : null}
        </div>
    );
}
