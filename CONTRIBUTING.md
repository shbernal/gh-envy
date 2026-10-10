# Contributing

Issues and pull requests are welcome, including ones written entirely by an AI agent.

## AI disclosure

If an AI tool wrote or substantially shaped your issue or pull request, say which harness and model, for example "Claude Code, Claude Opus 5.5". It helps reviewers know what to double-check.

## Setup

The Go version is pinned in `go.mod`. Hooks run through [lefthook](https://lefthook.dev):

```sh
lefthook install
```

## Checks

```sh
go test ./...
```

```sh
golangci-lint run
```

Tests run the commands against an in-process fake of the GitHub API (`internal/cmd/fake_test.go`), so they need no network or token.

## Try a local build

```sh
go build -o gh-envy . && gh extension install .
```

## Demos

The terminal animations in `assets/demo/` are produced by running the real commands against the in-memory repository in `internal/fakegh`. Regenerate them after changing output:

```sh
go run ./tools/demo-svg assets/demo
```

## Releases

Pushing a `v*` tag builds binaries for every platform with `cli/gh-extension-precompile` and publishes a GitHub release.
