package cmd

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestDeploymentsTable(t *testing.T) {
	g := newFakeGH(t)
	var query string
	g.mux.HandleFunc("GET /repos/owner/repo/deployments", func(w http.ResponseWriter, req *http.Request) {
		query = req.URL.RawQuery
		_, _ = io.WriteString(w, "["+deploymentJSON(6, "main", "abc1234def5678")+","+deploymentJSON(5, "v1.0.0", "0123456789abcd")+"]")
	})
	g.json("GET /repos/owner/repo/deployments/6/statuses", 200, `[{"state":"success","environment_url":"https://app.example.com"}]`)
	g.json("GET /repos/owner/repo/deployments/5/statuses", 200, `[]`)

	r, _ := run(t, g, true, nil, "deployments", "production")
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.errOut)
	}
	assertContains(t, query, "environment=production", "per_page=30")
	lines := strings.Split(strings.TrimSpace(r.out), "\n")
	if len(lines) != 3 {
		t.Fatalf("want header + 2 rows:\n%s", r.out)
	}
	assertContains(t, lines[0], "ENVIRONMENT", "SHA", "REF", "CREATOR", "STATE", "CREATED", "ID")
	assertContains(t, lines[1], "production", "abc1234", "main", "hubot", "success", "about 1 day ago", "6")
	assertContains(t, lines[2], "production", "0123456", "v1.0.0", "5")
	if strings.Contains(lines[1], "abc1234def5678") {
		t.Errorf("full SHA in a terminal: %q", lines[1])
	}

	r, _ = run(t, g, false, nil, "deployments", "production")
	rows := strings.Split(strings.TrimSpace(r.out), "\n")
	if rows[0] != "production\tabc1234def5678\tmain\thubot\tsuccess\t2026-10-09T12:00:00Z\t6" {
		t.Errorf("tsv row = %q", rows[0])
	}
}

func TestDeploymentsLimitAndEmpty(t *testing.T) {
	g := newFakeGH(t)
	var query string
	g.mux.HandleFunc("GET /repos/owner/repo/deployments", func(w http.ResponseWriter, req *http.Request) {
		query = req.URL.RawQuery
		_, _ = io.WriteString(w, `[]`)
	})
	r, _ := run(t, g, true, nil, "deployments", "-L", "5")
	if r.code != 0 || r.errOut != "No deployments in owner/repo\n" || r.out != "" {
		t.Fatalf("exit %d out %q err %q", r.code, r.out, r.errOut)
	}
	assertContains(t, query, "per_page=5")
	if strings.Contains(query, "environment=") {
		t.Errorf("filtered by environment without one: %s", query)
	}

	r, _ = run(t, g, true, nil, "deployments", "staging")
	if r.errOut != "No deployments to staging in owner/repo\n" {
		t.Errorf("err %q", r.errOut)
	}

	r, _ = run(t, g, false, nil, "deployments")
	if r.code != 0 || r.errOut != "" || r.out != "" {
		t.Errorf("piped empty: exit %d out %q err %q", r.code, r.out, r.errOut)
	}

	r, _ = run(t, g, false, nil, "deployments", "-L", "0")
	if r.code == 0 {
		t.Errorf("accepted -L 0")
	}
}

func TestDeploymentsJSON(t *testing.T) {
	g := newFakeGH(t)
	g.json("GET /repos/owner/repo/deployments", 200, "["+deploymentJSON(5, "main", "abc1234def5678")+"]")
	g.json("GET /repos/owner/repo/deployments/5/statuses", 200, `[{"state":"failure","log_url":"https://github.com/owner/repo/actions/runs/99"}]`)

	r, _ := run(t, g, false, nil, "deployments", "--json", "id,sha,ref", "--jq", `.[] | "\(.id) \(.sha) \(.ref)"`)
	if want := "5 abc1234def5678 main"; strings.TrimSpace(r.out) != want {
		t.Errorf("json = %q, want %q", r.out, want)
	}
	if n := len(g.find("GET", "/repos/owner/repo/deployments/5/statuses")); n != 0 {
		t.Errorf("read statuses %d times without state or url", n)
	}

	r, _ = run(t, g, false, nil, "deployments", "--json", "state,url,creator,environment", "--jq", `.[] | "\(.state) \(.url) \(.creator) \(.environment)"`)
	if want := "failure https://github.com/owner/repo/actions/runs/99 hubot production"; strings.TrimSpace(r.out) != want {
		t.Errorf("json = %q, want %q", r.out, want)
	}
}
