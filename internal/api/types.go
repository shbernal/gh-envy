package api

import (
	"encoding/json"
	"strings"
	"time"
)

// Environment is a deployment environment as returned by the REST API.
type Environment struct {
	ID                     int64                   `json:"id"`
	Name                   string                  `json:"name"`
	HTMLURL                string                  `json:"html_url"`
	CreatedAt              time.Time               `json:"created_at"`
	UpdatedAt              time.Time               `json:"updated_at"`
	CanAdminsBypass        bool                    `json:"can_admins_bypass"`
	ProtectionRules        []ProtectionRule        `json:"protection_rules"`
	DeploymentBranchPolicy *DeploymentBranchPolicy `json:"deployment_branch_policy"`
}

// ProtectionRule is one entry of an environment's protection_rules. Its shape
// depends on Type: wait_timer, required_reviewers or branch_policy.
type ProtectionRule struct {
	ID                int64           `json:"id"`
	Type              string          `json:"type"`
	WaitTimer         int             `json:"wait_timer"`
	PreventSelfReview bool            `json:"prevent_self_review"`
	Reviewers         []ReviewerEntry `json:"reviewers"`
}

// ReviewerEntry is a required reviewer, either a User or a Team.
type ReviewerEntry struct {
	Type     string   `json:"type"`
	Reviewer Reviewer `json:"reviewer"`
}

// Reviewer carries the fields shared by the user and team payloads.
type Reviewer struct {
	ID      int64  `json:"id"`
	Login   string `json:"login"`
	Slug    string `json:"slug"`
	HTMLURL string `json:"html_url"`
}

// DeploymentBranchPolicy selects which refs may deploy. A nil policy allows all.
type DeploymentBranchPolicy struct {
	ProtectedBranches    bool `json:"protected_branches"`
	CustomBranchPolicies bool `json:"custom_branch_policies"`
}

// BranchPolicy is a name pattern allowed to deploy to a custom-policy environment.
type BranchPolicy struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

// ProtectionApp is a custom deployment protection rule backed by a GitHub App.
type ProtectionApp struct {
	ID      int64  `json:"id"`
	Enabled bool   `json:"enabled"`
	App     AppRef `json:"app"`
}

// AppRef identifies a GitHub App integration.
type AppRef struct {
	ID             int64  `json:"id"`
	Slug           string `json:"slug"`
	IntegrationURL string `json:"integration_url"`
}

// Variable is an environment variable.
type Variable struct {
	Name      string    `json:"name"`
	Value     string    `json:"value"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Secret is an environment secret. Values are never readable.
type Secret struct {
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Actor is the login of a user attached to a run or deployment.
type Actor struct {
	Login string `json:"login"`
}

// WorkflowRun is an Actions workflow run.
type WorkflowRun struct {
	ID              int64     `json:"id"`
	Name            string    `json:"name"`
	DisplayTitle    string    `json:"display_title"`
	HeadBranch      string    `json:"head_branch"`
	Event           string    `json:"event"`
	HTMLURL         string    `json:"html_url"`
	CreatedAt       time.Time `json:"created_at"`
	RunStartedAt    time.Time `json:"run_started_at"`
	TriggeringActor Actor     `json:"triggering_actor"`
	Path            string    `json:"path"`
}

// PendingDeployment is an environment gate a waiting run is held at.
type PendingDeployment struct {
	Environment struct {
		ID      int64  `json:"id"`
		Name    string `json:"name"`
		HTMLURL string `json:"html_url"`
	} `json:"environment"`
	WaitTimer             int             `json:"wait_timer"`
	WaitTimerStartedAt    *time.Time      `json:"wait_timer_started_at"`
	CurrentUserCanApprove bool            `json:"current_user_can_approve"`
	Reviewers             []ReviewerEntry `json:"reviewers"`
}

// Deployment is a deployment to an environment with the state of its latest status.
type Deployment struct {
	ID        int64     `json:"id"`
	SHA       string    `json:"sha"`
	Ref       string    `json:"ref"`
	Creator   Actor     `json:"creator"`
	CreatedAt time.Time `json:"created_at"`
	// State and URL come from the latest deployment status; State is empty
	// when the deployment has no status yet.
	State string `json:"-"`
	URL   string `json:"-"`
}

// ReviewerRef is a reviewer in the shape the PUT endpoint expects.
type ReviewerRef struct {
	Type string `json:"type"`
	ID   int64  `json:"id"`
}

// Protection is the desired protection state of an environment.
type Protection struct {
	WaitTimer         int
	PreventSelfReview bool
	Reviewers         []ReviewerRef
	CanAdminsBypass   bool
	BranchPolicy      *DeploymentBranchPolicy
}

// WaitTimer returns the wait timer in minutes, 0 when unset.
func (e *Environment) WaitTimer() int {
	for _, r := range e.ProtectionRules {
		if r.Type == "wait_timer" {
			return r.WaitTimer
		}
	}
	return 0
}

// Reviewers returns the required reviewers and the prevent-self-review flag.
func (e *Environment) Reviewers() ([]ReviewerEntry, bool) {
	for _, r := range e.ProtectionRules {
		if r.Type == "required_reviewers" {
			return r.Reviewers, r.PreventSelfReview
		}
	}
	return nil, false
}

// BranchPolicyMode is "all", "protected" or "custom".
func (e *Environment) BranchPolicyMode() string {
	return BranchPolicyMode(e.DeploymentBranchPolicy)
}

// BranchPolicyMode names a deployment branch policy setting.
func BranchPolicyMode(p *DeploymentBranchPolicy) string {
	switch {
	case p == nil:
		return "all"
	case p.ProtectedBranches:
		return "protected"
	default:
		return "custom"
	}
}

// Protection returns the environment's current protection state.
func (e *Environment) Protection() Protection {
	entries, preventSelf := e.Reviewers()
	refs := make([]ReviewerRef, 0, len(entries))
	for _, r := range entries {
		refs = append(refs, ReviewerRef{Type: r.Type, ID: r.Reviewer.ID})
	}
	var bp *DeploymentBranchPolicy
	if e.DeploymentBranchPolicy != nil {
		copied := *e.DeploymentBranchPolicy
		bp = &copied
	}
	return Protection{
		WaitTimer:         e.WaitTimer(),
		PreventSelfReview: preventSelf,
		Reviewers:         refs,
		CanAdminsBypass:   e.CanAdminsBypass,
		BranchPolicy:      bp,
	}
}

// Name renders a reviewer as a login for users and org/slug for teams.
func (r ReviewerEntry) Name() string {
	if r.Type == "Team" {
		if org := teamOrg(r.Reviewer.HTMLURL); org != "" {
			return org + "/" + r.Reviewer.Slug
		}
		return r.Reviewer.Slug
	}
	return r.Reviewer.Login
}

// teamOrg extracts ORG from https://HOST/orgs/ORG/teams/SLUG.
func teamOrg(htmlURL string) string {
	parts := strings.Split(htmlURL, "/")
	for i, p := range parts {
		if p == "orgs" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

// putBody builds the PUT payload. Plan-gated fields (wait timer, reviewers) are
// sent only when they are set or are being cleared, because sending them at all
// fails with 422 on plans without protection rules.
func putBody(desired Protection, current *Protection) ([]byte, error) {
	body := map[string]any{
		"can_admins_bypass":        desired.CanAdminsBypass,
		"deployment_branch_policy": desired.BranchPolicy,
	}
	curWait, curReviewers, curSelf := 0, 0, false
	if current != nil {
		curWait, curReviewers, curSelf = current.WaitTimer, len(current.Reviewers), current.PreventSelfReview
	}
	if desired.WaitTimer != 0 || curWait != 0 {
		body["wait_timer"] = desired.WaitTimer
	}
	if len(desired.Reviewers) > 0 || curReviewers > 0 {
		reviewers := desired.Reviewers
		if reviewers == nil {
			reviewers = []ReviewerRef{}
		}
		body["reviewers"] = reviewers
	}
	if desired.PreventSelfReview || curSelf {
		body["prevent_self_review"] = desired.PreventSelfReview
	}
	return json.Marshal(body)
}
