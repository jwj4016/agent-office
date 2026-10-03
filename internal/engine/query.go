package engine

import (
	"context"
	"database/sql"
	"encoding/json"
)

// ArtifactView is one stored step result.
type ArtifactView struct {
	ID        string `json:"id"`
	OutputKey string `json:"outputKey"`
	Version   int    `json:"version"`
	Type      string `json:"type"`
	Path      string `json:"path"`
	Hash      string `json:"hash"`
	Validity  string `json:"validity"`
}

// StepView is a step's current state within a run.
type StepView struct {
	ID           string          `json:"id"`
	Title        string          `json:"title"`
	Kind         string          `json:"kind"`
	Status       string          `json:"status"`
	AssignmentID string          `json:"assignmentId"`
	AttemptID    string          `json:"attemptId"`
	Generation   int             `json:"generation"`
	Attempt      int             `json:"attempt"`
	Round        int             `json:"round"`
	Error        string          `json:"error,omitempty"`
	ApprovalID   string          `json:"approvalId,omitempty"`
	Inputs       json.RawMessage `json:"inputs,omitempty"`
	Artifacts    []ArtifactView  `json:"artifacts"`
}

type RunDetail struct {
	ID            string     `json:"id"`
	ProjectID     string     `json:"projectId"`
	VersionID     string     `json:"versionId"`
	VersionNumber int        `json:"versionNumber"`
	Title         string     `json:"title"`
	Status        string     `json:"status"`
	Paused        bool       `json:"paused"`
	Steps         []StepView `json:"steps"`
}

// RunDetail returns a run's steps at their current generations.
func (e *Engine) RunDetail(ctx context.Context, projectID, runID string) (RunDetail, error) {
	if err := e.runInProject(ctx, projectID, runID); err != nil {
		return RunDetail{}, err
	}
	q := e.db.Read()
	st, err := loadState(ctx, q, runID)
	if err != nil {
		return RunDetail{}, err
	}
	d := RunDetail{ID: runID, ProjectID: projectID, VersionID: st.run.VersionID, VersionNumber: st.version.Number,
		Title: st.version.Spec.Title, Status: st.run.Status, Paused: st.run.Paused}
	for _, id := range st.graph.Order() {
		n := st.graph.Node(id)
		v := StepView{ID: id, Title: n.Title, Kind: string(n.Kind), Status: st.status(id), AssignmentID: n.AssignmentID,
			Generation: st.run.Gens[id].G, Round: st.run.Gens[id].R, Artifacts: []ArtifactView{}}
		if a, ok := st.attempts[id]; ok {
			v.AttemptID, v.Attempt, v.Inputs = a.ID, a.Attempt, a.InputManifest
			q.QueryRowContext(ctx, `SELECT error FROM step_attempts WHERE id = ?`, a.ID).Scan(&v.Error)
			var apr sql.NullString
			q.QueryRowContext(ctx, `SELECT id FROM approvals WHERE step_attempt_id = ? AND kind = 'step'`, a.ID).Scan(&apr)
			v.ApprovalID = apr.String
			rows, err := q.QueryContext(ctx, `SELECT id, output_key, version, type, path, hash, validity FROM artifacts WHERE step_attempt_id = ? ORDER BY output_key, version`, a.ID)
			if err != nil {
				return d, err
			}
			for rows.Next() {
				var av ArtifactView
				rows.Scan(&av.ID, &av.OutputKey, &av.Version, &av.Type, &av.Path, &av.Hash, &av.Validity)
				v.Artifacts = append(v.Artifacts, av)
			}
			rows.Close()
		}
		d.Steps = append(d.Steps, v)
	}
	return d, nil
}

// Step returns one step of a RunDetail.
func (d RunDetail) Step(id string) StepView {
	for _, s := range d.Steps {
		if s.ID == id {
			return s
		}
	}
	return StepView{}
}

// Runs lists a project's runs, newest first.
func (e *Engine) Runs(ctx context.Context, projectID string) ([]RunDetail, error) {
	if err := e.db.CheckScope(ctx, projectID); err != nil {
		return nil, err
	}
	rows, err := e.db.Read().QueryContext(ctx, `SELECT id FROM runs WHERE project_id = ? ORDER BY started_at DESC`, projectID)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		rows.Scan(&id)
		ids = append(ids, id)
	}
	rows.Close()
	out := []RunDetail{}
	for _, id := range ids {
		d, err := e.RunDetail(ctx, projectID, id)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}
