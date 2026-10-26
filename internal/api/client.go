// Package api wraps the GitHub REST endpoints for deployment environments.
package api

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	ghapi "github.com/cli/go-gh/v2/pkg/api"
	"github.com/cli/go-gh/v2/pkg/repository"
	"golang.org/x/crypto/nacl/box"
)

// ErrNotFound reports a missing environment.
var ErrNotFound = errors.New("not found")

const perPage = 100

// Client talks to the environment endpoints of one repository.
type Client struct {
	rest *ghapi.RESTClient
	Repo repository.Repository
}

// New returns a Client for repo. A nil transport uses gh's default HTTP stack.
func New(repo repository.Repository, opts ghapi.ClientOptions) (*Client, error) {
	if opts.Host == "" {
		opts.Host = repo.Host
	}
	rest, err := ghapi.NewRESTClient(opts)
	if err != nil {
		return nil, err
	}
	return &Client{rest: rest, Repo: repo}, nil
}

// ForRepo returns a client sharing the same transport for another repository.
func (c *Client) ForRepo(repo repository.Repository) *Client {
	return &Client{rest: c.rest, Repo: repo}
}

// FullName is OWNER/REPO.
func (c *Client) FullName() string {
	return c.Repo.Owner + "/" + c.Repo.Name
}

func (c *Client) repoPath(format string, args ...any) string {
	return fmt.Sprintf("repos/%s/%s/", url.PathEscape(c.Repo.Owner), url.PathEscape(c.Repo.Name)) + fmt.Sprintf(format, args...)
}

func (c *Client) envPath(env string, suffix string) string {
	return c.repoPath("environments/%s%s", url.PathEscape(env), suffix)
}

func (c *Client) do(method, path string, body any, out any) error {
	return translate(c.doRaw(method, path, body, out))
}

// doRaw is do without error translation.
func (c *Client) doRaw(method, path string, body any, out any) error {
	// url.PathEscape leaves "." and ".." intact; a server or proxy that
	// normalizes them would retarget the request, e.g. DELETE on the repository.
	for _, seg := range strings.Split(strings.SplitN(path, "?", 2)[0], "/") {
		if seg == "." || seg == ".." {
			return fmt.Errorf("invalid name %q", seg)
		}
	}
	var reader *bytes.Reader
	switch b := body.(type) {
	case nil:
	case []byte:
		reader = bytes.NewReader(b)
	default:
		encoded, err := json.Marshal(b)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(encoded)
	}
	if out == nil {
		out = &json.RawMessage{}
	}
	var err error
	if reader == nil {
		err = c.rest.Do(method, path, nil, out)
	} else {
		err = c.rest.Do(method, path, reader, out)
	}
	return err
}

// translate turns API errors into messages that say what to do.
func translate(err error) error {
	var httpErr *ghapi.HTTPError
	if !errors.As(err, &httpErr) {
		return err
	}
	msg := strings.ToLower(httpErr.Message)
	switch {
	case httpErr.StatusCode == http.StatusNotFound:
		return fmt.Errorf("%w: %s", ErrNotFound, httpErr.RequestURL.Path)
	case httpErr.StatusCode == http.StatusUnprocessableEntity && strings.Contains(msg, "billing plan"):
		return fmt.Errorf("%s\nRequired reviewers and wait timers on private repositories need GitHub Pro, Team or Enterprise", httpErr.Message)
	case httpErr.StatusCode == http.StatusForbidden || (httpErr.StatusCode == http.StatusUnauthorized):
		return fmt.Errorf("%s\nManaging environments needs admin access to the repository (token scope: repo)", httpErr.Message)
	}
	return err
}

// paginate fetches every page of a list endpoint. extract returns the items of
// one page decoded from raw JSON and how many there were.
func paginate[T any](c *Client, path string, limit int, field string) ([]T, error) {
	var all []T
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	for page := 1; ; page++ {
		var raw map[string]json.RawMessage
		if err := c.do(http.MethodGet, fmt.Sprintf("%s%sper_page=%d&page=%d", path, sep, perPage, page), nil, &raw); err != nil {
			return nil, err
		}
		var items []T
		if data, ok := raw[field]; ok {
			if err := json.Unmarshal(data, &items); err != nil {
				return nil, err
			}
		}
		all = append(all, items...)
		if limit > 0 && len(all) >= limit {
			return all[:limit], nil
		}
		if len(items) < perPage {
			return all, nil
		}
	}
}

// ListEnvironments returns up to limit environments, all when limit is 0.
func (c *Client) ListEnvironments(limit int) ([]Environment, error) {
	return paginate[Environment](c, c.repoPath("environments"), limit, "environments")
}

// GetEnvironment returns one environment.
func (c *Client) GetEnvironment(name string) (*Environment, error) {
	var env Environment
	if err := c.do(http.MethodGet, c.envPath(name, ""), nil, &env); err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, fmt.Errorf("environment %q %w in %s", name, ErrNotFound, c.FullName())
		}
		return nil, err
	}
	return &env, nil
}

// Exists reports whether the environment exists.
func (c *Client) Exists(name string) (bool, error) {
	_, err := c.GetEnvironment(name)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	return err == nil, err
}

// PutEnvironment creates or updates an environment. current is the state
// before the change, nil when creating.
func (c *Client) PutEnvironment(name string, desired Protection, current *Protection) (*Environment, error) {
	body, err := putBody(desired, current)
	if err != nil {
		return nil, err
	}
	var env Environment
	if err := c.do(http.MethodPut, c.envPath(name, ""), body, &env); err != nil {
		return nil, err
	}
	return &env, nil
}

// DeleteEnvironment deletes an environment.
func (c *Client) DeleteEnvironment(name string) error {
	err := c.do(http.MethodDelete, c.envPath(name, ""), nil, nil)
	if errors.Is(err, ErrNotFound) {
		return fmt.Errorf("environment %q %w in %s", name, ErrNotFound, c.FullName())
	}
	return err
}

// ListBranchPolicies returns the branch and tag patterns of a custom-policy environment.
func (c *Client) ListBranchPolicies(env string) ([]BranchPolicy, error) {
	return paginate[BranchPolicy](c, c.envPath(env, "/deployment-branch-policies"), 0, "branch_policies")
}

// CreateBranchPolicy adds a branch or tag pattern.
func (c *Client) CreateBranchPolicy(env, name, kind string) error {
	return c.do(http.MethodPost, c.envPath(env, "/deployment-branch-policies"), map[string]string{"name": name, "type": kind}, nil)
}

// DeleteBranchPolicy removes a pattern by ID.
func (c *Client) DeleteBranchPolicy(env string, id int64) error {
	return c.do(http.MethodDelete, c.envPath(env, fmt.Sprintf("/deployment-branch-policies/%d", id)), nil, nil)
}

// ListProtectionApps returns the custom deployment protection rules of an environment.
func (c *Client) ListProtectionApps(env string) ([]ProtectionApp, error) {
	var resp struct {
		Rules []ProtectionApp `json:"custom_deployment_protection_rules"`
	}
	if err := c.do(http.MethodGet, c.envPath(env, "/deployment_protection_rules"), nil, &resp); err != nil {
		return nil, err
	}
	return resp.Rules, nil
}

// ListAvailableApps returns the protection rule integrations installable on an environment.
func (c *Client) ListAvailableApps(env string) ([]AppRef, error) {
	return paginate[AppRef](c, c.envPath(env, "/deployment_protection_rules/apps"), 0, "available_custom_deployment_protection_rule_integrations")
}

// EnableProtectionApp enables a custom protection rule for an integration.
func (c *Client) EnableProtectionApp(env string, integrationID int64) error {
	return c.do(http.MethodPost, c.envPath(env, "/deployment_protection_rules"), map[string]int64{"integration_id": integrationID}, nil)
}

// DisableProtectionApp disables a custom protection rule by rule ID.
func (c *Client) DisableProtectionApp(env string, ruleID int64) error {
	return c.do(http.MethodDelete, c.envPath(env, fmt.Sprintf("/deployment_protection_rules/%d", ruleID)), nil, nil)
}

// ListVariables returns the environment's variables.
func (c *Client) ListVariables(env string) ([]Variable, error) {
	return paginate[Variable](c, c.envPath(env, "/variables"), 0, "variables")
}

// SetVariable creates a variable, or updates it when update is true.
func (c *Client) SetVariable(env, name, value string, update bool) error {
	body := map[string]string{"name": name, "value": value}
	if update {
		return c.do(http.MethodPatch, c.envPath(env, "/variables/"+url.PathEscape(name)), body, nil)
	}
	return c.do(http.MethodPost, c.envPath(env, "/variables"), body, nil)
}

// ListSecrets returns the environment's secret names.
func (c *Client) ListSecrets(env string) ([]Secret, error) {
	return paginate[Secret](c, c.envPath(env, "/secrets"), 0, "secrets")
}

// SetSecret creates or updates a secret, sealed with the environment's public key.
func (c *Client) SetSecret(env, name, value string) error {
	var pk struct {
		KeyID string `json:"key_id"`
		Key   string `json:"key"`
	}
	if err := c.do(http.MethodGet, c.envPath(env, "/secrets/public-key"), nil, &pk); err != nil {
		return err
	}
	raw, err := base64.StdEncoding.DecodeString(pk.Key)
	if err != nil || len(raw) != 32 {
		return fmt.Errorf("invalid public key for environment %q", env)
	}
	sealed, err := box.SealAnonymous(nil, []byte(value), (*[32]byte)(raw), rand.Reader)
	if err != nil {
		return err
	}
	body := map[string]string{"encrypted_value": base64.StdEncoding.EncodeToString(sealed), "key_id": pk.KeyID}
	return c.do(http.MethodPut, c.envPath(env, "/secrets/"+url.PathEscape(name)), body, nil)
}

// ListWaitingRuns returns up to limit runs held at an environment gate, all
// when limit is 0. workflow is a file name, an ID or a display name; branch
// filters by head branch. Both are optional.
func (c *Client) ListWaitingRuns(workflow, branch string, limit int) ([]WorkflowRun, error) {
	path := c.repoPath("actions/runs")
	if workflow != "" {
		id, err := c.resolveWorkflow(workflow)
		if err != nil {
			return nil, err
		}
		path = c.repoPath("actions/workflows/%s/runs", id)
	}
	q := url.Values{"status": {"waiting"}}
	if branch != "" {
		q.Set("branch", branch)
	}
	return paginate[WorkflowRun](c, path+"?"+q.Encode(), limit, "workflow_runs")
}

// resolveWorkflow returns the path segment the API accepts for a workflow:
// file names and IDs as given, display names looked up by name.
func (c *Client) resolveWorkflow(workflow string) (string, error) {
	if _, err := strconv.ParseInt(workflow, 10, 64); err == nil || strings.HasSuffix(workflow, ".yml") || strings.HasSuffix(workflow, ".yaml") {
		return url.PathEscape(workflow), nil
	}
	workflows, err := paginate[struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}](c, c.repoPath("actions/workflows"), 0, "workflows")
	if err != nil {
		return "", err
	}
	for _, w := range workflows {
		if strings.EqualFold(w.Name, workflow) {
			return strconv.FormatInt(w.ID, 10), nil
		}
	}
	return "", fmt.Errorf("could not find any workflows named %q in %s", workflow, c.FullName())
}

// GetRun returns one workflow run.
func (c *Client) GetRun(runID int64) (*WorkflowRun, error) {
	var run WorkflowRun
	if err := c.do(http.MethodGet, c.repoPath("actions/runs/%d", runID), nil, &run); err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, fmt.Errorf("run %d %w in %s", runID, ErrNotFound, c.FullName())
		}
		return nil, err
	}
	return &run, nil
}

// PendingDeployments returns the environment gates a run is waiting at.
func (c *Client) PendingDeployments(runID int64) ([]PendingDeployment, error) {
	var pending []PendingDeployment
	if err := c.do(http.MethodGet, c.repoPath("actions/runs/%d/pending_deployments", runID), nil, &pending); err != nil {
		return nil, err
	}
	return pending, nil
}

// ReviewDeployments approves or rejects a run's gates at the given
// environments. state is "approved" or "rejected".
func (c *Client) ReviewDeployments(runID int64, envIDs []int64, state, comment string) error {
	body := map[string]any{"environment_ids": envIDs, "state": state, "comment": comment}
	err := c.doRaw(http.MethodPost, c.repoPath("actions/runs/%d/pending_deployments", runID), body, nil)
	var httpErr *ghapi.HTTPError
	if errors.As(err, &httpErr) && (httpErr.StatusCode == http.StatusForbidden || httpErr.StatusCode == http.StatusUnprocessableEntity) {
		// GitHub answers a reviewer it does not accept with 422 and a string
		// "errors" field that go-gh cannot decode, leaving Message empty.
		msg := httpErr.Message
		if msg == "" {
			msg = "No pending deployment requests to approve or reject"
		}
		return fmt.Errorf("%s\nReviewing a deployment needs write access to the repository and a place among the environment's required reviewers", msg)
	}
	return translate(err)
}

// CurrentUser returns the login of the authenticated user.
func (c *Client) CurrentUser() (string, error) {
	var user Actor
	if err := c.do(http.MethodGet, "user", nil, &user); err != nil {
		return "", err
	}
	return user.Login, nil
}

// LastDeployment returns the latest deployment to env, nil when there is none.
func (c *Client) LastDeployment(env string) (*Deployment, error) {
	var deployments []Deployment
	q := url.Values{"environment": {env}, "per_page": {"1"}}
	if err := c.do(http.MethodGet, c.repoPath("deployments?%s", q.Encode()), nil, &deployments); err != nil {
		return nil, err
	}
	if len(deployments) == 0 {
		return nil, nil
	}
	d := &deployments[0]
	var statuses []struct {
		State          string `json:"state"`
		EnvironmentURL string `json:"environment_url"`
		LogURL         string `json:"log_url"`
	}
	if err := c.do(http.MethodGet, c.repoPath("deployments/%d/statuses?per_page=1", d.ID), nil, &statuses); err != nil {
		return nil, err
	}
	if len(statuses) > 0 {
		d.State = statuses[0].State
		d.URL = statuses[0].EnvironmentURL
		if d.URL == "" {
			d.URL = statuses[0].LogURL
		}
	}
	return d, nil
}

// ResolveReviewer turns "login" or "org/team-slug" into a reviewer reference.
func (c *Client) ResolveReviewer(name string) (ReviewerRef, error) {
	name = strings.TrimPrefix(strings.TrimSpace(name), "@")
	var resp struct {
		ID int64 `json:"id"`
	}
	if org, slug, ok := strings.Cut(name, "/"); ok {
		if err := c.do(http.MethodGet, fmt.Sprintf("orgs/%s/teams/%s", url.PathEscape(org), url.PathEscape(slug)), nil, &resp); err != nil {
			if errors.Is(err, ErrNotFound) {
				return ReviewerRef{}, fmt.Errorf("team %q not found or not visible to you", name)
			}
			return ReviewerRef{}, err
		}
		return ReviewerRef{Type: "Team", ID: resp.ID}, nil
	}
	if err := c.do(http.MethodGet, "users/"+url.PathEscape(name), nil, &resp); err != nil {
		if errors.Is(err, ErrNotFound) {
			return ReviewerRef{}, fmt.Errorf("user %q not found", name)
		}
		return ReviewerRef{}, err
	}
	return ReviewerRef{Type: "User", ID: resp.ID}, nil
}

// SettingsURL is the environment's settings page on the web.
func (c *Client) SettingsURL(env *Environment) string {
	return fmt.Sprintf("https://%s/%s/settings/environments/%d/edit", c.Repo.Host, c.FullName(), env.ID)
}
