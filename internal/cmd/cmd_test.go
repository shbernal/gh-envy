package cmd

import (
	"reflect"
	"strings"
	"testing"
)

func assertContains(t *testing.T, s string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(s, w) {
			t.Errorf("output missing %q:\n%s", w, s)
		}
	}
}

func listJSON(envs ...string) string {
	return `{"total_count":` + string(rune('0'+len(envs))) + `,"environments":[` + strings.Join(envs, ",") + `]}`
}

func TestListTable(t *testing.T) {
	g := newFakeGH(t)
	g.json("GET /repos/owner/repo/environments", 200, listJSON(envJSON("production", "wait=30", "reviewers", "custom"), envJSON("staging")))

	r, _ := run(t, g, true, nil, "list")
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.errOut)
	}
	assertContains(t, r.out, "NAME", "production", "octocat, acme/ops", "30m", "custom", "staging", "none", "all")

	r, _ = run(t, g, false, nil, "list")
	lines := strings.Split(strings.TrimSpace(r.out), "\n")
	if lines[0] != "production\toctocat,acme/ops\t30\tcustom\ttrue\t2026-10-09T12:00:00Z" {
		t.Errorf("tsv row = %q", lines[0])
	}
}

func TestListJSONFetchesOnlyRequestedDetails(t *testing.T) {
	g := newFakeGH(t)
	g.json("GET /repos/owner/repo/environments", 200, listJSON(envJSON("staging")))
	g.json("GET "+envBase+"staging/variables", 200, `{"total_count":1,"variables":[{"name":"REGION","value":"eu","updated_at":"2026-10-01T00:00:00Z"}]}`)

	r, _ := run(t, g, false, nil, "list", "--json", "name,variables", "--jq", ".[].variables[].value")
	if r.code != 0 || strings.TrimSpace(r.out) != "eu" {
		t.Fatalf("exit %d out %q err %q", r.code, r.out, r.errOut)
	}
	if len(g.find("GET", envBase+"staging/secrets")) != 0 {
		t.Error("secrets fetched though not requested")
	}
}

func TestJSONFlagErrors(t *testing.T) {
	g := newFakeGH(t)
	r, _ := run(t, g, false, nil, "list", "--jq", ".")
	if r.code != 1 || !strings.Contains(r.errOut, "cannot use `--jq` without specifying `--json`") {
		t.Errorf("jq without json: %d %q", r.code, r.errOut)
	}
	r, _ = run(t, g, false, nil, "list", "--json", "nope")
	assertContains(t, r.errOut, `Unknown JSON field: "nope"`, "waitTimer")
	r, _ = run(t, g, false, nil, "view", "x", "--json", "name", "--web")
	assertContains(t, r.errOut, "cannot use `--web` with `--json`")
}

func TestView(t *testing.T) {
	g := newFakeGH(t)
	g.env("production", "wait=90", "reviewers", "custom", "no-bypass")
	g.json("GET "+envBase+"production/deployment-branch-policies", 200, `{"total_count":2,"branch_policies":[{"id":7,"name":"main","type":"branch"},{"id":8,"name":"v*","type":"tag"}]}`)
	g.json("GET "+envBase+"production/deployment_protection_rules", 200, `{"total_count":1,"custom_deployment_protection_rules":[{"id":5,"enabled":true,"app":{"id":99,"slug":"datadog"}}]}`)
	g.json("GET "+envBase+"production/variables", 200, `{"total_count":1,"variables":[{"name":"REGION","value":"eu-west-1"}]}`)
	g.json("GET "+envBase+"production/secrets", 200, `{"total_count":1,"secrets":[{"name":"DEPLOY_KEY","updated_at":"2026-10-08T12:00:00Z"}]}`)

	r, _ := run(t, g, true, nil, "view", "production")
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.errOut)
	}
	assertContains(t, r.out,
		"production", "owner/repo",
		"octocat, acme/ops (prevent self-review)",
		"1h30m", "not allowed",
		"main (branch), v* (tag)",
		"datadog",
		"REGION", "eu-west-1", "DEPLOY_KEY",
		"https://github.com/owner/repo/settings/environments/42/edit")

	r, _ = run(t, g, false, nil, "view", "production")
	assertContains(t, r.out, "reviewers:\toctocat,acme/ops\n", "wait-timer:\t90\n", "branch-patterns:\tbranch:main,tag:v*\n")

	_, b := run(t, g, false, nil, "view", "production", "--web")
	if len(b.opened) != 1 || !strings.HasSuffix(b.opened[0], "/environments/42/edit") {
		t.Errorf("opened %v", b.opened)
	}
}

func TestViewNotFound(t *testing.T) {
	g := newFakeGH(t)
	g.json("GET "+envBase+"nope", 404, `{"message":"Not Found"}`)
	r, _ := run(t, g, false, nil, "view", "nope")
	if r.code != 1 || strings.TrimSpace(r.errOut) != `environment "nope" not found in owner/repo` {
		t.Errorf("exit %d err %q", r.code, r.errOut)
	}
}

func TestViewPromptsForEnvironment(t *testing.T) {
	g := newFakeGH(t)
	g.json("GET /repos/owner/repo/environments", 200, listJSON(envJSON("a"), envJSON("b")))
	g.env("b")
	g.emptyDetails("b")
	r, _ := run(t, g, true, &stubPrompter{t: t, answers: []any{1}}, "view")
	if r.code != 0 || !strings.Contains(r.out, "b") {
		t.Fatalf("exit %d: %s %s", r.code, r.out, r.errOut)
	}
	r, _ = run(t, g, false, nil, "view")
	assertContains(t, r.errOut, "environment name required")
}

func TestEnvironmentNamesAreEscaped(t *testing.T) {
	g := newFakeGH(t)
	g.json("GET "+envBase+"feature%2Fone", 200, envJSON("feature/one"))
	g.emptyDetails("feature%2Fone")
	r, _ := run(t, g, false, nil, "view", "feature/one")
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.errOut)
	}
	g.one("GET", envBase+"feature%2Fone")
}

func TestCreatePlain(t *testing.T) {
	g := newFakeGH(t)
	g.json("GET "+envBase+"staging", 404, `{"message":"Not Found"}`)
	g.json("PUT "+envBase+"staging", 200, envJSON("staging"))

	r, _ := run(t, g, true, nil, "create", "staging")
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.errOut)
	}
	assertContains(t, r.out, "✓ Created environment staging in owner/repo")
	body := g.one("PUT", envBase+"staging").Body
	want := map[string]any{"can_admins_bypass": true, "deployment_branch_policy": nil}
	if !reflect.DeepEqual(body, want) {
		t.Errorf("PUT body = %v, want %v (plan-gated fields must be omitted)", body, want)
	}
}

func TestCreateWithProtection(t *testing.T) {
	g := newFakeGH(t)
	g.users()
	g.json("GET "+envBase+"production", 404, `{"message":"Not Found"}`)
	g.json("PUT "+envBase+"production", 200, envJSON("production"))
	g.json("GET "+envBase+"production/deployment-branch-policies", 200, `{"total_count":0,"branch_policies":[]}`)
	g.json("POST "+envBase+"production/deployment-branch-policies", 200, `{}`)

	r, _ := run(t, g, false, nil, "create", "production",
		"--reviewer", "octocat", "-r", "@acme/ops", "--prevent-self-review",
		"--wait-timer", "2h", "--branch", "main", "--tag", "v*", "--admin-bypass=false")
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.errOut)
	}
	if r.out != "" {
		t.Errorf("non-tty create printed %q", r.out)
	}
	body := g.one("PUT", envBase+"production").Body
	want := map[string]any{
		"can_admins_bypass":        false,
		"wait_timer":               float64(120),
		"prevent_self_review":      true,
		"reviewers":                []any{map[string]any{"type": "User", "id": float64(10)}, map[string]any{"type": "Team", "id": float64(20)}},
		"deployment_branch_policy": map[string]any{"protected_branches": false, "custom_branch_policies": true},
	}
	if !reflect.DeepEqual(body, want) {
		t.Errorf("PUT body =\n%v\nwant\n%v", body, want)
	}
	posts := g.find("POST", envBase+"production/deployment-branch-policies")
	if len(posts) != 2 || posts[0].Body["name"] != "main" || posts[1].Body["type"] != "tag" {
		t.Errorf("policies = %v", posts)
	}
}

func TestCreateRefusesExisting(t *testing.T) {
	g := newFakeGH(t)
	g.env("staging")
	r, _ := run(t, g, false, nil, "create", "staging")
	if r.code != 1 || !strings.Contains(r.errOut, "already exists") {
		t.Errorf("exit %d err %q", r.code, r.errOut)
	}
}

func TestCreateValidation(t *testing.T) {
	g := newFakeGH(t)
	g.json("GET "+envBase+"x", 404, `{"message":"Not Found"}`)
	cases := map[string][]string{
		"choose one of":                        {"--protected-branches", "--branch", "main"},
		"needs at least one required reviewer": {"--prevent-self-review"},
		"invalid wait timer":                   {"--wait-timer", "soon"},
		"between 0 and 43200":                  {"--wait-timer", "31d"},
	}
	for want, flags := range cases {
		r, _ := run(t, g, false, nil, append([]string{"create", "x"}, flags...)...)
		if r.code != 1 || !strings.Contains(r.errOut, want) {
			t.Errorf("%v: exit %d err %q, want %q", flags, r.code, r.errOut, want)
		}
	}
}

func TestBillingErrorIsExplained(t *testing.T) {
	g := newFakeGH(t)
	g.json("GET "+envBase+"prod", 404, `{"message":"Not Found"}`)
	g.json("PUT "+envBase+"prod", 422, `{"message":"Failed to create the environment protection rule. Please ensure the billing plan supports the wait timer protection rule."}`)
	r, _ := run(t, g, false, nil, "create", "prod", "--wait-timer", "5")
	assertContains(t, r.errOut, "billing plan", "GitHub Pro, Team or Enterprise")
}

func TestEditKeepsUnchangedSettings(t *testing.T) {
	g := newFakeGH(t)
	g.users()
	g.env("production", "wait=30", "reviewers", "protected")
	g.json("PUT "+envBase+"production", 200, envJSON("production"))

	r, _ := run(t, g, true, nil, "edit", "production", "--add-reviewer", "hubot", "--remove-reviewer", "acme/ops")
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.errOut)
	}
	assertContains(t, r.out, "Updated environment production")
	body := g.one("PUT", envBase+"production").Body
	want := map[string]any{
		"can_admins_bypass":        true,
		"wait_timer":               float64(30),
		"prevent_self_review":      true,
		"reviewers":                []any{map[string]any{"type": "User", "id": float64(10)}, map[string]any{"type": "User", "id": float64(11)}},
		"deployment_branch_policy": map[string]any{"protected_branches": true, "custom_branch_policies": false},
	}
	if !reflect.DeepEqual(body, want) {
		t.Errorf("PUT body =\n%v\nwant\n%v", body, want)
	}
}

func TestEditClearsProtection(t *testing.T) {
	g := newFakeGH(t)
	g.env("staging", "wait=30", "reviewers", "custom")
	g.json("PUT "+envBase+"staging", 200, envJSON("staging"))

	r, _ := run(t, g, false, nil, "edit", "staging", "--no-reviewers", "--wait-timer", "0", "--any-branch")
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.errOut)
	}
	body := g.one("PUT", envBase+"staging").Body
	want := map[string]any{
		"can_admins_bypass":        true,
		"wait_timer":               float64(0),
		"prevent_self_review":      false,
		"reviewers":                []any{},
		"deployment_branch_policy": nil,
	}
	if !reflect.DeepEqual(body, want) {
		t.Errorf("PUT body =\n%v\nwant\n%v", body, want)
	}
}

func TestEditSyncsPatterns(t *testing.T) {
	g := newFakeGH(t)
	g.env("production", "custom")
	g.json("PUT "+envBase+"production", 200, envJSON("production", "custom"))
	g.json("GET "+envBase+"production/deployment-branch-policies", 200, `{"total_count":2,"branch_policies":[{"id":7,"name":"main","type":"branch"},{"id":8,"name":"release/*","type":"branch"}]}`)
	g.json("DELETE "+envBase+"production/deployment-branch-policies/8", 204, ``)
	g.json("POST "+envBase+"production/deployment-branch-policies", 200, `{}`)

	r, _ := run(t, g, false, nil, "edit", "production", "--remove-branch", "release/*", "--add-tag", "v*", "--add-branch", "main")
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.errOut)
	}
	g.one("DELETE", envBase+"production/deployment-branch-policies/8")
	post := g.one("POST", envBase+"production/deployment-branch-policies")
	if post.Body["name"] != "v*" || post.Body["type"] != "tag" {
		t.Errorf("POST body %v", post.Body)
	}
}

func TestEditNeedsFlagsWhenNotInteractive(t *testing.T) {
	g := newFakeGH(t)
	g.env("staging")
	r, _ := run(t, g, false, nil, "edit", "staging")
	assertContains(t, r.errOut, "specify at least one flag")
}

func TestEditInteractive(t *testing.T) {
	g := newFakeGH(t)
	g.users()
	g.env("staging")
	g.json("PUT "+envBase+"staging", 200, envJSON("staging"))
	g.json("GET "+envBase+"staging/deployment-branch-policies", 200, `{"total_count":0,"branch_policies":[]}`)
	g.json("POST "+envBase+"staging/deployment-branch-policies", 200, `{}`)

	answers := []any{"octocat", true, "15m", false, 2, "main, release/*", ""}
	r, _ := run(t, g, true, &stubPrompter{t: t, answers: answers}, "edit", "staging")
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.errOut)
	}
	body := g.one("PUT", envBase+"staging").Body
	if body["wait_timer"] != float64(15) || body["prevent_self_review"] != true || body["can_admins_bypass"] != false {
		t.Errorf("PUT body %v", body)
	}
	if n := len(g.find("POST", envBase+"staging/deployment-branch-policies")); n != 2 {
		t.Errorf("created %d patterns, want 2", n)
	}
}

func TestDelete(t *testing.T) {
	g := newFakeGH(t)
	g.env("preview")
	g.json("DELETE "+envBase+"preview", 204, ``)

	r, _ := run(t, g, false, nil, "delete", "preview")
	assertContains(t, r.errOut, "`--yes` required")
	if len(g.find("DELETE", envBase+"preview")) != 0 {
		t.Fatal("deleted without confirmation")
	}

	r, _ = run(t, g, true, &stubPrompter{t: t, answers: []any{"previw"}}, "delete", "preview")
	if r.code != 1 || len(g.find("DELETE", envBase+"preview")) != 0 {
		t.Fatalf("mismatched confirmation deleted: exit %d", r.code)
	}

	r, _ = run(t, g, true, &stubPrompter{t: t, answers: []any{"preview"}}, "delete", "preview")
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.errOut)
	}
	assertContains(t, r.out, "Deleted environment preview")

	r, _ = run(t, g, false, nil, "delete", "preview", "--yes")
	if r.code != 0 || len(g.find("DELETE", envBase+"preview")) != 2 {
		t.Fatalf("exit %d: %s", r.code, r.errOut)
	}
}

func TestClone(t *testing.T) {
	g := newFakeGH(t)
	g.env("staging", "wait=10", "reviewers", "custom")
	g.json("GET "+envBase+"staging/deployment-branch-policies", 200, `{"total_count":1,"branch_policies":[{"id":7,"name":"main","type":"branch"}]}`)
	g.json("GET "+envBase+"staging/deployment_protection_rules", 200, `{"total_count":1,"custom_deployment_protection_rules":[{"id":5,"enabled":true,"app":{"id":99,"slug":"datadog"}}]}`)
	g.json("GET "+envBase+"staging/variables", 200, `{"total_count":1,"variables":[{"name":"REGION","value":"eu"}]}`)
	g.json("GET "+envBase+"staging/secrets", 200, `{"total_count":1,"secrets":[{"name":"TOKEN"}]}`)

	other := "/repos/acme/svc/environments/qa"
	g.json("GET "+other, 404, `{"message":"Not Found"}`)
	g.json("PUT "+other, 200, envJSON("qa"))
	g.json("GET "+other+"/deployment-branch-policies", 200, `{"total_count":0,"branch_policies":[]}`)
	g.json("POST "+other+"/deployment-branch-policies", 200, `{}`)
	g.json("GET "+other+"/deployment_protection_rules", 200, `{"total_count":0,"custom_deployment_protection_rules":[]}`)
	g.json("POST "+other+"/deployment_protection_rules", 200, `{}`)
	g.json("POST "+other+"/variables", 201, `{}`)

	r, _ := run(t, g, true, nil, "clone", "staging", "qa", "--to-repo", "acme/svc")
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.errOut)
	}
	put := g.one("PUT", other).Body
	if put["wait_timer"] != float64(10) || len(put["reviewers"].([]any)) != 2 {
		t.Errorf("PUT body %v", put)
	}
	if g.one("POST", other+"/deployment-branch-policies").Body["name"] != "main" {
		t.Error("pattern not copied")
	}
	if g.one("POST", other+"/deployment_protection_rules").Body["integration_id"] != float64(99) {
		t.Error("protection app not copied")
	}
	if g.one("POST", other+"/variables").Body["value"] != "eu" {
		t.Error("variable not copied")
	}
	assertContains(t, r.out, "Created environment qa in acme/svc from staging (1 variables)")
	assertContains(t, r.errOut, "gh secret set TOKEN --env qa -R acme/svc")
}

func TestCloneRefusesExisting(t *testing.T) {
	g := newFakeGH(t)
	g.env("staging")
	g.env("qa")
	r, _ := run(t, g, false, nil, "clone", "staging", "qa")
	assertContains(t, r.errOut, `"qa" already exists`)
}

func TestProtectionApps(t *testing.T) {
	g := newFakeGH(t)
	g.json("GET "+envBase+"production/deployment_protection_rules", 200, `{"total_count":1,"custom_deployment_protection_rules":[{"id":5,"enabled":true,"app":{"id":99,"slug":"datadog"}}]}`)
	g.json("GET "+envBase+"production/deployment_protection_rules/apps", 200, `{"total_count":2,"available_custom_deployment_protection_rule_integrations":[{"id":99,"slug":"datadog"},{"id":100,"slug":"honeycomb"}]}`)
	g.json("POST "+envBase+"production/deployment_protection_rules", 201, `{}`)
	g.json("DELETE "+envBase+"production/deployment_protection_rules/5", 204, ``)

	r, _ := run(t, g, false, nil, "protection", "list", "production", "--available")
	if r.out != "datadog\tenabled\t99\nhoneycomb\tavailable\t100\n" {
		t.Errorf("list = %q", r.out)
	}
	r, _ = run(t, g, false, nil, "protection", "enable", "production", "nope")
	assertContains(t, r.errOut, "available: datadog, honeycomb")
	r, _ = run(t, g, true, nil, "protection", "enable", "production", "honeycomb")
	assertContains(t, r.out, "Enabled honeycomb on production")
	if g.one("POST", envBase+"production/deployment_protection_rules").Body["integration_id"] != float64(100) {
		t.Error("wrong integration enabled")
	}
	r, _ = run(t, g, false, nil, "protection", "disable", "production", "datadog")
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.errOut)
	}
	g.one("DELETE", envBase+"production/deployment_protection_rules/5")
}

func TestWaitTimer(t *testing.T) {
	for in, want := range map[string]int{"30": 30, "45m": 45, "2h": 120, "1h30m": 90, "1d": 1440, "0": 0, "30d": 43200} {
		got, err := parseWait(in)
		if err != nil || got != want {
			t.Errorf("parseWait(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	for _, in := range []string{"soon", "30s", "-5", "31d"} {
		if _, err := parseWait(in); err == nil {
			t.Errorf("parseWait(%q) accepted", in)
		}
	}
	for in, want := range map[int]string{0: "none", 45: "45m", 60: "1h", 90: "1h30m", 2880: "2d"} {
		if got := formatWait(in); got != want {
			t.Errorf("formatWait(%d) = %q, want %q", in, got, want)
		}
	}
}
