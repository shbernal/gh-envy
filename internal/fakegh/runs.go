package fakegh

import (
	"encoding/json"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Run is a workflow run, waiting while it has gates.
type Run struct {
	ID        int64
	Workflow  string // display name
	Path      string // .github/workflows/FILE
	Title     string
	Branch    string
	Actor     string
	CreatedAt time.Time
	Gates     []Gate
}

// Gate is an environment a run is held at.
type Gate struct {
	Env          string
	CanApprove   bool
	TimerStarted *time.Time
}

// Deployment is a past deployment with its latest status.
type Deployment struct {
	ID        int64
	Env       string
	SHA       string
	Ref       string
	Creator   string
	CreatedAt time.Time
	State     string
	URL       string
}

func (s *State) runPayload(repo string, r *Run) map[string]any {
	return map[string]any{
		"id": r.ID, "name": r.Workflow, "display_title": r.Title, "head_branch": r.Branch, "event": "push",
		"path": r.Path, "html_url": "https://github.com/" + repo + "/actions/runs/" + strconv.FormatInt(r.ID, 10),
		"created_at": r.CreatedAt, "run_started_at": r.CreatedAt, "triggering_actor": map[string]any{"login": r.Actor},
	}
}

func (s *State) findRun(repo, id string) *Run {
	for _, r := range s.Runs[repo] {
		if strconv.FormatInt(r.ID, 10) == id {
			return r
		}
	}
	return nil
}

func (s *State) runHandlers(mux *http.ServeMux) {
	repo := func(r *http.Request) string { return r.PathValue("o") + "/" + r.PathValue("r") }
	const base = "/repos/{o}/{r}/actions"

	listRuns := func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		wf, q := r.PathValue("wf"), r.URL.Query()
		out := []map[string]any{}
		for _, run := range s.Runs[repo(r)] {
			if q.Get("page") > "1" || (q.Get("status") == "waiting" && len(run.Gates) == 0) ||
				(q.Get("branch") != "" && run.Branch != q.Get("branch")) || (wf != "" && !strings.HasSuffix(run.Path, "/"+wf)) {
				continue
			}
			out = append(out, s.runPayload(repo(r), run))
		}
		write(w, 200, map[string]any{"total_count": len(out), "workflow_runs": out})
	}
	mux.HandleFunc("GET "+base+"/runs", listRuns)
	mux.HandleFunc("GET "+base+"/workflows/{wf}/runs", listRuns)
	mux.HandleFunc("GET "+base+"/runs/{id}", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if run := s.findRun(repo(r), r.PathValue("id")); run != nil {
			write(w, 200, s.runPayload(repo(r), run))
			return
		}
		notFound(w)
	})
	mux.HandleFunc("GET "+base+"/runs/{id}/pending_deployments", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		run := s.findRun(repo(r), r.PathValue("id"))
		if run == nil {
			notFound(w)
			return
		}
		out := []map[string]any{}
		for _, g := range run.Gates {
			e := s.find(repo(r), g.Env)
			if e == nil {
				continue
			}
			wait := 0
			if g.TimerStarted != nil {
				wait = e.WaitTimer
			}
			out = append(out, map[string]any{
				"environment":              map[string]any{"id": e.ID, "name": e.Name},
				"wait_timer":               wait,
				"wait_timer_started_at":    g.TimerStarted,
				"current_user_can_approve": g.CanApprove,
				"reviewers":                nonNil(e.Reviewers),
			})
		}
		write(w, 200, out)
	})
	mux.HandleFunc("POST "+base+"/runs/{id}/pending_deployments", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		run := s.findRun(repo(r), r.PathValue("id"))
		if run == nil {
			notFound(w)
			return
		}
		var body struct {
			EnvironmentIDs []int64 `json:"environment_ids"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		run.Gates = slices.DeleteFunc(run.Gates, func(g Gate) bool {
			e := s.find(repo(r), g.Env)
			return e != nil && slices.Contains(body.EnvironmentIDs, e.ID)
		})
		write(w, 200, []any{})
	})

	mux.HandleFunc("GET /user", func(w http.ResponseWriter, _ *http.Request) {
		write(w, 200, map[string]any{"login": s.Login})
	})
	mux.HandleFunc("GET /repos/{o}/{r}/deployments", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		out := []map[string]any{}
		for _, d := range s.Deployments[repo(r)] {
			if env := r.URL.Query().Get("environment"); env != "" && d.Env != env {
				continue
			}
			out = append(out, map[string]any{
				"id": d.ID, "sha": d.SHA, "ref": d.Ref, "environment": d.Env,
				"creator": map[string]any{"login": d.Creator}, "created_at": d.CreatedAt,
			})
		}
		write(w, 200, out)
	})
	mux.HandleFunc("GET /repos/{o}/{r}/deployments/{id}/statuses", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		for _, d := range s.Deployments[repo(r)] {
			if strconv.FormatInt(d.ID, 10) == r.PathValue("id") {
				write(w, 200, []map[string]any{{"state": d.State, "environment_url": d.URL}})
				return
			}
		}
		write(w, 200, []any{})
	})
}
