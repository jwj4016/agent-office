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
export type OutputType = 'markdown' | 'json' | 'file' | 'code_change' | 'report' | 'diagram';

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
    id: string; title: string; kind: NodeKind; status: StepStatus; assignmentId: string; participants?: string[]; attemptId: string;
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
    stepId?: string; assignmentId?: string; attempt?: number;
};

export type Delta = { projectId: string; runId: string; stepAttemptId: string; text: string };

export type SystemStatus = {
    dataDir: string; schemaVersion: number; secretsPersistent: boolean; version: string; error: string;
    /** Started for performance measurement (AGENT_OFFICE_PERF). */
    perf?: boolean;
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

export type ExportResult = {
    path: string; projectName: string; tables: number; rows: number; artifacts: number; missing: string[]; masked: number;
};


// AI help on one workflow: an edit proposal or a pipeline diagram.
export type AssistKind = 'edit' | 'diagram';
export type AssistStatus = 'working' | 'ready' | 'failed' | 'applied' | 'discarded';
export type AssistRecord = {
    id: string; projectId: string; workflowId: string; kind: AssistKind; request: string; baseRevision: number;
    connectionId: string; model: string; status: AssistStatus; result: EditResult | DiagramResult | Record<string, never>;
    error: string; createdAt: string; updatedAt: string;
    baseWorkflow?: WorkflowSpec; parentId?: string; scopeNodeId?: string;
};
export type EditResult = {
    workflow: WorkflowSpec; notes: string; issues: Issue[] | null; canVersion: boolean;
    changes: { added: string[]; removed: string[]; changed: string[] };
};
export type DiagramResult = { path: string; checked: boolean };

// ---- departments and work (service operations) ----

export type ActiveStatus = 'active' | 'inactive';

// A company-wide starting point; copying it into a service makes an
// independent department.
export type DepartmentTemplate = {
    id: string; organizationId: string; name: string; responsibility: string; roleIds: string[]; updatedAt: string;
};

// A department inside one service. Members are the service's assignments.
export type Department = {
    id: string; projectId: string; name: string; responsibility: string; leadAssignmentId: string; memberIds: string[];
    templateId: string; automation: unknown; limits: unknown; status: ActiveStatus; createdAt: string; updatedAt: string;
};

export type WorkKind = 'goal' | 'work';
export type WorkPriority = 'urgent' | 'high' | 'normal' | 'low';
export type WorkStatus = 'received' | 'ready' | 'in_progress' | 'waiting' | 'blocked' | 'done' | 'cancelled';

export type WorkItem = {
    id: string; projectId: string; kind: WorkKind; parentId: string; departmentId: string; assigneeAssignmentId: string;
    title: string; description: string; doneCriteria: string; priority: WorkPriority; status: WorkStatus; statusReason: string;
    categories: string[]; dueAt: string; createdAt: string; updatedAt: string; closedAt: string;
    // reviewBeforeStart keeps a person's review before every start, even in auto mode.
    reviewBeforeStart: boolean;
};

export type WorkNote = {
    id: string; workItemId: string; kind: 'note' | 'status'; body: string; fromStatus?: string; toStatus?: string; author: string; createdAt: string;
};

export type WorkDetail = { item: WorkItem; notes: WorkNote[]; children: WorkItem[] };

export type WorkFilter = {
    kind?: string; status?: string; priority?: string; category?: string; departmentId?: string; parentId?: string; dueBefore?: string;
};

export type DeliveryStatus = 'pending' | 'processing' | 'waiting_answer' | 'done' | 'skipped' | 'held' | 'failed';
export type DeliveryPurpose = 'judge' | 'notify' | 'collab_request';

// The payload of work, document and department events.
export type WorkEventPayload = {
    departmentId: string; targetDepartmentId?: string; workItemId?: string; goalId?: string; title?: string; note?: string;
    categories: string[]; causeSequence: number | null; chainRootSequence: number | null; chainDepth: number; automated: boolean;
    documentId?: string; documentVersionId?: string;
};

export type Delivery = {
    id: string; projectId: string; eventSequence: number; eventKind: string; event: WorkEventPayload; eventAt: string;
    departmentId: string; subscriptionId: string; purpose: DeliveryPurpose; status: DeliveryStatus; note: string; error: string;
    holdReason: string; createdAt: string; updatedAt: string;
};

export type WorkEventView = { sequence: number; kind: string; event: WorkEventPayload; createdAt: string; deliveries: Delivery[] };

export type Subscription = {
    id: string; projectId: string; departmentId: string; eventKinds: string[]; categories: string[]; guidance: string; enabled: boolean;
    updatedAt: string;
};

export type DeliveryFilter = { departmentId?: string; status?: string; limit?: number };

// How one work item is done: who, extra instructions, materials, outputs.
// With workflowVersionId the work runs that confirmed workflow version as it is.
export type WorkPlan = {
    workItemId: string; projectId: string; assignmentId: string; instructions: string; materials: string;
    outputs: WorkflowOutput[]; workflowVersionId: string; updatedAt: string;
};

// The approval the last start was made under; valid while nothing changed since.
export type StartApproval = { by: '' | 'user' | 'auto'; at: string; valid: boolean };

export type AutoStartStatus = 'pending' | 'planning' | 'approved' | 'started' | 'finished' | 'review' | 'cancelled';

// The auto starter's record of one work item made from an event in an auto-mode service.
export type AutoStart = {
    id: string; projectId: string; projectName: string; workItemId: string; workTitle: string; departmentId: string; causeSequence: number;
    status: AutoStartStatus; planJobId: string; runId: string; reason: string; note: string; createdAt: string; updatedAt: string;
};

export type WorkflowChoice = { versionId: string; workflowId: string; title: string; number: number; createdAt: string };

// What a department lets happen without a person. Empty categories allow every
// category, empty connectionIds add no limit, empty outputTypes mean documents only.
export type DepartmentAutomation = {
    autoStart: boolean; categories: string[]; connectionIds: string[]; outputTypes: OutputType[]; workflowVersionIds: string[];
    autoShare: boolean; autoCollab: boolean;
};

// saved is false when no plan is stored (plan is then a starting point);
// preview is what a run would be told; problem says why it cannot run.
export type WorkPlanView = { plan: WorkPlan; saved: boolean; preview: string; problem: string; approval: StartApproval; autoStart?: AutoStart };

export type AIJobStatus = 'pending' | 'working' | 'waiting_answer' | 'ready' | 'held' | 'failed';

// One AI request made outside a run (a work plan proposal for now).
export type AIJob = {
    id: string; projectId: string; kind: string; lane: 'interactive' | 'background'; requestKey: string; workItemId: string;
    assignmentId: string; status: AIJobStatus; error: string; holdReason: string; maxAttempts: number; attempts: number;
    appliedAt: string; createdAt: string; updatedAt: string;
};

export type ProposedPlan = { assignmentId: string; instructions: string; materials: string; outputs: WorkflowOutput[]; notes: string };

// stale: the work changed after the proposal was made, so it cannot apply.
export type PlanJobView = { job: AIJob; proposal?: ProposedPlan; stale: boolean };

// ---- shared documents ----

export type DocumentVersion = {
    id: string; documentId: string; projectId: string; number: number; stage: 'draft' | 'final'; confirmedBy: string; confirmedAt: string;
    confirmationNote: string; sourceKind: 'artifact' | 'workspace'; sourceArtifactId: string; sourcePath: string; fileName: string; path: string;
    hash: string; size: number; withdrawnAt: string; createdAt: string;
};

export type SharedDocument = {
    id: string; projectId: string; title: string; categories: string[]; ownerDepartmentId: string; workItemId: string;
    scope: 'service' | 'departments'; scopeDepartments: string[]; status: 'active' | 'withdrawn'; createdAt: string; updatedAt: string;
    versions: DocumentVersion[];
};

// With documentId it is a new version of that document; otherwise a new
// document is made from the other fields.
export type NewDocumentVersion = {
    documentId?: string; title?: string; categories?: string[]; ownerDepartmentId?: string; workItemId?: string;
    scope?: 'service' | 'departments'; scopeDepartments?: string[];
};

export type DocumentFilter = {
    query?: string; category?: string; ownerDepartmentId?: string; visibleTo?: string; stage?: string; includeWithdrawn?: boolean;
};

export type LinkedDocument = { version: DocumentVersion; document: SharedDocument; newer: number; linkedAt: string };

export type ImpactCheck = {
    id: string; projectId: string; workItemId: string; cause: 'revised' | 'withdrawn' | 'required_reopened'; refId: string; documentId: string;
    summary: string; status: 'open' | 'resolved' | 'dismissed'; note: string; createdAt: string;
};

export type DocumentContent = DocumentVersion & { title: string; content: string; binary: boolean; truncated: boolean; hashOk: boolean };

// ---- department judgments ----

export type JudgmentOutcome = 'none' | 'propose' | 'question';

// questionStatus: '' without a question; open until answered; answered once
// the department judges again; manual when it asked again and a person
// decides; closed when the person closed it.
export type Judgment = {
    id: string; projectId: string; projectName: string; deliveryId: string; round: number; aiJobId: string; departmentId: string;
    departmentName: string; outcome: JudgmentOutcome; reason: string; evidence: string[]; question: string; answer: string;
    questionStatus: '' | 'open' | 'answered' | 'manual' | 'closed'; eventSequence: number; eventKind: string; event: WorkEventPayload;
    createdAt: string;
};

// duplicateOf names an open work item or proposal with the same title.
export type Proposal = {
    id: string; projectId: string; projectName: string; judgmentId: string; position: number; departmentId: string; departmentName: string;
    title: string; description: string; doneCriteria: string; categories: string[]; priority: WorkPriority;
    status: 'proposed' | 'accepted' | 'rejected'; workItemId: string; duplicateOf: string; decisionNote: string; reason: string;
    eventSequence: number; eventKind: string; eventTitle: string; createdAt: string;
};

export type ProposalEdit = { title?: string; description?: string; doneCriteria?: string; categories?: string[]; priority?: WorkPriority };

export type ProposalBatch = { done: number; errors: string[] };

// ---- collaboration requests ----

export type CollabStatus = 'requested' | 'question' | 'accepted' | 'declined' | 'cancelled';
export type CollabRequirement = 'reference' | 'required';
export type CollabAction = 'requested' | 'asked' | 'answered' | 'accepted' | 'declined' | 'cancelled' | 'required' | 'reference' | 'replaced' | 'reverted';

// asDepartmentId is the department the person acted for ('' for the person
// in charge of the asking work).
export type CollabHistory = { action: CollabAction; body: string; asDepartmentId: string; author: string; createdAt: string };

// One department asking another for help on a work item. satisfied: accepted
// and its work done; a required request that is not satisfied blocks
// completing the asking work and its goal.
export type CollabRequest = {
    id: string; projectId: string; projectName: string; fromWorkItemId: string; fromWorkTitle: string; fromWorkStatus: WorkStatus; goalId: string;
    fromDepartmentId: string; fromDepartmentName: string; toDepartmentId: string; toDepartmentName: string; question: string;
    inputVersionIds: string[]; requirement: CollabRequirement; requirementReason: string; status: CollabStatus; followUp: string;
    decidedBy: '' | 'user' | 'ai'; decisionJobId: string; decisionReason: string; decidedAt: string; reviewedAt: string; revertedAt: string;
    revertReason: string; resultWorkItemId: string; resultWorkTitle: string;
    resultWorkStatus: WorkStatus | ''; replacesId: string; replacedById: string; dueAt: string; eventSequence: number;
    satisfied: boolean; overdue: boolean; history: CollabHistory[]; createdAt: string; updatedAt: string;
};

export type NewCollabRequest = {
    fromWorkItemId?: string; toDepartmentId: string; question: string; inputVersionIds?: string[]; requirement?: CollabRequirement;
    requirementReason?: string; dueAt?: string;
};

export type CollabAcceptance = {
    existingWorkItemId?: string; title?: string; description?: string; doneCriteria?: string; priority?: WorkPriority; note?: string;
};

// A work item's collaboration: what it (or, for a goal, its work) asked,
// the request it is the work of, what keeps it from completing, and checks
// about required work that changed.
export type WorkCollab = { requests: CollabRequest[]; incoming: CollabRequest[]; blockers: CollabRequest[]; checks: ImpactCheck[] };

export type RequiredNotice = ImpactCheck & { projectName: string; workTitle: string; workKind: WorkKind };

// aiDecisions: collaboration the lead AIs decided that the person has not looked at;
// autoReviews: automatic starts that need the person.
export type Decisions = {
    questions: Judgment[]; proposals: Proposal[]; collabs: CollabRequest[]; notices: RequiredNotice[]; aiDecisions: CollabRequest[];
    autoReviews: AutoStart[];
};
