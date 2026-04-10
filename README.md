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

### Export / Import

```bash
# Export workspace config to file
uptimyctl export -f config.json

# Export to stdout (pipe to jq, etc.)
uptimyctl export | jq .

# Import config into workspace
uptimyctl import config.json

# Import from stdin
cat config.json | uptimyctl import -
```

## CI/CD Examples

### GitHub Actions — Maintenance Windows

```yaml
name: Deploy
on:
  push:
    branches: [main]

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - name: Start maintenance
        id: maint
        run: |
          RESULT=$(uptimyctl maintenances create \
            --start-at "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
            --finish-at "$(date -u -d '+1 hour' +%Y-%m-%dT%H:%M:%SZ)" \
            --description "Deploying ${{ github.sha }}" \
            -o json)
          echo "uuid=$(echo $RESULT | jq -r .data.uuid)" >> $GITHUB_OUTPUT
        env:
          UPTIMYCTL_API_KEY: ${{ secrets.UPTIMYCTL_API_KEY }}

      - name: Deploy application
        run: # your deploy steps here

      - name: End maintenance
        if: always()
        run: uptimyctl maintenances resolve ${{ steps.maint.outputs.uuid }}
        env:
          UPTIMYCTL_API_KEY: ${{ secrets.UPTIMYCTL_API_KEY }}
```

### Terraform-style Config Management

```bash
# Export from production workspace
UPTIMYCTL_API_KEY=upt_prod... uptimyctl export -f monitoring.json

# Import into staging workspace
UPTIMYCTL_API_KEY=upt_staging... uptimyctl import monitoring.json
```

## Output Formats

```bash
uptimyctl applications list                  # table (default)
uptimyctl applications list -o json          # JSON
```

## Configuration

Config file location: `~/.config/uptimyctl/config.yaml`

```yaml
api_url: https://api.upti.my
api_key: upt_abc123...
```

Environment variables (override config file):

| Variable | Description |
|---|---|
| `UPTIMYCTL_API_KEY` | API key |
| `UPTIMYCTL_API_URL` | API base URL |

Flag overrides (highest priority):

```bash
uptimyctl --api-key upt_... --api-url https://custom.api.com applications list
```

## License

Apache License 2.0 - see [LICENSE](LICENSE).