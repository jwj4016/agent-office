package main

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// EventPing is emitted after every Ping call so the UI can verify the
// Go → frontend event path independently of the binding return value.
const EventPing = "system:ping"

// App is the only struct bound to the frontend. Its methods are the whole
// UI-facing API, so each one must validate its inputs.
type App struct {
	ctx  context.Context
	emit func(ctx context.Context, name string, data ...interface{})
	seq  atomic.Int64
}

func NewApp() *App {
	return &App{emit: runtime.EventsEmit}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) shutdown(ctx context.Context) {}

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
