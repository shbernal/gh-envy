package cmd

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

const runsBase = "/repos/owner/repo/actions/runs"

func TestWaitingTable(t *testing.T) {
	g := newFakeGH(t)
	g.json("GET "+runsBase, 200, runsJSON(runJSON(101, "Deploy", "main"), runJSON(102, "Deploy", "dev")))
	g.json("GET "+runsBase+"/101/pending_deployments", 200, "["+pendingJSON(1, "staging", "approver")+","+pendingJSON(2, "production")+"]")
	g.json("GET "+runsBase+"/102/pending_deployments", 200, "["+pendingJSON(1, "staging", "no-reviewers", "timer=10@2026-10-10T11:56:00Z")+"]")

	r, _ := run(t, g, true, nil, "waiting")
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.errOut)
	}
	lines := strings.Split(strings.TrimSpace(r.out), "\n")
	if len(lines) != 4 {
		t.Fatalf("want header + 3 rows:\n%s", r.out)
	}
	assertContains(t, lines[0], "ENVIRONMENT", "WORKFLOW", "TITLE", "BRANCH", "RUN ID", "WAITING", "GATE")
	assertContains(t, lines[1], "staging", "Deploy", "Ship it", "main", "101", "about 1 day ago", "you")
	assertContains(t, lines[2], "production", "101", "reviewers")
	assertContains(t, lines[3], "staging", "102", "dev", "timer 6m")
	if call := g.calls[0]; call.Path != runsBase {
		t.Errorf("first call %s", call.Path)
	}

	r, _ = run(t, g, false, nil, "waiting")
	rows := strings.Split(strings.TrimSpace(r.out), "\n")
	if rows[0] != "staging\tDeploy\tShip it\tmain\t101\t2026-10-09T12:00:00Z\tyou" {
		t.Errorf("tsv row = %q", rows[0])
	}
}

func TestWaitingQuery(t *testing.T) {
	g := newFakeGH(t)
	var query string
	g.mux.HandleFunc("GET "+runsBase, func(w http.ResponseWriter, req *http.Request) {
		query = req.URL.RawQuery
		_, _ = io.WriteString(w, runsJSON())
	})
	r, _ := run(t, g, true, nil, "waiting", "-b", "main")
	if r.code != 0 || r.errOut != "No runs waiting in owner/repo\n" || r.out != "" {
		t.Fatalf("exit %d out %q err %q", r.code, r.out, r.errOut)
	}
	assertContains(t, query, "status=waiting", "branch=main")

	r, _ = run(t, g, false, nil, "waiting")
	if r.code != 0 || r.errOut != "" || r.out != "" {
		t.Errorf("piped empty: exit %d out %q err %q", r.code, r.out, r.errOut)
	}
}

func TestWaitingEnvironmentFilterAndJSON(t *testing.T) {
	g := newFakeGH(t)
	g.json("GET "+runsBase, 200, runsJSON(runJSON(101, "Deploy", "main")))
	g.json("GET "+runsBase+"/101/pending_deployments", 200, "["+pendingJSON(1, "staging", "approver")+","+pendingJSON(2, "production")+"]")

	r, _ := run(t, g, false, nil, "waiting", "Production", "--json", "runId,environment,environmentId,canApprove,reviewers,actor")
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.errOut)
	}
	want := `[{"actor":"hubot","canApprove":false,"environment":"production","environmentId":2,"reviewers":[{"id":10,"name":"octocat","type":"User"}],"runId":101}]`
	if strings.TrimSpace(r.out) != want {
		t.Errorf("json = %s\nwant   %s", r.out, want)
	}
}

func TestWaitingWorkflow(t *testing.T) {
	g := newFakeGH(t)
	g.json("GET /repos/owner/repo/actions/workflows/deploy.yml/runs", 200, runsJSON())
	g.json("GET /repos/owner/repo/actions/workflows", 200, `{"total_count":2,"workflows":[{"id":7,"name":"CI"},{"id":8,"name":"Deploy"}]}`)
	g.json("GET /repos/owner/repo/actions/workflows/8/runs", 200, runsJSON())

	for _, w := range []string{"deploy.yml", "deploy", "8"} {
		if r, _ := run(t, g, false, nil, "waiting", "-w", w); r.code != 0 {
			t.Fatalf("-w %s: exit %d: %s", w, r.code, r.errOut)
		}
	}
	g.one("GET", "/repos/owner/repo/actions/workflows/deploy.yml/runs")
	g.one("GET", "/repos/owner/repo/actions/workflows")
	if got := g.find("GET", "/repos/owner/repo/actions/workflows/8/runs"); len(got) != 2 {
		t.Errorf("display name and ID lookups = %d", len(got))
	}

	r, _ := run(t, g, false, nil, "waiting", "-w", "Release")
	assertContains(t, r.errOut, `could not find any workflows named "Release"`)
}

func TestWaitingLimitCountsRuns(t *testing.T) {
	g := newFakeGH(t)
	g.json("GET "+runsBase, 200, runsJSON(runJSON(101, "Deploy", "main"), runJSON(102, "Deploy", "main"), runJSON(103, "Deploy", "main")))
	g.json("GET "+runsBase+"/101/pending_deployments", 200, "["+pendingJSON(1, "staging")+","+pendingJSON(2, "production")+"]")
	g.json("GET "+runsBase+"/102/pending_deployments", 200, "["+pendingJSON(1, "staging")+"]")

	r, _ := run(t, g, false, nil, "waiting", "-L", "2")
	if n := len(strings.Split(strings.TrimSpace(r.out), "\n")); r.code != 0 || n != 3 {
		t.Fatalf("exit %d, %d rows: %s %s", r.code, n, r.out, r.errOut)
	}
	if len(g.find("GET", runsBase+"/103/pending_deployments")) != 0 {
		t.Error("fetched gates of a run beyond the limit")
	}
}

func TestGateReasons(t *testing.T) {
	tests := []struct {
		opts []string
		want string
	}{
		{[]string{"approver"}, "you"},
		{nil, "reviewers"},
		{[]string{"no-reviewers", "timer=10@2026-10-10T11:56:00Z"}, "timer 6m"},
		{[]string{"no-reviewers", "timer=90@null"}, "timer 1h30m"},
		{[]string{"no-reviewers", "timer=10@2026-10-10T11:00:00Z"}, "app"},
		{[]string{"no-reviewers"}, "app"},
		{[]string{"approver", "timer=5@2026-10-10T11:59:30Z"}, "you,timer 5m"},
	}
	for _, tt := range tests {
		g := newFakeGH(t)
		g.json("GET "+runsBase, 200, runsJSON(runJSON(101, "Deploy", "main")))
		g.json("GET "+runsBase+"/101/pending_deployments", 200, "["+pendingJSON(1, "staging", tt.opts...)+"]")
		r, _ := run(t, g, false, nil, "waiting")
		fields := strings.Split(strings.TrimSpace(r.out), "\t")
		if got := fields[len(fields)-1]; got != tt.want {
			t.Errorf("%v: gate %q, want %q", tt.opts, got, tt.want)
		}
	}
}
