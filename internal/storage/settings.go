package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// SettingKeys lists the app settings the UI may read and write. Anything
// else is rejected so the binding cannot become a generic key/value store.
var SettingKeys = map[string]bool{
	"ui.reducedMotion": true,
	"ui.lastProjectId": true,
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
	return db.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO app_settings (key, value, updated_at) VALUES (?, ?, ?)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
			key, value, Now())
		return err
	})
}
