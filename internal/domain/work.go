package domain

import (
	"encoding/json"
	"slices"
)

// Department statuses.
const (
	StatusActive   = "active"
	StatusInactive = "inactive"
)

// DepartmentTemplate is a company-wide starting point for a department.
// Copying it into a service makes an independent department.
type DepartmentTemplate struct {
	ID             string   `json:"id"`
	OrganizationID string   `json:"organizationId"`
	Name           string   `json:"name"`
	Responsibility string   `json:"responsibility"`
	RoleIDs        []string `json:"roleIds"` // roles that usually work in it
	UpdatedAt      string   `json:"updatedAt"`
}

// Department is a unit inside one service (a service is one company).
// Its members are the service's assignments; each assignment belongs to
// at most one department, and the lead is one of the members.
type Department struct {
	ID               string          `json:"id"`
	ProjectID        string          `json:"projectId"`
	Name             string          `json:"name"`
	Responsibility   string          `json:"responsibility"`
	LeadAssignmentID string          `json:"leadAssignmentId"`
	MemberIDs        []string        `json:"memberIds"` // assignment ids
	TemplateID       string          `json:"templateId"`
	Automation       json.RawMessage `json:"automation"`
	Limits           json.RawMessage `json:"limits"`
	Status           string          `json:"status"` // active | inactive
	CreatedAt        string          `json:"createdAt"`
	UpdatedAt        string          `json:"updatedAt"`
}

// DefaultCategories is a new service's category list.
var DefaultCategories = []string{"기획", "디자인", "개발", "비용", "법무", "운영", "기타"}

// Work item kinds: a shared goal groups the work of several departments.
const (
	WorkGoal = "goal"
	WorkTask = "work"
)

// Work priorities, most urgent first.
var WorkPriorities = []string{"urgent", "high", "normal", "low"}

// Work statuses. Waiting and blocked need a reason; done and cancelled
// close the item until it is reopened.
const (
	WorkReceived   = "received"
	WorkReady      = "ready"
	WorkInProgress = "in_progress"
	WorkWaiting    = "waiting"
	WorkBlocked    = "blocked"
	WorkDone       = "done"
	WorkCancelled  = "cancelled"
)

var WorkStatuses = []string{WorkReceived, WorkReady, WorkInProgress, WorkWaiting, WorkBlocked, WorkDone, WorkCancelled}

// WorkClosed reports whether a status closes the item.
func WorkClosed(status string) bool { return status == WorkDone || status == WorkCancelled }

// WorkNeedsReason reports whether a status must say why.
func WorkNeedsReason(status string) bool { return status == WorkWaiting || status == WorkBlocked }

// WorkItem is work that stays until it is resolved. A goal (kind
// "goal") is the parent of other departments' work items.
type WorkItem struct {
	ID                   string   `json:"id"`
	ProjectID            string   `json:"projectId"`
	Kind                 string   `json:"kind"` // goal | work
	ParentID             string   `json:"parentId"`
	DepartmentID         string   `json:"departmentId"`
	AssigneeAssignmentID string   `json:"assigneeAssignmentId"`
	Title                string   `json:"title"`
	Description          string   `json:"description"`
	DoneCriteria         string   `json:"doneCriteria"`
	Priority             string   `json:"priority"`
	Status               string   `json:"status"`
	StatusReason         string   `json:"statusReason"`
	Categories           []string `json:"categories"`
	DueAt                string   `json:"dueAt"` // YYYY-MM-DD or empty
	// ReviewBeforeStart keeps a person's review before every start, even
	// when the service and department would start it on their own.
	ReviewBeforeStart bool   `json:"reviewBeforeStart"`
	CreatedAt         string `json:"createdAt"`
	UpdatedAt         string `json:"updatedAt"`
	ClosedAt          string `json:"closedAt"`
	// OriginRequestID is the request the work was registered from.
	OriginRequestID string `json:"originRequestId,omitempty"`
	// AfterIDs are the work items this one waits for: automatic start
	// holds it until they are all closed.
	AfterIDs []string `json:"afterIds,omitempty"`
}

// Work note kinds: a person's memo, or a status change kept as history.
const (
	NoteMemo   = "note"
	NoteStatus = "status"
)

// WorkNote is one entry in a work item's history.
type WorkNote struct {
	ID         string `json:"id"`
	WorkItemID string `json:"workItemId"`
	Kind       string `json:"kind"` // note | status
	Body       string `json:"body"`
	FromStatus string `json:"fromStatus,omitempty"`
	ToStatus   string `json:"toStatus,omitempty"`
	Author     string `json:"author"`
	CreatedAt  string `json:"createdAt"`
}

// Work event kinds (plan §4.2). They are stored in execution_events
// without a run and are told apart from run events by their prefix.
const (
	EventWorkCreated         = "work.created"
	EventWorkCollabRequested = "work.collab_requested"
	EventWorkCompleted       = "work.completed"
	EventWorkReopened        = "work.reopened"
	EventDocumentShared      = "document.shared"
	EventDocumentRevised     = "document.revised"
	EventDocumentConfirmed   = "document.confirmed"
	EventDocumentWithdrawn   = "document.withdrawn"
	EventDepartmentCheck     = "department.check_requested"
)

// SubscribableEvents are the kinds a department can subscribe to.
// Direct requests (check, collaboration) reach their target without one.
var SubscribableEvents = []string{
	EventWorkCreated, EventWorkCompleted, EventWorkReopened,
	EventDocumentShared, EventDocumentRevised, EventDocumentConfirmed, EventDocumentWithdrawn,
}

// IsWorkEvent reports whether an event kind belongs to service operations.
func IsWorkEvent(kind string) bool {
	for _, p := range []string{"work.", "document.", "department."} {
		if len(kind) > len(p) && kind[:len(p)] == p {
			return true
		}
	}
	return false
}

// Subscription is what a department wants to hear about: event kinds,
// optionally narrowed to categories, and the point of view it judges from.
type Subscription struct {
	ID           string   `json:"id"`
	ProjectID    string   `json:"projectId"`
	DepartmentID string   `json:"departmentId"`
	EventKinds   []string `json:"eventKinds"`
	Categories   []string `json:"categories"` // empty: every category
	Guidance     string   `json:"guidance"`
	Enabled      bool     `json:"enabled"`
	UpdatedAt    string   `json:"updatedAt"`
}

// Delivery purposes and statuses.
const (
	PurposeJudge         = "judge"
	PurposeNotify        = "notify"
	PurposeCollabRequest = "collab_request"

	DeliveryPending       = "pending"
	DeliveryProcessing    = "processing"
	DeliveryWaitingAnswer = "waiting_answer"
	DeliveryDone          = "done"
	DeliverySkipped       = "skipped"
	DeliveryHeld          = "held"
	DeliveryFailed        = "failed"
)

// DefaultJudgeFanout is how many departments one event may send to AI
// judgment through subscriptions; the rest are held (plan §4.3, §9.3).
const DefaultJudgeFanout = 5

// WorkPlan is how one work item will be done: who does it, extra
// instructions, materials to read, and the outputs to produce. Running it
// makes a one-step workflow version (plan §4.6).
//
// With WorkflowVersionID the work runs that confirmed version of one of
// the service's workflows as it is, for repeated work (plan §4.6).
type WorkPlan struct {
	WorkItemID        string   `json:"workItemId"`
	ProjectID         string   `json:"projectId"`
	AssignmentID      string   `json:"assignmentId"`
	Instructions      string   `json:"instructions"`
	Materials         string   `json:"materials"`
	Outputs           []Output `json:"outputs"`
	WorkflowVersionID string   `json:"workflowVersionId"`
	UpdatedAt         string   `json:"updatedAt"`
}

// WorkStepID is the id of the single step in a work item's workflow.
const WorkStepID = "work"

// DefaultWorkOutputs is what a plan produces when it names nothing.
func DefaultWorkOutputs() []Output { return []Output{{Key: "result", Type: OutMarkdown}} }

// DefaultChainDepth is how deep automatic follow-up judgment goes in one
// chain of events (plan §4.7, §9.3).
const DefaultChainDepth = 3

// DefaultChainWork is how many work items automatic processing may
// register in one chain of events (plan §4.7, §9.3).
const DefaultChainWork = 10

// DepartmentAutomation is what a department lets happen without a person
// (plan §4.6). Everything is off for a new department.
type DepartmentAutomation struct {
	// AutoStart lets work made from events start on its own when the
	// service is in auto mode and the rest of these rules allow it.
	AutoStart bool `json:"autoStart"`
	// Categories narrows AutoStart to work whose categories are all in
	// the list; empty allows every category.
	Categories []string `json:"categories"`
	// ConnectionIDs are the AI connections (tools) automatic work may use,
	// on top of the service's auto policy; empty adds no limit.
	ConnectionIDs []string `json:"connectionIds"`
	// OutputTypes is the scope of automatic work: the result types it may
	// produce. Empty means DefaultAutoOutputTypes.
	OutputTypes []OutputType `json:"outputTypes"`
	// WorkflowVersionIDs are the confirmed workflow versions a work item
	// may run on its own.
	WorkflowVersionIDs []string `json:"workflowVersionIds"`
	// AutoShare shares the results of automatically started runs as
	// drafts.
	AutoShare bool `json:"autoShare"`
	// AutoCollab lets the lead AI accept, decline or question reference
	// collaboration requests. Required requests are always a person's.
	AutoCollab bool `json:"autoCollab"`
}

// DefaultAutoOutputTypes are results automatic work may produce unless a
// department says otherwise: documents only, nothing that changes files.
var DefaultAutoOutputTypes = []OutputType{OutMarkdown, OutJSON, OutReport}

// ParseAutomation reads a department's stored rules; anything unreadable
// is treated as everything off.
func ParseAutomation(raw json.RawMessage) DepartmentAutomation {
	var a DepartmentAutomation
	json.Unmarshal(raw, &a)
	a.Categories, a.ConnectionIDs, a.WorkflowVersionIDs = orNil(a.Categories), orNil(a.ConnectionIDs), orNil(a.WorkflowVersionIDs)
	if a.OutputTypes == nil {
		a.OutputTypes = []OutputType{}
	}
	return a
}

// AllowsOutput reports whether automatic work may produce t.
func (a DepartmentAutomation) AllowsOutput(t OutputType) bool {
	if len(a.OutputTypes) == 0 {
		return slices.Contains(DefaultAutoOutputTypes, t)
	}
	return slices.Contains(a.OutputTypes, t)
}

// AllowsCategories reports whether work with these categories may start
// on its own.
func (a DepartmentAutomation) AllowsCategories(cats []string) bool {
	if len(a.Categories) == 0 {
		return true
	}
	if len(cats) == 0 {
		return false
	}
	for _, c := range cats {
		if !slices.Contains(a.Categories, c) {
			return false
		}
	}
	return true
}

// AllowsConnection reports whether automatic work may use a connection,
// as far as the department is concerned.
func (a DepartmentAutomation) AllowsConnection(id string) bool {
	return len(a.ConnectionIDs) == 0 || slices.Contains(a.ConnectionIDs, id)
}

func orNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// ValidOutputType reports whether t is a result type workflows support.
func ValidOutputType(t OutputType) bool { return outputTypes[t] }
