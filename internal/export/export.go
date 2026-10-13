// Package export writes one service (project) to a zip file a person can
// keep or move: its organization context, roles, people, flows, versions,
// runs, records and result files. Keys, key references, login files,
// provider sessions and executable paths are never included (spec §5,
// T20), and any stored key value found in the content is masked.
package export

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"agent-office/internal/secrets"
	"agent-office/internal/storage"
)

// Format names the archive layout; FormatVersion changes when it does.
const (
	Format        = "agent-office-project-export"
	FormatVersion = 1
	// Mask replaces any key value found in exported content.
	Mask = "[삭제된 비밀값]"
)

type Options struct {
	DB         *storage.DB
	DataDir    string
	Secrets    secrets.Store
	AppVersion string
	Now        func() time.Time
}

type Result struct {
	ProjectName string `json:"projectName"`
	Tables      int    `json:"tables"`
	Rows        int    `json:"rows"`
	Artifacts   int    `json:"artifacts"`
	// Missing lists result files recorded in the database but not found
	// (or outside the data folder); they are listed, not exported.
	Missing []string `json:"missing"`
	// Masked counts key values (or key-shaped text) replaced by Mask.
	Masked int `json:"masked"`
}

// Excluded is written into the manifest so a reader knows what is not
// there on purpose.
var Excluded = []string{
	"API 키와 OS 비밀 저장소의 값",
	"키 참조(secret_ref)와 연결의 실행 파일 경로·설정",
	"Codex·Claude 로그인 파일과 토큰",
	"공급자 세션 ID(provider_session)",
	"앱 설정(app_settings)",
}

// projectTables are exported whole for the project.
var projectTables = []string{
	"assignments", "workflows", "workflow_versions", "runs", "step_attempts", "artifacts",
	"messages", "approvals", "execution_events", "workspaces", "design_requests",
}

// blanked columns are machine- or session-specific and never exported.
var blanked = map[string][]string{
	"step_attempts": {"provider_session"},
}

// keyShaped catches common API key formats even when no stored key is
// known (e.g. a key someone pasted into an instruction).
var keyShaped = regexp.MustCompile(`\b(sk-ant-[A-Za-z0-9_\-]{16,}|sk-(proj-)?[A-Za-z0-9_\-]{20,}|ghp_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,}|AIza[0-9A-Za-z_\-]{30,})`)

// Project writes the project's archive to w.
func Project(ctx context.Context, o Options, projectID string, w io.Writer) (Result, error) {
	if o.Now == nil {
		o.Now = time.Now
	}
	q := o.DB.Read()
	var res Result
	var orgID string
	if err := q.QueryRowContext(ctx, `SELECT name, organization_id FROM projects WHERE id = ?`, projectID).Scan(&res.ProjectName, &orgID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return res, fmt.Errorf("서비스를 찾을 수 없습니다")
		}
		return res, err
	}
	m := masker{known: knownSecrets(ctx, q, o.Secrets)}

	tables := map[string][]map[string]any{}
	dump := func(name, query string, args ...any) error {
		rows, err := rowsOf(ctx, q, query, args...)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		for _, r := range rows {
			for _, col := range blanked[name] {
				if _, ok := r[col]; ok {
					r[col] = ""
				}
			}
		}
		tables[name] = rows
		return nil
	}
	if err := dump("projects", `SELECT * FROM projects WHERE id = ?`, projectID); err != nil {
		return res, err
	}
	if err := dump("organizations", `SELECT * FROM organizations WHERE id = ?`, orgID); err != nil {
		return res, err
	}
	for _, t := range projectTables {
		if err := dump(t, `SELECT * FROM `+t+` WHERE project_id = ?`, projectID); err != nil {
			return res, err
		}
	}
	// Roles the project uses, with their parents (instructions inherit).
	if err := dump("roles", `WITH RECURSIVE used(id) AS (
		SELECT role_id FROM assignments WHERE project_id = ?
		UNION SELECT r.parent_role_id FROM roles r JOIN used u ON r.id = u.id WHERE r.parent_role_id <> '')
		SELECT * FROM roles WHERE id IN (SELECT id FROM used) ORDER BY name`, projectID); err != nil {
		return res, err
	}
	// Connections only by name and kind, so a reader knows who did what.
	if err := dump("provider_connections", `SELECT id, name, provider FROM provider_connections WHERE id IN (
		SELECT connection_id FROM assignments WHERE project_id = ? AND connection_id IS NOT NULL
		UNION SELECT connection_id FROM design_requests WHERE project_id = ?) ORDER BY name`, projectID, projectID); err != nil {
		return res, err
	}

	zw := zip.NewWriter(w)
	put := func(name string, data []byte) error {
		f, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: o.Now().UTC()})
		if err != nil {
			return err
		}
		_, err = f.Write(m.mask(data))
		return err
	}
	names := make([]string, 0, len(tables))
	for n := range tables {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		data, err := json.MarshalIndent(tables[n], "", "  ")
		if err != nil {
			return res, err
		}
		if err := put("data/"+n+".json", data); err != nil {
			return res, err
		}
		res.Tables++
		res.Rows += len(tables[n])
	}

	// Result files, read only from inside the data folder.
	base, err := filepath.EvalSymlinks(o.DataDir)
	if err != nil {
		return res, err
	}
	for _, a := range tables["artifacts"] {
		rel, _ := a["path"].(string)
		data, err := readInside(base, rel)
		if err != nil {
			res.Missing = append(res.Missing, rel)
			continue
		}
		if err := put("files/"+path.Clean(rel), data); err != nil {
			return res, err
		}
		res.Artifacts++
	}

	var schema int
	q.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&schema)
	manifest, _ := json.MarshalIndent(map[string]any{
		"format": Format, "formatVersion": FormatVersion, "exportedAt": o.Now().UTC().Format(time.RFC3339),
		"appVersion": o.AppVersion, "schemaVersion": schema, "projectId": projectID, "projectName": res.ProjectName,
		"tables": names, "artifacts": res.Artifacts, "missingFiles": res.Missing, "excluded": Excluded,
	}, "", "  ")
	if err := put("manifest.json", manifest); err != nil {
		return res, err
	}
	if err := put("README.txt", []byte(readme(res.ProjectName))); err != nil {
		return res, err
	}
	res.Masked = m.count
	if res.Missing == nil {
		res.Missing = []string{}
	}
	return res, zw.Close()
}

func readme(name string) string {
	return "Agent Office 서비스 내보내기: " + name + "\n\n" +
		"data/  서비스·역할·담당자·업무 흐름·버전·실행·기록(JSON, DB 행 그대로)\n" +
		"files/ 검증된 결과 파일(데이터 폴더 기준 경로)\n" +
		"manifest.json  형식 버전과 포함하지 않은 항목\n\n" +
		"포함하지 않은 항목:\n- " + strings.Join(Excluded, "\n- ") + "\n\n" +
		"내용 중 저장된 키와 같은 값이나 키 형태의 문자열은 '" + Mask + "'로 바꿨습니다.\n"
}

// rowsOf returns query rows as column→value maps (text for text and
// blobs, numbers as numbers, NULL as nil).
func rowsOf(ctx context.Context, q *sql.DB, query string, args ...any) ([]map[string]any, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	out := []map[string]any{}
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		r := make(map[string]any, len(cols))
		for i, c := range cols {
			if b, ok := vals[i].([]byte); ok {
				r[c] = string(b)
			} else {
				r[c] = vals[i]
			}
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// knownSecrets returns every stored key value, so it can be masked
// wherever it appears.
func knownSecrets(ctx context.Context, q *sql.DB, store secrets.Store) []string {
	if store == nil {
		return nil
	}
	rows, err := q.QueryContext(ctx, `SELECT secret_ref FROM provider_connections WHERE secret_ref <> ''`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var ref string
		rows.Scan(&ref)
		if v, err := store.Get(ref); err == nil && len(strings.TrimSpace(v)) >= 8 {
			out = append(out, strings.TrimSpace(v))
		}
	}
	return out
}

type masker struct {
	known []string
	count int
}

func (m *masker) mask(data []byte) []byte {
	for _, k := range m.known {
		if n := bytes.Count(data, []byte(k)); n > 0 {
			m.count += n
			data = bytes.ReplaceAll(data, []byte(k), []byte(Mask))
		}
	}
	return keyShaped.ReplaceAllFunc(data, func([]byte) []byte {
		m.count++
		return []byte(Mask)
	})
}

// readInside reads a data-folder-relative file, refusing paths and
// symlinks that lead outside the data folder.
func readInside(base, rel string) ([]byte, error) {
	clean := path.Clean(rel)
	if rel == "" || path.IsAbs(clean) || filepath.IsAbs(rel) || clean == ".." || strings.HasPrefix(clean, "../") {
		return nil, os.ErrNotExist
	}
	full := filepath.Join(base, filepath.FromSlash(rel))
	resolved, err := filepath.EvalSymlinks(full)
	if err != nil {
		return nil, err
	}
	if resolved != full {
		return nil, fmt.Errorf("데이터 폴더 밖을 가리킵니다")
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.Mode().IsRegular() {
		return nil, os.ErrNotExist
	}
	return os.ReadFile(resolved)
}
