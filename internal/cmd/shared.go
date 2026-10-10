package cmd

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/shbernal/gh-envy/internal/api"
	"github.com/shbernal/gh-envy/internal/cmdutil"
)

const maxWaitMinutes = 43200

// envArg returns the environment named by args[0], or prompts for one.
func envArg(f *cmdutil.Factory, c *api.Client, args []string) (string, error) {
	if len(args) > 0 {
		return args[0], nil
	}
	if !f.IO.CanPrompt() {
		return "", cmdutil.FlagError{Err: errors.New("environment name required when not running interactively")}
	}
	envs, err := c.ListEnvironments(0)
	if err != nil {
		return "", err
	}
	if len(envs) == 0 {
		return "", fmt.Errorf("no environments in %s", c.FullName())
	}
	names := make([]string, len(envs))
	for i, e := range envs {
		names[i] = e.Name
	}
	i, err := f.Prompter.Select("Environment", "", names)
	if err != nil {
		return "", cmdutil.ErrCancel
	}
	return names[i], nil
}

// completeEnvs completes environment names for the first positional argument.
func completeEnvs(f *cmdutil.Factory, maxArgs int) cobra.CompletionFunc {
	return func(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
		if maxArgs >= 0 && len(args) >= maxArgs {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		c, err := f.Client()
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		envs, err := c.ListEnvironments(0)
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		var names []string
		for _, e := range envs {
			if !slices.Contains(args, e.Name) {
				names = append(names, e.Name)
			}
		}
		return names, cobra.ShellCompDirectiveNoFileComp
	}
}

// parseWait reads a wait timer: bare minutes ("30") or a duration ("45m", "2h", "1d", "1h30m").
func parseWait(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	var minutes int
	if n, err := strconv.Atoi(s); err == nil {
		minutes = n
	} else if days, ok := strings.CutSuffix(s, "d"); ok {
		n, err := strconv.Atoi(days)
		if err != nil {
			return 0, fmt.Errorf("invalid wait timer %q", s)
		}
		minutes = n * 1440
	} else {
		d, err := time.ParseDuration(s)
		if err != nil || d%time.Minute != 0 {
			return 0, fmt.Errorf("invalid wait timer %q: use minutes (30) or a duration in whole minutes (45m, 2h, 1d)", s)
		}
		minutes = int(d / time.Minute)
	}
	if minutes < 0 || minutes > maxWaitMinutes {
		return 0, fmt.Errorf("wait timer must be between 0 and %d minutes (30 days)", maxWaitMinutes)
	}
	return minutes, nil
}

// formatWait renders minutes compactly: 0 → "none", 90 → "1h30m", 2880 → "2d".
func formatWait(minutes int) string {
	if minutes == 0 {
		return "none"
	}
	if minutes%1440 == 0 {
		return fmt.Sprintf("%dd", minutes/1440)
	}
	if minutes%60 == 0 {
		return fmt.Sprintf("%dh", minutes/60)
	}
	d := time.Duration(minutes) * time.Minute
	return strings.TrimSuffix(d.String(), "0s")
}

// reviewerNames lists reviewers as logins and org/team slugs.
func reviewerNames(entries []api.ReviewerEntry) []string {
	names := make([]string, len(entries))
	for i, r := range entries {
		names[i] = r.Name()
	}
	return names
}
