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

## Notes

- Do not commit real API keys or workspace exports containing secrets.
- Release flow is tag-based via GitHub Actions and GoReleaser.
- Install/update script: `scripts/install.sh`
- Uninstall script: `scripts/uninstall.sh`