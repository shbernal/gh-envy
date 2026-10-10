# Command reference

Every command takes `-R, --repo [HOST/]OWNER/REPO`. Without it, gh envy uses `GH_REPO`, then the GitHub remote of the current directory, like core gh.

Commands that take an `<environment>` prompt for one when it is omitted in a terminal.

## list

```
gh envy list [--limit N] [--json FIELDS [--jq EXPR | --template TMPL]]
```

Lists environments with their reviewers, wait timer, deployment ref policy, admin bypass and last update. The ref policy is `all`, `protected` or `custom`.

Piped output is tab-separated with no header: name, reviewers (comma-separated), wait timer in minutes, ref policy, admin bypass (`true`/`false`), updated time in RFC 3339.

## view

```
gh envy view [<environment>] [--web] [--json FIELDS [--jq EXPR | --template TMPL]]
```

Shows protection rules, branch and tag patterns, protection apps, variables with values, and secret names. `--web` opens the environment's settings page.

Piped output is `key:\tvalue` lines.

## create

```
gh envy create <environment> [flags]
```

| Flag | Meaning |
| --- | --- |
| `-r, --reviewer` | Required reviewer: a user login or `ORG/TEAM`. Repeatable, at most 6. A leading `@` is accepted. |
| `--prevent-self-review` | The person who triggered a deployment cannot approve it. Needs a reviewer. |
| `-w, --wait-timer` | Delay before deployments proceed: minutes (`30`) or a duration in whole minutes (`45m`, `2h`, `1d`). Up to 30 days. |
| `--admin-bypass` | Let administrators bypass the rules. On by default; `--admin-bypass=false` to turn off. |
| `--protected-branches` | Only branches with branch protection can deploy. |
| `-b, --branch` | Branch name pattern allowed to deploy. Repeatable. |
| `--tag` | Tag name pattern allowed to deploy. Repeatable. |
| `--force` | Update the environment if it exists instead of failing. |

`--protected-branches` and the pattern flags are mutually exclusive. Without either, every branch and tag can deploy.

## edit

```
gh envy edit [<environment>] [flags]
```

Takes the `create` flags, where `--reviewer`, `--branch` and `--tag` replace the current set, plus:

| Flag | Meaning |
| --- | --- |
| `--add-reviewer`, `--remove-reviewer` | Change the reviewer set in place. |
| `--no-reviewers` | Remove every required reviewer. |
| `--add-branch`, `--remove-branch` | Change branch patterns in place. |
| `--add-tag`, `--remove-tag` | Change tag patterns in place. |
| `--any-branch` | Drop the ref policy: every branch and tag can deploy. |

Settings not named by a flag are kept. `--wait-timer 0` removes the wait timer.

With no flags in a terminal, `edit` asks for each setting, seeded with the current values.

## delete

```
gh envy delete <environment>... [--yes]
```

Deletes environments with their secrets, variables and protection rules. In a terminal, you type each name to confirm. Piped or scripted runs need `--yes`.

## clone

```
gh envy clone <source> <destination> [--to-repo OWNER/REPO] [--no-variables] [--force]
```

Creates `<destination>` with the reviewers, wait timer, admin bypass, ref policy and patterns, protection apps and variables of `<source>`. `--to-repo` targets another repository on the same host.

Secret values cannot be read through the API, so secrets are not copied. The command prints a `gh secret set` line for each one.

`--force` overwrites an existing destination's settings and merges variables into it.

## protection

```
gh envy protection list [<environment>] [--available] [--json appId,id,slug,status]
gh envy protection enable <environment> <app-slug>
gh envy protection disable <environment> <app-slug>
```

Manages custom deployment protection rules, which gate deployments on a GitHub App. An app must be installed on the repository before it shows in `--available` and can be enabled.

## JSON fields

`list` and `view` accept:

| Field | Type |
| --- | --- |
| `name`, `id`, `url` | environment name, numeric ID, settings page URL |
| `createdAt`, `updatedAt` | timestamps |
| `reviewers` | `[{type, name, id}]`, `name` is a login or `ORG/TEAM` |
| `preventSelfReview`, `adminBypass` | booleans |
| `waitTimer` | minutes |
| `branchPolicy` | `all`, `protected` or `custom` |
| `branchPolicies` | `[{name, type, id}]`, `type` is `branch` or `tag` |
| `protectionApps` | `[{slug, appId, id, enabled}]` |
| `variables` | `[{name, value, updatedAt}]` |
| `secrets` | `[{name, updatedAt}]` |

Fields that need extra API calls (`branchPolicies`, `protectionApps`, `variables`, `secrets`) are fetched only when requested.

## Plans and permissions

- Managing environments needs admin access to the repository. A token with the `repo` scope is enough.
- On private repositories, required reviewers and wait timers need GitHub Pro, Team or Enterprise. Branch policies and admin bypass work on every plan. gh envy only sends the plan-gated fields when they are set or being cleared, so plain environments work on free plans.
