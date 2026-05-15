# uptimyctl

Command-line tool for managing [upti.my](https://upti.my) workspaces via API keys.

## Installation

### Quick install (Linux / macOS)

```bash
curl -sSfL https://raw.githubusercontent.com/uptimy/uptimyctl/master/scripts/install.sh | sudo bash
```

To install a specific version:

```bash
curl -sSfL https://raw.githubusercontent.com/uptimy/uptimyctl/master/scripts/install.sh | UPTIMYCTL_VERSION=1.0.0 sudo bash
```

### Install with `go install`

```bash
go install github.com/uptimy/uptimyctl@latest
```

### Build from source

Requires Go 1.25+.

```bash
git clone https://github.com/uptimy/uptimyctl.git
cd uptimyctl
make build
```

The binary is placed in `bin/uptimyctl`. To install it system-wide:

```bash
sudo cp bin/uptimyctl /usr/local/bin/
```

### Uninstall

```bash
curl -sSfL https://raw.githubusercontent.com/uptimy/uptimyctl/master/scripts/uninstall.sh | sudo bash
```

To keep your config (`~/.config/uptimyctl`):

```bash
curl -sSfL https://raw.githubusercontent.com/uptimy/uptimyctl/master/scripts/uninstall.sh | UPTIMYCTL_KEEP_CONFIG=1 sudo bash
```

## Authentication

Create an API key at **Settings > API Keys** in the upti.my dashboard, then:

```bash
# Interactive login (saves to ~/.config/uptimyctl/config.yaml)
uptimyctl auth login

# Or use environment variable
export UPTIMYCTL_API_KEY=upt_abc123...

# Or pass per-command
uptimyctl --api-key upt_abc123... applications list
```

## Commands

### Applications

```bash
uptimyctl applications list
uptimyctl apps list                              # alias
uptimyctl applications get <uuid>
```

### Healthchecks

```bash
uptimyctl healthchecks list
uptimyctl hc list                                # alias
uptimyctl healthchecks get <uuid>
uptimyctl healthchecks trigger <uuid>            # trigger immediate check
uptimyctl healthchecks pause <uuid>              # disable/pause healthcheck
uptimyctl healthchecks resume <uuid>             # enable/resume healthcheck
uptimyctl healthchecks delete <uuid>             # delete healthcheck
```

### Incidents

```bash
uptimyctl incidents list
uptimyctl incidents list --status Resolved --severity critical
uptimyctl incidents get <uuid>
uptimyctl incidents stats

uptimyctl incidents create --title "API Down" --severity critical --public
uptimyctl incidents update <uuid> --title "API Down" --status Investigating
uptimyctl incidents resolve <uuid>
uptimyctl incidents add-update <uuid> --message "Root cause found" --public
```

### Maintenances

```bash
uptimyctl maintenances list
uptimyctl maint list                             # alias

uptimyctl maintenances create \
  --start-at "2026-04-10T02:00:00Z" \
  --finish-at "2026-04-10T04:00:00Z" \
  --description "Database migration"

uptimyctl maintenances resolve <uuid>            # resolve now
uptimyctl maintenances resolve <uuid> --resolved-at "2026-04-10T03:30:00Z"
uptimyctl maintenances delete <uuid>
```

### Schedulers (Regions)

```bash
uptimyctl schedulers list
uptimyctl regions list                           # alias
uptimyctl schedulers get <uuid>
```

### Export / Import

```bash
uptimyctl export                                 # print config to stdout
uptimyctl export -f config.json                  # save to file
uptimyctl import config.json                     # import from file
cat config.json | uptimyctl import -             # import from stdin
```

### Version

```bash
uptimyctl version
```

## Global Flags

| Flag | Description |
|------|-------------|
| `--api-key` | API key (overrides config and env) |
| `--api-url` | API base URL (overrides config and env) |
| `-o, --output` | Output format: `table` (default), `json` |

## Automation

`uptimyctl` is designed to work well in CI, scripts, and AI-driven tooling.

```bash
# Non-interactive auth
UPTIMYCTL_API_KEY=upt_abc123... uptimyctl applications list -o json

# Pause or resume a healthcheck
UPTIMYCTL_API_KEY=upt_abc123... uptimyctl healthchecks pause <uuid> -o json
UPTIMYCTL_API_KEY=upt_abc123... uptimyctl healthchecks resume <uuid> -o json

# Export and import workspace configuration
UPTIMYCTL_API_KEY=upt_prod... uptimyctl export -f monitoring.json -o json
UPTIMYCTL_API_KEY=upt_staging... uptimyctl import monitoring.json -o json

# Inspect auth or version as JSON
uptimyctl auth status -o json
uptimyctl version -o json
```

Config file location: `~/.config/uptimyctl/config.yaml`

```yaml
api_url: https://api.upti.my
api_key: upt_abc123...
```

Environment variables override the config file:

| Variable | Description |
|---|---|
| `UPTIMYCTL_API_KEY` | API key |
| `UPTIMYCTL_API_URL` | API base URL |

## Development

```bash
make fmt          # format code
make vet          # run go vet
make lint         # run golangci-lint
make test         # run tests with race detector
make coverage     # generate coverage report
make tidy         # go mod tidy
```

## License

See [LICENSE](LICENSE) for details.