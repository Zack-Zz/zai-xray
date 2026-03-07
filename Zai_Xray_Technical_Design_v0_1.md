# Zai Xray（zai）技术文档 & 技术路线图（v0.1）

> **定位**：开源、终端优先的 AI 调用调试与追踪工具（AI debugging & tracing CLI）。  
> **一句话**：**像 `strace/tcpdump` 一样在终端里看见每一次 AI 调用的 tokens / cost / latency / 错误 / 重试。**  
> **产品名**：Zai Xray  
> **CLI 命令**：`zai`  
> **仓库建议名**：`zai-xray`（repo），二进制命令名：`zai`

---

## 目录

- [1. 项目描述](#1-项目描述)
- [2. 目标用户与典型场景](#2-目标用户与典型场景)
- [3. 核心价值与非目标](#3-核心价值与非目标)
- [4. 产品愿景](#4-产品愿景)
- [5. 功能列表（v0.1/v0.2/v1.0）](#5-功能列表v01v02v10)
- [6. 技术框架与总体架构](#6-技术框架与总体架构)
- [7. Trace 数据模型（本地）](#7-trace-数据模型本地)
- [8. Provider/解析策略与成本估算](#8-provider解析策略与成本估算)
- [9. 命令与使用清单（尽量具体）](#9-命令与使用清单尽量具体)
- [10. 配置与密钥管理](#10-配置与密钥管理)
- [11. 安全与隐私设计](#11-安全与隐私设计)
- [12. 开发路线图（可执行）](#12-开发路线图可执行)
- [13. 测试计划](#13-测试计划)
- [14. 发布与分发（开源优先）](#14-发布与分发开源优先)
- [15. 里程碑指标（开源成功指标）](#15-里程碑指标开源成功指标)
- [16. FAQ](#16-faq)

---

## 1. 项目描述

### 1.1 背景

开发者在使用 AI（OpenAI / Anthropic / Gemini 等）或 AI CLI（Claude CLI / Codex / Kiro 等）时，经常遇到：

- **为什么这次调用这么慢？**（TTFT、网络、重试、流式输出）
- **为什么 tokens 这么高、费用这么贵？**（prompt 太长？输出太啰嗦？重复调用？）
- **为什么失败？**（429、5xx、网络错误、参数错误）
- **今天到底花了多少钱？最贵的 prompt 是什么？**

现有解决方式通常是：去某个 SaaS 平台、或者接入 SDK、或者把请求改走 gateway——对个人开发者和轻量场景来说偏重。

### 1.2 Zai Xray 做什么

Zai Xray 的 v0.1 聚焦在 **“AI CLI debugging”**（你选的 A 路线）：

- **本地优先、默认不上传**
- **终端可见、可复制/可查询**
- **不要求改代码**
- **先把“tokens/cost/latency/retry/error”这个闭环做极致**

---

## 2. 目标用户与典型场景

### 2.1 目标用户

- 使用 AI API 的开发者（后端/全栈/数据/AI 应用开发）
- 使用 AI CLI 的开发者（Claude CLI / Codex / Kiro / 自研脚本）
- 需要在本机快速定位「慢/贵/失败」原因的人

### 2.2 典型场景（MVP 必须覆盖）

1. **终端里直接问 AI：**
   - `zai run "explain this stacktrace"`  
   - 立即看到：模型、耗时、tokens、cost、错误码

2. **包装现有 CLI：**
   - `zai wrap claude -- <原参数…>`  
   - 记录该 CLI 产生的模型请求（至少 latency/失败/重试；能拿 usage 则拿 usage）

3. **日常统计：**
   - `zai stats today`：今天总花费、最贵 top N、最慢 top N

4. **回看一次调用：**
   - `zai trace show <trace_id>`：请求/响应概要、usage、耗时分解、错误与重试

---

## 3. 核心价值与非目标

### 3.1 核心价值（v0.1）

- **一条命令可用**：`zai run ...` / `zai wrap ...`  
- **本地可追溯**：SQLite 保存 trace，可查询/过滤/聚合
- **对用户透明**：输出清晰的可读报告（人类读 + 机器可读 JSON）

### 3.2 非目标（v0.1 不做）

- 不做完整 Agent tracing（tool call/MCP/rules）——留到 v2+
- 不做 GUI（先 CLI）
- 不做云端 SaaS（先开源验证）
- 不做复杂 eval / benchmark（后续可扩展）

---

## 4. 产品愿景

### 4.1 愿景（长线）

把 AI 交互变成像微服务一样可观测：

- trace（调用链）
- metrics（成本/耗时/失败率）
- logs（结构化记录）

最终形态可以演进为：

- **AI Jaeger / AI Datadog**（但注意：开源阶段先做 CLI）

### 4.2 开源阶段的愿景（短线）

成为开发者日常工具箱的一部分：

- `zai` 像 `jq` / `rg` 一样常驻
- “今天 AI 花多少钱 / 哪次最慢”成为日常命令

---

## 5. 功能列表（v0.1/v0.2/v1.0）

### 5.1 v0.1（MVP：两周~三周）

**必做：**
- `zai run`：直接调用 provider（至少 OpenAI）
- `zai trace list/show`：本地回看
- `zai stats`：聚合统计（today / last7d）
- 本地 SQLite 持久化
- 基础成本估算（按模型价表）
- 输出支持 `--json`（便于脚本集成）

**可选（强烈推荐，但不挡 MVP）：**
- `zai wrap`：包装至少一个 AI CLI（优先 Claude CLI，因支持 proxy）
- `zai doctor`：环境自检（key、依赖、网络、可写目录）

### 5.2 v0.2（加强版）

- 支持 Anthropic provider（Claude API）
- proxy 增强：HTTPS CONNECT、重试检测
- tokens 估算策略完善（usage 缺失时 fallback）
- redaction（脱敏）规则：token / key / email / phone 等

### 5.3 v1.0（开源稳定版）

- 覆盖更多入口：codex/kiro（基于 wrapper/proxy，按可用性逐个适配）
- 更完整的 timeline：TTFT（time-to-first-token）、stream duration
- 输出格式增强：markdown report / json report / minimal one-line
- 更强 stats：按 provider/model/command 分组、趋势

---

## 6. 技术框架与总体架构

### 6.1 技术栈（Go）

- 语言：Go
- CLI 框架：`cobra`
- 本地存储：SQLite（建议 `modernc.org/sqlite` 或 `mattn/go-sqlite3`）
- 配置：YAML（`gopkg.in/yaml.v3`）+ env override
- HTTP 客户端：`net/http`
- （可选）tokenizer：tiktoken-go（估算 tokens）

### 6.2 模块划分

```
cmd/
  zai/                 # CLI 入口
internal/
  cli/                 # cobra commands
  execution/           # run/wrap 的执行逻辑
  providers/           # openai/anthropic 等 provider 适配器
  proxy/               # 本地代理（可选，wrap 模式必需）
  trace/               # trace/span 数据结构与采集器
  store/               # sqlite 存取
  config/              # 配置加载与合并
  redact/              # 脱敏/过滤（v0.2+）
pkg/
  tokenizer/           # tokens 估算（可选）
```

### 6.3 两条主路径（Run vs Wrap）

#### A) Run：直接调用（最稳）

```
zai run
  -> provider.Call()
  -> 收到响应（含 usage）
  -> 计算 cost
  -> 写 trace 到 sqlite
  -> 输出结果
```

优点：信息最完整（tokens/cost 几乎都可得）  
缺点：只覆盖 `zai run` 触发的调用

#### B) Wrap：包装现有 CLI（最省用户心智）

```
zai wrap claude -- <args...>
  -> 启动本地 proxy（127.0.0.1:port）
  -> 生成 TRACE_ID
  -> spawn 子进程（claude）
     注入 HTTP(S)_PROXY/TRACE_ID 等 env（仅对子进程生效）
  -> proxy 捕获请求/响应与耗时
  -> 尽量解析 usage（若能）
  -> 写 trace 到 sqlite
  -> 输出摘要（one-line + trace_id）
```

优点：用户不用改代码；只需把命令前面加 `zai wrap`  
缺点：对“封闭工具”只能抓到边界（HTTP 请求级别），内部步骤不可见

---

## 7. Trace 数据模型（本地）

### 7.1 概念

- **Trace**：一次“用户命令触发的一次 AI 调用”（或一次会话的一个请求）
- **Span**：Trace 内部步骤（v0.1 可以非常少，后续扩展）

### 7.2 SQLite 表结构（建议 v0.1 先做够用）

> 时间统一使用 Unix ms（int64），方便统计与排序。

#### traces

```sql
CREATE TABLE IF NOT EXISTS traces (
  trace_id TEXT PRIMARY KEY,
  source TEXT NOT NULL,                 -- run / wrap:claude / wrap:codex ...
  command TEXT NOT NULL,                -- 完整命令（可截断存摘要 + 原文单独存）
  provider TEXT,                        -- openai / anthropic / unknown
  model TEXT,
  status TEXT NOT NULL,                 -- ok / error
  http_status INTEGER,
  error_type TEXT,                      -- timeout / rate_limit / auth / parse ...
  error_message TEXT,

  start_time_ms INTEGER NOT NULL,
  end_time_ms   INTEGER NOT NULL,
  latency_ms    INTEGER NOT NULL,

  prompt_tokens INTEGER,
  completion_tokens INTEGER,
  total_tokens INTEGER,
  tokens_estimated INTEGER DEFAULT 0,   -- 1 表示估算

  cost_usd REAL,
  cost_estimated INTEGER DEFAULT 0,     -- 1 表示估算

  retry_count INTEGER DEFAULT 0,

  request_bytes INTEGER,
  response_bytes INTEGER,

  created_at_ms INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_traces_created_at ON traces(created_at_ms);
CREATE INDEX IF NOT EXISTS idx_traces_source ON traces(source);
CREATE INDEX IF NOT EXISTS idx_traces_provider_model ON traces(provider, model);
```

#### artifacts（可选：存 prompt/response 摘要或 hash）

为避免把敏感内容直接落库，推荐 v0.1 默认只存 **hash + 截断 preview**：

```sql
CREATE TABLE IF NOT EXISTS artifacts (
  trace_id TEXT PRIMARY KEY,
  prompt_preview TEXT,       -- 默认截断 200~500 chars
  response_preview TEXT,     -- 默认截断
  prompt_sha256 TEXT,
  response_sha256 TEXT
);
```

> v0.2+ 可加入 `--store-full`（显式允许存完整内容）并配合脱敏。

---

## 8. Provider/解析策略与成本估算

### 8.1 Provider 适配器接口（Go）

```go
type Provider interface {
    Name() string
    Call(ctx context.Context, req CallRequest) (CallResponse, error)
    EstimateCost(model string, promptTokens, completionTokens int) (costUSD float64, ok bool)
}
```

### 8.2 usage 优先级

1. **优先使用 API 返回的 usage**（最准确）  
2. **若 usage 缺失**：
   - 尝试 tokenizer 估算 input/output token（标记 `tokens_estimated=1`）
3. **cost 计算**：
   - 若能找到该模型价表（input/output 单价）→ 计算并标记估算与否
   - 若找不到价表 → cost 为空（或标记 unknown）

### 8.3 成本价表（v0.1 简化做法）

- v0.1：在代码内置一个简单 `prices.json`（可更新）
- v0.2：允许用户覆盖/自定义价表（config）

---

## 9. 命令与使用清单（尽量具体）

### 9.1 `zai run`

```bash
zai run "<prompt>" [flags]
```

示例：

```bash
zai run "explain this stacktrace"
zai run "summarize this log" --provider openai --model gpt-4o
kubectl logs pod-xxx | zai run --stdin "summarize errors and give fixes"
zai run "explain this error" --json
zai run "..." --one-line
```

flags 建议：

- `--provider <openai|anthropic>`
- `--model <name>`
- `--stdin`
- `--json`
- `--one-line`
- `--timeout 30s`

---

### 9.2 `zai wrap`

```bash
zai wrap <target> -- <target_args...>
```

示例：

```bash
zai wrap claude -- "explain this error"
zai wrap custom --bin /path/to/somecli -- --help
```

flags 建议：

- `--json` / `--one-line`
- `--env KEY=VALUE`
- `--bin <path>`
- `--no-proxy`

---

### 9.3 `zai trace`

```bash
zai trace list [--limit 20] [--source run|wrap:claude] [--since 24h]
zai trace show <trace_id> [--json] [--full]
```

---

### 9.4 `zai stats`

```bash
zai stats [today|yesterday|last7d] [--by model|provider|source]
```

---

### 9.5 `zai doctor`

```bash
zai doctor
```

---

## 10. 配置与密钥管理

- `~/.zai/config.yaml`
- `~/.zai/traces.db`

密钥来源优先级：

1. flags
2. env
3. config

---

## 11. 安全与隐私设计

- 默认本地存储、默认不上传
- 默认仅存摘要与 hash
- `--store-full` 必须显式开启（v0.2+）
- v0.2+ 引入脱敏规则

---

## 12. 开发路线图（可执行）

Week 1：Run + SQLite + Trace  
Week 2：Wrap + Proxy（Claude）  
Week 3：Anthropic + 估算 + 稳定性

---

## 13. 测试计划

- provider 解析
- sqlite CRUD
- proxy 集成测试
- wrap e2e（mock CLI）

---

## 14. 发布与分发（开源优先）

- GitHub Releases 多平台二进制
- Homebrew/Scoop（后续）
- README 首屏展示 one-line 输出

---

## 15. 里程碑指标（开源成功指标）

- 100–200 stars：方向正确
- 1000 stars：可考虑 v2（agent/mcp）
- 5000 stars：考虑云端增值与商业化

---

## 16. FAQ

- CLI 为什么先免费：建立信任 + 分发
- 为什么先做 run：最稳闭环
- wrap 为什么后做：适配复杂、依赖外部工具行为

---

## 附录：建议默认输出

### one-line

```
TRACE 91b2  model=gpt-4o  latency=2.14s  tokens=in:312 out:188  cost=$0.0041  status=ok
```

### json

```json
{
  "trace_id": "91b2",
  "source": "run",
  "provider": "openai",
  "model": "gpt-4o",
  "latency_ms": 2140,
  "tokens": {"in": 312, "out": 188, "total": 500, "estimated": false},
  "cost_usd": 0.0041,
  "cost_estimated": false,
  "status": "ok",
  "retry_count": 0
}
```
