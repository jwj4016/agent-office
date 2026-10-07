package storage

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
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
