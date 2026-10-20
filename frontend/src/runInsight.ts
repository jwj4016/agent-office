// What a person needs to know about a run at a glance: how far it got,
// what is happening now and, when it stopped, which step stopped it and
// what can be done. Derived from stored step states only.
import type {RunDetail, RunStatus, StepView} from './types';

const finishedStep = new Set(['succeeded', 'skipped', 'superseded']);
const engineStep = (s: StepView) => s.kind === 'condition' || s.kind === 'join';

export function progress(run: RunDetail): { done: number; total: number } {
    const steps = run.steps.filter((s) => s.status !== 'superseded');
    return {done: steps.filter((s) => finishedStep.has(s.status)).length, total: steps.length};
}

// currentSteps are the steps being worked on or waiting for a person.
export function currentSteps(run: RunDetail): StepView[] {
    return run.steps.filter((s) => ['running', 'verifying', 'waiting_human', 'waiting_approval', 'waiting_input'].includes(s.status));
}

export const runEnded = (s: RunStatus) => s === 'succeeded' || s === 'cancelled';
export const runStopped = (s: RunStatus) => s === 'failed' || s === 'interrupted';

// blockedSteps are the steps that stopped a failed or interrupted run. A
// step cancelled inside a run that then failed counts too: it is often the
// only trace left of why the run stopped.
export function blockedSteps(run: RunDetail): StepView[] {
    if (run.status === 'cancelled' || run.status === 'succeeded') return [];
    return run.steps.filter((s) => !engineStep(s) && (s.status === 'failed' || s.status === 'interrupted'
        || (s.status === 'cancelled' && runStopped(run.status))));
}

// blockReason says why a step stopped, in words, even when the engine did
// not record a message.
export function blockReason(s: StepView): { text: string; recorded: boolean } {
    if (s.error) return {text: s.error, recorded: true};
    switch (s.status) {
        case 'interrupted':
            return {text: '앱이 종료되었거나 연결이 끊겨 작업이 중단되었습니다.', recorded: false};
        case 'cancelled':
            return {text: '이 업무가 취소되어 실행이 멈췄습니다. 취소 사유는 기록되지 않았습니다.', recorded: false};
        default:
            return {text: '실패 원인이 기록되지 않았습니다.', recorded: false};
    }
}

export const canRetry = (run: RunDetail, s: StepView) =>
    run.status !== 'cancelled' && ['failed', 'interrupted', 'cancelled'].includes(s.status);

// runControls lists the run-level actions that make sense in its state.
export function runControls(run: RunDetail): { pause: boolean; resume: boolean; cancel: boolean } {
    if (runEnded(run.status)) return {pause: false, resume: false, cancel: false};
    return {
        pause: !run.paused && (run.status === 'running' || run.status === 'waiting'),
        resume: run.paused,
        cancel: true,
    };
}
