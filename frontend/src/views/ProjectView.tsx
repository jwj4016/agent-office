import {
    Activity, Archive, ArchiveRestore, ArrowRight, FileText, GitBranch, Inbox as InboxIcon, LayoutDashboard, ListChecks,
    PauseCircle, Settings2, Users, Workflow,
} from 'lucide-react';
import {useEffect, useRef, useState} from 'react';
import type {Route} from '../App';
import {api} from '../api';
import {t} from '../i18n';
import {useLoad} from '../live';
import {blockedSteps, blockReason, currentSteps, progress, runStopped} from '../runInsight';
import type {Budget, Project, RunDetail} from '../types';
import {
    Banner, Button, ConfirmButton, EmptyState, ErrorBox, Field, Icon, Loading, PageHeader, ProgressBar, RunBadge, Section, StatusBadge, StepBadge,
    runTone, type TabItem, Tabs, useAction, useDetail, useToast,
} from '../ui';
import {ArtifactViewer} from './ArtifactViewer';
import {ExportButton} from './ExportButton';
import {FolderInput} from './FolderInput';
import {DeskDetail, Office, OfficeList, useOffice} from './Office';
import {Runs} from './Runs';
import {Team} from './Team';
import {Workflows} from './Workflows';

export type ProjectTab = 'status' | 'office' | 'runs' | 'workflows' | 'team' | 'settings';

type ProjectRoute = Extract<Route, { view: 'project' }>;

const tabs: TabItem<ProjectTab>[] = [
    {id: 'status', label: t.project.tabs.status, icon: LayoutDashboard},
    {id: 'office', label: t.project.tabs.office, icon: Users},
    {id: 'runs', label: t.project.tabs.runs, icon: Activity},
    {id: 'workflows', label: t.project.tabs.workflows, icon: Workflow},
    {id: 'team', label: t.project.tabs.team, icon: ListChecks},
    {id: 'settings', label: t.project.tabs.settings, icon: Settings2},
];

export function ProjectView({route, onRoute, onChanged}: {
    route: ProjectRoute; onRoute: (r: Route) => void; onChanged: () => void;
}) {
    const {data, error, reload} = useLoad(
        () => api.dashboard(true).then((all) => all.find((s) => s.project.id === route.projectId)?.project ?? null),
        [route.projectId], route.projectId);
    // Opening a run or a workflow from inside the content replaces the
    // button that had focus; land on the panel instead of the page body.
    // Tab changes from the tab list keep focus on the tab.
    const panel = useRef<HTMLDivElement>(null);
    const sub = `${route.tab}/${route.runId ?? ''}/${route.workflowId ?? ''}`;
    useEffect(() => {
        const id = window.requestAnimationFrame(() => {
            const a = document.activeElement;
            if (!a || a === document.body || !document.contains(a)) panel.current?.focus();
        });
        return () => window.cancelAnimationFrame(id);
    }, [sub]);
    if (error) return <div className="page"><ErrorBox error={error}/></div>;
    if (data === undefined) return <div className="page"><Loading/></div>;
    if (data === null) return <div className="page"><EmptyState title="서비스를 찾을 수 없습니다">삭제되었거나 다른 데이터 폴더의 서비스입니다.</EmptyState></div>;
    const p = data;
    const archived = p.status === 'archived';
    const go = (tab: ProjectTab) => onRoute({view: 'project', projectId: p.id, tab});
    // The workflow editor needs the whole width.
    const wide = route.tab === 'workflows' && !!route.workflowId;
    return (
        <div className={`page ${wide ? 'wide' : ''}`}>
            <PageHeader eyebrow={<><Icon icon={GitBranch} size={14}/>서비스</>} title={p.name}
                        badges={<>
                            <StatusBadge tone={p.mode === 'auto' ? 'info' : ''}>{p.mode === 'auto' ? t.project.modeAuto : t.project.modeReview}</StatusBadge>
                            {archived ? <StatusBadge icon={Archive}>보관됨</StatusBadge> : null}
                        </>}/>
            {archived ? <Banner tone="info" icon={Archive}>{t.project.archivedNote}</Banner> : null}
            <Tabs label="서비스 메뉴" items={tabs} value={route.tab} onChange={go}/>
            <div role="tabpanel" ref={panel} tabIndex={-1} aria-label={tabs.find((x) => x.id === route.tab)?.label} className="stack loose">
                {route.tab === 'status' ? <Status project={p} onRoute={onRoute}/> : null}
                {route.tab === 'office' ? <Office project={p} onRoute={onRoute}/> : null}
                {route.tab === 'runs' ? (
                    <Runs project={p} runId={route.runId}
                          onOpen={(runId) => onRoute({view: 'project', projectId: p.id, tab: 'runs', runId})}/>
                ) : null}
                {route.tab === 'workflows' ? (
                    <Workflows project={p} workflowId={route.workflowId}
                               onOpen={(workflowId) => onRoute({view: 'project', projectId: p.id, tab: 'workflows', workflowId})}
                               onStarted={(runId) => onRoute({view: 'project', projectId: p.id, tab: 'runs', runId})}/>
                ) : null}
                {route.tab === 'team' ? <Team project={p}/> : null}
                {route.tab === 'settings' ? <ProjectSettings project={p} onSaved={() => { reload(); onChanged(); }}/> : null}
            </div>
        </div>
    );
}

// Status is the service's first tab: what stopped, what is running, who
// is doing what and the latest results.
function Status({project, onRoute}: { project: Project; onRoute: (r: Route) => void }) {
    const runs = useLoad(() => api.runs(project.id), [project.id], project.id);
    const inbox = useLoad(() => api.inbox(), [], '*');
    const {office} = useOffice(project.id);
    const setDetail = useDetail();
    if (runs.error) return <ErrorBox error={runs.error}/>;
    if (!runs.data) return <Loading/>;
    const all = runs.data;
    const mine = (inbox.data ?? []).filter((i) => i.projectId === project.id);
    const stopped = all.filter((r) => runStopped(r.status));
    const live = all.filter((r) => r.status === 'running' || r.status === 'waiting' || r.status === 'paused');
    const openRun = (r: RunDetail) => onRoute({view: 'project', projectId: project.id, tab: 'runs', runId: r.id});
    const latest = all.find((r) => r.steps.some((s) => s.artifacts.length));
    const results = latest ? latest.steps.flatMap((s) => s.artifacts.filter((a) => a.validity === 'valid').map((a) => ({s, a}))).slice(-6).reverse() : [];

    if (all.length === 0 && office?.desks.length === 0) {
        return (
            <EmptyState icon={Workflow} title="아직 시작한 일이 없습니다"
                        actions={<>
                            <Button variant="primary" onClick={() => onRoute({view: 'design'})}>새 목표로 설계하기</Button>
                            <Button onClick={() => onRoute({view: 'project', projectId: project.id, tab: 'team'})}>담당자 배정</Button>
                        </>}>
                새 목표를 적으면 설계 AI가 담당자와 업무 흐름을 제안합니다. 직접 담당자를 배정하고 업무 흐름을 만들어도 됩니다.
            </EmptyState>
        );
    }

    return (
        <>
            {stopped.length || mine.length ? (
                <Section title="확인이 필요한 일">
                    {stopped.map((r) => {
                        const blocked = blockedSteps(r);
                        return (
                            <Banner key={r.id} tone="bad" title={`‘${r.title}’ 실행이 멈췄습니다`}
                                    actions={<Button variant="primary" size="sm" onClick={() => openRun(r)}>원인 보기<Icon icon={ArrowRight} size={14}/></Button>}>
                                {blocked.length
                                    ? blocked.map((s) => <div key={s.id}><strong>{s.title}</strong> — {blockReason(s).text}</div>)
                                    : '멈춘 업무를 찾지 못했습니다. 실행 화면에서 각 업무 상태를 확인하세요.'}
                            </Banner>
                        );
                    })}
                    {mine.length ? (
                        <Banner tone="warn" icon={InboxIcon} title={`내 할 일 ${mine.length}건이 기다리고 있습니다`}
                                actions={<Button size="sm" onClick={() => onRoute({view: 'inbox'})}>처리하기</Button>}>
                            {mine.slice(0, 3).map((i) => `${t.inboxKind[i.kind]}: ${i.stepTitle}`).join(' · ')}
                        </Banner>
                    ) : null}
                </Section>
            ) : <Banner tone="ok">막힌 실행이나 기다리는 일이 없습니다.</Banner>}

            {live.length ? (
                <Section title="진행 중인 실행" count={live.length}>
                    <div className="stack">
                        {live.map((r) => {
                            const p = progress(r);
                            const now = currentSteps(r);
                            return (
                                <button key={r.id} className="card clickable stack" onClick={() => openRun(r)}>
                                    <div className="row between">
                                        <strong>{r.title} <span className="muted small">v{r.versionNumber}</span></strong>
                                        <span className="row">{r.paused ? <StatusBadge tone="warn" icon={PauseCircle}>일시 정지</StatusBadge> : null}<RunBadge status={r.status}/></span>
                                    </div>
                                    <ProgressBar done={p.done} total={p.total} tone={runTone[r.status]}/>
                                    {now.length ? (
                                        <div className="row small">
                                            <span className="muted">지금:</span>
                                            {now.map((s) => <span key={s.id} className="row nowrap" style={{gap: 4}}>{s.title} <StepBadge status={s.status}/></span>)}
                                        </div>
                                    ) : null}
                                </button>
                            );
                        })}
                    </div>
                </Section>
            ) : null}

            {office && office.desks.length ? (
                <Section title="담당자 상태" count={office.desks.length}
                         actions={<Button size="sm" variant="ghost" onClick={() => onRoute({view: 'project', projectId: project.id, tab: 'office'})}>
                             사무실로 보기</Button>}>
                    <OfficeList office={office} onOpen={(d) =>
                        setDetail(<DeskDetail project={project} assignmentId={d.assignment.id} onRoute={onRoute}/>)}/>
                </Section>
            ) : null}

            {results.length ? (
                <Section title="최근 결과물" label="최근 결과물">
                    <div className="grid-cards">
                        {results.map(({s, a}) => (
                            <button key={a.id} className="card clickable row nowrap" onClick={() => setDetail(
                                <ArtifactViewer projectId={project.id} artifactId={a.id} onClose={() => setDetail(null)}/>)}>
                                <Icon icon={FileText} size={20}/>
                                <span className="stack tight" style={{minWidth: 0}}>
                                    <strong className="truncate">{a.outputKey} <span className="muted small">v{a.version}</span></strong>
                                    <span className="muted xsmall truncate">{s.title} · {t.outputType[a.type] ?? a.type}</span>
                                </span>
                            </button>
                        ))}
                    </div>
                </Section>
            ) : null}

            {!live.length && !stopped.length && all.length ? (
                <p className="muted small">최근 실행: {all[0].title} — <RunBadge status={all[0].status}/>{' '}
                    <Button size="sm" variant="ghost" onClick={() => openRun(all[0])}>열기</Button></p>
            ) : null}
        </>
    );
}

const budgetOf = (p: Project): Budget => (p.budget && typeof p.budget === 'object' ? p.budget as Budget : {});
const optNum = (v: string) => (v.trim() === '' ? undefined : Number(v));

// ProjectSettings holds everything about a service that is set rather
// than run: identity, how it runs, budget, export and archiving.
function ProjectSettings({project, onSaved}: { project: Project; onSaved: () => void }) {
    const [form, setForm] = useState(project);
    const [budget, setBudget] = useState<Budget>(budgetOf(project));
    useEffect(() => { setForm(project); setBudget(budgetOf(project)); }, [project]);
    const usage = useLoad(() => api.projectUsage(project.id), [project.id], project.id);
    const action = useAction();
    const toast = useToast();
    const archived = project.status === 'archived';
    const saved = budgetOf(project);
    const dirty = form.name !== project.name || form.goal !== project.goal || form.instructions !== project.instructions || form.mode !== project.mode
        || form.workspacePath !== project.workspacePath || budget.maxTokens !== saved.maxTokens || budget.maxCostUsd !== saved.maxCostUsd;
    const save = () => action.run(async () => {
        await api.updateProject({id: project.id, name: form.name, goal: form.goal, instructions: form.instructions, mode: form.mode, budget,
            workspacePath: form.workspacePath});
        onSaved();
        usage.reload();
        toast('서비스 설정을 저장했습니다');
    });
    const u = usage.data;
    return (
        <div className="stack loose" style={{maxWidth: 720}}>
            <form className="stack loose" aria-label="서비스 설정" onSubmit={(e) => { e.preventDefault(); save(); }}>
                <Section title="기본 정보">
                    <div className="card stack">
                        <Field label={t.project.name}>
                            <input value={form.name} disabled={archived} onChange={(e) => setForm({...form, name: e.target.value})}/></Field>
                        <Field label={t.project.goal} hint="담당자 모두가 참고하는 이 서비스의 목적입니다.">
                            <textarea value={form.goal} disabled={archived} onChange={(e) => setForm({...form, goal: e.target.value})}/></Field>
                        <Field label={t.project.instructions} optional hint="이 서비스의 모든 담당자가 따를 규칙입니다. 회사 지침 다음에 적용됩니다.">
                            <textarea value={form.instructions} disabled={archived} rows={6}
                                      onChange={(e) => setForm({...form, instructions: e.target.value})}/></Field>
                        <Field label="작업 폴더" optional hint="AI가 코드를 읽고 고치는 폴더입니다. 비우면 앱 데이터 폴더 안에서 작업합니다. 바꾼 뒤 ‘저장’을 누르세요.">
                            <FolderInput value={form.workspacePath} disabled={archived} title="작업 폴더 선택"
                                         onChange={(workspacePath) => setForm({...form, workspacePath})}/></Field>
                    </div>
                </Section>

                <Section title={t.project.mode}>
                    <fieldset className="choice-group" disabled={archived}>
                        <legend className="sr-only">{t.project.mode}</legend>
                        <label className="choice">
                            <input type="radio" name="mode" checked={form.mode === 'review'} onChange={() => setForm({...form, mode: 'review'})}/>
                            <span><span className="choice-title">{t.project.modeReview} (기본)</span>
                                <span className="choice-desc" style={{display: 'block'}}>설계안과 업무 흐름을 내가 확인한 뒤 실행합니다.</span></span>
                        </label>
                        <label className="choice">
                            <input type="radio" name="mode" checked={form.mode === 'auto'} onChange={() => setForm({...form, mode: 'auto'})}/>
                            <span><span className="choice-title">{t.project.modeAuto}</span>
                                <span className="choice-desc" style={{display: 'block'}}>허용한 AI 연결과 예산 안에서 바로 적용·시작합니다. 사람 업무와 승인은 그래도 기다립니다.</span></span>
                        </label>
                    </fieldset>
                </Section>

                <Section title="AI 사용량과 예산">
                    <div className="card stack">
                        {u ? (
                            <dl className="kv">
                                <dt>토큰</dt><dd className="num">입력 {u.inputTokens.toLocaleString()} · 출력 {u.outputTokens.toLocaleString()}</dd>
                                <dt>AI 업무</dt><dd className="num">{u.attempts.toLocaleString()}건</dd>
                                <dt>비용</dt><dd className="num">${u.costUsd.toFixed(4)}
                                    {u.unknownCostAttempts > 0 ? <span className="muted"> · 비용을 알려 주지 않은 업무 {u.unknownCostAttempts}건은 빠져 있습니다</span> : null}</dd>
                            </dl>
                        ) : null}
                        {u?.holdReason ? <Banner tone="warn" title="새 AI 업무가 멈춰 있습니다">{u.holdReason}</Banner> : null}
                        <div className="grid-2">
                            <Field label="토큰 예산" optional hint="비우면 제한 없음">
                                <input inputMode="numeric" disabled={archived} value={budget.maxTokens ?? ''}
                                       onChange={(e) => setBudget({...budget, maxTokens: optNum(e.target.value)})}/></Field>
                            <Field label="비용 예산 (USD)" optional hint="비용을 알려 주는 연결만 계산됩니다">
                                <input inputMode="decimal" disabled={archived} value={budget.maxCostUsd ?? ''}
                                       onChange={(e) => setBudget({...budget, maxCostUsd: optNum(e.target.value)})}/></Field>
                        </div>
                        <p className="muted small">새 AI 업무를 시작하기 전에 확인합니다. 진행 중인 요청과 청구 지연 때문에 정확한 상한은 아닙니다.
                            비용을 알려 주지 않는 연결(예: Codex)은 토큰 예산으로 관리하세요.</p>
                    </div>
                </Section>

                <ErrorBox error={action.error}/>
                <div className="form-actions">
                    <Button variant="primary" type="submit" busy={action.busy} disabled={archived || !dirty}>{t.project.save}</Button>
                    {dirty ? <span className="save-state dirty">저장하지 않은 변경이 있습니다</span> : null}
                </div>
            </form>

            {form.mode === 'auto' ? <AutoScope project={project}/> : null}

            <Section title="내보내기">
                <div className="card stack">
                    <p className="muted small">역할·담당자·업무 흐름·실행 기록과 결과 파일을 ZIP 하나로 저장합니다.
                        API 키, 로그인 정보, 공급자 세션은 넣지 않고 내용에 섞인 키 값은 가립니다.</p>
                    <ExportButton projectId={project.id}/>
                </div>
            </Section>

            <Section title="보관">
                <div className="card stack" style={{borderColor: archived ? undefined : 'var(--danger)'}}>
                    <p className="small">{archived
                        ? '보관을 해제하면 다시 수정하고 실행할 수 있습니다.'
                        : '보관하면 목록에서 숨기고 수정할 수 없게 됩니다. 기록은 남고 언제든 해제할 수 있습니다.'}</p>
                    <div>
                        {archived
                            ? <Button icon={ArchiveRestore} busy={action.busy}
                                      onClick={() => action.run(async () => { await api.setArchived(project.id, false); onSaved(); toast('보관을 해제했습니다'); })}>
                                {t.project.restore}</Button>
                            : <ConfirmButton label="서비스 보관" confirmLabel="보관하기" className="btn danger" icon={Archive}
                                             onConfirm={() => action.run(async () => { await api.setArchived(project.id, true); onSaved(); toast('보관했습니다'); })}/>}
                    </div>
                </div>
            </Section>
        </div>
    );
}

// AutoScope sets which connections auto mode may use for this service.
function AutoScope({project}: { project: Project }) {
    const conns = useLoad(() => api.connections(), []);
    const current = (project.autoPolicy && typeof project.autoPolicy === 'object'
        ? (project.autoPolicy as { allowedConnectionIds?: string[] }).allowedConnectionIds : undefined) ?? [];
    const [allowed, setAllowed] = useState<string[]>(current);
    const action = useAction();
    const toast = useToast();
    const usable = (conns.data ?? []).filter((c) => c.usable);
    return (
        <Section title="자동 실행 허용 범위" label="자동 실행 허용 범위">
            <div className="card stack">
                <p className="muted small">자동 구성·실행은 여기서 허용한 연결과 위의 예산 안에서만 시작합니다.</p>
                {usable.map((c) => (
                    <label key={c.id} className="check">
                        <input type="checkbox" checked={allowed.includes(c.id)}
                               onChange={(e) => setAllowed(e.target.checked ? [...allowed, c.id] : allowed.filter((x) => x !== c.id))}/>{c.name}
                    </label>
                ))}
                {usable.length === 0 ? <p className="muted small">사용 가능한 연결이 없습니다. 설정의 AI 연결에서 실제 호출 시험을 통과시키세요.</p> : null}
                <ErrorBox error={action.error}/>
                <div><Button busy={action.busy} onClick={() => action.run(async () => { await api.setAutoPolicy(project.id, allowed); toast('허용 범위를 저장했습니다'); })}>
                    허용 범위 저장</Button></div>
            </div>
        </Section>
    );
}
