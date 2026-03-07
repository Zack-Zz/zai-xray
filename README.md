# Zai Xray

Open-source, terminal-first AI tracing and debugging CLI.

`zai` helps you inspect every AI call like `strace/tcpdump`: tokens, cost, latency, errors, retries, and historical stats.

## Features (v0.1)

- `zai run` direct provider call (OpenAI)
- `zai wrap` wrap an existing CLI command and record execution/proxy-level metadata
- HTTPS `CONNECT` tunnel pass-through in wrap proxy mode
- `zai trace list/show` inspect local traces
- `zai stats` aggregate daily and weekly usage
- `zai doctor` local environment diagnostics
- Local-first persistence with SQLite
- Output modes for humans and scripts: pretty (default), one-line, JSON
- Usage/cost fallback estimation when provider usage data is missing

## Install

### Prerequisites

- Go `1.26+`
- `sqlite3` CLI available in `PATH`

### Build

```bash
go mod tidy
go build -o zai ./cmd/zai
```

## Quick Start

```bash
export OPENAI_API_KEY=your_key_here

# direct call
./zai run "explain this stacktrace"

# pretty output is default; use JSON for scripting
./zai run "summarize logs" --json

# list traces
./zai trace list --limit 20

# show one trace
./zai trace show <trace_id>

# aggregated stats
./zai stats today --by model

# diagnostics
./zai doctor
```

## Commands

- `zai run "<prompt>" [--provider --model --stdin --timeout --json --one-line --pretty --no-color]`
- `zai wrap <target> -- <target_args...> [--json --one-line --pretty --no-color --env --bin --no-proxy --timeout]`
- `zai trace list [--limit --source --since --json --one-line --pretty --no-color]`
- `zai trace show <trace_id> [--full --json --one-line --pretty --no-color]`
- `zai stats [today|yesterday|last7d] [--by model|provider|source --json --one-line --pretty --no-color]`
- `zai doctor [--json --one-line --pretty --no-color]`

## Output Modes

Priority order:

1. `--json`
2. `--one-line`
3. `--pretty` (default)

Color behavior:

- Disabled automatically for non-TTY output
- Disabled manually with `--no-color`

## Local Data

- Config: `~/.zai/config.yaml`
- SQLite DB: `~/.zai/traces.db`

Schema highlights:

- `traces`: core trace metadata (status, latency, tokens, cost, errors, bytes)
- `artifacts`: prompt/response previews + hashes
- `schema_migrations`: schema version tracking

## Configuration

Example `~/.zai/config.yaml`:

```yaml
db_path: ~/.zai/traces.db
default_provider: openai
default_model: gpt-4o-mini
timeout: 30s
openai:
  api_key: ""
  base_url: https://api.openai.com
```

Environment overrides:

- `ZAI_DB_PATH`
- `ZAI_PROVIDER`
- `ZAI_MODEL`
- `ZAI_TIMEOUT`
- `OPENAI_API_KEY` / `ZAI_OPENAI_API_KEY`
- `OPENAI_BASE_URL` / `ZAI_OPENAI_BASE_URL`

## Development

```bash
make fmt
make vet
make test
make lint
```

## Testing

```bash
go test ./...
go test -race ./...
go test -run Baseline ./internal/store -v
```

Core stack:

- Go standard library CLI parser + command framework
- Local config parsing with env overrides
- SQLite persistence through local `sqlite3` command integration

## Performance Baseline

- `trace list --limit 50` and `stats today` baselines are covered by store benchmark/tests in:
- `internal/store/perf_test.go`

## Roadmap

Detailed technical roadmap:

- [Zai_Xray_Technical_Design_v0_1.md](./Zai_Xray_Technical_Design_v0_1.md)

## License

MIT (recommended; add `LICENSE` file if not present).
