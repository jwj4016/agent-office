package engine

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"agent-office/internal/storage"
)

// maxSharedFile bounds a shared copy.
const maxSharedFile = 50 << 20

// maxDocumentsInline bounds the shared text pasted into a step's
// instructions; longer material is given by path only.
const maxDocumentsInline = 32 << 10

// ShareArtifact copies a run result into the shared documents as a new
// document or a new version of one. The copy is read-only and its hash is
// checked against the stored result before it is recorded.
func (e *Engine) ShareArtifact(ctx context.Context, projectID, artifactID string, nv storage.NewDocumentVersion) (storage.DocumentVersion, error) {
	if err := e.db.CheckScope(ctx, projectID, storage.Ref{Kind: storage.RefArtifact, ID: artifactID}); err != nil {
		return storage.DocumentVersion{}, err
	}
	var rel, hash, validity string
	if err := e.db.Read().QueryRowContext(ctx, `SELECT path, hash, validity FROM artifacts WHERE id = ?`, artifactID).Scan(&rel, &hash, &validity); err != nil {
		return storage.DocumentVersion{}, err
	}
	if validity != "valid" {
		return storage.DocumentVersion{}, fmt.Errorf("%w: 재작업으로 대체된 결과는 공유할 수 없습니다", storage.ErrInvalid)
	}
	src, err := e.resolveInside(filepath.Join(e.cfg.DataDir, filepath.FromSlash(rel)))
	if err != nil {
		return storage.DocumentVersion{}, fmt.Errorf("결과 파일을 열 수 없습니다: %w", err)
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return storage.DocumentVersion{}, err
	}
	if sum := sha256.Sum256(data); hex.EncodeToString(sum[:]) != hash {
		return storage.DocumentVersion{}, fmt.Errorf("%w: 결과 파일이 기록과 다릅니다. 공유하지 않았습니다", storage.ErrInvalid)
	}
	nv.SourceKind, nv.SourceArtifactID = "artifact", artifactID
	return e.recordCopy(ctx, projectID, filepath.Base(src), data, nv)
}

// ShareWorkspaceFile copies a file from the service's work folder into
// the shared documents. Only regular files inside the folder are taken
// (no links, nothing outside it), and a file that changes while it is
// copied is refused so the person can try again once it is stable.
func (e *Engine) ShareWorkspaceFile(ctx context.Context, projectID, relPath string, nv storage.NewDocumentVersion) (storage.DocumentVersion, error) {
	p, err := e.db.Project(ctx, projectID)
	if err != nil {
		return storage.DocumentVersion{}, err
	}
	if p.WorkspacePath == "" {
		return storage.DocumentVersion{}, fmt.Errorf("%w: 이 서비스에는 작업 폴더가 정해져 있지 않습니다", storage.ErrInvalid)
	}
	clean := filepath.Clean(filepath.FromSlash(strings.TrimSpace(relPath)))
	if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || filepath.VolumeName(clean) != "" {
		return storage.DocumentVersion{}, fmt.Errorf("%w: 작업 폴더 안의 파일 경로를 적으세요", storage.ErrInvalid)
	}
	root, err := filepath.EvalSymlinks(p.WorkspacePath)
	if err != nil {
		return storage.DocumentVersion{}, fmt.Errorf("작업 폴더를 열 수 없습니다: %w", err)
	}
	path := filepath.Join(root, clean)
	before, err := os.Lstat(path)
	if err != nil {
		return storage.DocumentVersion{}, fmt.Errorf("%w: 파일을 찾을 수 없습니다: %s", storage.ErrInvalid, filepath.ToSlash(clean))
	}
	if !before.Mode().IsRegular() {
		return storage.DocumentVersion{}, fmt.Errorf("%w: 일반 파일만 공유할 수 있습니다(링크·폴더 제외)", storage.ErrInvalid)
	}
	if resolved, err := filepath.EvalSymlinks(path); err != nil || resolved != path {
		return storage.DocumentVersion{}, fmt.Errorf("%w: 작업 폴더 밖을 가리키는 경로는 공유할 수 없습니다", storage.ErrInvalid)
	}
	if before.Size() > maxSharedFile {
		return storage.DocumentVersion{}, fmt.Errorf("%w: %dMB보다 큰 파일은 공유할 수 없습니다", storage.ErrInvalid, maxSharedFile>>20)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return storage.DocumentVersion{}, err
	}
	after, err := os.Lstat(path)
	if err != nil || after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) || int64(len(data)) != before.Size() {
		return storage.DocumentVersion{}, fmt.Errorf("%w: 파일이 복사되는 동안 바뀌었습니다. 저장이 끝난 뒤 다시 공유하세요", storage.ErrInvalid)
	}
	nv.SourceKind, nv.SourcePath = "workspace", filepath.ToSlash(clean)
	return e.recordCopy(ctx, projectID, filepath.Base(path), data, nv)
}

// recordCopy writes a read-only copy, checks it, and records it. A copy
// the database refuses (e.g. the same content as the last version) is
// removed again.
func (e *Engine) recordCopy(ctx context.Context, projectID, name string, data []byte, nv storage.NewDocumentVersion) (storage.DocumentVersion, error) {
	dir := filepath.Join(e.projectDir(projectID), "documents", storage.NewID("copy"))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return storage.DocumentVersion{}, err
	}
	dst := filepath.Join(dir, name)
	if err := os.WriteFile(dst, data, 0o400); err != nil {
		return storage.DocumentVersion{}, err
	}
	discard := func() { os.Chmod(dst, 0o600); os.RemoveAll(dir) }
	copied, err := os.ReadFile(dst)
	if err != nil || !bytes.Equal(copied, data) {
		discard()
		return storage.DocumentVersion{}, errors.New("공유 사본을 확인하지 못했습니다. 다시 시도하세요")
	}
	sum := sha256.Sum256(data)
	rel, _ := filepath.Rel(e.cfg.DataDir, dst)
	nv.FileName, nv.Path, nv.Hash, nv.Size = name, filepath.ToSlash(rel), hex.EncodeToString(sum[:]), int64(len(data))
	_, v, err := e.db.AddDocumentVersion(ctx, projectID, nv)
	if err != nil {
		discard()
		return v, err
	}
	return v, nil
}

// DocumentContent is a shared version's text, for reading in the app.
type DocumentContent struct {
	storage.DocumentVersion
	Title     string `json:"title"`
	Content   string `json:"content"`
	Binary    bool   `json:"binary"`
	Truncated bool   `json:"truncated"`
	HashOK    bool   `json:"hashOk"`
}

// ReadDocumentVersion reads a shared copy and checks its hash.
func (e *Engine) ReadDocumentVersion(ctx context.Context, projectID, versionID string) (DocumentContent, error) {
	v, doc, err := e.db.DocumentVersion(ctx, projectID, versionID)
	if err != nil {
		return DocumentContent{}, err
	}
	c := DocumentContent{DocumentVersion: v, Title: doc.Title}
	data, err := e.readDocumentCopy(v)
	if err != nil {
		return c, err
	}
	sum := sha256.Sum256(data)
	c.HashOK = hex.EncodeToString(sum[:]) == v.Hash
	if len(data) > maxArtifactRead {
		data, c.Truncated = data[:maxArtifactRead], true
	}
	if utf8.Valid(data) || c.Truncated {
		c.Content = string(data)
	} else {
		c.Binary = true
	}
	return c, nil
}

func (e *Engine) readDocumentCopy(v storage.DocumentVersion) ([]byte, error) {
	full, err := e.resolveInside(filepath.Join(e.cfg.DataDir, filepath.FromSlash(v.Path)))
	if err != nil {
		return nil, fmt.Errorf("공유 자료 파일을 열 수 없습니다: %w", err)
	}
	return os.ReadFile(full)
}

// WorkDocumentsSection is what a work item's run is told about the
// shared documents it links, and the versions to pin. Text up to a total
// limit is pasted in; everything else is given by its read-only path.
func (e *Engine) WorkDocumentsSection(ctx context.Context, projectID, workItemID string) (string, []string, error) {
	links, err := e.db.WorkDocuments(ctx, projectID, workItemID)
	if err != nil {
		return "", nil, err
	}
	if len(links) == 0 {
		return "", nil, nil
	}
	var b strings.Builder
	var ids []string
	budget := maxDocumentsInline
	b.WriteString("## 참고 공유 자료\n다른 업무에서 공유한 자료입니다. 고정된 사본이므로 수정하지 말고 읽기만 하세요.\n")
	for _, l := range links {
		v := l.Version
		if v.WithdrawnAt != "" {
			continue
		}
		ids = append(ids, v.ID)
		stage := "초안"
		if v.Stage == "final" {
			stage = "확정"
		}
		full := filepath.Join(e.cfg.DataDir, filepath.FromSlash(v.Path))
		fmt.Fprintf(&b, "\n### %s %d판 (%s) — %s\n파일: %s\n", l.Document.Title, v.Number, stage, v.FileName, full)
		data, err := e.readDocumentCopy(v)
		switch {
		case err != nil:
			b.WriteString("(사본을 읽지 못했습니다. 경로의 파일을 확인하세요)\n")
		case !utf8.Valid(data) || len(data) > budget:
			b.WriteString("(내용이 길거나 텍스트가 아니어서 경로만 전달합니다)\n")
		default:
			budget -= len(data)
			b.WriteString("````\n" + string(data) + "\n````\n")
		}
	}
	if len(ids) == 0 {
		return "", nil, nil
	}
	return strings.TrimSpace(b.String()), ids, nil
}
