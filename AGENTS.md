# AI project guidelines

`gh-envy`: a `gh` CLI extension (Go) that manages GitHub deployment environments, invoked as `gh envy`. It covers what core `gh` lacks (cli/cli#5149): environment CRUD, protection rules, branch/tag deployment policies and custom protection apps, with `--json/--jq/--template` output in core `gh` style.

- Key commands
  - `go build -o gh-envy . && gh extension install .` (local install, rebuild to update)
  - `go test ./...`
  - `golangci-lint run`

- Key documentation
  - [docs/commands.md](docs/commands.md)

- Conventions
  - UX mirrors core `gh` (flags, `-R`, exporters, prompts, exit codes). When in doubt, copy how `gh variable` / `gh secret` behave.
  - Built on `github.com/cli/go-gh/v2` and cobra, the same stack as `gh`.

- Iron Laws
  - State of the art AI models/harnesses need minimal guidance. Don't repeat yourself, don't babysit, don't be over-specific.
  - Only document items here when model diverges repeatedly from expected behavior. Don't document behaviors implemented without errors.
  - No journaling: code comments / documentation describe current state, they don't carry a log of their own edit history.
