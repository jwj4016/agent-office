package storage

import (
	"context"
	"crypto/rand"
	"database/sql"
	"embed"
	"encoding/base32"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// DB wraps SQLite. Reads may run concurrently; writes are serialized
// through Write so transactions stay short and never contend.
type DB struct {
	sql     *sql.DB
	writeMu sync.Mutex
	subs    subscribers
}

// Open opens (creating if needed) the database at path and applies
// pending migrations.
func Open(ctx context.Context, path string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	q := url.Values{}
	for _, p := range []string{"foreign_keys(1)", "busy_timeout(5000)", "journal_mode(WAL)", "synchronous(NORMAL)"} {
		q.Add("_pragma", p)
	}
	q.Set("_txlock", "immediate")
	conn, err := sql.Open("sqlite", "file:"+path+"?"+q.Encode())
	if err != nil {
		return nil, err
	}
	db := &DB{sql: conn}
	if err := db.migrate(ctx); err != nil {
		conn.Close()
		return nil, err
	}
	return db, nil
}

func (db *DB) Close() error { return db.sql.Close() }

// Read exposes the pool for queries. Never use it for writes.
func (db *DB) Read() *sql.DB { return db.sql }

// Write runs fn in a transaction, one writer at a time. Use Change
// instead when the write must also record execution events.
func (db *DB) Write(ctx context.Context, fn func(tx *sql.Tx) error) error {
	return db.write(ctx, fn, nil)
}

// write runs fn in a transaction and, after a successful commit, calls
// afterCommit while still holding the writer lock.
func (db *DB) write(ctx context.Context, fn func(tx *sql.Tx) error, afterCommit func()) error {
	db.writeMu.Lock()
	defer db.writeMu.Unlock()
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if afterCommit != nil {
		afterCommit()
	}
	return nil
}

// Now returns the canonical stored time format: UTC ISO 8601.
func Now() string { return time.Now().UTC().Format(time.RFC3339Nano) }

// NewID returns a collision-resistant id with a readable prefix, e.g.
// "prj-01j9x6k2r8b7m3q4d5f6g7h8". IDs are never display names.
func NewID(prefix string) string {
	b := make([]byte, 12)
	rand.Read(b)
	return prefix + "-" + strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b))
}

type migration struct {
	version int
	name    string
	sql     string
}

func loadMigrations() ([]migration, error) {
	entries, err := fs.ReadDir(migrationFiles, "migrations")
	if err != nil {
		return nil, err
	}
	var ms []migration
	for _, e := range entries {
		num, _, ok := strings.Cut(e.Name(), "_")
		v, err := strconv.Atoi(num)
		if !ok || err != nil {
			return nil, fmt.Errorf("migration %q: name must start with a number and '_'", e.Name())
		}
		body, err := migrationFiles.ReadFile("migrations/" + e.Name())
		if err != nil {
			return nil, err
		}
		ms = append(ms, migration{version: v, name: e.Name(), sql: string(body)})
	}
	sort.Slice(ms, func(i, j int) bool { return ms[i].version < ms[j].version })
	for i := 1; i < len(ms); i++ {
		if ms[i].version == ms[i-1].version {
			return nil, fmt.Errorf("duplicate migration version %d", ms[i].version)
		}
	}
	return ms, nil
}

func (db *DB) migrate(ctx context.Context) error {
	ms, err := loadMigrations()
	if err != nil {
		return err
	}
	return db.Write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY, name TEXT NOT NULL, applied_at TEXT NOT NULL)`); err != nil {
			return err
		}
		var current int
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&current); err != nil {
			return err
		}
		if latest := ms[len(ms)-1].version; current > latest {
			return fmt.Errorf("database schema version %d is newer than this app (%d)", current, latest)
		}
		for _, m := range ms {
			if m.version <= current {
				continue
			}
			if _, err := tx.ExecContext(ctx, m.sql); err != nil {
				return fmt.Errorf("migration %s: %w", m.name, err)
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, ?, ?)`,
				m.version, m.name, Now()); err != nil {
				return err
			}
		}
		return nil
	})
}

// SchemaVersion reports the highest applied migration.
func (db *DB) SchemaVersion(ctx context.Context) (int, error) {
	var v int
	err := db.sql.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&v)
	return v, err
}
