// Package fakegh is an in-memory stand-in for the GitHub environment
// endpoints, used to record the README demos without touching real repositories.
package fakegh

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"sync"
	"time"
)

// Rule mirrors one protection_rules entry.
type Rule struct {
	ID                int64      `json:"id"`
	Type              string     `json:"type"`
	WaitTimer         int        `json:"wait_timer,omitempty"`
	PreventSelfReview bool       `json:"prevent_self_review,omitempty"`
	Reviewers         []Reviewer `json:"reviewers,omitempty"`
}

// Reviewer mirrors a required reviewer entry.
type Reviewer struct {
	Type     string         `json:"type"`
	Reviewer map[string]any `json:"reviewer"`
}

// Policy is a deployment branch policy.
type Policy struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

// App is a custom protection rule.
type App struct {
	ID      int64          `json:"id"`
	Enabled bool           `json:"enabled"`
	App     map[string]any `json:"app"`
}

// Var is an environment variable.
type Var struct {
	Name      string    `json:"name"`
	Value     string    `json:"value"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Secret is an environment secret name.
type Secret struct {
	Name      string    `json:"name"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Env is one environment's full state.
type Env struct {
	ID              int64           `json:"id"`
	Name            string          `json:"name"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
	CanAdminsBypass bool            `json:"can_admins_bypass"`
	WaitTimer       int             `json:"wait_timer"`
	PreventSelf     bool            `json:"prevent_self"`
	Reviewers       []Reviewer      `json:"reviewers"`
	BranchPolicy    *map[string]any `json:"branch_policy"`
	Policies        []Policy        `json:"policies"`
	Apps            []App           `json:"apps"`
	Vars            []Var           `json:"vars"`
	Secrets         []Secret        `json:"secrets"`
}

// Account is a user or team that can review.
type Account struct {
	ID   int64
	Type string
	Org  string
	Slug string
}

// State is the whole fake: repositories by OWNER/REPO, then environments in order.
type State struct {
	Repos  map[string][]*Env   `json:"repos"`
	NextID int64               `json:"next_id"`
	Apps   map[string][]string `json:"apps"` // installable protection apps by repo
	Users  map[string]Account  `json:"-"`
	Now    func() time.Time    `json:"-"`
	mu     sync.Mutex
}

func (s *State) id() int64 {
	s.NextID++
	return s.NextID
}

func (s *State) find(repo, name string) *Env {
	for _, e := range s.Repos[repo] {
		if e.Name == name {
			return e
		}
	}
	return nil
}

func (s *State) payload(e *Env) map[string]any {
	var rules []Rule
	if e.WaitTimer > 0 {
		rules = append(rules, Rule{ID: e.ID*10 + 1, Type: "wait_timer", WaitTimer: e.WaitTimer})
	}
	if len(e.Reviewers) > 0 {
		rules = append(rules, Rule{ID: e.ID*10 + 2, Type: "required_reviewers", PreventSelfReview: e.PreventSelf, Reviewers: e.Reviewers})
	}
	if e.BranchPolicy != nil {
		rules = append(rules, Rule{ID: e.ID*10 + 3, Type: "branch_policy"})
	}
	if rules == nil {
		rules = []Rule{}
	}
	var bp any
	if e.BranchPolicy != nil {
		bp = *e.BranchPolicy
	}
	return map[string]any{
		"id": e.ID, "name": e.Name, "created_at": e.CreatedAt, "updated_at": e.UpdatedAt,
		"can_admins_bypass": e.CanAdminsBypass, "protection_rules": rules, "deployment_branch_policy": bp,
	}
}

func (s *State) reviewer(typ string, id int64) (Reviewer, bool) {
	for login, a := range s.Users {
		if a.ID != id || a.Type != typ {
			continue
		}
		if typ == "Team" {
			return Reviewer{Type: typ, Reviewer: map[string]any{"id": id, "slug": a.Slug, "html_url": "https://github.com/orgs/" + a.Org + "/teams/" + a.Slug}}, true
		}
		return Reviewer{Type: typ, Reviewer: map[string]any{"id": id, "login": login}}, true
	}
	return Reviewer{}, false
}

func write(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

func notFound(w http.ResponseWriter) {
	write(w, http.StatusNotFound, map[string]string{"message": "Not Found"})
}

// Handler serves the fake API.
func (s *State) Handler() http.Handler {
	mux := http.NewServeMux()
	repo := func(r *http.Request) string { return r.PathValue("o") + "/" + r.PathValue("r") }
	withEnv := func(fn func(w http.ResponseWriter, r *http.Request, e *Env)) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			s.mu.Lock()
			defer s.mu.Unlock()
			e := s.find(repo(r), r.PathValue("env"))
			if e == nil {
				notFound(w)
				return
			}
			fn(w, r, e)
		}
	}
	const base = "/repos/{o}/{r}/environments"

	mux.HandleFunc("GET "+base, func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		out := []map[string]any{}
		if r.URL.Query().Get("page") == "1" || r.URL.Query().Get("page") == "" {
			for _, e := range s.Repos[repo(r)] {
				out = append(out, s.payload(e))
			}
		}
		write(w, 200, map[string]any{"total_count": len(out), "environments": out})
	})
	mux.HandleFunc("GET "+base+"/{env}", withEnv(func(w http.ResponseWriter, _ *http.Request, e *Env) {
		write(w, 200, s.payload(e))
	}))
	mux.HandleFunc("PUT "+base+"/{env}", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		var body map[string]json.RawMessage
		_ = json.NewDecoder(r.Body).Decode(&body)
		e := s.find(repo(r), r.PathValue("env"))
		if e == nil {
			e = &Env{ID: s.id(), Name: r.PathValue("env"), CreatedAt: s.Now(), CanAdminsBypass: true}
			s.Repos[repo(r)] = append(s.Repos[repo(r)], e)
		}
		if v, ok := body["wait_timer"]; ok {
			_ = json.Unmarshal(v, &e.WaitTimer)
		}
		if v, ok := body["prevent_self_review"]; ok {
			_ = json.Unmarshal(v, &e.PreventSelf)
		}
		if v, ok := body["can_admins_bypass"]; ok {
			_ = json.Unmarshal(v, &e.CanAdminsBypass)
		}
		if v, ok := body["reviewers"]; ok {
			var refs []struct {
				Type string `json:"type"`
				ID   int64  `json:"id"`
			}
			_ = json.Unmarshal(v, &refs)
			e.Reviewers = nil
			for _, ref := range refs {
				if rv, ok := s.reviewer(ref.Type, ref.ID); ok {
					e.Reviewers = append(e.Reviewers, rv)
				}
			}
		}
		if v, ok := body["deployment_branch_policy"]; ok {
			var bp *map[string]any
			_ = json.Unmarshal(v, &bp)
			if bp == nil || (*bp)["custom_branch_policies"] != true {
				e.Policies = nil
			}
			e.BranchPolicy = bp
		}
		e.UpdatedAt = s.Now()
		write(w, 200, s.payload(e))
	})
	mux.HandleFunc("DELETE "+base+"/{env}", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		envs := s.Repos[repo(r)]
		i := slices.IndexFunc(envs, func(e *Env) bool { return e.Name == r.PathValue("env") })
		if i < 0 {
			notFound(w)
			return
		}
		s.Repos[repo(r)] = slices.Delete(envs, i, i+1)
		write(w, 204, nil)
	})

	mux.HandleFunc("GET "+base+"/{env}/deployment-branch-policies", withEnv(func(w http.ResponseWriter, _ *http.Request, e *Env) {
		write(w, 200, map[string]any{"total_count": len(e.Policies), "branch_policies": nonNil(e.Policies)})
	}))
	mux.HandleFunc("POST "+base+"/{env}/deployment-branch-policies", withEnv(func(w http.ResponseWriter, r *http.Request, e *Env) {
		var p Policy
		_ = json.NewDecoder(r.Body).Decode(&p)
		if p.Type == "" {
			p.Type = "branch"
		}
		p.ID = s.id()
		e.Policies = append(e.Policies, p)
		write(w, 200, p)
	}))
	mux.HandleFunc("DELETE "+base+"/{env}/deployment-branch-policies/{id}", withEnv(func(w http.ResponseWriter, r *http.Request, e *Env) {
		id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
		e.Policies = slices.DeleteFunc(e.Policies, func(p Policy) bool { return p.ID == id })
		write(w, 204, nil)
	}))

	mux.HandleFunc("GET "+base+"/{env}/deployment_protection_rules", withEnv(func(w http.ResponseWriter, _ *http.Request, e *Env) {
		write(w, 200, map[string]any{"total_count": len(e.Apps), "custom_deployment_protection_rules": nonNil(e.Apps)})
	}))
	mux.HandleFunc("GET "+base+"/{env}/deployment_protection_rules/apps", withEnv(func(w http.ResponseWriter, r *http.Request, _ *Env) {
		var apps []map[string]any
		for i, slug := range s.Apps[repo(r)] {
			apps = append(apps, map[string]any{"id": 9000 + i, "slug": slug})
		}
		write(w, 200, map[string]any{"total_count": len(apps), "available_custom_deployment_protection_rule_integrations": nonNil(apps)})
	}))
	mux.HandleFunc("POST "+base+"/{env}/deployment_protection_rules", withEnv(func(w http.ResponseWriter, r *http.Request, e *Env) {
		var body struct {
			IntegrationID int64 `json:"integration_id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		i := int(body.IntegrationID - 9000)
		apps := s.Apps[repo(r)]
		if i < 0 || i >= len(apps) {
			write(w, 422, map[string]string{"message": "integration not installed"})
			return
		}
		app := App{ID: s.id(), Enabled: true, App: map[string]any{"id": body.IntegrationID, "slug": apps[i]}}
		e.Apps = append(e.Apps, app)
		write(w, 201, app)
	}))
	mux.HandleFunc("DELETE "+base+"/{env}/deployment_protection_rules/{id}", withEnv(func(w http.ResponseWriter, r *http.Request, e *Env) {
		id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
		e.Apps = slices.DeleteFunc(e.Apps, func(a App) bool { return a.ID == id })
		write(w, 204, nil)
	}))

	mux.HandleFunc("GET "+base+"/{env}/variables", withEnv(func(w http.ResponseWriter, _ *http.Request, e *Env) {
		write(w, 200, map[string]any{"total_count": len(e.Vars), "variables": nonNil(e.Vars)})
	}))
	mux.HandleFunc("POST "+base+"/{env}/variables", withEnv(func(w http.ResponseWriter, r *http.Request, e *Env) {
		var v Var
		_ = json.NewDecoder(r.Body).Decode(&v)
		v.UpdatedAt = s.Now()
		e.Vars = append(e.Vars, v)
		write(w, 201, map[string]any{})
	}))
	mux.HandleFunc("PATCH "+base+"/{env}/variables/{name}", withEnv(func(w http.ResponseWriter, r *http.Request, e *Env) {
		var v Var
		_ = json.NewDecoder(r.Body).Decode(&v)
		for i := range e.Vars {
			if e.Vars[i].Name == r.PathValue("name") {
				e.Vars[i].Value, e.Vars[i].UpdatedAt = v.Value, s.Now()
			}
		}
		write(w, 204, nil)
	}))
	mux.HandleFunc("GET "+base+"/{env}/secrets", withEnv(func(w http.ResponseWriter, _ *http.Request, e *Env) {
		write(w, 200, map[string]any{"total_count": len(e.Secrets), "secrets": nonNil(e.Secrets)})
	}))

	mux.HandleFunc("GET /users/{login}", func(w http.ResponseWriter, r *http.Request) {
		if a, ok := s.Users[r.PathValue("login")]; ok && a.Type == "User" {
			write(w, 200, map[string]any{"id": a.ID, "login": r.PathValue("login")})
			return
		}
		notFound(w)
	})
	mux.HandleFunc("GET /orgs/{org}/teams/{slug}", func(w http.ResponseWriter, r *http.Request) {
		if a, ok := s.Users[r.PathValue("org")+"/"+r.PathValue("slug")]; ok && a.Type == "Team" {
			write(w, 200, map[string]any{"id": a.ID, "slug": a.Slug})
			return
		}
		notFound(w)
	})
	return mux
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// Transport routes requests to h in-process.
type Transport struct{ H http.Handler }

// RoundTrip implements http.RoundTripper.
func (t Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	rec := httptest.NewRecorder()
	t.H.ServeHTTP(rec, req)
	resp := rec.Result()
	resp.Request = req
	return resp, nil
}
