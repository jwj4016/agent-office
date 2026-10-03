import {useState} from 'react';
import {api} from '../api';
import {useLoad} from '../live';
import type {Project} from '../types';
import {ErrorBox, shortTime, useAction} from '../ui';
import {WorkflowEditor} from './WorkflowEditor';

export function Workflows({project, workflowId, onOpen, onStarted}: {
    project: Project; workflowId?: string; onOpen: (id?: string) => void; onStarted: (runId: string) => void;
}) {
    const list = useLoad(() => api.workflows(project.id), [project.id], project.id);
    const templates = useLoad(() => api.templates(), []);
    const [templateId, setTemplateId] = useState('blank');
    const [title, setTitle] = useState('');
    const action = useAction();
    const archived = project.status === 'archived';

    if (workflowId) {
        return <WorkflowEditor project={project} workflowId={workflowId} onStarted={onStarted} onClose={() => onOpen(undefined)}/>;
    }
    const create = async () => {
        const w = await action.run(() => api.createWorkflow(project.id, templateId, title));
        if (w) onOpen(w.id);
    };
    return (
        <div className="stack">
            <ErrorBox error={list.error || action.error}/>
            {(list.data ?? []).length === 0 ? <p className="muted">아직 업무 흐름이 없습니다.</p> : null}
            <div className="grid-cards">
                {(list.data ?? []).map((w) => (
                    <button key={w.id} className="card clickable" onClick={() => onOpen(w.id)}>
                        <h2 style={{margin: 0}}>{w.title}</h2>
                        <p className="muted small">업무 {w.draft.nodes?.length ?? 0}개 · 수정 {w.revision} · {shortTime(w.updatedAt)}</p>
                    </button>
                ))}
            </div>
            {!archived ? (
                <form className="card stack" style={{maxWidth: 560}} aria-label="새 업무 흐름" onSubmit={(e) => { e.preventDefault(); create(); }}>
                    <h2>새 업무 흐름</h2>
                    <label className="field"><span>시작 방법</span>
                        <select value={templateId} onChange={(e) => setTemplateId(e.target.value)}>
                            {(templates.data ?? []).map((tp) => <option key={tp.id} value={tp.id}>{tp.title}</option>)}
                        </select></label>
                    <p className="muted small">{templates.data?.find((tp) => tp.id === templateId)?.description}</p>
                    <label className="field"><span>이름 (비우면 템플릿 이름)</span>
                        <input value={title} onChange={(e) => setTitle(e.target.value)}/></label>
                    <div className="row"><button className="btn primary" type="submit" disabled={action.busy}>만들기</button></div>
                </form>
            ) : null}
        </div>
    );
}
