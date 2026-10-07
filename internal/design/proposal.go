// Package design turns a natural-language goal into a proposed
// organization and workflow (spec §3.1). The model only proposes: its
// JSON is parsed strictly, checked against the real organization and
// connections, and never creates tools, permissions or keys.
package design

import (
	"bytes"
	"encoding/json"
	"fmt"

	"agent-office/internal/domain"
)

// Proposal is the designer AI's answer. Assignment ids in Workflow are
// the proposal's own assignment refs until applied.
type Proposal struct {
	Project     ProposedProject      `json:"project"`
	Roles       []ProposedRole       `json:"roles"`
	Assignments []ProposedAssignment `json:"assignments"`
	Workflow    domain.WorkflowSpec  `json:"workflow"`
	// HumanSteps maps each task the person said they will do to the step
	// that carries it, so the app can check it really went to a person.
	HumanSteps []HumanStep `json:"humanSteps"`
	// Missing lists what is needed but not available (connections,
	// materials, unclear scope).
	Missing []MissingItem `json:"missing"`
	// RoleChanges suggests edits to existing company roles. They are shown
	// separately and never applied automatically.
	RoleChanges []RoleChange `json:"roleChanges"`
	Notes       string       `json:"notes"`
}

type ProposedProject struct {
	Name         string `json:"name"`
	Goal         string `json:"goal"`
	Instructions string `json:"instructions"`
}

// ProposedRole either reuses an existing company role (ReuseRoleID) or
// describes a new one.
type ProposedRole struct {
	Ref          string `json:"ref"`
	ReuseRoleID  string `json:"reuseRoleId"`
	Name         string `json:"name"`
	Mission      string `json:"mission"`
	Instructions string `json:"instructions"`
	// ParentRef names another proposed role ref or an existing role id.
	ParentRef string `json:"parentRef"`
}

type ProposedAssignment struct {
	Ref          string `json:"ref"`
	RoleRef      string `json:"roleRef"`
	ActorKind    string `json:"actorKind"` // ai | human
	DisplayName  string `json:"displayName"`
	ConnectionID string `json:"connectionId"`
	Model        string `json:"model"`
	Instructions string `json:"instructions"`
}

type HumanStep struct {
	Request string `json:"request"`
	StepID  string `json:"stepId"`
}

type MissingItem struct {
	Kind   string `json:"kind"` // connection | material | scope
	Detail string `json:"detail"`
}

type RoleChange struct {
	RoleID       string `json:"roleId"`
	Instructions string `json:"instructions"`
	Reason       string `json:"reason"`
}

// Parse decodes a proposal strictly: unknown fields (e.g. a "tools" or
// "apiKey" the model invented) are an error, not ignored.
func Parse(data []byte) (*Proposal, error) {
	dec := json.NewDecoder(bytes.NewReader(bytes.TrimSpace(data)))
	dec.DisallowUnknownFields()
	var p Proposal
	if err := dec.Decode(&p); err != nil {
		return nil, fmt.Errorf("설계안이 약속한 JSON 형식이 아닙니다: %w", err)
	}
	if dec.More() {
		return nil, fmt.Errorf("설계안 뒤에 다른 내용이 있습니다")
	}
	return &p, nil
}

// Schema is the JSON schema given to the designer model.
func Schema() map[string]any {
	str := map[string]any{"type": "string"}
	obj := func(props map[string]any, required ...string) map[string]any {
		return map[string]any{"type": "object", "properties": props, "required": required, "additionalProperties": false}
	}
	arr := func(item map[string]any) map[string]any { return map[string]any{"type": "array", "items": item} }
	return obj(map[string]any{
		"project": obj(map[string]any{"name": str, "goal": str, "instructions": str}, "name", "goal", "instructions"),
		"roles": arr(obj(map[string]any{"ref": str, "reuseRoleId": str, "name": str, "mission": str, "instructions": str, "parentRef": str},
			"ref", "reuseRoleId", "name", "mission", "instructions", "parentRef")),
		"assignments": arr(obj(map[string]any{"ref": str, "roleRef": str, "actorKind": map[string]any{"type": "string", "enum": []string{"ai", "human"}},
			"displayName": str, "connectionId": str, "model": str, "instructions": str},
			"ref", "roleRef", "actorKind", "displayName", "connectionId", "model", "instructions")),
		"workflow":    map[string]any{"type": "object", "description": "Agent Office 업무 흐름 JSON (schemaVersion 1). assignmentId에는 assignments[].ref를 넣는다"},
		"humanSteps":  arr(obj(map[string]any{"request": str, "stepId": str}, "request", "stepId")),
		"missing":     arr(obj(map[string]any{"kind": map[string]any{"type": "string", "enum": []string{"connection", "material", "scope"}}, "detail": str}, "kind", "detail")),
		"roleChanges": arr(obj(map[string]any{"roleId": str, "instructions": str, "reason": str}, "roleId", "instructions", "reason")),
		"notes":       str,
	}, "project", "roles", "assignments", "workflow", "humanSteps", "missing", "roleChanges", "notes")
}
