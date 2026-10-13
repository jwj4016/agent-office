import {describe as group, expect, it} from 'vitest';
import {inboxItem} from '../test/fakeApi';
import type {Assignment, RunDetail, StepView} from '../types';
import {buildOffice, describe} from './model';

const asg = (id: string, kind: 'ai' | 'human' = 'ai'): Assignment => ({
    id, projectId: 'prj-1', roleId: `role-${id}`, actorKind: kind, actorId: kind === 'human' ? 'local-owner' : '',
    displayName: id, connectionId: '', model: '', overrides: {},
});

const step = (id: string, assignmentId: string, status: StepView['status'], over: Partial<StepView> = {}): StepView => ({
    id, title: id, kind: 'task', status, assignmentId, attemptId: `att-${id}`, generation: 1, attempt: 1, round: 0, artifacts: [], ...over,
});

const run = (steps: StepView[], status: RunDetail['status'] = 'running', id = 'run-1'): RunDetail => ({
    id, projectId: 'prj-1', versionId: 'v1', versionNumber: 1, title: '서비스 개발', status, paused: false, steps,
});

const team = [asg('plan'), asg('dev'), asg('qa'), asg('me', 'human')];
const roles = team.map((a) => ({id: a.roleId, organizationId: 'o', parentRoleId: '', name: `역할 ${a.id}`, mission: '', instructions: '',
    outputDefaults: {}, policy: {}, appearance: {}, updatedAt: ''}));

const stateOf = (o: ReturnType<typeof buildOffice>) => Object.fromEntries(o.desks.map((d) => [d.assignment.id, d.state]));

group('buildOffice', () => {
    it('maps each real step status to a desk state', () => {
        const o = buildOffice(team, roles, [run([
            step('plan', 'plan', 'succeeded'), step('dev', 'dev', 'running'), step('qa', 'qa', 'verifying'), step('review', 'me', 'waiting_human'),
        ])], []);
        expect(stateOf(o)).toEqual({plan: 'done', dev: 'working', qa: 'checking', me: 'waiting'});
        expect(o.desks.find((d) => d.assignment.id === 'me')!.label).toBe('내 할 일 있음');
        expect(o.desks.find((d) => d.assignment.id === 'dev')!.roleName).toBe('역할 dev');
    });

    it('shows nobody busy when no run is active', () => {
        const o = buildOffice(team, roles, [run([step('dev', 'dev', 'succeeded')], 'succeeded'), run([step('dev', 'dev', 'running')], 'cancelled', 'r2')], []);
        expect(new Set(Object.values(stateOf(o)))).toEqual(new Set(['idle']));
    });

    it('seats meeting participants with the decider', () => {
        const o = buildOffice(team, roles, [run([step('design', 'plan', 'running', {participants: ['dev', 'qa']})])], []);
        expect(stateOf(o)).toMatchObject({plan: 'meeting', dev: 'meeting', qa: 'meeting', me: 'idle'});
        expect(o.meetings).toEqual([{runId: 'run-1', stepId: 'design', title: 'design', members: ['plan', 'dev', 'qa']}]);
        expect(o.desks.find((d) => d.assignment.id === 'dev')!.work[0].participant).toBe(true);
    });

    it('marks an AI blocked on a tool permission as waiting for a person', () => {
        const o = buildOffice(team, roles, [run([step('dev', 'dev', 'running')])],
            [inboxItem({kind: 'tool_approval', runId: 'run-1', stepId: 'dev'})]);
        const dev = o.desks.find((d) => d.assignment.id === 'dev')!;
        expect(dev.state).toBe('waiting');
        expect(dev.label).toBe('사람 응답 대기');
        expect(dev.work[0].inbox).toEqual(['tool_approval']);
    });

    it('puts a failure first and offers a retry', () => {
        const o = buildOffice(team, roles, [
            run([step('dev', 'dev', 'running')], 'running', 'r1'),
            run([step('fix', 'dev', 'failed', {error: '검증 실패'})], 'failed', 'r2'),
        ], []);
        const dev = o.desks.find((d) => d.assignment.id === 'dev')!;
        expect(dev.state).toBe('problem');
        expect(dev.work.map((w) => w.stepId)).toEqual(['fix', 'dev']);
        expect(dev.work[0].canRetry).toBe(true);
        expect(describe(dev)).toBe('dev (AI, 역할 dev): 문제 발생 — fix · 서비스 개발');
    });

    it('ignores pending, skipped and superseded steps', () => {
        const o = buildOffice(team, roles, [run([step('plan', 'plan', 'pending'), step('dev', 'dev', 'skipped'), step('qa', 'qa', 'superseded')])], []);
        expect(o.desks.every((d) => d.state === 'idle' && d.work.length === 0)).toBe(true);
    });
});
