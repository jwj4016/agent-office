package storage

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"sync"
	"testing"
)

func openTemp(t *testing.T, path string) *DB {
	t.Helper()
	db, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return db
}

func TestMigrationsApplyOnceAndPersistAcrossReopen(t *testing.T) {
	ctx := context.Background()
	// Korean and space in the path on purpose (T21).
	path := filepath.Join(t.TempDir(), "데이터 폴더", "agent-office.db")

	db := openTemp(t, path)
	if err := db.SetSetting(ctx, "ui.reducedMotion", "true"); err != nil {
		t.Fatal(err)
	}
	v1, _ := db.SchemaVersion(ctx)
	db.Close()

	db = openTemp(t, path)
	defer db.Close()
	v2, _ := db.SchemaVersion(ctx)
	if v1 == 0 || v1 != v2 {
		t.Fatalf("schema version changed on reopen: %d -> %d", v1, v2)
	}
	got, err := db.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got["ui.reducedMotion"] != "true" {
		t.Fatalf("setting not restored: %v", got)
	}
}

func TestPragmasAreActive(t *testing.T) {
	db := openTemp(t, filepath.Join(t.TempDir(), "a.db"))
	defer db.Close()
	var fk int
	var mode string
	db.Read().QueryRow(`PRAGMA foreign_keys`).Scan(&fk)
	db.Read().QueryRow(`PRAGMA journal_mode`).Scan(&mode)
	if fk != 1 || mode != "wal" {
		t.Fatalf("foreign_keys=%d journal_mode=%s", fk, mode)
	}
}

func TestUnknownSettingRejected(t *testing.T) {
	db := openTemp(t, filepath.Join(t.TempDir(), "a.db"))
	defer db.Close()
	err := db.SetSetting(context.Background(), "provider.apiKey", "x")
	if !errors.Is(err, ErrUnknownSetting) {
		t.Fatalf("want ErrUnknownSetting, got %v", err)
	}
}

func TestWriteRollsBackOnError(t *testing.T) {
	ctx := context.Background()
	db := openTemp(t, filepath.Join(t.TempDir(), "a.db"))
	defer db.Close()
	boom := errors.New("boom")
	err := db.Write(ctx, func(tx *sql.Tx) error {
		tx.Exec(`INSERT INTO app_settings (key, value, updated_at) VALUES ('ui.lastProjectId', 'p1', 'now')`)
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatal(err)
	}
	got, _ := db.Settings(ctx)
	if _, ok := got["ui.lastProjectId"]; ok {
		t.Fatal("write was not rolled back")
	}
}

func TestConcurrentWritesAreSerialized(t *testing.T) {
	ctx := context.Background()
	db := openTemp(t, filepath.Join(t.TempDir(), "a.db"))
	defer db.Close()
	db.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`CREATE TABLE counter (n INTEGER NOT NULL); INSERT INTO counter VALUES (0)`)
		return err
	})
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := db.Write(ctx, func(tx *sql.Tx) error {
				var n int
				if err := tx.QueryRow(`SELECT n FROM counter`).Scan(&n); err != nil {
					return err
				}
				_, err := tx.Exec(`UPDATE counter SET n = ?`, n+1)
				return err
			}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	var n int
	db.Read().QueryRow(`SELECT n FROM counter`).Scan(&n)
	if n != 50 {
		t.Fatalf("lost updates: n=%d", n)
	}
}

// Review finding 4: a second process must not open the same data dir.
func TestSecondOpenIsLocked(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.db")
	first := openTemp(t, path)
	if _, err := Open(context.Background(), path); !errors.Is(err, ErrLocked) {
		t.Fatalf("second open = %v, want ErrLocked", err)
	}
	first.Close()
	again := openTemp(t, path)
	again.Close()
}

func TestClaimEngineOnce(t *testing.T) {
	db := openTemp(t, filepath.Join(t.TempDir(), "a.db"))
	defer db.Close()
	release, err := db.ClaimEngine()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ClaimEngine(); !errors.Is(err, ErrEngineClaimed) {
		t.Fatalf("second claim = %v", err)
	}
	release()
	if r, err := db.ClaimEngine(); err != nil {
		t.Fatal(err)
	} else {
		r()
	}
}
