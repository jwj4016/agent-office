package storage

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

// schemaFixture inserts two projects, each with a role, assignment,
// workflow, version, run and step attempt, using raw SQL so the
// constraints themselves are under test.
type schemaFixture struct {
	db *DB
}

func newSchemaFixture(t *testing.T) *schemaFixture {
	t.Helper()
	db := openTemp(t, filepath.Join(t.TempDir(), "a.db"))
	t.Cleanup(func() { db.Close() })
	f := &schemaFixture{db: db}
	now := Now()
	f.mustExec(t, `INSERT INTO roles (id, organization_id, name, created_at, updated_at) VALUES ('role-dev', 'org-default', '개발자', ?, ?)`, now, now)
	for _, p := range []string{"game", "estate"} {
		f.mustExec(t, `INSERT INTO projects (id, organization_id, name, created_at, updated_at) VALUES (?, 'org-default', ?, ?, ?)`, p, p, now, now)
		f.mustExec(t, `INSERT INTO assignments (id, project_id, role_id, actor_kind, display_name, created_at, updated_at) VALUES (?, ?, 'role-dev', 'ai', 'dev', ?, ?)`, "a-"+p, p, now, now)
		f.mustExec(t, `INSERT INTO workflows (id, project_id, title, draft_json, created_at, updated_at) VALUES (?, ?, 't', '{}', ?, ?)`, "wf-"+p, p, now, now)
		f.mustExec(t, `INSERT INTO workflow_versions (id, workflow_id, project_id, number, spec_json, assignments_snapshot, policy_snapshot, created_at) VALUES (?, ?, ?, 1, '{}', '{}', '{}', ?)`, "wv-"+p, "wf-"+p, p, now)
		f.mustExec(t, `INSERT INTO runs (id, project_id, workflow_version_id, status, started_at) VALUES (?, ?, ?, 'running', ?)`, "run-"+p, p, "wv-"+p, now)
		f.mustExec(t, `INSERT INTO step_attempts (id, project_id, run_id, step_id, generation, status, assignment_id, created_at) VALUES (?, ?, ?, 'plan', 1, 'running', ?, ?)`, "sa-"+p, p, "run-"+p, "a-"+p, now)
	}
	return f
}

func (f *schemaFixture) exec(q string, args ...any) error {
	return f.db.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(q, args...)
		return err
	})
}

func (f *schemaFixture) mustExec(t *testing.T, q string, args ...any) {
	t.Helper()
	if err := f.exec(q, args...); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
}

func (f *schemaFixture) mustFail(t *testing.T, want, q string, args ...any) {
	t.Helper()
	err := f.exec(q, args...)
	if err == nil {
		t.Fatalf("expected failure for: %s", q)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error %q does not mention %q", err, want)
	}
}

func TestDefaultOrganizationSeeded(t *testing.T) {
	f := newSchemaFixture(t)
	var name string
	if err := f.db.Read().QueryRow(`SELECT name FROM organizations WHERE id = 'org-default'`).Scan(&name); err != nil || name == "" {
		t.Fatalf("default org missing: %v", err)
	}
}

func TestCrossProjectReferencesRejected(t *testing.T) {
	f := newSchemaFixture(t)
	now := Now()
	const fk = "FOREIGN KEY"
	// A run in "estate" may not use the "game" workflow version.
	f.mustFail(t, fk, `INSERT INTO runs (id, project_id, workflow_version_id, status, started_at) VALUES ('r-x', 'estate', 'wv-game', 'running', ?)`, now)
	// A step attempt in "estate" may not hang off the "game" run.
	f.mustFail(t, fk, `INSERT INTO step_attempts (id, project_id, run_id, step_id, generation, status, created_at) VALUES ('sa-x', 'estate', 'run-game', 's', 1, 'pending', ?)`, now)
	// Nor use another project's assignment.
	f.mustFail(t, fk, `INSERT INTO step_attempts (id, project_id, run_id, step_id, generation, status, assignment_id, created_at) VALUES ('sa-y', 'estate', 'run-estate', 's', 1, 'pending', 'a-game', ?)`, now)
	// Artifacts, messages, approvals and events are all pinned the same way.
	f.mustFail(t, fk, `INSERT INTO artifacts (id, project_id, run_id, step_attempt_id, output_key, version, type, path, hash, size, created_at) VALUES ('ar-x', 'estate', 'run-estate', 'sa-game', 'spec', 1, 'markdown', 'p', 'h', 1, ?)`, now)
	f.mustFail(t, fk, `INSERT INTO messages (id, project_id, run_id, sender, recipient, kind, body, created_at) VALUES ('m-x', 'estate', 'run-game', 'a', 'b', 'question', '?', ?)`, now)
	f.mustFail(t, fk, `INSERT INTO approvals (id, project_id, run_id, step_attempt_id, generation, kind, target_manifest, created_at) VALUES ('ap-x', 'estate', 'run-estate', 'sa-game', 1, 'step', '{}', ?)`, now)
	f.mustFail(t, fk, `INSERT INTO execution_events (id, project_id, run_id, kind, created_at) VALUES ('ev-x', 'estate', 'run-game', 'x', ?)`, now)
	// The same rows inside the right project are fine.
	f.mustExec(t, `INSERT INTO artifacts (id, project_id, run_id, step_attempt_id, output_key, version, type, path, hash, size, created_at) VALUES ('ar-ok', 'estate', 'run-estate', 'sa-estate', 'spec', 1, 'markdown', 'p', 'h', 1, ?)`, now)
}

func TestUniquenessAndChecks(t *testing.T) {
	f := newSchemaFixture(t)
	now := Now()
	f.mustFail(t, "UNIQUE", `INSERT INTO workflow_versions (id, workflow_id, project_id, number, spec_json, assignments_snapshot, policy_snapshot, created_at) VALUES ('wv-dup', 'wf-game', 'game', 1, '{}', '{}', '{}', ?)`, now)
	f.mustFail(t, "UNIQUE", `INSERT INTO step_attempts (id, project_id, run_id, step_id, generation, status, created_at) VALUES ('sa-dup', 'game', 'run-game', 'plan', 1, 'pending', ?)`, now)
	f.mustFail(t, "CHECK", `INSERT INTO assignments (id, project_id, role_id, actor_kind, display_name, created_at, updated_at) VALUES ('a-h', 'game', 'role-dev', 'human', 'me', ?, ?)`, now, now)
	f.mustFail(t, "CHECK", `INSERT INTO step_attempts (id, project_id, run_id, step_id, generation, status, created_at) VALUES ('sa-bad', 'game', 'run-game', 'qa', 1, 'done', ?)`, now)
	f.mustFail(t, "CHECK", `INSERT INTO provider_connections (id, name, provider, secret_ref, created_at, updated_at) VALUES ('c1', 'k', 'openai', 'sk-plaintext', ?, ?)`, now, now)
	f.mustFail(t, "CHECK", `UPDATE roles SET parent_role_id = id WHERE id = 'role-dev'`)
}

func TestStepApprovalDecidedOnce(t *testing.T) {
	f := newSchemaFixture(t)
	now := Now()
	f.mustExec(t, `INSERT INTO approvals (id, project_id, run_id, step_attempt_id, generation, kind, target_manifest, created_at) VALUES ('ap1', 'game', 'run-game', 'sa-game', 1, 'step', '{}', ?)`, now)
	f.mustFail(t, "UNIQUE", `INSERT INTO approvals (id, project_id, run_id, step_attempt_id, generation, kind, target_manifest, created_at) VALUES ('ap2', 'game', 'run-game', 'sa-game', 1, 'step', '{}', ?)`, now)
	// Many tool approvals per attempt are fine when keyed by request.
	f.mustExec(t, `INSERT INTO approvals (id, project_id, run_id, step_attempt_id, generation, kind, request_key, target_manifest, created_at) VALUES ('ap3', 'game', 'run-game', 'sa-game', 1, 'tool', 'req-1', '{}', ?)`, now)
	f.mustExec(t, `INSERT INTO approvals (id, project_id, run_id, step_attempt_id, generation, kind, request_key, target_manifest, created_at) VALUES ('ap4', 'game', 'run-game', 'sa-game', 1, 'tool', 'req-2', '{}', ?)`, now)

	decide := func(decision string) int64 {
		var n int64
		f.db.Write(context.Background(), func(tx *sql.Tx) error {
			res, err := tx.Exec(`UPDATE approvals SET decision = ?, decided_at = ? WHERE id = 'ap1' AND decision IS NULL`, decision, Now())
			if err == nil {
				n, _ = res.RowsAffected()
			}
			return err
		})
		return n
	}
	if decide("approved") != 1 || decide("rejected") != 0 {
		t.Fatal("approval decision applied more than once")
	}
	f.mustFail(t, "CHECK", `UPDATE approvals SET decided_at = NULL WHERE id = 'ap1'`)
}

func TestEventSequenceIsMonotonic(t *testing.T) {
	f := newSchemaFixture(t)
	for i := 0; i < 3; i++ {
		f.mustExec(t, `INSERT INTO execution_events (id, project_id, run_id, kind, created_at) VALUES (?, 'game', 'run-game', 'x', ?)`, "ev"+string(rune('a'+i)), Now())
	}
	rows, _ := f.db.Read().Query(`SELECT sequence FROM execution_events ORDER BY sequence`)
	defer rows.Close()
	prev := int64(0)
	for rows.Next() {
		var s int64
		rows.Scan(&s)
		if s <= prev {
			t.Fatalf("sequence not increasing: %d after %d", s, prev)
		}
		prev = s
	}
}

// Departments and work items are pinned to their service like every
// other project-owned row, and the work rules hold in the schema itself.
func TestWorkSchemaConstraints(t *testing.T) {
	f := newSchemaFixture(t)
	now := Now()
	dept := `INSERT INTO departments (id, project_id, name, lead_assignment_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`
	f.mustExec(t, dept, "d-game", "game", "개발팀", "a-game", now, now)
	f.mustExec(t, dept, "d-estate", "estate", "개발팀", nil, now, now) // same name, other service
	f.mustFail(t, "UNIQUE", dept, "d-dup", "game", "개발팀", nil, now, now)
	f.mustFail(t, "FOREIGN KEY", dept, "d-x", "estate", "법무팀", "a-game", now, now)
	member := `INSERT INTO department_members (assignment_id, project_id, department_id, created_at) VALUES (?, ?, ?, ?)`
	f.mustFail(t, "FOREIGN KEY", member, "a-game", "game", "d-estate", now)
	f.mustExec(t, member, "a-game", "game", "d-game", now)
	f.mustFail(t, "UNIQUE", member, "a-game", "game", "d-game", now)

	work := `INSERT INTO work_items (id, project_id, kind, parent_id, department_id, title, status, status_reason, closed_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	f.mustExec(t, work, "goal", "game", "goal", nil, "d-game", "목표", "received", "", nil, now, now)
	// Another service may not use this service's department or goal.
	f.mustFail(t, "FOREIGN KEY", work, "w-x", "estate", "work", nil, "d-game", "x", "received", "", nil, now, now)
	f.mustFail(t, "FOREIGN KEY", work, "w-y", "estate", "work", "goal", nil, "x", "received", "", nil, now, now)
	// A goal needs a department and no parent.
	f.mustFail(t, "CHECK", work, "g2", "game", "goal", nil, nil, "x", "received", "", nil, now, now)
	f.mustFail(t, "CHECK", work, "g3", "game", "goal", "goal", "d-game", "x", "received", "", nil, now, now)
	// Waiting needs a reason; closed and closed_at go together.
	f.mustFail(t, "CHECK", work, "w1", "game", "work", "goal", nil, "x", "waiting", "", nil, now, now)
	f.mustFail(t, "CHECK", work, "w2", "game", "work", "goal", nil, "x", "done", "", nil, now, now)
	f.mustFail(t, "CHECK", work, "w3", "game", "work", "goal", nil, "x", "ready", "", now, now, now)
	f.mustExec(t, work, "w4", "game", "work", "goal", nil, "x", "done", "", now, now, now)
	f.mustFail(t, "FOREIGN KEY", `INSERT INTO work_item_notes (id, project_id, work_item_id, kind, body, author, created_at) VALUES ('n1', 'estate', 'w4', 'note', 'x', 'me', ?)`, now)
	f.mustFail(t, "CHECK", `INSERT INTO work_item_notes (id, project_id, work_item_id, kind, body, author, created_at) VALUES ('n2', 'game', 'w4', 'note', '', 'me', ?)`, now)
}

// A delivery exists once per event, department and purpose, belongs to
// the event's service, and a failure or hold always says why.
func TestDeliverySchemaConstraints(t *testing.T) {
	f := newSchemaFixture(t)
	now := Now()
	f.mustExec(t, `INSERT INTO departments (id, project_id, name, created_at, updated_at) VALUES ('d-game', 'game', '개발팀', ?, ?)`, now, now)
	f.mustExec(t, `INSERT INTO execution_events (sequence, id, project_id, kind, payload, created_at) VALUES (900, 'ev-w', 'game', 'work.created', '{}', ?)`, now)
	dlv := `INSERT INTO event_deliveries (id, project_id, event_sequence, department_id, purpose, status, error, hold_reason, created_at, updated_at)
		VALUES (?, ?, 900, 'd-game', ?, ?, ?, ?, ?, ?)`
	f.mustExec(t, dlv, "dl1", "game", "judge", "pending", "", "", now, now)
	f.mustFail(t, "UNIQUE", dlv, "dl2", "game", "judge", "pending", "", "", now, now)
	f.mustExec(t, dlv, "dl3", "game", "notify", "pending", "", "", now, now)
	f.mustFail(t, "FOREIGN KEY", dlv, "dl4", "estate", "collab_request", "pending", "", "", now, now)
	f.mustFail(t, "CHECK", dlv, "dl5", "game", "collab_request", "failed", "", "", now, now)
	f.mustFail(t, "CHECK", dlv, "dl6", "game", "collab_request", "held", "", "", now, now)
}

// A judgment belongs to its delivery's service, and a question always
// has a question status; an accepted proposal names its work.
func TestJudgmentSchemaConstraints(t *testing.T) {
	f := newSchemaFixture(t)
	now := Now()
	f.mustExec(t, `INSERT INTO departments (id, project_id, name, created_at, updated_at) VALUES ('d-game', 'game', '개발팀', ?, ?)`, now, now)
	f.mustExec(t, `INSERT INTO execution_events (sequence, id, project_id, kind, payload, created_at) VALUES (900, 'ev-w', 'game', 'work.created', '{}', ?)`, now)
	f.mustExec(t, `INSERT INTO event_deliveries (id, project_id, event_sequence, department_id, purpose, status, created_at, updated_at)
		VALUES ('dl1', 'game', 900, 'd-game', 'judge', 'processing', ?, ?)`, now, now)
	job := `INSERT INTO work_ai_jobs (id, project_id, kind, lane, request_key, delivery_id, round, created_at, updated_at)
		VALUES (?, 'game', 'judgment', 'background', ?, 'dl1', ?, ?, ?)`
	f.mustExec(t, job, "j1", "k1", 1, now, now)
	f.mustExec(t, job, "j2", "k2", 2, now, now)
	jd := `INSERT INTO judgments (id, project_id, delivery_id, round, ai_job_id, department_id, outcome, question, question_status, created_at, updated_at)
		VALUES (?, ?, 'dl1', ?, ?, 'd-game', ?, ?, ?, ?, ?)`
	f.mustFail(t, "FOREIGN KEY", jd, "x1", "estate", 1, "j1", "none", "", "", now, now)
	f.mustFail(t, "CHECK", jd, "x2", "game", 1, "j1", "question", "", "open", now, now)
	f.mustFail(t, "CHECK", jd, "x3", "game", 1, "j1", "question", "왜?", "", now, now)
	f.mustFail(t, "CHECK", jd, "x4", "game", 1, "j1", "none", "", "open", now, now)
	f.mustExec(t, jd, "jd1", "game", 1, "j1", "propose", "", "", now, now)
	f.mustFail(t, "UNIQUE", jd, "x5", "game", 1, "j2", "none", "", "", now, now)
	prop := `INSERT INTO work_proposals (id, project_id, judgment_id, position, department_id, title, status, work_item_id, decided_at, created_at)
		VALUES (?, 'game', 'jd1', ?, 'd-game', '업무', ?, ?, ?, ?)`
	f.mustExec(t, prop, "p1", 1, "proposed", nil, nil, now)
	f.mustFail(t, "UNIQUE", prop, "p2", 1, "proposed", nil, nil, now)
	f.mustFail(t, "CHECK", prop, "p3", 6, "proposed", nil, nil, now)
	f.mustFail(t, "CHECK", prop, "p4", 2, "accepted", nil, now, now)
	f.mustFail(t, "CHECK", prop, "p5", 2, "rejected", nil, nil, now)
}

// A collaboration request stays inside its service, never asks its own
// department, and keeps its decision fields consistent with its status.
func TestCollabSchemaConstraints(t *testing.T) {
	f := newSchemaFixture(t)
	now := Now()
	dept := `INSERT INTO departments (id, project_id, name, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`
	f.mustExec(t, dept, "d-mkt", "game", "마케팅팀", now, now)
	f.mustExec(t, dept, "d-dev", "game", "개발팀", now, now)
	f.mustExec(t, dept, "d-other", "estate", "개발팀", now, now)
	work := `INSERT INTO work_items (id, project_id, kind, department_id, title, created_at, updated_at) VALUES (?, 'game', 'work', 'd-mkt', ?, ?, ?)`
	f.mustExec(t, work, "w1", "기획", now, now)
	f.mustExec(t, work, "w2", "구현", now, now)
	req := `INSERT INTO collab_requests (id, project_id, from_work_item_id, from_department_id, to_department_id, question, requirement, status,
		follow_up, decided_by, decided_at, result_work_item_id, created_at, updated_at) VALUES (?, 'game', 'w1', 'd-mkt', ?, '검토', ?, ?, ?, ?, ?, ?, ?, ?)`
	f.mustExec(t, req, "c1", "d-dev", "required", "requested", "", "", nil, nil, now, now)
	f.mustFail(t, "FOREIGN KEY", req, "c2", "d-other", "reference", "requested", "", "", nil, nil, now, now)
	f.mustFail(t, "CHECK", req, "c3", "d-mkt", "reference", "requested", "", "", nil, nil, now, now)
	f.mustFail(t, "CHECK", req, "c4", "d-dev", "reference", "accepted", "", "user", now, nil, now, now)
	f.mustFail(t, "CHECK", req, "c5", "d-dev", "required", "accepted", "", "ai", now, "w2", now, now)
	f.mustFail(t, "CHECK", req, "c6", "d-dev", "reference", "question", "", "", nil, nil, now, now)
	f.mustFail(t, "CHECK", req, "c7", "d-dev", "reference", "declined", "", "user", nil, nil, now, now)
	f.mustFail(t, "CHECK", req, "c8", "d-dev", "maybe", "requested", "", "", nil, nil, now, now)
	f.mustExec(t, req, "c9", "d-dev", "reference", "accepted", "", "ai", now, "w2", now, now)
	f.mustFail(t, "UNIQUE", req, "c10", "d-dev", "reference", "accepted", "", "user", now, "w2", now, now)
	f.mustFail(t, "FOREIGN KEY", `INSERT INTO collab_request_history (id, project_id, request_id, action, author, created_at) VALUES ('h1', 'estate', 'c1', 'asked', 'me', ?)`, now)
	f.mustFail(t, "CHECK", `INSERT INTO collab_request_history (id, project_id, request_id, action, author, created_at) VALUES ('h2', 'game', 'c1', 'shrugged', 'me', ?)`, now)
	f.mustExec(t, `INSERT INTO collab_request_history (id, project_id, request_id, action, author, created_at) VALUES ('h3', 'game', 'c9', 'reverted', 'me', ?)`, now)
}

// An auto start is one per work item of its service, says why it needs a
// person, and has a run once started.
func TestAutoStartSchemaConstraints(t *testing.T) {
	f := newSchemaFixture(t)
	now := Now()
	f.mustExec(t, `INSERT INTO work_items (id, project_id, kind, title, created_at, updated_at) VALUES ('w1', 'game', 'work', '기획', ?, ?)`, now, now)
	as := `INSERT INTO auto_starts (id, project_id, work_item_id, status, run_id, reason, created_at, updated_at) VALUES (?, ?, 'w1', ?, ?, ?, ?, ?)`
	f.mustFail(t, "FOREIGN KEY", as, "s0", "estate", "pending", nil, "", now, now)
	f.mustFail(t, "CHECK", as, "s1", "game", "review", nil, "", now, now)
	f.mustFail(t, "CHECK", as, "s2", "game", "started", nil, "", now, now)
	f.mustFail(t, "CHECK", as, "s3", "game", "waiting", nil, "", now, now)
	f.mustExec(t, as, "s4", "game", "started", "run-game", "", now, now)
	f.mustFail(t, "UNIQUE", as, "s5", "game", "pending", nil, "", now, now)
	f.mustFail(t, "FOREIGN KEY", `UPDATE auto_starts SET run_id = 'run-estate' WHERE id = 's4'`)
}
