package storage

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"agent-office/internal/domain"
)

// A database left at schema v2 (with data) upgrades in place.
func TestUpgradeFromV2KeepsData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	raw, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	ms, _ := loadMigrations()
	if _, err := raw.Exec(`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, name TEXT NOT NULL, applied_at TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	for _, m := range ms[:2] {
		if _, err := raw.Exec(m.sql); err != nil {
			t.Fatalf("%s: %v", m.name, err)
		}
		raw.Exec(`INSERT INTO schema_migrations VALUES (?, ?, ?)`, m.version, m.name, Now())
	}
	raw.Exec(`INSERT INTO projects (id, organization_id, name, created_at, updated_at) VALUES ('prj-old', 'org-default', '예전 서비스', ?, ?)`, Now(), Now())
	raw.Close()

	db := openTemp(t, path)
	defer db.Close()
	if v, _ := db.SchemaVersion(context.Background()); v != ms[len(ms)-1].version {
		t.Fatalf("schema version = %d", v)
	}
	var name, policy string
	if err := db.Read().QueryRow(`SELECT name, auto_policy FROM projects WHERE id = 'prj-old'`).Scan(&name, &policy); err != nil || name != "예전 서비스" || policy != "{}" {
		t.Fatalf("old project after upgrade: %q %q %v", name, policy, err)
	}
}

// v5 rebuilds artifacts to allow the diagram type. Existing artifacts,
// including a rework copy pointing at its source, survive with their
// foreign keys intact, and the type check still rejects unknown types.
func TestUpgradeToDiagramArtifactsKeepsArtifacts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v4.db")
	raw, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	ms, _ := loadMigrations()
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := raw.Exec(q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	exec(`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, name TEXT NOT NULL, applied_at TEXT NOT NULL)`)
	for _, m := range ms {
		if m.version > 4 {
			break
		}
		exec(m.sql)
		exec(`INSERT INTO schema_migrations VALUES (?, ?, ?)`, m.version, m.name, Now())
	}
	now := Now()
	exec(`INSERT INTO projects (id, organization_id, name, created_at, updated_at) VALUES ('prj-1', 'org-default', '서비스', ?, ?)`, now, now)
	exec(`INSERT INTO workflows (id, project_id, title, draft_json, created_at, updated_at) VALUES ('wf-1', 'prj-1', '흐름', '{}', ?, ?)`, now, now)
	exec(`INSERT INTO workflow_versions (id, workflow_id, project_id, number, spec_json, assignments_snapshot, policy_snapshot, created_at) VALUES ('wv-1', 'wf-1', 'prj-1', 1, '{}', '[]', '{}', ?)`, now)
	exec(`INSERT INTO runs (id, project_id, workflow_version_id, status, started_at) VALUES ('run-1', 'prj-1', 'wv-1', 'running', ?)`, now)
	exec(`INSERT INTO step_attempts (id, project_id, run_id, step_id, generation, status, created_at) VALUES ('att-1', 'prj-1', 'run-1', 'plan', 1, 'succeeded', ?)`, now)
	art := `INSERT INTO artifacts (id, project_id, run_id, step_attempt_id, output_key, version, type, path, hash, size, validity, source_artifact_id, created_at) VALUES (?, 'prj-1', 'run-1', 'att-1', 'spec', ?, 'markdown', 'p', 'h', 1, ?, ?, ?)`
	exec(art, "art-1", 1, "stale", nil, now)
	exec(art, "art-2", 2, "valid", "art-1", now)
	raw.Close()

	db := openTemp(t, path)
	defer db.Close()
	ctx := context.Background()
	if v, _ := db.SchemaVersion(ctx); v != ms[len(ms)-1].version {
		t.Fatalf("schema version = %d", v)
	}
	var n int
	var src string
	db.Read().QueryRow(`SELECT COUNT(*) FROM artifacts`).Scan(&n)
	db.Read().QueryRow(`SELECT source_artifact_id FROM artifacts WHERE id = 'art-2'`).Scan(&src)
	if n != 2 || src != "art-1" {
		t.Fatalf("artifacts after upgrade: count=%d source=%q", n, src)
	}
	var schema string
	db.Read().QueryRow(`SELECT sql FROM sqlite_master WHERE name = 'artifacts'`).Scan(&schema)
	if !strings.Contains(schema, "REFERENCES \"artifacts\"") && !strings.Contains(schema, "REFERENCES artifacts") {
		t.Fatalf("self reference not renamed:\n%s", schema)
	}
	if props, err := db.Proposals(ctx, "prj-1", ""); err != nil || len(props) != 0 {
		t.Fatalf("proposals after upgrade: %v %v", props, err)
	}
	if rows, _ := db.Read().Query(`PRAGMA foreign_key_check`); rows.Next() {
		rows.Close()
		t.Fatal("foreign key violations after upgrade")
	}
	err = db.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(strings.Replace(art, "'markdown'", "'diagram'", 1), "art-3", 3, "valid", "art-2", now)
		return err
	})
	if err != nil {
		t.Fatalf("diagram artifact rejected: %v", err)
	}
	err = db.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(strings.Replace(art, "'markdown'", "'video'", 1), "art-4", 4, "valid", nil, now)
		return err
	})
	if err == nil {
		t.Fatal("unknown artifact type accepted")
	}
	var idx int
	db.Read().QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = 'artifacts_run'`).Scan(&idx)
	if idx != 1 {
		t.Fatal("artifacts_run index missing")
	}
}

// v8 adds department templates, departments, categories and work. A v7
// database keeps its services, roles and assignments; each existing
// service gets the default categories and can take departments and work
// right away.
func TestUpgradeToDepartmentsKeepsServices(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v7.db")
	// v9 (deliveries) applies on top in the same upgrade.
	raw, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	ms, _ := loadMigrations()
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := raw.Exec(q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	exec(`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, name TEXT NOT NULL, applied_at TEXT NOT NULL)`)
	for _, m := range ms {
		if m.version > 7 {
			break
		}
		exec(m.sql)
		exec(`INSERT INTO schema_migrations VALUES (?, ?, ?)`, m.version, m.name, Now())
	}
	now := Now()
	exec(`INSERT INTO projects (id, organization_id, name, created_at, updated_at) VALUES ('prj-1', 'org-default', '서비스', ?, ?)`, now, now)
	exec(`INSERT INTO roles (id, organization_id, name, created_at, updated_at) VALUES ('role-1', 'org-default', '개발자', ?, ?)`, now, now)
	exec(`INSERT INTO assignments (id, project_id, role_id, actor_kind, display_name, created_at, updated_at) VALUES ('asg-1', 'prj-1', 'role-1', 'ai', '개발 AI', ?, ?)`, now, now)
	raw.Close()

	db := openTemp(t, path)
	defer db.Close()
	ctx := context.Background()
	if v, _ := db.SchemaVersion(ctx); v != ms[len(ms)-1].version {
		t.Fatalf("schema version = %d", v)
	}
	cats, err := db.ProjectCategories(ctx, "prj-1")
	if err != nil || strings.Join(cats, ",") != strings.Join(domain.DefaultCategories, ",") {
		t.Fatalf("categories after upgrade: %v %v", cats, err)
	}
	tpl, err := db.SaveDepartmentTemplate(ctx, domain.DepartmentTemplate{Name: "개발팀", RoleIDs: []string{"role-1"}})
	if err != nil {
		t.Fatal(err)
	}
	dept, err := db.ImportDepartmentTemplate(ctx, "prj-1", tpl.ID)
	if err != nil || dept.LeadAssignmentID != "asg-1" {
		t.Fatalf("department from template: %+v %v", dept, err)
	}
	if _, err := db.CreateWorkItem(ctx, domain.WorkItem{ProjectID: "prj-1", Kind: domain.WorkGoal, DepartmentID: dept.ID, Title: "목표"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.RequestDepartmentCheck(ctx, "prj-1", dept.ID, ""); err != nil {
		t.Fatalf("delivery after upgrade: %v", err)
	}
	if _, _, err := db.AddDocumentVersion(ctx, "prj-1", NewDocumentVersion{Title: "자료", OwnerDepartmentID: dept.ID, SourceKind: "workspace",
		SourcePath: "a.md", FileName: "a.md", Path: "projects/prj-1/documents/a.md", Hash: "h", Size: 1}); err != nil {
		t.Fatalf("document after upgrade: %v", err)
	}
	if props, err := db.Proposals(ctx, "prj-1", ""); err != nil || len(props) != 0 {
		t.Fatalf("proposals after upgrade: %v %v", props, err)
	}
	fin, err := db.SaveDepartment(ctx, domain.Department{ProjectID: "prj-1", Name: "재무팀"})
	if err != nil {
		t.Fatal(err)
	}
	work, err := db.CreateWorkItem(ctx, domain.WorkItem{ProjectID: "prj-1", DepartmentID: dept.ID, Title: "기획"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.RequestCollab(ctx, "prj-1", NewCollabRequest{FromWorkItemID: work.ID, ToDepartmentID: fin.ID, Question: "예산 확인",
		Requirement: CollabRequired}); err != nil {
		t.Fatalf("collaboration request after upgrade: %v", err)
	}
	if _, err := db.SetWorkStatus(ctx, "prj-1", work.ID, domain.WorkDone, ""); err == nil {
		t.Fatal("required collaboration did not block after upgrade")
	}
	if rows, _ := db.Read().Query(`PRAGMA foreign_key_check`); rows.Next() {
		rows.Close()
		t.Fatal("foreign key violations after upgrade")
	}
}
