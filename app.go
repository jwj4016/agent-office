package main

import (
	"context"
	"errors"
	"log"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"agent-office/internal/secrets"
	"agent-office/internal/storage"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// EventPing is emitted after every Ping call so the UI can verify the
// Go → frontend event path independently of the binding return value.
const EventPing = "system:ping"

var errNotReady = errors.New("저장소가 준비되지 않았습니다")

// App is the only struct bound to the frontend. Its methods are the whole
// UI-facing API, so each one must validate its inputs.
type App struct {
	ctx     context.Context
	emit    func(ctx context.Context, name string, data ...interface{})
	seq     atomic.Int64
	dataDir string
	db      *storage.DB
	secrets secrets.Store
	initErr error
}

func NewApp() *App {
	return &App{emit: runtime.EventsEmit}
}

// dataDir returns AGENT_OFFICE_DATA_DIR when set (tests, side-by-side
// installs), otherwise the per-user config directory.
func dataDir() (string, error) {
	if d := os.Getenv("AGENT_OFFICE_DATA_DIR"); d != "" {
		return d, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "AgentOffice"), nil
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.secrets = secrets.Open()
	dir, err := dataDir()
	if err == nil {
		a.dataDir = dir
		a.db, err = storage.Open(ctx, filepath.Join(dir, "agent-office.db"))
	}
	if err != nil {
		a.initErr = err
		log.Printf("storage init failed: %v", err)
	}
}

func (a *App) shutdown(ctx context.Context) {
	if a.db != nil {
		a.db.Close()
	}
}

// PingResult is returned by Ping and also sent as the EventPing payload.
type PingResult struct {
	Sequence int64  `json:"sequence"`
	Message  string `json:"message"`
	At       string `json:"at"`
}

// Ping echoes the message back and emits the same result as an event.
func (a *App) Ping(message string) PingResult {
	res := PingResult{
		Sequence: a.seq.Add(1),
		Message:  message,
		At:       time.Now().UTC().Format(time.RFC3339Nano),
	}
	if a.ctx != nil {
		a.emit(a.ctx, EventPing, res)
	}
	return res
}

// SystemStatus describes where data lives and how secrets are kept.
type SystemStatus struct {
	DataDir           string `json:"dataDir"`
	SchemaVersion     int    `json:"schemaVersion"`
	SecretsPersistent bool   `json:"secretsPersistent"`
	Error             string `json:"error"`
}

func (a *App) SystemStatus() SystemStatus {
	st := SystemStatus{DataDir: a.dataDir}
	if a.secrets != nil {
		st.SecretsPersistent = a.secrets.Persistent()
	}
	if a.initErr != nil {
		st.Error = a.initErr.Error()
		return st
	}
	st.SchemaVersion, _ = a.db.SchemaVersion(a.ctx)
	return st
}

// GetSettings returns the stored UI settings (allow-listed keys only).
func (a *App) GetSettings() (map[string]string, error) {
	if a.db == nil {
		return nil, errNotReady
	}
	return a.db.Settings(a.ctx)
}

// SetSetting stores one allow-listed UI setting.
func (a *App) SetSetting(key, value string) error {
	if a.db == nil {
		return errNotReady
	}
	return a.db.SetSetting(a.ctx, key, value)
}
