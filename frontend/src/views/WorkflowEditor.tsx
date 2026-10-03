import {
    Background, Controls, type Edge, Handle, type Node, type NodeProps, Position, ReactFlow, type Connection as FlowConnection,
} from '@xyflow/react';
import '@xyflow/react/dist/style.css';
import {useEffect, useMemo, useState} from 'react';
import {api} from '../api';
import {t} from '../i18n';
import {useLoad} from '../live';
import type {Assignment, Issue, NodeKind, OutputType, Project, WorkflowNode, WorkflowSpec} from '../types';
import {ActorBadge, ConfirmButton, ErrorBox, IssueList, useAction, useDetail} from '../ui';
import {
    addNode, ancestors, availableInputs, byId, connect, disconnect, layout, removeNode, renameNode, updateNode,
} from '../workflowModel';

type StepData = { node: WorkflowNode; assignee?: Assignment; hasError: boolean; selected: boolean };

function StepNode({data}: NodeProps<Node<StepData>>) {
    const {node, assignee, hasError, selected} = data;
    return (
        <div className={`flow-node ${selected ? 'selected' : ''} ${hasError ? 'has-error' : ''}`}>
            <Handle type="target" position={Position.Left}/>
            <div className="kind">{t.nodeKind[node.kind] ?? node.kind} · {node.id}</div>
            <div><strong>{node.title || '(이름 없음)'}</strong></div>
            {node.kind === 'condition' || node.kind === 'join' ? null : assignee
                ? <div className="row small"><ActorBadge kind={assignee.actorKind}/> {assignee.displayName}</div>
                : <div className="small" style={{color: 'var(--danger)'}}>담당자 없음</div>}
            <Handle type="source" position={Position.Right}/>
        </div>
    );
}

const nodeTypes = {step: StepNode};

export function WorkflowEditor({project, workflowId, onStarted, onClose}: {
    project: Project; workflowId: string; onStarted: (runId: string) => void; onClose: () => void;
}) {
    const loaded = useLoad(() => api.workflow(project.id, workflowId), [project.id, workflowId]);
    const team = useLoad(() => api.assignments(project.id), [project.id], project.id);
    const [draft, setDraft] = useState<WorkflowSpec | null>(null);
    const [revision, setRevision] = useState(0);
    const [dirty, setDirty] = useState(false);
    const [selected, setSelected] = useState<string | null>(null);
    const [issues, setIssues] = useState<Issue[]>([]);
    const [notice, setNotice] = useState<string | null>(null);
    const [version, setVersion] = useState<{ id: string; number: number } | null>(null);
    const action = useAction();
    const setDetail = useDetail();
    const archived = project.status === 'archived';

    useEffect(() => {
        if (loaded.data && !dirty) {
            setDraft(loaded.data.draft);
            setRevision(loaded.data.revision);
        }
        // only (re)load from the server when there are no local edits
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [loaded.data]);

    const edit = (next: WorkflowSpec) => {
        setDraft(next);
        setDirty(true);
        setVersion(null);
        setNotice(null);
    };

    const people = team.data ?? [];
    const assignee = (id?: string) => people.find((a) => a.id === id);
    const errorNodes = new Set(issues.filter((i) => i.nodeId).map((i) => i.nodeId!));

    const flow = useMemo(() => {
        if (!draft) return {nodes: [], edges: []};
        const pos = layout(draft);
        const nodes: Node<StepData>[] = draft.nodes.map((n) => ({
            id: n.id, type: 'step', position: pos[n.id],
            data: {node: n, assignee: assignee(n.assignmentId), hasError: errorNodes.has(n.id), selected: n.id === selected},
        }));
        const edges: Edge[] = draft.nodes.flatMap((n) => n.dependsOn.map((d) => ({id: `${d}->${n.id}`, source: d, target: n.id})));
        return {nodes, edges};
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [draft, people, issues, selected]);

    // The selected node's form lives in the detail pane.
    useEffect(() => {
        if (!draft || !selected || !byId(draft).has(selected)) {
            setDetail(null);
            return;
        }
        setDetail(<NodeForm spec={draft} id={selected} people={people} readOnly={archived}
                            onChange={edit} onRenamed={setSelected} onDeleted={() => setSelected(null)}
                            onClose={() => setSelected(null)}/>);
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [draft, selected, people, archived]);
    useEffect(() => () => setDetail(null), [setDetail]);

    if (loaded.error) return <ErrorBox error={loaded.error}/>;
    if (!draft) return <p className="muted">{t.common.loading}</p>;

    const save = () => action.run(async () => {
        const w = await api.saveDraft(project.id, workflowId, revision, draft);
        setRevision(w.revision);
        setDirty(false);
        const v = await api.validate(project.id, workflowId);
        setIssues(v.issues);
        setNotice(v.issues.length ? null : '저장했습니다. 문제가 없습니다.');
        return w;
    });

    const confirm = () => action.run(async () => {
        if (dirty) await save();
        const w = await api.workflow(project.id, workflowId);
        const res = await api.confirmVersion(project.id, workflowId, w.revision);
        setIssues(res.validation.issues);
        if (res.version) {
            setVersion({id: res.version.id, number: res.version.number});
            setNotice(res.validation.canRun
                ? `버전 ${res.version.number}을 확정했습니다. 실행 중인 기록은 이전 버전을 계속 사용합니다.`
                : `버전 ${res.version.number}을 확정했지만 실행 전에 해결할 항목이 있습니다.`);
        } else {
            setNotice(null);
        }
    });

    const start = () => action.run(async () => {
        if (!version) return;
        const res = await api.startRun(project.id, version.id);
        if (res.runId) onStarted(res.runId);
        else setIssues(res.issues);
    });

    return (
        <div className="stack">
            <div className="row between">
                <div className="row">
                    <button className="btn small" onClick={onClose}>← 목록</button>
                    <input aria-label="흐름 이름" value={draft.title} disabled={archived} onChange={(e) => edit({...draft, title: e.target.value})}/>
                    <span className="muted small">수정 {revision}{dirty ? ' · 저장 안 됨' : ''}</span>
                </div>
                <div className="row">
                    <label className="row small">
                        <span className="muted">업무 추가</span>
                        <select aria-label="추가할 업무 종류" value="" disabled={archived} onChange={(e) => {
                            if (!e.target.value) return;
                            const {spec, id} = addNode(draft, e.target.value as NodeKind, selected ?? undefined);
                            edit(spec);
                            setSelected(id);
                        }}>
                            <option value="">종류 선택…</option>
                            {(['task', 'review', 'approval', 'join', 'condition'] as NodeKind[]).map((k) =>
                                <option key={k} value={k}>{t.nodeKind[k]}</option>)}
                        </select>
                    </label>
                    <button className="btn" disabled={archived || action.busy} onClick={save}>저장·검증</button>
                    <button className="btn primary" disabled={archived || action.busy} onClick={confirm}>버전 확정</button>
                    {version ? <button className="btn primary" disabled={action.busy} onClick={start}>버전 {version.number} 실행</button> : null}
                </div>
            </div>
            <ErrorBox error={action.error}/>
            {notice ? <div className="notice">{notice}</div> : null}
            <IssueList issues={issues}/>
            <div className="flow-wrap" aria-label="업무 흐름 편집기">
                <ReactFlow
                    nodes={flow.nodes} edges={flow.edges} nodeTypes={nodeTypes} fitView
                    nodesDraggable={false} nodesConnectable={!archived} elementsSelectable
                    onNodeClick={(_, n) => setSelected(n.id)}
                    onPaneClick={() => setSelected(null)}
                    onConnect={(c: FlowConnection) => {
                        if (!c.source || !c.target) return;
                        const res = connect(draft, c.source, c.target);
                        if (res.error) action.setError(res.error); else edit(res.spec);
                    }}
                    onEdgesDelete={(es) => !archived && edit(es.reduce((s, e) => disconnect(s, e.source, e.target), draft))}
                    onNodesDelete={(ns) => !archived && edit(ns.reduce((s, n) => removeNode(s, n.id), draft))}
                    deleteKeyCode={archived ? null : ['Backspace', 'Delete']}
                >
                    <Background/>
                    <Controls showInteractive={false}/>
                </ReactFlow>
            </div>
            <p className="muted small">노드 오른쪽 점에서 다음 업무의 왼쪽 점으로 끌어 연결합니다. 연결선이나 업무를 선택하고 Delete 키로 지웁니다.
                반려 후 되돌아가는 흐름은 연결선이 아니라 리뷰·승인 업무의 ‘수정 요청 대상’으로 지정합니다.</p>
            <StepList spec={draft} people={people} issues={issues} onSelect={setSelected}/>
        </div>
    );
}

// StepList is the keyboard- and screen-reader-friendly view of the graph.
function StepList({spec, people, issues, onSelect}: {
    spec: WorkflowSpec; people: Assignment[]; issues: Issue[]; onSelect: (id: string) => void;
}) {
    return (
        <table className="list" aria-label="업무 목록">
            <thead><tr><th>업무</th><th>종류</th><th>담당자</th><th>선행 업무</th><th>문제</th></tr></thead>
            <tbody>
            {spec.nodes.map((n) => {
                const a = people.find((p) => p.id === n.assignmentId);
                const mine = issues.filter((i) => i.nodeId === n.id);
                return (
                    <tr key={n.id}>
                        <td><button className="btn small" onClick={() => onSelect(n.id)}>{n.title || n.id}</button></td>
                        <td>{t.nodeKind[n.kind] ?? n.kind}</td>
                        <td>{a ? <>{a.displayName} <ActorBadge kind={a.actorKind}/></> : n.kind === 'condition' || n.kind === 'join' ? '—' : <span className="badge bad">없음</span>}</td>
                        <td className="small">{n.dependsOn.join(', ') || '시작'}</td>
                        <td className="small">{mine.length ? <span className="badge bad">{mine.length}</span> : ''}</td>
                    </tr>
                );
            })}
            </tbody>
        </table>
    );
}

const outputTypes: OutputType[] = ['markdown', 'json', 'report', 'code_change', 'file'];

function NodeForm({spec, id, people, readOnly, onChange, onRenamed, onDeleted, onClose}: {
    spec: WorkflowSpec; id: string; people: Assignment[]; readOnly: boolean;
    onChange: (s: WorkflowSpec) => void; onRenamed: (id: string) => void; onDeleted: () => void; onClose: () => void;
}) {
    const n = byId(spec).get(id)!;
    const [idText, setIdText] = useState(id);
    const [error, setError] = useState<string | null>(null);
    useEffect(() => setIdText(id), [id]);
    const set = (patch: Partial<WorkflowNode>) => onChange(updateNode(spec, id, patch));
    const upstream = ancestors(spec, id);
    const map = byId(spec);
    const inputs = availableInputs(spec, id);
    const needsAssignee = n.kind !== 'condition' && n.kind !== 'join';
    const candidates = n.kind === 'approval' ? people.filter((p) => p.actorKind === 'human') : people;

    return (
        <fieldset className="stack" disabled={readOnly} style={{border: 'none', padding: 0, margin: 0}} aria-label="업무 편집">
            <div className="row between"><h2>업무 편집</h2><button type="button" className="btn small" onClick={onClose}>{t.common.close}</button></div>
            <label className="field"><span>이름</span><input value={n.title} onChange={(e) => set({title: e.target.value})}/></label>
            <label className="field"><span>ID</span>
                <input value={idText} onChange={(e) => setIdText(e.target.value)} onBlur={() => {
                    if (idText === id) return;
                    const res = renameNode(spec, id, idText);
                    if (res.error) { setError(res.error); setIdText(id); } else { setError(null); onChange(res.spec); onRenamed(idText); }
                }}/></label>
            <label className="field"><span>종류</span>
                <select value={n.kind} onChange={(e) => set({kind: e.target.value as NodeKind})}>
                    {(['task', 'review', 'approval', 'condition', 'join'] as NodeKind[]).map((k) => <option key={k} value={k}>{t.nodeKind[k]}</option>)}
                </select></label>
            {needsAssignee ? (
                <label className="field"><span>담당자{n.kind === 'approval' ? ' (승인은 사람만)' : ''}</span>
                    <select value={n.assignmentId ?? ''} onChange={(e) => set({assignmentId: e.target.value})}>
                        <option value="">선택하세요</option>
                        {candidates.map((p) => <option key={p.id} value={p.id}>{p.displayName} ({p.actorKind === 'ai' ? 'AI' : '나'})</option>)}
                    </select></label>
            ) : null}
            {needsAssignee ? (
                <label className="field"><span>이번 업무 지시</span>
                    <textarea value={n.instructions ?? ''} onChange={(e) => set({instructions: e.target.value})}/></label>
            ) : null}

            {n.kind === 'task' || n.kind === 'review' ? (
                <div className="stack">
                    <h3>결과</h3>
                    {(n.outputs ?? []).map((o, i) => (
                        <div key={i} className="row">
                            <input aria-label="결과 키" style={{width: 100}} value={o.key}
                                   onChange={(e) => set({outputs: n.outputs!.map((x, j) => j === i ? {...x, key: e.target.value} : x)})}/>
                            <select aria-label="결과 형식" value={o.type}
                                    onChange={(e) => set({outputs: n.outputs!.map((x, j) => j === i ? {...x, type: e.target.value as OutputType} : x)})}>
                                {outputTypes.map((ty) => <option key={ty} value={ty}>{t.outputType[ty]}</option>)}
                            </select>
                            <label className="check small"><input type="checkbox" checked={o.required !== false}
                                onChange={(e) => set({outputs: n.outputs!.map((x, j) => j === i ? {...x, required: e.target.checked ? undefined : false} : x)})}/>{t.common.required}</label>
                            <button type="button" className="btn small danger" onClick={() => set({outputs: n.outputs!.filter((_, j) => j !== i)})}>삭제</button>
                        </div>
                    ))}
                    <button type="button" className="btn small" onClick={() => set({outputs: [...(n.outputs ?? []), {key: `out${(n.outputs?.length ?? 0) + 1}`, type: 'markdown'}]})}>결과 추가</button>
                </div>
            ) : null}

            {needsAssignee ? (
                <div className="stack">
                    <h3>입력 자료</h3>
                    {(n.inputs ?? []).map((inp, i) => (
                        <div key={i} className="row small">
                            <span>{inp.name} ← {map.get(inp.fromStep)?.title ?? inp.fromStep}.{inp.outputKey}</span>
                            <label className="check"><input type="checkbox" checked={inp.required !== false}
                                onChange={(e) => set({inputs: n.inputs!.map((x, j) => j === i ? {...x, required: e.target.checked ? undefined : false} : x)})}/>{t.common.required}</label>
                            <button type="button" className="btn small danger" onClick={() => set({inputs: n.inputs!.filter((_, j) => j !== i)})}>삭제</button>
                        </div>
                    ))}
                    {inputs.length ? (
                        <select aria-label="입력 추가" value="" onChange={(e) => {
                            const pick = inputs[Number(e.target.value)];
                            if (!pick) return;
                            set({inputs: [...(n.inputs ?? []), {name: `${pick.title} ${pick.output.key}`, fromStep: pick.fromStep, outputKey: pick.output.key}]});
                        }}>
                            <option value="">입력 추가…</option>
                            {inputs.map((x, i) => <option key={i} value={i}>{x.title} · {x.output.key} ({t.outputType[x.output.type]})</option>)}
                        </select>
                    ) : <p className="muted small">선행 업무를 연결하면 그 결과를 입력으로 고를 수 있습니다.</p>}
                </div>
            ) : null}

            {n.kind === 'review' || n.kind === 'approval' ? (
                <div className="stack">
                    <h3>수정 요청 대상</h3>
                    <p className="muted small">반려·수정 요청 때 돌려보낼 수 있는 선행 업무입니다.</p>
                    {[...upstream].filter((u) => ['task', 'review'].includes(map.get(u)!.kind)).map((u) => (
                        <label key={u} className="check small">
                            <input type="checkbox" checked={(n.reworkTargets ?? []).includes(u)} onChange={(e) => set({
                                reworkTargets: e.target.checked ? [...(n.reworkTargets ?? []), u] : (n.reworkTargets ?? []).filter((x) => x !== u),
                            })}/>{map.get(u)!.title}
                        </label>
                    ))}
                </div>
            ) : null}

            {n.kind === 'condition' ? (
                <div className="stack">
                    <h3>분기 규칙</h3>
                    <p className="muted small">조건·분기 편집 화면은 M4에서 완성됩니다. 지금은 규칙을 그대로 보여 줍니다.</p>
                    <pre className="content">{JSON.stringify(n.routing ?? null, null, 2)}</pre>
                </div>
            ) : null}

            <div className="stack">
                <h3>선행 업무</h3>
                {n.dependsOn.length === 0 ? <p className="muted small">없음 (시작 업무)</p> : n.dependsOn.map((d) => (
                    <div key={d} className="row small">{map.get(d)?.title ?? d}
                        <button type="button" className="btn small" onClick={() => onChange(disconnect(spec, d, id))}>연결 끊기</button></div>
                ))}
            </div>
            {error ? <ErrorBox error={error}/> : null}
            <ConfirmButton label="업무 삭제" confirmLabel="삭제하기" className="btn danger"
                           onConfirm={() => { onChange(removeNode(spec, id)); onDeleted(); }}/>
        </fieldset>
    );
}
