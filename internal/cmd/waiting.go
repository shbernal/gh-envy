package cmd

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/cli/go-gh/v2/pkg/tableprinter"
	"github.com/spf13/cobra"

	"github.com/shbernal/gh-envy/internal/api"
	"github.com/shbernal/gh-envy/internal/cmdutil"
)

// waitingFields are the --json fields of waiting.
var waitingFields = []string{
	"actor", "branch", "canApprove", "createdAt", "environment", "environmentId",
	"event", "reviewers", "runId", "title", "url", "waitTimer",
	"waitTimerStartedAt", "workflow",
}

// gate is a run held at one environment.
type gate struct {
	Run     api.WorkflowRun
	Pending api.PendingDeployment
}

// waitingGates lists the gates of up to limit waiting runs, keeping only those
// at env when it is set.
func waitingGates(c *api.Client, workflow, branch, env string, limit int) ([]gate, error) {
	runs, err := c.ListWaitingRuns(workflow, branch, limit)
	if err != nil {
		return nil, err
	}
	var gates []gate
	for _, run := range runs {
		pending, err := c.PendingDeployments(run.ID)
		if err != nil {
			return nil, err
		}
		for _, p := range pending {
			if env == "" || strings.EqualFold(p.Environment.Name, env) {
				gates = append(gates, gate{Run: run, Pending: p})
			}
		}
	}
	return gates, nil
}

// remainingWait is the time left on a gate's wait timer, 0 when none runs.
// A timer that has not started yet counts in full.
func remainingWait(p api.PendingDeployment, now time.Time) time.Duration {
	if p.WaitTimer == 0 {
		return 0
	}
	total := time.Duration(p.WaitTimer) * time.Minute
	if p.WaitTimerStartedAt == nil {
		return total
	}
	return max(p.WaitTimerStartedAt.Add(total).Sub(now), 0)
}

// gateReasons explains why a run is held, from the current user's side:
// "you" can approve, other "reviewers" must, a "timer" is running, or an
// "app" (or another rule) holds it.
func gateReasons(p api.PendingDeployment, now time.Time) []string {
	var reasons []string
	if len(p.Reviewers) > 0 {
		if p.CurrentUserCanApprove {
			reasons = append(reasons, "you")
		} else {
			reasons = append(reasons, "reviewers")
		}
	}
	if left := remainingWait(p, now); left > 0 {
		reasons = append(reasons, "timer "+formatWait(int(math.Ceil(left.Minutes()))))
	}
	if len(reasons) == 0 {
		reasons = append(reasons, "app")
	}
	return reasons
}

func gateRecord(g gate) map[string]any {
	return map[string]any{
		"actor":              g.Run.TriggeringActor.Login,
		"branch":             g.Run.HeadBranch,
		"canApprove":         g.Pending.CurrentUserCanApprove,
		"createdAt":          g.Run.CreatedAt,
		"environment":        g.Pending.Environment.Name,
		"environmentId":      g.Pending.Environment.ID,
		"event":              g.Run.Event,
		"reviewers":          reviewerRecords(g.Pending.Reviewers),
		"runId":              g.Run.ID,
		"title":              g.Run.DisplayTitle,
		"url":                g.Run.HTMLURL,
		"waitTimer":          g.Pending.WaitTimer,
		"waitTimerStartedAt": g.Pending.WaitTimerStartedAt,
		"workflow":           g.Run.Name,
	}
}

func newWaitingCmd(f *cmdutil.Factory) *cobra.Command {
	var workflow, branch string
	var limit int
	var exporter *cmdutil.Exporter
	cmd := &cobra.Command{
		Use:   "waiting [<environment>]",
		Short: "List runs waiting at environment gates",
		Long: `List workflow runs held at an environment gate: required reviewers, a wait
timer or a custom protection app. A run waiting on several environments is
listed once per environment; pass an environment to show only its gates.

The GATE column says what holds the run: "you" when you can approve it,
"reviewers" when others must, "timer" with the time left, and "app" when a
protection app or another rule does.

--limit counts runs, not rows, so the output can hold more rows than the limit.
Each run costs one extra API request.`,
		Example: `  $ gh envy waiting
  $ gh envy waiting production -w deploy.yml
  $ gh envy waiting --json runId,environment,canApprove --jq '.[] | select(.canApprove) | .runId'`,
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeEnvs(f, 1),
		RunE: func(_ *cobra.Command, args []string) error {
			if limit < 1 {
				return cmdutil.FlagError{Err: fmt.Errorf("invalid limit: %d", limit)}
			}
			c, err := f.Client()
			if err != nil {
				return err
			}
			env := ""
			if len(args) > 0 {
				env = args[0]
			}
			gates, err := waitingGates(c, workflow, branch, env, limit)
			if err != nil {
				return err
			}
			if exporter != nil {
				rows := make([]map[string]any, len(gates))
				for i, g := range gates {
					rows[i] = exporter.Pick(gateRecord(g))
				}
				return exporter.Write(f.IO, rows)
			}
			if len(gates) == 0 {
				if f.IO.StdoutTTY {
					fmt.Fprintf(f.IO.ErrOut, "No runs waiting in %s\n", c.FullName())
				}
				return nil
			}
			return printWaiting(f.IO, gates)
		},
	}
	cmd.Flags().StringVarP(&workflow, "workflow", "w", "", "Filter by workflow file name, ID or name")
	cmd.Flags().StringVarP(&branch, "branch", "b", "", "Filter by branch")
	cmd.Flags().IntVarP(&limit, "limit", "L", 30, "Maximum number of runs to fetch")
	cmdutil.AddJSONFlags(cmd, &exporter, waitingFields)
	return cmd
}

func printWaiting(io *cmdutil.IO, gates []gate) error {
	now := io.Now()
	t := tableprinter.New(io.Out, io.StdoutTTY, io.Width)
	t.AddHeader([]string{"ENVIRONMENT", "WORKFLOW", "TITLE", "BRANCH", "RUN ID", "WAITING", "GATE"})
	for _, g := range gates {
		reasons := gateReasons(g.Pending, now)
		t.AddField(g.Pending.Environment.Name, tableprinter.WithColor(io.Bold))
		t.AddField(g.Run.Name)
		t.AddField(g.Run.DisplayTitle)
		t.AddField(g.Run.HeadBranch, tableprinter.WithColor(io.Gray))
		t.AddField(strconv.FormatInt(g.Run.ID, 10))
		if io.StdoutTTY {
			t.AddField(io.Ago(g.Run.CreatedAt), tableprinter.WithColor(io.Gray))
			color := io.Gray
			if reasons[0] == "you" {
				color = io.Green
			}
			t.AddField(strings.Join(reasons, ", "), tableprinter.WithColor(color))
		} else {
			t.AddField(g.Run.CreatedAt.Format(time.RFC3339))
			t.AddField(strings.Join(reasons, ","))
		}
		t.EndRow()
	}
	return t.Render()
}
