export namespace connect {
	
	export class Config {
	    authMode?: string;
	    node?: string;
	    script?: string;
	    baseUrl?: string;
	    inputPerMTok?: number;
	    outputPerMTok?: number;
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.authMode = source["authMode"];
	        this.node = source["node"];
	        this.script = source["script"];
	        this.baseUrl = source["baseUrl"];
	        this.inputPerMTok = source["inputPerMTok"];
	        this.outputPerMTok = source["outputPerMTok"];
	    }
	}
	export class Step {
	    id: string;
	    label: string;
	    status: string;
	    detail: string;
	
	    static createFrom(source: any = {}) {
	        return new Step(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.label = source["label"];
	        this.status = source["status"];
	        this.detail = source["detail"];
	    }
	}
	export class Report {
	    steps: Step[];
	    ready: boolean;
	    version?: string;
	
	    static createFrom(source: any = {}) {
	        return new Report(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.steps = this.convertValues(source["steps"], Step);
	        this.ready = source["ready"];
	        this.version = source["version"];
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

export namespace design {
	
	export class ApplyResult {
	    applied?: storage.DesignApplied;
	    issues: domain.Issue[];
	    runId?: string;
	    version?: number;
	
	    static createFrom(source: any = {}) {
	        return new ApplyResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.applied = this.convertValues(source["applied"], storage.DesignApplied);
	        this.issues = this.convertValues(source["issues"], domain.Issue);
	        this.runId = source["runId"];
	        this.version = source["version"];
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
	export class AssignmentView {
	    ref: string;
	    roleRef: string;
	    actorKind: string;
	    displayName: string;
	    connectionId: string;
	    model: string;
	    instructions: string;
	    roleName: string;
	    connectionName: string;
	    connected: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AssignmentView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ref = source["ref"];
	        this.roleRef = source["roleRef"];
	        this.actorKind = source["actorKind"];
	        this.displayName = source["displayName"];
	        this.connectionId = source["connectionId"];
	        this.model = source["model"];
	        this.instructions = source["instructions"];
	        this.roleName = source["roleName"];
	        this.connectionName = source["connectionName"];
	        this.connected = source["connected"];
	    }
	}
	export class AssistInput {
	    projectId: string;
	    workflowId: string;
	    kind: string;
	    request: string;
	    connectionId: string;
	    model: string;
	    parentId: string;
	    scopeNodeId: string;
	
	    static createFrom(source: any = {}) {
	        return new AssistInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.projectId = source["projectId"];
	        this.workflowId = source["workflowId"];
	        this.kind = source["kind"];
	        this.request = source["request"];
	        this.connectionId = source["connectionId"];
	        this.model = source["model"];
	        this.parentId = source["parentId"];
	        this.scopeNodeId = source["scopeNodeId"];
	    }
	}
	export class MissingItem {
	    kind: string;
	    detail: string;
	
	    static createFrom(source: any = {}) {
	        return new MissingItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.detail = source["detail"];
	    }
	}
	export class StepView {
	    id: string;
	    title: string;
	    kind: string;
	    assignee: string;
	    human: boolean;
	
	    static createFrom(source: any = {}) {
	        return new StepView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.kind = source["kind"];
	        this.assignee = source["assignee"];
	        this.human = source["human"];
	    }
	}
	export class RoleChangeView {
	    roleId: string;
	    instructions: string;
	    reason: string;
	    roleName: string;
	    current: string;
	
	    static createFrom(source: any = {}) {
	        return new RoleChangeView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.roleId = source["roleId"];
	        this.instructions = source["instructions"];
	        this.reason = source["reason"];
	        this.roleName = source["roleName"];
	        this.current = source["current"];
	    }
	}
	export class ProposedRole {
	    ref: string;
	    reuseRoleId: string;
	    name: string;
	    mission: string;
	    instructions: string;
	    parentRef: string;
	
	    static createFrom(source: any = {}) {
	        return new ProposedRole(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ref = source["ref"];
	        this.reuseRoleId = source["reuseRoleId"];
	        this.name = source["name"];
	        this.mission = source["mission"];
	        this.instructions = source["instructions"];
	        this.parentRef = source["parentRef"];
	    }
	}
	export class RoleRef {
	    ref: string;
	    id: string;
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new RoleRef(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ref = source["ref"];
	        this.id = source["id"];
	        this.name = source["name"];
	    }
	}
	export class Diff {
	    reuseRoles: RoleRef[];
	    newRoles: ProposedRole[];
	    roleChanges: RoleChangeView[];
	    assignments: AssignmentView[];
	    steps: StepView[];
	    missing: MissingItem[];
	
	    static createFrom(source: any = {}) {
	        return new Diff(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.reuseRoles = this.convertValues(source["reuseRoles"], RoleRef);
	        this.newRoles = this.convertValues(source["newRoles"], ProposedRole);
	        this.roleChanges = this.convertValues(source["roleChanges"], RoleChangeView);
	        this.assignments = this.convertValues(source["assignments"], AssignmentView);
	        this.steps = this.convertValues(source["steps"], StepView);
	        this.missing = this.convertValues(source["missing"], MissingItem);
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
	export class HumanStep {
	    request: string;
	    stepId: string;
	
	    static createFrom(source: any = {}) {
	        return new HumanStep(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.request = source["request"];
	        this.stepId = source["stepId"];
	    }
	}
	
	export class RoleChange {
	    roleId: string;
	    instructions: string;
	    reason: string;
	
	    static createFrom(source: any = {}) {
	        return new RoleChange(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.roleId = source["roleId"];
	        this.instructions = source["instructions"];
	        this.reason = source["reason"];
	    }
	}
	export class ProposedAssignment {
	    ref: string;
	    roleRef: string;
	    actorKind: string;
	    displayName: string;
	    connectionId: string;
	    model: string;
	    instructions: string;
	
	    static createFrom(source: any = {}) {
	        return new ProposedAssignment(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ref = source["ref"];
	        this.roleRef = source["roleRef"];
	        this.actorKind = source["actorKind"];
	        this.displayName = source["displayName"];
	        this.connectionId = source["connectionId"];
	        this.model = source["model"];
	        this.instructions = source["instructions"];
	    }
	}
	export class ProposedProject {
	    name: string;
	    goal: string;
	    instructions: string;
	
	    static createFrom(source: any = {}) {
	        return new ProposedProject(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.goal = source["goal"];
	        this.instructions = source["instructions"];
	    }
	}
	export class Proposal {
	    project: ProposedProject;
	    roles: ProposedRole[];
	    assignments: ProposedAssignment[];
	    workflow: domain.WorkflowSpec;
	    humanSteps: HumanStep[];
	    missing: MissingItem[];
	    roleChanges: RoleChange[];
	    notes: string;
	
	    static createFrom(source: any = {}) {
	        return new Proposal(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.project = this.convertValues(source["project"], ProposedProject);
	        this.roles = this.convertValues(source["roles"], ProposedRole);
	        this.assignments = this.convertValues(source["assignments"], ProposedAssignment);
	        this.workflow = this.convertValues(source["workflow"], domain.WorkflowSpec);
	        this.humanSteps = this.convertValues(source["humanSteps"], HumanStep);
	        this.missing = this.convertValues(source["missing"], MissingItem);
	        this.roleChanges = this.convertValues(source["roleChanges"], RoleChange);
	        this.notes = source["notes"];
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
	
	
	
	export class Result {
	    proposal?: Proposal;
	    issues: domain.Issue[];
	    diff: Diff;
	    canApply: boolean;
	    canStart: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Result(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.proposal = this.convertValues(source["proposal"], Proposal);
	        this.issues = this.convertValues(source["issues"], domain.Issue);
	        this.diff = this.convertValues(source["diff"], Diff);
	        this.canApply = source["canApply"];
	        this.canStart = source["canStart"];
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
	
	
	
	
	export class AutoPolicy {
	    allowedConnectionIds: string[];
	
	    static createFrom(source: any = {}) {
	        return new AutoPolicy(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.allowedConnectionIds = source["allowedConnectionIds"];
	    }
	}
	export class StoredInput {
	    goal: string;
	    projectId: string;
	    projectName: string;
	    humanTasks: string[];
	    mode: string;
	    // Go type: AutoPolicy
	    autoPolicy: any;
	    model: string;
	    budget: engine.Budget;
	
	    static createFrom(source: any = {}) {
	        return new StoredInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.goal = source["goal"];
	        this.projectId = source["projectId"];
	        this.projectName = source["projectName"];
	        this.humanTasks = source["humanTasks"];
	        this.mode = source["mode"];
	        this.autoPolicy = this.convertValues(source["autoPolicy"], null);
	        this.model = source["model"];
	        this.budget = this.convertValues(source["budget"], engine.Budget);
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
	export class Department {
	    id: string;
	    projectId: string;
	    name: string;
	    responsibility: string;
	    leadAssignmentId: string;
	    memberIds: string[];
	    templateId: string;
	    automation: number[];
	    limits: number[];
	    status: string;
	    createdAt: string;
	    updatedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new Department(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.projectId = source["projectId"];
	        this.name = source["name"];
	        this.responsibility = source["responsibility"];
	        this.leadAssignmentId = source["leadAssignmentId"];
	        this.memberIds = source["memberIds"];
	        this.templateId = source["templateId"];
	        this.automation = source["automation"];
	        this.limits = source["limits"];
	        this.status = source["status"];
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	    }
	}
	export class DepartmentAutomation {
	    autoStart: boolean;
	    categories: string[];
	    connectionIds: string[];
	    outputTypes: string[];
	    workflowVersionIds: string[];
	    autoShare: boolean;
	    autoCollab: boolean;
	
	    static createFrom(source: any = {}) {
	        return new DepartmentAutomation(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.autoStart = source["autoStart"];
	        this.categories = source["categories"];
	        this.connectionIds = source["connectionIds"];
	        this.outputTypes = source["outputTypes"];
	        this.workflowVersionIds = source["workflowVersionIds"];
	        this.autoShare = source["autoShare"];
	        this.autoCollab = source["autoCollab"];
	    }
	}
	export class DepartmentSchedule {
	    id: string;
	    projectId: string;
	    departmentId: string;
	    frequency: string;
	    timeOfDay: string;
	    weekday?: number;
	    timezone: string;
	    note: string;
	    enabled: boolean;
	    activeFrom: string;
	    createdAt: string;
	    updatedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new DepartmentSchedule(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.projectId = source["projectId"];
	        this.departmentId = source["departmentId"];
	        this.frequency = source["frequency"];
	        this.timeOfDay = source["timeOfDay"];
	        this.weekday = source["weekday"];
	        this.timezone = source["timezone"];
	        this.note = source["note"];
	        this.enabled = source["enabled"];
	        this.activeFrom = source["activeFrom"];
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	    }
	}
	export class DepartmentTemplate {
	    id: string;
	    organizationId: string;
	    name: string;
	    responsibility: string;
	    roleIds: string[];
	    updatedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new DepartmentTemplate(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.organizationId = source["organizationId"];
	        this.name = source["name"];
	        this.responsibility = source["responsibility"];
	        this.roleIds = source["roleIds"];
	        this.updatedAt = source["updatedAt"];
	    }
	}
	export class DeskBudget {
	    monthlyCostUsd?: number;
	
	    static createFrom(source: any = {}) {
	        return new DeskBudget(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.monthlyCostUsd = source["monthlyCostUsd"];
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
	
	export class IntakeDesk {
	    id: string;
	    organizationId: string;
	    projectId: string;
	    name: string;
	    assignmentId: string;
	    connectionId: string;
	    model: string;
	    instructions: string;
	    budget: DeskBudget;
	    enabled: boolean;
	    createdAt: string;
	    updatedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new IntakeDesk(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.organizationId = source["organizationId"];
	        this.projectId = source["projectId"];
	        this.name = source["name"];
	        this.assignmentId = source["assignmentId"];
	        this.connectionId = source["connectionId"];
	        this.model = source["model"];
	        this.instructions = source["instructions"];
	        this.budget = this.convertValues(source["budget"], DeskBudget);
	        this.enabled = source["enabled"];
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
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
	export class Meeting {
	    participants: string[];
	    maxRounds?: number;
	
	    static createFrom(source: any = {}) {
	        return new Meeting(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.participants = source["participants"];
	        this.maxRounds = source["maxRounds"];
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
	    meeting?: Meeting;
	
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
	        this.meeting = this.convertValues(source["meeting"], Meeting);
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
	    autoPolicy: number[];
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
	        this.autoPolicy = source["autoPolicy"];
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
	export class RequestItem {
	    id: string;
	    requestId: string;
	    projectId: string;
	    round: number;
	    position: number;
	    kind: string;
	    payload: number[];
	    status: string;
	    workItemId: string;
	    workTitle: string;
	    workStatus: string;
	    childRequestId: string;
	    result: number[];
	    error: string;
	    decidedBy: string;
	    decisionNote: string;
	    decidedAt: string;
	    createdAt: string;
	
	    static createFrom(source: any = {}) {
	        return new RequestItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.requestId = source["requestId"];
	        this.projectId = source["projectId"];
	        this.round = source["round"];
	        this.position = source["position"];
	        this.kind = source["kind"];
	        this.payload = source["payload"];
	        this.status = source["status"];
	        this.workItemId = source["workItemId"];
	        this.workTitle = source["workTitle"];
	        this.workStatus = source["workStatus"];
	        this.childRequestId = source["childRequestId"];
	        this.result = source["result"];
	        this.error = source["error"];
	        this.decidedBy = source["decidedBy"];
	        this.decisionNote = source["decisionNote"];
	        this.decidedAt = source["decidedAt"];
	        this.createdAt = source["createdAt"];
	    }
	}
	export class Request {
	    id: string;
	    organizationId: string;
	    projectId: string;
	    projectName: string;
	    parentId: string;
	    previousId: string;
	    body: string;
	    status: string;
	    outcome: string;
	    round: number;
	    answer: string;
	    question: string;
	    reply: string;
	    reason: string;
	    error: string;
	    holdReason: string;
	    maxAttempts: number;
	    attempts: number;
	    createdAt: string;
	    updatedAt: string;
	    closedAt: string;
	    staffedDesk?: string;
	    items: RequestItem[];
	    children: Request[];
	
	    static createFrom(source: any = {}) {
	        return new Request(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.organizationId = source["organizationId"];
	        this.projectId = source["projectId"];
	        this.projectName = source["projectName"];
	        this.parentId = source["parentId"];
	        this.previousId = source["previousId"];
	        this.body = source["body"];
	        this.status = source["status"];
	        this.outcome = source["outcome"];
	        this.round = source["round"];
	        this.answer = source["answer"];
	        this.question = source["question"];
	        this.reply = source["reply"];
	        this.reason = source["reason"];
	        this.error = source["error"];
	        this.holdReason = source["holdReason"];
	        this.maxAttempts = source["maxAttempts"];
	        this.attempts = source["attempts"];
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	        this.closedAt = source["closedAt"];
	        this.staffedDesk = source["staffedDesk"];
	        this.items = this.convertValues(source["items"], RequestItem);
	        this.children = this.convertValues(source["children"], Request);
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
	
	
	export class Subscription {
	    id: string;
	    projectId: string;
	    departmentId: string;
	    eventKinds: string[];
	    categories: string[];
	    guidance: string;
	    enabled: boolean;
	    updatedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new Subscription(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.projectId = source["projectId"];
	        this.departmentId = source["departmentId"];
	        this.eventKinds = source["eventKinds"];
	        this.categories = source["categories"];
	        this.guidance = source["guidance"];
	        this.enabled = source["enabled"];
	        this.updatedAt = source["updatedAt"];
	    }
	}
	
	export class WorkItem {
	    id: string;
	    projectId: string;
	    kind: string;
	    parentId: string;
	    departmentId: string;
	    assigneeAssignmentId: string;
	    title: string;
	    description: string;
	    doneCriteria: string;
	    priority: string;
	    status: string;
	    statusReason: string;
	    categories: string[];
	    dueAt: string;
	    reviewBeforeStart: boolean;
	    createdAt: string;
	    updatedAt: string;
	    closedAt: string;
	    originRequestId?: string;
	
	    static createFrom(source: any = {}) {
	        return new WorkItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.projectId = source["projectId"];
	        this.kind = source["kind"];
	        this.parentId = source["parentId"];
	        this.departmentId = source["departmentId"];
	        this.assigneeAssignmentId = source["assigneeAssignmentId"];
	        this.title = source["title"];
	        this.description = source["description"];
	        this.doneCriteria = source["doneCriteria"];
	        this.priority = source["priority"];
	        this.status = source["status"];
	        this.statusReason = source["statusReason"];
	        this.categories = source["categories"];
	        this.dueAt = source["dueAt"];
	        this.reviewBeforeStart = source["reviewBeforeStart"];
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	        this.closedAt = source["closedAt"];
	        this.originRequestId = source["originRequestId"];
	    }
	}
	export class WorkNote {
	    id: string;
	    workItemId: string;
	    kind: string;
	    body: string;
	    fromStatus?: string;
	    toStatus?: string;
	    author: string;
	    createdAt: string;
	
	    static createFrom(source: any = {}) {
	        return new WorkNote(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.workItemId = source["workItemId"];
	        this.kind = source["kind"];
	        this.body = source["body"];
	        this.fromStatus = source["fromStatus"];
	        this.toStatus = source["toStatus"];
	        this.author = source["author"];
	        this.createdAt = source["createdAt"];
	    }
	}
	export class WorkPlan {
	    workItemId: string;
	    projectId: string;
	    assignmentId: string;
	    instructions: string;
	    materials: string;
	    outputs: Output[];
	    workflowVersionId: string;
	    updatedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new WorkPlan(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.workItemId = source["workItemId"];
	        this.projectId = source["projectId"];
	        this.assignmentId = source["assignmentId"];
	        this.instructions = source["instructions"];
	        this.materials = source["materials"];
	        this.outputs = this.convertValues(source["outputs"], Output);
	        this.workflowVersionId = source["workflowVersionId"];
	        this.updatedAt = source["updatedAt"];
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
	export class WorkProposalPayload {
	    title: string;
	    description: string;
	    doneCriteria: string;
	    categories: string[];
	    priority: string;
	    departmentId?: string;
	    assigneeAssignmentId?: string;
	    goalId?: string;
	
	    static createFrom(source: any = {}) {
	        return new WorkProposalPayload(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.title = source["title"];
	        this.description = source["description"];
	        this.doneCriteria = source["doneCriteria"];
	        this.categories = source["categories"];
	        this.priority = source["priority"];
	        this.departmentId = source["departmentId"];
	        this.assigneeAssignmentId = source["assigneeAssignmentId"];
	        this.goalId = source["goalId"];
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
	export class ArtifactRef {
	    id: string;
	    stepId?: string;
	    outputKey: string;
	    version: number;
	    hash: string;
	
	    static createFrom(source: any = {}) {
	        return new ArtifactRef(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.stepId = source["stepId"];
	        this.outputKey = source["outputKey"];
	        this.version = source["version"];
	        this.hash = source["hash"];
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
	export class Budget {
	    maxTokens?: number;
	    maxCostUsd?: number;
	
	    static createFrom(source: any = {}) {
	        return new Budget(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.maxTokens = source["maxTokens"];
	        this.maxCostUsd = source["maxCostUsd"];
	    }
	}
	export class CodeDelivery {
	    method: string;
	    commit?: string;
	    branch?: string;
	    folder?: string;
	    workspace?: boolean;
	    note?: string;
	    error?: string;
	    at: string;
	    requestItemId?: string;
	
	    static createFrom(source: any = {}) {
	        return new CodeDelivery(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.method = source["method"];
	        this.commit = source["commit"];
	        this.branch = source["branch"];
	        this.folder = source["folder"];
	        this.workspace = source["workspace"];
	        this.note = source["note"];
	        this.error = source["error"];
	        this.at = source["at"];
	        this.requestItemId = source["requestItemId"];
	    }
	}
	export class ContextNote {
	    label: string;
	    ref: string;
	    text: string;
	
	    static createFrom(source: any = {}) {
	        return new ContextNote(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.label = source["label"];
	        this.ref = source["ref"];
	        this.text = source["text"];
	    }
	}
	export class DeliverInput {
	    projectId: string;
	    runId: string;
	    method: string;
	    folder?: string;
	    useAsWorkspace?: boolean;
	    requestItemId?: string;
	
	    static createFrom(source: any = {}) {
	        return new DeliverInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.projectId = source["projectId"];
	        this.runId = source["runId"];
	        this.method = source["method"];
	        this.folder = source["folder"];
	        this.useAsWorkspace = source["useAsWorkspace"];
	        this.requestItemId = source["requestItemId"];
	    }
	}
	export class DocumentContent {
	    id: string;
	    documentId: string;
	    projectId: string;
	    number: number;
	    stage: string;
	    confirmedBy: string;
	    confirmedAt: string;
	    confirmationNote: string;
	    sourceKind: string;
	    sourceArtifactId: string;
	    sourcePath: string;
	    fileName: string;
	    path: string;
	    hash: string;
	    size: number;
	    withdrawnAt: string;
	    createdAt: string;
	    title: string;
	    content: string;
	    binary: boolean;
	    truncated: boolean;
	    hashOk: boolean;
	
	    static createFrom(source: any = {}) {
	        return new DocumentContent(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.documentId = source["documentId"];
	        this.projectId = source["projectId"];
	        this.number = source["number"];
	        this.stage = source["stage"];
	        this.confirmedBy = source["confirmedBy"];
	        this.confirmedAt = source["confirmedAt"];
	        this.confirmationNote = source["confirmationNote"];
	        this.sourceKind = source["sourceKind"];
	        this.sourceArtifactId = source["sourceArtifactId"];
	        this.sourcePath = source["sourcePath"];
	        this.fileName = source["fileName"];
	        this.path = source["path"];
	        this.hash = source["hash"];
	        this.size = source["size"];
	        this.withdrawnAt = source["withdrawnAt"];
	        this.createdAt = source["createdAt"];
	        this.title = source["title"];
	        this.content = source["content"];
	        this.binary = source["binary"];
	        this.truncated = source["truncated"];
	        this.hashOk = source["hashOk"];
	    }
	}
	export class FinalStep {
	    stepId: string;
	    title: string;
	    commit: string;
	
	    static createFrom(source: any = {}) {
	        return new FinalStep(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.stepId = source["stepId"];
	        this.title = source["title"];
	        this.commit = source["commit"];
	    }
	}
	export class FinalCode {
	    runId: string;
	    repo: string;
	    appRepo: boolean;
	    base: string;
	    steps: FinalStep[];
	    commit?: string;
	    stat: workspace.Stat;
	    included: boolean;
	    deliveries: CodeDelivery[];
	
	    static createFrom(source: any = {}) {
	        return new FinalCode(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.runId = source["runId"];
	        this.repo = source["repo"];
	        this.appRepo = source["appRepo"];
	        this.base = source["base"];
	        this.steps = this.convertValues(source["steps"], FinalStep);
	        this.commit = source["commit"];
	        this.stat = this.convertValues(source["stat"], workspace.Stat);
	        this.included = source["included"];
	        this.deliveries = this.convertValues(source["deliveries"], CodeDelivery);
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
	
	export class WorkspaceInfo {
	    kind: string;
	    path: string;
	    branch?: string;
	    start?: string;
	    commit?: string;
	    status: string;
	    conflicts: string[];
	    note?: string;
	
	    static createFrom(source: any = {}) {
	        return new WorkspaceInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.path = source["path"];
	        this.branch = source["branch"];
	        this.start = source["start"];
	        this.commit = source["commit"];
	        this.status = source["status"];
	        this.conflicts = source["conflicts"];
	        this.note = source["note"];
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
	export class ToolRequest {
	    name: string;
	    path?: string;
	    command?: string;
	    input?: string;
	
	    static createFrom(source: any = {}) {
	        return new ToolRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.command = source["command"];
	        this.input = source["input"];
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
	    tool?: ToolRequest;
	    inputs: number[];
	    outputs: domain.Output[];
	    reworkTargets: StepRef[];
	    context: ContextNote[];
	    escalation?: string;
	    workspace?: WorkspaceInfo;
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
	        this.tool = this.convertValues(source["tool"], ToolRequest);
	        this.inputs = source["inputs"];
	        this.outputs = this.convertValues(source["outputs"], domain.Output);
	        this.reworkTargets = this.convertValues(source["reworkTargets"], StepRef);
	        this.context = this.convertValues(source["context"], ContextNote);
	        this.escalation = source["escalation"];
	        this.workspace = this.convertValues(source["workspace"], WorkspaceInfo);
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
	export class MessageRefs {
	    artifacts?: ArtifactRef[];
	    generation?: number;
	    from?: string;
	    note?: boolean;
	    round?: number;
	    agree?: boolean;
	    reason?: string;
	
	    static createFrom(source: any = {}) {
	        return new MessageRefs(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.artifacts = this.convertValues(source["artifacts"], ArtifactRef);
	        this.generation = source["generation"];
	        this.from = source["from"];
	        this.note = source["note"];
	        this.round = source["round"];
	        this.agree = source["agree"];
	        this.reason = source["reason"];
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
	export class MessageView {
	    id: string;
	    kind: string;
	    sender: string;
	    senderName: string;
	    recipient: string;
	    recipientName: string;
	    stepId: string;
	    body: string;
	    replyTo?: string;
	    refs: MessageRefs;
	    createdAt: string;
	
	    static createFrom(source: any = {}) {
	        return new MessageView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.kind = source["kind"];
	        this.sender = source["sender"];
	        this.senderName = source["senderName"];
	        this.recipient = source["recipient"];
	        this.recipientName = source["recipientName"];
	        this.stepId = source["stepId"];
	        this.body = source["body"];
	        this.replyTo = source["replyTo"];
	        this.refs = this.convertValues(source["refs"], MessageRefs);
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
	export class RepoInfo {
	    kind: string;
	    path: string;
	    base?: string;
	    note?: string;
	
	    static createFrom(source: any = {}) {
	        return new RepoInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.path = source["path"];
	        this.base = source["base"];
	        this.note = source["note"];
	    }
	}
	
	export class StepView {
	    id: string;
	    title: string;
	    kind: string;
	    status: string;
	    assignmentId: string;
	    participants?: string[];
	    attemptId: string;
	    generation: number;
	    attempt: number;
	    round: number;
	    error?: string;
	    approvalId?: string;
	    inputs?: number[];
	    artifacts: ArtifactView[];
	    workspace?: WorkspaceInfo;
	
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
	        this.participants = source["participants"];
	        this.attemptId = source["attemptId"];
	        this.generation = source["generation"];
	        this.attempt = source["attempt"];
	        this.round = source["round"];
	        this.error = source["error"];
	        this.approvalId = source["approvalId"];
	        this.inputs = source["inputs"];
	        this.artifacts = this.convertValues(source["artifacts"], ArtifactView);
	        this.workspace = this.convertValues(source["workspace"], WorkspaceInfo);
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
	    repo?: RepoInfo;
	
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
	        this.repo = this.convertValues(source["repo"], RepoInfo);
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
	
	
	
	export class Usage {
	    inputTokens: number;
	    outputTokens: number;
	    costUsd: number;
	    unknownCostAttempts: number;
	    attempts: number;
	    budget: Budget;
	    holdReason?: string;
	
	    static createFrom(source: any = {}) {
	        return new Usage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.inputTokens = source["inputTokens"];
	        this.outputTokens = source["outputTokens"];
	        this.costUsd = source["costUsd"];
	        this.unknownCostAttempts = source["unknownCostAttempts"];
	        this.attempts = source["attempts"];
	        this.budget = this.convertValues(source["budget"], Budget);
	        this.holdReason = source["holdReason"];
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
	
	export class ActionInput {
	    projectId: string;
	    itemId: string;
	    method?: string;
	    folder?: string;
	    useAsWorkspace?: boolean;
	    note?: string;
	
	    static createFrom(source: any = {}) {
	        return new ActionInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.projectId = source["projectId"];
	        this.itemId = source["itemId"];
	        this.method = source["method"];
	        this.folder = source["folder"];
	        this.useAsWorkspace = source["useAsWorkspace"];
	        this.note = source["note"];
	    }
	}
	export class ActionOutcome {
	    item: domain.RequestItem;
	    cancelled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ActionOutcome(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.item = this.convertValues(source["item"], domain.RequestItem);
	        this.cancelled = source["cancelled"];
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
	export class ConnectionInput {
	    id: string;
	    name: string;
	    provider: string;
	    executablePath: string;
	    settings: connect.Config;
	
	    static createFrom(source: any = {}) {
	        return new ConnectionInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.provider = source["provider"];
	        this.executablePath = source["executablePath"];
	        this.settings = this.convertValues(source["settings"], connect.Config);
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
	    hasKey: boolean;
	    settings: connect.Config;
	
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
	        this.hasKey = source["hasKey"];
	        this.settings = this.convertValues(source["settings"], connect.Config);
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
	export class Decisions {
	    questions: storage.Judgment[];
	    proposals: storage.Proposal[];
	    collabs: storage.CollabRequest[];
	    notices: storage.RequiredNotice[];
	    aiDecisions: storage.CollabRequest[];
	    autoReviews: storage.AutoStart[];
	    results: storage.ResultReview[];
	    goals: Record<string, storage.GoalRef>;
	    requests: domain.Request[];
	
	    static createFrom(source: any = {}) {
	        return new Decisions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.questions = this.convertValues(source["questions"], storage.Judgment);
	        this.proposals = this.convertValues(source["proposals"], storage.Proposal);
	        this.collabs = this.convertValues(source["collabs"], storage.CollabRequest);
	        this.notices = this.convertValues(source["notices"], storage.RequiredNotice);
	        this.aiDecisions = this.convertValues(source["aiDecisions"], storage.CollabRequest);
	        this.autoReviews = this.convertValues(source["autoReviews"], storage.AutoStart);
	        this.results = this.convertValues(source["results"], storage.ResultReview);
	        this.goals = this.convertValues(source["goals"], storage.GoalRef, true);
	        this.requests = this.convertValues(source["requests"], domain.Request);
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
	export class DesignInput {
	    goal: string;
	    projectId: string;
	    projectName: string;
	    humanTasks: string[];
	    mode: string;
	    connectionId: string;
	    model: string;
	    budget: engine.Budget;
	    allowedConnectionIds: string[];
	
	    static createFrom(source: any = {}) {
	        return new DesignInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.goal = source["goal"];
	        this.projectId = source["projectId"];
	        this.projectName = source["projectName"];
	        this.humanTasks = source["humanTasks"];
	        this.mode = source["mode"];
	        this.connectionId = source["connectionId"];
	        this.model = source["model"];
	        this.budget = this.convertValues(source["budget"], engine.Budget);
	        this.allowedConnectionIds = source["allowedConnectionIds"];
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
	export class DesignView {
	    id: string;
	    projectId: string;
	    goal: string;
	    input: number[];
	    mode: string;
	    connectionId: string;
	    status: string;
	    proposal: number[];
	    checkResult: number[];
	    error: string;
	    applied: number[];
	    createdAt: string;
	    updatedAt: string;
	    request: design.StoredInput;
	    result?: design.Result;
	    autoIssues: domain.Issue[];
	
	    static createFrom(source: any = {}) {
	        return new DesignView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.projectId = source["projectId"];
	        this.goal = source["goal"];
	        this.input = source["input"];
	        this.mode = source["mode"];
	        this.connectionId = source["connectionId"];
	        this.status = source["status"];
	        this.proposal = source["proposal"];
	        this.checkResult = source["checkResult"];
	        this.error = source["error"];
	        this.applied = source["applied"];
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	        this.request = this.convertValues(source["request"], design.StoredInput);
	        this.result = this.convertValues(source["result"], design.Result);
	        this.autoIssues = this.convertValues(source["autoIssues"], domain.Issue);
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
	export class DeskView {
	    desk: domain.IntakeDesk;
	    exists: boolean;
	    monthUsage?: storage.AIAttemptUsage;
	    assignee?: domain.Assignment;
	    lastStaffing?: storage.Staffing;
	    suggestedConnectionId: string;
	    suggestedModel: string;
	
	    static createFrom(source: any = {}) {
	        return new DeskView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.desk = this.convertValues(source["desk"], domain.IntakeDesk);
	        this.exists = source["exists"];
	        this.monthUsage = this.convertValues(source["monthUsage"], storage.AIAttemptUsage);
	        this.assignee = this.convertValues(source["assignee"], domain.Assignment);
	        this.lastStaffing = this.convertValues(source["lastStaffing"], storage.Staffing);
	        this.suggestedConnectionId = source["suggestedConnectionId"];
	        this.suggestedModel = source["suggestedModel"];
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
	export class DetectedPaths {
	    codex: string;
	    node: string;
	    bridge: string;
	    claude: string;
	
	    static createFrom(source: any = {}) {
	        return new DetectedPaths(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.codex = source["codex"];
	        this.node = source["node"];
	        this.bridge = source["bridge"];
	        this.claude = source["claude"];
	    }
	}
	export class ExportResult {
	    projectName: string;
	    tables: number;
	    rows: number;
	    artifacts: number;
	    missing: string[];
	    masked: number;
	    path: string;
	
	    static createFrom(source: any = {}) {
	        return new ExportResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.projectName = source["projectName"];
	        this.tables = source["tables"];
	        this.rows = source["rows"];
	        this.artifacts = source["artifacts"];
	        this.missing = source["missing"];
	        this.masked = source["masked"];
	        this.path = source["path"];
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
	export class NewDeskInput {
	    projectId: string;
	    name: string;
	    connectionId: string;
	    model: string;
	    instructions: string;
	
	    static createFrom(source: any = {}) {
	        return new NewDeskInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.projectId = source["projectId"];
	        this.name = source["name"];
	        this.connectionId = source["connectionId"];
	        this.model = source["model"];
	        this.instructions = source["instructions"];
	    }
	}
	export class Operations {
	    departments: storage.DepartmentLoad[];
	    troubles: storage.DeliveryTrouble[];
	    troubleCount: number;
	    usage: engine.Usage;
	
	    static createFrom(source: any = {}) {
	        return new Operations(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.departments = this.convertValues(source["departments"], storage.DepartmentLoad);
	        this.troubles = this.convertValues(source["troubles"], storage.DeliveryTrouble);
	        this.troubleCount = source["troubleCount"];
	        this.usage = this.convertValues(source["usage"], engine.Usage);
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
	export class PlanJobView {
	    job: storage.AIJob;
	    proposal?: work.ProposedPlan;
	    stale: boolean;
	
	    static createFrom(source: any = {}) {
	        return new PlanJobView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.job = this.convertValues(source["job"], storage.AIJob);
	        this.proposal = this.convertValues(source["proposal"], work.ProposedPlan);
	        this.stale = source["stale"];
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
	export class ProjectInput {
	    id: string;
	    name: string;
	    goal: string;
	    instructions: string;
	    mode: string;
	    budget: engine.Budget;
	    workspacePath: string;
	
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
	        this.budget = this.convertValues(source["budget"], engine.Budget);
	        this.workspacePath = source["workspacePath"];
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
	export class ProposalBatch {
	    done: number;
	    errors: string[];
	
	    static createFrom(source: any = {}) {
	        return new ProposalBatch(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.done = source["done"];
	        this.errors = source["errors"];
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
	    perf: boolean;
	
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
	        this.perf = source["perf"];
	    }
	}
	export class WorkCollab {
	    requests: storage.CollabRequest[];
	    incoming: storage.CollabRequest[];
	    blockers: storage.CollabRequest[];
	    checks: storage.ImpactCheck[];
	
	    static createFrom(source: any = {}) {
	        return new WorkCollab(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.requests = this.convertValues(source["requests"], storage.CollabRequest);
	        this.incoming = this.convertValues(source["incoming"], storage.CollabRequest);
	        this.blockers = this.convertValues(source["blockers"], storage.CollabRequest);
	        this.checks = this.convertValues(source["checks"], storage.ImpactCheck);
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
	export class WorkDetail {
	    item: domain.WorkItem;
	    notes: domain.WorkNote[];
	    children: domain.WorkItem[];
	
	    static createFrom(source: any = {}) {
	        return new WorkDetail(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.item = this.convertValues(source["item"], domain.WorkItem);
	        this.notes = this.convertValues(source["notes"], domain.WorkNote);
	        this.children = this.convertValues(source["children"], domain.WorkItem);
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
	export class WorkPlanView {
	    plan: domain.WorkPlan;
	    saved: boolean;
	    preview: string;
	    problem: string;
	    approval: storage.StartApproval;
	    autoStart?: storage.AutoStart;
	
	    static createFrom(source: any = {}) {
	        return new WorkPlanView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.plan = this.convertValues(source["plan"], domain.WorkPlan);
	        this.saved = source["saved"];
	        this.preview = source["preview"];
	        this.problem = source["problem"];
	        this.approval = this.convertValues(source["approval"], storage.StartApproval);
	        this.autoStart = this.convertValues(source["autoStart"], storage.AutoStart);
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

export namespace storage {
	
	export class AIAttemptUsage {
	    Attempts: number;
	    UnknownCost: number;
	    InputTokens: number;
	    OutputTokens: number;
	    CostUSD: number;
	
	    static createFrom(source: any = {}) {
	        return new AIAttemptUsage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Attempts = source["Attempts"];
	        this.UnknownCost = source["UnknownCost"];
	        this.InputTokens = source["InputTokens"];
	        this.OutputTokens = source["OutputTokens"];
	        this.CostUSD = source["CostUSD"];
	    }
	}
	export class AIJob {
	    id: string;
	    projectId: string;
	    kind: string;
	    lane: string;
	    requestKey: string;
	    workItemId: string;
	    deliveryId: string;
	    round: number;
	    assignmentId: string;
	    connectionId: string;
	    model: string;
	    status: string;
	    inputSnapshot: number[];
	    inputHash: string;
	    result: number[];
	    error: string;
	    holdReason: string;
	    maxAttempts: number;
	    attempts: number;
	    appliedAt: string;
	    createdAt: string;
	    updatedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new AIJob(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.projectId = source["projectId"];
	        this.kind = source["kind"];
	        this.lane = source["lane"];
	        this.requestKey = source["requestKey"];
	        this.workItemId = source["workItemId"];
	        this.deliveryId = source["deliveryId"];
	        this.round = source["round"];
	        this.assignmentId = source["assignmentId"];
	        this.connectionId = source["connectionId"];
	        this.model = source["model"];
	        this.status = source["status"];
	        this.inputSnapshot = source["inputSnapshot"];
	        this.inputHash = source["inputHash"];
	        this.result = source["result"];
	        this.error = source["error"];
	        this.holdReason = source["holdReason"];
	        this.maxAttempts = source["maxAttempts"];
	        this.attempts = source["attempts"];
	        this.appliedAt = source["appliedAt"];
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	    }
	}
	export class EventRecord {
	    sequence: number;
	    id: string;
	    projectId: string;
	    runId?: string;
	    stepAttemptId?: string;
	    kind: string;
	    payload: number[];
	    createdAt: string;
	    stepId?: string;
	    assignmentId?: string;
	    attempt?: number;
	
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
	        this.stepId = source["stepId"];
	        this.assignmentId = source["assignmentId"];
	        this.attempt = source["attempt"];
	    }
	}
	export class ActivityPage {
	    events: EventRecord[];
	    hasMore: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ActivityPage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.events = this.convertValues(source["events"], EventRecord);
	        this.hasMore = source["hasMore"];
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
	export class AssistRecord {
	    id: string;
	    projectId: string;
	    workflowId: string;
	    kind: string;
	    request: string;
	    baseRevision: number;
	    connectionId: string;
	    model: string;
	    status: string;
	    result: number[];
	    error: string;
	    createdAt: string;
	    updatedAt: string;
	    baseWorkflow: number[];
	    parentId?: string;
	    scopeNodeId?: string;
	
	    static createFrom(source: any = {}) {
	        return new AssistRecord(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.projectId = source["projectId"];
	        this.workflowId = source["workflowId"];
	        this.kind = source["kind"];
	        this.request = source["request"];
	        this.baseRevision = source["baseRevision"];
	        this.connectionId = source["connectionId"];
	        this.model = source["model"];
	        this.status = source["status"];
	        this.result = source["result"];
	        this.error = source["error"];
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	        this.baseWorkflow = source["baseWorkflow"];
	        this.parentId = source["parentId"];
	        this.scopeNodeId = source["scopeNodeId"];
	    }
	}
	export class AutoStart {
	    id: string;
	    projectId: string;
	    projectName: string;
	    workItemId: string;
	    workTitle: string;
	    departmentId: string;
	    causeSequence: number;
	    status: string;
	    planJobId: string;
	    runId: string;
	    reason: string;
	    note: string;
	    createdAt: string;
	    updatedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new AutoStart(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.projectId = source["projectId"];
	        this.projectName = source["projectName"];
	        this.workItemId = source["workItemId"];
	        this.workTitle = source["workTitle"];
	        this.departmentId = source["departmentId"];
	        this.causeSequence = source["causeSequence"];
	        this.status = source["status"];
	        this.planJobId = source["planJobId"];
	        this.runId = source["runId"];
	        this.reason = source["reason"];
	        this.note = source["note"];
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	    }
	}
	export class AutoStartFilter {
	    workItemId: string;
	    status: string;
	
	    static createFrom(source: any = {}) {
	        return new AutoStartFilter(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.workItemId = source["workItemId"];
	        this.status = source["status"];
	    }
	}
	export class CollabAcceptance {
	    existingWorkItemId: string;
	    title: string;
	    description: string;
	    doneCriteria: string;
	    priority: string;
	    note: string;
	
	    static createFrom(source: any = {}) {
	        return new CollabAcceptance(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.existingWorkItemId = source["existingWorkItemId"];
	        this.title = source["title"];
	        this.description = source["description"];
	        this.doneCriteria = source["doneCriteria"];
	        this.priority = source["priority"];
	        this.note = source["note"];
	    }
	}
	export class CollabFilter {
	    workItemId: string;
	    resultWorkItemId: string;
	    departmentId: string;
	    open: boolean;
	    aiUnreviewed: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CollabFilter(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.workItemId = source["workItemId"];
	        this.resultWorkItemId = source["resultWorkItemId"];
	        this.departmentId = source["departmentId"];
	        this.open = source["open"];
	        this.aiUnreviewed = source["aiUnreviewed"];
	    }
	}
	export class CollabHistory {
	    action: string;
	    body: string;
	    asDepartmentId: string;
	    author: string;
	    createdAt: string;
	
	    static createFrom(source: any = {}) {
	        return new CollabHistory(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.action = source["action"];
	        this.body = source["body"];
	        this.asDepartmentId = source["asDepartmentId"];
	        this.author = source["author"];
	        this.createdAt = source["createdAt"];
	    }
	}
	export class CollabRequest {
	    id: string;
	    projectId: string;
	    projectName: string;
	    fromWorkItemId: string;
	    fromWorkTitle: string;
	    fromWorkStatus: string;
	    goalId: string;
	    fromDepartmentId: string;
	    fromDepartmentName: string;
	    toDepartmentId: string;
	    toDepartmentName: string;
	    question: string;
	    inputVersionIds: string[];
	    requirement: string;
	    requirementReason: string;
	    status: string;
	    followUp: string;
	    decidedBy: string;
	    decisionJobId: string;
	    decisionReason: string;
	    decidedAt: string;
	    reviewedAt: string;
	    revertedAt: string;
	    revertReason: string;
	    aiStatus: string;
	    aiProblem: string;
	    resultWorkItemId: string;
	    resultWorkTitle: string;
	    resultWorkStatus: string;
	    replacesId: string;
	    replacedById: string;
	    dueAt: string;
	    eventSequence: number;
	    satisfied: boolean;
	    overdue: boolean;
	    history: CollabHistory[];
	    createdAt: string;
	    updatedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new CollabRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.projectId = source["projectId"];
	        this.projectName = source["projectName"];
	        this.fromWorkItemId = source["fromWorkItemId"];
	        this.fromWorkTitle = source["fromWorkTitle"];
	        this.fromWorkStatus = source["fromWorkStatus"];
	        this.goalId = source["goalId"];
	        this.fromDepartmentId = source["fromDepartmentId"];
	        this.fromDepartmentName = source["fromDepartmentName"];
	        this.toDepartmentId = source["toDepartmentId"];
	        this.toDepartmentName = source["toDepartmentName"];
	        this.question = source["question"];
	        this.inputVersionIds = source["inputVersionIds"];
	        this.requirement = source["requirement"];
	        this.requirementReason = source["requirementReason"];
	        this.status = source["status"];
	        this.followUp = source["followUp"];
	        this.decidedBy = source["decidedBy"];
	        this.decisionJobId = source["decisionJobId"];
	        this.decisionReason = source["decisionReason"];
	        this.decidedAt = source["decidedAt"];
	        this.reviewedAt = source["reviewedAt"];
	        this.revertedAt = source["revertedAt"];
	        this.revertReason = source["revertReason"];
	        this.aiStatus = source["aiStatus"];
	        this.aiProblem = source["aiProblem"];
	        this.resultWorkItemId = source["resultWorkItemId"];
	        this.resultWorkTitle = source["resultWorkTitle"];
	        this.resultWorkStatus = source["resultWorkStatus"];
	        this.replacesId = source["replacesId"];
	        this.replacedById = source["replacedById"];
	        this.dueAt = source["dueAt"];
	        this.eventSequence = source["eventSequence"];
	        this.satisfied = source["satisfied"];
	        this.overdue = source["overdue"];
	        this.history = this.convertValues(source["history"], CollabHistory);
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
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
	export class WorkEvent {
	    departmentId: string;
	    targetDepartmentId?: string;
	    collabRequestId?: string;
	    workItemId?: string;
	    goalId?: string;
	    title?: string;
	    note?: string;
	    categories: string[];
	    causeSequence?: number;
	    chainRootSequence?: number;
	    chainDepth: number;
	    automated: boolean;
	    documentId?: string;
	    documentVersionId?: string;
	    scheduleSlot?: string;
	
	    static createFrom(source: any = {}) {
	        return new WorkEvent(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.departmentId = source["departmentId"];
	        this.targetDepartmentId = source["targetDepartmentId"];
	        this.collabRequestId = source["collabRequestId"];
	        this.workItemId = source["workItemId"];
	        this.goalId = source["goalId"];
	        this.title = source["title"];
	        this.note = source["note"];
	        this.categories = source["categories"];
	        this.causeSequence = source["causeSequence"];
	        this.chainRootSequence = source["chainRootSequence"];
	        this.chainDepth = source["chainDepth"];
	        this.automated = source["automated"];
	        this.documentId = source["documentId"];
	        this.documentVersionId = source["documentVersionId"];
	        this.scheduleSlot = source["scheduleSlot"];
	    }
	}
	export class Delivery {
	    id: string;
	    projectId: string;
	    eventSequence: number;
	    eventKind: string;
	    event: WorkEvent;
	    eventAt: string;
	    departmentId: string;
	    subscriptionId: string;
	    purpose: string;
	    status: string;
	    note: string;
	    error: string;
	    holdReason: string;
	    createdAt: string;
	    updatedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new Delivery(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.projectId = source["projectId"];
	        this.eventSequence = source["eventSequence"];
	        this.eventKind = source["eventKind"];
	        this.event = this.convertValues(source["event"], WorkEvent);
	        this.eventAt = source["eventAt"];
	        this.departmentId = source["departmentId"];
	        this.subscriptionId = source["subscriptionId"];
	        this.purpose = source["purpose"];
	        this.status = source["status"];
	        this.note = source["note"];
	        this.error = source["error"];
	        this.holdReason = source["holdReason"];
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
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
	export class DeliveryFilter {
	    departmentId: string;
	    status: string;
	    limit: number;
	
	    static createFrom(source: any = {}) {
	        return new DeliveryFilter(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.departmentId = source["departmentId"];
	        this.status = source["status"];
	        this.limit = source["limit"];
	    }
	}
	export class DeliveryTrouble {
	    id: string;
	    departmentName: string;
	    status: string;
	    reason: string;
	    eventTitle: string;
	    updatedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new DeliveryTrouble(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.departmentName = source["departmentName"];
	        this.status = source["status"];
	        this.reason = source["reason"];
	        this.eventTitle = source["eventTitle"];
	        this.updatedAt = source["updatedAt"];
	    }
	}
	export class WorkRef {
	    id: string;
	    title: string;
	    status: string;
	    reason: string;
	
	    static createFrom(source: any = {}) {
	        return new WorkRef(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.status = source["status"];
	        this.reason = source["reason"];
	    }
	}
	export class DepartmentLoad {
	    departmentId: string;
	    name: string;
	    status: string;
	    open: number;
	    waiting: number;
	    blocked: number;
	    resultsToCheck: number;
	    proposals: number;
	    questions: number;
	    collabIncoming: number;
	    autoReviews: number;
	    failedDeliveries: number;
	    heldDeliveries: number;
	    attention: WorkRef[];
	
	    static createFrom(source: any = {}) {
	        return new DepartmentLoad(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.departmentId = source["departmentId"];
	        this.name = source["name"];
	        this.status = source["status"];
	        this.open = source["open"];
	        this.waiting = source["waiting"];
	        this.blocked = source["blocked"];
	        this.resultsToCheck = source["resultsToCheck"];
	        this.proposals = source["proposals"];
	        this.questions = source["questions"];
	        this.collabIncoming = source["collabIncoming"];
	        this.autoReviews = source["autoReviews"];
	        this.failedDeliveries = source["failedDeliveries"];
	        this.heldDeliveries = source["heldDeliveries"];
	        this.attention = this.convertValues(source["attention"], WorkRef);
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
	export class DesignApplied {
	    projectId: string;
	    roleIds: Record<string, string>;
	    assignmentIds: Record<string, string>;
	    workflowId: string;
	    newProject: boolean;
	
	    static createFrom(source: any = {}) {
	        return new DesignApplied(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.projectId = source["projectId"];
	        this.roleIds = source["roleIds"];
	        this.assignmentIds = source["assignmentIds"];
	        this.workflowId = source["workflowId"];
	        this.newProject = source["newProject"];
	    }
	}
	export class DocumentFilter {
	    query: string;
	    category: string;
	    ownerDepartmentId: string;
	    visibleTo: string;
	    stage: string;
	    includeWithdrawn: boolean;
	
	    static createFrom(source: any = {}) {
	        return new DocumentFilter(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.query = source["query"];
	        this.category = source["category"];
	        this.ownerDepartmentId = source["ownerDepartmentId"];
	        this.visibleTo = source["visibleTo"];
	        this.stage = source["stage"];
	        this.includeWithdrawn = source["includeWithdrawn"];
	    }
	}
	export class DocumentVersion {
	    id: string;
	    documentId: string;
	    projectId: string;
	    number: number;
	    stage: string;
	    confirmedBy: string;
	    confirmedAt: string;
	    confirmationNote: string;
	    sourceKind: string;
	    sourceArtifactId: string;
	    sourcePath: string;
	    fileName: string;
	    path: string;
	    hash: string;
	    size: number;
	    withdrawnAt: string;
	    createdAt: string;
	
	    static createFrom(source: any = {}) {
	        return new DocumentVersion(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.documentId = source["documentId"];
	        this.projectId = source["projectId"];
	        this.number = source["number"];
	        this.stage = source["stage"];
	        this.confirmedBy = source["confirmedBy"];
	        this.confirmedAt = source["confirmedAt"];
	        this.confirmationNote = source["confirmationNote"];
	        this.sourceKind = source["sourceKind"];
	        this.sourceArtifactId = source["sourceArtifactId"];
	        this.sourcePath = source["sourcePath"];
	        this.fileName = source["fileName"];
	        this.path = source["path"];
	        this.hash = source["hash"];
	        this.size = source["size"];
	        this.withdrawnAt = source["withdrawnAt"];
	        this.createdAt = source["createdAt"];
	    }
	}
	
	export class GoalRef {
	    id: string;
	    title: string;
	
	    static createFrom(source: any = {}) {
	        return new GoalRef(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	    }
	}
	export class ImpactCheck {
	    id: string;
	    projectId: string;
	    workItemId: string;
	    cause: string;
	    refId: string;
	    documentId: string;
	    summary: string;
	    status: string;
	    note: string;
	    createdAt: string;
	
	    static createFrom(source: any = {}) {
	        return new ImpactCheck(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.projectId = source["projectId"];
	        this.workItemId = source["workItemId"];
	        this.cause = source["cause"];
	        this.refId = source["refId"];
	        this.documentId = source["documentId"];
	        this.summary = source["summary"];
	        this.status = source["status"];
	        this.note = source["note"];
	        this.createdAt = source["createdAt"];
	    }
	}
	export class Judgment {
	    id: string;
	    projectId: string;
	    projectName: string;
	    deliveryId: string;
	    round: number;
	    aiJobId: string;
	    departmentId: string;
	    departmentName: string;
	    outcome: string;
	    reason: string;
	    evidence: string[];
	    question: string;
	    answer: string;
	    questionStatus: string;
	    eventSequence: number;
	    eventKind: string;
	    event: WorkEvent;
	    createdAt: string;
	
	    static createFrom(source: any = {}) {
	        return new Judgment(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.projectId = source["projectId"];
	        this.projectName = source["projectName"];
	        this.deliveryId = source["deliveryId"];
	        this.round = source["round"];
	        this.aiJobId = source["aiJobId"];
	        this.departmentId = source["departmentId"];
	        this.departmentName = source["departmentName"];
	        this.outcome = source["outcome"];
	        this.reason = source["reason"];
	        this.evidence = source["evidence"];
	        this.question = source["question"];
	        this.answer = source["answer"];
	        this.questionStatus = source["questionStatus"];
	        this.eventSequence = source["eventSequence"];
	        this.eventKind = source["eventKind"];
	        this.event = this.convertValues(source["event"], WorkEvent);
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
	export class SharedDocument {
	    id: string;
	    projectId: string;
	    title: string;
	    categories: string[];
	    ownerDepartmentId: string;
	    workItemId: string;
	    scope: string;
	    scopeDepartments: string[];
	    status: string;
	    createdAt: string;
	    updatedAt: string;
	    versions: DocumentVersion[];
	
	    static createFrom(source: any = {}) {
	        return new SharedDocument(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.projectId = source["projectId"];
	        this.title = source["title"];
	        this.categories = source["categories"];
	        this.ownerDepartmentId = source["ownerDepartmentId"];
	        this.workItemId = source["workItemId"];
	        this.scope = source["scope"];
	        this.scopeDepartments = source["scopeDepartments"];
	        this.status = source["status"];
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	        this.versions = this.convertValues(source["versions"], DocumentVersion);
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
	export class LinkedDocument {
	    version: DocumentVersion;
	    document: SharedDocument;
	    newer: number;
	    linkedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new LinkedDocument(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = this.convertValues(source["version"], DocumentVersion);
	        this.document = this.convertValues(source["document"], SharedDocument);
	        this.newer = source["newer"];
	        this.linkedAt = source["linkedAt"];
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
	export class LinkedWork {
	    id: string;
	    title: string;
	    status: string;
	    departmentName: string;
	    via: string;
	
	    static createFrom(source: any = {}) {
	        return new LinkedWork(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.status = source["status"];
	        this.departmentName = source["departmentName"];
	        this.via = source["via"];
	    }
	}
	export class NewCollabRequest {
	    fromWorkItemId: string;
	    toDepartmentId: string;
	    question: string;
	    inputVersionIds: string[];
	    requirement: string;
	    requirementReason: string;
	    dueAt: string;
	
	    static createFrom(source: any = {}) {
	        return new NewCollabRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.fromWorkItemId = source["fromWorkItemId"];
	        this.toDepartmentId = source["toDepartmentId"];
	        this.question = source["question"];
	        this.inputVersionIds = source["inputVersionIds"];
	        this.requirement = source["requirement"];
	        this.requirementReason = source["requirementReason"];
	        this.dueAt = source["dueAt"];
	    }
	}
	export class NewDocumentVersion {
	    documentId: string;
	    title: string;
	    categories: string[];
	    ownerDepartmentId: string;
	    workItemId: string;
	    scope: string;
	    scopeDepartments: string[];
	
	    static createFrom(source: any = {}) {
	        return new NewDocumentVersion(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.documentId = source["documentId"];
	        this.title = source["title"];
	        this.categories = source["categories"];
	        this.ownerDepartmentId = source["ownerDepartmentId"];
	        this.workItemId = source["workItemId"];
	        this.scope = source["scope"];
	        this.scopeDepartments = source["scopeDepartments"];
	    }
	}
	export class Proposal {
	    id: string;
	    projectId: string;
	    projectName: string;
	    judgmentId: string;
	    position: number;
	    departmentId: string;
	    departmentName: string;
	    title: string;
	    description: string;
	    doneCriteria: string;
	    categories: string[];
	    priority: string;
	    status: string;
	    workItemId: string;
	    duplicateOf: string;
	    decisionNote: string;
	    reason: string;
	    eventSequence: number;
	    eventKind: string;
	    eventTitle: string;
	    eventWorkItemId: string;
	    eventDocumentId: string;
	    createdAt: string;
	
	    static createFrom(source: any = {}) {
	        return new Proposal(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.projectId = source["projectId"];
	        this.projectName = source["projectName"];
	        this.judgmentId = source["judgmentId"];
	        this.position = source["position"];
	        this.departmentId = source["departmentId"];
	        this.departmentName = source["departmentName"];
	        this.title = source["title"];
	        this.description = source["description"];
	        this.doneCriteria = source["doneCriteria"];
	        this.categories = source["categories"];
	        this.priority = source["priority"];
	        this.status = source["status"];
	        this.workItemId = source["workItemId"];
	        this.duplicateOf = source["duplicateOf"];
	        this.decisionNote = source["decisionNote"];
	        this.reason = source["reason"];
	        this.eventSequence = source["eventSequence"];
	        this.eventKind = source["eventKind"];
	        this.eventTitle = source["eventTitle"];
	        this.eventWorkItemId = source["eventWorkItemId"];
	        this.eventDocumentId = source["eventDocumentId"];
	        this.createdAt = source["createdAt"];
	    }
	}
	export class ProposalEdit {
	    title: string;
	    description: string;
	    doneCriteria: string;
	    categories: string[];
	    priority: string;
	
	    static createFrom(source: any = {}) {
	        return new ProposalEdit(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.title = source["title"];
	        this.description = source["description"];
	        this.doneCriteria = source["doneCriteria"];
	        this.categories = source["categories"];
	        this.priority = source["priority"];
	    }
	}
	export class RequestFilter {
	    projectId: string;
	    all: boolean;
	    statuses: string[];
	    limit: number;
	
	    static createFrom(source: any = {}) {
	        return new RequestFilter(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.projectId = source["projectId"];
	        this.all = source["all"];
	        this.statuses = source["statuses"];
	        this.limit = source["limit"];
	    }
	}
	export class RequiredNotice {
	    id: string;
	    projectId: string;
	    workItemId: string;
	    cause: string;
	    refId: string;
	    documentId: string;
	    summary: string;
	    status: string;
	    note: string;
	    createdAt: string;
	    projectName: string;
	    workTitle: string;
	    workKind: string;
	
	    static createFrom(source: any = {}) {
	        return new RequiredNotice(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.projectId = source["projectId"];
	        this.workItemId = source["workItemId"];
	        this.cause = source["cause"];
	        this.refId = source["refId"];
	        this.documentId = source["documentId"];
	        this.summary = source["summary"];
	        this.status = source["status"];
	        this.note = source["note"];
	        this.createdAt = source["createdAt"];
	        this.projectName = source["projectName"];
	        this.workTitle = source["workTitle"];
	        this.workKind = source["workKind"];
	    }
	}
	export class ResultReview {
	    workItemId: string;
	    projectId: string;
	    projectName: string;
	    title: string;
	    departmentName: string;
	    goalId: string;
	    runId: string;
	    finishedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new ResultReview(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.workItemId = source["workItemId"];
	        this.projectId = source["projectId"];
	        this.projectName = source["projectName"];
	        this.title = source["title"];
	        this.departmentName = source["departmentName"];
	        this.goalId = source["goalId"];
	        this.runId = source["runId"];
	        this.finishedAt = source["finishedAt"];
	    }
	}
	export class ScheduleView {
	    id: string;
	    projectId: string;
	    departmentId: string;
	    frequency: string;
	    timeOfDay: string;
	    weekday?: number;
	    timezone: string;
	    note: string;
	    enabled: boolean;
	    activeFrom: string;
	    createdAt: string;
	    updatedAt: string;
	    departmentName: string;
	    lastSlotAt: string;
	    lastSequence: number;
	    nextDueAt: string;
	
	    static createFrom(source: any = {}) {
	        return new ScheduleView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.projectId = source["projectId"];
	        this.departmentId = source["departmentId"];
	        this.frequency = source["frequency"];
	        this.timeOfDay = source["timeOfDay"];
	        this.weekday = source["weekday"];
	        this.timezone = source["timezone"];
	        this.note = source["note"];
	        this.enabled = source["enabled"];
	        this.activeFrom = source["activeFrom"];
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	        this.departmentName = source["departmentName"];
	        this.lastSlotAt = source["lastSlotAt"];
	        this.lastSequence = source["lastSequence"];
	        this.nextDueAt = source["nextDueAt"];
	    }
	}
	
	export class Staffing {
	    id: string;
	    projectId: string;
	    requestId: string;
	    kind: string;
	    connectionId: string;
	    model: string;
	    status: string;
	    result: number[];
	    error: string;
	    startedAt: string;
	    endedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new Staffing(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.projectId = source["projectId"];
	        this.requestId = source["requestId"];
	        this.kind = source["kind"];
	        this.connectionId = source["connectionId"];
	        this.model = source["model"];
	        this.status = source["status"];
	        this.result = source["result"];
	        this.error = source["error"];
	        this.startedAt = source["startedAt"];
	        this.endedAt = source["endedAt"];
	    }
	}
	export class StartApproval {
	    by: string;
	    at: string;
	    valid: boolean;
	
	    static createFrom(source: any = {}) {
	        return new StartApproval(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.by = source["by"];
	        this.at = source["at"];
	        this.valid = source["valid"];
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
	export class WorkCause {
	    via: string;
	    proposalId?: string;
	    collabRequestId?: string;
	    eventSequence: number;
	    eventKind: string;
	    eventTitle: string;
	    eventNote: string;
	    scheduleSlot?: string;
	    fromWorkItemId?: string;
	    fromWorkTitle?: string;
	    fromDepartmentName?: string;
	    documentId?: string;
	    documentTitle?: string;
	    departmentName: string;
	    reason: string;
	    decidedBy: string;
	    requestId?: string;
	    requestBody?: string;
	
	    static createFrom(source: any = {}) {
	        return new WorkCause(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.via = source["via"];
	        this.proposalId = source["proposalId"];
	        this.collabRequestId = source["collabRequestId"];
	        this.eventSequence = source["eventSequence"];
	        this.eventKind = source["eventKind"];
	        this.eventTitle = source["eventTitle"];
	        this.eventNote = source["eventNote"];
	        this.scheduleSlot = source["scheduleSlot"];
	        this.fromWorkItemId = source["fromWorkItemId"];
	        this.fromWorkTitle = source["fromWorkTitle"];
	        this.fromDepartmentName = source["fromDepartmentName"];
	        this.documentId = source["documentId"];
	        this.documentTitle = source["documentTitle"];
	        this.departmentName = source["departmentName"];
	        this.reason = source["reason"];
	        this.decidedBy = source["decidedBy"];
	        this.requestId = source["requestId"];
	        this.requestBody = source["requestBody"];
	    }
	}
	
	export class WorkEventView {
	    sequence: number;
	    kind: string;
	    event: WorkEvent;
	    createdAt: string;
	    deliveries: Delivery[];
	
	    static createFrom(source: any = {}) {
	        return new WorkEventView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sequence = source["sequence"];
	        this.kind = source["kind"];
	        this.event = this.convertValues(source["event"], WorkEvent);
	        this.createdAt = source["createdAt"];
	        this.deliveries = this.convertValues(source["deliveries"], Delivery);
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
	export class WorkFilter {
	    kind: string;
	    status: string;
	    priority: string;
	    category: string;
	    departmentId: string;
	    parentId: string;
	    dueBefore: string;
	
	    static createFrom(source: any = {}) {
	        return new WorkFilter(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.status = source["status"];
	        this.priority = source["priority"];
	        this.category = source["category"];
	        this.departmentId = source["departmentId"];
	        this.parentId = source["parentId"];
	        this.dueBefore = source["dueBefore"];
	    }
	}
	export class WorkLineage {
	    causes: WorkCause[];
	    next: LinkedWork[];
	
	    static createFrom(source: any = {}) {
	        return new WorkLineage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.causes = this.convertValues(source["causes"], WorkCause);
	        this.next = this.convertValues(source["next"], LinkedWork);
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
	
	export class WorkflowChoice {
	    versionId: string;
	    workflowId: string;
	    title: string;
	    number: number;
	    createdAt: string;
	
	    static createFrom(source: any = {}) {
	        return new WorkflowChoice(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.versionId = source["versionId"];
	        this.workflowId = source["workflowId"];
	        this.title = source["title"];
	        this.number = source["number"];
	        this.createdAt = source["createdAt"];
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

export namespace work {
	
	export class ProposedPlan {
	    assignmentId: string;
	    instructions: string;
	    materials: string;
	    outputs: domain.Output[];
	    notes: string;
	
	    static createFrom(source: any = {}) {
	        return new ProposedPlan(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.assignmentId = source["assignmentId"];
	        this.instructions = source["instructions"];
	        this.materials = source["materials"];
	        this.outputs = this.convertValues(source["outputs"], domain.Output);
	        this.notes = source["notes"];
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

export namespace workspace {
	
	export class Stat {
	    files: number;
	    insertions: number;
	    deletions: number;
	
	    static createFrom(source: any = {}) {
	        return new Stat(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.files = source["files"];
	        this.insertions = source["insertions"];
	        this.deletions = source["deletions"];
	    }
	}

}

