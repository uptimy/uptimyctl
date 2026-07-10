# AGENTS.md

This repository is a Go CLI for managing upti.my workspaces.

## Build and Validation

- Build: `make build`
- Test: `make test`
- Lint: `make lint`
- Format: `make fmt`

## Authentication

- Preferred non-interactive auth for automation: `UPTIMYCTL_API_KEY=upt_...`
- Config file path: `~/.config/uptimyctl/config.yaml`
- Default API URL: `https://api.upti.my`
- Optional override for non-production or local API usage: `--api-url` or `UPTIMYCTL_API_URL`
- `incidents` and `maintenances` use a separate domain (default `https://workflows.upti.my`), overridable via `--incidents-api-url` or `UPTIMYCTL_INCIDENTS_API_URL`

## Machine-Friendly Usage

- Prefer `-o json` for automation and AI agents.
- Use environment variables instead of interactive login in CI.
- Example:

```bash
UPTIMYCTL_API_KEY=upt_... uptimyctl applications list -o json
UPTIMYCTL_API_KEY=upt_... uptimyctl healthchecks pause <uuid> -o json
UPTIMYCTL_API_KEY=upt_... uptimyctl export -f monitoring.json
```

## Configuring Monitoring End-to-End

A typical agent flow to set up and verify monitoring:

```bash
# 1. Create applications and healthchecks (one call, all-or-nothing)
uptimyctl healthchecks bulk -f monitors.json -o json

# 2. Wire up alerting per application (JSON spec via -f, or - for stdin)
uptimyctl alert-rules create --application <app-uuid> -f rule.json -o json

# 3. Publish a status page and group checks on it
uptimyctl status-pages create -f page.json -o json
uptimyctl status-pages groups create <status-page-uuid> -f group.json -o json

# 4. Verify: trigger a check and read its results
uptimyctl healthchecks trigger <hc-uuid> -o json
uptimyctl analytics executions <hc-uuid> -o json   # defaults to last 24h
```

Commands that take complex payloads (`healthchecks upsert|bulk`, `alert-rules create|update`, `status-pages create|update`, `status-pages groups create|update`) read a JSON spec with `-f <file>` or `-f -` for stdin; run them with `--help` to see the spec format.

## Notes

- Do not commit real API keys or workspace exports containing secrets.
- Release flow is tag-based via GitHub Actions and GoReleaser.
- Install/update script: `scripts/install.sh`
- Uninstall script: `scripts/uninstall.sh`