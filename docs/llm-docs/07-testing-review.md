# New API — 测试体系与代码质量 Review

> 生成时间：2026-06-16
> 追踪方式：扫描全部 `*_test.go` 文件、CI workflow、lint/typecheck 配置

---

## 1. 当前测试框架

### 后端（Go）

| 项目 | 详情 |
|---|---|
| **测试框架** | Go 标准 `testing` 包 |
| **断言库** | `github.com/stretchr/testify`（`assert` + `require`） |
| **Mock 方式** | 内存 SQLite（`:memory:`）+ `httptest.NewRecorder` |
| **测试运行** | `go test ./...`（本地），**CI 中未配置自动运行** |
| **测试文件数** | 43 个 `_test.go` 文件 |
| **测试函数数** | 393 个 `Test*` 函数 |
| **源文件数** | 542 个 `.go` 文件（不含测试） |
| **测试/源文件比** | 43 / 542 = **7.9%** |

### 前端（TypeScript/React）

| 项目 | 详情 |
|---|---|
| **测试框架** | **未配置**（无 vitest/jest/mocha） |
| **测试文件** | 仅 1 个：`web/default/src/components/ui/dropdown-menu.test.tsx` |
| **package.json scripts** | 无 `test` 命令 |
| **状态** | 前端完全没有测试基础设施 |

---

## 2. 测试覆盖的模块

### 2.1 有测试的模块（按测试函数数排序）

| 模块 | 测试文件 | 测试函数数 | 覆盖内容 |
|---|---|---|---|
| `relay/common/` | `override_test.go` | **82** | 参数覆盖（param override）的各种模式 |
| `pkg/billingexpr/` | `billingexpr_test.go` | **56** | 计费表达式编译/执行/边界条件 |
| `relay/helper/` | `stream_scanner_test.go` | **25** | SSE 流式扫描器 |
| `service/` | `tiered_settle_test.go` | **30** | 分层计费结算 |
| `service/` | `task_billing_test.go` | **17** | 任务计费 |
| `relay/common/` | `stream_status_test.go` | **14** | 流式状态管理 |
| `relay/channel/claude/` | `relay_claude_test.go` | **13** | Claude 适配器请求转换 |
| `controller/` | `channel_upstream_update_test.go` | **12** | 上游模型同步逻辑 |
| `service/` | `text_quota_test.go` | **11** | 文本配额计算 |
| `model/` | `task_cas_test.go` | **10** | 任务 CAS 状态更新 |
| `middleware/` | `header_nav_test.go` | **10** | Header Nav 模块认证 |
| `service/` | `waffo_pancake_test.go` | **9** | Waffo Pancake 支付 |
| `controller/` | `token_test.go` | **9** | Token 自动迁移和 key 脱敏 |
| `service/` | `channel_affinity_template_test.go` | **8** | 渠道亲和性模板 |
| `setting/operation_setting/` | `status_code_ranges_test.go` | **8** | 状态码范围解析 |
| `relay/channel/openai/` | `image_stream_test.go` + `image_edit_test.go` | **5** | OpenAI 图片流/编辑 |
| `service/` | `error_test.go` | **5** | 错误处理 |
| `controller/` | `payment_webhook_availability_test.go` | **5** | 支付 Webhook 可用性 |

### 2.2 完全没有测试的模块（**63 个目录**）

以下目录包含业务代码但**零测试**：

#### 高风险无测试区域

| 目录 | 文件数（推测） | 风险等级 | 原因 |
|---|---|---|---|
| `relay/`（根目录 handler） | 14 | **极高** | `TextHelper`/`ClaudeHelper`/`GeminiHelper`/`ImageHelper`/`AudioHelper` 等核心中继 handler 全部无测试 |
| `middleware/` | 24 文件，仅 1 个有测试 | **极高** | `auth.go`（TokenAuth/UserAuth/AdminAuth）、`distributor.go`（渠道分配）、`rate-limit.go` 全部无测试 |
| `router/` | 6 | **高** | 路由注册无测试，依赖手动验证 |
| `oauth/` | 8 | **高** | GitHub/Discord/OIDC/LinuxDo/自定义 OAuth 全部无测试 |
| `i18n/` | 3 | **中** | 国际化加载和翻译逻辑无测试 |
| `types/` | 9 | **中** | 错误类型转换（`ToOpenAIError`/`ToClaudeError`）无测试 |
| `constant/` | 15 | **低** | 常量定义，测试价值低 |
| `logger/` | — | **低** | 日志初始化，测试价值低 |

#### 无测试的渠道适配器（35 个）

`relay/channel/` 下 36 个提供商适配器中，仅 `openai`、`claude`、`gemini`、`aws`、`minimax`、`moonshot` 有测试，其余 **30 个适配器完全无测试**：

`ai360`, `ali`, `baidu`, `baidu_v2`, `cloudflare`, `codex`, `cohere`, `coze`, `deepseek`, `dify`, `jimeng`, `jina`, `lingyiwanwu`, `mistral`, `mokaai`, `ollama`, `openrouter`, `palm`, `perplexity`, `replicate`, `siliconflow`, `submodel`, `tencent`, `vertex`, `volcengine`, `xai`, `xinference`, `xunfei`, `zhipu`, `zhipu_4v`

#### 无测试的任务适配器（10 个）

`relay/channel/task/` 下所有任务适配器完全无测试：`ali`, `doubao`, `gemini`, `hailuo`, `jimeng`, `kling`, `sora`, `suno`, `taskcommon`, `vertex`, `vidu`

#### 无测试的 service 子包

| 目录 | 说明 |
|---|---|
| `service/openaicompat/` | OpenAI 兼容层 |
| `service/passkey/` | Passkey/WebAuthn 认证 |

#### 无测试的 setting 子包（7 个）

`billing_setting/`, `console_setting/`, `model_setting/`（除 `claude_test.go`）, `perf_metrics_setting/`, `performance_setting/`, `ratio_setting/`, `reasoning/`, `system_setting/`

---

## 3. 缺失测试的高风险区域

### 3.1 P0 — 核心中继链路（零测试）

**风险**：这是系统最核心的代码路径，每次 API 请求都经过。

| 文件 | 函数 | 风险 |
|---|---|---|
| `relay/compatible_handler.go` | `TextHelper()` | 聊天补全的核心处理，包含格式转换、上游请求、流式响应、计费结算 |
| `relay/claude_handler.go` | `ClaudeHelper()` | Claude 格式中继 |
| `relay/gemini_handler.go` | `GeminiHelper()` | Gemini 格式中继 |
| `relay/image_handler.go` | `ImageHelper()` | 图片生成中继 |
| `relay/audio_handler.go` | `AudioHelper()` | 音频中继 |
| `relay/embedding_handler.go` | `EmbeddingHelper()` | 嵌入向量中继 |
| `relay/relay_task.go` | `RelayTaskSubmit()` | 异步任务提交 |
| `relay/relay_adaptor.go` | `GetAdaptor()` | 适配器工厂（35 个 switch case） |

**影响**：任何一个 handler 的 bug 都会导致所有用户的请求失败。

### 3.2 P0 — 认证中间件（零测试）

**风险**：认证是安全边界，任何绕过都是严重漏洞。

| 文件 | 函数 | 风险 |
|---|---|---|
| `middleware/auth.go` | `TokenAuth()` | API Token 认证，支持 6 种 Key 来源格式 |
| `middleware/auth.go` | `UserAuth()` | Session/Access Token 认证 |
| `middleware/auth.go` | `AdminAuth()` | 管理员认证 |
| `middleware/distributor.go` | `Distribute()` | 渠道分配，直接影响请求路由到哪个上游 |

### 3.3 P0 — 渠道选择算法（间接测试不足）

**风险**：渠道选择错误会导致请求发送到错误的上游，或无法找到可用渠道。

| 文件 | 函数 | 现有测试 |
|---|---|---|
| `service/channel_select.go` | `CacheGetRandomSatisfiedChannel()` | 无直接测试 |
| `service/channel_affinity.go` | 亲和性逻辑 | 有模板测试，但缺少端到端测试 |
| `model/channel_cache.go` | `InitChannelCache()` / `SyncChannelCache()` | 无测试 |
| `model/channel_satisfy.go` | `IsChannelSatisfy()` | 无测试 |

### 3.4 P1 — 计费系统（部分测试）

**风险**：计费错误直接导致经济损失。

| 文件 | 现有测试 | 缺失 |
|---|---|---|
| `service/text_quota.go` | `text_quota_test.go` 有 11 个测试 | 缺少边界条件（零 token、负数配额） |
| `service/billing_session.go` | 无直接测试 | `BillingSession.Settle()`/`Refund()` 的并发安全未测试 |
| `service/funding_source.go` | 无直接测试 | `WalletFunding`/`SubscriptionFunding` 的 PreConsume/Settle/Refund 未测试 |
| `service/pre_consume_quota.go` | 无直接测试 | 预扣配额的竞态条件未测试 |
| `model/subscription.go` | 无直接测试 | 订阅预扣/退款的事务正确性未测试 |

### 3.5 P1 — OAuth 登录（零测试）

**风险**：OAuth 是用户登录的重要方式，实现错误会导致登录失败或安全漏洞。

`oauth/` 目录下 8 个文件全部无测试：
- `github.go` / `discord.go` / `oidc.go` / `linuxdo.go` — 第三方 OAuth
- `generic.go` — 自定义 OAuth 通用实现
- `registry.go` — 提供商注册表
- `provider.go` — Provider 接口

### 3.6 P1 — 前端（零测试基础设施）

**风险**：前端没有测试框架，所有 UI 变更只能手动验证。

- 无 vitest/jest 配置
- 无 `test` script
- 仅 1 个孤立测试文件 `dropdown-menu.test.tsx`
- 40+ 个 features 模块、60+ 个组件全部无测试

---

## 4. Mock / Fixture / 集成测试设计

### 4.1 现有的 Mock 模式

项目使用以下 Mock 策略：

#### (1) 内存 SQLite 数据库 Mock

**文件**：`model/task_cas_test.go`

```go
func TestMain(m *testing.M) {
    db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
    DB = db
    LOG_DB = db
    common.UsingSQLite = true
    common.RedisEnabled = false
    // AutoMigrate 所有需要的表
    db.AutoMigrate(&Task{}, &User{}, &Token{}, ...)
    os.Exit(m.Run())
}
```

**评价**：这是项目中最成熟的 Mock 模式，使用真实 SQLite 内存数据库，覆盖了 GORM 层的行为。但仅 `model/` 包使用了此模式。

#### (2) httptest 请求 Mock

**文件**：`relay/helper/stream_scanner_test.go`

```go
func setupStreamTest(t *testing.T, body io.Reader) (*gin.Context, *http.Response, *relaycommon.RelayInfo) {
    recorder := httptest.NewRecorder()
    c, _ := gin.CreateTestContext(recorder)
    c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
    resp := &http.Response{Body: io.NopCloser(body)}
    info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}
    return c, resp, info
}
```

**评价**：用于测试流式扫描器，创建了最小化的 Gin Context 和 HTTP Response。但没有模拟上游 API 的响应。

#### (3) 表达式驱动测试

**文件**：`service/tiered_settle_test.go`, `pkg/billingexpr/billingexpr_test.go`

使用 Go 表格驱动测试模式，覆盖了大量边界条件：

```go
func TestTryTieredSettleUsesFrozenRequestInput(t *testing.T) {
    exprStr := `param("service_tier") == "fast" ? tier("fast", p * 2) : tier("normal", p)`
    relayInfo := &relaycommon.RelayInfo{...}
    // ...
}
```

**评价**：计费表达式和分层结算的测试质量较高，是项目中测试最好的模块。

### 4.2 缺失的 Mock / Fixture

| 缺失项 | 影响 | 建议 |
|---|---|---|
| **上游 API Mock Server** | 无法测试完整的中继链路 | 使用 `httptest.NewServer` 模拟 OpenAI/Claude/Gemini 上游 |
| **Redis Mock** | 涉及 Redis 的逻辑无法测试 | 使用 `miniredis` 或内嵌 Redis |
| **Gin 中间件 Mock** | 中间件无法独立测试 | 使用 `httptest` + `gin.CreateTestContext` |
| **支付 Webhook Mock** | 支付回调逻辑无法自动化测试 | 使用各支付商的测试 webhook payload |
| **Fixture 数据** | 每个测试自行构造数据，无共享 fixture | 创建 `testdata/` 目录和 fixture 加载器 |

### 4.3 集成测试

**现状**：项目**没有集成测试**。

- 没有 `docker-compose.test.yml` 用于启动测试环境
- 没有端到端 API 测试
- 没有数据库迁移测试
- `makefile` 中没有 `test` 目标

---

## 5. Lint / Typecheck / CI 配置

### 5.1 后端 Lint

| 工具 | 配置 | 状态 |
|---|---|---|
| `go vet` | 标准工具 | **未在 CI 中运行** |
| `golangci-lint` | 无 `.golangci.yml` | **未配置** |
| `staticcheck` | 无配置 | **未配置** |

**评价**：后端没有配置任何 lint 工具，代码质量完全依赖人工 review。

### 5.2 前端 Lint / Typecheck

| 工具 | 配置文件 | 状态 |
|---|---|---|
| **ESLint** | `web/default/eslint.config.mjs` | 已配置，规则合理 |
| **TypeScript** | `web/default/tsconfig.json` | 已配置 |
| **Prettier** | `web/default/package.json` 中有 `format` 脚本 | 已配置 |

**ESLint 关键规则**：
- `no-console: error` — 禁止 console.log
- `@typescript-eslint/no-unused-vars: error` — 禁止未使用变量
- `@typescript-eslint/consistent-type-imports: error` — 强制 type import
- `no-duplicate-imports: error` — 禁止重复导入

**评价**：前端 lint 配置质量较好，但**未在 CI 中强制执行**。

### 5.3 CI 配置

**文件**：`.github/workflows/`

| Workflow | 触发条件 | 做了什么 | **没做什么** |
|---|---|---|---|
| `pr-check.yml` | PR opened/reopened | anti-slop 反 AI 水文检测、PR 模板检查 | **不运行测试、不运行 lint** |
| `release.yml` | tag push | 构建前端 + 编译 Go 二进制 + 发布 | **不运行测试** |
| `docker-build.yml` | tag push / workflow_dispatch | Docker 多架构构建 | **不运行测试** |
| `docker-image-alpha.yml` | tag push | Alpha Docker 镜像 | **不运行测试** |
| `docker-image-nightly.yml` | schedule (daily) | Nightly Docker 镜像 | **不运行测试** |
| `electron-build.yml` | — | Electron 构建 | **不运行测试** |
| `sync-to-gitee.yml` | push | 同步到 Gitee | — |

**关键发现**：**所有 CI workflow 都不运行测试或 lint**。代码在合并前没有任何自动化质量检查。

### 5.4 Makefile

**文件**：`makefile`

**缺失目标**：
- `make test` — 不存在
- `make lint` — 不存在
- `make vet` — 不存在
- `make fmt` — 不存在

---

## 6. 建议优先补充的测试清单

### 6.1 第一优先级（P0 — 安全与核心链路）

| # | 测试项 | 文件 | 测试类型 | 估计工作量 |
|---|---|---|---|---|
| 1 | `TokenAuth()` 认证中间件 | `middleware/auth.go` | 单元测试 | 中 |
| 2 | `Distribute()` 渠道分配 | `middleware/distributor.go` | 单元测试 | 高 |
| 3 | `TextHelper()` 聊天中继 | `relay/compatible_handler.go` | 集成测试（需 Mock 上游） | 高 |
| 4 | `GetAdaptor()` 适配器工厂 | `relay/relay_adaptor.go` | 单元测试 | 低 |
| 5 | `PreConsumeQuota()` 预扣配额 | `service/pre_consume_quota.go` | 单元测试（含并发） | 中 |
| 6 | `BillingSession.Settle()/Refund()` | `service/billing_session.go` | 单元测试（含并发） | 中 |
| 7 | `WalletFunding` / `SubscriptionFunding` | `service/funding_source.go` | 单元测试 | 中 |

### 6.2 第二优先级（P1 — 业务逻辑）

| # | 测试项 | 文件 | 测试类型 | 估计工作量 |
|---|---|---|---|---|
| 8 | `CacheGetRandomSatisfiedChannel()` | `service/channel_select.go` | 单元测试 | 中 |
| 9 | `IsChannelSatisfy()` | `model/channel_satisfy.go` | 单元测试 | 低 |
| 10 | `InitChannelCache()` / `SyncChannelCache()` | `model/channel_cache.go` | 单元测试 | 中 |
| 11 | `PostTextConsumeQuota()` 边界条件 | `service/text_quota.go` | 补充测试 | 中 |
| 12 | `RelayTaskSubmit()` 任务提交 | `relay/relay_task.go` | 集成测试 | 高 |
| 13 | `TaskPollingLoop()` 任务轮询 | `service/task_polling.go` | 单元测试 | 中 |
| 14 | OAuth `HandleOAuth()` | `controller/oauth.go` | 单元测试 | 中 |
| 15 | `NewAPIError` 格式转换 | `types/error.go` | 单元测试 | 低 |

### 6.3 第三优先级（P2 — 基础设施）

| # | 测试项 | 文件 | 测试类型 | 估计工作量 |
|---|---|---|---|---|
| 16 | CI 添加 `go test ./...` | `.github/workflows/` | CI 配置 | 低 |
| 17 | CI 添加 `go vet ./...` | `.github/workflows/` | CI 配置 | 低 |
| 18 | CI 添加 `bun run lint` | `.github/workflows/` | CI 配置 | 低 |
| 19 | CI 添加 `bun run typecheck` | `.github/workflows/` | CI 配置 | 低 |
| 20 | Makefile 添加 `test` / `lint` 目标 | `makefile` | 配置 | 低 |
| 21 | 前端测试框架搭建（vitest） | `web/default/` | 基础设施 | 中 |
| 22 | 上游 API Mock Server | `testdata/mock_apis/` | 基础设施 | 高 |

### 6.4 长期建议

| 建议 | 说明 |
|---|---|
| **引入 `golangci-lint`** | 配置 `.golangci.yml`，启用 `errcheck`、`gosec`、`ineffassign` 等 |
| **集成测试环境** | 创建 `docker-compose.test.yml`，包含 PostgreSQL + Redis + 应用 |
| **前端组件测试** | 使用 vitest + @testing-library/react 对核心 features 编写测试 |
| **E2E 测试** | 使用 Playwright 或 Cypress 对核心流程（登录→创建 Token→发送请求）编写端到端测试 |
| **覆盖率门槛** | CI 中添加覆盖率检查，设置最低阈值（建议初始 30%，逐步提升） |
| **测试数据工厂** | 创建 `model/testutil/` 包，提供 User/Channel/Token 的工厂函数 |
| **契约测试** | 对 36 个渠道适配器，使用录制/回放（record/replay）模式测试与上游 API 的契约 |
