package cmd

import (
	"fmt"
	"strconv"
	"time"

	"github.com/cli/go-gh/v2/pkg/tableprinter"
	"github.com/spf13/cobra"

	"github.com/shbernal/gh-envy/internal/api"
	"github.com/shbernal/gh-envy/internal/cmdutil"
)

// deploymentFields are the --json fields of deployments.
var deploymentFields = []string{
	"createdAt", "creator", "environment", "id", "ref", "sha", "state", "url",
}

func deploymentRecord(d api.Deployment) map[string]any {
	return map[string]any{
		"createdAt":   d.CreatedAt,
		"creator":     d.Creator.Login,
		"environment": d.Environment,
		"id":          d.ID,
		"ref":         d.Ref,
		"sha":         d.SHA,
		"state":       d.State,
		"url":         d.URL,
	}
}

func newDeploymentsCmd(f *cmdutil.Factory) *cobra.Command {
	var limit int
	var exporter *cmdutil.Exporter
	cmd := &cobra.Command{
		Use:   "deployments [<environment>]",
		Short: "List the deployment history of an environment",
		Long: `List deployments, newest first, with the state of each one's latest status.
Pass an environment to show only its deployments.

Each deployment costs one extra API request to read its state, skipped when
--json selects neither state nor url.`,
		Example: `  $ gh envy deployments production
  $ gh envy deployments -L 100
  $ gh envy deployments production --json sha,state --jq '.[] | select(.state == "failure") | .sha'`,
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
			withState := exporter == nil || exporter.Has("state") || exporter.Has("url")
			deployments, err := c.ListDeployments(env, limit, withState)
			if err != nil {
				return err
			}
			if exporter != nil {
				rows := make([]map[string]any, len(deployments))
				for i, d := range deployments {
					rows[i] = exporter.Pick(deploymentRecord(d))
				}
				return exporter.Write(f.IO, rows)
			}
			if len(deployments) == 0 {
				if f.IO.StdoutTTY {
					if env == "" {
						fmt.Fprintf(f.IO.ErrOut, "No deployments in %s\n", c.FullName())
					} else {
						fmt.Fprintf(f.IO.ErrOut, "No deployments to %s in %s\n", env, c.FullName())
					}
				}
				return nil
			}
			return printDeployments(f.IO, deployments)
		},
	}
	cmd.Flags().IntVarP(&limit, "limit", "L", 30, "Maximum number of deployments to fetch")
	cmdutil.AddJSONFlags(cmd, &exporter, deploymentFields)
	return cmd
}

func printDeployments(io *cmdutil.IO, deployments []api.Deployment) error {
	t := tableprinter.New(io.Out, io.StdoutTTY, io.Width)
	t.AddHeader([]string{"ENVIRONMENT", "SHA", "REF", "CREATOR", "STATE", "CREATED", "ID"})
	for _, d := range deployments {
		t.AddField(d.Environment, tableprinter.WithColor(io.Bold))
		if io.StdoutTTY {
			t.AddField(shortSHA(d.SHA))
		} else {
			t.AddField(d.SHA)
		}
		t.AddField(d.Ref, tableprinter.WithColor(io.Gray))
		t.AddField(d.Creator.Login)
		t.AddField(d.State, tableprinter.WithColor(stateColor(io, d.State)))
		if io.StdoutTTY {
			t.AddField(io.Ago(d.CreatedAt), tableprinter.WithColor(io.Gray))
		} else {
			t.AddField(d.CreatedAt.Format(time.RFC3339))
		}
		t.AddField(strconv.FormatInt(d.ID, 10))
		t.EndRow()
	}
	return t.Render()
}

// stateColor colors a deployment state the way the GitHub web UI does.
func stateColor(io *cmdutil.IO, state string) func(string) string {
	switch state {
	case "success":
		return io.Green
	case "failure", "error":
		return io.Red
	case "in_progress", "queued", "pending":
		return io.Yellow
	}
	return io.Gray
}
