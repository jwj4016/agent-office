import {useEffect, useState} from 'react';
import type {Route} from '../App';
import {api} from '../api';
import {t} from '../i18n';
import {useLoad} from '../live';
import type {Project} from '../types';
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

function Overview({project, onSaved}: { project: Project; onSaved: () => void }) {
    const [form, setForm] = useState(project);
    useEffect(() => setForm(project), [project]);
    const action = useAction();
    const archived = project.status === 'archived';
    const dirty = form.name !== project.name || form.goal !== project.goal || form.instructions !== project.instructions || form.mode !== project.mode;
    const save = () => action.run(async () => {
        await api.updateProject({id: project.id, name: form.name, goal: form.goal, instructions: form.instructions, mode: form.mode});
        onSaved();
    });
    return (
        <div className="stack" style={{maxWidth: 640}}>
            <label className="field"><span>{t.project.name}</span>
                <input value={form.name} disabled={archived} onChange={(e) => setForm({...form, name: e.target.value})}/></label>
            <label className="field"><span>{t.project.goal}</span>
                <textarea value={form.goal} disabled={archived} onChange={(e) => setForm({...form, goal: e.target.value})}/></label>
            <label className="field"><span>{t.project.instructions}</span>
                <textarea value={form.instructions} disabled={archived} placeholder="이 서비스의 모든 담당자가 따를 지침"
                          onChange={(e) => setForm({...form, instructions: e.target.value})}/></label>
            <label className="field"><span>{t.project.mode}</span>
                <select value={form.mode} disabled={archived} onChange={(e) => setForm({...form, mode: e.target.value as Project['mode']})}>
                    <option value="review">{t.project.modeReview} (기본)</option>
                    <option value="auto">{t.project.modeAuto}</option>
                </select>
            </label>
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
