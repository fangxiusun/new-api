# New API — 可修改性评估

> 生成时间：2026-06-16
> 目的：在不修改代码的前提下，评估各区域的修改风险和安全边界

---

## 1. 最容易安全修改的区域

### 1.1 新增渠道适配器（低风险，孤立性强）

**原因**：36 个现有适配器都是独立目录，遵循统一的 `channel.Adaptor` 接口。新增一个适配器不会影响任何现有代码。

**操作路径**：
1. 在 `relay/channel/<new_provider>/` 创建 `adaptor.go` 实现 `channel.Adaptor` 接口
2. 在 `constant/api_type.go` 添加 `APIType<NewProvider>` 常量
3. 在 `relay/relay_adaptor.go` 的 `GetAdaptor()` switch 中添加一行 case
4. 在 `constant/channel.go` 添加渠道类型常量

**风险点**：仅 `relay/relay_adaptor.go:GetAdaptor()` 需要修改现有代码（添加一个 case），影响范围极小。

**验证方式**：编写独立的适配器单元测试即可，不需要修改其他模块。

### 1.2 前端 UI 组件和页面（低风险，前后端隔离）

**原因**：前端是纯展示层，修改不会影响后端逻辑。`web/default/src/features/` 下的每个功能模块相对独立。

**安全区域**：
- `web/default/src/components/ui/` — 基础 UI 组件
- `web/default/src/features/*/` — 各功能页面
- `web/default/src/styles/` — 样式调整
- `web/default/src/i18n/locales/` — 翻译文件

**风险点**：修改 `web/default/src/lib/api.ts`（API 调用封装）时需确保与后端接口一致。

### 1.3 系统配置项（低风险，KV 存储）

**原因**：`model/option.go` 中的配置项是 KV 存储，新增配置项只需：
1. 在 `model/InitOptionMap()` 中添加默认值
2. 在对应的 setting 子模块中读取

**安全区域**：`setting/operation_setting/`、`setting/performance_setting/`、`setting/system_setting/` 下的配置读取逻辑。

### 1.4 计费表达式（中低风险，有完善测试）

**原因**：`pkg/billingexpr/` 是独立的内部包，有 56 个测试函数覆盖，修改时可以通过测试验证。表达式语言的设计文档在 `pkg/billingexpr/expr.md`。

**验证方式**：`go test ./pkg/billingexpr/...`

### 1.5 DTO 结构体（低风险，纯数据定义）

**原因**：`dto/` 包只包含请求/响应结构体定义，不包含业务逻辑。新增字段通常只影响序列化/反序列化。

**注意**：遵循 AGENTS.md Rule 6 — 可选标量字段必须用指针类型 + `omitempty`。

### 1.6 前端国际化翻译（极低风险）

**原因**：`web/default/src/i18n/locales/*.json` 是纯翻译文件，添加/修改翻译不会影响功能逻辑。

**工具**：`bun run i18n:sync`（从 `web/default/` 目录执行）

---

## 2. 修改风险最高的区域

### 2.1 `middleware/auth.go` — 认证中间件（风险：极高）

**原因**：
- `TokenAuth()` 被所有 `/v1/*` 和 `/dashboard/*` 路由依赖
- 支持 6 种 Key 来源格式（Bearer / x-api-key / x-goog-api-key / query key / WebSocket / mj-api-secret）
- 修改认证逻辑可能导致所有 API 请求失败或安全漏洞
- **无测试覆盖**

**如果必须修改**：
1. 先编写 `TokenAuth()` 的单元测试，覆盖所有 Key 来源格式
2. 使用 `httptest.NewRecorder` + `gin.CreateTestContext` 构造测试请求
3. 验证正向（有效 Token）和反向（无效 Token / 过期 / IP 限制）场景

### 2.2 `middleware/distributor.go` — 渠道分配（风险：极高）

**原因**：
- 被所有中继路由依赖，直接影响请求路由到哪个上游
- 依赖 5 个 service 函数 + 4 个 model 函数 + 15 个 constant
- 涉及渠道亲和性、auto 分组、跨分组重试等复杂逻辑
- **无测试覆盖**

**如果必须修改**：
1. 先理解 `service/channel_select.go:CacheGetRandomSatisfiedChannel()` 的完整逻辑
2. 准备内存 SQLite + 预置渠道数据的测试环境
3. 验证各种分组/优先级/重试场景

### 2.3 `relay/compatible_handler.go:TextHelper()` — 文本中继核心（风险：极高）

**原因**：
- 这是 `/v1/chat/completions` 的核心处理函数
- 串联了 8 个步骤：初始化 → 适配器创建 → 格式转换 → 上游请求 → 响应处理 → 计费结算
- 依赖 service（5 个函数）、relay/channel（适配器）、dto（请求/响应）、common（工具）
- **无测试覆盖**
- 修改任何一步都可能影响所有用户的聊天请求

### 2.4 `model/channel_cache.go` — 渠道缓存（风险：高）

**原因**：
- `group2model2channels` 和 `channelsIDM` 是全局内存缓存，被渠道选择算法直接使用
- 使用 `sync.RWMutex` 保护，并发修改可能导致死锁或数据竞争
- 缓存刷新逻辑涉及从旧缓存迁移多 key 轮询索引

### 2.5 `service/billing_session.go` + `service/funding_source.go` — 计费会话（风险：高）

**原因**：
- `BillingSession.Settle()` 和 `Refund()` 直接操作用户钱包和订阅余额
- 使用 `sync.Mutex` 保护，但 `Refund()` 的幂等性依赖外部函数的正确性
- `WalletFunding.Refund()` 的注释明确说明 "IncreaseUserQuota 是非幂等操作，不能重试"
- 修改错误可能导致用户资金损失

### 2.6 `model/main.go` — 数据库初始化和迁移（风险：高）

**原因**：
- `AutoMigrate()` 在启动时自动执行，修改模型结构直接影响数据库 schema
- 跨库兼容（SQLite/MySQL/PostgreSQL）的约束增加了复杂度
- SQLite 不支持 `ALTER COLUMN`，某些迁移需要特殊处理

### 2.7 Gin Context Key 传递链（风险：高，隐性耦合）

**原因**：
- 中间件之间通过 `c.Set(key, value)` 传递数据，形成隐性依赖链
- `constant/context_key.go` 定义了 30+ 个 Context Key
- 修改任何一个 Key 的写入位置或读取时机都可能破坏下游逻辑
- 这种耦合没有类型检查保护，只有运行时才会发现错误

**示例**：`Distribute()` 写入 `ContextKeyChannelId`，`controller/relay.go` 读取它，`relay/common/relay_info.go:genBaseRelayInfo()` 再读取它。任何一环断裂都会导致空指针或错误的渠道选择。

---

## 3. 模块耦合点

### 3.1 包依赖扇入（被多少文件 import）

| 包 | 被引用文件数 | 耦合程度 | 说明 |
|---|---|---|---|
| `common` | **242** | 极高 | 几乎所有文件都依赖，修改影响面最大 |
| `relay/` (全部子包) | **354** | 极高 | 整个 relay 层（含 channel 适配器） |
| `setting/` (全部子包) | **164** | 高 | 配置读取遍布各处 |
| `dto` | **150** | 高 | 请求/响应结构体被广泛使用 |
| `types` | **132** | 高 | 错误类型和格式枚举被广泛使用 |
| `constant` | **102** | 高 | 常量定义被广泛使用 |
| `model` | **99** | 高 | 数据访问层被 service/controller/relay 依赖 |
| `service` | **86** | 中高 | 业务逻辑层被 controller/relay 依赖 |

### 3.2 关键耦合热点

#### (1) `common` 包 — 全局状态中心

**318 处**引用了 `common` 包的全局变量（`OptionMap`、`UsingPostgreSQL`、`RedisEnabled`、`IsMasterNode` 等）。

**风险**：修改 `common/init.go` 中的任何环境变量解析逻辑都可能影响 318 处代码的行为。

**建议**：不要修改 `common` 包的现有接口，只做追加。

#### (2) Gin Context — 隐性数据管道

中间件 → Controller → Relay 之间通过 Gin Context 传递 30+ 个隐式参数：

```
TokenAuth() 写入 → ContextKeyUserId, ContextKeyTokenId, ContextKeyTokenGroup...
Distribute() 写入 → ContextKeyChannelId, ContextKeyChannelType, ContextKeyChannelKey...
controller.Relay() 读取 → 构建 RelayInfo
relay.TextHelper() 读取 → info.InitChannelMeta(c)
```

**风险**：这些传递没有类型安全，修改写入/读取顺序会导致运行时错误。

**建议**：新增数据传递优先通过 `RelayInfo` 结构体，而非新增 Context Key。

#### (3) `relay/relay_adaptor.go` — 适配器工厂

`GetAdaptor()` 包含 35 个 switch case，`GetTaskAdaptor()` 包含 12 个 case。每次新增提供商都需要修改此处。

**风险**：低（只是添加 case），但这是唯一的修改点。

#### (4) `service` ↔ `relay` 双向依赖

- `relay/compatible_handler.go` 调用 `service.PostTextConsumeQuota()`
- `service/task_polling.go` 通过 `GetTaskAdaptorFunc` 回调 `relay.GetTaskAdaptor()`
- 依赖通过 `main.go` 的函数注入打破循环

**风险**：如果需要在 relay 层新增对 service 的调用，必须确认不会重新引入循环依赖。

#### (5) `constant` 包 — 跨模块共享枚举

`constant/api_type.go` 和 `constant/channel.go` 定义了所有提供商的类型枚举。修改这些枚举会影响：
- `relay/relay_adaptor.go` 的 switch case
- `middleware/distributor.go` 的渠道类型判断
- `model/channel.go` 的渠道类型字段

**建议**：只追加新常量，不要修改或删除现有常量的值。

---

## 4. 配置和环境变量风险

### 4.1 环境变量全景（24 个）

| 类别 | 变量 | 默认值 | 修改风险 |
|---|---|---|---|
| **数据库** | `SQL_DSN` | 空（SQLite） | 高 — 切换数据库类型影响所有 SQL 兼容性 |
| | `SQLITE_PATH` | `one-api.db` | 中 — 路径变更需确保文件可访问 |
| | `LOG_SQL_DSN` | 空（同主库） | 中 — 日志数据库独立配置 |
| **缓存** | `REDIS_CONN_STRING` | 空（禁用） | 中 — 启用 Redis 改变缓存行为 |
| | `SYNC_FREQUENCY` | 60 | 低 |
| | `MEMORY_CACHE_ENABLED` | false | 中 — 影响渠道缓存 |
| **安全** | `SESSION_SECRET` | 随机生成 | **极高** — 修改会导致所有现有 Session 失效 |
| | `CRYPTO_SECRET` | 同 SESSION_SECRET | **极高** — 修改会导致加密数据无法解密 |
| **运行模式** | `DEBUG` | false | 低 |
| | `GIN_MODE` | release | 低 |
| | `NODE_TYPE` | 空（master） | 高 — 影响后台任务启动 |
| **性能** | `RELAY_TIMEOUT` | 0 | 中 — 影响所有上游请求 |
| | `STREAMING_TIMEOUT` | 120 | 中 — 影响流式请求 |
| | `BATCH_UPDATE_ENABLED` | false | 中 — 影响配额更新频率 |
| **前端** | `FRONTEND_BASE_URL` | 空 | 中 — 影响前端路由 |
| | `THEME` | default | 低 |

### 4.2 配置热加载

以下配置支持运行时修改（通过 `/api/option/` 管理接口）：
- 模型定价比例
- 分组比例
- 系统公告/隐私政策等文本
- 各种功能开关

以下配置**不支持热加载**，修改后需要重启：
- 数据库连接字符串
- Redis 连接字符串
- Session Secret
- 端口号
- 节点类型

### 4.3 数据库兼容性陷阱

修改涉及 raw SQL 的代码时，必须注意：
- PostgreSQL 用 `"group"` 引用保留字列，MySQL/SQLite 用 `` `group` ``
- PostgreSQL 布尔值用 `true`/`false`，MySQL/SQLite 用 `1`/`0`
- SQLite 不支持 `ALTER COLUMN`
- MySQL 需要 `parseTime=true` 参数

**安全做法**：始终使用 GORM 链式 API，避免 raw SQL。如果必须用 raw SQL，使用 `commonGroupCol`/`commonKeyCol`/`commonTrueVal`/`commonFalseVal` 变量。

---

## 5. 推荐的本地验证命令

### 5.1 后端验证

```bash
# 编译检查（最快，发现语法/类型错误）
go build ./...

# 运行全部测试
go test ./...

# 运行特定模块测试
go test ./model/...           # 数据模型层
go test ./pkg/billingexpr/... # 计费表达式
go test ./relay/helper/...    # 流式扫描器
go test ./service/...         # 业务逻辑层
go test ./dto/...             # DTO 序列化

# 静态分析
go vet ./...

# 检查未使用的依赖
go mod tidy

# 启动后端（验证运行时行为）
go run main.go

# 启动完整开发环境（含 PostgreSQL + Redis）
make dev-api
make dev-web
```

### 5.2 前端验证

```bash
# 进入前端目录
cd web/default

# TypeScript 类型检查
bun run typecheck

# ESLint 检查
bun run lint

# 格式检查
bun run format:check

# 构建验证（类型检查 + 打包）
bun run build:check

# 启动开发服务器
bun run dev

# 同步国际化翻译
bun run i18n:sync
```

### 5.3 完整验证流程（推荐在提交前执行）

```bash
# 后端
go build ./... && go vet ./... && go test ./...

# 前端
cd web/default && bun run typecheck && bun run lint && bun run build

# 本地运行验证
go run main.go
# 访问 http://localhost:3000 验证前端
# 使用 curl 测试 API
curl http://localhost:3000/api/status
```

---

## 6. 第一个适合练手的小改动建议

### 推荐：添加一个新的系统配置项

**改动内容**：新增一个布尔类型的系统配置项，控制某个 UI 功能的开关。

**为什么推荐**：
- 不涉及核心链路（中继/计费/认证）
- 不修改现有代码的行为，只是追加
- 涉及的文件链路完整但简短（后端配置 → API → 前端读取）
- 可以完整走一遍开发→验证→前端展示的流程

**改动步骤**：

```
1. 后端：model/option.go → InitOptionMap() 中添加默认值
2. 后端：common/ 或对应的 setting 子包中添加变量
3. 后端：controller/option.go 确保新配置项可读写（通常已自动支持）
4. 前端：在对应的系统设置页面读取和展示该配置
5. 验证：go test ./model/... && go build ./... && bun run typecheck
```

**预期修改文件**：2-3 个文件
**预期风险**：极低（追加操作，不修改现有逻辑）
**验证方式**：`go build ./...` + `bun run typecheck` + 手动在管理页面验证

---

### 备选练手建议

| # | 改动 | 文件数 | 风险 | 学习价值 |
|---|---|---|---|---|
| A | 添加前端 i18n 翻译 | 1 | 极低 | 学习 i18n 工作流 |
| B | 新增一个 API 查询参数 | 2-3 | 低 | 学习 controller → model 链路 |
| C | 修改前端某个页面的布局 | 1-2 | 低 | 学习前端组件结构 |
| D | 为 `types/error.go` 补充单元测试 | 1 | 极低 | 学习测试编写模式 |
| E | 为 `GetAdaptor()` 补充单元测试 | 1 | 极低 | 学习适配器工厂模式 |
| F | 添加一个新的 Vendor 元数据 | 1 | 极低 | 学习管理 API 的 CRUD 模式 |
