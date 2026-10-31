package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
)

// SettingKeys lists the app settings the UI may read and write. Anything
// else is rejected so the binding cannot become a generic key/value store.
var SettingKeys = map[string]bool{
	"ui.reducedMotion": true,
	"ui.lastProjectId": true,
	"ui.officeView":    true,
	"ui.theme":         true,
	"ui.tutorialSeen":  true,
	SettingMaxActive:   true,
	SettingNotifyTodos: true,
}

// SettingNotifyTodos is "off" when the person turned off notifications of
// new to-dos (on by default).
const SettingNotifyTodos = "ui.notifyTodos"

// SettingMaxActive is how many AI steps may run at once, app-wide and per
// project (the engine's MaxActive and ProjectMaxActive).
const SettingMaxActive = "engine.maxActive"

// MaxActive bounds and default for SettingMaxActive. More at once uses a
// subscription's usage limit up faster.
const (
	MaxActiveMin     = 1
	MaxActiveMax     = 6
	MaxActiveDefault = 2
)

// ParseMaxActive reads a stored SettingMaxActive value.
func ParseMaxActive(v string) (int, error) {
	n, err := strconv.Atoi(v)
	if err != nil || n < MaxActiveMin || n > MaxActiveMax {
		return 0, fmt.Errorf("동시에 실행할 AI 업무 수는 %d~%d 사이여야 합니다: %q", MaxActiveMin, MaxActiveMax, v)
	}
	return n, nil
}

var ErrUnknownSetting = errors.New("unknown setting key")

func checkKey(key string) error {
	if !SettingKeys[key] {
		return fmt.Errorf("%w: %q", ErrUnknownSetting, key)
	}
	return nil
}

// Settings returns every stored allowed setting.
func (db *DB) Settings(ctx context.Context) (map[string]string, error) {
	rows, err := db.sql.QueryContext(ctx, `SELECT key, value FROM app_settings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		if SettingKeys[k] {
			out[k] = v
		}
	}
	return out, rows.Err()
}

func (db *DB) SetSetting(ctx context.Context, key, value string) error {
	if err := checkKey(key); err != nil {
		return err
	}
	if key == SettingMaxActive {
		if _, err := ParseMaxActive(value); err != nil {
			return err
		}
	}
	return db.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO app_settings (key, value, updated_at) VALUES (?, ?, ?)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
			key, value, Now())
		return err
	})
}
