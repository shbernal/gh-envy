package cmd

import (
	"reflect"
	"strings"
	"testing"
)

const pendingPath = runsBase + "/101/pending_deployments"

// waitingRun serves run 101 on main, waiting at the given gates.
func (g *fakeGH) waitingRun(gates ...string) {
	g.json("GET "+runsBase+"/101", 200, runJSON(101, "Deploy", "main"))
	g.json("GET "+pendingPath, 200, "["+strings.Join(gates, ",")+"]")
	g.json("POST "+pendingPath, 200, `[]`)
}

func TestApproveByID(t *testing.T) {
	g := newFakeGH(t)
	g.waitingRun(pendingJSON(1, "staging", "approver"))

	r, _ := run(t, g, true, nil, "approve", "101", "-c", "ship it")
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.errOut)
	}
	assertContains(t, r.out, "✓ Approved staging for run 101 (Deploy, main)", "gh run watch 101\n")
	body := g.one("POST", pendingPath).Body
	want := map[string]any{"environment_ids": []any{float64(1)}, "state": "approved", "comment": "ship it"}
	if !reflect.DeepEqual(body, want) {
		t.Errorf("body = %v, want %v", body, want)
	}

	r, _ = run(t, g, false, nil, "approve", "101")
	if r.code != 0 || r.out != "" {
		t.Errorf("piped: exit %d out %q err %q", r.code, r.out, r.errOut)
	}
	if body := g.find("POST", pendingPath)[1].Body; body["comment"] != "" {
		t.Errorf("comment = %v, want empty string", body["comment"])
	}
}

func TestReject(t *testing.T) {
	g := newFakeGH(t)
	g.waitingRun(pendingJSON(1, "staging", "approver"))
	r, _ := run(t, g, true, nil, "reject", "101")
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.errOut)
	}
	assertContains(t, r.out, "✗ Rejected staging for run 101")
	if strings.Contains(r.out, "gh run watch") {
		t.Error("reject printed the watch hint")
	}
	if s := g.one("POST", pendingPath).Body["state"]; s != "rejected" {
		t.Errorf("state = %v", s)
	}
}

func TestApproveURLOverridesRepoFlag(t *testing.T) {
	g := newFakeGH(t)
	g.json("GET /repos/acme/app/actions/runs/7", 200, runJSON(7, "Deploy", "main"))
	g.json("GET /repos/acme/app/actions/runs/7/pending_deployments", 200, "["+pendingJSON(1, "staging", "approver")+"]")
	g.json("POST /repos/acme/app/actions/runs/7/pending_deployments", 200, `[]`)

	r, _ := run(t, g, true, nil, "approve", "-R", "other/repo", "https://github.com/acme/app/actions/runs/7/job/9")
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.errOut)
	}
	assertContains(t, r.out, "gh run watch 7 -R acme/app")
	g.one("POST", "/repos/acme/app/actions/runs/7/pending_deployments")
}

func TestApproveNotWaiting(t *testing.T) {
	g := newFakeGH(t)
	g.waitingRun()
	r, _ := run(t, g, false, nil, "approve", "101")
	if r.code != 1 || strings.TrimSpace(r.errOut) != "run 101 is not waiting for review" {
		t.Errorf("exit %d err %q", r.code, r.errOut)
	}
	r, _ = run(t, g, false, nil, "approve", "nope")
	assertContains(t, r.errOut, `invalid run "nope"`, "Usage:")
}

func TestApprovePicksSingleWaitingRun(t *testing.T) {
	g := newFakeGH(t)
	g.json("GET "+runsBase, 200, runsJSON(runJSON(101, "Deploy", "main"), runJSON(102, "Deploy", "dev")))
	g.json("GET "+runsBase+"/102/pending_deployments", 200, "["+pendingJSON(1, "staging", "approver")+"]")
	g.waitingRun(pendingJSON(2, "production", "approver"))

	r, _ := run(t, g, false, nil, "approve", "-e", "production")
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.errOut)
	}
	if ids := g.one("POST", pendingPath).Body["environment_ids"]; !reflect.DeepEqual(ids, []any{float64(2)}) {
		t.Errorf("environment_ids = %v", ids)
	}
}

func TestApproveAmbiguous(t *testing.T) {
	g := newFakeGH(t)
	g.json("GET "+runsBase, 200, runsJSON(runJSON(101, "Deploy", "main"), runJSON(102, "Deploy", "dev")))
	g.json("GET "+runsBase+"/102/pending_deployments", 200, "["+pendingJSON(1, "staging", "approver")+"]")
	g.waitingRun(pendingJSON(1, "staging", "approver"))

	r, _ := run(t, g, false, nil, "approve")
	if r.code != 1 {
		t.Fatalf("exit %d", r.code)
	}
	assertContains(t, r.errOut, "2 runs are waiting for review", "  101  Deploy  main", "  102  Deploy  dev", "Usage:")
	if len(g.find("POST", pendingPath)) != 0 {
		t.Error("approved an ambiguous run")
	}

	r, _ = run(t, g, true, &stubPrompter{t: t, answers: []any{0}}, "approve")
	if r.code != 0 {
		t.Fatalf("tty: exit %d: %s", r.code, r.errOut)
	}
	g.one("POST", pendingPath)
}

func TestApproveNoneWaiting(t *testing.T) {
	g := newFakeGH(t)
	g.json("GET "+runsBase, 200, runsJSON())
	r, _ := run(t, g, false, nil, "approve")
	if r.code != 1 || strings.TrimSpace(r.errOut) != "no runs waiting for review" {
		t.Errorf("exit %d err %q", r.code, r.errOut)
	}
}

func TestApproveEnvNotWaiting(t *testing.T) {
	g := newFakeGH(t)
	g.waitingRun(pendingJSON(1, "staging", "approver"))
	r, _ := run(t, g, false, nil, "approve", "101", "-e", "production")
	if r.code != 1 || strings.TrimSpace(r.errOut) != "run 101 is not waiting on production (waiting: staging)" {
		t.Errorf("exit %d err %q", r.code, r.errOut)
	}
}

func TestApproveSeveralEnvs(t *testing.T) {
	g := newFakeGH(t)
	g.waitingRun(pendingJSON(1, "staging", "approver"), pendingJSON(2, "production", "approver"))

	r, _ := run(t, g, false, nil, "approve", "101")
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.errOut)
	}
	if ids := g.one("POST", pendingPath).Body["environment_ids"]; !reflect.DeepEqual(ids, []any{float64(1), float64(2)}) {
		t.Errorf("non-tty environment_ids = %v", ids)
	}

	r, _ = run(t, g, true, &stubPrompter{t: t, answers: []any{[]int{1}}}, "approve", "101")
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.errOut)
	}
	if ids := g.find("POST", pendingPath)[1].Body["environment_ids"]; !reflect.DeepEqual(ids, []any{float64(2)}) {
		t.Errorf("picked environment_ids = %v", ids)
	}
	assertContains(t, r.out, "Approved production for run 101")

	r, _ = run(t, g, true, &stubPrompter{t: t, answers: []any{[]int{}}}, "approve", "101")
	if r.code != 2 || len(g.find("POST", pendingPath)) != 2 {
		t.Errorf("empty pick: exit %d", r.code)
	}
}

func TestApproveBlocked(t *testing.T) {
	g := newFakeGH(t)
	g.waitingRun(pendingJSON(1, "staging"), pendingJSON(2, "production", "approver"))
	g.env("staging", "reviewers")
	g.json("GET /user", 200, `{"login":"hubot"}`)

	r, _ := run(t, g, false, nil, "approve", "101", "-e", "staging")
	if r.code != 1 || strings.TrimSpace(r.errOut) != "you triggered this run and staging prevents self-review; another reviewer must approve" {
		t.Errorf("self-review: exit %d err %q", r.code, r.errOut)
	}

	r, _ = run(t, g, false, nil, "approve", "101")
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.errOut)
	}
	assertContains(t, r.errOut, "! Skipping staging: you triggered this run")
	if ids := g.one("POST", pendingPath).Body["environment_ids"]; !reflect.DeepEqual(ids, []any{float64(2)}) {
		t.Errorf("environment_ids = %v", ids)
	}
}

func TestApproveNotReviewer(t *testing.T) {
	g := newFakeGH(t)
	g.waitingRun(pendingJSON(1, "staging"), pendingJSON(2, "qa", "no-reviewers"))
	g.env("staging")
	r, _ := run(t, g, false, nil, "approve", "101")
	if r.code != 1 {
		t.Fatalf("exit %d", r.code)
	}
	assertContains(t, r.errOut, "you are not a required reviewer of staging (reviewers: octocat)", "qa has no required reviewers")
}

func TestApproveReviewError(t *testing.T) {
	g := newFakeGH(t)
	g.json("GET "+runsBase+"/101", 200, runJSON(101, "Deploy", "main"))
	g.json("GET "+pendingPath, 200, "["+pendingJSON(1, "staging", "approver")+"]")
	g.json("POST "+pendingPath, 422, `{"message":"Validation Failed","errors":"No pending deployment requests to approve or reject","status":"422"}`)
	r, _ := run(t, g, false, nil, "approve", "101")
	if r.code != 1 || !strings.HasPrefix(r.errOut, "No pending deployment requests to approve or reject\nReviewing a deployment needs write access") {
		t.Errorf("exit %d err %q", r.code, r.errOut)
	}
}

func TestApproveWatch(t *testing.T) {
	g := newFakeGH(t)
	g.waitingRun(pendingJSON(1, "staging", "approver"), pendingJSON(2, "production", "approver"))

	r, _ := run(t, g, false, nil, "approve", "101", "--watch")
	if r.code != 1 || !strings.Contains(r.errOut, "`--watch` requires a terminal") || len(g.calls) != 0 {
		t.Errorf("non-tty: exit %d err %q calls %d", r.code, r.errOut, len(g.calls))
	}

	r, _ = run(t, g, true, &stubPrompter{t: t, answers: []any{[]int{0, 1}}}, "approve", "101", "--watch", "-R", "owner/repo")
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.errOut)
	}
	if strings.Contains(r.out, "gh run watch") {
		t.Error("printed the watch hint with --watch")
	}
	if want := [][]string{{"run", "watch", "101", "-R", "owner/repo"}}; !reflect.DeepEqual(g.execs, want) {
		t.Errorf("execs = %v, want %v", g.execs, want)
	}
}
