import {describe, expect, it} from 'vitest';
import {blockedSteps, blockReason, canRetry, progress, runControls} from './runInsight';
import type {RunDetail, StepView} from './types';

const step = (id: string, status: StepView['status'], over: Partial<StepView> = {}): StepView => ({
    id, title: id, kind: 'task', status, assignmentId: 'a', attemptId: '', generation: 1, attempt: 1, round: 0, artifacts: [], ...over,
});
const run = (status: RunDetail['status'], steps: StepView[], paused = false): RunDetail =>
    ({id: 'r', projectId: 'p', versionId: 'v', versionNumber: 1, title: 'r', status, paused, steps});

describe('run insight', () => {
    it('finds the cancelled step behind a failed run and explains it without a recorded error', () => {
        const r = run('failed', [step('prd', 'cancelled'), step('data', 'pending'), step('route', 'cancelled', {kind: 'condition'})]);
        expect(blockedSteps(r).map((s) => s.id)).toEqual(['prd']);
        expect(blockReason(r.steps[0])).toEqual({text: expect.stringContaining('취소'), recorded: false});
        expect(canRetry(r, r.steps[0])).toBe(true);
    });

    it('prefers the recorded error', () => {
        expect(blockReason(step('dev', 'failed', {error: '검증 명령 실패'}))).toEqual({text: '검증 명령 실패', recorded: true});
    });

    it('offers no pause on a failed run and nothing on a finished one', () => {
        expect(runControls(run('failed', []))).toEqual({pause: false, resume: false, cancel: true});
        expect(runControls(run('running', []))).toEqual({pause: true, resume: false, cancel: true});
        expect(runControls(run('running', [], true))).toEqual({pause: false, resume: true, cancel: true});
        expect(runControls(run('succeeded', []))).toEqual({pause: false, resume: false, cancel: false});
    });

    it('counts finished steps, leaving out superseded attempts', () => {
        expect(progress(run('running', [step('a', 'succeeded'), step('b', 'skipped'), step('c', 'running'), step('d', 'superseded')])))
            .toEqual({done: 2, total: 3});
    });

    it('reports nothing blocked for a cancelled run', () => {
        expect(blockedSteps(run('cancelled', [step('a', 'cancelled')]))).toEqual([]);
    });
});
