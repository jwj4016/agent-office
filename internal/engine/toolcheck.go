package engine

import (
	"context"
	"encoding/json"
	"strings"

	"agent-office/internal/providers"
	"agent-office/internal/storage"
	"agent-office/internal/toolpolicy"
)

// toolVerdict checks a tool request of a live attempt against the app's
// rules and the service's (company requests, 6th revision). The full tool
// input is used when the provider sends it; otherwise the command is read
// out of the request's text, and a request cut too short to read asks.
func toolVerdict(ctx context.Context, q querier, a *activeAttempt, p providers.RequestPayload) (toolpolicy.Verdict, error) {
	rules, err := storage.ToolPolicyRules(ctx, q, a.projectID)
	if err != nil {
		return toolpolicy.Verdict{}, err
	}
	a.mu.Lock()
	req := a.folders
	a.mu.Unlock()
	req.Tool = p.Action
	var in struct {
		Command  string `json:"command"`
		FilePath string `json:"file_path"`
		Path     string `json:"path"`
		Notebook string `json:"notebook_path"`
	}
	if len(p.Input) > 0 {
		json.Unmarshal(p.Input, &in)
	} else if i := strings.IndexByte(p.Detail, '{'); i >= 0 {
		// "Bash {…}" from an older bridge: complete only when not cut.
		json.Unmarshal([]byte(p.Detail[i:]), &in)
	}
	switch p.Action {
	case "command":
		req.Command = p.Detail // Codex sends the command itself
	case "Bash":
		req.Command = in.Command
		if req.Command == "" {
			return toolpolicy.Verdict{Summary: "명령 실행", Risks: []toolpolicy.Risk{{Reason: "요청 내용이 잘려 어떤 명령인지 확인할 수 없습니다"}}}, nil
		}
	default:
		req.Path = in.FilePath
		if req.Path == "" {
			req.Path = in.Path
		}
		if req.Path == "" {
			req.Path = in.Notebook
		}
	}
	return toolpolicy.Check(req, rules), nil
}

// storedVerdict reads the check kept with a tool approval.
func storedVerdict(ctx context.Context, q querier, approvalID string) (toolpolicy.Verdict, bool) {
	var target string
	if err := q.QueryRowContext(ctx, `SELECT target_manifest FROM approvals WHERE id = ? AND kind = 'tool'`, approvalID).Scan(&target); err != nil {
		return toolpolicy.Verdict{}, false
	}
	var t struct {
		Verdict *toolpolicy.Verdict `json:"verdict"`
	}
	json.Unmarshal([]byte(target), &t)
	if t.Verdict == nil {
		return toolpolicy.Verdict{}, false
	}
	return *t.Verdict, true
}

// RememberToolRules keeps, for the approval's service, the rules that
// waive what its request was asked about; risks that may not be waived
// have no rule and are refused. It returns the rules kept.
func (e *Engine) RememberToolRules(ctx context.Context, projectID, approvalID string) ([]string, error) {
	if err := e.db.CheckScope(ctx, projectID, storage.Ref{Kind: storage.RefApproval, ID: approvalID}); err != nil {
		return nil, err
	}
	v, ok := storedVerdict(ctx, e.db.Read(), approvalID)
	if !ok {
		return nil, ErrInvalid
	}
	rules, ok := v.Rules()
	if !ok {
		return nil, ErrInvalid
	}
	if err := e.db.AddToolRules(ctx, projectID, rules); err != nil {
		return nil, err
	}
	var keys []string
	for _, r := range rules {
		keys = append(keys, r.Key())
	}
	return keys, nil
}

// ApproveAllowedToolRequests checks a service's waiting tool requests
// again (after its rules changed) and approves those now allowed. It
// returns how many it approved.
func (e *Engine) ApproveAllowedToolRequests(ctx context.Context, projectID string) (int, error) {
	rows, err := e.db.Read().QueryContext(ctx, `SELECT id, step_attempt_id, target_manifest FROM approvals
		WHERE project_id = ? AND kind = 'tool' AND decision IS NULL`, projectID)
	if err != nil {
		return 0, err
	}
	type pending struct{ id, attempt, target string }
	var list []pending
	for rows.Next() {
		var p pending
		rows.Scan(&p.id, &p.attempt, &p.target)
		list = append(list, p)
	}
	rows.Close()
	n := 0
	for _, p := range list {
		live := e.lookupActive(p.attempt)
		if live == nil {
			continue
		}
		var t struct {
			Action, Detail string
			Input          json.RawMessage
		}
		json.Unmarshal([]byte(p.target), &t)
		v, err := toolVerdict(ctx, e.db.Read(), live, providers.RequestPayload{Action: t.Action, Detail: t.Detail, Input: t.Input})
		if err != nil || !v.Allow {
			continue
		}
		if _, err := e.DecideToolApproval(ctx, projectID, p.id, true); err == nil {
			n++
		}
	}
	return n, nil
}
