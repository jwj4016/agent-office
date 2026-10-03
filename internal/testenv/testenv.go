// Package testenv builds realistic projects for tests. It is imported
// only from _test.go files.
package testenv

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"agent-office/internal/domain"
	"agent-office/internal/storage"
)

// RepoRoot returns the repository root from this source file's location.
func RepoRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..")
}

// Fixture reads tests/fixtures/<rel>.
func Fixture(t testing.TB, rel string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(RepoRoot(), "tests", "fixtures", rel))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// OpenDB opens a fresh database in a temp dir.
func OpenDB(t testing.TB) *storage.DB {
	t.Helper()
	db, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// Project is a seeded project whose workflow draft uses real ids.
type Project struct {
	ID          string
	WorkflowID  string
	Assignments map[string]string // fixture assignment id -> real id
	Connection  domain.ProviderConnection
}

// ServiceDev creates a project running the spec §6.2 workflow
// (tests/fixtures/workflows/service-dev.json) with the test provider for
// every AI assignment and local-owner for a-owner.
func ServiceDev(t testing.TB, db *storage.DB, name string) Project {
	t.Helper()
	return Seed(t, db, name, "workflows/service-dev.json", map[string]string{
		"a-planner": "기획", "a-architect": "설계", "a-backend": "백엔드", "a-frontend": "프론트엔드", "a-qa": "QA",
	}, []string{"a-owner"})
}

// Seed creates a project from a workflow fixture. aiRoles maps fixture
// assignment ids to role names; humans lists fixture ids for local-owner.
func Seed(t testing.TB, db *storage.DB, name, workflowFixture string, aiRoles map[string]string, humans []string) Project {
	t.Helper()
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	p, err := db.CreateProject(ctx, domain.Project{Name: name, Goal: name + " 목표"})
	must(err)
	conn, err := db.SaveConnection(ctx, domain.ProviderConnection{Name: "test-" + p.ID, Provider: "test"})
	must(err)
	out := Project{ID: p.ID, Assignments: map[string]string{}, Connection: conn}

	roleID := func(roleName string) string {
		roles, err := db.Roles(ctx)
		must(err)
		for _, r := range roles {
			if r.Name == roleName {
				return r.ID
			}
		}
		r, err := db.SaveRole(ctx, domain.Role{Name: roleName, Instructions: roleName + " 지침"})
		must(err)
		return r.ID
	}
	for fixtureID, roleName := range aiRoles {
		a, err := db.SaveAssignment(ctx, domain.Assignment{
			ProjectID: p.ID, RoleID: roleID(roleName), ActorKind: domain.ActorAI,
			DisplayName: roleName + " AI", ConnectionID: conn.ID, Model: "plan-success",
		})
		must(err)
		out.Assignments[fixtureID] = a.ID
	}
	for _, fixtureID := range humans {
		a, err := db.SaveAssignment(ctx, domain.Assignment{
			ProjectID: p.ID, RoleID: roleID("검토자"), ActorKind: domain.ActorHuman, DisplayName: "나",
		})
		must(err)
		out.Assignments[fixtureID] = a.ID
	}
	draft := string(Fixture(t, workflowFixture))
	for fixtureID, real := range out.Assignments {
		draft = strings.ReplaceAll(draft, `"`+fixtureID+`"`, `"`+real+`"`)
	}
	w, err := db.CreateWorkflow(ctx, p.ID, json.RawMessage(draft))
	must(err)
	out.WorkflowID = w.ID
	return out
}
