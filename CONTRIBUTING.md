# Contributing to uptimyctl

Thanks for helping. Bug reports, fixes and new commands are all welcome. This page covers how to get set up, how the code is laid out, and what a good command looks like.

For anything bigger than a fix (a new command, a change in behavior or in the workspace file format), please [open an issue](https://github.com/uptimy/uptimyctl/issues/new/choose) first so we can agree on the approach before you spend time on it.

## Setting up

You need Go 1.25+.

```bash
git clone https://github.com/uptimy/uptimyctl && cd uptimyctl
make build
./bin/uptimyctl auth login
```

`auth login` opens your browser to approve a key for one of your Uptimy workspaces. Commands that change things change that workspace, so try them on one you don't mind changing.

| Command | What it does |
| --- | --- |
| `make test` | Tests with the race detector |
| `make lint` | [golangci-lint](https://golangci-lint.run/welcome/install/), the same as CI |
| `make build` | A binary in `bin/` |
| `make fmt` | `go fmt` |

## How the code is laid out

```
main.go                entrypoint
cmd/                   the commands (cobra), one file per resource: healthchecks.go, incidents.go, reports.go, ...
cmd/workspace_file.go  export, plan and apply
internal/workspace/    the workspace file: its format, reading the live workspace, the plan and applying it
internal/client/       HTTP client for Uptimy's API
internal/output/       tables and -o json
internal/config/       ~/.config/uptimyctl/config.yaml, environment variables and flags
internal/selfupdate/   uptimyctl update
internal/updatecheck/  the "newer release" notice
```

Each command calls Uptimy's `/v1/api/*` routes with the workspace's API key. Most live on `api.upti.my` (`newClient()`); incidents and maintenances on `workflows.upti.my` (`newIncidentsClient()`), heartbeats on `heartbeats.upti.my` (`newHeartbeatsClient()`) and reports on `reports.upti.my` (`newReportsClient()`).

## Adding a command

Commands live in the file for their resource and register themselves in `init()`; see `cmd/tags.go` for a small one. A good command:

- prints a table by default and the API's JSON with `-o json` (`output.IsJSON()`), so scripts and agents can rely on it
- says what went wrong in a sentence someone can act on (`exitErr`), and accepts names wherever people would rather not copy a UUID
- has a `Long` help text with an example, since `--help` is the reference

Tests sit next to the code (`cmd/*_test.go`, `internal/workspace/workspace_test.go`); the workspace file's tests run plan and apply against a fake API.

**Changing the workspace file** (`internal/workspace`): `export` then `plan` against the same workspace must show no changes, and a file written by an older version must still apply without removing anything it doesn't mention. The format is documented in [AGENTS.md](AGENTS.md#the-workspace-as-a-file); update it with the code.

**Document it**: the README's command list, and AGENTS.md if agents should know about it.

## Pull requests

- Keep a PR to one change, with tests. `make lint` and `make test` should pass; CI runs both and builds for Linux and macOS.
- Describe what changes for people using uptimyctl, not only the code.
- Match the code around you: comments explain *why*, errors are sentences a user can act on, and nothing prints or logs an API key.
- By contributing you agree your work is licensed under the [Apache 2.0 license](LICENSE).

Found a security issue? Please don't open an issue; see [SECURITY.md](SECURITY.md).
