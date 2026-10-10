package cmd

import (
	"errors"
	"fmt"
	"slices"

	"github.com/spf13/cobra"

	"github.com/shbernal/gh-envy/internal/api"
	"github.com/shbernal/gh-envy/internal/cmdutil"
)

func newCloneCmd(f *cmdutil.Factory) *cobra.Command {
	var toRepo string
	var noVariables, force bool
	cmd := &cobra.Command{
		Use:   "clone <source> <destination>",
		Short: "Copy an environment's settings to a new environment",
		Long: `Create <destination> with the protection rules, branch and tag patterns,
protection apps and variables of <source>.

Secret values cannot be read through the API, so secrets are not copied:
the command prints the gh secret set lines to run for each of them.

With --to-repo, the destination lives in another repository on the same host.`,
		Example: `  $ gh envy clone staging qa
  $ gh envy clone production production --to-repo my-org/other-service
  $ gh envy clone production staging --force --no-variables`,
		Args:              cobra.ExactArgs(2),
		ValidArgsFunction: completeEnvs(f, 1),
		RunE: func(_ *cobra.Command, args []string) error {
			src, err := f.Client()
			if err != nil {
				return err
			}
			dst := src
			if toRepo != "" {
				repo, err := f.ParseRepo(toRepo)
				if err != nil {
					return err
				}
				if repo.Host != src.Repo.Host {
					return cmdutil.FlagError{Err: errors.New("`--to-repo` must be on the same host as the source repository")}
				}
				dst = src.ForRepo(repo)
			}
			srcName, dstName := args[0], args[1]
			if dst.FullName() == src.FullName() && srcName == dstName {
				return cmdutil.FlagError{Err: errors.New("source and destination are the same environment")}
			}
			return cloneEnv(f, src, dst, srcName, dstName, !noVariables, force)
		},
	}
	cmd.Flags().StringVar(&toRepo, "to-repo", "", "Create the destination in another `[HOST/]OWNER/REPO`")
	cmd.Flags().BoolVar(&noVariables, "no-variables", false, "Do not copy variables")
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite an existing destination; variables are merged")
	return cmd
}

func cloneEnv(f *cmdutil.Factory, src, dst *api.Client, srcName, dstName string, withVars, force bool) error {
	env, err := src.GetEnvironment(srcName)
	if err != nil {
		return err
	}
	var current *api.Protection
	existing, err := dst.GetEnvironment(dstName)
	switch {
	case err == nil && !force:
		return fmt.Errorf("environment %q already exists in %s; use `--force` to overwrite it", dstName, dst.FullName())
	case err == nil:
		p := existing.Protection()
		current = &p
	case !errors.Is(err, api.ErrNotFound):
		return err
	}

	pl := plan{protection: env.Protection()}
	if env.BranchPolicyMode() == "custom" {
		_, pats, err := listPatterns(src, srcName)
		if err != nil {
			return err
		}
		pl.patterns = append([]pattern{}, pats...)
	}
	if err := apply(dst, dstName, pl, current); err != nil {
		return err
	}

	io := f.IO
	warn := func(format string, a ...any) {
		fmt.Fprintf(io.ErrOut, "%s %s\n", io.WarningIcon(), fmt.Sprintf(format, a...))
	}

	apps, err := src.ListProtectionApps(srcName)
	if err != nil {
		return err
	}
	if len(apps) > 0 {
		have, err := dst.ListProtectionApps(dstName)
		if err != nil {
			return err
		}
		for _, a := range apps {
			if slices.ContainsFunc(have, func(h api.ProtectionApp) bool { return h.App.ID == a.App.ID }) {
				continue
			}
			if err := dst.EnableProtectionApp(dstName, a.App.ID); err != nil {
				warn("protection app %s not enabled: %v", a.App.Slug, err)
			}
		}
	}

	copied := 0
	if withVars {
		vars, err := src.ListVariables(srcName)
		if err != nil {
			return err
		}
		var have []api.Variable
		if current != nil {
			if have, err = dst.ListVariables(dstName); err != nil {
				return err
			}
		}
		for _, v := range vars {
			exists := slices.ContainsFunc(have, func(h api.Variable) bool { return h.Name == v.Name })
			if err := dst.SetVariable(dstName, v.Name, v.Value, exists); err != nil {
				return fmt.Errorf("copying variable %s: %w", v.Name, err)
			}
			copied++
		}
	}

	secrets, err := src.ListSecrets(srcName)
	if err != nil {
		return err
	}

	if io.StdoutTTY {
		verb := "Created"
		if current != nil {
			verb = "Updated"
		}
		fmt.Fprintf(io.Out, "%s %s environment %s in %s from %s", io.SuccessIcon(), verb, io.Bold(dstName), dst.FullName(), srcName)
		if copied > 0 {
			fmt.Fprintf(io.Out, " (%d variables)", copied)
		}
		fmt.Fprintln(io.Out)
	}
	if len(secrets) > 0 {
		warn("%d secrets were not copied; set them with:", len(secrets))
		for _, s := range secrets {
			fmt.Fprintf(io.ErrOut, "  gh secret set %s --env %s -R %s\n", s.Name, dstName, dst.FullName())
		}
	}
	return nil
}
