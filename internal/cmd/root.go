// Package cmd implements the gh envy command tree.
package cmd

import (
	"errors"
	"fmt"
	"runtime/debug"

	"github.com/spf13/cobra"

	"github.com/shbernal/gh-envy/internal/cmdutil"
)

// Version is set at build time; it falls back to the module version.
var Version = ""

func version() string {
	if Version != "" {
		return Version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "dev"
}

// NewRoot builds the gh envy command tree.
func NewRoot(f *cmdutil.Factory) *cobra.Command {
	root := &cobra.Command{
		Use:   "envy <command>",
		Short: "Manage GitHub deployment environments",
		Long: `Manage the deployment environments of a repository: list, view, create,
edit, delete and clone them, with their protection rules.

Environment secrets and variables are managed by core gh:
  gh secret set NAME --env ENV
  gh variable set NAME --env ENV`,
		Example: `  $ gh envy list
  $ gh envy create production --reviewer octocat --reviewer my-org/ops --wait-timer 30m --branch main
  $ gh envy view production --json reviewers,waitTimer
  $ gh envy clone staging qa`,
		Version:       version(),
		SilenceErrors: true,
		SilenceUsage:  true,
		Annotations:   map[string]string{cobra.CommandDisplayNameAnnotation: "gh envy"},
	}
	root.SetVersionTemplate("gh envy {{.Version}}\n")
	root.PersistentFlags().StringVarP(&f.RepoFlag, "repo", "R", "", "Select another repository using the `[HOST/]OWNER/REPO` format")
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return cmdutil.FlagError{Err: err} })
	root.CompletionOptions.HiddenDefaultCmd = true

	root.AddCommand(
		newListCmd(f),
		newViewCmd(f),
		newCreateCmd(f),
		newEditCmd(f),
		newDeleteCmd(f),
		newCloneCmd(f),
		newProtectionCmd(f),
		newWaitingCmd(f),
		newReviewCmd(f, "approved"),
		newReviewCmd(f, "rejected"),
	)
	return root
}

// Execute runs the command tree and returns the process exit code.
func Execute(f *cmdutil.Factory, args []string) int {
	root := NewRoot(f)
	root.SetArgs(args)
	root.SetOut(f.IO.Out)
	root.SetErr(f.IO.ErrOut)
	cmd, err := root.ExecuteC()
	if err == nil {
		return 0
	}
	switch {
	case errors.Is(err, cmdutil.ErrSilent):
		return 1
	case errors.Is(err, cmdutil.ErrCancel):
		return 2
	}
	fmt.Fprintln(f.IO.ErrOut, err)
	var flagErr cmdutil.FlagError
	if errors.As(err, &flagErr) {
		fmt.Fprintf(f.IO.ErrOut, "\nUsage:  %s\n", cmd.UseLine())
	}
	return 1
}
