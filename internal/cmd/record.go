package cmd

import (
	"github.com/shbernal/gh-envy/internal/api"
)

// environmentFields are the --json fields of list and view.
var environmentFields = []string{
	"adminBypass", "branchPolicies", "branchPolicy", "createdAt", "id", "name",
	"preventSelfReview", "protectionApps", "reviewers", "secrets", "updatedAt",
	"url", "variables", "waitTimer",
}

// details are the parts of an environment that need extra API calls.
type details struct {
	BranchPolicies []api.BranchPolicy
	ProtectionApps []api.ProtectionApp
	Variables      []api.Variable
	Secrets        []api.Secret
}

// fetchDetails loads the parts of env selected by want.
func fetchDetails(c *api.Client, env *api.Environment, want func(string) bool) (details, error) {
	var d details
	var err error
	if want("branchPolicies") && env.BranchPolicyMode() == "custom" {
		if d.BranchPolicies, err = c.ListBranchPolicies(env.Name); err != nil {
			return d, err
		}
	}
	if want("protectionApps") {
		if d.ProtectionApps, err = c.ListProtectionApps(env.Name); err != nil {
			return d, err
		}
	}
	if want("variables") {
		if d.Variables, err = c.ListVariables(env.Name); err != nil {
			return d, err
		}
	}
	if want("secrets") {
		if d.Secrets, err = c.ListSecrets(env.Name); err != nil {
			return d, err
		}
	}
	return d, nil
}

// record is the full JSON shape of an environment.
func record(c *api.Client, env *api.Environment, d details) map[string]any {
	entries, preventSelf := env.Reviewers()
	reviewers := make([]map[string]any, len(entries))
	for i, r := range entries {
		reviewers[i] = map[string]any{"type": r.Type, "name": r.Name(), "id": r.Reviewer.ID}
	}
	policies := make([]map[string]any, len(d.BranchPolicies))
	for i, p := range d.BranchPolicies {
		policies[i] = map[string]any{"name": p.Name, "type": p.Type, "id": p.ID}
	}
	apps := make([]map[string]any, len(d.ProtectionApps))
	for i, a := range d.ProtectionApps {
		apps[i] = map[string]any{"id": a.ID, "slug": a.App.Slug, "appId": a.App.ID, "enabled": a.Enabled}
	}
	vars := make([]map[string]any, len(d.Variables))
	for i, v := range d.Variables {
		vars[i] = map[string]any{"name": v.Name, "value": v.Value, "updatedAt": v.UpdatedAt}
	}
	secrets := make([]map[string]any, len(d.Secrets))
	for i, s := range d.Secrets {
		secrets[i] = map[string]any{"name": s.Name, "updatedAt": s.UpdatedAt}
	}
	return map[string]any{
		"adminBypass":       env.CanAdminsBypass,
		"branchPolicies":    policies,
		"branchPolicy":      env.BranchPolicyMode(),
		"createdAt":         env.CreatedAt,
		"id":                env.ID,
		"name":              env.Name,
		"preventSelfReview": preventSelf,
		"protectionApps":    apps,
		"reviewers":         reviewers,
		"secrets":           secrets,
		"updatedAt":         env.UpdatedAt,
		"url":               c.SettingsURL(env),
		"variables":         vars,
		"waitTimer":         env.WaitTimer(),
	}
}
