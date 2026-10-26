// Command demo-svg renders the README demos: it runs real gh envy commands
// in-process against the fakegh demo repository and writes each scene as an
// animated terminal SVG, in a light and a dark palette.
//
//	go run ./tools/demo-svg assets/demo
package main

import (
	"bytes"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/cli/go-gh/v2/pkg/repository"

	"github.com/shbernal/gh-envy/internal/cmd"
	"github.com/shbernal/gh-envy/internal/cmdutil"
	"github.com/shbernal/gh-envy/internal/fakegh"
)

const (
	cols       = 108
	fontSize   = 14.0
	charW      = fontSize * 0.6
	lineH      = 21.0
	padX       = 22.0
	padTop     = 50.0
	padBottom  = 22.0
	typePerCh  = 0.045
	afterEnter = 0.35
	afterOut   = 1.6
	hold       = 9.0
)

type step struct {
	shown   []string // command as displayed, one entry per visual line
	args    []string
	answers []string
}

type scene struct {
	name, title string
	steps       []step
}

var scenes = []scene{
	{"overview", "acme/shop", []step{
		{shown: []string{"gh envy list"}, args: []string{"list"}},
		{shown: []string{"gh envy view production"}, args: []string{"view", "production"}},
	}},
	{"create", "acme/shop", []step{
		{
			shown: []string{"gh envy create canary --reviewer octocat -r acme/sre --prevent-self-review \\", "  --wait-timer 15m --branch main --tag 'v*' --admin-bypass=false"},
			args:  []string{"create", "canary", "--reviewer", "octocat", "-r", "acme/sre", "--prevent-self-review", "--wait-timer", "15m", "--branch", "main", "--tag", "v*", "--admin-bypass=false"},
		},
		{
			shown: []string{"gh envy edit canary --add-reviewer hubot --wait-timer 1h --remove-tag 'v*'"},
			args:  []string{"edit", "canary", "--add-reviewer", "hubot", "--wait-timer", "1h", "--remove-tag", "v*"},
		},
		{
			shown: []string{"gh envy view canary --json name,reviewers,waitTimer,branchPolicies \\", "  --jq '{env: .name, reviewers: [.reviewers[].name], wait: .waitTimer, refs: [.branchPolicies[].name]}'"},
			args:  []string{"view", "canary", "--json", "name,reviewers,waitTimer,branchPolicies", "--jq", "{env: .name, reviewers: [.reviewers[].name], wait: .waitTimer, refs: [.branchPolicies[].name]}"},
		},
	}},
	{"approve", "acme/shop", []step{
		{shown: []string{"gh envy waiting"}, args: []string{"waiting"}},
		{shown: []string{"gh envy approve -w deploy.yml -c 'ship it'"}, args: []string{"approve", "-w", "deploy.yml", "-c", "ship it"}},
		{shown: []string{"gh envy waiting"}, args: []string{"waiting"}},
	}},
	{"clone", "acme/shop", []step{
		{shown: []string{"gh envy clone staging qa"}, args: []string{"clone", "staging", "qa"}},
		{shown: []string{"gh envy delete preview/pr-128"}, args: []string{"delete", "preview/pr-128"}, answers: []string{"preview/pr-128"}},
		{shown: []string{"gh envy list"}, args: []string{"list"}},
	}},
}

type palette struct {
	bg, chrome, border, fg, title string
	ansi                          map[int]string
}

var palettes = map[string]palette{
	"dark": {"#1a1b26", "#16161e", "#2a2e42", "#c0caf5", "#565f89", map[int]string{
		31: "#f7768e", 32: "#9ece6a", 33: "#e0af68", 34: "#7aa2f7", 36: "#7dcfff", 90: "#6b7394",
	}},
	"light": {"#fbfbf8", "#eef0e8", "#d5d9cc", "#343b58", "#8990a3", map[int]string{
		31: "#b4334d", 32: "#4f7a1f", 33: "#8f5e15", 34: "#2e5cb8", 36: "#0f6e85", 90: "#7a8096",
	}},
}

type span struct {
	text  string
	color int
	bold  bool
}

type line struct {
	spans  []span
	at     float64 // when the line appears
	typing bool    // command line typed out from at
	chars  int
}

func main() {
	out := "assets/demo"
	if len(os.Args) > 1 {
		out = os.Args[1]
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		fail(err)
	}
	for _, sc := range scenes {
		lines, total := record(sc)
		for name, p := range palettes {
			path := filepath.Join(out, fmt.Sprintf("%s-%s.svg", sc.name, name))
			if err := os.WriteFile(path, render(sc, lines, total, p), 0o644); err != nil {
				fail(err)
			}
			fmt.Println(path)
		}
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

// surveyPrompter prints prompts the way gh's interactive prompts look.
type surveyPrompter struct {
	out     *bytes.Buffer
	answers []string
}

func (p *surveyPrompter) Input(prompt, _ string) (string, error) {
	a := p.answers[0]
	p.answers = p.answers[1:]
	fmt.Fprintf(p.out, "\x1b[32m?\x1b[0m \x1b[1m%s\x1b[0m \x1b[36m%s\x1b[0m\n", prompt, a)
	return a, nil
}

// Password masks the answer like survey does.
func (p *surveyPrompter) Password(prompt string) (string, error) {
	a := p.answers[0]
	p.answers = p.answers[1:]
	fmt.Fprintf(p.out, "\x1b[32m?\x1b[0m \x1b[1m%s\x1b[0m %s\n", prompt, strings.Repeat("*", len(a)))
	return a, nil
}

func (p *surveyPrompter) Select(string, string, []string) (int, error) { return 0, nil }
func (p *surveyPrompter) MultiSelect(_ string, _, options []string) ([]int, error) {
	all := make([]int, len(options))
	for i := range all {
		all[i] = i
	}
	return all, nil
}
func (p *surveyPrompter) Confirm(string, bool) (bool, error) { return true, nil }

func record(sc scene) ([]line, float64) {
	now := time.Now()
	state := fakegh.Seed(now)
	transport := fakegh.Transport{H: state.Handler()}
	owner, name, _ := strings.Cut(fakegh.DemoRepo, "/")
	repo := repository.Repository{Host: "github.com", Owner: owner, Name: name}

	var lines []line
	t := 0.6
	for n, st := range sc.steps {
		if n > 0 {
			lines = append(lines, line{at: t})
		}
		for i, shown := range st.shown {
			prompt := "$ "
			if i > 0 {
				prompt = "  "
			}
			lines = append(lines, line{
				spans:  []span{{prompt, 32, true}, {shown, 0, false}},
				at:     t,
				typing: true,
				chars:  len([]rune(shown)),
			})
			t += float64(len([]rune(shown)))*typePerCh + 0.15
		}
		t += afterEnter

		var buf bytes.Buffer
		io := &cmdutil.IO{
			In: &bytes.Buffer{}, Out: &buf, ErrOut: &buf,
			StdoutTTY: true, StdinTTY: true, ColorEnabled: true,
			Width: cols, Now: func() time.Time { return now },
		}
		f := &cmdutil.Factory{IO: io, Transport: transport, Repo: &repo, Prompter: &surveyPrompter{out: &buf, answers: st.answers}}
		if code := cmd.Execute(f, st.args); code != 0 {
			fail(fmt.Errorf("%s: exit %d\n%s", strings.Join(st.args, " "), code, buf.String()))
		}
		for _, l := range strings.Split(strings.TrimRight(buf.String(), "\n"), "\n") {
			lines = append(lines, line{spans: parseANSI(l), at: t})
		}
		t += afterOut
	}
	return lines, t + hold
}

// parseANSI splits a line on SGR escape sequences.
func parseANSI(s string) []span {
	var out []span
	cur := span{}
	for len(s) > 0 {
		i := strings.Index(s, "\x1b[")
		if i < 0 {
			cur.text = s
			out = append(out, cur)
			break
		}
		if i > 0 {
			cur.text = s[:i]
			out = append(out, cur)
		}
		s = s[i+2:]
		end := strings.IndexByte(s, 'm')
		if end < 0 {
			break
		}
		for _, code := range strings.Split(s[:end], ";") {
			n, _ := strconv.Atoi(code)
			switch {
			case n == 0:
				cur.color, cur.bold = 0, false
			case n == 1:
				cur.bold = true
			case n == 22:
				cur.bold = false
			case n == 39:
				cur.color = 0
			case (n >= 30 && n <= 37) || (n >= 90 && n <= 97):
				cur.color = n
			}
		}
		s = s[end+1:]
	}
	return out
}

func render(sc scene, lines []line, total float64, p palette) []byte {
	width := padX*2 + cols*charW
	height := padTop + float64(len(lines))*lineH + padBottom
	pct := func(t float64) string { return strconv.FormatFloat(t/total*100, 'f', 3, 64) }

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" xml:space="preserve" width="%.0f" height="%.0f" viewBox="0 0 %.0f %.0f" role="img" aria-label="gh envy demo: %s">`, width, height, width, height, sc.name)
	b.WriteString("\n<style>\n")
	fmt.Fprintf(&b, "text{font-family:'JetBrains Mono','SF Mono',Menlo,Consolas,'Liberation Mono',monospace;font-size:%.0fpx;white-space:pre;fill:%s}\n", fontSize, p.fg)
	b.WriteString(".b{font-weight:700}\n")
	for i, l := range lines {
		if l.typing {
			done := l.at + float64(l.chars)*typePerCh
			fmt.Fprintf(&b, "@keyframes t%d{0%%,%s%%{clip-path:inset(0 100%% 0 0)}%s%%{clip-path:inset(0 100%% 0 0);animation-timing-function:steps(%d,end)}%s%%,100%%{clip-path:inset(0 0 0 0)}}\n",
				i, pct(l.at), pct(l.at), max(l.chars, 1), pct(done))
			fmt.Fprintf(&b, ".c%d{animation:t%d %.2fs infinite}\n", i, i, total)
		}
		fmt.Fprintf(&b, "@keyframes a%d{0%%,%s%%{opacity:0}%s%%,100%%{opacity:1}}\n", i, pct(l.at), pct(l.at+0.01))
		fmt.Fprintf(&b, ".l%d{animation:a%d %.2fs infinite}\n", i, i, total)
	}
	b.WriteString("@media (prefers-reduced-motion:reduce){*{animation:none!important}}\n</style>\n")

	fmt.Fprintf(&b, `<rect x="0.5" y="0.5" width="%.0f" height="%.0f" rx="10" fill="%s" stroke="%s"/>`+"\n", width-1, height-1, p.bg, p.border)
	fmt.Fprintf(&b, `<path d="M0.5 10.5a10 10 0 0 1 10-10h%.0fa10 10 0 0 1 10 10v20h-%.0fz" fill="%s"/>`+"\n", width-21, width-1, p.chrome)
	for i, c := range []string{"#f7768e", "#e0af68", "#9ece6a"} {
		fmt.Fprintf(&b, `<circle cx="%d" cy="16" r="5.5" fill="%s"/>`, 22+i*18, c)
	}
	fmt.Fprintf(&b, "\n"+`<text x="%.0f" y="20.5" text-anchor="middle" style="font-size:12px;fill:%s">%s</text>`+"\n", width/2, p.title, html.EscapeString(sc.title))

	for i, l := range lines {
		y := padTop + float64(i)*lineH + fontSize
		class := fmt.Sprintf("l%d", i)
		if l.typing {
			// The prompt shows with the line; the command after it is typed out.
			fmt.Fprintf(&b, `<g class="%s"><text x="%.1f" y="%.1f">%s</text>`, class, padX, y, tspans(l.spans[:1], p))
			fmt.Fprintf(&b, `<text class="c%d" x="%.1f" y="%.1f">%s</text></g>`+"\n", i, padX+2*charW, y, tspans(l.spans[1:], p))
			continue
		}
		fmt.Fprintf(&b, `<text class="%s" x="%.1f" y="%.1f">%s</text>`+"\n", class, padX, y, tspans(l.spans, p))
	}
	b.WriteString("</svg>\n")
	return []byte(b.String())
}

func tspans(spans []span, p palette) string {
	var b strings.Builder
	for _, s := range spans {
		if s.text == "" {
			continue
		}
		attrs := ""
		if c, ok := p.ansi[s.color]; ok {
			attrs += ` fill="` + c + `"`
		}
		if s.bold {
			attrs += ` class="b"`
		}
		fmt.Fprintf(&b, "<tspan%s>%s</tspan>", attrs, html.EscapeString(s.text))
	}
	return b.String()
}
