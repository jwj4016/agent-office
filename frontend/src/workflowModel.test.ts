import {describe, expect, it} from 'vitest';
import type {WorkflowSpec} from './types';
import {addNode, ancestors, availableInputs, connect, disconnect, layout, removeNode, renameNode} from './workflowModel';

const base = (): WorkflowSpec => ({
    schemaVersion: 1, title: 't', nodes: [
        {id: 'plan', title: '기획', kind: 'task', dependsOn: [], outputs: [{key: 'spec', type: 'markdown'}]},
        {id: 'approve', title: '승인', kind: 'approval', dependsOn: ['plan'], reworkTargets: ['plan'],
            inputs: [{name: '기획서', fromStep: 'plan', outputKey: 'spec'}]},
        {id: 'dev', title: '개발', kind: 'task', dependsOn: ['approve'], outputs: [{key: 'change', type: 'code_change'}],
            inputs: [{name: '기획서', fromStep: 'plan', outputKey: 'spec'}]},
    ],
});

describe('workflowModel', () => {
    it('lays nodes out by depth', () => {
        const pos = layout(base());
        expect(pos.plan.x).toBe(0);
        expect(pos.approve.x).toBeGreaterThan(pos.plan.x);
        expect(pos.dev.x).toBeGreaterThan(pos.approve.x);
    });

    it('adds nodes with unique ids and sensible defaults', () => {
        const a = addNode(base(), 'review', 'dev');
        const b = addNode(a.spec, 'task');
        expect(a.id).not.toBe(b.id);
        const review = a.spec.nodes.find((n) => n.id === a.id)!;
        expect(review.dependsOn).toEqual(['dev']);
        expect(review.outputs?.[0].type).toBe('report');
        expect(addNode(base(), 'join').spec.nodes.at(-1)!.assignmentId).toBeUndefined();
    });

    it('refuses cycles and self links', () => {
        expect(connect(base(), 'dev', 'plan').error).toMatch(/순환/);
        expect(connect(base(), 'plan', 'plan').error).toBeTruthy();
        const ok = connect(base(), 'plan', 'dev');
        expect(ok.error).toBeUndefined();
        expect(ok.spec.nodes.find((n) => n.id === 'dev')!.dependsOn).toEqual(['approve', 'plan']);
    });

    it('removes a node and all references', () => {
        const s = removeNode(base(), 'plan');
        const approve = s.nodes.find((n) => n.id === 'approve')!;
        expect(approve.dependsOn).toEqual([]);
        expect(approve.inputs).toEqual([]);
        expect(approve.reworkTargets).toEqual([]);
    });

    it('keeps inputs that are still upstream after disconnecting', () => {
        const s = disconnect(connect(base(), 'plan', 'dev').spec, 'plan', 'dev');
        expect(s.nodes.find((n) => n.id === 'dev')!.inputs).toHaveLength(1); // still reachable via approve
        const cut = disconnect(base(), 'approve', 'dev');
        expect(cut.nodes.find((n) => n.id === 'dev')!.inputs).toEqual([]);
    });

    it('renames ids everywhere', () => {
        const {spec, error} = renameNode(base(), 'plan', 'planning');
        expect(error).toBeUndefined();
        expect(ancestors(spec, 'dev')).toEqual(new Set(['approve', 'planning']));
        expect(spec.nodes.find((n) => n.id === 'approve')!.reworkTargets).toEqual(['planning']);
        expect(renameNode(base(), 'plan', 'dev').error).toBeTruthy();
        expect(renameNode(base(), 'plan', '../x').error).toBeTruthy();
    });

    it('lists only upstream outputs as possible inputs', () => {
        expect(availableInputs(base(), 'dev').map((i) => i.fromStep)).toEqual(['plan']);
        expect(availableInputs(base(), 'plan')).toEqual([]);
    });
});
