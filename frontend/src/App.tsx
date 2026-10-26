import {Building2, CircleHelp, Home, Inbox as InboxIcon, Network, Plus, Settings as SettingsIcon} from 'lucide-react';
import {type ReactNode, useEffect, useRef, useState} from 'react';
import {api} from './api';
import {t} from './i18n';
import {LiveProvider, useLoad} from './live';
import {markReady, startLagProbe} from './perf';
import type {ProjectSummary} from './types';
import {applyMotion, applyTheme, Banner, DetailPane, DetailProvider, Icon, REDUCED_MOTION, THEME, ToastProvider, useToast} from './ui';
import {Dashboard} from './views/Dashboard';
import {Design} from './views/Design';
import {Inbox} from './views/Inbox';
import {pendingCount} from './views/Judgments';
import {Organization} from './views/Organization';
import {ProjectView, type ProjectTab} from './views/ProjectView';
import {Settings} from './views/Settings';
import {Tutorial, TUTORIAL_SEEN} from './views/Tutorial';

export type Route =
    | { view: 'dashboard' }
    | { view: 'inbox' }
    | { view: 'design' }
    | { view: 'org' }
    | { view: 'settings' }
    | { view: 'project'; projectId: string; tab: ProjectTab; workflowId?: string; runId?: string; workItemId?: string };

const LAST_PROJECT = 'ui.lastProjectId';

export default function App() {
    return (
        <LiveProvider>
            <ToastProvider>
                <Shell/>
            </ToastProvider>
        </LiveProvider>
    );
}

function Shell() {
    const status = useLoad(() => api.systemStatus(), []);
    if (status.data?.error) {
        // e.g. another Agent Office already uses this data folder.
        return (
            <main className="page narrow" style={{paddingTop: 80}}>
                <div className="brand"><span className="brand-mark"><Icon icon={Building2} size={18}/></span>{t.app.name}</div>
                <h1>{t.app.name}을 시작할 수 없습니다</h1>
                <Banner tone="bad">{status.data.error}</Banner>
                <p className="muted">이미 열려 있는 Agent Office 창을 사용하거나, 그 창을 닫은 뒤 다시 실행하세요.</p>
            </main>
        );
    }
    return <Workspace perf={status.data ? status.data.perf === true : undefined}/>;
}

// serviceState is the one-word state a service shows in the sidebar.
export function serviceState(s: ProjectSummary): { tone: 'bad' | 'warn' | 'info' | ''; label: string } {
    if (s.failedRuns > 0) return {tone: 'bad', label: '막힘'};
    if (s.inbox > 0 || s.waitingRuns > 0) return {tone: 'warn', label: '대기'};
    if (s.pausedRuns > 0) return {tone: 'warn', label: '정지'};
    if (s.activeRuns > 0) return {tone: 'info', label: '진행 중'};
    return {tone: '', label: ''};
}

// routeKey changes when the screen changes. Tabs and items inside a
// service are not new screens: moving focus there would pull it out of
// the tab list while the person is using the arrow keys.
const routeKey = (r: Route) => (r.view === 'project' ? `project/${r.projectId}` : r.view);

function Workspace({perf}: { perf: boolean | undefined }) {
    const [route, setRoute] = useState<Route>({view: 'dashboard'});
    const [detail, setDetail] = useState<ReactNode | null>(null);
    const [tutorialOpen, setTutorialOpen] = useState(false);
    const tutorialTouched = useRef(false);
    const toast = useToast();
    const dash = useLoad(() => api.dashboard(false), [], '*');
    const inbox = useLoad(() => api.inbox(), [], '*');
    // Department questions, proposals, collaboration and automatic-start
    // reviews also wait for the person; they count as their to-dos.
    const decisions = useLoad(() => api.pendingDecisions(), [], '*');

    // Measurement runs only (perf.go): first screen and screen lag.
    const readyMarked = useRef(false);
    useEffect(() => {
        if (perf && dash.data !== undefined && !readyMarked.current) {
            readyMarked.current = true;
            markReady();
        }
    }, [perf, dash.data]);
    useEffect(() => (perf ? startLagProbe() : undefined), [perf]);

    // Display settings, and the last selected service (switching never
    // affects runs).
    useEffect(() => {
        if (perf === undefined) return;
        let cancelled = false;
        api.settings().then((s) => {
            if (cancelled) return;
            applyMotion(s[REDUCED_MOTION] === 'true');
            applyTheme(s[THEME]);
            if (!perf && !tutorialTouched.current && s[TUTORIAL_SEEN] !== 'true') setTutorialOpen(true);
            const id = s[LAST_PROJECT];
            if (id) setRoute((r) => (r.view === 'dashboard' ? {view: 'project', projectId: id, tab: 'status'} : r));
        }).catch(() => {});
        return () => { cancelled = true; };
    }, [perf]);

    const closeTutorial = () => {
        tutorialTouched.current = true;
        setTutorialOpen(false);
        api.setSetting(TUTORIAL_SEEN, 'true').catch(() => {
            toast('튜토리얼 확인 상태를 저장하지 못했습니다. 다음 실행 때 다시 열릴 수 있습니다.', 'bad');
        });
    };

    // After a route change, move focus to the new screen's title so
    // keyboard and screen-reader users start at the top of it.
    const key = routeKey(route);
    const firstRoute = useRef(true);
    useEffect(() => {
        if (firstRoute.current) { firstRoute.current = false; return; }
        const id = window.requestAnimationFrame(() => document.getElementById('page-title')?.focus());
        return () => window.cancelAnimationFrame(id);
    }, [key]);

    const go = (r: Route) => {
        setDetail(null);
        setRoute(r);
        if (r.view === 'project') api.setSetting(LAST_PROJECT, r.projectId).catch(() => {});
    };

    const projects = (dash.data ?? []).map((s) => ({...s, inbox: s.inbox + pendingCount(decisions.data, s.project.id)}));
    const current = route.view === 'project' ? dash.data?.find((p) => p.project.id === route.projectId) : undefined;
    // A remembered project that no longer exists (or is archived) falls back.
    useEffect(() => {
        if (route.view === 'project' && dash.data && !current) {
            api.dashboard(true).then((all) => {
                if (!all.some((p) => p.project.id === route.projectId)) setRoute({view: 'dashboard'});
            });
        }
    }, [route, dash.data, current]);

    const decisionCount = pendingCount(decisions.data);
    const inboxCount = (inbox.data?.length ?? 0) + decisionCount;
    const navButton = (label: string, icon: typeof Home, r: Route, active: boolean, extra?: ReactNode) => (
        <button className="nav-item" aria-current={active ? 'page' : undefined} onClick={() => go(r)} title={label}>
            <Icon icon={icon} size={18}/><span className="nav-label">{label}</span>{extra}
        </button>
    );

    let main: ReactNode;
    switch (route.view) {
        case 'dashboard':
            main = <Dashboard summaries={projects} inbox={inbox.data ?? []} decisions={decisionCount} loaded={dash.data !== undefined} onRoute={go} onCreated={dash.reload}/>;
            break;
        case 'inbox':
            main = <Inbox onOpenRun={(projectId, runId) => go({view: 'project', projectId, tab: 'runs', runId})}
                          onOpenWork={(projectId, workItemId) => go({view: 'project', projectId, tab: 'work', workItemId})}/>;
            break;
        case 'design':
            main = <Design onOpenWorkflow={(projectId, workflowId) => go({view: 'project', projectId, tab: 'workflows', workflowId})}
                           onOpenRun={(projectId, runId) => go({view: 'project', projectId, tab: 'runs', runId})}/>;
            break;
        case 'org':
            main = <Organization/>;
            break;
        case 'settings':
            main = <Settings/>;
            break;
        case 'project':
            main = <ProjectView key={route.projectId} route={route} onRoute={go} onChanged={dash.reload}/>;
            break;
    }

    return (
        <DetailProvider value={setDetail}>
            <a className="skip-link" href="#main">본문으로 건너뛰기</a>
            <div className={`shell ${detail ? '' : 'no-detail'}`}>
                <nav className="sidebar" aria-label="메뉴">
                    <div className="brand">
                        <span className="brand-mark"><Icon icon={Building2} size={18}/></span>
                        <span className="brand-name">{t.app.name}</span>
                    </div>
                    <button className="btn primary new-goal" aria-current={route.view === 'design' ? 'page' : undefined}
                            onClick={() => go({view: 'design'})} title={t.nav.design}>
                        <Icon icon={Plus}/><span className="label">{t.nav.design}</span>
                    </button>
                    {navButton(t.nav.dashboard, Home, {view: 'dashboard'}, route.view === 'dashboard')}
                    {navButton(t.nav.inbox, InboxIcon, {view: 'inbox'}, route.view === 'inbox',
                        inboxCount > 0 ? <span className="badge warn count" aria-label={`${inboxCount}건`}>{inboxCount}</span> : null)}
                    <div className="nav-section">{t.nav.services}</div>
                    {projects.map((s) => {
                        const st = serviceState(s);
                        return (
                            <button key={s.project.id} className="nav-item" title={s.project.name}
                                    aria-current={route.view === 'project' && route.projectId === s.project.id ? 'page' : undefined}
                                    onClick={() => go({view: 'project', projectId: s.project.id, tab: 'status'})}>
                                <span className="nav-initial" aria-hidden>{s.project.name.slice(0, 1)}</span>
                                <span className={`dot ${st.tone}`} aria-hidden/>
                                <span className="nav-label">{s.project.name}</span>
                                {st.label ? <span className={`badge svc-state ${st.tone}`}>{s.inbox > 0 ? `${st.label} ${s.inbox}` : st.label}</span> : null}
                            </button>
                        );
                    })}
                    {dash.data && projects.length === 0 ? <div className="nav-empty">아직 서비스가 없습니다</div> : null}
                    <div className="spacer"/>
                    <button className="nav-item" onClick={() => { tutorialTouched.current = true; setTutorialOpen(true); }} title="튜토리얼 다시 보기" aria-label="튜토리얼 다시 보기">
                        <Icon icon={CircleHelp} size={18}/><span className="nav-label">튜토리얼</span>
                    </button>
                    {navButton(t.nav.organization, Network, {view: 'org'}, route.view === 'org')}
                    {navButton(t.nav.settings, SettingsIcon, {view: 'settings'}, route.view === 'settings')}
                </nav>
                <main className="main" id="main" tabIndex={-1}>{main}</main>
                <DetailPane detail={detail} onClose={() => setDetail(null)}/>
            </div>
            {tutorialOpen ? <Tutorial onClose={closeTutorial}/> : null}
        </DetailProvider>
    );
}
