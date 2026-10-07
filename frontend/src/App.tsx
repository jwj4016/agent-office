import {type ReactNode, useEffect, useState} from 'react';
import {api} from './api';
import {t} from './i18n';
import {LiveProvider, useLoad} from './live';
import {DetailProvider} from './ui';
import {Dashboard} from './views/Dashboard';
import {Inbox} from './views/Inbox';
import {Organization} from './views/Organization';
import {ProjectView, type ProjectTab} from './views/ProjectView';
import {Settings} from './views/Settings';

export type Route =
    | { view: 'dashboard' }
    | { view: 'inbox' }
    | { view: 'org' }
    | { view: 'settings' }
    | { view: 'project'; projectId: string; tab: ProjectTab; workflowId?: string; runId?: string };

const LAST_PROJECT = 'ui.lastProjectId';

export default function App() {
    return (
        <LiveProvider>
            <Shell/>
        </LiveProvider>
    );
}

function Shell() {
    const status = useLoad(() => api.systemStatus(), []);
    if (status.data?.error) {
        // e.g. another Agent Office already uses this data folder.
        return (
            <main className="main" style={{maxWidth: 640}}>
                <h1>{t.app.name}을 시작할 수 없습니다</h1>
                <div className="alert" role="alert">{status.data.error}</div>
                <p className="muted">이미 열려 있는 Agent Office 창을 사용하거나, 그 창을 닫은 뒤 다시 실행하세요.</p>
            </main>
        );
    }
    return <Workspace/>;
}

function Workspace() {
    const [route, setRoute] = useState<Route>({view: 'dashboard'});
    const [detail, setDetail] = useState<ReactNode | null>(null);
    const dash = useLoad(() => api.dashboard(false), [], '*');
    const inbox = useLoad(() => api.inbox(), [], '*');

    // Restore the last selected service; switching never affects runs.
    useEffect(() => {
        api.settings().then((s) => {
            const id = s[LAST_PROJECT];
            if (id) setRoute((r) => (r.view === 'dashboard' ? {view: 'project', projectId: id, tab: 'overview'} : r));
        }).catch(() => {});
    }, []);

    const go = (r: Route) => {
        setDetail(null);
        setRoute(r);
        if (r.view === 'project') api.setSetting(LAST_PROJECT, r.projectId).catch(() => {});
    };

    const projects = dash.data ?? [];
    const current = route.view === 'project' ? projects.find((p) => p.project.id === route.projectId) : undefined;
    // A remembered project that no longer exists (or is archived) falls back.
    useEffect(() => {
        if (route.view === 'project' && dash.data && !current) {
            api.dashboard(true).then((all) => {
                if (!all.some((p) => p.project.id === route.projectId)) setRoute({view: 'dashboard'});
            });
        }
    }, [route, dash.data, current]);

    const inboxCount = inbox.data?.length ?? 0;
    const navButton = (label: string, r: Route, active: boolean, extra?: ReactNode) => (
        <button className="nav-item" aria-current={active ? 'page' : undefined} onClick={() => go(r)}>
            <span>{label}</span>{extra}
        </button>
    );

    let main: ReactNode;
    switch (route.view) {
        case 'dashboard':
            main = <Dashboard summaries={projects} loaded={dash.data !== undefined} onOpen={(id) => go({view: 'project', projectId: id, tab: 'overview'})} onCreated={dash.reload}/>;
            break;
        case 'inbox':
            main = <Inbox onOpenRun={(projectId, runId) => go({view: 'project', projectId, tab: 'runs', runId})}/>;
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
            <div className={`shell ${detail ? '' : 'no-detail'}`}>
                <nav className="sidebar" aria-label="메뉴">
                    <div className="brand">{t.app.name}</div>
                    {navButton(t.nav.dashboard, {view: 'dashboard'}, route.view === 'dashboard')}
                    {navButton(t.nav.inbox, {view: 'inbox'}, route.view === 'inbox',
                        inboxCount > 0 ? <span className="badge warn" aria-label={`${inboxCount}건`}>{inboxCount}</span> : null)}
                    {navButton(t.nav.organization, {view: 'org'}, route.view === 'org')}
                    <div className="section">{t.nav.services}</div>
                    {projects.map((s) => (
                        <button key={s.project.id} className="nav-item"
                                aria-current={route.view === 'project' && route.projectId === s.project.id ? 'page' : undefined}
                                onClick={() => go({view: 'project', projectId: s.project.id, tab: 'overview'})}>
                            <span>{s.project.name}</span>
                            {s.inbox > 0 ? <span className="badge warn">{s.inbox}</span>
                                : s.activeRuns > 0 ? <span className="badge info">{t.runStatus.running}</span> : null}
                        </button>
                    ))}
                    {projects.length === 0 ? <div className="muted small" style={{padding: '0 8px'}}>아직 서비스가 없습니다</div> : null}
                    <div className="spacer"/>
                    {navButton(t.nav.settings, {view: 'settings'}, route.view === 'settings')}
                </nav>
                <main className="main">{main}</main>
                {detail ? <aside className="detail" aria-label="상세">{detail}</aside> : null}
            </div>
        </DetailProvider>
    );
}
