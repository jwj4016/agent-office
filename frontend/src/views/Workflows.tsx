import {ChevronRight, Plus, Workflow as WorkflowIcon} from 'lucide-react';
import {useState} from 'react';
import {api} from '../api';
import {useLoad} from '../live';
import type {Project} from '../types';
import {Button, EmptyState, ErrorBox, Field, Icon, Loading, relativeTime, Section, useAction} from '../ui';
import {WorkflowEditor} from './WorkflowEditor';

export function Workflows({project, workflowId, onOpen, onStarted}: {
    project: Project; workflowId?: string; onOpen: (id?: string) => void; onStarted: (runId: string) => void;
}) {
    const list = useLoad(() => api.workflows(project.id), [project.id], project.id);
    const templates = useLoad(() => api.templates(), []);
    const [templateId, setTemplateId] = useState('blank');
    const [title, setTitle] = useState('');
    const [creating, setCreating] = useState(false);
    const action = useAction();
    const archived = project.status === 'archived';

    if (workflowId) {
        return <WorkflowEditor project={project} workflowId={workflowId} onStarted={onStarted} onClose={() => onOpen(undefined)}/>;
    }
    if (!list.data && !list.error) return <Loading/>;
    const create = async () => {
        const w = await action.run(() => api.createWorkflow(project.id, templateId, title));
        if (w) onOpen(w.id);
    };
    const flows = list.data ?? [];
    const form = (
        <form className="card stack" style={{maxWidth: 640}} aria-label="새 업무 흐름" onSubmit={(e) => { e.preventDefault(); create(); }}>
            <h2>새 업무 흐름</h2>
            <fieldset className="choice-group">
                <legend>시작 방법</legend>
                {(templates.data ?? []).map((tp) => (
                    <label key={tp.id} className="choice">
                        <input type="radio" name="template" checked={templateId === tp.id} onChange={() => setTemplateId(tp.id)}/>
                        <span><span className="choice-title">{tp.title}</span>
                            {tp.description ? <span className="choice-desc" style={{display: 'block'}}>{tp.description}</span> : null}</span>
                    </label>
                ))}
            </fieldset>
            <Field label="이름" optional hint="비우면 템플릿 이름을 씁니다."><input value={title} onChange={(e) => setTitle(e.target.value)}/></Field>
            <ErrorBox error={action.error}/>
            <div className="form-actions">
                <Button variant="primary" type="submit" busy={action.busy}>만들고 편집하기</Button>
                {flows.length ? <Button onClick={() => setCreating(false)}>취소</Button> : null}
            </div>
        </form>
    );
    return (
        <div className="stack loose">
            <ErrorBox error={list.error}/>
            {flows.length === 0 ? (
                archived ? <EmptyState icon={WorkflowIcon} title="업무 흐름이 없습니다"/> : (
                    <>
                        <EmptyState icon={WorkflowIcon} title="아직 업무 흐름이 없습니다" compact>
                            업무 흐름은 누가 어떤 순서로 일하는지 정한 설계도입니다. 템플릿에서 시작하거나 ‘새 목표’로 AI에게 맡길 수 있습니다.
                        </EmptyState>
                        {form}
                    </>
                )
            ) : (
                <Section title="업무 흐름" count={flows.length}
                         actions={!archived && !creating ? <Button icon={Plus} onClick={() => setCreating(true)}>새 업무 흐름</Button> : null}>
                    {creating ? form : null}
                    <div className="grid-cards">
                        {flows.map((w) => (
                            <button key={w.id} className="card clickable row nowrap" onClick={() => onOpen(w.id)}>
                                <span className="icon-wrap info"><Icon icon={WorkflowIcon} size={18}/></span>
                                <span className="stack tight" style={{flex: 1, minWidth: 0}}>
                                    <strong className="truncate">{w.title}</strong>
                                    <span className="muted xsmall">업무 {w.draft.nodes?.length ?? 0}개 · 수정 {w.revision} · {relativeTime(w.updatedAt)}</span>
                                </span>
                                <Icon icon={ChevronRight}/>
                            </button>
                        ))}
                    </div>
                </Section>
            )}
        </div>
    );
}
