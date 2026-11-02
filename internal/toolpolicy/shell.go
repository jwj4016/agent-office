package toolpolicy

import (
	"bytes"
	"cmp"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// segment is one simple command of a shell line.
type segment struct {
	words  []string
	writes []string // redirection targets
	pipeIn bool     // reads the previous command's output
}

// parseShell splits a command line into simple commands. Heredoc bodies
// and $(…) contents come back separately; ok is false when the quoting
// does not close.
func parseShell(cmd string) (segs []segment, inner []string, heredocs []string, ok bool) {
	cmd, heredocs = stripHeredocs(cmd)
	var cur segment
	var word strings.Builder
	inWord := false
	flushWord := func() {
		if inWord {
			cur.words = append(cur.words, word.String())
			word.Reset()
			inWord = false
		}
	}
	flushSeg := func(pipe bool) {
		flushWord()
		// Redirections: ">x", ">", "x" become writes.
		var words []string
		for i := 0; i < len(cur.words); i++ {
			w := cur.words[i]
			target, isRedirect := redirectTarget(w)
			switch {
			case !isRedirect:
				words = append(words, w)
			case target != "":
				cur.writes = append(cur.writes, target)
			case i+1 < len(cur.words):
				if !strings.HasPrefix(w, "<") {
					cur.writes = append(cur.writes, cur.words[i+1])
				}
				i++
			}
		}
		cur.words = words
		if len(cur.words) > 0 || len(cur.writes) > 0 {
			segs = append(segs, cur)
		}
		cur = segment{pipeIn: pipe}
	}
	rs := []rune(cmd)
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		switch {
		case r == '\\' && i+1 < len(rs):
			i++
			if rs[i] != '\n' {
				word.WriteRune(rs[i])
				inWord = true
			}
		case r == '\'':
			j := i + 1
			for j < len(rs) && rs[j] != '\'' {
				j++
			}
			if j >= len(rs) {
				return nil, nil, nil, false
			}
			word.WriteString(string(rs[i+1 : j]))
			inWord = true
			i = j
		case r == '"':
			j := i + 1
			for j < len(rs) && rs[j] != '"' {
				if rs[j] == '\\' {
					j++
				} else if rs[j] == '$' && j+1 < len(rs) && rs[j+1] == '(' {
					end := closeParen(rs, j+1)
					if end < 0 {
						return nil, nil, nil, false
					}
					inner = append(inner, string(rs[j+2:end]))
					j = end
				}
				j++
			}
			if j >= len(rs) {
				return nil, nil, nil, false
			}
			word.WriteString(strings.ReplaceAll(string(rs[i+1:j]), `\"`, `"`))
			inWord = true
			i = j
		case r == '$' && i+1 < len(rs) && rs[i+1] == '(':
			end := closeParen(rs, i+1)
			if end < 0 {
				return nil, nil, nil, false
			}
			inner = append(inner, string(rs[i+2:end]))
			word.WriteString("$(…)")
			inWord = true
			i = end
		case r == '`':
			j := i + 1
			for j < len(rs) && rs[j] != '`' {
				j++
			}
			if j >= len(rs) {
				return nil, nil, nil, false
			}
			inner = append(inner, string(rs[i+1:j]))
			word.WriteString("$(…)")
			inWord = true
			i = j
		case r == ';' || r == '\n':
			flushSeg(false)
		case r == '&' && i+1 < len(rs) && rs[i+1] == '&':
			flushSeg(false)
			i++
		case r == '|' && i+1 < len(rs) && rs[i+1] == '|':
			flushSeg(false)
			i++
		case r == '|':
			flushSeg(true)
		case r == '&' && (i == 0 || rs[i-1] != '>') && (i+1 >= len(rs) || rs[i+1] != '>'):
			flushSeg(false) // a background command
		case r == '(' || r == ')' || r == '{' || r == '}':
			flushWord()
		case r == ' ' || r == '\t':
			flushWord()
		default:
			word.WriteRune(r)
			inWord = true
		}
	}
	flushSeg(false)
	return segs, inner, heredocs, true
}

func closeParen(rs []rune, open int) int {
	depth := 0
	for k := open; k < len(rs); k++ {
		switch rs[k] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return k
			}
		}
	}
	return -1
}

var redirectPattern = regexp.MustCompile(`^(\d*|&)(>>?|<)(&\d+|&-)?(.*)$`)

// redirectTarget reports whether w is a redirection, with its target when
// it is written in the same word ("2>/dev/null"). Duplications (2>&1) have
// no target.
func redirectTarget(w string) (string, bool) {
	m := redirectPattern.FindStringSubmatch(w)
	if m == nil {
		return "", false
	}
	if m[3] != "" {
		return "/dev/null", true
	}
	if m[2] == "<" {
		return "/dev/null", true // reading: not a write
	}
	return m[4], true
}

var heredocPattern = regexp.MustCompile(`<<-?\s*['"]?([A-Za-z_][A-Za-z0-9_]*)['"]?`)

// stripHeredocs removes heredoc bodies from a command, returning them.
func stripHeredocs(cmd string) (string, []string) {
	var bodies []string
	for {
		loc := heredocPattern.FindStringSubmatchIndex(cmd)
		if loc == nil {
			return cmd, bodies
		}
		delim := cmd[loc[2]:loc[3]]
		nl := strings.IndexByte(cmd[loc[1]:], '\n')
		if nl < 0 {
			return cmd[:loc[0]] + cmd[loc[1]:], bodies
		}
		bodyStart := loc[1] + nl + 1
		lines := strings.SplitAfter(cmd[bodyStart:], "\n")
		end, bodyEnd, consumed := len(cmd), len(cmd), 0
		for _, l := range lines {
			consumed += len(l)
			if strings.TrimSpace(l) == delim {
				end, bodyEnd = bodyStart+consumed, bodyStart+consumed-len(l)
				break
			}
		}
		body := cmd[bodyStart:bodyEnd]
		bodies = append(bodies, body)
		cmd = cmd[:loc[0]] + cmd[loc[1]:bodyStart] + cmd[min(end, len(cmd)):]
	}
}

// command checks a shell command line.
func (c *checker) command(cmd string) {
	if strings.TrimSpace(cmd) == "" {
		c.risk("빈 명령입니다", nil)
		return
	}
	segs, inner, heredocs, ok := parseShell(cmd)
	if !ok {
		c.risk("명령의 따옴표가 닫히지 않아 내용을 읽을 수 없습니다", &Rule{Program: "unreadable"})
		return
	}
	// Scripts this command writes are checked from what it writes, not
	// from what is on disk now.
	prevWritten, prevHeredocs := c.written, c.heredocs
	c.written, c.heredocs = map[string]bool{}, heredocs
	defer func() { c.written, c.heredocs = prevWritten, prevHeredocs }()
	env := map[string]string{"HOME": home()}
	dir := c.req.Workspace
	prevFetchesWeb := false
	for _, s := range segs {
		words := expandAll(s.words, env)
		// Plain assignments set variables for later commands.
		if len(words) > 0 && allAssignments(words) {
			for _, w := range words {
				k, v, _ := strings.Cut(w, "=")
				env[k] = v
			}
			continue
		}
		for _, w := range expandAll(s.writes, env) {
			c.write(c.pathIn(w, dir))
			c.wrote(c.pathIn(w, dir))
		}
		words = stripAssignments(words)
		if len(words) == 0 {
			continue
		}
		if s.pipeIn && prevFetchesWeb && runsStdin(words) {
			c.risk("인터넷에서 내려받은 내용을 바로 실행합니다", nil)
		}
		dir = c.simple(words, dir, env)
		prevFetchesWeb = (base(words[0]) == "curl" || base(words[0]) == "wget") && !fetchesLocal(words[1:])
	}
	for _, in := range inner {
		c.command(in)
	}
	for _, h := range heredocs {
		c.script(h, "python", true)
	}
	if len(c.kinds) > 0 {
		c.summary = strings.Join(c.kinds, ", ")
	}
}

var shells = []string{"sh", "bash", "zsh", "python", "python3", "node", "perl", "ruby"}

var (
	prefixKeywords = []string{"do", "then", "else", "elif", "if", "while", "until", "!", "time"}
	headerKeywords = []string{"for", "case", "select", "function"}
	endKeywords    = []string{"done", "fi", "esac", ";;"}
)

// runsStdin reports whether a command runs the program it reads (a shell
// or interpreter given no script, or "-").
func runsStdin(words []string) bool {
	if !slices.Contains(shells, base(words[0])) {
		return false
	}
	for _, a := range words[1:] {
		if a == "-" || a == "-s" {
			return true
		}
		if !strings.HasPrefix(a, "-") || a == "-m" || a == "-c" {
			return false
		}
	}
	return true
}

// localOnly reports whether nc names only this computer ("nc -z
// 127.0.0.1 5173" checks a port) and does not listen.
func localOnly(args []string) bool {
	host := false
	for _, a := range args {
		switch {
		case strings.HasPrefix(a, "-"):
			if strings.Contains(a, "l") || strings.Contains(a, "e") || strings.Contains(a, "c") {
				return false
			}
		case slices.Contains(localHosts, a):
			host = true
		case !isNumberish(a):
			return false
		}
	}
	return host
}

// fetchesLocal reports whether every address a curl or wget names is on
// this computer.
func fetchesLocal(args []string) bool {
	for _, a := range args {
		if strings.HasPrefix(a, "-") || !strings.Contains(a, "://") && !strings.Contains(a, ".") {
			continue
		}
		u, err := url.Parse(a)
		if err != nil || u.Hostname() == "" {
			continue
		}
		if !slices.Contains(localHosts, u.Hostname()) {
			return false
		}
	}
	return true
}

func base(p string) string {
	b := filepath.Base(p)
	if strings.HasPrefix(b, "python3.") {
		return "python3"
	}
	return b
}

func allAssignments(words []string) bool {
	for _, w := range words {
		if !isAssignment(w) {
			return false
		}
	}
	return true
}

var assignPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)

func isAssignment(w string) bool { return assignPattern.MatchString(w) }

func stripAssignments(words []string) []string {
	for len(words) > 0 && isAssignment(words[0]) {
		words = words[1:]
	}
	return words
}

var varPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}|\$([A-Za-z_][A-Za-z0-9_]*)`)

func expandAll(words []string, env map[string]string) []string {
	out := make([]string, 0, len(words))
	for _, w := range words {
		out = append(out, varPattern.ReplaceAllStringFunc(w, func(m string) string {
			name := strings.Trim(m, "${}")
			if v, ok := env[name]; ok {
				return v
			}
			return m
		}))
	}
	return out
}

// pathIn resolves p against dir for checks that take an absolute path.
func (c *checker) pathIn(p, dir string) string {
	if p == "" || strings.ContainsAny(p, "$`*?") || isAbs(p) || p == "~" || strings.HasPrefix(p, "~/") {
		return p
	}
	return filepath.Join(dir, p)
}

func looksLikePath(w string) bool {
	return !strings.HasPrefix(w, "-") && (strings.ContainsRune(w, '/') || strings.HasPrefix(w, "~") || strings.HasPrefix(w, "."))
}

// readArgs checks the files a reading program names.
func (c *checker) readArgs(args []string, dir string) {
	for _, a := range args {
		if looksLikePath(a) && !strings.ContainsAny(a, "$`*?") && !strings.Contains(a, "://") {
			c.read(c.abs(a, dir))
		}
	}
}

func (c *checker) writeArgs(args []string, dir string) {
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			c.write(c.pathIn(a, dir))
		}
	}
}

// Programs grouped by what they do.
var (
	readers = []string{"ls", "cat", "head", "tail", "wc", "grep", "egrep", "fgrep", "rg", "ag", "awk", "sort", "uniq", "cut", "tr", "diff",
		"cmp", "file", "stat", "du", "df", "tree", "less", "more", "xxd", "od", "hexdump", "column", "nl", "fold", "paste", "comm",
		"md5", "md5sum", "shasum", "sha256sum", "jq", "yq", "realpath", "readlink", "basename", "dirname", "strings", "iconv"}
	harmless = []string{"echo", "printf", "true", "false", "test", "[", "pwd", "which", "type", "command", "date", "sleep", "seq", "uname",
		"whoami", "hostname", "sw_vers", "id", "ps", "pgrep", "lsof", "export", "set", "unset", "wait", "jobs", "exit", "return", "local",
		"read", "shift", "trap", "ulimit", "umask", "env", "printenv", "nproc", "sysctl", "arch", "locale", "tput", "clear", "history", "hash"}
	wrappers = []string{"timeout", "gtimeout", "nohup", "time", "nice", "caffeinate", "stdbuf", "exec"}
	servers  = []string{"uvicorn", "gunicorn", "flask", "vite", "next", "serve", "http-server", "streamlit", "hypercorn", "daphne"}
	testers  = []string{"pytest", "vitest", "jest", "mocha", "tsc", "eslint", "prettier", "ruff", "mypy", "black", "flake8", "pylint",
		"isort", "playwright", "go", "cargo", "make", "cmake", "gofmt", "rustfmt", "swift", "swiftc", "javac", "java", "mvn", "gradle", "deno", "bun"}
	safeNpx = []string{"vitest", "tsc", "vite", "eslint", "prettier", "playwright", "jest", "ts-node", "tsx", "next", "serve", "http-server"}
	system  = map[string]string{
		"dd": "디스크를 직접 씁니다", "mkfs": "디스크를 포맷합니다", "diskutil": "디스크를 바꿉니다", "shutdown": "컴퓨터를 끕니다",
		"reboot": "컴퓨터를 다시 켭니다", "halt": "컴퓨터를 멈춥니다", "launchctl": "컴퓨터의 백그라운드 서비스를 바꿉니다",
		"crontab": "정해진 시각에 도는 작업을 바꿉니다", "defaults": "컴퓨터·앱 설정을 바꿀 수 있습니다", "systemctl": "컴퓨터의 서비스를 바꿉니다",
		"security": "키체인(비밀번호 보관함)에 접근합니다", "csrutil": "컴퓨터 보안 설정을 바꿉니다", "spctl": "컴퓨터 보안 설정을 바꿉니다",
		"nvram":     "컴퓨터 설정을 바꿉니다",
		"osascript": appControl, "automator": appControl, "shortcuts": appControl, "cliclick": appControl, "chflags": "파일 보호 설정을 바꿉니다", "passwd": "비밀번호를 바꿉니다", "dscl": "사용자 계정을 바꿉니다",
	}
)

// simple checks one simple command and returns the folder later
// commands run in.
func (c *checker) simple(words []string, dir string, env map[string]string) string {
	for len(words) > 0 && slices.Contains(wrappers, base(words[0])) {
		w := base(words[0])
		words = words[1:]
		if w == "timeout" || w == "gtimeout" || w == "nice" {
			for len(words) > 0 && (strings.HasPrefix(words[0], "-") || isNumberish(words[0])) {
				words = words[1:]
			}
		}
		for len(words) > 0 && strings.HasPrefix(words[0], "-") {
			words = words[1:]
		}
	}
	if len(words) == 0 {
		return dir
	}
	if base(words[0]) == "env" {
		rest := words[1:]
		for len(rest) > 0 && (isAssignment(rest[0]) || strings.HasPrefix(rest[0], "-")) {
			rest = rest[1:]
		}
		if len(rest) == 0 {
			return dir
		}
		words = rest
	}
	// Shell keywords: a loop or case header has nothing to run; "do",
	// "then", "if"… prefix the command that follows.
	for len(words) > 0 && slices.Contains(prefixKeywords, words[0]) {
		words = words[1:]
	}
	if len(words) == 0 || slices.Contains(headerKeywords, words[0]) || slices.Contains(endKeywords, words[0]) {
		return dir
	}
	prog, args := base(words[0]), words[1:]
	if strings.Contains(words[0], "$(") || (strings.Contains(words[0], "$") && !strings.Contains(words[0], "/")) {
		c.risk(fmt.Sprintf("실행할 프로그램(%s)을 확인할 수 없습니다", words[0]), &Rule{Program: "unresolved-program"})
		return dir
	}
	switch {
	case prog == "cd" || prog == "pushd":
		if len(args) == 0 || args[0] == "-" {
			return dir
		}
		return c.abs(args[0], dir)
	case prog == "sudo" || prog == "su" || prog == "doas":
		c.risk("관리자 권한으로 실행합니다. 컴퓨터 전체를 바꿀 수 있습니다", nil)
	case system[prog] != "":
		c.risk(system[prog], nil)
	case slices.Contains(harmless, prog):
	case slices.Contains(readers, prog):
		c.kind("파일 보기·검색")
		c.readArgs(args, dir)
	case prog == "sed":
		c.kind("파일 보기·검색")
		c.sed(args, dir)
	case prog == "find":
		c.kind("파일 보기·검색")
		c.find(args, dir, env)
	case prog == "xargs":
		c.xargs(args)
	case prog == "tee" || prog == "mkdir" || prog == "touch" || prog == "truncate" || prog == "ln" || prog == "chmod" || prog == "chown":
		c.kind("파일 정리")
		if prog == "chmod" || prog == "chown" {
			args = dropMode(args)
		}
		c.writeArgs(args, dir)
		if prog == "tee" {
			for _, a := range args {
				if !strings.HasPrefix(a, "-") {
					c.wrote(c.pathIn(a, dir))
				}
			}
		}
	case prog == "rm" || prog == "rmdir" || prog == "unlink" || prog == "shred" || prog == "trash":
		c.kind("파일 지우기")
		c.remove(args, dir)
	case prog == "cp" || prog == "mv" || prog == "install" || prog == "rsync" || prog == "ditto":
		c.kind("파일 정리")
		c.copy(prog, args, dir)
	case prog == "tar" || prog == "unzip" || prog == "zip" || prog == "gzip" || prog == "gunzip":
		c.kind("파일 정리")
		c.archive(prog, args, dir)
	case prog == "curl" || prog == "wget":
		c.fetch(prog, args, dir)
	case prog == "git":
		c.git(args, dir)
	case prog == "npm" || prog == "pnpm" || prog == "yarn":
		c.npm(prog, args)
	case prog == "npx":
		c.npx(args)
	case prog == "pip" || prog == "pip3" || prog == "uv" || prog == "poetry" || prog == "pipx" || prog == "conda":
		c.pip(prog, args)
	case prog == "python" || prog == "python3" || prog == "node" || prog == "ruby" || prog == "perl":
		c.interpreter(prog, args, dir)
	case slices.Contains(shells, prog) || prog == "source" || prog == ".":
		c.shellRun(prog, args, dir)
	case prog == "sqlite3":
		c.sqlite(args, dir)
	case slices.Contains(servers, prog):
		c.kind("로컬 서버 실행")
	case slices.Contains(testers, prog):
		c.tool(prog, args)
	case prog == "pyenv" || prog == "nvm" || prog == "rbenv" || prog == "asdf" || prog == "volta":
		if sub := firstWord(args); slices.Contains([]string{"install", "uninstall", "global"}, sub) {
			c.risk("프로그래밍 언어 버전을 컴퓨터에 설치하거나 기본값을 바꿉니다", &Rule{Program: prog, Sub: sub})
		} else {
			c.kind("스크립트 실행")
		}
	case prog == "brew" || prog == "port" || prog == "apt" || prog == "apt-get":
		c.risk("컴퓨터 전체에 프로그램을 설치하거나 바꿉니다", &Rule{Program: prog, Sub: firstWord(args)})
	case prog == "docker" || prog == "podman" || prog == "docker-compose":
		c.risk("도커 컨테이너·이미지를 다룹니다(컴퓨터 자원과 네트워크를 씁니다)", &Rule{Program: prog, Sub: firstWord(args)})
	case prog == "kill" || prog == "pkill" || prog == "killall":
		c.risk("실행 중인 프로그램을 끕니다. 사장님이 쓰는 프로그램일 수도 있습니다", &Rule{Program: prog})
	case prog == "open" || prog == "xdg-open":
		c.risk("다른 앱을 열거나 조작합니다", &Rule{Program: prog})
	case (prog == "nc" || prog == "ncat") && localOnly(args):
		c.kind("로컬 주소 확인")
	case prog == "ssh" || prog == "scp" || prog == "sftp" || prog == "nc" || prog == "ncat" || prog == "telnet" || prog == "ftp":
		c.risk("다른 컴퓨터에 접속합니다", &Rule{Program: prog})
	default:
		// A program inside the step's folders is the step's own code;
		// what a script in it runs is checked too.
		if strings.Contains(words[0], "/") && c.mayRun(c.abs(words[0], dir)) {
			c.runFile("", c.abs(words[0], dir))
			return dir
		}
		c.risk(fmt.Sprintf("앱이 모르는 프로그램입니다(%s)", prog), &Rule{Program: prog})
	}
	return dir
}

// sed reads, or with -i rewrites, the files after its script.
func (c *checker) sed(args []string, dir string) {
	inPlace, script := false, false
	var files []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case strings.HasPrefix(a, "-i") || a == "--in-place":
			inPlace = true
			// BSD sed: "-i ''" or "-i .bak" names the backup suffix.
			if a == "-i" && i+1 < len(args) && (args[i+1] == "" || strings.HasPrefix(args[i+1], ".")) {
				i++
			}
		case a == "-e" || a == "-f":
			script = true
			i++
		case strings.HasPrefix(a, "-"):
		case !script:
			script = true // the first word is the script
		default:
			files = append(files, a)
		}
	}
	if inPlace {
		c.writeArgs(files, dir)
	} else {
		c.readArgs(files, dir)
	}
}

func isNumberish(s string) bool {
	return regexp.MustCompile(`^\d+[smhd]?$`).MatchString(s)
}

func firstWord(args []string) string {
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			return a
		}
	}
	return ""
}

func dropMode(args []string) []string {
	var out []string
	skipped := false
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			continue
		}
		if !skipped {
			skipped = true // the mode or owner
			continue
		}
		out = append(out, a)
	}
	return out
}

// remove checks a deletion: everything named must be inside the folders,
// and never a whole folder the step was given.
func (c *checker) remove(args []string, dir string) {
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			continue
		}
		p := c.pathIn(a, dir)
		c.write(p)
		if !strings.ContainsAny(p, "$`*?") {
			abs := clean(c.abs(p, dir))
			for _, r := range append([]string{home(), "/"}, c.roots...) {
				if abs == clean(r) {
					c.risk(fmt.Sprintf("폴더 전체(%s)를 지웁니다", c.show(abs)), nil)
				}
			}
		}
	}
}

func (c *checker) copy(prog string, args []string, dir string) {
	var paths []string
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			paths = append(paths, a)
		}
	}
	if len(paths) == 0 {
		return
	}
	if prog == "rsync" && slices.ContainsFunc(paths, func(p string) bool { return strings.Contains(p, ":") && !strings.HasPrefix(p, "/") }) {
		c.risk("다른 컴퓨터와 파일을 주고받습니다", &Rule{Program: "rsync"})
		return
	}
	dest := paths[len(paths)-1]
	c.write(c.pathIn(dest, dir))
	c.wrote(c.pathIn(dest, dir))
	for _, src := range paths[:len(paths)-1] {
		if prog == "mv" {
			c.write(c.pathIn(src, dir)) // moving removes the source
		} else {
			c.readArgs([]string{src}, dir)
		}
	}
}

func (c *checker) archive(prog string, args []string, dir string) {
	switch prog {
	case "unzip":
		for i, a := range args {
			if a == "-d" && i+1 < len(args) {
				c.write(c.pathIn(args[i+1], dir))
				return
			}
		}
		c.write(dir)
	case "tar":
		extract := slices.ContainsFunc(args, func(a string) bool {
			return strings.HasPrefix(a, "-") && strings.Contains(a, "x") || strings.HasPrefix(a, "x")
		})
		for i, a := range args {
			if (a == "-C" || a == "--directory") && i+1 < len(args) {
				c.write(c.pathIn(args[i+1], dir))
				return
			}
			if (a == "-f" || strings.HasPrefix(a, "-") && strings.HasSuffix(a, "f")) && i+1 < len(args) && !extract {
				c.write(c.pathIn(args[i+1], dir))
				return
			}
		}
		if extract {
			c.write(dir)
		}
	default:
		c.writeArgs(args, dir)
	}
}

func (c *checker) find(args []string, dir string, env map[string]string) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "-delete":
			for _, p := range args[:i] {
				if looksLikePath(p) || p == "." {
					c.write(c.pathIn(p, dir))
				}
			}
			c.kind("파일 지우기")
		case "-exec", "-execdir", "-ok":
			j := i + 1
			for j < len(args) && args[j] != ";" && args[j] != "+" && args[j] != `\;` {
				j++
			}
			if i+1 < j {
				sub := slices.Clone(args[i+1 : j])
				for k := range sub {
					if sub[k] == "{}" {
						sub[k] = dir
					}
				}
				c.simple(sub, dir, env)
			}
			i = j
		}
	}
	for _, a := range args {
		if looksLikePath(a) {
			c.read(c.abs(a, dir))
		}
	}
}

func (c *checker) xargs(args []string) {
	i := 0
	for i < len(args) && strings.HasPrefix(args[i], "-") {
		if slices.Contains([]string{"-I", "-n", "-P", "-L", "-d", "-s", "-E"}, args[i]) {
			i++
		}
		i++
	}
	if i >= len(args) {
		return
	}
	prog := base(args[i])
	if slices.Contains(readers, prog) || slices.Contains(harmless, prog) || prog == "sed" && !slices.Contains(args, "-i") {
		c.kind("파일 보기·검색")
		return
	}
	c.risk(fmt.Sprintf("앞 명령의 결과로 받은 대상에 %s를 실행합니다(대상을 미리 확인할 수 없음)", prog), &Rule{Program: "xargs", Sub: prog})
}

var localHosts = []string{"localhost", "127.0.0.1", "0.0.0.0", "::1", "[::1]"}

var (
	bareHost = regexp.MustCompile(`^[A-Za-z0-9.-]+\.[A-Za-z]{2,}(:\d+)?(/|$)`)
	// fileExt marks a word that names a file, not an address.
	fileExt = regexp.MustCompile(`(?i)\.(json|xml|txt|html?|log|md|csv|ya?ml|sh|py|js|ts|body|out|tmp|gz|zip|tar|png|jpe?g|pdf)$`)
)

// takesValue reports whether a curl or wget option takes the next word as
// its value (a file, a header, a time…), which is then not an address.
func takesValue(prog, opt string) bool {
	if strings.Contains(opt, "=") {
		return false
	}
	if prog == "wget" {
		return slices.Contains([]string{"-O", "-o", "-a", "-P", "-T", "-t", "-U", "-e", "--header", "--output-document",
			"--output-file", "--directory-prefix", "--timeout", "--tries", "--user-agent", "--post-data", "--post-file"}, opt)
	}
	if strings.HasPrefix(opt, "--") {
		return slices.Contains([]string{"--output", "--header", "--data", "--data-raw", "--data-binary", "--data-urlencode",
			"--form", "--user", "--user-agent", "--referer", "--cookie", "--cookie-jar", "--upload-file", "--config", "--proxy",
			"--range", "--retry", "--write-out", "--max-time", "--connect-timeout", "--trace", "--trace-ascii", "--dump-header",
			"--stderr", "--cacert", "--cert", "--key", "--request", "--resolve", "--output-dir"}, opt)
	}
	// Short options may be combined ("-sSo file"): the last letter decides.
	return len(opt) >= 2 && opt[0] == '-' && strings.ContainsRune("oHdXmwuAebcTFKxrCEyYzD", rune(opt[len(opt)-1]))
}

func (c *checker) fetch(prog string, args []string, dir string) {
	sends := false
	for i, a := range args {
		switch {
		case a == "-o" || a == "--output" || a == "-O" && prog == "wget" || prog == "curl" && i+1 < len(args) && writesFile(a):
			if i+1 < len(args) {
				c.write(c.pathIn(args[i+1], dir))
			}
		case a == "-d" || strings.HasPrefix(a, "--data") || a == "-F" || a == "--form" || a == "-T" || a == "--upload-file" || a == "--post-data" || a == "--post-file":
			sends = true
		case a == "-X" && i+1 < len(args) && slices.Contains([]string{"POST", "PUT", "PATCH", "DELETE"}, strings.ToUpper(args[i+1])):
			sends = true
		}
	}
	local := true
	for i, a := range args {
		if strings.HasPrefix(a, "-") || i > 0 && takesValue(prog, args[i-1]) {
			continue
		}
		host := ""
		if strings.Contains(a, "://") {
			if u, err := url.Parse(a); err == nil {
				host = u.Hostname()
			}
		} else if bareHost.MatchString(a) && !fileExt.MatchString(a) {
			host = strings.Split(strings.Split(a, "/")[0], ":")[0]
		} else if strings.HasPrefix(a, "localhost") || strings.HasPrefix(a, "127.0.0.1") {
			host = "localhost"
		}
		if host == "" {
			continue
		}
		if slices.Contains(localHosts, host) {
			continue
		}
		local = false
		reason := fmt.Sprintf("인터넷 주소(%s)에 접속합니다", host)
		if sends {
			reason = fmt.Sprintf("인터넷 주소(%s)로 데이터를 보냅니다", host)
		}
		c.risk(reason, &Rule{Program: prog, Sub: host})
	}
	if local {
		c.kind("로컬 주소 확인")
	}
}

// writesFile reports whether a curl option writes its value as a file.
func writesFile(opt string) bool {
	switch opt {
	case "--dump-header", "--trace", "--trace-ascii", "--stderr", "--cookie-jar", "--output":
		return true
	}
	return !strings.HasPrefix(opt, "--") && strings.HasPrefix(opt, "-") && strings.ContainsRune("oDc", rune(opt[len(opt)-1]))
}

func (c *checker) git(args []string, dir string) {
	sub := firstWord(args)
	// "git -C dir sub"
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "-C" {
			dir = c.abs(args[i+1], dir)
			rest := args[i+2:]
			sub = firstWord(rest)
			args = rest
			break
		}
	}
	rule := &Rule{Program: "git", Sub: sub}
	switch sub {
	case "status", "log", "diff", "show", "rev-parse", "ls-files", "blame", "grep", "describe", "shortlog", "rev-list", "cat-file", "ls-tree", "reflog", "version", "help", "archive", "worktree":
		c.kind("Git 기록 보기")
	case "branch":
		if slices.ContainsFunc(args[1:], func(a string) bool { return a == "-D" || a == "-d" || a == "-m" || a == "-M" || a == "--delete" }) {
			c.risk("Git 브랜치를 지우거나 이름을 바꿉니다", rule)
		} else {
			c.kind("Git 기록 보기")
		}
	case "remote":
		if len(args) > 1 && !slices.Contains([]string{"-v", "show", "get-url"}, args[1]) {
			c.risk("Git 원격 저장소 설정을 바꿉니다", rule)
		} else {
			c.kind("Git 기록 보기")
		}
	case "config":
		if slices.ContainsFunc(args[1:], func(a string) bool { return a == "--get" || a == "--list" || a == "-l" }) {
			c.kind("Git 기록 보기")
		} else {
			c.risk("Git 설정을 바꿉니다", rule)
		}
	case "push":
		c.risk("원격 저장소로 올립니다. 다른 사람에게 공개될 수 있습니다", rule)
	case "reset", "clean", "restore", "checkout", "switch", "stash":
		c.risk("작업 중인 파일이나 Git 기록을 되돌립니다. 저장하지 않은 변경이 사라질 수 있습니다", rule)
	case "clone", "fetch", "pull", "submodule":
		c.risk("다른 저장소에서 내려받습니다", rule)
	default:
		c.risk("Git 기록을 바꿉니다. 앱이 업무 결과를 직접 커밋하므로 기록이 섞일 수 있습니다", rule)
	}
}

func (c *checker) npm(prog string, args []string) {
	sub := firstWord(args)
	switch sub {
	case "install", "i", "ci", "add", "update", "upgrade", "uninstall", "remove", "rm", "dedupe", "rebuild":
		if slices.Contains(args, "-g") || slices.Contains(args, "--global") {
			c.risk("패키지를 컴퓨터 전체에 설치합니다", &Rule{Program: prog, Sub: "install -g"})
			return
		}
		c.risk("패키지를 인터넷에서 내려받아 설치합니다", &Rule{Program: prog, Sub: "install"})
	case "publish", "unpublish", "deprecate", "login", "adduser", "token", "owner":
		c.risk("패키지를 공개 저장소에 올리거나 계정을 다룹니다", nil)
	case "test", "t":
		c.kind("테스트 실행")
	case "run", "run-script", "start", "exec", "ls", "list", "view", "outdated", "audit", "pack", "version", "--version", "-v", "config", "help", "why", "explain", "":
		if len(args) > 1 && (strings.Contains(args[1], "test") || strings.Contains(args[1], "check") || strings.Contains(args[1], "lint")) {
			c.kind("테스트 실행")
		} else if len(args) > 1 && strings.Contains(args[1], "build") {
			c.kind("빌드")
		} else {
			c.kind("스크립트 실행")
		}
	default:
		// "yarn build", "pnpm dev": a package script.
		c.kind("스크립트 실행")
	}
}

func (c *checker) npx(args []string) {
	pkg := ""
	for _, a := range args {
		if a == "--yes" || a == "-y" || strings.HasPrefix(a, "-") {
			continue
		}
		pkg = a
		break
	}
	name := strings.Split(strings.TrimPrefix(pkg, "@"), "@")[0]
	if strings.HasPrefix(pkg, "@") {
		name = "@" + name
	}
	if slices.Contains(safeNpx, name) {
		c.kind("테스트 실행")
		return
	}
	c.risk(fmt.Sprintf("npx로 패키지(%s)를 인터넷에서 내려받아 실행할 수 있습니다", name), &Rule{Program: "npx", Sub: name})
}

func (c *checker) pip(prog string, args []string) {
	sub := firstWord(args)
	switch sub {
	case "list", "show", "freeze", "check", "--version", "-V", "help", "run", "venv", "":
		c.kind("스크립트 실행")
	default:
		c.risk("패키지를 인터넷에서 내려받아 설치합니다", &Rule{Program: prog, Sub: sub})
	}
}

func (c *checker) interpreter(prog string, args []string, dir string) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-m" && i+1 < len(args):
			mod := args[i+1]
			switch mod {
			case "pip":
				c.pip("pip", args[i+2:])
			case "pytest", "unittest", "doctest", "py_compile", "compileall", "mypy", "ruff", "black":
				c.kind("테스트 실행")
			case "http.server":
				c.kind("로컬 서버 실행")
			default:
				c.kind("스크립트 실행")
			}
			return
		case a == "-c" || a == "-e" || a == "--eval" || a == "-p":
			if i+1 < len(args) {
				c.script(args[i+1], prog, true)
			}
			c.kind("스크립트 실행")
			return
		case a == "-":
			c.kind("스크립트 실행")
			return
		case strings.HasPrefix(a, "-"):
			continue
		default:
			if !strings.ContainsAny(a, "$`") {
				c.runFile(prog, c.abs(a, dir))
			}
			c.kind("스크립트 실행")
			c.readArgs(args[i+1:], dir)
			return
		}
	}
	c.kind("스크립트 실행")
}

// mayRun reports whether a script may run: it is in the step's folders or
// in the folders the app gave it to read (its inputs, bundled skills).
func (c *checker) mayRun(p string) bool {
	if c.insideRoots(p) {
		return true
	}
	p = clean(p)
	for _, r := range c.req.Readable {
		if inside(clean(r), p) {
			return true
		}
	}
	return false
}

// riskyCode spots what an inline script does that a person should know.
var riskyCode = []struct {
	re     *regexp.Regexp
	reason string
}{
	{appControlCode, appControl},
	{regexp.MustCompile(`shutil\.rmtree|os\.remove|os\.unlink|os\.rmdir|\.unlink\(|fs\.rm|rmSync|unlinkSync|rimraf`), "직접 쓴 스크립트가 파일을 지웁니다"},
	{regexp.MustCompile(`subprocess|os\.system|os\.popen|child_process|execSync|spawnSync|\bexec\(`), "직접 쓴 스크립트가 다른 프로그램을 실행합니다"},
	{regexp.MustCompile(`requests\.(post|put|patch|delete)|urlopen\(|http\.client|socket\.|fetch\(['"]https?://(?:[^'"/]*)(?:[^l/]|l[^o])`), "직접 쓴 스크립트가 인터넷과 통신합니다"},
}

// appControl is never waived: a script that drives other apps can read
// and click the person's signed-in pages.
const appControl = "다른 앱(브라우저 등)을 조작합니다. 로그인된 화면을 읽거나 누를 수 있습니다"

var appControlCode = regexp.MustCompile(`osascript|(?i:tell\s+application)|System Events|NSAppleScript|pyautogui|cliclick|CGEventPost`)

// script checks the code of a script. full is false for the step's own
// project code, where only driving other apps is looked for.
func (c *checker) script(code, prog string, full bool) {
	for _, r := range riskyCode {
		if r.reason == appControl {
			if r.re.MatchString(code) {
				c.risk(appControl, nil)
			}
			continue
		}
		if full && r.re.MatchString(code) {
			c.risk(r.reason+" — 내용을 확인하세요", &Rule{Program: prog, Sub: "inline"})
		}
	}
	if !full {
		return
	}
	for _, m := range regexp.MustCompile(`['"](~?/[^'"\s]+)['"]`).FindAllStringSubmatch(code, -1) {
		c.read(c.abs(m[1], c.req.Workspace))
	}
}

// wrote records a file the command being checked writes.
func (c *checker) wrote(p string) {
	if c.written != nil && p != "" {
		c.written[clean(p)] = true
	}
}

// maxScript bounds how much of a script file is read to check it.
const maxScript = 256 << 10

// runFile checks a script or program file a command runs: where it is and
// what it does. A shell script is checked like a command. A script the
// AI wrote (in this command, in the temporary folder or in an output
// folder) is checked in full; the project's own code only for driving
// other apps. A file whose content cannot be checked runs only when it is
// the project's own.
func (c *checker) runFile(prog, p string) {
	if !c.mayRun(p) {
		c.risk(fmt.Sprintf("작업 폴더 밖의 스크립트(%s)를 실행합니다", c.show(p)), &Rule{Program: cmp.Or(prog, "script"), Sub: "outside"})
		return
	}
	c.kind("스크립트 실행")
	own := c.ownCode(p)
	var bodies []string
	if c.written[clean(p)] {
		// Written by this command: only a heredoc shows its content.
		if len(c.heredocs) == 0 {
			c.risk(fmt.Sprintf("이 명령이 만든 스크립트(%s)의 내용을 확인할 수 없습니다", c.show(p)), nil)
			return
		}
		bodies, own = c.heredocs, false
	} else {
		b, err := readHead(p)
		if err != nil || bytes.IndexByte(b, 0) >= 0 {
			if !own {
				c.risk(fmt.Sprintf("AI가 만든 프로그램(%s)의 내용을 확인할 수 없습니다", c.show(p)), nil)
			}
			return
		}
		bodies = []string{string(b)}
	}
	if c.depth >= 3 {
		c.risk("스크립트가 다른 스크립트를 여러 겹으로 실행해 끝까지 확인할 수 없습니다", nil)
		return
	}
	c.depth++
	defer func() { c.depth-- }()
	for _, body := range bodies {
		lang := scriptLang(prog, p, body)
		if lang == "shell" {
			c.command(body)
		} else {
			c.script(body, lang, !own)
		}
	}
}

// ownCode reports whether p is the project's code or a file the app gave
// the step, rather than something the AI put in a temporary or output
// folder.
func (c *checker) ownCode(p string) bool {
	p = clean(p)
	if c.req.Workspace != "" && inside(clean(c.req.Workspace), p) {
		return true
	}
	for _, r := range c.req.Readable {
		if inside(clean(r), p) {
			return true
		}
	}
	return false
}

func readHead(p string) ([]byte, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, maxScript))
}

// scriptLang says how a script runs: by the program that runs it, its
// first line (#!) or its extension. Text without either runs in a shell.
func scriptLang(prog, p, body string) string {
	switch prog {
	case "sh", "bash", "zsh", "source", ".":
		return "shell"
	case "":
	default:
		return prog
	}
	if first, _, _ := strings.Cut(body, "\n"); strings.HasPrefix(first, "#!") {
		for _, l := range []string{"python", "node", "ruby", "perl"} {
			if strings.Contains(first, l) {
				return l
			}
		}
		return "shell"
	}
	switch filepath.Ext(p) {
	case ".py":
		return "python"
	case ".js", ".mjs", ".cjs":
		return "node"
	case ".rb":
		return "ruby"
	case ".pl":
		return "perl"
	}
	return "shell"
}

func (c *checker) shellRun(prog string, args []string, dir string) {
	for i, a := range args {
		// "-c", and combined flags like "-lc" or "-ec", take a command.
		if strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") && strings.Contains(a, "c") && i+1 < len(args) {
			c.command(args[i+1])
			return
		}
	}
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			if !strings.ContainsAny(a, "$`") {
				c.runFile(prog, c.abs(a, dir))
			}
			c.kind("스크립트 실행")
			return
		}
	}
	c.kind("스크립트 실행")
}

var sqlWrite = regexp.MustCompile(`(?i)\b(insert|update|delete|drop|create|alter|replace|attach|vacuum|pragma\s+\w+\s*=)\b`)

func (c *checker) sqlite(args []string, dir string) {
	c.kind("데이터베이스 조회")
	var db string
	var sql []string
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			continue
		}
		if db == "" {
			db = a
			continue
		}
		sql = append(sql, a)
	}
	if db == "" || strings.HasPrefix(db, "file:") && strings.Contains(db, "mode=ro") {
		return
	}
	p := c.abs(strings.TrimPrefix(db, "file:"), dir)
	if c.insideRoots(p) {
		return
	}
	if strings.Contains(p, "AgentOffice") {
		c.risk("Agent Office 데이터(다른 서비스의 기록 포함)를 엽니다", &Rule{Program: "sqlite3", Sub: "app-data"})
		return
	}
	if sqlWrite.MatchString(strings.Join(sql, " ")) {
		c.risk(fmt.Sprintf("작업 폴더 밖의 데이터베이스(%s)를 바꿉니다", c.show(p)), nil)
		return
	}
	c.read(p)
}

func (c *checker) tool(prog string, args []string) {
	sub := firstWord(args)
	switch prog {
	case "go":
		switch sub {
		case "get", "install":
			c.risk("Go 패키지를 인터넷에서 내려받아 설치합니다", &Rule{Program: "go", Sub: sub})
		case "test", "vet":
			c.kind("테스트 실행")
		default:
			c.kind("빌드")
		}
	case "cargo":
		switch sub {
		case "install", "add", "publish":
			c.risk("Rust 패키지를 내려받거나 올립니다", &Rule{Program: "cargo", Sub: sub})
		case "test", "check", "clippy":
			c.kind("테스트 실행")
		default:
			c.kind("빌드")
		}
	case "playwright":
		if sub == "install" {
			c.risk("브라우저를 인터넷에서 내려받아 설치합니다", &Rule{Program: "playwright", Sub: "install"})
		} else {
			c.kind("테스트 실행")
		}
	case "make", "cmake", "gradle", "mvn", "swiftc", "javac":
		c.kind("빌드")
	default:
		c.kind("테스트 실행")
	}
}
