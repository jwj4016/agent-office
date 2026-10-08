// UI-side shapes of the Go JSON payloads. Written by hand because the
// generated wailsjs models type json.RawMessage fields as number[].

export type Project = {
    id: string; organizationId: string; name: string; goal: string; instructions: string;
    workspacePath: string; budget: unknown; mode: 'review' | 'auto'; status: 'active' | 'archived'; autoPolicy?: unknown;
    createdAt: string; updatedAt: string;
};

export type RunBrief = { id: string; title: string; status: RunStatus; startedAt: string };

export type ProjectSummary = {
    project: Project; activeRuns: number; waitingRuns: number; failedRuns: number;
    pausedRuns: number; inbox: number; lastRun: RunBrief | null;
};

export type Organization = { id: string; name: string; instructions: string; policy: unknown };

export type Role = {
    id: string; organizationId: string; parentRoleId: string; name: string; mission: string;
    instructions: string; outputDefaults: unknown; policy: unknown; appearance: unknown; updatedAt: string;
};

export type ActorKind = 'ai' | 'human';

export type Assignment = {
    id: string; projectId: string; roleId: string; actorKind: ActorKind; actorId: string;
    displayName: string; connectionId: string; model: string;
    overrides: { instructions?: string; appearance?: unknown };
};

export type ConnectionKind = 'test' | 'codex' | 'claude' | 'claude_api' | 'openai_api';

export type ConnectionSettings = {
    authMode?: 'api_key' | 'local_login'; node?: string; script?: string; baseUrl?: string;
    inputPerMTok?: number; outputPerMTok?: number;
};

export type Connection = {
    id: string; name: string; provider: ConnectionKind; executablePath: string; secretRef: string;
    config: unknown; verifiedCapabilities: unknown; usable: boolean; note: string; hasKey: boolean;
    settings: ConnectionSettings;
};

export type CheckStep = { id: string; label: string; status: 'ok' | 'failed' | 'skipped'; detail: string };
export type CheckReport = { steps: CheckStep[]; ready: boolean; version?: string };
export type DetectedPaths = { codex: string; node: string; bridge: string; claude: string };

export type InstructionLayer = { source: string; name: string; text: string };
export type InstructionPreview = { layers: InstructionLayer[]; composed: string };

export type NodeKind = 'task' | 'review' | 'approval' | 'condition' | 'join';
export type OutputType = 'markdown' | 'json' | 'file' | 'code_change' | 'report';

export type WorkflowInput = { name: string; fromStep: string; outputKey: string; required?: boolean };
export type WorkflowOutput = { key: string; type: OutputType; required?: boolean; schema?: unknown };
export type Branch = { operator: 'eq' | 'ne' | 'in' | 'exists'; value?: unknown; targetStep: string };
export type Routing = {
    source: { fromStep: string; outputKey: string; fieldPath: string };
    branches: Branch[]; defaultTarget: string; joinStep: string;
};

export type WorkflowNode = {
    id: string; title: string; kind: NodeKind; assignmentId?: string; dependsOn: string[];
    inputs?: WorkflowInput[]; instructions?: string; outputs?: WorkflowOutput[];
    completion?: { commands?: { executable: string; args?: string[]; timeout?: string }[] };
    reworkTargets?: string[]; limits?: Record<string, unknown>; routing?: Routing;
    meeting?: Meeting;
};

export type Meeting = { participants: string[]; maxRounds?: number };

export type WorkflowSpec = { schemaVersion: number; title: string; nodes: WorkflowNode[] };

export type Workflow = { id: string; projectId: string; title: string; draft: WorkflowSpec; revision: number; updatedAt: string };

export type Template = { id: string; title: string; description: string; workflow: WorkflowSpec };

export type Severity = 'error' | 'run';
export type Issue = { nodeId?: string; field?: string; code: string; message: string; severity: Severity };
export type Validation = { issues: Issue[]; canVersion: boolean; canRun: boolean };

export type WorkflowVersion = { id: string; workflowId: string; projectId: string; number: number; createdAt: string };
export type ConfirmResult = { version: WorkflowVersion | null; validation: Validation };
export type StartResult = { runId: string; issues: Issue[] };

export type RunStatus = 'running' | 'waiting' | 'paused' | 'succeeded' | 'failed' | 'interrupted' | 'cancelled';
export type StepStatus =
    'pending' | 'running' | 'verifying' | 'waiting_human' | 'waiting_approval' | 'waiting_input' |
    'succeeded' | 'skipped' | 'failed' | 'interrupted' | 'cancelled' | 'superseded';

export type ManifestEntry = {
    name: string; fromStep: string; outputKey: string; required: boolean;
    artifactId?: string; path?: string; hash?: string; type?: string;
};

export type ArtifactView = {
    id: string; outputKey: string; version: number; type: OutputType; path: string; hash: string;
    validity: 'valid' | 'stale' | 'invalid';
};

export type WorkspaceInfo = {
    kind: 'code' | 'view' | 'shared'; path: string; branch?: string; start?: string; commit?: string;
    status: string; conflicts: string[]; note?: string;
};
export type RepoInfo = { kind: 'base' | 'shared'; path: string; base?: string; note?: string };

export type StepView = {
    id: string; title: string; kind: NodeKind; status: StepStatus; assignmentId: string; attemptId: string;
    generation: number; attempt: number; round: number; error?: string; approvalId?: string;
    inputs?: ManifestEntry[] | null; artifacts: ArtifactView[]; workspace?: WorkspaceInfo;
};

export type RunDetail = {
    id: string; projectId: string; versionId: string; versionNumber: number; title: string;
    status: RunStatus; paused: boolean; steps: StepView[]; repo?: RepoInfo;
};

export type CodeChange = {
    baseCommit: string; startCommit: string; commit: string; branch: string;
    changes: { path: string; status: string; from?: string }[];
    diffStat: { files: number; insertions: number; deletions: number };
    patch: string; patchTruncated: boolean; summary?: string; tests?: unknown;
};

export type ArtifactContent = ArtifactView & {
    stepId: string; content: string; binary: boolean; truncated: boolean; hashOk: boolean;
};

export type InboxKind = 'task' | 'review' | 'approval' | 'tool_approval' | 'question' | 'budget';

export type InboxItem = {
    kind: InboxKind; projectId: string; projectName: string; runId: string; runTitle: string;
    versionNumber: number; stepId: string; stepTitle: string; instructions: string; attemptId: string;
    generation: number; attempt: number; round: number; approvalId?: string; messageId?: string;
    detail?: string; inputs: ManifestEntry[] | null; outputs: WorkflowOutput[];
    reworkTargets: { id: string; title: string }[]; since: string;
    context?: ContextNote[] | null; escalation?: string; workspace?: WorkspaceInfo;
};

export type ContextNote = { label: string; ref: string; text: string };

export type MessageKind = 'question' | 'answer' | 'review_request' | 'proposal' | 'decision' | 'handoff' | 'escalation';
export type ArtifactRef = { id: string; stepId?: string; outputKey: string; version: number; hash: string };
export type MessageView = {
    id: string; kind: MessageKind; sender: string; senderName: string; recipient: string; recipientName: string;
    stepId: string; body: string; replyTo?: string; createdAt: string;
    refs: { artifacts?: ArtifactRef[]; generation?: number; from?: string; note?: boolean; round?: number; agree?: boolean; reason?: string };
};

export type Outcome = { status: StepStatus | string; already: boolean; decision?: string };
export type ActionResult = { outcome: Outcome; problem: string };

export type StoredEvent = {
    sequence: number; id: string; projectId: string; runId?: string; stepAttemptId?: string;
    kind: string; payload: unknown; createdAt: string;
};

export type Delta = { projectId: string; runId: string; stepAttemptId: string; text: string };

export type SystemStatus = {
    dataDir: string; schemaVersion: number; secretsPersistent: boolean; version: string; error: string;
};

export type Budget = { maxTokens?: number; maxCostUsd?: number };
export type Usage = {
    inputTokens: number; outputTokens: number; costUsd: number; unknownCostAttempts: number; attempts: number;
    budget: Budget; holdReason?: string;
};

export type ProposedRole = { ref: string; reuseRoleId: string; name: string; mission: string; instructions: string; parentRef: string };
export type ProposedAssignment = {
    ref: string; roleRef: string; actorKind: 'ai' | 'human'; displayName: string; connectionId: string; model: string; instructions: string;
};
export type DesignDiff = {
    reuseRoles: { ref: string; id: string; name: string }[] | null;
    newRoles: ProposedRole[] | null;
    roleChanges: { roleId: string; instructions: string; reason: string; roleName: string; current: string }[] | null;
    assignments: (ProposedAssignment & { roleName: string; connectionName: string; connected: boolean })[] | null;
    steps: { id: string; title: string; kind: NodeKind; assignee: string; human: boolean }[] | null;
    missing: { kind: string; detail: string }[] | null;
};
export type DesignResult = {
    proposal: { project: { name: string; goal: string; instructions: string }; notes: string; workflow: WorkflowSpec };
    issues: Issue[]; diff: DesignDiff; canApply: boolean; canStart: boolean;
};
export type DesignStatus = 'drafting' | 'ready' | 'failed' | 'applied' | 'discarded';
export type DesignView = {
    id: string; projectId: string; goal: string; mode: 'review' | 'auto'; connectionId: string; status: DesignStatus; proposal?: unknown;
    error: string; createdAt: string; updatedAt: string; applied: { projectId?: string; workflowId?: string } | null;
    request: { goal: string; projectId: string; projectName: string; humanTasks: string[] | null; mode: string; model: string };
    result: DesignResult | null; autoIssues: Issue[];
};
export type ApplyResult = {
    applied: { projectId: string; workflowId: string; newProject: boolean } | null; issues: Issue[]; runId?: string; version?: number;
};
