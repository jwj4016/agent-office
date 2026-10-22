package domain

import "encoding/json"

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
	CreatedAt            string   `json:"createdAt"`
	UpdatedAt            string   `json:"updatedAt"`
	ClosedAt             string   `json:"closedAt"`
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
