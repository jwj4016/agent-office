package domain

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"time"
)

// WorkflowSchemaVersion is the only accepted workflow schemaVersion.
const WorkflowSchemaVersion = 1

type NodeKind string

const (
	KindTask      NodeKind = "task"
	KindReview    NodeKind = "review"
	KindApproval  NodeKind = "approval"
	KindCondition NodeKind = "condition"
	KindJoin      NodeKind = "join"
)

type OutputType string

const (
	OutMarkdown   OutputType = "markdown"
	OutJSON       OutputType = "json"
	OutFile       OutputType = "file"
	OutCodeChange OutputType = "code_change"
	OutReport     OutputType = "report"
)

var outputTypes = map[OutputType]bool{OutMarkdown: true, OutJSON: true, OutFile: true, OutCodeChange: true, OutReport: true}

// WorkflowSpec is the workflow graph (spec §6). Normal progress is a DAG;
// rework is a separate revision round, never a back edge.
type WorkflowSpec struct {
	SchemaVersion int    `json:"schemaVersion"`
	Title         string `json:"title"`
	Nodes         []Node `json:"nodes"`
}

type Node struct {
	ID            string      `json:"id"`
	Title         string      `json:"title"`
	Kind          NodeKind    `json:"kind"`
	AssignmentID  string      `json:"assignmentId,omitempty"`
	DependsOn     []string    `json:"dependsOn"`
	Inputs        []Input     `json:"inputs,omitempty"`
	Instructions  string      `json:"instructions,omitempty"`
	Outputs       []Output    `json:"outputs,omitempty"`
	Completion    *Completion `json:"completion,omitempty"`
	ReworkTargets []string    `json:"reworkTargets,omitempty"`
	Limits        *NodeLimits `json:"limits,omitempty"`
	Routing       *Routing    `json:"routing,omitempty"`
}

type Input struct {
	Name      string `json:"name"`
	FromStep  string `json:"fromStep"`
	OutputKey string `json:"outputKey"`
	// Required defaults to true.
	Required *bool `json:"required,omitempty"`
}

func (i Input) IsRequired() bool { return i.Required == nil || *i.Required }

type Output struct {
	Key  string     `json:"key"`
	Type OutputType `json:"type"`
	// Required defaults to true.
	Required *bool           `json:"required,omitempty"`
	Schema   json.RawMessage `json:"schema,omitempty"`
}

func (o Output) IsRequired() bool { return o.Required == nil || *o.Required }

// Completion lists checks beyond "required outputs exist and match their
// schema", which always applies.
type Completion struct {
	Commands []VerifyCommand `json:"commands,omitempty"`
}

// VerifyCommand runs an executable with separate args in the step's
// workspace. It is never passed through a shell.
type VerifyCommand struct {
	Executable string   `json:"executable"`
	Args       []string `json:"args,omitempty"`
	Timeout    string   `json:"timeout,omitempty"`
}

type NodeLimits struct {
	Timeout      string `json:"timeout,omitempty"`
	MaxRevisions *int   `json:"maxRevisions,omitempty"`
	MaxRetries   *int   `json:"maxRetries,omitempty"`
	MaxTokens    *int   `json:"maxTokens,omitempty"`
}

// Routing configures a condition node. The first matching branch wins;
// otherwise DefaultTarget is taken. Branches meet again at JoinStep.
type Routing struct {
	Source        RouteSource `json:"source"`
	Branches      []Branch    `json:"branches"`
	DefaultTarget string      `json:"defaultTarget"`
	JoinStep      string      `json:"joinStep"`
}

type RouteSource struct {
	FromStep  string `json:"fromStep"`
	OutputKey string `json:"outputKey"`
	FieldPath string `json:"fieldPath"`
}

type Operator string

const (
	OpEq     Operator = "eq"
	OpNe     Operator = "ne"
	OpIn     Operator = "in"
	OpExists Operator = "exists"
)

type Branch struct {
	Operator   Operator        `json:"operator"`
	Value      json.RawMessage `json:"value,omitempty"`
	TargetStep string          `json:"targetStep"`
}

// ParseWorkflow decodes a workflow strictly: unknown fields are errors,
// because drafts may come from a model and typos must not be ignored.
func ParseWorkflow(data []byte) (*WorkflowSpec, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var w WorkflowSpec
	if err := dec.Decode(&w); err != nil {
		return nil, fmt.Errorf("workflow json: %w", err)
	}
	if dec.More() {
		return nil, fmt.Errorf("workflow json: trailing data")
	}
	return &w, nil
}

// Node returns the node with id, or nil.
func (w *WorkflowSpec) Node(id string) *Node {
	for i := range w.Nodes {
		if w.Nodes[i].ID == id {
			return &w.Nodes[i]
		}
	}
	return nil
}

var (
	idPattern        = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)
	fieldPathPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+(\.[A-Za-z0-9_-]+)*$`)
)

func parseDuration(s string) error {
	if s == "" {
		return nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	if d <= 0 {
		return fmt.Errorf("must be positive")
	}
	return nil
}
