package domain

import (
	"encoding/json"
	"strings"
)

// LocalOwner is the only human actor in the first release.
const LocalOwner = "local-owner"

// DefaultOrganizationID is seeded by migration 0002.
const DefaultOrganizationID = "org-default"

type Organization struct {
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	Instructions string          `json:"instructions"`
	Policy       json.RawMessage `json:"policy"`
}

type Role struct {
	ID             string          `json:"id"`
	OrganizationID string          `json:"organizationId"`
	ParentRoleID   string          `json:"parentRoleId"`
	Name           string          `json:"name"`
	Mission        string          `json:"mission"`
	Instructions   string          `json:"instructions"`
	OutputDefaults json.RawMessage `json:"outputDefaults"`
	Policy         json.RawMessage `json:"policy"`
	Appearance     json.RawMessage `json:"appearance"`
	UpdatedAt      string          `json:"updatedAt"`
}

type ProjectMode string

const (
	ModeReview ProjectMode = "review"
	ModeAuto   ProjectMode = "auto"
)

type Project struct {
	ID             string          `json:"id"`
	OrganizationID string          `json:"organizationId"`
	Name           string          `json:"name"`
	Goal           string          `json:"goal"`
	Instructions   string          `json:"instructions"`
	WorkspacePath  string          `json:"workspacePath"`
	Budget         json.RawMessage `json:"budget"`
	Mode           ProjectMode     `json:"mode"`
	Status         string          `json:"status"` // active | archived
	// AutoPolicy is the scope auto mode may use without asking.
	AutoPolicy json.RawMessage `json:"autoPolicy"`
	CreatedAt  string          `json:"createdAt"`
	UpdatedAt  string          `json:"updatedAt"`
}

type ActorKind string

const (
	ActorAI    ActorKind = "ai"
	ActorHuman ActorKind = "human"
)

// AssignmentOverrides are per-project changes to a shared role.
type AssignmentOverrides struct {
	Instructions string          `json:"instructions,omitempty"`
	Appearance   json.RawMessage `json:"appearance,omitempty"`
}

type Assignment struct {
	ID           string              `json:"id"`
	ProjectID    string              `json:"projectId"`
	RoleID       string              `json:"roleId"`
	ActorKind    ActorKind           `json:"actorKind"`
	ActorID      string              `json:"actorId"`
	DisplayName  string              `json:"displayName"`
	ConnectionID string              `json:"connectionId"`
	Model        string              `json:"model"`
	Overrides    AssignmentOverrides `json:"overrides"`
}

type ProviderConnection struct {
	ID                   string          `json:"id"`
	Name                 string          `json:"name"`
	Provider             string          `json:"provider"` // test | codex | claude | openai_api | claude_api
	ExecutablePath       string          `json:"executablePath"`
	SecretRef            string          `json:"secretRef"`
	Config               json.RawMessage `json:"config"`
	VerifiedCapabilities json.RawMessage `json:"verifiedCapabilities"`
}

// Verified reports whether the connection has passed a real check. Only
// verified connections may start AI work.
func (c ProviderConnection) Verified() bool {
	s := strings.TrimSpace(string(c.VerifiedCapabilities))
	return s != "" && s != "{}" && s != "null"
}

type Workflow struct {
	ID        string          `json:"id"`
	ProjectID string          `json:"projectId"`
	Title     string          `json:"title"`
	Draft     json.RawMessage `json:"draft"`
	Revision  int             `json:"revision"`
	UpdatedAt string          `json:"updatedAt"`
}

// InstructionLayer is one level of the merged instructions, in display
// order: company → parent roles → role → project → assignment → task →
// rework feedback (spec §2.3).
type InstructionLayer struct {
	Source string `json:"source"` // organization | role | project | assignment | task | feedback
	Name   string `json:"name"`
	Text   string `json:"text"`
}

// ComposeInstructions joins non-empty layers into the final prompt text.
func ComposeInstructions(layers []InstructionLayer) string {
	var b strings.Builder
	for _, l := range layers {
		if strings.TrimSpace(l.Text) == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString("## ")
		b.WriteString(l.Name)
		b.WriteString("\n")
		b.WriteString(strings.TrimSpace(l.Text))
	}
	return b.String()
}

// AssignmentSnapshot is an assignment frozen into a workflow version,
// including the instruction layers in effect at that moment.
type AssignmentSnapshot struct {
	Assignment
	RoleName     string             `json:"roleName"`
	Instructions []InstructionLayer `json:"instructions"`
	Connected    bool               `json:"connected"`
}

// PolicySnapshot freezes project-level settings for a version.
type PolicySnapshot struct {
	ProjectMode   ProjectMode     `json:"projectMode"`
	Budget        json.RawMessage `json:"budget"`
	WorkspacePath string          `json:"workspacePath"`
	Goal          string          `json:"goal"`
}

type WorkflowVersion struct {
	ID          string                        `json:"id"`
	WorkflowID  string                        `json:"workflowId"`
	ProjectID   string                        `json:"projectId"`
	Number      int                           `json:"number"`
	Spec        WorkflowSpec                  `json:"spec"`
	Assignments map[string]AssignmentSnapshot `json:"assignments"`
	Policy      PolicySnapshot                `json:"policy"`
	CreatedAt   string                        `json:"createdAt"`
}
