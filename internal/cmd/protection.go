package cmd

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/cli/go-gh/v2/pkg/tableprinter"
	"github.com/spf13/cobra"

	"github.com/shbernal/gh-envy/internal/api"
	"github.com/shbernal/gh-envy/internal/cmdutil"
)

func newProtectionCmd(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "protection <command>",
		Aliases: []string{"apps"},
		Short:   "Manage custom deployment protection apps",
		Long: `Custom deployment protection rules gate deployments on a GitHub App
(Datadog, Honeycomb, ServiceNow, your own). The app must be installed on the
repository before it can be enabled on an environment.`,
	}
	cmd.AddCommand(newProtectionListCmd(f), newProtectionToggleCmd(f, true), newProtectionToggleCmd(f, false))
	return cmd
}

func newProtectionListCmd(f *cmdutil.Factory) *cobra.Command {
	var available bool
	var exporter *cmdutil.Exporter
	cmd := &cobra.Command{
		Use:               "list [<environment>]",
		Aliases:           []string{"ls"},
		Short:             "List the protection apps of an environment",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeEnvs(f, 1),
		RunE: func(_ *cobra.Command, args []string) error {
			c, err := f.Client()
			if err != nil {
				return err
			}
			env, err := envArg(f, c, args)
			if err != nil {
				return err
			}
			rows, err := protectionRows(c, env, available)
			if err != nil {
				return err
			}
			if exporter != nil {
				picked := make([]map[string]any, len(rows))
				for i, r := range rows {
					picked[i] = exporter.Pick(r)
				}
				return exporter.Write(f.IO, picked)
			}
			if len(rows) == 0 {
				if f.IO.StdoutTTY {
					hint := ""
					if !available {
						hint = "; see installable apps with --available"
					}
					fmt.Fprintf(f.IO.ErrOut, "No protection apps on %s%s\n", env, hint)
				}
				return nil
			}
			t := tableprinter.New(f.IO.Out, f.IO.StdoutTTY, f.IO.Width)
			t.AddHeader([]string{"APP", "STATUS", "APP ID"})
			for _, r := range rows {
				t.AddField(r["slug"].(string), tableprinter.WithColor(f.IO.Bold))
				t.AddField(r["status"].(string))
				t.AddField(strconv.FormatInt(r["appId"].(int64), 10), tableprinter.WithColor(f.IO.Gray))
				t.EndRow()
			}
			return t.Render()
		},
	}
	cmd.Flags().BoolVarP(&available, "available", "a", false, "Include installed apps that can be enabled")
	cmdutil.AddJSONFlags(cmd, &exporter, []string{"appId", "id", "slug", "status"})
	return cmd
}

func protectionRows(c *api.Client, env string, available bool) ([]map[string]any, error) {
	rules, err := c.ListProtectionApps(env)
	if err != nil {
		return nil, err
	}
	var rows []map[string]any
	for _, r := range rules {
		status := "enabled"
		if !r.Enabled {
			status = "disabled"
		}
		rows = append(rows, map[string]any{"slug": r.App.Slug, "status": status, "appId": r.App.ID, "id": r.ID})
	}
	if available {
		apps, err := c.ListAvailableApps(env)
		if err != nil {
			return nil, err
		}
		for _, a := range apps {
			if slices.ContainsFunc(rules, func(r api.ProtectionApp) bool { return r.App.ID == a.ID }) {
				continue
			}
			rows = append(rows, map[string]any{"slug": a.Slug, "status": "available", "appId": a.ID, "id": nil})
		}
	}
	return rows, nil
}

func newProtectionToggleCmd(f *cmdutil.Factory, enable bool) *cobra.Command {
	use, short := "enable", "Enable a protection app on an environment"
	if !enable {
		use, short = "disable", "Disable a protection app on an environment"
	}
	return &cobra.Command{
		Use:               use + " <environment> <app-slug>",
		Short:             short,
		Args:              cobra.ExactArgs(2),
		ValidArgsFunction: completeEnvs(f, 1),
		RunE: func(_ *cobra.Command, args []string) error {
			c, err := f.Client()
			if err != nil {
				return err
			}
			env, slug := args[0], args[1]
			if enable {
				apps, err := c.ListAvailableApps(env)
				if err != nil {
					return err
				}
				i := slices.IndexFunc(apps, func(a api.AppRef) bool { return a.Slug == slug })
				if i < 0 {
					return fmt.Errorf("app %q is not installable on %s; available: %s", slug, env, slugList(apps))
				}
				if err := c.EnableProtectionApp(env, apps[i].ID); err != nil {
					return err
				}
			} else {
				rules, err := c.ListProtectionApps(env)
				if err != nil {
					return err
				}
				i := slices.IndexFunc(rules, func(r api.ProtectionApp) bool { return r.App.Slug == slug })
				if i < 0 {
					return fmt.Errorf("app %q is not enabled on %s", slug, env)
				}
				if err := c.DisableProtectionApp(env, rules[i].ID); err != nil {
					return err
				}
			}
			if f.IO.StdoutTTY {
				fmt.Fprintf(f.IO.Out, "%s %sd %s on %s\n", f.IO.SuccessIcon(), strings.ToUpper(use[:1])+use[1:], f.IO.Bold(slug), env)
			}
			return nil
		},
	}
}

func slugList(apps []api.AppRef) string {
	if len(apps) == 0 {
		return "none (install the app on the repository first)"
	}
	slugs := make([]string, len(apps))
	for i, a := range apps {
		slugs[i] = a.Slug
	}
	return strings.Join(slugs, ", ")
}
