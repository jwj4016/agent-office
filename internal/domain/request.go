package domain

import (
	"encoding/json"
	"slices"
)

// Company requests (docs/plan/company-requests/plan.md): a person asks the
// company or one of its services in words, and an intake desk decides what
// to do with it.

// Request statuses (plan §4.2).
const (
	ReqReceived    = "received"
	ReqWaitingDesk = "waiting_desk"
	ReqHeld        = "held"
	ReqTriaging    = "triaging"
	ReqQuestion    = "question"
	ReqManual      = "manual"
	ReqProposed    = "proposed"
	ReqRouted      = "routed"
	ReqInProgress  = "in_progress"
	ReqDone        = "done"
	ReqDeclined    = "declined"
	ReqCancelled   = "cancelled"
	ReqFailed      = "failed"
)

// RequestClosed reports whether a request is over.
func RequestClosed(status string) bool {
	return slices.Contains([]string{ReqDone, ReqDeclined, ReqCancelled, ReqFailed}, status)
}

// What a desk decided (plan §4.3).
const (
	OutcomeAnswer   = "answer"
	OutcomeRoute    = "route"
	OutcomeWork     = "work"
	OutcomeAction   = "action"
	OutcomeQuestion = "question"
	OutcomeDecline  = "decline"
)

// Request item kinds and statuses.
const (
	ItemRoute  = "route"
	ItemWork   = "work"
	ItemAction = "action"

	ItemProposed = "proposed"
	ItemAccepted = "accepted"
	ItemRejected = "rejected"
	ItemRunning  = "running"
	ItemDone     = "done"
	ItemFailed   = "failed"
	ItemCheck    = "check"
)

// Actions the app carries out for a request once a person approves
// (plan §4.5). A desk only picks one and names what it applies to.
const (
	ActionDeliverCode   = "deliver_code"
	ActionSaveResult    = "save_result"
	ActionExportService = "export_service"
	ActionStartWorkflow = "start_workflow"
)

// Actions lists every action in the order desks are told about them.
var Actions = []string{ActionDeliverCode, ActionSaveResult, ActionExportService, ActionStartWorkflow}

// IntakeDesk is the AI that takes requests: one for the company
// (ProjectID "") and one per service.
type IntakeDesk struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organizationId"`
	ProjectID      string `json:"projectId"`
	Name           string `json:"name"`
	// AssignmentID is a service desk's AI assignee.
	AssignmentID string `json:"assignmentId"`
	// ConnectionID and Model are the company desk's own.
	ConnectionID string     `json:"connectionId"`
	Model        string     `json:"model"`
	Instructions string     `json:"instructions"`
	Budget       DeskBudget `json:"budget"`
	Enabled      bool       `json:"enabled"`
	CreatedAt    string     `json:"createdAt"`
	UpdatedAt    string     `json:"updatedAt"`
}

// DeskBudget is the company desk's monthly limit on reported cost.
type DeskBudget struct {
	MonthlyCostUSD *float64 `json:"monthlyCostUsd,omitempty"`
}

// Request is one thing a person asked.
type Request struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organizationId"`
	ProjectID      string `json:"projectId"`
	ProjectName    string `json:"projectName"`
	ParentID       string `json:"parentId"`
	PreviousID     string `json:"previousId"`
	Body           string `json:"body"`
	Status         string `json:"status"`
	Outcome        string `json:"outcome"`
	Round          int    `json:"round"`
	Answer         string `json:"answer"`
	Question       string `json:"question"`
	Reply          string `json:"reply"`
	Reason         string `json:"reason"`
	Error          string `json:"error"`
	HoldReason     string `json:"holdReason"`
	MaxAttempts    int    `json:"maxAttempts"`
	// Attempts counts the provider calls of the current round.
	Attempts  int    `json:"attempts"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
	ClosedAt  string `json:"closedAt"`
	// StaffedDesk names the desk an AI set up because of this request.
	StaffedDesk string        `json:"staffedDesk,omitempty"`
	Items       []RequestItem `json:"items"`
	Children    []Request     `json:"children"`
}

// RequestItem is one route, piece of work or action a desk asked for.
type RequestItem struct {
	ID             string          `json:"id"`
	RequestID      string          `json:"requestId"`
	ProjectID      string          `json:"projectId"`
	Round          int             `json:"round"`
	Position       int             `json:"position"`
	Kind           string          `json:"kind"`
	Payload        json.RawMessage `json:"payload"`
	Status         string          `json:"status"`
	WorkItemID     string          `json:"workItemId"`
	WorkTitle      string          `json:"workTitle"`
	WorkStatus     string          `json:"workStatus"`
	ChildRequestID string          `json:"childRequestId"`
	Result         json.RawMessage `json:"result"`
	Error          string          `json:"error"`
	DecidedBy      string          `json:"decidedBy"`
	DecisionNote   string          `json:"decisionNote"`
	DecidedAt      string          `json:"decidedAt"`
	CreatedAt      string          `json:"createdAt"`
}

// WorkProposalPayload is the work a desk proposes.
type WorkProposalPayload struct {
	Title                string   `json:"title"`
	Description          string   `json:"description"`
	DoneCriteria         string   `json:"doneCriteria"`
	Categories           []string `json:"categories"`
	Priority             string   `json:"priority"`
	DepartmentID         string   `json:"departmentId,omitempty"`
	AssigneeAssignmentID string   `json:"assigneeAssignmentId,omitempty"`
	GoalID               string   `json:"goalId,omitempty"`
}

// ActionPayload is the action a desk proposes and what it applies to.
type ActionPayload struct {
	Action     string `json:"action"`
	RunID      string `json:"runId,omitempty"`
	ArtifactID string `json:"artifactId,omitempty"`
	VersionID  string `json:"versionId,omitempty"`
	// Title names the target for the person (run, result or flow title).
	Title string `json:"title,omitempty"`
	// Folder is where the desk suggests code be copied out (deliver_code),
	// read from the request; the person sees it filled in and may change
	// it when approving.
	Folder string `json:"folder,omitempty"`
	Reason string `json:"reason"`
}

// RoutePayload passes a company request to a service.
type RoutePayload struct {
	ProjectID string `json:"projectId"`
	Body      string `json:"body"`
	Reason    string `json:"reason"`
}

// IntakeDecision is a desk's checked answer to a request.
type IntakeDecision struct {
	Outcome  string                `json:"outcome"`
	Answer   string                `json:"answer,omitempty"`
	Question string                `json:"question,omitempty"`
	Reason   string                `json:"reason,omitempty"`
	Routes   []RoutePayload        `json:"routes,omitempty"`
	Work     []WorkProposalPayload `json:"work,omitempty"`
	Actions  []ActionPayload       `json:"actions,omitempty"`
}
