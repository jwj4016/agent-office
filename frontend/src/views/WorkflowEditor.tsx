import {
    Background, Controls, type Edge, Handle, type Node, type NodeProps, Position, ReactFlow, type Connection as FlowConnection,
} from '@xyflow/react';
import '@xyflow/react/dist/style.css';
import {useEffect, useMemo, useState} from 'react';
import {api} from '../api';
import {t} from '../i18n';
import {useLoad} from '../live';
import type {Assignment, Branch, Issue, NodeKind, OutputType, Project, Routing, WorkflowNode, WorkflowSpec} from '../types';
import {ActorBadge, ConfirmButton, ErrorBox, IssueList, useAction, useDetail} from '../ui';
import {
    addNode, ancestors, availableInputs, branchLabel, branchStarts, byId, connect, describeBranch, disconnect, formatBranchValue,
    joinCandidates, layout, parseBranchValue, removeNode, renameNode, routingSources, scaffoldBranch, updateNode,
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
        const map = byId(draft);
        const edges: Edge[] = draft.nodes.flatMap((n) => n.dependsOn.map((d) => {
            const label = map.get(d)?.kind === 'condition' ? branchLabel(map.get(d)!.routing, n.id) : undefined;
            return {id: `${d}->${n.id}`, source: d, target: n.id, ...(label ? {label} : {})};
        }));
        // Rework targets are not edges of the graph; they are drawn dashed
        // for reference and cannot be selected or deleted as connections.
        const rework: Edge[] = draft.nodes.flatMap((n) => (n.reworkTargets ?? []).filter((r) => map.has(r)).map((r) => ({
            id: `rework:${n.id}->${r}`, source: n.id, target: r, label: '수정 요청', selectable: false, deletable: false, focusable: false,
            style: {strokeDasharray: '5 4', stroke: 'var(--danger)'}, labelStyle: {fill: 'var(--danger)'},
        })));
        return {nodes, edges: [...edges, ...rework]};
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

    // persist saves the draft and re-validates. It throws on failure so a
    // caller (confirm) stops instead of versioning stale content.
    const persist = async () => {
        const w = await api.saveDraft(project.id, workflowId, revision, draft);
        setRevision(w.revision);
        setDirty(false);
        const v = await api.validate(project.id, workflowId);
        setIssues(v.issues);
        setNotice(v.issues.length ? null : '저장했습니다. 문제가 없습니다.');
        return w;
    };

    const save = () => action.run(persist);

    const confirm = () => action.run(async () => {
        if (dirty) await persist();
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
                    onEdgesDelete={(es) => !archived && edit(es.filter((e) => !e.id.startsWith('rework:')).reduce((s, e) => disconnect(s, e.source, e.target), draft))}
                    onNodesDelete={(ns) => !archived && edit(ns.reduce((s, n) => removeNode(s, n.id), draft))}
                    deleteKeyCode={archived ? null : ['Backspace', 'Delete']}
                >
                    <Background/>
                    <Controls showInteractive={false}/>
                </ReactFlow>
            </div>
            <p className="muted small">노드 오른쪽 점에서 다음 업무의 왼쪽 점으로 끌어 연결합니다. 연결선이나 업무를 선택하고 Delete 키로 지웁니다.
                반려 후 되돌아가는 흐름은 연결선이 아니라 리뷰·승인 업무의 ‘수정 요청 대상’으로 지정하며, 점선으로 표시됩니다.
                조건 분기 뒤 연결선에는 그 경로를 고르는 규칙이 표시됩니다.</p>
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
            <thead><tr><th>업무</th><th>종류</th><th>담당자</th><th>선행 업무</th><th>설정</th><th>문제</th></tr></thead>
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
                        <td className="small">{settings(n).join(' · ')}</td>
                        <td className="small">{mine.length ? <span className="badge bad">{mine.length}</span> : ''}</td>
                    </tr>
                );
            })}
            </tbody>
        </table>
    );
}

// settings summarises a node's extra rules for the list view.
function settings(n: WorkflowNode): string[] {
    const out: string[] = [];
    if (n.routing) out.push(`분기 ${n.routing.branches.map(describeBranch).join(', ') || '없음'} → 기본 ${n.routing.defaultTarget || '?'}`);
    if (n.meeting) out.push(`회의 ${n.meeting.participants.length}명 · ${n.meeting.maxRounds || 3}라운드`);
    if (n.reworkTargets?.length) out.push(`수정 대상 ${n.reworkTargets.join(', ')}`);
    if (n.completion?.commands?.length) out.push(`검증 명령 ${n.completion.commands.length}개`);
    if (n.limits && Object.keys(n.limits).length) out.push('제한 있음');
    return out;
}

const outputTypes: OutputType[] = ['markdown', 'json', 'report', 'code_change', 'file'];

export function NodeForm({spec, id, people, readOnly, onChange, onRenamed, onDeleted, onClose}: {
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

            {n.kind === 'condition' ? <RoutingForm spec={spec} node={n} onChange={onChange}/> : null}
            {n.kind === 'task' ? <MeetingForm node={n} people={people} set={set}/> : null}
            {n.kind === 'task' || n.kind === 'review' ? <CompletionForm node={n} set={set}/> : null}
            {n.kind === 'task' || n.kind === 'review' ? <LimitsForm node={n} set={set}/> : null}

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

const operators: Branch['operator'][] = ['eq', 'ne', 'in', 'exists'];
const operatorLabel: Record<Branch['operator'], string> = {eq: '같음', ne: '다름', in: '목록 중 하나', exists: '값이 있음'};

// RoutingForm edits a condition: which JSON result and field decide,
// which next step each rule leads to, the default path and the join
// where the paths meet again. The Go validator checks the whole shape.
function RoutingForm({spec, node, onChange}: { spec: WorkflowSpec; node: WorkflowNode; onChange: (s: WorkflowSpec) => void }) {
    const sources = routingSources(spec, node.id);
    const starts = branchStarts(spec, node.id);
    const joins = joinCandidates(spec, node.id);
    const r = node.routing;
    if (!r) {
        return (
            <div className="stack">
                <h3>분기 규칙</h3>
                <p className="muted small">앞선 업무의 JSON 결과 값에 따라 다음 업무를 고릅니다. 고르지 않은 경로의 업무는 건너뜁니다.</p>
                {sources.length ? (
                    <button type="button" className="btn" onClick={() => onChange(scaffoldBranch(spec, node.id).spec)}>분기 구조 만들기</button>
                ) : <p className="small">분기 기준이 될 필수 JSON·보고 결과가 있는 선행 업무를 먼저 연결하세요.</p>}
            </div>
        );
    }
    const setR = (patch: Partial<Routing>) => onChange(updateNode(spec, node.id, {routing: {...r, ...patch}}));
    const setBranch = (i: number, patch: Partial<Branch>) => setR({branches: r.branches.map((b, j) => (j === i ? {...b, ...patch} : b))});
    const srcIndex = sources.findIndex((x) => x.fromStep === r.source.fromStep && x.output.key === r.source.outputKey);
    const target = (value: string, onPick: (v: string) => void, label: string) => (
        <select aria-label={label} value={value} onChange={(e) => onPick(e.target.value)}>
            <option value="">선택하세요</option>
            {starts.map((s) => <option key={s.id} value={s.id}>{s.title} ({s.kind === 'join' ? '바로 합류' : s.id})</option>)}
        </select>
    );
    return (
        <div className="stack" aria-label="분기 규칙">
            <h3>분기 규칙</h3>
            <label className="field"><span>기준 결과</span>
                <select aria-label="기준 결과" value={srcIndex >= 0 ? String(srcIndex) : ''} onChange={(e) => {
                    const pick = sources[Number(e.target.value)];
                    if (pick) setR({source: {...r.source, fromStep: pick.fromStep, outputKey: pick.output.key}});
                }}>
                    <option value="">선택하세요</option>
                    {sources.map((x, i) => <option key={i} value={i}>{x.title} · {x.output.key}</option>)}
                </select></label>
            <label className="field"><span>필드 경로 (예: legal.needed, items.0.kind)</span>
                <input aria-label="필드 경로" value={r.source.fieldPath} onChange={(e) => setR({source: {...r.source, fieldPath: e.target.value}})}/></label>
            <p className="muted small">위에서부터 처음 맞는 규칙의 업무로 갑니다. 값은 true·false·숫자·["목록"]은 그대로, 그 밖은 글자로 비교합니다.</p>
            {r.branches.map((b, i) => (
                <div key={`${node.id}-${i}`} className="row small" aria-label={`분기 ${i + 1}`}>
                    <select aria-label="비교 방식" value={b.operator} onChange={(e) => {
                        const op = e.target.value as Branch['operator'];
                        setBranch(i, op === 'exists' ? {operator: op, value: undefined} : {operator: op, value: parseBranchValue(op, formatBranchValue(b.value))});
                    }}>
                        {operators.map((o) => <option key={o} value={o}>{operatorLabel[o]}</option>)}
                    </select>
                    {b.operator !== 'exists' ? (
                        <input aria-label="비교 값" style={{width: 110}} defaultValue={formatBranchValue(b.value)}
                               onBlur={(e) => setBranch(i, {value: parseBranchValue(b.operator, e.target.value)})}/>
                    ) : null}
                    <span>→</span>
                    {target(b.targetStep, (v) => setBranch(i, {targetStep: v}), '분기 대상')}
                    <button type="button" className="btn small danger" onClick={() => setR({branches: r.branches.filter((_, j) => j !== i)})}>삭제</button>
                </div>
            ))}
            <div className="row">
                <button type="button" className="btn small" onClick={() => setR({branches: [...r.branches, {operator: 'eq', value: '', targetStep: ''}]})}>규칙 추가</button>
                <button type="button" className="btn small" onClick={() => {
                    const a = addNode(spec, 'task', node.id);
                    onChange(updateNode(a.spec, node.id, {routing: {...r, branches: [...r.branches, {operator: 'eq', value: '', targetStep: a.id}]}}));
                }}>새 분기 업무와 규칙 추가</button>
            </div>
            <label className="field"><span>맞는 규칙이 없을 때 (기본 경로)</span>{target(r.defaultTarget, (v) => setR({defaultTarget: v}), '기본 경로')}</label>
            <label className="field"><span>합류 업무 (모든 경로가 다시 만나는 곳)</span>
                <select aria-label="합류 업무" value={r.joinStep} onChange={(e) => setR({joinStep: e.target.value})}>
                    <option value="">선택하세요</option>
                    {joins.map((j) => <option key={j.id} value={j.id}>{j.title} ({j.id})</option>)}
                </select></label>
            {!joins.length ? <p className="small">‘업무 추가’에서 합류 업무를 만들고 각 분기의 마지막 업무를 연결하세요.</p> : null}
            <p className="muted small">분기 안의 업무는 분기 밖 업무에 의존할 수 없고, 분기 뒤의 업무가 분기 결과를 필수 입력으로 쓸 수 없습니다(선택 입력은 가능).</p>
        </div>
    );
}

// MeetingForm turns a task into a consultation: AI participants give
// opinions for some rounds, then the task's assignee decides.
function MeetingForm({node, people, set}: { node: WorkflowNode; people: Assignment[]; set: (p: Partial<WorkflowNode>) => void }) {
    const m = node.meeting;
    const ais = people.filter((p) => p.actorKind === 'ai' && p.id !== node.assignmentId);
    return (
        <div className="stack">
            <label className="check"><input type="checkbox" checked={!!m}
                onChange={(e) => set({meeting: e.target.checked ? {participants: [], maxRounds: 3} : undefined})}/>회의(협의 작업)로 진행</label>
            {m ? (
                <div className="stack">
                    <p className="muted small">참여 AI가 라운드마다 의견을 내고, 모두 동의하면 일찍 끝납니다. 그다음 이 업무의 담당자가 결정해 결과를 만듭니다.</p>
                    <fieldset className="row" style={{border: 'none', padding: 0}}>
                        <legend className="small muted">참여자 (AI)</legend>
                        {ais.map((p) => (
                            <label key={p.id} className="check small">
                                <input type="checkbox" checked={m.participants.includes(p.id)} onChange={(e) => set({
                                    meeting: {...m, participants: e.target.checked ? [...m.participants, p.id] : m.participants.filter((x) => x !== p.id)},
                                })}/>{p.displayName}
                            </label>
                        ))}
                        {!ais.length ? <span className="small">참여할 AI 담당자가 없습니다.</span> : null}
                    </fieldset>
                    <label className="field"><span>최대 라운드 (1~10)</span>
                        <input type="number" min={1} max={10} aria-label="최대 라운드" value={m.maxRounds ?? 3}
                               onChange={(e) => set({meeting: {...m, maxRounds: Number(e.target.value) || undefined}})}/></label>
                </div>
            ) : null}
        </div>
    );
}

type Command = { executable: string; args?: string[]; timeout?: string };

// CompletionForm lists commands that must succeed for the step to
// complete (tests, builds). They run in the step's workspace, never
// through a shell.
function CompletionForm({node, set}: { node: WorkflowNode; set: (p: Partial<WorkflowNode>) => void }) {
    const cmds: Command[] = node.completion?.commands ?? [];
    const save = (next: Command[]) => set({completion: next.length ? {...node.completion, commands: next} : undefined});
    return (
        <div className="stack">
            <h3>검증 명령</h3>
            <p className="muted small">업무가 끝났다고 인정되려면 성공해야 하는 명령입니다(테스트·빌드). 작업 공간에서 셸 없이 실행되며, 인수는 한 줄에 하나씩 씁니다.</p>
            {cmds.map((c, i) => (
                <div key={i} className="stack card" aria-label={`검증 명령 ${i + 1}`}>
                    <label className="field"><span>실행 파일</span>
                        <input aria-label="실행 파일" value={c.executable} placeholder="예: go, npm, python"
                               onChange={(e) => save(cmds.map((x, j) => (j === i ? {...x, executable: e.target.value} : x)))}/></label>
                    <label className="field"><span>인수 (한 줄에 하나)</span>
                        <textarea aria-label="인수" rows={2} value={(c.args ?? []).join('\n')} placeholder={'test\n./...'}
                                  onChange={(e) => save(cmds.map((x, j) => (j === i ? {...x, args: e.target.value.split('\n')} : x)))}
                                  onBlur={() => save(cmds.map((x, j) => (j === i ? {...x, args: (x.args ?? []).filter((a) => a.trim() !== '')} : x)))}/></label>
                    <div className="row">
                        <label className="field"><span>제한 시간 (예: 5m)</span>
                            <input aria-label="명령 제한 시간" value={c.timeout ?? ''} onChange={(e) => save(cmds.map((x, j) => (j === i ? {...x, timeout: e.target.value || undefined} : x)))}/></label>
                        <button type="button" className="btn small danger" onClick={() => save(cmds.filter((_, j) => j !== i))}>삭제</button>
                    </div>
                </div>
            ))}
            <button type="button" className="btn small" onClick={() => save([...cmds, {executable: '', args: []}])}>검증 명령 추가</button>
        </div>
    );
}

// LimitsForm sets the step's time and revision limits.
function LimitsForm({node, set}: { node: WorkflowNode; set: (p: Partial<WorkflowNode>) => void }) {
    const l = (node.limits ?? {}) as { timeout?: string; maxRevisions?: number };
    const save = (patch: Record<string, unknown>) => {
        const next: Record<string, unknown> = {...l, ...patch};
        Object.keys(next).forEach((k) => (next[k] === undefined || next[k] === '') && delete next[k]);
        set({limits: Object.keys(next).length ? next : undefined});
    };
    return (
        <div className="row">
            <label className="field"><span>제한 시간 (예: 30m)</span>
                <input aria-label="업무 제한 시간" value={l.timeout ?? ''} onChange={(e) => save({timeout: e.target.value || undefined})}/></label>
            <label className="field"><span>최대 수정 회차 (비우면 3)</span>
                <input type="number" min={0} aria-label="최대 수정 회차" value={l.maxRevisions ?? ''}
                       onChange={(e) => save({maxRevisions: e.target.value === '' ? undefined : Number(e.target.value)})}/></label>
        </div>
    );
}
