package cmd

import (
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/shbernal/gh-envy/internal/api"
	"github.com/shbernal/gh-envy/internal/cmdutil"
)

const maxReviewers = 6

// pattern is a deployment branch policy pattern.
type pattern struct{ Name, Type string }

// protectionFlags are the flags shared by create and edit.
type protectionFlags struct {
	reviewers, addReviewers, removeReviewers []string
	noReviewers                              bool
	wait                                     string
	preventSelfReview, adminBypass           bool
	protectedBranches, anyBranch             bool
	branches, tags                           []string
	addBranches, removeBranches              []string
	addTags, removeTags                      []string
}

func (p *protectionFlags) register(fs *pflag.FlagSet, edit bool) {
	fs.StringSliceVarP(&p.reviewers, "reviewer", "r", nil, "Required reviewer: a `login` or ORG/TEAM, repeatable (max 6)")
	fs.StringVarP(&p.wait, "wait-timer", "w", "", "Delay before deployments proceed: minutes or a `duration` like 45m, 2h, 1d")
	fs.BoolVar(&p.preventSelfReview, "prevent-self-review", false, "Prevent the user who triggered a deployment from approving it")
	fs.BoolVar(&p.adminBypass, "admin-bypass", true, "Allow administrators to bypass the protection rules")
	fs.BoolVar(&p.protectedBranches, "protected-branches", false, "Only branches with branch protection rules can deploy")
	fs.StringSliceVarP(&p.branches, "branch", "b", nil, "Branch name `pattern` allowed to deploy, repeatable")
	fs.StringSliceVar(&p.tags, "tag", nil, "Tag name `pattern` allowed to deploy, repeatable")
	if !edit {
		return
	}
	fs.StringSliceVar(&p.addReviewers, "add-reviewer", nil, "Add a required reviewer `login` or ORG/TEAM")
	fs.StringSliceVar(&p.removeReviewers, "remove-reviewer", nil, "Remove a required reviewer `login` or ORG/TEAM")
	fs.BoolVar(&p.noReviewers, "no-reviewers", false, "Remove all required reviewers")
	fs.BoolVar(&p.anyBranch, "any-branch", false, "Allow every branch and tag to deploy")
	fs.StringSliceVar(&p.addBranches, "add-branch", nil, "Add a branch name `pattern`")
	fs.StringSliceVar(&p.removeBranches, "remove-branch", nil, "Remove a branch name `pattern`")
	fs.StringSliceVar(&p.addTags, "add-tag", nil, "Add a tag name `pattern`")
	fs.StringSliceVar(&p.removeTags, "remove-tag", nil, "Remove a tag name `pattern`")
}

// plan is the resolved change to apply to an environment.
type plan struct {
	protection api.Protection
	// patterns is the desired custom policy pattern set, nil when unchanged.
	patterns []pattern
	// names labels reviewers in warnings.
	names map[api.ReviewerRef]string
}

// resolve computes the desired state from the current one and the changed flags.
func (p *protectionFlags) resolve(fs *pflag.FlagSet, c *api.Client, current api.Protection, currentPatterns func() ([]pattern, error)) (plan, error) {
	changed := fs.Changed
	desired := current
	desired.Reviewers = slices.Clone(current.Reviewers)

	if p.noReviewers && (changed("reviewer") || changed("add-reviewer")) {
		return plan{}, cmdutil.FlagError{Err: errors.New("`--no-reviewers` cannot be combined with `--reviewer` or `--add-reviewer`")}
	}
	labels := map[api.ReviewerRef]string{}
	resolveAll := func(names []string) ([]api.ReviewerRef, error) {
		refs := make([]api.ReviewerRef, 0, len(names))
		for _, n := range names {
			ref, err := c.ResolveReviewer(n)
			if err != nil {
				return nil, err
			}
			refs = append(refs, ref)
			labels[ref] = strings.TrimPrefix(strings.TrimSpace(n), "@")
		}
		return refs, nil
	}
	if p.noReviewers {
		desired.Reviewers = nil
	}
	if changed("reviewer") {
		refs, err := resolveAll(p.reviewers)
		if err != nil {
			return plan{}, err
		}
		desired.Reviewers = nil
		for _, r := range refs {
			if !slices.Contains(desired.Reviewers, r) {
				desired.Reviewers = append(desired.Reviewers, r)
			}
		}
	}
	if len(p.addReviewers) > 0 {
		refs, err := resolveAll(p.addReviewers)
		if err != nil {
			return plan{}, err
		}
		for _, r := range refs {
			if !slices.Contains(desired.Reviewers, r) {
				desired.Reviewers = append(desired.Reviewers, r)
			}
		}
	}
	if len(p.removeReviewers) > 0 {
		refs, err := resolveAll(p.removeReviewers)
		if err != nil {
			return plan{}, err
		}
		desired.Reviewers = slices.DeleteFunc(desired.Reviewers, func(r api.ReviewerRef) bool { return slices.Contains(refs, r) })
	}
	if len(desired.Reviewers) > maxReviewers {
		return plan{}, fmt.Errorf("an environment accepts at most %d required reviewers, got %d", maxReviewers, len(desired.Reviewers))
	}

	if changed("wait-timer") {
		minutes, err := parseWait(p.wait)
		if err != nil {
			return plan{}, cmdutil.FlagError{Err: err}
		}
		desired.WaitTimer = minutes
	}
	if changed("prevent-self-review") {
		desired.PreventSelfReview = p.preventSelfReview
	}
	if len(desired.Reviewers) == 0 {
		if changed("prevent-self-review") && p.preventSelfReview {
			return plan{}, cmdutil.FlagError{Err: errors.New("`--prevent-self-review` needs at least one required reviewer")}
		}
		desired.PreventSelfReview = false
	}
	if changed("admin-bypass") {
		desired.CanAdminsBypass = p.adminBypass
	}

	custom := changed("branch") || changed("tag") || len(p.addBranches)+len(p.removeBranches)+len(p.addTags)+len(p.removeTags) > 0
	modes := 0
	for _, m := range []bool{p.protectedBranches, p.anyBranch, custom} {
		if m {
			modes++
		}
	}
	if modes > 1 {
		return plan{}, cmdutil.FlagError{Err: errors.New("choose one of `--protected-branches`, `--any-branch`, or branch/tag patterns")}
	}
	out := plan{protection: desired, names: labels}
	switch {
	case p.protectedBranches:
		out.protection.BranchPolicy = &api.DeploymentBranchPolicy{ProtectedBranches: true}
	case p.anyBranch:
		out.protection.BranchPolicy = nil
	case custom:
		var existing []pattern
		if api.BranchPolicyMode(current.BranchPolicy) == "custom" {
			var err error
			if existing, err = currentPatterns(); err != nil {
				return plan{}, err
			}
		}
		out.protection.BranchPolicy = &api.DeploymentBranchPolicy{CustomBranchPolicies: true}
		out.patterns = mergePatterns(existing, p)
		if out.patterns == nil {
			out.patterns = []pattern{}
		}
	}
	return out, nil
}

// mergePatterns applies --branch/--tag (replace per type) and the add/remove flags.
func mergePatterns(existing []pattern, p *protectionFlags) []pattern {
	set := slices.Clone(existing)
	replace := func(kind string, names []string) {
		if names == nil {
			return
		}
		set = slices.DeleteFunc(set, func(x pattern) bool { return x.Type == kind })
		for _, n := range names {
			set = append(set, pattern{n, kind})
		}
	}
	replace("branch", p.branches)
	replace("tag", p.tags)
	for _, n := range p.addBranches {
		set = append(set, pattern{n, "branch"})
	}
	for _, n := range p.addTags {
		set = append(set, pattern{n, "tag"})
	}
	set = slices.DeleteFunc(set, func(x pattern) bool {
		return (x.Type == "branch" && slices.Contains(p.removeBranches, x.Name)) ||
			(x.Type == "tag" && slices.Contains(p.removeTags, x.Name))
	})
	var out []pattern
	for _, x := range set {
		if !slices.Contains(out, x) {
			out = append(out, x)
		}
	}
	return out
}

// listPatterns returns the custom policy patterns of env.
func listPatterns(c *api.Client, env string) ([]api.BranchPolicy, []pattern, error) {
	policies, err := c.ListBranchPolicies(env)
	if err != nil {
		return nil, nil, err
	}
	out := make([]pattern, len(policies))
	for i, p := range policies {
		out[i] = pattern{p.Name, p.Type}
	}
	return policies, out, nil
}

// syncPatterns makes env's custom policy patterns equal to want.
func syncPatterns(c *api.Client, env string, want []pattern) error {
	policies, _, err := listPatterns(c, env)
	if err != nil {
		return err
	}
	var have []pattern
	for _, p := range policies {
		cur := pattern{p.Name, p.Type}
		if !slices.Contains(want, cur) {
			if err := c.DeleteBranchPolicy(env, p.ID); err != nil {
				return fmt.Errorf("removing %s pattern %q: %w", p.Type, p.Name, err)
			}
			continue
		}
		have = append(have, cur)
	}
	for _, w := range want {
		if slices.Contains(have, w) {
			continue
		}
		if err := c.CreateBranchPolicy(env, w.Name, w.Type); err != nil {
			return fmt.Errorf("adding %s pattern %q: %w", w.Type, w.Name, err)
		}
	}
	return nil
}

// apply writes a plan to env and syncs its patterns. GitHub accepts reviewers
// without access to the repository and silently drops them, so apply warns
// about every requested reviewer missing from the response.
func apply(io *cmdutil.IO, c *api.Client, env string, pl plan, current *api.Protection) error {
	got, err := c.PutEnvironment(env, pl.protection, current)
	if err != nil {
		return err
	}
	kept := got.Protection().Reviewers
	for _, r := range pl.protection.Reviewers {
		if slices.Contains(kept, r) {
			continue
		}
		name := pl.names[r]
		if name == "" {
			name = fmt.Sprintf("%s %d", strings.ToLower(r.Type), r.ID)
		}
		fmt.Fprintf(io.ErrOut, "%s GitHub dropped required reviewer %s: reviewers need access to %s as a collaborator or through a team\n", io.WarningIcon(), name, c.FullName())
	}
	if pl.patterns != nil {
		return syncPatterns(c, env, pl.patterns)
	}
	return nil
}

func newCreateCmd(f *cmdutil.Factory) *cobra.Command {
	var flags protectionFlags
	var force bool
	var secrets []string
	cmd := &cobra.Command{
		Use:   "create <environment>",
		Short: "Create an environment",
		Long: `Create a deployment environment, optionally with protection rules.

Reviewers are user logins or ORG/TEAM slugs. Branch and tag patterns use
fnmatch syntax, as on the settings page (release/*, v*).

--secret NAME=VALUE sets a secret once the environment exists. A bare NAME
asks for the value in a terminal; otherwise a single bare NAME reads it from
standard input.`,
		Example: `  $ gh envy create staging
  $ gh envy create production --reviewer octocat --reviewer my-org/ops --prevent-self-review
  $ gh envy create production --wait-timer 30m --branch main --tag 'v*'
  $ gh envy create preview --protected-branches --admin-bypass=false
  $ gh envy create staging --secret DATABASE_URL --secret API_KEY="$API_KEY"
  $ gh envy create staging --secret DATABASE_URL < db-url.txt`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := f.Client()
			if err != nil {
				return err
			}
			name := args[0]
			if !force {
				exists, err := c.Exists(name)
				if err != nil {
					return err
				}
				if exists {
					return fmt.Errorf("environment %q already exists in %s; use `gh envy edit` or `--force`", name, c.FullName())
				}
			}
			pl, err := flags.resolve(cmd.Flags(), c, api.Protection{CanAdminsBypass: true}, func() ([]pattern, error) { return nil, nil })
			if err != nil {
				return err
			}
			values, err := secretValues(f, secrets)
			if err != nil {
				return err
			}
			if err := apply(f.IO, c, name, pl, nil); err != nil {
				return err
			}
			if f.IO.StdoutTTY {
				fmt.Fprintf(f.IO.Out, "%s Created environment %s in %s\n", f.IO.SuccessIcon(), f.IO.Bold(name), c.FullName())
			}
			for _, s := range values {
				if err := c.SetSecret(name, s.name, s.value); err != nil {
					return fmt.Errorf("setting secret %s: %w", s.name, err)
				}
				if f.IO.StdoutTTY {
					fmt.Fprintf(f.IO.Out, "%s Set secret %s\n", f.IO.SuccessIcon(), f.IO.Bold(s.name))
				}
			}
			return nil
		},
	}
	flags.register(cmd.Flags(), false)
	cmd.Flags().BoolVar(&force, "force", false, "Update the environment if it already exists")
	cmd.Flags().StringArrayVar(&secrets, "secret", nil, "Set a secret: `NAME` or NAME=VALUE, repeatable")
	return cmd
}

type secret struct{ name, value string }

// secretValues resolves --secret flags, prompting or reading stdin for bare names.
func secretValues(f *cmdutil.Factory, flags []string) ([]secret, error) {
	out := make([]secret, len(flags))
	var bare []int
	for i, s := range flags {
		name, value, ok := strings.Cut(s, "=")
		if name == "" {
			return nil, cmdutil.FlagError{Err: fmt.Errorf("invalid `--secret` %q: expected NAME or NAME=VALUE", s)}
		}
		out[i] = secret{name, value}
		if !ok {
			bare = append(bare, i)
		}
	}
	if len(bare) == 0 {
		return out, nil
	}
	if f.IO.CanPrompt() {
		for _, i := range bare {
			v, err := f.Prompter.Password("Value for secret " + out[i].name)
			if err != nil {
				return nil, cmdutil.ErrCancel
			}
			out[i].value = v
		}
		return out, nil
	}
	if len(bare) > 1 || f.IO.StdinTTY {
		return nil, cmdutil.FlagError{Err: errors.New("pass values as `--secret NAME=VALUE`, or pipe one value to a single bare `--secret NAME`, when not running interactively")}
	}
	raw, err := io.ReadAll(f.IO.In)
	if err != nil {
		return nil, err
	}
	out[bare[0]].value = strings.TrimRight(string(raw), "\r\n")
	return out, nil
}

func newEditCmd(f *cmdutil.Factory) *cobra.Command {
	var flags protectionFlags
	cmd := &cobra.Command{
		Use:   "edit [<environment>]",
		Short: "Edit an environment's protection rules",
		Long: `Change the protection rules of an environment. Only the given flags change;
everything else is kept.

--reviewer, --branch and --tag replace the current set; the --add-* and
--remove-* forms change it in place. With no flags in a terminal, opens an
interactive editor seeded with the current settings.`,
		Example: `  $ gh envy edit production --add-reviewer hubot --wait-timer 1h
  $ gh envy edit production --remove-branch 'release/*' --add-tag 'v*'
  $ gh envy edit staging --any-branch --no-reviewers --wait-timer 0
  $ gh envy edit production`,
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeEnvs(f, 1),
		RunE: func(cmd *cobra.Command, args []string) error {
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
			current := env.Protection()
			if cmd.Flags().NFlag() == countRepoFlag(cmd) {
				if !f.IO.CanPrompt() {
					return cmdutil.FlagError{Err: errors.New("specify at least one flag to change when not running interactively")}
				}
				if err := promptEdit(f, c, env, &flags, cmd.Flags()); err != nil {
					return err
				}
			}
			pl, err := flags.resolve(cmd.Flags(), c, current, func() ([]pattern, error) {
				_, pats, err := listPatterns(c, name)
				return pats, err
			})
			if err != nil {
				return err
			}
			if err := apply(f.IO, c, name, pl, &current); err != nil {
				return err
			}
			if f.IO.StdoutTTY {
				fmt.Fprintf(f.IO.Out, "%s Updated environment %s in %s\n", f.IO.SuccessIcon(), f.IO.Bold(name), c.FullName())
			}
			return nil
		},
	}
	flags.register(cmd.Flags(), true)
	return cmd
}

func countRepoFlag(cmd *cobra.Command) int {
	if cmd.Flags().Changed("repo") {
		return 1
	}
	return 0
}

// promptEdit asks for each setting, seeded with the current values, and sets
// the matching flags so resolve treats the answers as explicit changes.
func promptEdit(f *cmdutil.Factory, c *api.Client, env *api.Environment, p *protectionFlags, fs *pflag.FlagSet) error {
	pr := f.Prompter
	entries, preventSelf := env.Reviewers()
	reviewers, err := pr.Input("Required reviewers (comma-separated logins or ORG/TEAM, empty for none)", strings.Join(reviewerNames(entries), ", "))
	if err != nil {
		return cmdutil.ErrCancel
	}
	names := splitList(reviewers)
	if len(names) == 0 {
		_ = fs.Set("no-reviewers", "true")
	} else {
		_ = fs.Set("reviewer", strings.Join(names, ","))
	}
	if len(names) > 0 {
		v, err := pr.Confirm("Prevent self-review?", preventSelf)
		if err != nil {
			return cmdutil.ErrCancel
		}
		_ = fs.Set("prevent-self-review", strconv.FormatBool(v))
	}
	wait, err := pr.Input("Wait timer (minutes or 45m, 2h, 1d; 0 for none)", formatWaitInput(env.WaitTimer()))
	if err != nil {
		return cmdutil.ErrCancel
	}
	if err := fs.Set("wait-timer", wait); err != nil {
		return err
	}
	bypass, err := pr.Confirm("Allow administrators to bypass these rules?", env.CanAdminsBypass)
	if err != nil {
		return cmdutil.ErrCancel
	}
	_ = fs.Set("admin-bypass", strconv.FormatBool(bypass))

	modes := []string{"all", "protected", "custom"}
	labels := []string{"All branches and tags", "Protected branches only", "Selected branches and tags"}
	current := slices.Index(modes, env.BranchPolicyMode())
	i, err := pr.Select("Which refs can deploy?", labels[current], labels)
	if err != nil {
		return cmdutil.ErrCancel
	}
	switch modes[i] {
	case "all":
		_ = fs.Set("any-branch", "true")
	case "protected":
		_ = fs.Set("protected-branches", "true")
	case "custom":
		var branches, tags []string
		if current == 2 {
			_, pats, err := listPatterns(c, env.Name)
			if err != nil {
				return err
			}
			for _, x := range pats {
				if x.Type == "tag" {
					tags = append(tags, x.Name)
				} else {
					branches = append(branches, x.Name)
				}
			}
		}
		b, err := pr.Input("Branch patterns (comma-separated)", strings.Join(branches, ", "))
		if err != nil {
			return cmdutil.ErrCancel
		}
		t, err := pr.Input("Tag patterns (comma-separated)", strings.Join(tags, ", "))
		if err != nil {
			return cmdutil.ErrCancel
		}
		p.branches, p.tags = append([]string{}, splitList(b)...), append([]string{}, splitList(t)...)
		fs.Lookup("branch").Changed = true
		fs.Lookup("tag").Changed = true
	}
	return nil
}

func formatWaitInput(minutes int) string {
	if minutes == 0 {
		return "0"
	}
	return formatWait(minutes)
}

func splitList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}
