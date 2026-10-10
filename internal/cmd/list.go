package cmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/cli/go-gh/v2/pkg/tableprinter"
	"github.com/spf13/cobra"

	"github.com/shbernal/gh-envy/internal/api"
	"github.com/shbernal/gh-envy/internal/cmdutil"
)

func newListCmd(f *cmdutil.Factory) *cobra.Command {
	var limit int
	var exporter *cmdutil.Exporter
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List environments",
		Args:    cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if limit < 1 {
				return cmdutil.FlagError{Err: fmt.Errorf("invalid limit: %d", limit)}
			}
			c, err := f.Client()
			if err != nil {
				return err
			}
			envs, err := c.ListEnvironments(limit)
			if err != nil {
				return err
			}
			if exporter != nil {
				rows := make([]map[string]any, 0, len(envs))
				for i := range envs {
					d, err := fetchDetails(c, &envs[i], exporter.Has)
					if err != nil {
						return err
					}
					rows = append(rows, exporter.Pick(record(c, &envs[i], d)))
				}
				return exporter.Write(f.IO, rows)
			}
			if len(envs) == 0 {
				if f.IO.StdoutTTY {
					fmt.Fprintf(f.IO.ErrOut, "No environments in %s\n", c.FullName())
				}
				return nil
			}
			return printList(f.IO, envs)
		},
	}
	cmd.Flags().IntVarP(&limit, "limit", "L", 100, "Maximum number of environments to list")
	cmdutil.AddJSONFlags(cmd, &exporter, environmentFields)
	return cmd
}

func printList(io *cmdutil.IO, envs []api.Environment) error {
	t := tableprinter.New(io.Out, io.StdoutTTY, io.Width)
	t.AddHeader([]string{"NAME", "REVIEWERS", "WAIT", "BRANCHES", "ADMIN BYPASS", "UPDATED"})
	for i := range envs {
		e := &envs[i]
		reviewers, _ := e.Reviewers()
		t.AddField(e.Name, tableprinter.WithColor(io.Bold))
		if io.StdoutTTY {
			t.AddField(strings.Join(reviewerNames(reviewers), ", "), tableprinter.WithColor(io.Gray))
			t.AddField(formatWait(e.WaitTimer()))
			t.AddField(e.BranchPolicyMode())
			t.AddField(yesNo(e.CanAdminsBypass))
			t.AddField(io.Ago(e.UpdatedAt), tableprinter.WithColor(io.Gray))
		} else {
			t.AddField(strings.Join(reviewerNames(reviewers), ","))
			t.AddField(strconv.Itoa(e.WaitTimer()))
			t.AddField(e.BranchPolicyMode())
			t.AddField(strconv.FormatBool(e.CanAdminsBypass))
			t.AddField(e.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"))
		}
		t.EndRow()
	}
	return t.Render()
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
