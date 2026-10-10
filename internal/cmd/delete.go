package cmd

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/shbernal/gh-envy/internal/cmdutil"
)

func newDeleteCmd(f *cmdutil.Factory) *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:     "delete <environment>...",
		Aliases: []string{"rm"},
		Short:   "Delete environments",
		Long: `Delete one or more environments, together with their secrets, variables
and protection rules. This cannot be undone.

In a terminal, asks you to type each environment's name to confirm.`,
		Example: `  $ gh envy delete preview-42
  $ gh envy delete preview-41 preview-42 --yes`,
		Args:              cobra.MinimumNArgs(1),
		ValidArgsFunction: completeEnvs(f, -1),
		RunE: func(_ *cobra.Command, args []string) error {
			if !yes && !f.IO.CanPrompt() {
				return cmdutil.FlagError{Err: errors.New("`--yes` required when not running interactively")}
			}
			c, err := f.Client()
			if err != nil {
				return err
			}
			for _, name := range args {
				if !yes {
					if _, err := c.GetEnvironment(name); err != nil {
						return err
					}
					fmt.Fprintf(f.IO.ErrOut, "%s Deleting %s removes its secrets, variables and protection rules.\n", f.IO.WarningIcon(), f.IO.Bold(name))
					typed, err := f.Prompter.Input(fmt.Sprintf("Type %s to confirm deletion:", name), "")
					if err != nil {
						return cmdutil.ErrCancel
					}
					if typed != name {
						return fmt.Errorf("confirmation did not match %q: nothing deleted for it", name)
					}
				}
				if err := c.DeleteEnvironment(name); err != nil {
					return err
				}
				if f.IO.StdoutTTY {
					fmt.Fprintf(f.IO.Out, "%s Deleted environment %s from %s\n", f.IO.Red("✓"), f.IO.Bold(name), c.FullName())
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip the confirmation prompt")
	return cmd
}
