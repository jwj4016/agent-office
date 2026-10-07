// Typed access to the Go bindings. Every call goes through here so tests
// can mock one module.
import * as Go from '../wailsjs/go/main/App';
import {EventsOn} from '../wailsjs/runtime/runtime';
import type {
    ActionResult, ArtifactContent, Assignment, CheckReport, ConfirmResult, Connection, ConnectionKind, ConnectionSettings, Delta,
    DetectedPaths, Usage, InboxItem, InstructionPreview,
    Organization, Project, ProjectSummary, Role, RunDetail, StartResult, StoredEvent, SystemStatus, Template,
    Validation, Workflow, WorkflowSpec,
} from './types';

// The generated d.ts types differ from the real JSON for raw fields, so
// results are re-typed here.
const call = <T>(p: Promise<unknown>) => p as Promise<T>;

export type ProjectInput = {
    id?: string; name: string; goal: string; instructions: string; mode: string; budget?: { maxTokens?: number; maxCostUsd?: number };
};

export const api = {
    systemStatus: () => call<SystemStatus>(Go.SystemStatus()),
    settings: () => call<Record<string, string>>(Go.GetSettings()),
    setSetting: (k: string, v: string) => call<void>(Go.SetSetting(k, v)),

    dashboard: (includeArchived = false) => call<ProjectSummary[]>(Go.Dashboard(includeArchived)),
    createProject: (p: ProjectInput) => call<Project>(Go.CreateProject(p as never)),
    updateProject: (p: ProjectInput) => call<Project>(Go.UpdateProject(p as never)),
    setArchived: (id: string, archived: boolean) => call<void>(Go.SetProjectArchived(id, archived)),
    projectUsage: (id: string) => call<Usage>(Go.ProjectUsage(id)),

    organization: () => call<Organization>(Go.GetOrganization()),
    updateOrganization: (name: string, instructions: string) => call<void>(Go.UpdateOrganization(name, instructions)),
    roles: () => call<Role[]>(Go.ListRoles()),
    saveRole: (r: Partial<Role>) => call<Role>(Go.SaveRole(r as never)),
    deleteRole: (id: string) => call<void>(Go.DeleteRole(id)),

    assignments: (projectId: string) => call<Assignment[]>(Go.ListAssignments(projectId)),
    saveAssignment: (a: Partial<Assignment>) => call<Assignment>(Go.SaveAssignment(a as never)),
    instructionPreview: (projectId: string, assignmentId: string) =>
        call<InstructionPreview>(Go.InstructionPreview(projectId, assignmentId)),
    connections: () => call<Connection[]>(Go.ListConnections()),
    ensureTestConnection: () => call<Connection>(Go.EnsureTestConnection()),
    saveConnection: (c: { id?: string; name: string; provider: ConnectionKind; executablePath: string; settings: ConnectionSettings }) =>
        call<Connection>(Go.SaveConnection(c as never)),
    setConnectionKey: (id: string, key: string) => call<Connection>(Go.SetConnectionKey(id, key)),
    clearConnectionKey: (id: string) => call<Connection>(Go.ClearConnectionKey(id)),
    checkConnection: (id: string) => call<CheckReport>(Go.CheckConnection(id)),
    testConnectionCall: (id: string, model: string) => call<CheckReport>(Go.TestConnectionCall(id, model)),
    detectPaths: () => call<DetectedPaths>(Go.DetectPaths()),

    templates: () => call<Template[]>(Go.WorkflowTemplates()),
    workflows: (projectId: string) => call<Workflow[]>(Go.ListWorkflows(projectId)),
    workflow: (projectId: string, id: string) => call<Workflow>(Go.GetWorkflow(projectId, id)),
    createWorkflow: (projectId: string, templateId: string, title: string) =>
        call<Workflow>(Go.CreateWorkflow(projectId, templateId, title)),
    saveDraft: (projectId: string, id: string, revision: number, draft: WorkflowSpec) =>
        call<Workflow>(Go.SaveWorkflowDraft(projectId, id, revision, JSON.stringify(draft))),
    validate: (projectId: string, id: string) => call<Validation>(Go.ValidateWorkflow(projectId, id)),
    confirmVersion: (projectId: string, id: string, revision: number) =>
        call<ConfirmResult>(Go.ConfirmVersion(projectId, id, revision)),

    startRun: (projectId: string, versionId: string) => call<StartResult>(Go.StartRun(projectId, versionId)),
    runs: (projectId: string) => call<RunDetail[]>(Go.ListRuns(projectId)),
    run: (projectId: string, runId: string) => call<RunDetail>(Go.GetRun(projectId, runId)),
    pauseRun: (projectId: string, runId: string) => call<void>(Go.PauseRun(projectId, runId)),
    resumeRun: (projectId: string, runId: string) => call<void>(Go.ResumeRun(projectId, runId)),
    cancelRun: (projectId: string, runId: string) => call<void>(Go.CancelRun(projectId, runId)),
    retryStep: (projectId: string, runId: string, stepId: string) => call<void>(Go.RetryStep(projectId, runId, stepId)),
    readArtifact: (projectId: string, artifactId: string) => call<ArtifactContent>(Go.ReadArtifact(projectId, artifactId)),

    inbox: () => call<InboxItem[]>(Go.Inbox()),
    submitHuman: (projectId: string, attemptId: string, generation: number, outputs: Record<string, string>) =>
        call<ActionResult>(Go.SubmitHumanResult(projectId, attemptId, generation, outputs)),
    submitReview: (projectId: string, attemptId: string, generation: number, decision: string, comment: string,
                   targets: string[], outputs: Record<string, string> = {}) =>
        call<ActionResult>(Go.SubmitReview(projectId, attemptId, generation, decision, comment, targets, outputs)),
    decideApproval: (projectId: string, approvalId: string, generation: number, decision: string, reason: string, targets: string[]) =>
        call<ActionResult>(Go.DecideApproval(projectId, approvalId, generation, decision, reason, targets)),
    decideTool: (projectId: string, approvalId: string, accept: boolean) =>
        call<ActionResult>(Go.DecideToolApproval(projectId, approvalId, accept)),
    answer: (projectId: string, messageId: string, answer: string) =>
        call<ActionResult>(Go.AnswerQuestion(projectId, messageId, answer)),
    eventsAfter: (projectId: string, after: number) => call<StoredEvent[]>(Go.EventsAfter(projectId, after)),

    onEvents: (cb: (evs: StoredEvent[]) => void) => EventsOn('events', cb),
    onDeltas: (cb: (ds: Delta[]) => void) => EventsOn('deltas', cb),
};

export function errorText(e: unknown): string {
    if (typeof e === 'string') return e;
    if (e instanceof Error) return e.message;
    return String(e);
}
