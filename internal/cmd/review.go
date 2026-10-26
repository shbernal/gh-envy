package cmd

import (
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"strconv"
	"strings"

	"github.com/cli/go-gh/v2/pkg/repository"
	"github.com/spf13/cobra"

	"github.com/shbernal/gh-envy/internal/api"
	"github.com/shbernal/gh-envy/internal/cmdutil"
)

func newReviewCmd(f *cmdutil.Factory, state string) *cobra.Command {
	var envs []string
	var workflow, branch, comment string
	var watch bool
	approve := state == "approved"
	use, short, long := "reject", "Reject a deployment waiting for review",
		`Reject the deployment of a workflow run held at an environment's required
reviewers. The run fails.`
	example := `  $ gh envy reject 123456789 -c "not during the freeze"
  $ gh envy reject -e production -w deploy.yml`
	if approve {
		use, short, long = "approve", "Approve a deployment waiting for review",
			`Approve the deployment of a workflow run held at an environment's required
reviewers.`
		example = `  $ gh envy approve
  $ gh envy approve 123456789 -e production -c "ship it"
  $ gh envy approve https://github.com/OWNER/REPO/actions/runs/123456789
  $ gh envy approve -w deploy.yml -b main --watch`
	}
	long += `

The run is a run ID or URL. Without one, picks among the runs waiting for
review, filtered by --env, --workflow and --branch. Without --env, acts on every
environment of the run you can review, or asks which ones in a terminal.`
	cmd := &cobra.Command{
		Use:     use + " [<run-id> | <run-url>]",
		Short:   short,
		Long:    long,
		Example: example,
		Args:    cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if watch && !f.IO.CanPrompt() {
				return cmdutil.FlagError{Err: errors.New("`--watch` requires a terminal")}
			}
			c, err := f.Client()
			if err != nil {
				return err
			}
			var run *api.WorkflowRun
			var pending []api.PendingDeployment
			repoArg := f.RepoFlag != ""
			if len(args) > 0 {
				repo, id, err := parseRunRef(args[0])
				if err != nil {
					return cmdutil.FlagError{Err: err}
				}
				if repo != nil {
					c, repoArg = c.ForRepo(*repo), true
				}
				if run, err = c.GetRun(id); err != nil {
					return err
				}
				if pending, err = c.PendingDeployments(id); err != nil {
					return err
				}
				if len(pending) == 0 {
					return fmt.Errorf("run %d is not waiting for review", id)
				}
			} else if run, pending, err = pickWaitingRun(f, c, envs, workflow, branch); err != nil {
				return err
			}

			targets, err := narrowEnvs(run, pending, envs)
			if err != nil {
				return err
			}
			var allowed, blocked []api.PendingDeployment
			for _, p := range targets {
				if p.CurrentUserCanApprove {
					allowed = append(allowed, p)
				} else {
					blocked = append(blocked, p)
				}
			}
			reasons := make([]string, len(blocked))
			for i, p := range blocked {
				if reasons[i], err = blockedReason(c, run, p); err != nil {
					return err
				}
			}
			if len(allowed) == 0 {
				return errors.New(strings.Join(reasons, "\n"))
			}
			for i, r := range reasons {
				fmt.Fprintf(f.IO.ErrOut, "%s Skipping %s: %s\n", f.IO.WarningIcon(), blocked[i].Environment.Name, r)
			}

			if len(envs) == 0 && len(allowed) > 1 && f.IO.CanPrompt() {
				names := make([]string, len(allowed))
				for i, p := range allowed {
					names[i] = p.Environment.Name
				}
				picked, err := f.Prompter.MultiSelect("Environments to "+use, names, names)
				if err != nil || len(picked) == 0 {
					return cmdutil.ErrCancel
				}
				chosen := make([]api.PendingDeployment, len(picked))
				for i, j := range picked {
					chosen[i] = allowed[j]
				}
				allowed = chosen
			}

			ids := make([]int64, len(allowed))
			names := make([]string, len(allowed))
			for i, p := range allowed {
				ids[i], names[i] = p.Environment.ID, p.Environment.Name
			}
			if err := c.ReviewDeployments(run.ID, ids, state, comment); err != nil {
				return err
			}

			watchArgs := []string{"run", "watch", strconv.FormatInt(run.ID, 10)}
			if repoArg {
				watchArgs = append(watchArgs, "-R", repoName(c.Repo))
			}
			if f.IO.StdoutTTY {
				icon, verb := f.IO.SuccessIcon(), "Approved"
				if !approve {
					icon, verb = f.IO.Red("✗"), "Rejected"
				}
				fmt.Fprintf(f.IO.Out, "%s %s %s for run %d %s\n", icon, verb, f.IO.Bold(strings.Join(names, ", ")), run.ID,
					f.IO.Gray(fmt.Sprintf("(%s, %s)", run.Name, run.HeadBranch)))
				if approve && !watch {
					fmt.Fprintln(f.IO.Out, f.IO.Gray("gh "+strings.Join(watchArgs, " ")))
				}
			}
			if watch {
				if err := f.Exec(watchArgs...); err != nil {
					var exitErr *exec.ExitError
					if errors.As(err, &exitErr) {
						return cmdutil.ErrSilent
					}
					return err
				}
			}
			return nil
		},
	}
	cmd.Flags().StringSliceVarP(&envs, "env", "e", nil, "Review only this `environment` (repeatable)")
	cmd.Flags().StringVarP(&workflow, "workflow", "w", "", "Pick among runs of this workflow file name, ID or name")
	cmd.Flags().StringVarP(&branch, "branch", "b", "", "Pick among runs on this branch")
	cmd.Flags().StringVarP(&comment, "comment", "c", "", "Comment recorded with the review")
	_ = cmd.RegisterFlagCompletionFunc("env", completeEnvs(f, -1))
	if approve {
		cmd.Flags().BoolVar(&watch, "watch", false, "Watch the run with \"gh run watch\" after approving")
	}
	return cmd
}

// pickWaitingRun finds the waiting run to review among those matching the filters.
func pickWaitingRun(f *cmdutil.Factory, c *api.Client, envs []string, workflow, branch string) (*api.WorkflowRun, []api.PendingDeployment, error) {
	gates, err := waitingGates(c, workflow, branch, "", 30)
	if err != nil {
		return nil, nil, err
	}
	var runs []api.WorkflowRun
	pending := map[int64][]api.PendingDeployment{}
	for _, g := range gates {
		if len(envs) > 0 && !slices.ContainsFunc(envs, func(e string) bool { return strings.EqualFold(e, g.Pending.Environment.Name) }) {
			continue
		}
		if _, seen := pending[g.Run.ID]; !seen {
			runs = append(runs, g.Run)
		}
		pending[g.Run.ID] = append(pending[g.Run.ID], g.Pending)
	}
	switch {
	case len(runs) == 0:
		return nil, nil, errors.New("no runs waiting for review")
	case len(runs) == 1:
		return &runs[0], pending[runs[0].ID], nil
	case !f.IO.CanPrompt():
		lines := make([]string, len(runs))
		for i, r := range runs {
			lines[i] = fmt.Sprintf("  %d  %s  %s", r.ID, r.Name, r.HeadBranch)
		}
		return nil, nil, cmdutil.FlagError{Err: fmt.Errorf("%d runs are waiting for review; pass a run ID, or narrow with --workflow, --branch or --env:\n%s",
			len(runs), strings.Join(lines, "\n"))}
	}
	labels := make([]string, len(runs))
	for i, r := range runs {
		var names []string
		for _, p := range pending[r.ID] {
			names = append(names, p.Environment.Name)
		}
		labels[i] = fmt.Sprintf("%d · %s · %s · %s (%s)", r.ID, r.Name, r.HeadBranch, r.DisplayTitle, strings.Join(names, ", "))
	}
	i, err := f.Prompter.Select("Run", "", labels)
	if err != nil {
		return nil, nil, cmdutil.ErrCancel
	}
	return &runs[i], pending[runs[i].ID], nil
}

// narrowEnvs keeps the gates at envs, all of them when envs is empty.
func narrowEnvs(run *api.WorkflowRun, pending []api.PendingDeployment, envs []string) ([]api.PendingDeployment, error) {
	if len(envs) == 0 {
		return pending, nil
	}
	var out []api.PendingDeployment
	for _, e := range envs {
		i := slices.IndexFunc(pending, func(p api.PendingDeployment) bool { return strings.EqualFold(p.Environment.Name, e) })
		if i < 0 {
			waiting := make([]string, len(pending))
			for j, p := range pending {
				waiting[j] = p.Environment.Name
			}
			return nil, fmt.Errorf("run %d is not waiting on %s (waiting: %s)", run.ID, e, strings.Join(waiting, ", "))
		}
		out = append(out, pending[i])
	}
	return out, nil
}

// blockedReason explains why the current user cannot review a gate.
func blockedReason(c *api.Client, run *api.WorkflowRun, p api.PendingDeployment) (string, error) {
	name := p.Environment.Name
	if len(p.Reviewers) == 0 {
		return fmt.Sprintf("%s has no required reviewers; it waits on a wait timer or a protection app", name), nil
	}
	env, err := c.GetEnvironment(name)
	if err != nil {
		return "", err
	}
	if _, preventSelf := env.Reviewers(); preventSelf {
		me, err := c.CurrentUser()
		if err != nil {
			return "", err
		}
		if strings.EqualFold(me, run.TriggeringActor.Login) {
			return fmt.Sprintf("you triggered this run and %s prevents self-review; another reviewer must approve", name), nil
		}
	}
	return fmt.Sprintf("you are not a required reviewer of %s (reviewers: %s)", name, strings.Join(reviewerNames(p.Reviewers), ", ")), nil
}

// repoName is [HOST/]OWNER/REPO, with the host only when it is not github.com.
func repoName(r repository.Repository) string {
	if r.Host == "" || r.Host == "github.com" {
		return r.Owner + "/" + r.Name
	}
	return r.Host + "/" + r.Owner + "/" + r.Name
}
