package cmd

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/cli/go-gh/v2/pkg/tableprinter"
	"github.com/cli/go-gh/v2/pkg/text"
	"github.com/spf13/cobra"

	"github.com/shbernal/gh-envy/internal/api"
	"github.com/shbernal/gh-envy/internal/cmdutil"
)

func newViewCmd(f *cmdutil.Factory) *cobra.Command {
	var web bool
	var exporter *cmdutil.Exporter
	cmd := &cobra.Command{
		Use:   "view [<environment>]",
		Short: "View an environment and its protection rules",
		Long: `Display an environment: required reviewers, wait timer, admin bypass,
deployment branch and tag policies, custom protection apps, variables, secret
names, the last deployment and the runs waiting at its gate.

With no argument, prompts for an environment.`,
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeEnvs(f, 1),
		RunE: func(_ *cobra.Command, args []string) error {
			c, err := f.Client()
			if err != nil {
				return err
			}
			name, err := envArg(f, c, args)
			if err != nil {
				return err
			}
			env, err := c.GetEnvironment(name)
			if err != nil {
				return err
			}
			if web {
				u := c.SettingsURL(env)
				if f.IO.StdoutTTY {
					fmt.Fprintf(f.IO.ErrOut, "Opening %s in your browser.\n", u)
				}
				return f.Browser.Browse(u)
			}
			want := func(string) bool { return true }
			if exporter != nil {
				want = exporter.Has
			}
			d, err := fetchDetails(c, env, want)
			if err != nil {
				return err
			}
			if exporter != nil {
				return exporter.Write(f.IO, exporter.Pick(record(c, env, d)))
			}
			if f.IO.StdoutTTY {
				printView(f.IO, c, env, d)
			} else {
				printViewRaw(f.IO, c, env, d)
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&web, "web", "w", false, "Open the environment settings in the browser")
	cmdutil.AddJSONFlags(cmd, &exporter, viewFields)
	return cmd
}

func printView(io *cmdutil.IO, c *api.Client, env *api.Environment, d details) {
	out := io.Out
	fmt.Fprintf(out, "%s %s\n", io.Bold(env.Name), io.Gray("· "+c.FullName()))
	fmt.Fprintf(out, "%s\n\n", io.Gray("Created "+io.Ago(env.CreatedAt)+" · updated "+io.Ago(env.UpdatedAt)))

	row := func(label, value string) { fmt.Fprintf(out, "  %-20s %s\n", label, value) }
	none := io.Gray("none")

	fmt.Fprintln(out, io.Bold("Protection"))
	entries, preventSelf := env.Reviewers()
	if len(entries) == 0 {
		row("Required reviewers", none)
	} else {
		v := strings.Join(reviewerNames(entries), ", ")
		if preventSelf {
			v += io.Gray(" (prevent self-review)")
		}
		row("Required reviewers", v)
	}
	if w := env.WaitTimer(); w == 0 {
		row("Wait timer", none)
	} else {
		row("Wait timer", formatWait(w))
	}
	row("Admin bypass", map[bool]string{true: "allowed", false: io.Yellow("not allowed")}[env.CanAdminsBypass])
	switch env.BranchPolicyMode() {
	case "all":
		row("Deployment refs", "all branches and tags")
	case "protected":
		row("Deployment refs", "protected branches")
	default:
		if len(d.BranchPolicies) == 0 {
			row("Deployment refs", io.Yellow("custom, no patterns: nothing can deploy"))
		} else {
			parts := make([]string, len(d.BranchPolicies))
			for i, p := range d.BranchPolicies {
				parts[i] = p.Name + io.Gray(" ("+p.Type+")")
			}
			row("Deployment refs", strings.Join(parts, ", "))
		}
	}
	if len(d.ProtectionApps) == 0 {
		row("Protection apps", none)
	} else {
		parts := make([]string, len(d.ProtectionApps))
		for i, a := range d.ProtectionApps {
			parts[i] = a.App.Slug
			if !a.Enabled {
				parts[i] += io.Gray(" (disabled)")
			}
		}
		row("Protection apps", strings.Join(parts, ", "))
	}

	fmt.Fprintf(out, "\n%s\n", io.Bold("Deployments"))
	if dep := d.LastDeployment; dep == nil {
		row("Last deployment", none)
	} else {
		v := fmt.Sprintf("%s %s by %s, %s", shortSHA(dep.SHA), dep.Ref, dep.Creator.Login, io.Ago(dep.CreatedAt))
		if dep.State != "" {
			v += " " + io.Gray("("+dep.State+")")
		}
		row("Last deployment", v)
	}
	if len(d.Waiting) == 0 {
		row("Waiting", none)
	} else {
		row("Waiting", text.Pluralize(len(d.Waiting), "run"))
		t := tableprinter.New(out, true, io.Width)
		for _, g := range d.Waiting {
			t.AddField("   ")
			t.AddField(strconv.FormatInt(g.Run.ID, 10))
			t.AddField(g.Run.Name)
			t.AddField(g.Run.HeadBranch, tableprinter.WithColor(io.Gray))
			t.AddField(io.Ago(g.Run.CreatedAt), tableprinter.WithColor(io.Gray))
			t.AddField(strings.Join(gateReasons(g.Pending, io.Now()), ", "))
			t.EndRow()
		}
		_ = t.Render()
	}

	fmt.Fprintf(out, "\n%s %s\n", io.Bold("Variables"), io.Gray(fmt.Sprintf("(%d)", len(d.Variables))))
	for _, v := range d.Variables {
		row(v.Name, v.Value)
	}
	fmt.Fprintf(out, "\n%s %s\n", io.Bold("Secrets"), io.Gray(fmt.Sprintf("(%d)", len(d.Secrets))))
	for _, s := range d.Secrets {
		row(s.Name, io.Gray("updated "+io.Ago(s.UpdatedAt)))
	}
	fmt.Fprintf(out, "\n%s\n", io.Gray("View in browser: "+c.SettingsURL(env)))
}

// printViewRaw prints "key:\tvalue" lines, the non-terminal format of gh view commands.
func printViewRaw(io *cmdutil.IO, c *api.Client, env *api.Environment, d details) {
	entries, preventSelf := env.Reviewers()
	patterns := make([]string, len(d.BranchPolicies))
	for i, p := range d.BranchPolicies {
		patterns[i] = p.Type + ":" + p.Name
	}
	apps := make([]string, len(d.ProtectionApps))
	for i, a := range d.ProtectionApps {
		apps[i] = a.App.Slug
	}
	vars := make([]string, len(d.Variables))
	for i, v := range d.Variables {
		vars[i] = v.Name
	}
	secrets := make([]string, len(d.Secrets))
	for i, s := range d.Secrets {
		secrets[i] = s.Name
	}
	lastDeployment := ""
	if dep := d.LastDeployment; dep != nil {
		lastDeployment = strings.Join([]string{dep.SHA, dep.Ref, dep.Creator.Login, dep.State, dep.CreatedAt.Format(time.RFC3339)}, " ")
	}
	waiting := make([]string, len(d.Waiting))
	for i, g := range d.Waiting {
		waiting[i] = strconv.FormatInt(g.Run.ID, 10)
	}
	lines := [][2]string{
		{"name", env.Name},
		{"repository", c.FullName()},
		{"reviewers", strings.Join(reviewerNames(entries), ",")},
		{"prevent-self-review", strconv.FormatBool(preventSelf)},
		{"wait-timer", strconv.Itoa(env.WaitTimer())},
		{"admin-bypass", strconv.FormatBool(env.CanAdminsBypass)},
		{"branch-policy", env.BranchPolicyMode()},
		{"branch-patterns", strings.Join(patterns, ",")},
		{"protection-apps", strings.Join(apps, ",")},
		{"variables", strings.Join(vars, ",")},
		{"secrets", strings.Join(secrets, ",")},
		{"last-deployment", lastDeployment},
		{"waiting", strings.Join(waiting, ",")},
		{"url", c.SettingsURL(env)},
	}
	for _, l := range lines {
		fmt.Fprintf(io.Out, "%s:\t%s\n", l[0], l[1])
	}
}

// shortSHA is the 7-character abbreviation of a commit SHA.
func shortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}
