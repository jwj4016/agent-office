// A controllable stand-in for ../api used by component tests.
import {vi} from 'vitest';
import type {ActionResult, InboxItem, ProjectSummary} from '../types';

export const state = {
    summaries: [] as ProjectSummary[],
    inbox: [] as InboxItem[],
    settings: {} as Record<string, string>,
};

const ok = <T, >(v: T) => vi.fn(async (..._args: unknown[]) => v);
const acted = (status: string) => vi.fn(async (..._args: unknown[]): Promise<ActionResult> => ({outcome: {status, already: false}, problem: ''}));

export const fakeApi = {
    systemStatus: vi.fn(async () => ({dataDir: '/data', schemaVersion: 2, secretsPersistent: true, version: 'test', error: ''})),
    settings: vi.fn(async () => ({...state.settings})),
    setSetting: vi.fn(async (k: string, v: string) => { state.settings[k] = v; }),
    dashboard: vi.fn(async () => state.summaries),
    createProject: vi.fn(async (p: { name: string; goal: string }) => {
        const project = {id: `prj-${state.summaries.length + 1}`, organizationId: 'org-default', name: p.name, goal: p.goal,
            instructions: '', workspacePath: '', budget: {}, mode: 'review' as const, status: 'active' as const, createdAt: '', updatedAt: ''};
        state.summaries.push({project, activeRuns: 0, waitingRuns: 0, failedRuns: 0, pausedRuns: 0, inbox: 0, lastRun: null});
        return project;
    }),
    inbox: vi.fn(async () => state.inbox),
    designs: vi.fn(async (): Promise<unknown[]> => []), design: vi.fn(), startDesign: vi.fn(), applyDesign: vi.fn(), discardDesign: ok(undefined), setAutoPolicy: ok(undefined),
    projectUsage: ok({inputTokens: 0, outputTokens: 0, costUsd: 0, unknownCostAttempts: 0, attempts: 0, budget: {}}),
    decideApproval: acted('succeeded'),
    submitReview: acted('succeeded'),
    submitHuman: acted('succeeded'),
    decideTool: acted('running'),
    answer: acted('running'),
    workflow: vi.fn(),
    saveDraft: vi.fn(),
    validate: ok({issues: [], canVersion: true, canRun: true}),
    confirmVersion: vi.fn(),
    startRun: vi.fn(),
    roles: ok([]), organization: ok({id: 'org-default', name: '내 회사', instructions: '', policy: {}}),
    assignments: ok([]), connections: vi.fn(async (): Promise<unknown[]> => []),
    setConnectionKey: ok({}), clearConnectionKey: ok({}), saveConnection: ok({}), detectPaths: ok({codex: '', node: '', bridge: '', claude: ''}),
    checkConnection: ok({steps: [], ready: false}),
    testConnectionCall: ok({steps: [{id: 'call', label: '실제 호출', status: 'ok', detail: 'pong'}], ready: true}), templates: ok([]), workflows: ok([]), runs: ok([]),
    onEvents: vi.fn(() => () => {}),
    onDeltas: vi.fn(() => () => {}),
};

export function inboxItem(over: Partial<InboxItem> = {}): InboxItem {
    return {
        kind: 'approval', projectId: 'prj-1', projectName: '게임', runId: 'run-1', runTitle: '서비스 개발', versionNumber: 1,
        stepId: 'approve', stepTitle: '기획 승인', instructions: '', attemptId: 'att-1', generation: 1, attempt: 1, round: 0,
        approvalId: 'apr-1', inputs: [], outputs: [], reworkTargets: [{id: 'plan', title: '기획'}], since: '2026-10-03T00:00:00Z',
        ...over,
    };
}
