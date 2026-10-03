export namespace domain {
	
	export class AssignmentOverrides {
	    instructions?: string;
	    appearance?: number[];
	
	    static createFrom(source: any = {}) {
	        return new AssignmentOverrides(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.instructions = source["instructions"];
	        this.appearance = source["appearance"];
	    }
	}
	export class Assignment {
	    id: string;
	    projectId: string;
	    roleId: string;
	    actorKind: string;
	    actorId: string;
	    displayName: string;
	    connectionId: string;
	    model: string;
	    overrides: AssignmentOverrides;
	
	    static createFrom(source: any = {}) {
	        return new Assignment(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.projectId = source["projectId"];
	        this.roleId = source["roleId"];
	        this.actorKind = source["actorKind"];
	        this.actorId = source["actorId"];
	        this.displayName = source["displayName"];
	        this.connectionId = source["connectionId"];
	        this.model = source["model"];
	        this.overrides = this.convertValues(source["overrides"], AssignmentOverrides);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class InstructionLayer {
	    source: string;
	    name: string;
	    text: string;
	
	    static createFrom(source: any = {}) {
	        return new InstructionLayer(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.source = source["source"];
	        this.name = source["name"];
	        this.text = source["text"];
	    }
	}
	export class AssignmentSnapshot {
	    id: string;
	    projectId: string;
	    roleId: string;
	    actorKind: string;
	    actorId: string;
	    displayName: string;
	    connectionId: string;
	    model: string;
	    overrides: AssignmentOverrides;
	    roleName: string;
	    instructions: InstructionLayer[];
	    connected: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AssignmentSnapshot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.projectId = source["projectId"];
	        this.roleId = source["roleId"];
	        this.actorKind = source["actorKind"];
	        this.actorId = source["actorId"];
	        this.displayName = source["displayName"];
	        this.connectionId = source["connectionId"];
	        this.model = source["model"];
	        this.overrides = this.convertValues(source["overrides"], AssignmentOverrides);
	        this.roleName = source["roleName"];
	        this.instructions = this.convertValues(source["instructions"], InstructionLayer);
	        this.connected = source["connected"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Branch {
	    operator: string;
	    value?: number[];
	    targetStep: string;
	
	    static createFrom(source: any = {}) {
	        return new Branch(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.operator = source["operator"];
	        this.value = source["value"];
	        this.targetStep = source["targetStep"];
	    }
	}
	export class VerifyCommand {
	    executable: string;
	    args?: string[];
	    timeout?: string;
	
	    static createFrom(source: any = {}) {
	        return new VerifyCommand(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.executable = source["executable"];
	        this.args = source["args"];
	        this.timeout = source["timeout"];
	    }
	}
	export class Completion {
	    commands?: VerifyCommand[];
	
	    static createFrom(source: any = {}) {
	        return new Completion(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.commands = this.convertValues(source["commands"], VerifyCommand);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Input {
	    name: string;
	    fromStep: string;
	    outputKey: string;
	    required?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Input(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.fromStep = source["fromStep"];
	        this.outputKey = source["outputKey"];
	        this.required = source["required"];
	    }
	}
	
	export class Issue {
	    nodeId?: string;
	    field?: string;
	    code: string;
	    message: string;
	    severity: string;
	
	    static createFrom(source: any = {}) {
	        return new Issue(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.nodeId = source["nodeId"];
	        this.field = source["field"];
	        this.code = source["code"];
	        this.message = source["message"];
	        this.severity = source["severity"];
	    }
	}
	export class RouteSource {
	    fromStep: string;
	    outputKey: string;
	    fieldPath: string;
	
	    static createFrom(source: any = {}) {
	        return new RouteSource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.fromStep = source["fromStep"];
	        this.outputKey = source["outputKey"];
	        this.fieldPath = source["fieldPath"];
	    }
	}
	export class Routing {
	    source: RouteSource;
	    branches: Branch[];
	    defaultTarget: string;
	    joinStep: string;
	
	    static createFrom(source: any = {}) {
	        return new Routing(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.source = this.convertValues(source["source"], RouteSource);
	        this.branches = this.convertValues(source["branches"], Branch);
	        this.defaultTarget = source["defaultTarget"];
	        this.joinStep = source["joinStep"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class NodeLimits {
	    timeout?: string;
	    maxRevisions?: number;
	    maxRetries?: number;
	    maxTokens?: number;
	
	    static createFrom(source: any = {}) {
	        return new NodeLimits(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.timeout = source["timeout"];
	        this.maxRevisions = source["maxRevisions"];
	        this.maxRetries = source["maxRetries"];
	        this.maxTokens = source["maxTokens"];
	    }
	}
	export class Output {
	    key: string;
	    type: string;
	    required?: boolean;
	    schema?: number[];
	
	    static createFrom(source: any = {}) {
	        return new Output(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.type = source["type"];
	        this.required = source["required"];
	        this.schema = source["schema"];
	    }
	}
	export class Node {
	    id: string;
	    title: string;
	    kind: string;
	    assignmentId?: string;
	    dependsOn: string[];
	    inputs?: Input[];
	    instructions?: string;
	    outputs?: Output[];
	    completion?: Completion;
	    reworkTargets?: string[];
	    limits?: NodeLimits;
	    routing?: Routing;
	
	    static createFrom(source: any = {}) {
	        return new Node(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.kind = source["kind"];
	        this.assignmentId = source["assignmentId"];
	        this.dependsOn = source["dependsOn"];
	        this.inputs = this.convertValues(source["inputs"], Input);
	        this.instructions = source["instructions"];
	        this.outputs = this.convertValues(source["outputs"], Output);
	        this.completion = this.convertValues(source["completion"], Completion);
	        this.reworkTargets = source["reworkTargets"];
	        this.limits = this.convertValues(source["limits"], NodeLimits);
	        this.routing = this.convertValues(source["routing"], Routing);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class Organization {
	    id: string;
	    name: string;
	    instructions: string;
	    policy: number[];
	
	    static createFrom(source: any = {}) {
	        return new Organization(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.instructions = source["instructions"];
	        this.policy = source["policy"];
	    }
	}
	
	export class PolicySnapshot {
	    projectMode: string;
	    budget: number[];
	    workspacePath: string;
	    goal: string;
	
	    static createFrom(source: any = {}) {
	        return new PolicySnapshot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.projectMode = source["projectMode"];
	        this.budget = source["budget"];
	        this.workspacePath = source["workspacePath"];
	        this.goal = source["goal"];
	    }
	}
	export class Project {
	    id: string;
	    organizationId: string;
	    name: string;
	    goal: string;
	    instructions: string;
	    workspacePath: string;
	    budget: number[];
	    mode: string;
	    status: string;
	    createdAt: string;
	    updatedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new Project(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.organizationId = source["organizationId"];
	        this.name = source["name"];
	        this.goal = source["goal"];
	        this.instructions = source["instructions"];
	        this.workspacePath = source["workspacePath"];
	        this.budget = source["budget"];
	        this.mode = source["mode"];
	        this.status = source["status"];
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	    }
	}
	export class ProviderConnection {
	    id: string;
	    name: string;
	    provider: string;
	    executablePath: string;
	    secretRef: string;
	    config: number[];
	    verifiedCapabilities: number[];
	
	    static createFrom(source: any = {}) {
	        return new ProviderConnection(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.provider = source["provider"];
	        this.executablePath = source["executablePath"];
	        this.secretRef = source["secretRef"];
	        this.config = source["config"];
	        this.verifiedCapabilities = source["verifiedCapabilities"];
	    }
	}
	export class Role {
	    id: string;
	    organizationId: string;
	    parentRoleId: string;
	    name: string;
	    mission: string;
	    instructions: string;
	    outputDefaults: number[];
	    policy: number[];
	    appearance: number[];
	    updatedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new Role(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.organizationId = source["organizationId"];
	        this.parentRoleId = source["parentRoleId"];
	        this.name = source["name"];
	        this.mission = source["mission"];
	        this.instructions = source["instructions"];
	        this.outputDefaults = source["outputDefaults"];
	        this.policy = source["policy"];
	        this.appearance = source["appearance"];
	        this.updatedAt = source["updatedAt"];
	    }
	}
	
	
	
	export class Workflow {
	    id: string;
	    projectId: string;
	    title: string;
	    draft: number[];
	    revision: number;
	    updatedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new Workflow(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.projectId = source["projectId"];
	        this.title = source["title"];
	        this.draft = source["draft"];
	        this.revision = source["revision"];
	        this.updatedAt = source["updatedAt"];
	    }
	}
	export class WorkflowSpec {
	    schemaVersion: number;
	    title: string;
	    nodes: Node[];
	
	    static createFrom(source: any = {}) {
	        return new WorkflowSpec(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.schemaVersion = source["schemaVersion"];
	        this.title = source["title"];
	        this.nodes = this.convertValues(source["nodes"], Node);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class WorkflowVersion {
	    id: string;
	    workflowId: string;
	    projectId: string;
	    number: number;
	    spec: WorkflowSpec;
	    assignments: Record<string, AssignmentSnapshot>;
	    policy: PolicySnapshot;
	    createdAt: string;
	
	    static createFrom(source: any = {}) {
	        return new WorkflowVersion(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.workflowId = source["workflowId"];
	        this.projectId = source["projectId"];
	        this.number = source["number"];
	        this.spec = this.convertValues(source["spec"], WorkflowSpec);
	        this.assignments = this.convertValues(source["assignments"], AssignmentSnapshot, true);
	        this.policy = this.convertValues(source["policy"], PolicySnapshot);
	        this.createdAt = source["createdAt"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace engine {
	
	export class ArtifactContent {
	    id: string;
	    outputKey: string;
	    version: number;
	    type: string;
	    path: string;
	    hash: string;
	    validity: string;
	    stepId: string;
	    content: string;
	    binary: boolean;
	    truncated: boolean;
	    hashOk: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ArtifactContent(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.outputKey = source["outputKey"];
	        this.version = source["version"];
	        this.type = source["type"];
	        this.path = source["path"];
	        this.hash = source["hash"];
	        this.validity = source["validity"];
	        this.stepId = source["stepId"];
	        this.content = source["content"];
	        this.binary = source["binary"];
	        this.truncated = source["truncated"];
	        this.hashOk = source["hashOk"];
	    }
	}
	export class ArtifactView {
	    id: string;
	    outputKey: string;
	    version: number;
	    type: string;
	    path: string;
	    hash: string;
	    validity: string;
	
	    static createFrom(source: any = {}) {
	        return new ArtifactView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.outputKey = source["outputKey"];
	        this.version = source["version"];
	        this.type = source["type"];
	        this.path = source["path"];
	        this.hash = source["hash"];
	        this.validity = source["validity"];
	    }
	}
	export class StepRef {
	    id: string;
	    title: string;
	
	    static createFrom(source: any = {}) {
	        return new StepRef(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	    }
	}
	export class InboxItem {
	    kind: string;
	    projectId: string;
	    projectName: string;
	    runId: string;
	    runTitle: string;
	    versionNumber: number;
	    stepId: string;
	    stepTitle: string;
	    instructions: string;
	    attemptId: string;
	    generation: number;
	    attempt: number;
	    round: number;
	    approvalId?: string;
	    messageId?: string;
	    detail?: string;
	    inputs: number[];
	    outputs: domain.Output[];
	    reworkTargets: StepRef[];
	    since: string;
	
	    static createFrom(source: any = {}) {
	        return new InboxItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.projectId = source["projectId"];
	        this.projectName = source["projectName"];
	        this.runId = source["runId"];
	        this.runTitle = source["runTitle"];
	        this.versionNumber = source["versionNumber"];
	        this.stepId = source["stepId"];
	        this.stepTitle = source["stepTitle"];
	        this.instructions = source["instructions"];
	        this.attemptId = source["attemptId"];
	        this.generation = source["generation"];
	        this.attempt = source["attempt"];
	        this.round = source["round"];
	        this.approvalId = source["approvalId"];
	        this.messageId = source["messageId"];
	        this.detail = source["detail"];
	        this.inputs = source["inputs"];
	        this.outputs = this.convertValues(source["outputs"], domain.Output);
	        this.reworkTargets = this.convertValues(source["reworkTargets"], StepRef);
	        this.since = source["since"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Outcome {
	    status: string;
	    already: boolean;
	    decision?: string;
	
	    static createFrom(source: any = {}) {
	        return new Outcome(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.status = source["status"];
	        this.already = source["already"];
	        this.decision = source["decision"];
	    }
	}
	export class RunBrief {
	    id: string;
	    title: string;
	    status: string;
	    startedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new RunBrief(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.status = source["status"];
	        this.startedAt = source["startedAt"];
	    }
	}
	export class ProjectSummary {
	    project: domain.Project;
	    activeRuns: number;
	    waitingRuns: number;
	    failedRuns: number;
	    pausedRuns: number;
	    inbox: number;
	    lastRun?: RunBrief;
	
	    static createFrom(source: any = {}) {
	        return new ProjectSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.project = this.convertValues(source["project"], domain.Project);
	        this.activeRuns = source["activeRuns"];
	        this.waitingRuns = source["waitingRuns"];
	        this.failedRuns = source["failedRuns"];
	        this.pausedRuns = source["pausedRuns"];
	        this.inbox = source["inbox"];
	        this.lastRun = this.convertValues(source["lastRun"], RunBrief);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class StepView {
	    id: string;
	    title: string;
	    kind: string;
	    status: string;
	    assignmentId: string;
	    attemptId: string;
	    generation: number;
	    attempt: number;
	    round: number;
	    error?: string;
	    approvalId?: string;
	    inputs?: number[];
	    artifacts: ArtifactView[];
	
	    static createFrom(source: any = {}) {
	        return new StepView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.kind = source["kind"];
	        this.status = source["status"];
	        this.assignmentId = source["assignmentId"];
	        this.attemptId = source["attemptId"];
	        this.generation = source["generation"];
	        this.attempt = source["attempt"];
	        this.round = source["round"];
	        this.error = source["error"];
	        this.approvalId = source["approvalId"];
	        this.inputs = source["inputs"];
	        this.artifacts = this.convertValues(source["artifacts"], ArtifactView);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class RunDetail {
	    id: string;
	    projectId: string;
	    versionId: string;
	    versionNumber: number;
	    title: string;
	    status: string;
	    paused: boolean;
	    steps: StepView[];
	
	    static createFrom(source: any = {}) {
	        return new RunDetail(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.projectId = source["projectId"];
	        this.versionId = source["versionId"];
	        this.versionNumber = source["versionNumber"];
	        this.title = source["title"];
	        this.status = source["status"];
	        this.paused = source["paused"];
	        this.steps = this.convertValues(source["steps"], StepView);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	

}

export namespace main {
	
	export class ActionResult {
	    outcome: engine.Outcome;
	    problem: string;
	
	    static createFrom(source: any = {}) {
	        return new ActionResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.outcome = this.convertValues(source["outcome"], engine.Outcome);
	        this.problem = source["problem"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ConfirmResult {
	    version?: domain.WorkflowVersion;
	    validation: storage.ValidationResult;
	
	    static createFrom(source: any = {}) {
	        return new ConfirmResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = this.convertValues(source["version"], domain.WorkflowVersion);
	        this.validation = this.convertValues(source["validation"], storage.ValidationResult);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ConnectionView {
	    id: string;
	    name: string;
	    provider: string;
	    executablePath: string;
	    secretRef: string;
	    config: number[];
	    verifiedCapabilities: number[];
	    usable: boolean;
	    note: string;
	
	    static createFrom(source: any = {}) {
	        return new ConnectionView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.provider = source["provider"];
	        this.executablePath = source["executablePath"];
	        this.secretRef = source["secretRef"];
	        this.config = source["config"];
	        this.verifiedCapabilities = source["verifiedCapabilities"];
	        this.usable = source["usable"];
	        this.note = source["note"];
	    }
	}
	export class InstructionPreview {
	    layers: domain.InstructionLayer[];
	    composed: string;
	
	    static createFrom(source: any = {}) {
	        return new InstructionPreview(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.layers = this.convertValues(source["layers"], domain.InstructionLayer);
	        this.composed = source["composed"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class PingResult {
	    sequence: number;
	    message: string;
	    at: string;
	
	    static createFrom(source: any = {}) {
	        return new PingResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sequence = source["sequence"];
	        this.message = source["message"];
	        this.at = source["at"];
	    }
	}
	export class ProjectInput {
	    id: string;
	    name: string;
	    goal: string;
	    instructions: string;
	    mode: string;
	
	    static createFrom(source: any = {}) {
	        return new ProjectInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.goal = source["goal"];
	        this.instructions = source["instructions"];
	        this.mode = source["mode"];
	    }
	}
	export class StartResult {
	    runId: string;
	    issues: domain.Issue[];
	
	    static createFrom(source: any = {}) {
	        return new StartResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.runId = source["runId"];
	        this.issues = this.convertValues(source["issues"], domain.Issue);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class SystemStatus {
	    dataDir: string;
	    schemaVersion: number;
	    secretsPersistent: boolean;
	    version: string;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new SystemStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.dataDir = source["dataDir"];
	        this.schemaVersion = source["schemaVersion"];
	        this.secretsPersistent = source["secretsPersistent"];
	        this.version = source["version"];
	        this.error = source["error"];
	    }
	}

}

export namespace storage {
	
	export class EventRecord {
	    sequence: number;
	    id: string;
	    projectId: string;
	    runId?: string;
	    stepAttemptId?: string;
	    kind: string;
	    payload: number[];
	    createdAt: string;
	
	    static createFrom(source: any = {}) {
	        return new EventRecord(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sequence = source["sequence"];
	        this.id = source["id"];
	        this.projectId = source["projectId"];
	        this.runId = source["runId"];
	        this.stepAttemptId = source["stepAttemptId"];
	        this.kind = source["kind"];
	        this.payload = source["payload"];
	        this.createdAt = source["createdAt"];
	    }
	}
	export class ValidationResult {
	    issues: domain.Issue[];
	    canVersion: boolean;
	    canRun: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ValidationResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.issues = this.convertValues(source["issues"], domain.Issue);
	        this.canVersion = source["canVersion"];
	        this.canRun = source["canRun"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace templates {
	
	export class Template {
	    id: string;
	    title: string;
	    description: string;
	    workflow: number[];
	
	    static createFrom(source: any = {}) {
	        return new Template(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.description = source["description"];
	        this.workflow = source["workflow"];
	    }
	}

}

