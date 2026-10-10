# AI project guidelines

`gh-envy`: a `gh` CLI extension (Go) that manages GitHub deployment environments, invoked as `gh envy`. It covers what core `gh` lacks (cli/cli#5149): environment CRUD, protection rules, branch/tag deployment policies and custom protection apps, with `--json/--jq/--template` output in core `gh` style.

- Key commands
  - `go build -o gh-envy . && gh extension install .` (local install, rebuild to update)
  - `go test ./...`
  - `golangci-lint run`

- Conventions
  - UX mirrors core `gh` (flags, `-R`, exporters, prompts, exit codes). When in doubt, copy how `gh variable` / `gh secret` behave.
  - Built on `github.com/cli/go-gh/v2` and cobra, the same stack as `gh`.

- Iron Laws
  - Tokens are expensive, state of the art models need minimal guidance, don't repeat yourself, don't babysit, don't be over-specific.
  - AI-native project. All code is AI-generated.
  - Minimal attention when model implements without errors, we document in more detail when model struggles.
  - Do not expect the user to have read each line, don't lose him on the internals, give visibility on a higher-architectural level.
  - No journaling: code comments / documentation describe current state, they don't carry a log of their own edit history.
