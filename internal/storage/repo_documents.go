package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"agent-office/internal/domain"
)

// SharedDocument is one piece of shared material and its versions
// (plan §4.4). Versions are numbered from 1 within the document.
type SharedDocument struct {
	ID                string            `json:"id"`
	ProjectID         string            `json:"projectId"`
	Title             string            `json:"title"`
	Categories        []string          `json:"categories"`
	OwnerDepartmentID string            `json:"ownerDepartmentId"`
	WorkItemID        string            `json:"workItemId"`
	Scope             string            `json:"scope"` // service | departments
	ScopeDepartments  []string          `json:"scopeDepartments"`
	Status            string            `json:"status"` // active | withdrawn
	CreatedAt         string            `json:"createdAt"`
	UpdatedAt         string            `json:"updatedAt"`
	Versions          []DocumentVersion `json:"versions"`
}

// DocumentVersion is one frozen copy. Its file, hash and number never
// change; only its review stage (draft, then final) and withdrawal do.
type DocumentVersion struct {
	ID               string `json:"id"`
	DocumentID       string `json:"documentId"`
	ProjectID        string `json:"projectId"`
	Number           int    `json:"number"`
	Stage            string `json:"stage"` // draft | final
	ConfirmedBy      string `json:"confirmedBy"`
	ConfirmedAt      string `json:"confirmedAt"`
	ConfirmationNote string `json:"confirmationNote"`
	SourceKind       string `json:"sourceKind"` // artifact | workspace
	SourceArtifactID string `json:"sourceArtifactId"`
	SourcePath       string `json:"sourcePath"`
	FileName         string `json:"fileName"`
	Path             string `json:"path"` // copy, relative to the data folder
	Hash             string `json:"hash"`
	Size             int64  `json:"size"`
	WithdrawnAt      string `json:"withdrawnAt"`
	CreatedAt        string `json:"createdAt"`
}

// NewDocumentVersion describes a copy to record. With DocumentID it is a
// new version of that document; otherwise a new document is made from
// the other fields.
type NewDocumentVersion struct {
	DocumentID        string   `json:"documentId"`
	Title             string   `json:"title"`
	Categories        []string `json:"categories"`
	OwnerDepartmentID string   `json:"ownerDepartmentId"`
	WorkItemID        string   `json:"workItemId"`
	Scope             string   `json:"scope"`
	ScopeDepartments  []string `json:"scopeDepartments"`

	SourceKind       string `json:"-"`
	SourceArtifactID string `json:"-"`
	SourcePath       string `json:"-"`
	FileName         string `json:"-"`
	Path             string `json:"-"`
	Hash             string `json:"-"`
	Size             int64  `json:"-"`
}

const documentCols = `id, project_id, title, categories, COALESCE(owner_department_id, ''), COALESCE(work_item_id, ''), scope,
	scope_departments, status, created_at, updated_at`

func scanDocument(s interface{ Scan(...any) error }) (SharedDocument, error) {
	var d SharedDocument
	var cats, scope string
	err := s.Scan(&d.ID, &d.ProjectID, &d.Title, &cats, &d.OwnerDepartmentID, &d.WorkItemID, &d.Scope, &scope, &d.Status, &d.CreatedAt, &d.UpdatedAt)
	json.Unmarshal([]byte(cats), &d.Categories)
	json.Unmarshal([]byte(scope), &d.ScopeDepartments)
	d.Categories, d.ScopeDepartments, d.Versions = orEmpty(d.Categories), orEmpty(d.ScopeDepartments), []DocumentVersion{}
	return d, err
}

const versionCols = `id, document_id, project_id, number, stage, confirmed_by, COALESCE(confirmed_at, ''), confirmation_note, source_kind,
	COALESCE(source_artifact_id, ''), source_path, file_name, path, hash, size, COALESCE(withdrawn_at, ''), created_at`

func scanVersion(s interface{ Scan(...any) error }) (DocumentVersion, error) {
	var v DocumentVersion
	err := s.Scan(&v.ID, &v.DocumentID, &v.ProjectID, &v.Number, &v.Stage, &v.ConfirmedBy, &v.ConfirmedAt, &v.ConfirmationNote, &v.SourceKind,
		&v.SourceArtifactID, &v.SourcePath, &v.FileName, &v.Path, &v.Hash, &v.Size, &v.WithdrawnAt, &v.CreatedAt)
	return v, err
}

func documentTx(ctx context.Context, q interface {
	queryer
	rowsQueryer
}, projectID, id string) (SharedDocument, error) {
	d, err := scanDocument(q.QueryRowContext(ctx, `SELECT `+documentCols+` FROM shared_documents WHERE id = ? AND project_id = ?`, id, projectID))
	if errors.Is(err, sql.ErrNoRows) {
		return d, fmt.Errorf("document %q: %w", id, ErrNotInProject)
	}
	if err != nil {
		return d, err
	}
	rows, err := q.QueryContext(ctx, `SELECT `+versionCols+` FROM shared_document_versions WHERE document_id = ? ORDER BY number`, id)
	if err != nil {
		return d, err
	}
	defer rows.Close()
	for rows.Next() {
		v, err := scanVersion(rows)
		if err != nil {
			return d, err
		}
		d.Versions = append(d.Versions, v)
	}
	return d, rows.Err()
}

func versionTx(ctx context.Context, q queryer, projectID, id string) (DocumentVersion, error) {
	v, err := scanVersion(q.QueryRowContext(ctx, `SELECT `+versionCols+` FROM shared_document_versions WHERE id = ? AND project_id = ?`, id, projectID))
	if errors.Is(err, sql.ErrNoRows) {
		return v, fmt.Errorf("document version %q: %w", id, ErrNotInProject)
	}
	return v, err
}

// Document returns one shared document with its versions.
func (db *DB) Document(ctx context.Context, projectID, id string) (SharedDocument, error) {
	if err := db.CheckScope(ctx, projectID); err != nil {
		return SharedDocument{}, err
	}
	return documentTx(ctx, db.sql, projectID, id)
}

// DocumentVersion returns a version and its document.
func (db *DB) DocumentVersion(ctx context.Context, projectID, id string) (DocumentVersion, SharedDocument, error) {
	v, err := versionTx(ctx, db.sql, projectID, id)
	if err != nil {
		return v, SharedDocument{}, err
	}
	d, err := documentTx(ctx, db.sql, projectID, v.DocumentID)
	return v, d, err
}

// VisibleTo reports whether a department may see, link and hear about
// the document. "" stands for a person acting for the whole service.
func (d SharedDocument) VisibleTo(departmentID string) bool {
	if d.Scope != "departments" || departmentID == "" {
		return true
	}
	return departmentID == d.OwnerDepartmentID || slices.Contains(d.ScopeDepartments, departmentID)
}

// Latest is the newest version that is not withdrawn, or false.
func (d SharedDocument) Latest() (DocumentVersion, bool) {
	for i := len(d.Versions) - 1; i >= 0; i-- {
		if d.Versions[i].WithdrawnAt == "" {
			return d.Versions[i], true
		}
	}
	return DocumentVersion{}, false
}

// DocumentFilter narrows the document list. VisibleTo keeps only what a
// department may see.
type DocumentFilter struct {
	Query            string `json:"query"`
	Category         string `json:"category"`
	OwnerDepartment  string `json:"ownerDepartmentId"`
	VisibleTo        string `json:"visibleTo"`
	Stage            string `json:"stage"` // stage of the latest version
	IncludeWithdrawn bool   `json:"includeWithdrawn"`
}

// Documents lists a service's shared documents, most recently changed
// first.
func (db *DB) Documents(ctx context.Context, projectID string, f DocumentFilter) ([]SharedDocument, error) {
	if err := db.CheckScope(ctx, projectID); err != nil {
		return nil, err
	}
	rows, err := db.sql.QueryContext(ctx, `SELECT id FROM shared_documents WHERE project_id = ? ORDER BY updated_at DESC, rowid DESC`, projectID)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	out := []SharedDocument{}
	q := strings.ToLower(strings.TrimSpace(f.Query))
	for _, id := range ids {
		d, err := documentTx(ctx, db.sql, projectID, id)
		if err != nil {
			return nil, err
		}
		latest, ok := d.Latest()
		switch {
		case !f.IncludeWithdrawn && (d.Status == "withdrawn" || !ok):
		case q != "" && !strings.Contains(strings.ToLower(d.Title), q) && !strings.Contains(strings.ToLower(latest.FileName), q):
		case f.Category != "" && !slices.Contains(d.Categories, f.Category):
		case f.OwnerDepartment != "" && d.OwnerDepartmentID != f.OwnerDepartment:
		case f.VisibleTo != "" && !d.VisibleTo(f.VisibleTo):
		case f.Stage != "" && latest.Stage != f.Stage:
		default:
			out = append(out, d)
		}
	}
	return out, nil
}

// AddDocumentVersion records a frozen copy as a new document or a new
// version of one, and emits document.shared or document.revised in the
// same transaction. A copy identical to the previous version is refused.
// Work items that used an earlier version get an impact check.
func (db *DB) AddDocumentVersion(ctx context.Context, projectID string, nv NewDocumentVersion) (SharedDocument, DocumentVersion, error) {
	var docID, versionID string
	_, err := db.Change(ctx, func(c *Change) error {
		tx := c.Tx
		if err := writableProjectTx(ctx, tx, projectID); err != nil {
			return err
		}
		var doc SharedDocument
		var err error
		if nv.DocumentID == "" {
			if doc, err = newDocumentTx(ctx, tx, projectID, nv); err != nil {
				return err
			}
		} else if doc, err = documentTx(ctx, tx, projectID, nv.DocumentID); err != nil {
			return err
		}
		if doc.Status != "active" {
			return invalid("철회된 자료에는 새 버전을 올릴 수 없습니다")
		}
		number := 1
		if n := len(doc.Versions); n > 0 {
			number = doc.Versions[n-1].Number + 1
			if prev, ok := doc.Latest(); ok && prev.Hash == nv.Hash {
				return invalid("직전 버전(%d판)과 내용이 같아 새 버전을 만들지 않았습니다", prev.Number)
			}
		}
		docID, versionID = doc.ID, NewID("docv")
		now := Now()
		if _, err := tx.ExecContext(ctx, `INSERT INTO shared_document_versions (id, document_id, project_id, number, source_kind, source_artifact_id,
			source_path, file_name, path, hash, size, created_at) VALUES (?, ?, ?, ?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?, ?)`,
			versionID, doc.ID, projectID, number, nv.SourceKind, nv.SourceArtifactID, nv.SourcePath, nv.FileName, nv.Path, nv.Hash, nv.Size, now); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE shared_documents SET updated_at = ? WHERE id = ?`, now, doc.ID); err != nil {
			return err
		}
		kind := domain.EventDocumentShared
		if number > 1 {
			kind = domain.EventDocumentRevised
		}
		rec, err := emitWorkEventTx(ctx, c, projectID, kind, documentEvent(doc, number, versionID, ""))
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE shared_document_versions SET event_sequence = ? WHERE id = ?`, rec.Sequence, versionID); err != nil {
			return err
		}
		if number == 1 {
			return nil
		}
		var earlier []string
		for _, v := range doc.Versions {
			earlier = append(earlier, v.ID)
		}
		return openImpactChecksTx(ctx, tx, projectID, doc, earlier, "revised", versionID,
			fmt.Sprintf("‘%s’ %d판이 공유되었습니다. 이 업무가 쓴 이전 판과 비교해 영향을 확인하세요", doc.Title, number))
	})
	if err != nil {
		return SharedDocument{}, DocumentVersion{}, err
	}
	doc, err := db.Document(ctx, projectID, docID)
	if err != nil {
		return doc, DocumentVersion{}, err
	}
	v, err := versionTx(ctx, db.sql, projectID, versionID)
	return doc, v, err
}

func newDocumentTx(ctx context.Context, tx *sql.Tx, projectID string, nv NewDocumentVersion) (SharedDocument, error) {
	title := strings.TrimSpace(nv.Title)
	if title == "" {
		return SharedDocument{}, invalid("자료 제목이 필요합니다")
	}
	if nv.Scope == "" {
		nv.Scope = "service"
	}
	if nv.Scope != "service" && nv.Scope != "departments" {
		return SharedDocument{}, invalid("공유 범위는 service 또는 departments입니다")
	}
	scopeDepts := dedupe(nv.ScopeDepartments)
	if nv.Scope == "departments" && len(scopeDepts) == 0 && nv.OwnerDepartmentID == "" {
		return SharedDocument{}, invalid("공유할 부서를 하나 이상 고르세요")
	}
	if nv.Scope == "service" {
		scopeDepts = nil
	}
	for _, d := range append(slices.Clone(scopeDepts), nv.OwnerDepartmentID) {
		if d == "" {
			continue
		}
		if _, err := departmentTx(ctx, tx, projectID, d); err != nil {
			return SharedDocument{}, err
		}
	}
	if nv.WorkItemID != "" {
		w, err := workTx(ctx, tx, projectID, nv.WorkItemID)
		if err != nil {
			return SharedDocument{}, err
		}
		if nv.OwnerDepartmentID == "" {
			nv.OwnerDepartmentID = w.DepartmentID
		}
	}
	cats, _ := json.Marshal(orEmpty(dedupe(nv.Categories)))
	scope, _ := json.Marshal(orEmpty(scopeDepts))
	id, now := NewID("doc"), Now()
	if _, err := tx.ExecContext(ctx, `INSERT INTO shared_documents (id, project_id, title, categories, owner_department_id, work_item_id, scope,
		scope_departments, created_at, updated_at) VALUES (?, ?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), ?, ?, ?, ?)`,
		id, projectID, title, string(cats), nv.OwnerDepartmentID, nv.WorkItemID, nv.Scope, string(scope), now, now); err != nil {
		return SharedDocument{}, err
	}
	return documentTx(ctx, tx, projectID, id)
}

func documentEvent(doc SharedDocument, number int, versionID, note string) WorkEvent {
	return WorkEvent{DepartmentID: doc.OwnerDepartmentID, WorkItemID: doc.WorkItemID, Title: fmt.Sprintf("%s %d판", doc.Title, number),
		Note: note, Categories: doc.Categories, DocumentID: doc.ID, DocumentVersionID: versionID}
}

// openImpactChecksTx asks every work item that linked or ran with one of
// versions (other than the document's own work) to check a change.
func openImpactChecksTx(ctx context.Context, tx *sql.Tx, projectID string, doc SharedDocument, versions []string, cause, refID, summary string) error {
	items, err := usersOfVersionsTx(ctx, tx, versions)
	if err != nil {
		return err
	}
	now := Now()
	for _, w := range items {
		if w == doc.WorkItemID {
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO impact_checks (id, project_id, work_item_id, cause, ref_id, document_id, summary, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT(work_item_id, cause, ref_id) DO NOTHING`,
			NewID("imp"), projectID, w, cause, refID, doc.ID, summary, now, now); err != nil {
			return err
		}
	}
	return nil
}

// usersOfVersionsTx lists the work items that link (now) or pinned any of
// the versions.
func usersOfVersionsTx(ctx context.Context, q rowsQueryer, versions []string) ([]string, error) {
	if len(versions) == 0 {
		return nil, nil
	}
	ph := strings.TrimSuffix(strings.Repeat("?, ", len(versions)), ", ")
	args := make([]any, 0, 2*len(versions))
	for _, v := range versions {
		args = append(args, v)
	}
	for _, v := range versions {
		args = append(args, v)
	}
	rows, err := q.QueryContext(ctx, `SELECT work_item_id FROM document_links WHERE unlinked_at IS NULL AND version_id IN (`+ph+`)
		UNION SELECT work_item_id FROM document_pins WHERE version_id IN (`+ph+`) ORDER BY 1`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// ConfirmDocumentVersion marks a version final without touching its
// content. Confirming twice changes nothing and emits nothing; a
// withdrawn version cannot be confirmed.
func (db *DB) ConfirmDocumentVersion(ctx context.Context, projectID, versionID, note string) (DocumentVersion, error) {
	_, err := db.Change(ctx, func(c *Change) error {
		if err := writableProjectTx(ctx, c.Tx, projectID); err != nil {
			return err
		}
		v, err := versionTx(ctx, c.Tx, projectID, versionID)
		if err != nil {
			return err
		}
		doc, err := documentTx(ctx, c.Tx, projectID, v.DocumentID)
		if err != nil {
			return err
		}
		if v.WithdrawnAt != "" || doc.Status != "active" {
			return invalid("철회된 자료는 확정할 수 없습니다")
		}
		if v.Stage == "final" {
			return nil
		}
		if _, err := c.Tx.ExecContext(ctx, `UPDATE shared_document_versions SET stage = 'final', confirmed_by = ?, confirmed_at = ?, confirmation_note = ?
			WHERE id = ?`, domain.LocalOwner, Now(), strings.TrimSpace(note), versionID); err != nil {
			return err
		}
		_, err = emitWorkEventTx(ctx, c, projectID, domain.EventDocumentConfirmed, documentEvent(doc, v.Number, v.ID, strings.TrimSpace(note)))
		return err
	})
	if err != nil {
		return DocumentVersion{}, err
	}
	return versionTx(ctx, db.sql, projectID, versionID)
}

// WithdrawDocument withdraws one version, or with versionID "" the whole
// document. Work items that used what was withdrawn get an impact check;
// nothing is deleted.
func (db *DB) WithdrawDocument(ctx context.Context, projectID, documentID, versionID, reason string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return invalid("철회 이유가 필요합니다")
	}
	_, err := db.Change(ctx, func(c *Change) error {
		if err := writableProjectTx(ctx, c.Tx, projectID); err != nil {
			return err
		}
		doc, err := documentTx(ctx, c.Tx, projectID, documentID)
		if err != nil {
			return err
		}
		if doc.Status != "active" {
			return invalid("이미 철회된 자료입니다")
		}
		now := Now()
		var gone []string
		number := 0
		for _, v := range doc.Versions {
			if v.WithdrawnAt == "" && (versionID == "" || v.ID == versionID) {
				gone = append(gone, v.ID)
				number = v.Number
			}
		}
		if len(gone) == 0 {
			return invalid("철회할 버전이 없습니다")
		}
		for _, id := range gone {
			if _, err := c.Tx.ExecContext(ctx, `UPDATE shared_document_versions SET withdrawn_at = ? WHERE id = ?`, now, id); err != nil {
				return err
			}
		}
		ref, title := documentID, "‘"+doc.Title+"’ 자료가 철회되었습니다"
		if versionID != "" {
			ref, title = versionID, fmt.Sprintf("‘%s’ %d판이 철회되었습니다", doc.Title, number)
		} else if _, err := c.Tx.ExecContext(ctx, `UPDATE shared_documents SET status = 'withdrawn', updated_at = ? WHERE id = ?`, now, documentID); err != nil {
			return err
		}
		ev := documentEvent(doc, number, versionID, reason)
		if versionID == "" {
			ev.Title = doc.Title
		}
		if _, err := emitWorkEventTx(ctx, c, projectID, domain.EventDocumentWithdrawn, ev); err != nil {
			return err
		}
		return openImpactChecksTx(ctx, c.Tx, projectID, doc, gone, "withdrawn", ref, title+": "+reason)
	})
	return err
}

// ---- links and pins ----

// LinkedDocument is a version a work item refers to, with its document.
type LinkedDocument struct {
	Version  DocumentVersion `json:"version"`
	Document SharedDocument  `json:"document"`
	// Newer is the latest version's number when it is newer than this one.
	Newer    int    `json:"newer"`
	LinkedAt string `json:"linkedAt"`
}

// LinkDocument makes a version an input of a work item. The work's
// department must be allowed to see the document, and withdrawn
// material cannot be linked.
func (db *DB) LinkDocument(ctx context.Context, projectID, workItemID, versionID string) error {
	return db.Write(ctx, func(tx *sql.Tx) error {
		if err := writableProjectTx(ctx, tx, projectID); err != nil {
			return err
		}
		w, err := workTx(ctx, tx, projectID, workItemID)
		if err != nil {
			return err
		}
		v, err := versionTx(ctx, tx, projectID, versionID)
		if err != nil {
			return err
		}
		doc, err := documentTx(ctx, tx, projectID, v.DocumentID)
		if err != nil {
			return err
		}
		if v.WithdrawnAt != "" || doc.Status != "active" {
			return invalid("철회된 자료는 연결할 수 없습니다")
		}
		if doc.Scope == "departments" && (w.DepartmentID == "" || !doc.VisibleTo(w.DepartmentID)) {
			return invalid("이 업무의 부서는 이 자료를 볼 수 없습니다")
		}
		// One version of a document per work item: linking another
		// version replaces the earlier link.
		now := Now()
		if _, err := tx.ExecContext(ctx, `UPDATE document_links SET unlinked_at = ? WHERE work_item_id = ? AND unlinked_at IS NULL
			AND version_id IN (SELECT id FROM shared_document_versions WHERE document_id = ?) AND version_id <> ?`, now, workItemID, doc.ID, versionID); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO document_links (id, project_id, work_item_id, version_id, linked_at) VALUES (?, ?, ?, ?, ?)
			ON CONFLICT(work_item_id, version_id) DO UPDATE SET unlinked_at = NULL, linked_at = excluded.linked_at`,
			NewID("dlink"), projectID, workItemID, versionID, now)
		return err
	})
}

// UnlinkDocument removes a version from a work item's inputs; the link's
// record stays.
func (db *DB) UnlinkDocument(ctx context.Context, projectID, workItemID, versionID string) error {
	return db.Write(ctx, func(tx *sql.Tx) error {
		if err := writableProjectTx(ctx, tx, projectID); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `UPDATE document_links SET unlinked_at = ? WHERE project_id = ? AND work_item_id = ? AND version_id = ? AND unlinked_at IS NULL`,
			Now(), projectID, workItemID, versionID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return fmt.Errorf("link: %w", ErrNotInProject)
		}
		return nil
	})
}

// WorkDocuments lists the versions a work item currently links.
func (db *DB) WorkDocuments(ctx context.Context, projectID, workItemID string) ([]LinkedDocument, error) {
	if err := db.CheckScope(ctx, projectID, Ref{RefWorkItem, workItemID}); err != nil {
		return nil, err
	}
	rows, err := db.sql.QueryContext(ctx, `SELECT version_id, linked_at FROM document_links WHERE work_item_id = ? AND unlinked_at IS NULL ORDER BY linked_at`, workItemID)
	if err != nil {
		return nil, err
	}
	type link struct{ version, at string }
	var links []link
	for rows.Next() {
		var l link
		if err := rows.Scan(&l.version, &l.at); err != nil {
			rows.Close()
			return nil, err
		}
		links = append(links, l)
	}
	rows.Close()
	out := []LinkedDocument{}
	for _, l := range links {
		v, doc, err := db.DocumentVersion(ctx, projectID, l.version)
		if err != nil {
			return nil, err
		}
		ld := LinkedDocument{Version: v, Document: doc, LinkedAt: l.at}
		if latest, ok := doc.Latest(); ok && latest.Number > v.Number {
			ld.Newer = latest.Number
		}
		out = append(out, ld)
	}
	return out, nil
}

// RunDocuments lists the versions a run was given.
func (db *DB) RunDocuments(ctx context.Context, projectID, runID string) ([]DocumentVersion, error) {
	if err := db.CheckScope(ctx, projectID, Ref{RefRun, runID}); err != nil {
		return nil, err
	}
	rows, err := db.sql.QueryContext(ctx, `SELECT `+versionCols+` FROM shared_document_versions WHERE id IN
		(SELECT version_id FROM document_pins WHERE run_id = ?) ORDER BY document_id, number`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DocumentVersion{}
	for rows.Next() {
		v, err := scanVersion(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// pinDocumentsTx fixes the versions a run starts with.
func pinDocumentsTx(ctx context.Context, tx *sql.Tx, projectID, workItemID, runID string, versionIDs []string) error {
	now := Now()
	for _, v := range versionIDs {
		cur, err := versionTx(ctx, tx, projectID, v)
		if err != nil {
			return err
		}
		if cur.WithdrawnAt != "" {
			return invalid("실행을 준비하는 동안 자료 %d판이 철회되었습니다. 다시 실행하세요", cur.Number)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO document_pins (id, project_id, work_item_id, version_id, run_id, pinned_at) VALUES (?, ?, ?, ?, ?, ?)`,
			NewID("dpin"), projectID, workItemID, v, runID, now); err != nil {
			return err
		}
	}
	return nil
}

// ---- impact checks ----

// ImpactCheck asks a person to check a change to something a work item
// used.
type ImpactCheck struct {
	ID         string `json:"id"`
	ProjectID  string `json:"projectId"`
	WorkItemID string `json:"workItemId"`
	Cause      string `json:"cause"`
	RefID      string `json:"refId"`
	DocumentID string `json:"documentId"`
	Summary    string `json:"summary"`
	Status     string `json:"status"`
	Note       string `json:"note"`
	CreatedAt  string `json:"createdAt"`
}

// ImpactChecks lists a service's impact checks, or one work item's.
// status may be a status, "" for all.
func (db *DB) ImpactChecks(ctx context.Context, projectID, workItemID, status string) ([]ImpactCheck, error) {
	if err := db.CheckScope(ctx, projectID); err != nil {
		return nil, err
	}
	rows, err := db.sql.QueryContext(ctx, `SELECT id, project_id, work_item_id, cause, ref_id, COALESCE(document_id, ''), summary, status, note, created_at
		FROM impact_checks WHERE project_id = ? AND (? = '' OR work_item_id = ?) AND (? = '' OR status = ?) ORDER BY created_at DESC, rowid DESC`,
		projectID, workItemID, workItemID, status, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ImpactCheck{}
	for rows.Next() {
		var c ImpactCheck
		if err := rows.Scan(&c.ID, &c.ProjectID, &c.WorkItemID, &c.Cause, &c.RefID, &c.DocumentID, &c.Summary, &c.Status, &c.Note, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// CloseImpactCheck records that a person checked a change (resolved) or
// decided it does not matter (dismissed).
func (db *DB) CloseImpactCheck(ctx context.Context, projectID, id, status, note string) error {
	if status != "resolved" && status != "dismissed" {
		return invalid("영향 확인은 resolved 또는 dismissed로 닫습니다")
	}
	_, err := db.Change(ctx, func(c *Change) error {
		if err := writableProjectTx(ctx, c.Tx, projectID); err != nil {
			return err
		}
		res, err := c.Tx.ExecContext(ctx, `UPDATE impact_checks SET status = ?, note = ?, updated_at = ? WHERE id = ? AND project_id = ? AND status = 'open'`,
			status, strings.TrimSpace(note), Now(), id, projectID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return invalid("이미 닫혔거나 찾을 수 없는 영향 확인입니다")
		}
		return c.Emit(projectID, "", "", "impact.updated", map[string]string{"impactId": id, "status": status})
	})
	return err
}
