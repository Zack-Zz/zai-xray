# Zai Xray（中文说明）

开源、终端优先的 AI 调用追踪与调试 CLI。

`zai` 让你像用 `strace/tcpdump` 一样看到每次 AI 调用的 token、成本、延迟、错误、重试和统计。

## 功能（v0.1）

- `zai run`：直接调用 provider（当前支持 OpenAI）
- `zai wrap`：包装现有 CLI，记录执行与代理层元信息
- `zai wrap` 代理支持 HTTPS `CONNECT` 隧道透传
- `zai trace list/show`：本地查询追踪记录
- `zai stats`：按时间窗口统计调用指标
- `zai doctor`：环境自检
- SQLite 本地持久化
- 输出模式：pretty（默认）/ one-line / json
- provider 未返回 usage 时支持 token/cost 回退估算

## 安装与构建

前置要求：

- Go `1.26+`
- 系统 `PATH` 中可用 `sqlite3` 命令

构建：

```bash
go mod tidy
go build -o zai ./cmd/zai
```

## 快速开始

```bash
export OPENAI_API_KEY=你的_key

# 直接调用
./zai run "解释这个报错"

# 脚本场景推荐 JSON
./zai run "总结日志" --json

# 查看 trace 列表
./zai trace list --limit 20

# 查看单条 trace
./zai trace show <trace_id>

# 统计
./zai stats today --by model

# 自检
./zai doctor
```

## 命令总览

- `zai run "<prompt>" [--provider --model --stdin --timeout --json --one-line --pretty --no-color]`
- `zai wrap <target> -- <target_args...> [--json --one-line --pretty --no-color --env --bin --no-proxy --timeout]`
- `zai trace list [--limit --source --since --json --one-line --pretty --no-color]`
- `zai trace show <trace_id> [--full --json --one-line --pretty --no-color]`
- `zai stats [today|yesterday|last7d] [--by model|provider|source --json --one-line --pretty --no-color]`
- `zai doctor [--json --one-line --pretty --no-color]`

## 输出模式

优先级：

1. `--json`
2. `--one-line`
3. `--pretty`（默认）

颜色策略：

- 非 TTY 自动关闭颜色
- `--no-color` 强制关闭颜色

## 本地数据路径

- 配置文件：`~/.zai/config.yaml`
- 数据库：`~/.zai/traces.db`

核心表：

- `traces`：状态、延迟、token、成本、错误、字节数等
- `artifacts`：prompt/response 摘要和哈希
- `schema_migrations`：schema 版本记录

## 配置示例

`~/.zai/config.yaml`：

```yaml
db_path: ~/.zai/traces.db
default_provider: openai
default_model: gpt-4o-mini
timeout: 30s
openai:
  api_key: ""
  base_url: https://api.openai.com
```

环境变量覆盖：

- `ZAI_DB_PATH`
- `ZAI_PROVIDER`
- `ZAI_MODEL`
- `ZAI_TIMEOUT`
- `OPENAI_API_KEY` / `ZAI_OPENAI_API_KEY`
- `OPENAI_BASE_URL` / `ZAI_OPENAI_BASE_URL`

## 开发与测试

```bash
make fmt
make vet
make test
make lint
```

```bash
go test ./...
go test -race ./...
go test -run Baseline ./internal/store -v
```

核心技术栈：

- Go 标准库命令解析与命令框架
- 本地配置文件解析 + 环境变量覆盖
- 通过本机 `sqlite3` 命令完成 SQLite 持久化

## 性能基线

- `trace list --limit 50` 与 `stats today` 的基线测试位于：
- `internal/store/perf_test.go`

## 技术设计文档

- [Zai_Xray_Technical_Design_v0_1.md](./Zai_Xray_Technical_Design_v0_1.md)
