package engine

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"agent-office/internal/domain"
	osproc "agent-office/internal/proc"
	"agent-office/internal/providers"
)

// DiagramSkill is the diagram-design plugin bundled with the app. Steps
// with a diagram output are told to follow it, Claude also loads it as a
// plugin, and every diagram result must pass its scripts/self_check.py.
type DiagramSkill struct {
	// Plugin is the plugin folder (holding .claude-plugin/plugin.json).
	// Empty: the skill is missing and diagrams are checked for format only.
	Plugin string
	// Python runs the self-check. Empty: the self-check is skipped (the
	// result still has to be an HTML file with inline SVG).
	Python string
}

// SkillDir is the skill folder inside the plugin, or "".
func (d DiagramSkill) SkillDir() string {
	if d.Plugin == "" {
		return ""
	}
	return filepath.Join(d.Plugin, "skills", "diagram-design")
}

func (d DiagramSkill) selfCheckScript() string {
	return filepath.Join(d.SkillDir(), "scripts", "self_check.py")
}

func hasDiagram(n *domain.Node) bool {
	for _, o := range n.Outputs {
		if o.Type == domain.OutDiagram {
			return true
		}
	}
	return false
}

// writeDiagramRules tells the AI to make its diagram results with the
// bundled skill and how they will be checked.
func writeDiagramRules(b *strings.Builder, d DiagramSkill) {
	b.WriteString("\n## 다이어그램 작성 규칙 (앱 정책)\n\n")
	if skill := d.SkillDir(); skill != "" {
		fmt.Fprintf(b, "- 다이어그램 결과는 반드시 diagram-design 스킬로 만드세요. 시작하기 전에 %s 를 읽고, 그 안내대로 필요한 references 파일(유형별 type-*.md, style-guide.md 등)을 읽어 따르세요. Claude에서는 diagram-design 스킬로 불러와도 됩니다.\n", filepath.Join(skill, "SKILL.md"))
	} else {
		b.WriteString("- 이 앱에서 diagram-design 스킬을 찾지 못했습니다. 아래 형식만 지키세요.\n")
	}
	b.WriteString("- 결과는 외부 파일 없이 열리는 HTML 한 파일(인라인 SVG)입니다. Mermaid·ASCII 그림·이미지 파일로 대신하지 마세요.\n")
	b.WriteString("- 스킬의 첫 사용 스타일 가이드 확인 단계에서 사람에게 묻지 마세요. 지침에 브랜드 색·글꼴이 있으면 그것을, 없으면 스킬의 기본 스타일을 씁니다.\n")
	b.WriteString("- 스킬 파일을 고치거나 프로필(~/.diagram-design)을 저장하지 마세요. 결과 파일만 지정된 경로에 저장합니다.\n")
	if d.SkillDir() != "" && d.Python != "" {
		fmt.Fprintf(b, "- 제출하면 앱이 `%s %s <결과 파일>`로 검사하고, 실패하면 업무가 완료되지 않습니다.\n", shellQuote(d.Python), shellQuote(d.selfCheckScript()))
		b.WriteString("- 미리 검사하려면 위 명령을 다른 명령과 묶지 말고(cd·&&·;·변수 없이) 결과 파일 경로를 따옴표로 감싸 한 줄로 실행하세요. 그 형태는 승인 없이 실행됩니다.\n")
		b.WriteString("- 스킬 안내에 나오는 저장소용 검사 스크립트(verify-*.py, lint-*.py 등)는 이 앱에 없습니다. 검사는 self_check.py만 쓰세요.\n")
	}
}

// Grant gives an AI request the skill: loaded as a plugin (Claude),
// readable without asking, and its self-check runnable without asking.
func (d DiagramSkill) Grant(req *providers.StartRequest) {
	if d.Plugin == "" {
		return
	}
	req.Plugins = []string{d.Plugin}
	req.ReadableDirs = []string{d.Plugin}
	if d.Python != "" {
		req.CheckCommand = []string{d.Python, d.selfCheckScript()}
	}
}

// Rules is the prompt section that tells the AI to use the skill.
func (d DiagramSkill) Rules() string {
	var b strings.Builder
	writeDiagramRules(&b, d)
	return b.String()
}

// selfCheckTimeout bounds one diagram self-check.
const selfCheckTimeout = 2 * time.Minute

func (e *Engine) checkDiagram(ctx context.Context, path string) error {
	return e.cfg.Diagram.Check(ctx, path)
}

// Check runs the skill's self-check on a diagram file. It returns nil
// without checking when the skill or Python is missing.
func (d DiagramSkill) Check(ctx context.Context, path string) error {
	if d.SkillDir() == "" || d.Python == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, selfCheckTimeout)
	defer cancel()
	// -I: ignore the user's Python environment and site packages. -X utf8:
	// print non-ASCII paths on Windows, where -I also ignores PYTHONUTF8.
	cmd := exec.CommandContext(ctx, d.Python, "-I", "-X", "utf8", d.selfCheckScript(), path)
	cmd.Dir = d.SkillDir()
	cmd.Env = verifyEnv()
	osproc.Hide(cmd)
	cmd.WaitDelay = 5 * time.Second
	var buf tail
	cmd.Stdout, cmd.Stderr = &buf, &buf
	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("다이어그램 검사(self_check.py) 시간 초과")
	}
	if err != nil {
		return fmt.Errorf("다이어그램 검사(self_check.py) 실패: %s", strings.TrimSpace(buf.String()))
	}
	return nil
}

// shellQuote single-quotes p when it holds anything but plain path
// characters, so the command shown to the AI can be run as written.
func shellQuote(p string) string {
	for _, r := range p {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("/._-+=,@:%", r)) {
			return "'" + strings.ReplaceAll(p, "'", `'\''`) + "'"
		}
	}
	return p
}
