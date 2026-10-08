package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"agent-office/internal/domain"
)

// inlineLimit is the largest text input pasted into the prompt; larger
// inputs are referenced by path only.
const inlineLimit = 16 << 10

// instructionLayers is the assignment's frozen layers plus this task and
// any rework feedback, in the spec §2.3 order.
func instructionLayers(snap domain.AssignmentSnapshot, n *domain.Node, feedback []string) []domain.InstructionLayer {
	layers := append([]domain.InstructionLayer(nil), snap.Instructions...)
	if n.Instructions != "" {
		layers = append(layers, domain.InstructionLayer{Source: "task", Name: "업무: " + n.Title, Text: n.Instructions})
	}
	if len(feedback) > 0 {
		layers = append(layers, domain.InstructionLayer{Source: "feedback", Name: "이번 수정 의견", Text: strings.Join(feedback, "\n\n")})
	}
	return layers
}

// feedbackFor returns rework reasons addressed to this step's current
// generation.
func feedbackFor(ctx context.Context, q querier, st *runState, stepID string) ([]string, error) {
	rows, err := q.QueryContext(ctx, `SELECT body, artifact_refs FROM messages WHERE run_id = ? AND kind = 'decision' AND recipient = ? ORDER BY created_at`,
		st.run.ID, "step:"+stepID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var body, refs string
		rows.Scan(&body, &refs)
		var meta struct {
			Generation int `json:"generation"`
		}
		json.Unmarshal([]byte(refs), &meta)
		if meta.Generation == st.run.Gens[stepID].G {
			out = append(out, body)
		}
	}
	return out, rows.Err()
}

func formatHint(t domain.OutputType) string {
	switch t {
	case domain.OutMarkdown:
		return "Markdown 텍스트"
	case domain.OutJSON:
		return "JSON"
	case domain.OutReport:
		return `JSON 객체 (예: {"summary": "...", "findings": [...]})`
	case domain.OutCodeChange:
		return `JSON 객체 {"baseCommit": "...", "changes": [{"path": "...", "status": "added|modified|deleted"}], "tests": {"command": "...", "passed": true}}`
	default:
		return "파일"
	}
}

// buildPrompt describes the task, pinned inputs and exactly where each
// output must be written.
func buildPrompt(dataDir string, st *runState, n *domain.Node, manifest []ManifestEntry, notes []contextNote, attemptDir string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# 업무: %s\n\n", n.Title)
	if g := st.version.Policy.Goal; g != "" {
		fmt.Fprintf(&b, "서비스 목표: %s\n\n", g)
	}
	if len(manifest) > 0 {
		b.WriteString("## 입력 자료\n\n")
		for _, m := range manifest {
			if m.ArtifactID == "" {
				fmt.Fprintf(&b, "- %s: 없음 (선택 입력, 이번 실행에서는 만들어지지 않음)\n", m.Name)
				continue
			}
			path := filepath.Join(dataDir, filepath.FromSlash(m.Path))
			fmt.Fprintf(&b, "- %s (%s.%s, sha256 %s): %s\n", m.Name, m.FromStep, m.OutputKey, short(m.Hash), path)
			if data, err := os.ReadFile(path); err == nil && len(data) <= inlineLimit && utf8.Valid(data) {
				fmt.Fprintf(&b, "\n```\n%s\n```\n\n", strings.TrimRight(string(data), "\n"))
			}
		}
		b.WriteString("\n")
	}
	writeContext(&b, notes)
	b.WriteString("## 결과 제출 방법\n\n")
	for _, o := range n.Outputs {
		req := "선택"
		if o.IsRequired() {
			req = "필수"
		}
		fmt.Fprintf(&b, "- %s (%s, %s): 다음 경로에 파일로 저장 → %s\n", o.Key, formatHint(o.Type), req, outputPath(attemptDir, o))
	}
	b.WriteString("\n## 완료 기준\n\n- 위의 필수 결과 파일이 형식에 맞게 존재해야 합니다.\n")
	if n.Completion != nil {
		for _, c := range n.Completion.Commands {
			fmt.Fprintf(&b, "- 검증 명령이 성공해야 합니다: %s %s\n", c.Executable, strings.Join(c.Args, " "))
		}
	}
	b.WriteString("- \"완료했다\"고 답하는 것만으로는 완료로 인정되지 않습니다. 파일을 저장한 뒤 작업을 마치세요.\n")
	return b.String()
}

func short(h string) string {
	if len(h) > 12 {
		return h[:12]
	}
	return h
}
