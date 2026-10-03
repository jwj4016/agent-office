// Pure editing operations on a workflow draft. The Go validator is the
// authority; these only keep the draft consistent while editing.
import type {NodeKind, WorkflowNode, WorkflowOutput, WorkflowSpec} from './types';

export function byId(spec: WorkflowSpec): Map<string, WorkflowNode> {
    return new Map(spec.nodes.map((n) => [n.id, n]));
}

export function ancestors(spec: WorkflowSpec, id: string): Set<string> {
    const map = byId(spec);
    const out = new Set<string>();
    const stack = [...(map.get(id)?.dependsOn ?? [])];
    while (stack.length) {
        const d = stack.pop()!;
        if (out.has(d) || !map.has(d)) continue;
        out.add(d);
        stack.push(...(map.get(d)?.dependsOn ?? []));
    }
    return out;
}

// layout places nodes in columns by their longest path from a start node.
export function layout(spec: WorkflowSpec): Record<string, { x: number; y: number }> {
    const map = byId(spec);
    const depth = new Map<string, number>();
    const visiting = new Set<string>();
    const d = (id: string): number => {
        if (depth.has(id)) return depth.get(id)!;
        if (visiting.has(id)) return 0; // cycle: validator reports it
        visiting.add(id);
        const deps = (map.get(id)?.dependsOn ?? []).filter((x) => map.has(x));
        const v = deps.length ? Math.max(...deps.map(d)) + 1 : 0;
        visiting.delete(id);
        depth.set(id, v);
        return v;
    };
    const rows = new Map<number, number>();
    const pos: Record<string, { x: number; y: number }> = {};
    for (const n of spec.nodes) {
        const col = d(n.id);
        const row = rows.get(col) ?? 0;
        rows.set(col, row + 1);
        pos[n.id] = {x: col * 230, y: row * 110};
    }
    return pos;
}

const defaultOutputs: Record<NodeKind, WorkflowOutput[]> = {
    task: [{key: 'result', type: 'markdown'}],
    review: [{key: 'review', type: 'report'}],
    approval: [], condition: [], join: [],
};

const defaultTitle: Record<NodeKind, string> = {
    task: '새 작업', review: '리뷰', approval: '승인', condition: '조건 분기', join: '합류',
};

export function newNodeId(spec: WorkflowSpec): string {
    const ids = new Set(spec.nodes.map((n) => n.id));
    for (let i = 1; ; i++) {
        const id = `step-${i}`;
        if (!ids.has(id)) return id;
    }
}

export function addNode(spec: WorkflowSpec, kind: NodeKind, after?: string): { spec: WorkflowSpec; id: string } {
    const id = newNodeId(spec);
    const node: WorkflowNode = {
        id, title: defaultTitle[kind], kind, dependsOn: after ? [after] : [],
        ...(kind === 'condition' || kind === 'join' ? {} : {assignmentId: ''}),
        ...(defaultOutputs[kind].length ? {outputs: defaultOutputs[kind].map((o) => ({...o}))} : {}),
    };
    return {spec: {...spec, nodes: [...spec.nodes, node]}, id};
}

const without = (xs: string[] | undefined, id: string) => (xs ?? []).filter((x) => x !== id);

// removeNode deletes a node and every reference to it.
export function removeNode(spec: WorkflowSpec, id: string): WorkflowSpec {
    return {
        ...spec,
        nodes: spec.nodes.filter((n) => n.id !== id).map((n) => ({
            ...n,
            dependsOn: without(n.dependsOn, id),
            ...(n.inputs ? {inputs: n.inputs.filter((i) => i.fromStep !== id)} : {}),
            ...(n.reworkTargets ? {reworkTargets: without(n.reworkTargets, id)} : {}),
        })),
    };
}

// connect makes target depend on source, refusing self-links and cycles.
export function connect(spec: WorkflowSpec, source: string, target: string): { spec: WorkflowSpec; error?: string } {
    if (source === target) return {spec, error: '자기 자신에게 연결할 수 없습니다'};
    if (ancestors(spec, source).has(target)) {
        return {spec, error: '순환이 생깁니다. 되돌려 보내는 흐름은 수정 요청 대상으로 지정하세요'};
    }
    return {
        spec: {
            ...spec,
            nodes: spec.nodes.map((n) => n.id === target && !n.dependsOn.includes(source)
                ? {...n, dependsOn: [...n.dependsOn, source]} : n),
        },
    };
}

export function disconnect(spec: WorkflowSpec, source: string, target: string): WorkflowSpec {
    const cut: WorkflowSpec = {
        ...spec,
        nodes: spec.nodes.map((n) => n.id === target ? {...n, dependsOn: without(n.dependsOn, source)} : n),
    };
    // Inputs from steps that are no longer upstream would be invalid.
    const still = ancestors(cut, target);
    return {
        ...cut,
        nodes: cut.nodes.map((n) => n.id === target && n.inputs ? {...n, inputs: n.inputs.filter((i) => still.has(i.fromStep))} : n),
    };
}

export function updateNode(spec: WorkflowSpec, id: string, patch: Partial<WorkflowNode>): WorkflowSpec {
    return {...spec, nodes: spec.nodes.map((n) => (n.id === id ? {...n, ...patch} : n))};
}

// renameNode changes a node id and every reference to it.
export function renameNode(spec: WorkflowSpec, from: string, to: string): { spec: WorkflowSpec; error?: string } {
    if (!/^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$/.test(to)) return {spec, error: 'ID는 영문·숫자·-·_로 64자 이내입니다'};
    if (from !== to && spec.nodes.some((n) => n.id === to)) return {spec, error: '이미 있는 ID입니다'};
    const swap = (x: string) => (x === from ? to : x);
    return {
        spec: {
            ...spec,
            nodes: spec.nodes.map((n) => ({
                ...n,
                id: swap(n.id),
                dependsOn: n.dependsOn.map(swap),
                ...(n.inputs ? {inputs: n.inputs.map((i) => ({...i, fromStep: swap(i.fromStep)}))} : {}),
                ...(n.reworkTargets ? {reworkTargets: n.reworkTargets.map(swap)} : {}),
                ...(n.routing ? {
                    routing: {
                        ...n.routing,
                        source: {...n.routing.source, fromStep: swap(n.routing.source.fromStep)},
                        branches: n.routing.branches.map((b) => ({...b, targetStep: swap(b.targetStep)})),
                        defaultTarget: swap(n.routing.defaultTarget),
                        joinStep: swap(n.routing.joinStep),
                    },
                } : {}),
            })),
        },
    };
}

// availableInputs lists outputs of upstream steps a node may take as input.
export function availableInputs(spec: WorkflowSpec, id: string): { fromStep: string; title: string; output: WorkflowOutput }[] {
    const map = byId(spec);
    return [...ancestors(spec, id)].flatMap((a) => (map.get(a)?.outputs ?? []).map((o) => ({
        fromStep: a, title: map.get(a)!.title, output: o,
    })));
}
