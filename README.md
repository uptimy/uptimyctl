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

```bash
# Opens your browser: pick a workspace and approve. uptimyctl exchanges the
# one-time code it gets back (OAuth with PKCE) for an API key and saves it to
# ~/.config/uptimyctl/config.yaml. Keys are named "uptimyctl (<hostname>)"
# and expire after 90 days.
uptimyctl auth login

# Over SSH or on a headless machine: open the printed URL anywhere, paste the code
uptimyctl auth login --no-browser

# Or pipe an existing key (create one at Settings > API Keys)
echo "$UPTIMY_KEY" | uptimyctl auth login

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
uptimyctl applications create --name "api" --description "Public API"
uptimyctl applications update <uuid> --name "api-v2"
uptimyctl applications delete <uuid>             # deletes healthchecks and alert rules too
```

### Healthchecks

```bash
uptimyctl healthchecks list
uptimyctl hc list                                # alias
uptimyctl healthchecks get <uuid>
uptimyctl healthchecks upsert --application <app-uuid> -f check.json   # create or update
uptimyctl healthchecks bulk -f monitors.json     # create apps + checks in one call
uptimyctl healthchecks trigger <uuid>            # trigger immediate check
uptimyctl healthchecks pause <uuid>              # disable/pause healthcheck
uptimyctl healthchecks resume <uuid>             # enable/resume healthcheck
uptimyctl healthchecks delete <uuid>             # delete healthcheck
```

See `uptimyctl healthchecks upsert --help` and `uptimyctl healthchecks bulk --help` for the JSON spec formats.

### Alert Rules

```bash
uptimyctl alert-rules list --application <app-uuid>
uptimyctl ar list --application <app-uuid>       # alias
uptimyctl alert-rules get <uuid> --application <app-uuid>
uptimyctl alert-rules create --application <app-uuid> -f rule.json
uptimyctl alert-rules update <uuid> --application <app-uuid> -f rule.json
uptimyctl alert-rules delete <uuid> --application <app-uuid>
```

### Status Pages

```bash
uptimyctl status-pages list
uptimyctl sp list                                # alias
uptimyctl status-pages get <uuid>
uptimyctl status-pages create -f page.json
uptimyctl status-pages update <uuid> -f page.json
uptimyctl status-pages delete <uuid>

uptimyctl status-pages groups list <status-page-uuid>
uptimyctl status-pages groups create <status-page-uuid> -f group.json
uptimyctl status-pages groups update <status-page-uuid> <group-uuid> -f group.json
uptimyctl status-pages groups delete <status-page-uuid> <group-uuid>
```

### Analytics

```bash
# Defaults to the last 24 hours; --from/--to take RFC3339 timestamps
uptimyctl analytics executions <healthcheck-uuid>
uptimyctl analytics minute-region <healthcheck-uuid> --scheduler <scheduler-uuid>
uptimyctl analytics minute-breakdown <healthcheck-uuid>
uptimyctl analytics hour-region <healthcheck-uuid> --from 2026-07-01T00:00:00Z --to 2026-07-08T00:00:00Z
uptimyctl analytics daily-region <healthcheck-uuid>
uptimyctl analytics monthly-region <healthcheck-uuid>
```

### Incidents

```bash
uptimyctl incidents list
uptimyctl incidents list --status Resolved --severity critical
uptimyctl incidents get <uuid>
uptimyctl incidents stats

# Create an incident and publish it to a status page in one call
uptimyctl incidents create --title "API Down" --severity critical \
  --status-page <status-page-uuid>

# Partial update: only the flags you pass are changed
uptimyctl incidents update <uuid> --status Investigating

# Post a public update, optionally transitioning status in the same command
uptimyctl incidents add-update <uuid> --message "Root cause found" --status Identified
uptimyctl incidents updates <uuid>               # list the timeline

# Publish/unpublish on status pages without touching other fields
uptimyctl incidents publish <uuid> --status-page <status-page-uuid>
uptimyctl incidents unpublish <uuid> --all

# Resolve with a final public note
uptimyctl incidents resolve <uuid> --message "Fix deployed, all systems normal."

# Attach a post-mortem (appended to the description and posted as a public update)
uptimyctl incidents post-mortem <uuid> -f post-mortem.md
```

Incident statuses: `Created`, `Acknowledged`, `Investigating`, `Identified`, `Monitoring`, `Resolved`. An incident is visible on a status page once published to it; individual updates appear publicly only when posted with `--public=true` (the default).

### Maintenances

```bash
uptimyctl maintenances list
uptimyctl maint list                             # alias

uptimyctl maintenances create \
  --title "Database migration" \
  --start-at "2026-04-10T02:00:00Z" \
  --finish-at "2026-04-10T04:00:00Z" \
  --description "Primary DB failover, expect brief write errors" \
  --status-page <status-page-uuid>

uptimyctl maintenances update <uuid> --finish-at "2026-04-10T05:00:00Z"
uptimyctl maintenances start <uuid>              # -> In Progress
uptimyctl maintenances resolve <uuid>            # -> Completed
uptimyctl maintenances cancel <uuid>             # -> Cancelled
uptimyctl maintenances delete <uuid>
```

### Heartbeats

Dead man's switches for cron jobs and workers: a heartbeat alerts when check-ins stop.

```bash
uptimyctl heartbeats list
uptimyctl heartbeats create --name "Nightly backup" --interval 24h --grace 30m   # prints the ping URL
uptimyctl heartbeats create -f heartbeat.json            # full spec, e.g. multi-step heartbeats
uptimyctl heartbeats create --name "Backup" --client-ref backup-prod             # idempotent: reruns reuse it
uptimyctl heartbeats get <uuid>                          # includes pingUrl
uptimyctl heartbeats update <uuid> --interval 12h        # only the flags you pass change
uptimyctl heartbeats pause <uuid>
uptimyctl heartbeats resume <uuid>
uptimyctl heartbeats ping <uuid>                         # send a test check-in
uptimyctl heartbeats delete <uuid>
```

Durations accept `90s`, `5m`, `1h`, `24h` or plain seconds. New heartbeats default to `--interval 1h --grace 5m`, like the web app. In the monitored job itself, call the ping URL directly (no API key needed):

```bash
0 3 * * *  /usr/local/bin/backup.sh && curl -fsS https://heartbeats.upti.my/v1/monitors/<id>
```

### Dependencies

Follow the third-party services Uptimy measures (GitHub, Cloudflare, Stripe, ...). When a followed service goes down, your workspace gets its own incident and your alert workflows run.

```bash
uptimyctl dependencies catalog                   # every service you can follow, with status and 30-day uptime
uptimyctl dependencies list                      # what this workspace follows
uptimyctl dependencies get github                # probes (website, API, ...), their status, which ones you follow
uptimyctl dependencies follow github             # follow every probe
uptimyctl dependencies follow github --probe rest-api --probe git-operations   # exactly these probes
uptimyctl dependencies unfollow github           # also resolves its open incident here
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
| `--incidents-api-url` | API base URL for incidents and maintenances (overrides config and env) |
| `--heartbeats-api-url` | API base URL for heartbeats (overrides config and env) |
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
incidents_api_url: https://workflows.upti.my
heartbeats_api_url: https://heartbeats.upti.my
api_key: upt_abc123...
```

> `incidents` and `maintenances` are served from a separate domain (`incidents_api_url`), and `heartbeats` from another (`heartbeats_api_url`); all other resources use `api_url`.

Environment variables override the config file:

| Variable | Description |
|---|---|
| `UPTIMYCTL_API_KEY` | API key |
| `UPTIMYCTL_API_URL` | API base URL |
| `UPTIMYCTL_INCIDENTS_API_URL` | API base URL for incidents and maintenances |
| `UPTIMYCTL_HEARTBEATS_API_URL` | API base URL for heartbeats |

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