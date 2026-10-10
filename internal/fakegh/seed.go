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

	return &State{
		NextID: 1000,
		Now:    func() time.Time { return now },
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
