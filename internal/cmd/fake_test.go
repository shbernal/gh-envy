package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cli/go-gh/v2/pkg/repository"

	"github.com/shbernal/gh-envy/internal/cmdutil"
)

// call is one request the fake API received.
type call struct {
	Method string
	Path   string
	Body   map[string]any
}

// fakeGH serves registered handlers in-process and records every request.
type fakeGH struct {
	t     *testing.T
	mux   *http.ServeMux
	calls []call
}

func newFakeGH(t *testing.T) *fakeGH {
	return &fakeGH{t: t, mux: http.NewServeMux()}
}

func (g *fakeGH) RoundTrip(req *http.Request) (*http.Response, error) {
	var body map[string]any
	if req.Body != nil {
		raw, _ := io.ReadAll(req.Body)
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &body); err != nil {
				g.t.Fatalf("%s %s: invalid JSON body: %s", req.Method, req.URL.Path, raw)
			}
		}
		req.Body = io.NopCloser(bytes.NewReader(raw))
	}
	g.calls = append(g.calls, call{req.Method, req.URL.EscapedPath(), body})
	rec := httptest.NewRecorder()
	if _, pattern := g.mux.Handler(req); pattern == "" {
		g.t.Errorf("unexpected request: %s %s", req.Method, req.URL.String())
		rec.WriteHeader(http.StatusNotFound)
		_, _ = rec.WriteString(`{"message":"Not Found"}`)
	} else {
		g.mux.ServeHTTP(rec, req)
	}
	resp := rec.Result()
	resp.Request = req
	return resp, nil
}

// json registers a handler answering with status and a JSON body.
func (g *fakeGH) json(pattern string, status int, body string) {
	g.mux.HandleFunc(pattern, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	})
}

// find returns the recorded calls matching method and path.
func (g *fakeGH) find(method, path string) []call {
	var out []call
	for _, c := range g.calls {
		if c.Method == method && c.Path == path {
			out = append(out, c)
		}
	}
	return out
}

func (g *fakeGH) one(method, path string) call {
	g.t.Helper()
	got := g.find(method, path)
	if len(got) != 1 {
		var seen []string
		for _, c := range g.calls {
			seen = append(seen, c.Method+" "+c.Path)
		}
		g.t.Fatalf("want 1 %s %s, got %d; calls:\n  %s", method, path, len(got), strings.Join(seen, "\n  "))
	}
	return got[0]
}

// stubPrompter answers prompts from a queue.
type stubPrompter struct {
	t       *testing.T
	answers []any
}

func (p *stubPrompter) next(prompt string) any {
	p.t.Helper()
	if len(p.answers) == 0 {
		p.t.Fatalf("unexpected prompt %q", prompt)
	}
	a := p.answers[0]
	p.answers = p.answers[1:]
	return a
}

func (p *stubPrompter) Select(prompt, _ string, _ []string) (int, error) {
	return p.next(prompt).(int), nil
}

func (p *stubPrompter) Input(prompt, _ string) (string, error) {
	return p.next(prompt).(string), nil
}

func (p *stubPrompter) Confirm(prompt string, _ bool) (bool, error) {
	return p.next(prompt).(bool), nil
}

type stubBrowser struct{ opened []string }

func (b *stubBrowser) Browse(u string) error {
	b.opened = append(b.opened, u)
	return nil
}

type result struct {
	code   int
	out    string
	errOut string
}

// run executes gh envy against the fake API for repo owner/repo.
func run(t *testing.T, g *fakeGH, tty bool, prompter *stubPrompter, args ...string) (result, *stubBrowser) {
	t.Helper()
	io, out, errOut := cmdutil.Test(tty)
	repo := repository.Repository{Host: "github.com", Owner: "owner", Name: "repo"}
	b := &stubBrowser{}
	if prompter == nil {
		prompter = &stubPrompter{t: t}
	}
	f := &cmdutil.Factory{IO: io, Transport: g, Prompter: prompter, Browser: b, Repo: &repo}
	code := Execute(f, args)
	return result{code, out.String(), errOut.String()}, b
}

// envJSON renders an environment payload.
func envJSON(name string, opts ...string) string {
	rules := ""
	policy := "null"
	bypass := "true"
	for _, o := range opts {
		switch {
		case strings.HasPrefix(o, "wait="):
			rules += fmt.Sprintf(`,{"id":1,"type":"wait_timer","wait_timer":%s}`, strings.TrimPrefix(o, "wait="))
		case o == "reviewers":
			rules += `,{"id":2,"type":"required_reviewers","prevent_self_review":true,"reviewers":[` +
				`{"type":"User","reviewer":{"id":10,"login":"octocat"}},` +
				`{"type":"Team","reviewer":{"id":20,"slug":"ops","html_url":"https://github.com/orgs/acme/teams/ops"}}]}`
		case o == "custom":
			policy = `{"protected_branches":false,"custom_branch_policies":true}`
			rules += `,{"id":3,"type":"branch_policy"}`
		case o == "protected":
			policy = `{"protected_branches":true,"custom_branch_policies":false}`
		case o == "no-bypass":
			bypass = "false"
		}
	}
	return fmt.Sprintf(`{"id":42,"name":%q,"html_url":"https://github.com/owner/repo/deployments","created_at":"2026-10-01T12:00:00Z","updated_at":"2026-10-09T12:00:00Z","can_admins_bypass":%s,"protection_rules":[%s],"deployment_branch_policy":%s}`,
		name, bypass, strings.TrimPrefix(rules, ","), policy)
}

const envBase = "/repos/owner/repo/environments/"

func (g *fakeGH) env(name string, opts ...string) {
	g.json("GET "+envBase+name, 200, envJSON(name, opts...))
}

func (g *fakeGH) emptyDetails(name string) {
	g.json("GET "+envBase+name+"/deployment_protection_rules", 200, `{"total_count":0,"custom_deployment_protection_rules":[]}`)
	g.json("GET "+envBase+name+"/variables", 200, `{"total_count":0,"variables":[]}`)
	g.json("GET "+envBase+name+"/secrets", 200, `{"total_count":0,"secrets":[]}`)
}

func (g *fakeGH) users() {
	g.json("GET /users/octocat", 200, `{"id":10,"login":"octocat"}`)
	g.json("GET /users/hubot", 200, `{"id":11,"login":"hubot"}`)
	g.json("GET /orgs/acme/teams/ops", 200, `{"id":20,"slug":"ops"}`)
}
