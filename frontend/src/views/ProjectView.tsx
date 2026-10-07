import {useEffect, useState} from 'react';
import type {Route} from '../App';
import {api} from '../api';
import {t} from '../i18n';
import {useLoad} from '../live';
import type {Budget, Project} from '../types';
import {ConfirmButton, ErrorBox, useAction} from '../ui';
import {Runs} from './Runs';
import {Team} from './Team';
import {Workflows} from './Workflows';

export type ProjectTab = 'overview' | 'team' | 'workflows' | 'runs';

type ProjectRoute = Extract<Route, { view: 'project' }>;

export function ProjectView({route, onRoute, onChanged}: {
    route: ProjectRoute; onRoute: (r: Route) => void; onChanged: () => void;
}) {
    const {data, error, reload} = useLoad(
        () => api.dashboard(true).then((all) => all.find((s) => s.project.id === route.projectId)?.project ?? null),
        [route.projectId], route.projectId);
    if (error) return <ErrorBox error={error}/>;
    if (data === undefined) return <p className="muted">{t.common.loading}</p>;
    if (data === null) return <p className="muted">서비스를 찾을 수 없습니다.</p>;
    const p = data;
    const archived = p.status === 'archived';
    const tab = (id: ProjectTab, label: string) => (
        <button role="tab" className="tab" aria-selected={route.tab === id}
                onClick={() => onRoute({view: 'project', projectId: p.id, tab: id})}>{label}</button>
    );
    return (
        <div>
            <div className="row between">
                <h1>{p.name} {archived ? <span className="badge">보관됨</span> : null}</h1>
                <span className="badge">{p.mode === 'auto' ? t.project.modeAuto : t.project.modeReview}</span>
            </div>
            {archived ? <p className="notice">{t.project.archivedNote}</p> : null}
            <div role="tablist" className="tabs">
                {tab('overview', t.project.tabs.overview)}
                {tab('team', t.project.tabs.team)}
                {tab('workflows', t.project.tabs.workflows)}
                {tab('runs', t.project.tabs.runs)}
            </div>
            {route.tab === 'overview' ? <Overview project={p} onSaved={() => { reload(); onChanged(); }}/> : null}
            {route.tab === 'team' ? <Team project={p}/> : null}
            {route.tab === 'workflows' ? (
                <Workflows project={p} workflowId={route.workflowId}
                           onOpen={(workflowId) => onRoute({view: 'project', projectId: p.id, tab: 'workflows', workflowId})}
                           onStarted={(runId) => onRoute({view: 'project', projectId: p.id, tab: 'runs', runId})}/>
            ) : null}
            {route.tab === 'runs' ? (
                <Runs project={p} runId={route.runId}
                      onOpen={(runId) => onRoute({view: 'project', projectId: p.id, tab: 'runs', runId})}/>
            ) : null}
        </div>
    );
}

const budgetOf = (p: Project): Budget => (p.budget && typeof p.budget === 'object' ? p.budget as Budget : {});
const optNum = (v: string) => (v.trim() === '' ? undefined : Number(v));

function Overview({project, onSaved}: { project: Project; onSaved: () => void }) {
    const [form, setForm] = useState(project);
    const [budget, setBudget] = useState<Budget>(budgetOf(project));
    useEffect(() => { setForm(project); setBudget(budgetOf(project)); }, [project]);
    const usage = useLoad(() => api.projectUsage(project.id), [project.id], project.id);
    const action = useAction();
    const archived = project.status === 'archived';
    const saved = budgetOf(project);
    const dirty = form.name !== project.name || form.goal !== project.goal || form.instructions !== project.instructions || form.mode !== project.mode
        || form.workspacePath !== project.workspacePath || budget.maxTokens !== saved.maxTokens || budget.maxCostUsd !== saved.maxCostUsd;
    const save = () => action.run(async () => {
        await api.updateProject({id: project.id, name: form.name, goal: form.goal, instructions: form.instructions, mode: form.mode, budget,
            workspacePath: form.workspacePath});
        onSaved();
        usage.reload();
    });
    const u = usage.data;
    return (
        <div className="stack" style={{maxWidth: 640}}>
            <label className="field"><span>{t.project.name}</span>
                <input value={form.name} disabled={archived} onChange={(e) => setForm({...form, name: e.target.value})}/></label>
            <label className="field"><span>{t.project.goal}</span>
                <textarea value={form.goal} disabled={archived} onChange={(e) => setForm({...form, goal: e.target.value})}/></label>
            <label className="field"><span>{t.project.instructions}</span>
                <textarea value={form.instructions} disabled={archived} placeholder="이 서비스의 모든 담당자가 따를 지침"
                          onChange={(e) => setForm({...form, instructions: e.target.value})}/></label>
            <label className="field"><span>작업 폴더 (AI가 코드를 읽고 고치는 폴더, 절대 경로 · 비우면 실행마다 임시 폴더)</span>
                <input value={form.workspacePath} disabled={archived} placeholder="/Users/me/dev/my-game"
                       onChange={(e) => setForm({...form, workspacePath: e.target.value})}/></label>
            <label className="field"><span>{t.project.mode}</span>
                <select value={form.mode} disabled={archived} onChange={(e) => setForm({...form, mode: e.target.value as Project['mode']})}>
                    <option value="review">{t.project.modeReview} (기본)</option>
                    <option value="auto">{t.project.modeAuto}</option>
                </select>
            </label>
            <section className="card stack" aria-label="사용량과 예산">
                <h2>AI 사용량과 예산</h2>
                {u ? (
                    <div className="small">
                        <div>토큰: 입력 {u.inputTokens.toLocaleString()} · 출력 {u.outputTokens.toLocaleString()} (AI 업무 {u.attempts}건)</div>
                        <div>보고된 비용: ${u.costUsd.toFixed(4)}
                            {u.unknownCostAttempts > 0 ? <span className="badge warn" style={{marginLeft: 6}}>비용 미보고 {u.unknownCostAttempts}건 (합계에 포함되지 않음)</span> : null}</div>
                    </div>
                ) : null}
                {u?.holdReason ? <div className="alert" role="alert">{u.holdReason}</div> : null}
                <div className="row">
                    <label className="field"><span>토큰 예산 (비우면 제한 없음)</span>
                        <input inputMode="numeric" disabled={archived} value={budget.maxTokens ?? ''}
                               onChange={(e) => setBudget({...budget, maxTokens: optNum(e.target.value)})}/></label>
                    <label className="field"><span>비용 예산 USD (보고된 비용 기준)</span>
                        <input inputMode="decimal" disabled={archived} value={budget.maxCostUsd ?? ''}
                               onChange={(e) => setBudget({...budget, maxCostUsd: optNum(e.target.value)})}/></label>
                </div>
                <p className="muted small">새 AI 업무를 시작하기 전에 확인합니다. 이미 진행 중인 요청과 공급자 청구 지연 때문에 실제 청구액의 절대 상한은 아닙니다.
                    비용을 보고하지 않는 연결(예: Codex)은 토큰 예산으로 관리하세요.</p>
            </section>
            <ErrorBox error={action.error}/>
            <div className="row">
                <button className="btn primary" disabled={archived || !dirty || action.busy} onClick={save}>{t.project.save}</button>
                <span className="spacer"/>
                {archived
                    ? <button className="btn" onClick={() => action.run(async () => { await api.setArchived(project.id, false); onSaved(); })}>{t.project.restore}</button>
                    : <ConfirmButton label={t.project.archive} confirmLabel="보관하기" className="btn danger"
                                     onConfirm={() => action.run(async () => { await api.setArchived(project.id, true); onSaved(); })}/>}
            </div>
        </div>
    );
}
