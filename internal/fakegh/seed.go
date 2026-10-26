package fakegh

import "time"

// DemoRepo is the repository the demo seed populates.
const DemoRepo = "acme/shop"

// Seed returns acme/shop with a realistic set of environments: an open
// development env, a gated staging, a reviewed production and two previews.
func Seed(now time.Time) *State {
	ago := func(d time.Duration) time.Time { return now.Add(-d) }
	day := 24 * time.Hour
	user := func(login string, id int64) Reviewer {
		return Reviewer{Type: "User", Reviewer: map[string]any{"id": id, "login": login}}
	}
	team := func(slug string, id int64) Reviewer {
		return Reviewer{Type: "Team", Reviewer: map[string]any{"id": id, "slug": slug, "html_url": "https://github.com/orgs/acme/teams/" + slug}}
	}
	custom := &map[string]any{"protected_branches": false, "custom_branch_policies": true}
	protected := &map[string]any{"protected_branches": true, "custom_branch_policies": false}

	timerStarted := ago(2 * time.Minute)

	return &State{
		NextID: 1000,
		Login:  "octocat",
		Runs: map[string][]*Run{DemoRepo: {
			{
				ID: 7731042, Workflow: "Deploy", Path: ".github/workflows/deploy.yml", Title: "Release v2.4.0",
				Branch: "main", Actor: "hubot", CreatedAt: ago(14 * time.Minute),
				Gates: []Gate{{Env: "production", CanApprove: true}},
			},
			{
				ID: 7730988, Workflow: "Migrate", Path: ".github/workflows/migrate.yml", Title: "Add orders index",
				Branch: "main", Actor: "monalisa", CreatedAt: ago(3 * time.Minute),
				Gates: []Gate{{Env: "staging", TimerStarted: &timerStarted}},
			},
		}},
		Deployments: map[string][]*Deployment{DemoRepo: {
			{ID: 5001, Env: "production", SHA: "4e1c9b2a7d03", Ref: "v2.3.1", Creator: "octocat", CreatedAt: ago(2 * day), State: "success", URL: "https://shop.acme.com"},
			{ID: 5000, Env: "staging", SHA: "b82f05e6c1a9", Ref: "main", Creator: "hubot", CreatedAt: ago(3 * time.Hour), State: "success", URL: "https://staging.shop.acme.com"},
		}},
		Now: func() time.Time { return now },
		Users: map[string]Account{
			"octocat":               {ID: 1, Type: "User"},
			"hubot":                 {ID: 2, Type: "User"},
			"monalisa":              {ID: 3, Type: "User"},
			"acme/release-managers": {ID: 50, Type: "Team", Org: "acme", Slug: "release-managers"},
			"acme/sre":              {ID: 51, Type: "Team", Org: "acme", Slug: "sre"},
		},
		Apps: map[string][]string{DemoRepo: {"datadog-deploy-gate", "honeycomb-slo-check"}},
		Repos: map[string][]*Env{DemoRepo: {
			{
				ID: 101, Name: "development", CreatedAt: ago(200 * day), UpdatedAt: ago(40 * day), CanAdminsBypass: true,
				Vars: []Var{{Name: "API_URL", Value: "https://api.dev.acme.test", UpdatedAt: ago(40 * day)}},
			},
			{
				ID: 102, Name: "staging", CreatedAt: ago(200 * day), UpdatedAt: ago(3 * day), CanAdminsBypass: true,
				WaitTimer: 5, BranchPolicy: custom,
				Policies: []Policy{{ID: 201, Name: "main", Type: "branch"}, {ID: 202, Name: "release/*", Type: "branch"}},
				Vars: []Var{
					{Name: "API_URL", Value: "https://api.staging.acme.test", UpdatedAt: ago(3 * day)},
					{Name: "REGION", Value: "eu-west-1", UpdatedAt: ago(30 * day)},
				},
				Secrets: []Secret{{Name: "DATABASE_URL", UpdatedAt: ago(12 * day)}, {Name: "STRIPE_KEY", UpdatedAt: ago(60 * day)}},
			},
			{
				ID: 103, Name: "production", CreatedAt: ago(200 * day), UpdatedAt: ago(1 * day), CanAdminsBypass: false,
				WaitTimer: 30, PreventSelf: true,
				Reviewers:    []Reviewer{user("octocat", 1), team("release-managers", 50)},
				BranchPolicy: custom,
				Policies:     []Policy{{ID: 203, Name: "main", Type: "branch"}, {ID: 204, Name: "v*", Type: "tag"}},
				Apps:         []App{{ID: 301, Enabled: true, App: map[string]any{"id": 9000, "slug": "datadog-deploy-gate"}}},
				Vars: []Var{
					{Name: "API_URL", Value: "https://api.acme.com", UpdatedAt: ago(90 * day)},
					{Name: "REGION", Value: "eu-west-1", UpdatedAt: ago(90 * day)},
				},
				Secrets: []Secret{{Name: "DATABASE_URL", UpdatedAt: ago(5 * day)}, {Name: "STRIPE_KEY", UpdatedAt: ago(60 * day)}},
			},
			{ID: 104, Name: "preview/pr-128", CreatedAt: ago(9 * day), UpdatedAt: ago(9 * day), CanAdminsBypass: true, BranchPolicy: protected},
			{ID: 105, Name: "preview/pr-131", CreatedAt: ago(2 * day), UpdatedAt: ago(2 * day), CanAdminsBypass: true, BranchPolicy: protected},
		}},
	}
}
