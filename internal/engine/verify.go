package engine

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"agent-office/internal/domain"
	"agent-office/internal/storage"
)

// MaxOutputBytes bounds a single step output file.
const MaxOutputBytes = 50 << 20

type verifiedOutput struct {
	Key, Type, RelPath, Hash string
	Size                     int64
}

// verifyOutputs checks a step's outputs against its definition and runs
// its completion commands. A model claiming success is never enough.
func (e *Engine) verifyOutputs(ctx context.Context, n *domain.Node, dir, work string) ([]verifiedOutput, error) {
	var problems []string
	var out []verifiedOutput
	for _, o := range n.Outputs {
		path := outputPath(dir, o)
		v, err := e.verifyOne(o, path)
		if errors.Is(err, os.ErrNotExist) {
			if o.IsRequired() {
				problems = append(problems, fmt.Sprintf("필수 결과 %q가 없습니다", o.Key))
			}
			continue
		}
		if err != nil {
			problems = append(problems, fmt.Sprintf("결과 %q: %v", o.Key, err))
			continue
		}
		out = append(out, v)
	}
	if len(problems) == 0 && n.Completion != nil {
		for _, cmd := range n.Completion.Commands {
			if err := runVerifyCommand(ctx, cmd, work); err != nil {
				problems = append(problems, err.Error())
				break
			}
		}
	}
	if len(problems) > 0 {
		return nil, errors.New(strings.Join(problems, "; "))
	}
	return out, nil
}

// errOutsideData is returned for paths that leave the data dir or pass
// through a symbolic link on the way.
var errOutsideData = errors.New("허용된 경로 밖입니다 (심볼릭 링크를 거치는 경로는 허용하지 않음)")

// resolveInside checks that path lies inside the data dir and that no
// component below the data dir is a symbolic link: the fully resolved
// path must equal the data dir's resolved path joined with the relative
// path. The data dir itself may sit behind a link (/tmp on macOS).
func (e *Engine) resolveInside(path string) (string, error) {
	rel, err := filepath.Rel(e.cfg.DataDir, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", errOutsideData
	}
	base, err := filepath.EvalSymlinks(e.cfg.DataDir)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err // includes os.ErrNotExist for missing outputs
	}
	if resolved != filepath.Join(base, rel) {
		return "", errOutsideData
	}
	return resolved, nil
}

func (e *Engine) verifyOne(o domain.Output, path string) (verifiedOutput, error) {
	resolved, err := e.resolveInside(path)
	if err != nil {
		return verifiedOutput{}, err
	}
	info, err := os.Lstat(resolved)
	if err != nil {
		return verifiedOutput{}, err
	}
	if !info.Mode().IsRegular() {
		return verifiedOutput{}, errors.New("일반 파일이 아닙니다 (심볼릭 링크·폴더는 허용하지 않음)")
	}
	if info.Size() == 0 {
		return verifiedOutput{}, errors.New("비어 있습니다")
	}
	if info.Size() > MaxOutputBytes {
		return verifiedOutput{}, fmt.Errorf("크기 제한(%dMB)을 넘었습니다", MaxOutputBytes>>20)
	}
	rel, _ := filepath.Rel(e.cfg.DataDir, path)
	data, err := os.ReadFile(resolved)
	if err != nil {
		return verifiedOutput{}, err
	}
	if err := checkFormat(o, data); err != nil {
		return verifiedOutput{}, err
	}
	sum := sha256.Sum256(data)
	return verifiedOutput{Key: o.Key, Type: string(o.Type), RelPath: filepath.ToSlash(rel), Hash: hex.EncodeToString(sum[:]), Size: info.Size()}, nil
}

func checkFormat(o domain.Output, data []byte) error {
	switch o.Type {
	case domain.OutJSON, domain.OutReport, domain.OutCodeChange:
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
		if err != nil {
			return fmt.Errorf("JSON 형식이 아닙니다: %v", err)
		}
		obj, isObj := doc.(map[string]any)
		if (o.Type == domain.OutReport || o.Type == domain.OutCodeChange) && !isObj {
			return errors.New("JSON 객체여야 합니다")
		}
		if o.Type == domain.OutCodeChange {
			if _, ok := obj["changes"].([]any); !ok {
				return errors.New("code_change 결과에는 changes 배열이 필요합니다")
			}
		}
		if len(o.Schema) > 0 {
			return validateSchema(o.Schema, doc)
		}
	case domain.OutMarkdown:
		if !utf8Text(data) {
			return errors.New("텍스트(UTF-8)가 아닙니다")
		}
	}
	return nil
}

func utf8Text(b []byte) bool { return strings.ToValidUTF8(string(b), "�") == string(b) }

// denyLoader refuses every external $ref: schemas may come from model
// drafts and must not make the app read files or fetch URLs.
type denyLoader struct{}

func (denyLoader) Load(url string) (any, error) {
	return nil, fmt.Errorf("external schema reference %q is not allowed", url)
}

func validateSchema(schema json.RawMessage, doc any) error {
	s, err := jsonschema.UnmarshalJSON(bytes.NewReader(schema))
	if err != nil {
		return fmt.Errorf("schema: %v", err)
	}
	c := jsonschema.NewCompiler()
	c.UseLoader(denyLoader{})
	if err := c.AddResource("output.json", s); err != nil {
		return fmt.Errorf("schema: %v", err)
	}
	sch, err := c.Compile("output.json")
	if err != nil {
		return fmt.Errorf("schema: %v", err)
	}
	if err := sch.Validate(doc); err != nil {
		return fmt.Errorf("schema 불일치: %v", err)
	}
	return nil
}

// verifyEnv is the minimal environment for completion commands: enough
// to run tools, no inherited credentials (spec §10.2).
func verifyEnv() []string {
	keep := []string{"PATH", "HOME", "USER", "LANG", "LC_ALL", "TMPDIR", "TEMP", "TMP", "SystemRoot", "ComSpec", "PATHEXT", "USERPROFILE"}
	var env []string
	for _, k := range keep {
		if v, ok := os.LookupEnv(k); ok {
			env = append(env, k+"="+v)
		}
	}
	return append(env, "CI=1", "AGENT_OFFICE_VERIFY=1")
}

func runVerifyCommand(ctx context.Context, vc domain.VerifyCommand, dir string) error {
	timeout := 10 * time.Minute
	if vc.Timeout != "" {
		if d, err := time.ParseDuration(vc.Timeout); err == nil {
			timeout = d
		}
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, vc.Executable, vc.Args...)
	cmd.Dir = dir
	cmd.Env = verifyEnv()
	cmd.WaitDelay = 5 * time.Second
	var buf tail
	cmd.Stdout, cmd.Stderr = &buf, &buf
	err := cmd.Run()
	label := strings.TrimSpace(vc.Executable + " " + strings.Join(vc.Args, " "))
	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("검증 명령 %q 시간 초과", label)
	}
	if err != nil {
		return fmt.Errorf("검증 명령 %q 실패 (%v): %s", label, err, strings.TrimSpace(buf.String()))
	}
	return nil
}

// tail keeps the last 4KB of command output for error messages.
type tail struct{ b []byte }

func (t *tail) Write(p []byte) (int, error) {
	t.b = append(t.b, p...)
	if len(t.b) > 4096 {
		t.b = t.b[len(t.b)-4096:]
	}
	return len(p), nil
}
func (t *tail) String() string { return string(t.b) }

var _ io.Writer = (*tail)(nil)

// freeze copies verified outputs into the artifact store as read-only
// files and re-checks each copy's hash. Recorded artifacts then never
// change, even if the attempt's working folder is touched later.
func (e *Engine) freeze(projectID, runID, attemptID string, results []verifiedOutput) ([]verifiedOutput, error) {
	dir := filepath.Join(e.projectDir(projectID), "artifacts", runID, attemptID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	out := make([]verifiedOutput, 0, len(results))
	for _, r := range results {
		src, err := e.resolveInside(filepath.Join(e.cfg.DataDir, filepath.FromSlash(r.RelPath)))
		if err != nil {
			return nil, err
		}
		data, err := os.ReadFile(src)
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != r.Hash {
			return nil, fmt.Errorf("결과 %q가 검증 후 바뀌었습니다", r.Key)
		}
		// A unique name per freeze: read-only files are never overwritten
		// (e.g. two submissions racing for the same attempt).
		base := filepath.Base(src)
		ext := filepath.Ext(base)
		dst := filepath.Join(dir, strings.TrimSuffix(base, ext)+"-"+storage.NewID("v")[2:10]+ext)
		if err := os.WriteFile(dst, data, 0o400); err != nil {
			return nil, err
		}
		rel, _ := filepath.Rel(e.cfg.DataDir, dst)
		r.RelPath = filepath.ToSlash(rel)
		out = append(out, r)
	}
	return out, nil
}
