// Pure editing operations on a workflow draft. The Go validator is the
// authority; these only keep the draft consistent while editing.
import type {Branch, NodeKind, Routing, WorkflowNode, WorkflowOutput, WorkflowSpec} from './types';

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

// removeNode deletes a node and every reference to it. Routing that
// pointed at it is cleared so the validator asks for a new choice.
export function removeNode(spec: WorkflowSpec, id: string): WorkflowSpec {
    const clear = (x: string) => (x === id ? '' : x);
    return {
        ...spec,
        nodes: spec.nodes.filter((n) => n.id !== id).map((n) => ({
            ...n,
            dependsOn: without(n.dependsOn, id),
            ...(n.inputs ? {inputs: n.inputs.filter((i) => i.fromStep !== id)} : {}),
            ...(n.reworkTargets ? {reworkTargets: without(n.reworkTargets, id)} : {}),
            ...(n.routing ? {
                routing: {
                    ...n.routing,
                    source: n.routing.source.fromStep === id ? {fromStep: '', outputKey: '', fieldPath: n.routing.source.fieldPath} : n.routing.source,
                    branches: n.routing.branches.filter((b) => b.targetStep !== id),
                    defaultTarget: clear(n.routing.defaultTarget),
                    joinStep: clear(n.routing.joinStep),
                },
            } : {}),
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

// descendants lists every node downstream of id.
export function descendants(spec: WorkflowSpec, id: string): Set<string> {
    const out = new Set<string>();
    const stack = [id];
    while (stack.length) {
        const cur = stack.pop()!;
        for (const n of spec.nodes) {
            if (n.dependsOn.includes(cur) && !out.has(n.id)) {
                out.add(n.id);
                stack.push(n.id);
            }
        }
    }
    return out;
}

// routingSources lists upstream JSON results a condition can branch on
// (only required json/report outputs are allowed).
export function routingSources(spec: WorkflowSpec, condition: string) {
    return availableInputs(spec, condition).filter((x) =>
        (x.output.type === 'json' || x.output.type === 'report') && x.output.required !== false);
}

// branchStarts are the nodes right after a condition: the possible
// branch targets (a join directly after it means an empty branch).
export function branchStarts(spec: WorkflowSpec, condition: string): WorkflowNode[] {
    return spec.nodes.filter((n) => n.dependsOn.includes(condition));
}

// joinCandidates are join nodes downstream of a condition.
export function joinCandidates(spec: WorkflowSpec, condition: string): WorkflowNode[] {
    const down = descendants(spec, condition);
    return spec.nodes.filter((n) => n.kind === 'join' && down.has(n.id));
}

// scaffoldBranch gives a condition a working shape: one branch task that
// runs when the rule matches, and a join that also takes the empty
// default path, so neither path can leave the join waiting (T09).
export function scaffoldBranch(spec: WorkflowSpec, condition: string): { spec: WorkflowSpec; branch: string; join: string } {
    const a = addNode(spec, 'task', condition);
    const branch = a.id;
    const withBranch = updateNode(a.spec, branch, {title: '조건이 맞을 때'});
    const j = addNode(withBranch, 'join');
    const join = j.id;
    const src = routingSources(j.spec, condition)[0];
    const routing: Routing = {
        source: {fromStep: src?.fromStep ?? '', outputKey: src?.output.key ?? '', fieldPath: ''},
        branches: [{operator: 'eq', value: true, targetStep: branch}],
        defaultTarget: join, joinStep: join,
    };
    let out = updateNode(j.spec, join, {dependsOn: [branch, condition]});
    out = updateNode(out, condition, {routing});
    return {spec: out, branch, join};
}

// branchLabel describes when a condition takes the path to target.
export function branchLabel(r: Routing | undefined, target: string): string | undefined {
    if (!r) return undefined;
    const labels = r.branches.filter((b) => b.targetStep === target).map(describeBranch);
    if (r.defaultTarget === target) labels.push('기본');
    return labels.length ? labels.join(' / ') : undefined;
}

export function describeBranch(b: Branch): string {
    if (b.operator === 'exists') return '값 있음';
    const v = typeof b.value === 'string' ? b.value : JSON.stringify(b.value);
    return {eq: '= ', ne: '≠ ', in: '∈ '}[b.operator] + v;
}

// parseBranchValue reads what a person typed: JSON (true, 3, "x", [..])
// when it parses, otherwise plain text; for `in`, comma separated text
// becomes a list.
export function parseBranchValue(op: Branch['operator'], text: string): unknown {
    const t = text.trim();
    let v: unknown;
    try {
        v = JSON.parse(t);
    } catch {
        v = t;
    }
    if (op === 'in' && !Array.isArray(v)) {
        return t === '' ? [] : t.split(',').map((x) => {
            try {
                return JSON.parse(x.trim());
            } catch {
                return x.trim();
            }
        });
    }
    return v;
}

export function formatBranchValue(v: unknown): string {
    if (v === undefined) return '';
    return typeof v === 'string' ? v : JSON.stringify(v);
}
