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

Shows protection rules, branch and tag patterns, protection apps, variables with values, secret names, the last deployment and the runs waiting at the environment's gate. `--web` opens the environment's settings page.

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
| `--secret` | Secret to set once the environment exists: `NAME=VALUE`, or a bare `NAME` to be asked for the value. Repeatable. |
| `--force` | Update the environment if it exists instead of failing. |

`--protected-branches` and the pattern flags are mutually exclusive. Without either, every branch and tag can deploy.

A bare `--secret NAME` asks for the value in a terminal with masked input. When piped, a single bare `NAME` reads its value from standard input, without a trailing newline, as `gh secret set` does. `gh secret set --env` fails on an environment that doesn't exist yet, so `create --secret` does both in one step.

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

## waiting

```
gh envy waiting [<environment>] [-w WORKFLOW] [-b BRANCH] [-L N] [--json FIELDS [--jq EXPR | --template TMPL]]
```

Lists workflow runs held at an environment gate, one row per run and environment. A run waiting on two environments shows twice, so `<environment>` filters by gate. `-w` takes a workflow file name (`deploy.yml`), ID or name.

`--limit` (default 30) counts runs, not rows. Each run costs one more API request to read its gates.

The GATE column says what holds the run:

| GATE | Meaning |
| --- | --- |
| `you` | The environment has required reviewers and you are one of them. |
| `reviewers` | The environment has required reviewers and you can't approve. |
| `timer 4m` | A wait timer is running, with the time left. |
| `app` | No reviewers and no running timer, so a protection app or another rule holds the run. |

Values combine, as in `you, timer 4m`. Piped output is tab-separated with no header: environment, workflow, title, branch, run ID, created time in RFC 3339, gate.

## approve, reject

```
gh envy approve [<run-id> | <run-url>] [-e ENV]... [-w WORKFLOW] [-b BRANCH] [-c COMMENT] [--watch]
gh envy reject  [<run-id> | <run-url>] [-e ENV]... [-w WORKFLOW] [-b BRANCH] [-c COMMENT]
```

Approves or rejects a run waiting at required reviewers. A rejected run fails.

The run is an ID or a URL such as `https://github.com/OWNER/REPO/actions/runs/123`, with or without a `/job/N` or `/attempts/N` suffix. A URL selects its repository and overrides `-R`. Without a run, gh envy looks at the waiting runs that match `-e`, `-w` and `-b`. It uses the only match, asks which one in a terminal, and fails listing the candidates otherwise.

`-e` names the environments to review and fails if the run isn't waiting on one of them. Without `-e`, gh envy acts on every environment of the run you can review. In a terminal it asks first, with all of them selected. Environments you can't review are skipped with a warning that says why. If none are left, the command fails.

`-c` records a comment with the review. `approve --watch` runs `gh run watch` on the run afterwards and needs a terminal.

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

`view` also accepts:

| Field | Type |
| --- | --- |
| `lastDeployment` | `{sha, ref, creator, state, createdAt, url}`, or `null` with no deployments. `state` comes from the latest status, `url` is its environment URL or log URL. |
| `waitingRuns` | runs waiting at this environment, as `waiting --json` returns them |

Fields that need extra API calls (`branchPolicies`, `protectionApps`, `variables`, `secrets`, `lastDeployment`, `waitingRuns`) are fetched only when requested.

`waiting` accepts:

| Field | Type |
| --- | --- |
| `runId`, `url` | run ID and web URL |
| `workflow`, `title`, `branch`, `event` | workflow name, run title, head branch, triggering event |
| `actor` | login of the user who triggered the run |
| `createdAt` | timestamp |
| `environment`, `environmentId` | the environment the run waits on |
| `canApprove` | whether you can review this gate |
| `reviewers` | `[{type, name, id}]`, as in `view` |
| `waitTimer`, `waitTimerStartedAt` | minutes, and when the timer started or `null` |

## Plans and permissions

- Managing environments needs admin access to the repository. A token with the `repo` scope is enough.
- Approving and rejecting needs write access to the repository and a place among the environment's required reviewers, directly or through a team. With prevent-self-review on, GitHub refuses a review from the person who triggered the run, even a repository admin with admin bypass on.
- On private repositories, required reviewers and wait timers need GitHub Pro, Team or Enterprise. Branch policies and admin bypass work on every plan. gh envy only sends the plan-gated fields when they are set or being cleared, so plain environments work on free plans.
