// Package toolpolicy decides which tool requests of an AI step the app
// may allow on its own, and says in plain words why it asks the person
// about the others (company requests, 6th revision).
//
// It is deliberately conservative: a command is allowed only when every
// part of it is understood and stays inside the step's folders (its work
// folder, its output and input folders, and the temporary folder). Anything
// it cannot read, any program it does not know, and anything on its list
// of risks goes to the person with the reasons. A service may keep rules
// ("npm install is fine here") for risks a person may waive; risks that
// are never waived (administrator rights, deleting or writing outside the
// folders, key files) always go to the person.
package toolpolicy

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// Request is one tool request of a step.
type Request struct {
	// Tool is the tool name (Bash, Read, WebFetch…) or the Codex action
	// (command, file_change).
	Tool string
	// Command is the shell command for Bash and Codex commands.
	Command string
	// Path is the file a file tool names.
	Path string
	// Workspace is the step's work folder; Writable and Readable are the
	// other folders it was given.
	Workspace string
	Writable  []string
	Readable  []string
}

// Rule lets a kind of command run without asking in one service: the
// program and, for programs with subcommands, the subcommand.
type Rule struct {
	Program string `json:"program"`
	Sub     string `json:"sub"`
}

// Key is how a rule is shown ("npm install").
func (r Rule) Key() string { return strings.TrimSpace(r.Program + " " + r.Sub) }

// Risk is one reason to ask the person.
type Risk struct {
	Reason string `json:"reason"`
	// Rule is the rule that would waive it; nil when it may not be waived.
	Rule *Rule `json:"rule,omitempty"`
}

// Verdict is the decision on a request.
type Verdict struct {
	Allow bool `json:"allow"`
	// Summary says what the request does ("테스트 실행", "파일 읽기").
	Summary string `json:"summary"`
	Risks   []Risk `json:"risks"`
	// Waived names the service rules that allowed it.
	Waived []string `json:"waived,omitempty"`
}

// Rules returns the distinct rules that would waive the verdict's risks;
// ok is false when some risk may not be waived.
func (v Verdict) Rules() (rules []Rule, ok bool) {
	ok = len(v.Risks) > 0
	for _, r := range v.Risks {
		if r.Rule == nil {
			ok = false
			continue
		}
		if !slices.Contains(rules, *r.Rule) {
			rules = append(rules, *r.Rule)
		}
	}
	return rules, ok
}

// Check decides a request.
func Check(req Request, rules []Rule) Verdict {
	c := checker{req: req, rules: rules}
	c.roots = c.writeRoots()
	switch req.Tool {
	case "Bash", "command":
		c.summary = "명령 실행"
		c.command(req.Command)
	case "Read", "Glob", "Grep", "LS", "NotebookRead":
		c.summary = "파일 읽기"
		c.read(req.Path)
	case "Write", "Edit", "MultiEdit", "NotebookEdit":
		c.summary = "파일 쓰기"
		c.write(req.Path)
	case "WebFetch", "WebSearch":
		c.summary = "인터넷 조회"
		c.risk("인터넷에서 정보를 가져옵니다. 외부 글에 섞인 지시가 AI에게 전달될 수 있습니다", &Rule{Program: req.Tool})
	case "file_change":
		c.summary = "파일 변경"
		c.risk("AI가 작업 폴더 밖의 파일을 바꾸려고 할 수 있습니다(바꿀 파일이 요청에 적혀 있지 않습니다)", &Rule{Program: "file_change"})
	default:
		c.summary = req.Tool
		c.risk(fmt.Sprintf("앱이 판단할 수 없는 도구입니다(%s)", req.Tool), &Rule{Program: req.Tool})
	}
	v := Verdict{Summary: c.summary, Risks: c.risks, Waived: c.waived}
	v.Allow = len(c.risks) == 0
	if v.Risks == nil {
		v.Risks = []Risk{}
	}
	return v
}

type checker struct {
	req     Request
	rules   []Rule
	roots   []string
	risks   []Risk
	waived  []string
	summary string
	// kinds of work seen in a command, for the summary
	kinds []string
}

func (c *checker) risk(reason string, rule *Rule) {
	if rule != nil && slices.Contains(c.rules, *rule) {
		if !slices.Contains(c.waived, rule.Key()) {
			c.waived = append(c.waived, rule.Key())
		}
		return
	}
	for _, r := range c.risks {
		if r.Reason == reason {
			return
		}
	}
	c.risks = append(c.risks, Risk{Reason: reason, Rule: rule})
}

func (c *checker) kind(k string) {
	if !slices.Contains(c.kinds, k) {
		c.kinds = append(c.kinds, k)
	}
}

// writeRoots are where a step may write: its folders and the temporary
// folder.
func (c *checker) writeRoots() []string {
	var roots []string
	add := func(p string) {
		if p == "" {
			return
		}
		roots = append(roots, clean(p))
	}
	add(c.req.Workspace)
	for _, p := range c.req.Writable {
		add(p)
	}
	add(os.TempDir())
	add("/tmp")
	add("/private/tmp")
	add("/var/folders")
	add("/private/var/folders")
	return roots
}

// clean resolves symbolic links (/tmp is /private/tmp on macOS), also
// for a path that does not exist yet: its nearest existing folder is
// resolved and the rest kept.
func clean(p string) string {
	p = filepath.Clean(p)
	rest := ""
	for cur := p; ; {
		if r, err := filepath.EvalSymlinks(cur); err == nil {
			return filepath.Join(r, rest)
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return p
		}
		rest = filepath.Join(filepath.Base(cur), rest)
		cur = parent
	}
}

func inside(root, p string) bool {
	rel, err := filepath.Rel(root, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func (c *checker) insideRoots(p string) bool {
	p = clean(p)
	for _, r := range c.roots {
		if inside(r, p) {
			return true
		}
	}
	return false
}

func home() string {
	h, _ := os.UserHomeDir()
	return h
}

// secretPattern matches files that hold keys, passwords and tokens.
var secretPattern = regexp.MustCompile(`(^|/)(\.ssh|\.aws|\.gnupg|\.kube|\.docker/config\.json|\.netrc|\.npmrc|\.pypirc|\.git-credentials|\.config/gh|Keychains|id_rsa|id_ed25519|id_ecdsa|\.pem$|\.p12$|credentials(\.json)?$|\.env(\.[a-z]+)?$)`)

// read checks a file a tool reads: anything but key and password files.
func (c *checker) read(p string) {
	if p == "" {
		return
	}
	abs := c.abs(p, c.req.Workspace)
	if secretPattern.MatchString(filepath.ToSlash(abs)) && !c.insideRoots(abs) {
		c.risk(fmt.Sprintf("열쇠·비밀번호가 든 파일을 읽습니다(%s)", c.show(abs)), nil)
	}
}

// write checks a file a tool writes or deletes.
func (c *checker) write(p string) {
	if p == "" {
		return
	}
	if strings.ContainsAny(p, "$`*?") {
		c.risk(fmt.Sprintf("변수나 패턴으로 된 경로(%s)라 어디에 쓰는지 확인할 수 없습니다", p), &Rule{Program: "unresolved-path"})
		return
	}
	abs := c.abs(p, c.req.Workspace)
	if dev := filepath.ToSlash(abs); dev == "/dev/null" || strings.HasPrefix(dev, "/dev/std") || strings.HasPrefix(dev, "/dev/fd/") {
		return
	}
	if !c.insideRoots(abs) {
		c.risk(fmt.Sprintf("작업 폴더 밖(%s)에 쓰거나 지웁니다", c.show(abs)), nil)
	}
}

func (c *checker) abs(p, dir string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		p = filepath.Join(home(), strings.TrimPrefix(p, "~"))
	}
	if !isAbs(p) {
		if dir == "" {
			dir = c.req.Workspace
		}
		p = filepath.Join(dir, p)
	}
	return filepath.Clean(p)
}

// isAbs also takes /path as absolute on Windows: commands run in a POSIX
// shell there too, and such a path is never inside the step's folders.
func isAbs(p string) bool { return filepath.IsAbs(p) || strings.HasPrefix(p, "/") }

// show writes a path for the person, with the home folder as ~.
func (c *checker) show(p string) string {
	if h := home(); h != "" && inside(h, p) {
		return "~" + strings.TrimPrefix(p, h)
	}
	return p
}
