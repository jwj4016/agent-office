// A controllable stand-in for ../api used by component tests.
import {vi} from 'vitest';
import type {ActionResult, InboxItem, ProjectSummary, WorkPlanView} from '../types';

export const state = {
    summaries: [] as ProjectSummary[],
    inbox: [] as InboxItem[],
    settings: {} as Record<string, string>,
};

const ok = <T, >(v: T) => vi.fn(async (..._args: unknown[]) => v);
const acted = (status: string) => vi.fn(async (..._args: unknown[]): Promise<ActionResult> => ({outcome: {status, already: false}, problem: ''}));

export const fakeApi = {
    systemStatus: vi.fn(async () => ({dataDir: '/data', schemaVersion: 2, secretsPersistent: true, version: 'test', error: ''})),
    perfMark: vi.fn(async (_n: string, _d: string) => {}),
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
    designs: vi.fn(async (): Promise<unknown[]> => []), design: vi.fn(), startDesign: vi.fn(), applyDesign: vi.fn(), discardDesign: ok(undefined), reparseDesign: vi.fn(), setAutoPolicy: ok(undefined),
    exportProject: vi.fn(),
    chooseFolder: vi.fn(async (..._args: unknown[]) => ''),
    startAssist: vi.fn(), assists: vi.fn(async (..._args: unknown[]): Promise<unknown[]> => []), assist: vi.fn(), applyAssist: vi.fn(),
    aiActivity: vi.fn(async (..._args: unknown[]): Promise<{events: unknown[]; hasMore: boolean}> => ({events: [], hasMore: false})),
    discardAssist: ok(undefined), workflowDiagram: vi.fn(async (..._args: unknown[]) => ''), saveWorkflowDiagram: vi.fn(async (..._args: unknown[]) => ''),
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
    roles: vi.fn(async (): Promise<unknown[]> => []), organization: ok({id: 'org-default', name: '내 회사', instructions: '', policy: {}}),
    assignments: vi.fn(async (..._args: unknown[]): Promise<unknown[]> => []), connections: vi.fn(async (): Promise<unknown[]> => []),
    setConnectionKey: ok({}), clearConnectionKey: ok({}), saveConnection: ok({}), detectPaths: ok({codex: '', node: '', bridge: '', claude: ''}),
    checkConnection: ok({steps: [], ready: false}),
    testConnectionCall: ok({steps: [{id: 'call', label: '실제 호출', status: 'ok', detail: 'pong'}], ready: true}), templates: ok([]), workflows: ok([]), runs: vi.fn(async (..._args: unknown[]): Promise<unknown[]> => []),
    run: vi.fn(), runMessages: vi.fn(async (..._args: unknown[]): Promise<unknown[]> => []), postNote: ok('msg-new'),
    pauseRun: ok(undefined), resumeRun: ok(undefined), cancelRun: ok(undefined), retryStep: ok(undefined), readArtifact: vi.fn(),
    departmentTemplates: vi.fn(async (): Promise<unknown[]> => []), saveDepartmentTemplate: vi.fn(), deleteDepartmentTemplate: ok(undefined),
    departments: vi.fn(async (..._args: unknown[]): Promise<unknown[]> => []), saveDepartment: vi.fn(),
    importDepartmentTemplate: vi.fn(), saveDepartmentAsTemplate: vi.fn(),
    categories: vi.fn(async (..._args: unknown[]): Promise<string[]> => []), setCategories: vi.fn(async (_p: string, names: string[]) => names),
    workItems: vi.fn(async (..._args: unknown[]): Promise<unknown[]> => []), workItem: vi.fn(), createWorkItem: vi.fn(), updateWorkItem: vi.fn(),
    setWorkStatus: vi.fn(), addWorkNote: vi.fn(),
    workEvents: vi.fn(async (..._args: unknown[]): Promise<unknown[]> => []), subscriptions: vi.fn(async (..._args: unknown[]): Promise<unknown[]> => []),
    saveSubscription: vi.fn(), saveArtifactAs: vi.fn(), revealArtifact: vi.fn(), schedules: vi.fn(async (..._args: unknown[]): Promise<unknown[]> => []), saveSchedule: vi.fn(), deliveries: vi.fn(async (..._args: unknown[]): Promise<unknown[]> => []),
    requestDepartmentCheck: ok(undefined), retryDelivery: ok(undefined), skipDelivery: ok(undefined),
    judgments: vi.fn(async (..._args: unknown[]): Promise<unknown[]> => []), answerJudgment: ok(undefined), closeJudgmentQuestion: ok(undefined),
    proposals: vi.fn(async (..._args: unknown[]): Promise<unknown[]> => []), acceptProposal: vi.fn(), rejectProposal: ok(undefined),
    decideProposals: vi.fn(async (..._args: unknown[]) => ({done: 0, errors: [] as string[]})),
    pendingDecisions: vi.fn(async (): Promise<{questions: unknown[]; proposals: unknown[]; collabs: unknown[]; notices: unknown[];
        aiDecisions: unknown[]; autoReviews: unknown[]; results: unknown[]; goals: Record<string, unknown>}> =>
        ({questions: [], proposals: [], collabs: [], notices: [], aiDecisions: [], autoReviews: [], results: [], goals: {}})),
    workLineage: vi.fn(async (..._args: unknown[]): Promise<unknown> => ({causes: [], next: []})),
    operations: vi.fn(async (..._args: unknown[]): Promise<unknown> => ({departments: [], troubles: [], troubleCount: 0,
        usage: {inputTokens: 0, outputTokens: 0, costUsd: 0, unknownCostAttempts: 0, attempts: 0, budget: {}}})),
    reviewCollabDecision: vi.fn(), revertCollabDecision: vi.fn(), workflowChoices: vi.fn(async (..._args: unknown[]): Promise<unknown[]> => []),
    setDepartmentAutomation: vi.fn(), autoStarts: vi.fn(async (..._args: unknown[]): Promise<unknown[]> => []),
    retryAutoStart: ok(undefined), dismissAutoStart: ok(undefined),
    workCollab: vi.fn(async (..._args: unknown[]): Promise<{requests: unknown[]; incoming: unknown[]; blockers: unknown[]; checks: unknown[]}> =>
        ({requests: [], incoming: [], blockers: [], checks: []})),
    collabRequests: vi.fn(async (..._args: unknown[]): Promise<unknown[]> => []),
    requestCollab: vi.fn(), acceptCollab: vi.fn(), askCollab: vi.fn(), answerCollab: vi.fn(), declineCollab: vi.fn(), cancelCollab: vi.fn(),
    setCollabRequirement: vi.fn(), replaceCollab: vi.fn(),
    requestDocumentReview: vi.fn(async (..._args: unknown[]) => 0),
    workPlan: vi.fn(async (..._args: unknown[]): Promise<WorkPlanView> => ({
        plan: {workItemId: '', projectId: '', assignmentId: '', instructions: '', materials: '', outputs: [{key: 'result', type: 'markdown'}],
            workflowVersionId: '', updatedAt: ''},
        saved: false, preview: '', problem: '', approval: {by: '', at: '', valid: false},
    })),
    saveWorkPlan: vi.fn(), startWork: vi.fn(),
    documents: vi.fn(async (..._args: unknown[]): Promise<unknown[]> => []), document: vi.fn(), readDocumentVersion: vi.fn(),
    shareArtifact: vi.fn(), shareWorkspaceFile: vi.fn(), confirmDocumentVersion: vi.fn(), withdrawDocument: ok(undefined),
    linkDocument: ok(undefined), unlinkDocument: ok(undefined), workDocuments: vi.fn(async (..._args: unknown[]): Promise<unknown[]> => []),
    runDocuments: vi.fn(async (..._args: unknown[]): Promise<unknown[]> => []),
    impactChecks: vi.fn(async (..._args: unknown[]): Promise<unknown[]> => []), closeImpactCheck: ok(undefined),
    requestWorkPlan: vi.fn(), planJobs: vi.fn(async (..._args: unknown[]): Promise<unknown[]> => []), applyPlanJob: vi.fn(), retryAIJob: ok(undefined), workRuns: vi.fn(async (..._args: unknown[]): Promise<unknown[]> => []),
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
