# New API — 架构说明文档

> 生成时间：2026-06-16
> 适用对象：需要深入理解系统架构的开发人员
> 文档性质：基于代码静态分析生成，部分设计意图为推测（已标注）

---

## 1. 系统分层

New API 采用经典的**分层架构**，请求自顶向下流经四层：

```
┌──────────────────────────────────────────────────────┐
│                    HTTP Client                        │
│            (用户 / 前端 / 第三方应用)                   │
└──────────────┬───────────────────────────┬────────────┘
               │                           │
       ┌───────▼────────┐         ┌────────▼───────┐
       │   /api/* 路由   │         │  /v1/* 路由     │
       │  (管理/业务 API) │         │  (AI 中继 API)  │
       └───────┬────────┘         └────────┬───────┘
               │                           │
       ┌───────▼────────────────────────────▼───────┐
       │              Middleware 层                   │
       │  auth → distributor → rate-limit → i18n    │
       └───────┬────────────────────────────┬───────┘
               │                           │
       ┌───────▼────────┐         ┌────────▼───────┐
       │  Controller 层  │         │  Controller 层  │
       │  (业务入口)      │         │  Relay() 入口   │
       └───────┬────────┘         └────────┬───────┘
               │                           │
       ┌───────▼────────┐         ┌────────▼───────┐
       │   Service 层    │         │   Relay 层      │
       │  (业务逻辑)      │         │ (AI 中继核心)   │
       └───────┬────────┘         └────────┬───────┘
               │                           │
       ┌───────▼────────────────────────────▼───────┐
       │                Model 层                     │
       │      (GORM 数据模型 + 内存缓存)              │
       └───────┬────────────────────────────────────┘
               │
       ┌───────▼──────────────────────────────────┐
       │          外部存储                           │
       │   SQLite / MySQL / PostgreSQL + Redis     │
       └──────────────────────────────────────────┘
```

### 各层职责边界

| 层 | 目录 | 职责 | 不应包含的逻辑 |
|---|---|---|---|
| **Router** | `router/` | URL 映射、路由分发、中间件编排 | 业务逻辑、数据访问 |
| **Middleware** | `middleware/` | 认证、授权、限流、日志、i18n、渠道分配 | 具体业务处理 |
| **Controller** | `controller/` | 参数校验、请求分发、响应格式化 | 复杂业务计算、数据持久化 |
| **Service** | `service/` | 业务逻辑、计费计算、渠道选择、HTTP 调用 | 直接数据库操作（应通过 model） |
| **Relay** | `relay/` | AI 请求格式转换、上游调用、流式响应处理 | 用户管理、计费结算（通过回调 service） |
| **Model** | `model/` | 数据结构定义、GORM 操作、缓存同步 | HTTP 处理、业务决策 |
| **Common** | `common/` | 基础工具（JSON、Redis、加密、日志等） | 任何业务逻辑 |
| **DTO** | `dto/` | 请求/响应结构体定义 | 行为逻辑 |
| **Constant** | `constant/` | 枚举和常量 | 动态配置（属于 setting） |
| **Setting** | `setting/` | 运行时可变配置管理 | 固定常量（属于 constant） |
| **Pkg** | `pkg/` | 可独立测试的内部包（计费表达式、缓存等） | 依赖业务上下文的逻辑 |

---

## 2. 主要模块之间的依赖关系

### 2.1 依赖拓扑图

```
router ──────► controller ──────► service ──────► model
   │               │                  │             ▲
   │               │                  │             │
   │               ▼                  ▼             │
   │            relay ──────► relay/channel ────────┘
   │               │
   ▼               ▼
middleware     relay/common ◄── relay/helper
   │               │
   ▼               ▼
common ◄──── constant ◄──── types ◄──── dto
   ▲
   │
setting ──► pkg/billingexpr
```

### 2.2 关键依赖规则

1. **Router → Controller / Relay / Middleware**：路由层直接引用控制器和中间件
2. **Controller → Service + Relay**：控制器调用业务逻辑层，同时直接调用 relay 中继层（`controller/relay.go` 中的 `relayHandler()`）
3. **Relay → Service**：relay 层回调 service 层进行计费（`service.PostTextConsumeQuota`、`service.SettleBilling`）和配额操作
4. **Service → Model**：service 层通过 model 层访问数据库
5. **Relay/Channel → Model**：适配器需要查询渠道信息（`model.GetChannelById`）
6. **Main → Service**：`main.go` 中注入 `service.GetTaskAdaptorFunc` 以**打破循环依赖**（`service` → `relay` → `relay/channel` → `service`）

### 2.3 循环依赖的处理

项目中存在一个已知的**循环依赖问题**及解决方案：

- **问题**：`service/task_polling.go` 需要调用 `relay/channel/task/` 中的任务适配器，但 `relay` 层已经依赖 `service` 层
- **解决方案**：在 `main.go` 中通过**函数注入**打破循环：

```go
// main.go
service.GetTaskAdaptorFunc = func(platform constant.TaskPlatform) service.TaskPollingAdaptor {
    a := relay.GetTaskAdaptor(platform)
    if a == nil {
        return nil
    }
    return a
}
```

对应定义在 `service/task_polling.go:30`：

```go
var GetTaskAdaptorFunc func(platform constant.TaskPlatform) TaskPollingAdaptor
```

---

## 3. 核心数据流

### 3.1 AI 中继请求流（同步/流式）

```
1. 用户发送 OpenAI 兼容请求
   POST /v1/chat/completions

2. middleware.TokenAuth()
   → 验证 API Key（Bearer Token）
   → 解析 Token 信息，写入 Context（userId, tokenId, tokenGroup, modelLimits 等）

3. middleware.Distribute()
   → 解析请求体中的 model 字段
   → 检查 Token 模型白名单
   → service.CacheGetRandomSatisfiedChannel() 选择渠道
     → 按 group → model → priority 查找可用渠道
     → 支持渠道亲和性（channel_affinity）会话粘性
     → 支持 auto 组跨分组重试
   → 将渠道信息写入 Context（channelId, channelType, baseUrl 等）

4. controller.Relay()
   → 根据 relayFormat 分发到对应 handler
   → 构建 RelayInfo 上下文对象

5. relay.TextHelper() / GeminiHelper() / ClaudeHelper() 等
   → adaptor = GetAdaptor(info.ApiType) 获取对应提供商适配器
   → adaptor.Init(info) 初始化
   → adaptor.ConvertOpenAIRequest() 格式转换
   → service.PreConsumeQuota() 预扣配额
   → adaptor.DoRequest() 发送请求到上游
   → adaptor.DoResponse() 处理响应（流式/非流式）
   → service.PostTextConsumeQuota() 结算实际消耗
   → 记录日志到 model.Log

6. 返回响应给用户
```

### 3.2 异步任务流（视频/音乐生成等）

```
1. 用户提交任务
   POST /v1/videos/submit

2. controller.RelayTask()
   → relay.RelayTaskAdaptor() 分发到 TaskAdaptor

3. relay 层处理
   → TaskAdaptor.ValidateRequestAndSetAction() 验证请求
   → TaskAdaptor.EstimateBilling() 估算计费
   → service.PreConsumeBilling() 预扣全额（ForcePreConsume=true）
   → TaskAdaptor.DoRequest() 提交到上游
   → TaskAdaptor.DoResponse() 解析提交结果
   → model.CreateTask() 保存任务记录

4. 后台轮询（service/task_polling.go）
   → TaskPollingLoop() 每 15 秒执行一次
   → sweepTimedOutTasks() 清理超时任务
   → 按平台分组查询未完成任务
   → TaskAdaptor.FetchTask() 查询上游状态
   → TaskAdaptor.ParseTaskResult() 解析结果
   → task.UpdateWithStatus() CAS 更新状态
   → TaskAdaptor.AdjustBillingOnComplete() 差额结算

5. 用户查询任务状态
   GET /v1/videos/fetch/:id
```

### 3.3 计费数据流

```
请求进入
  │
  ▼
service.PreConsumeBilling()
  │→ NewBillingSession() 创建计费会话
  │→ 选择 FundingSource（Wallet 或 Subscription）
  │  ├─ WalletFunding: model.DecreaseUserQuota()
  │  └─ SubscriptionFunding: model.PreConsumeUserSubscription()
  │→ BillingSession 存储在 RelayInfo.Billing
  │
请求完成
  │
  ▼
service.SettleBilling()
  │→ 计算 delta = actualQuota - preConsumedQuota
  │→ BillingSession.Settle(delta)
  │  ├─ Funding.Settle(delta) 调整资金来源
  │  └─ model.DecreaseTokenQuota() / IncreaseTokenQuota() 调整令牌额度
  │
请求失败
  │
  ▼
BillingSession.Refund()
  ├─ Funding.Refund() 退还资金来源
  └─ model.IncreaseTokenQuota() 退还令牌额度
```

### 3.4 渠道选择数据流

```
middleware.Distribute()
  │
  ├─ Token 指定渠道？ → model.GetChannelById()
  │
  └─ 动态选择：
     │
     ├─ 1. 检查渠道亲和性（channel_affinity）
     │     service.GetPreferredChannelByAffinity()
     │     → 基于请求特征（IP/用户/模型等）的缓存映射
     │
     ├─ 2. 自动分组路由（auto group）
     │     → service.GetUserAutoGroup() 获取用户可用分组
     │     → model.IsChannelEnabledForGroupModel() 检查可用性
     │
     └─ 3. 按优先级随机选择
           service.CacheGetRandomSatisfiedChannel()
           → group2model2channels 缓存查找
           → 按 priority 排序
           → 支持跨分组重试（cross_group_retry）
           → 检查 model.IsChannelSatisfy()
```

---

## 4. 请求 / 任务 / 命令的处理流程

### 4.1 管理 API 请求流程

```
POST /api/user/register
  │
  ├─ middleware.CriticalRateLimit()      严格限流
  ├─ middleware.AnonymousRequestBodyLimit()  请求体大小限制
  ├─ middleware.TurnstileCheck()          人机验证
  │
  └─ controller.Register()
     ├─ 参数校验（validator/v10）
     ├─ service 层业务逻辑
     ├─ model 层数据操作
     └─ common.ApiSuccess(c, data)  统一响应格式
```

### 4.2 认证流程

系统支持多种认证方式，均在 `middleware/auth.go` 中实现：

| 认证方式 | 中间件 | 适用场景 |
|---|---|---|
| Session (Cookie) | `UserAuth()` / `AdminAuth()` | 前端管理界面 |
| Access Token (Header) | `UserAuth()` | 前端 API 调用 |
| Bearer Token | `TokenAuth()` | AI API 调用（`/v1/*`） |
| Token Read-Only | `TokenAuthReadOnly()` | 日志查询 |

认证信息存储在 Gin Context 中，通过 `constant/context_key.go` 定义的 key 访问：

```go
constant.ContextKeyUserId           // 用户 ID
constant.ContextKeyTokenId          // 令牌 ID
constant.ContextKeyTokenGroup       // 令牌分组
constant.ContextKeyUsingGroup       // 当前使用的分组
constant.ContextKeyOriginalModel    // 原始模型名
constant.ContextKeyChannelType      // 渠道类型
```

### 4.3 后台任务系统

系统有三类后台任务，在 `main.go` 中启动：

| 任务 | 启动方式 | 说明 |
|---|---|---|
| `model.SyncChannelCache()` | `go` goroutine | 定时同步渠道缓存 |
| `model.SyncOptions()` | `go` goroutine | 定时同步配置项 |
| `controller.UpdateMidjourneyTaskBulk()` | `gopool.Go()` | Midjourney 任务批量轮询 |
| `service.TaskPollingLoop()` | `gopool.Go()`（通过 `controller.UpdateTaskBulk()`） | 通用异步任务轮询（每 15 秒） |
| `service.StartCodexCredentialAutoRefreshTask()` | 启动时 | Codex 凭证自动刷新 |
| `service.StartSubscriptionQuotaResetTask()` | 启动时 | 订阅配额定期重置 |
| `controller.StartChannelUpstreamModelUpdateTask()` | 启动时 | 上游模型列表更新 |
| `controller.AutomaticallyTestChannels()` | 启动时 | 渠道自动测试 |

---

## 5. 关键抽象、接口、类和函数

### 5.1 核心接口

#### `channel.Adaptor` — AI 提供商适配器接口

**文件**：`relay/channel/adapter.go`

```go
type Adaptor interface {
    Init(info *relaycommon.RelayInfo)
    GetRequestURL(info *relaycommon.RelayInfo) (string, error)
    SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error
    ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error)
    ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ClaudeRequest) (any, error)
    ConvertGeminiRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeminiChatRequest) (any, error)
    ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error)
    ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error)
    ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error)
    ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error)
    ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error)
    DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error)
    DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError)
    GetModelList() []string
    GetChannelName() string
}
```

这是整个中继层的**核心抽象**。36 个上游提供商各自实现此接口，系统通过 `relay.GetAdaptor(apiType)` 工厂函数选择对应实现。

#### `channel.TaskAdaptor` — 异步任务适配器接口

**文件**：`relay/channel/adapter.go`

```go
type TaskAdaptor interface {
    Init(info *relaycommon.RelayInfo)
    ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError
    EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64
    AdjustBillingOnSubmit(info *relaycommon.RelayInfo, taskData []byte) map[string]float64
    AdjustBillingOnComplete(task *model.Task, taskResult *relaycommon.TaskInfo) int
    DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error)
    DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*TaskSubmitResult, *types.NewAPIError)
    FetchTask(baseURL string, key string, body map[string]any, proxy string) (*http.Response, error)
    ParseTaskResult(body []byte) (*relaycommon.TaskInfo, error)
    GetModelList() []string
    GetChannelName() string
}
```

#### `relaycommon.BillingSettler` — 计费会话接口

**文件**：`relay/common/billing.go`

```go
type BillingSettler interface {
    Settle(actualQuota int) error
    Refund(c *gin.Context)
}
```

由 `service.BillingSession` 实现，存储在 `RelayInfo.Billing` 上。通过接口解耦 relay 层和 service 层的计费逻辑。

#### `service.FundingSource` — 资金来源接口

**文件**：`service/funding_source.go`

```go
type FundingSource interface {
    Source() string           // "wallet" 或 "subscription"
    PreConsume(amount int) error
    Settle(delta int) error
    Refund() error
}
```

两个实现：
- `WalletFunding` — 钱包扣费
- `SubscriptionFunding` — 订阅扣费

#### `service.TaskPollingAdaptor` — 任务轮询适配器接口

**文件**：`service/task_polling.go`

```go
type TaskPollingAdaptor interface {
    Init(info *relaycommon.RelayInfo)
    FetchTask(baseURL string, key string, body map[string]any, proxy string) (*http.Response, error)
    ParseTaskResult(body []byte) (*relaycommon.TaskInfo, error)
    AdjustBillingOnComplete(task *model.Task, taskResult *relaycommon.TaskInfo) int
}
```

这是 `channel.TaskAdaptor` 的**最小子集**，用于 service 层轮询任务，避免 service → relay 的循环依赖。

### 5.2 核心数据结构

#### `relaycommon.RelayInfo` — 中继上下文

**文件**：`relay/common/relay_info.go`

这是贯穿整个请求生命周期的**核心上下文对象**，包含：

- **身份信息**：`TokenId`, `TokenKey`, `UserId`, `UserGroup`
- **请求配置**：`IsStream`, `RelayMode`, `OriginModelName`
- **渠道信息**：通过 `ChannelMeta` 内嵌结构（`ChannelType`, `ChannelId`, `BaseUrl`, `ApiKey` 等）
- **计费信息**：`Billing BillingSettler`, `BillingSource`, `FinalPreConsumedQuota`, `PriceData`
- **分层计费**：`TieredBillingSnapshot`, `BillingRequestInput`
- **流式状态**：`StreamStatus`

#### `model.Channel` — 渠道模型

**文件**：`model/channel.go`

```go
type Channel struct {
    Id            int
    Type          int           // 提供商类型（constant.ChannelType*）
    Key           string        // API Key（支持多 key，逗号分隔）
    Status        int           // 启用/禁用
    BaseURL       *string       // 上游 API 地址
    Models        string        // 支持的模型列表
    Group         string        // 所属分组
    Priority      *int64        // 优先级
    Weight        *uint         // 权重
    ModelMapping  *string       // 模型名映射
    Setting       *string       // 渠道额外设置
    ParamOverride *string       // 参数覆盖
    ChannelInfo   ChannelInfo   // 多 key 模式等元信息
}
```

#### `types.NewAPIError` — 统一错误类型

**文件**：`types/error.go`

项目使用统一的错误类型，支持转换为 OpenAI / Claude / Gemini 等不同格式的错误响应：

```go
type NewAPIError struct {
    StatusCode int
    ErrorCode  ErrorCode
    ErrorType  ErrorType
    // 支持 ToOpenAIError(), ToClaudeError(), ToGeminiError() 转换
}
```

### 5.3 核心函数

| 函数 | 文件 | 职责 |
|---|---|---|
| `relay.GetAdaptor(apiType)` | `relay/relay_adaptor.go` | 工厂函数，根据 API 类型创建对应适配器 |
| `relay.GetTaskAdaptor(platform)` | `relay/relay_adaptor.go` | 工厂函数，根据平台创建任务适配器 |
| `relay.TextHelper(c, info)` | `relay/compatible_handler.go` | 文本补全/聊天的核心处理函数 |
| `controller.Relay(c, format)` | `controller/relay.go` | 中继请求的总入口，按格式分发 |
| `controller.relayHandler(c, info)` | `controller/relay.go` | 按 RelayMode 分发到具体 helper |
| `middleware.Distribute()` | `middleware/distributor.go` | 渠道分配中间件 |
| `service.CacheGetRandomSatisfiedChannel()` | `service/channel_select.go` | 渠道选择算法 |
| `service.PreConsumeQuota()` | `service/pre_consume_quota.go` | 预扣配额 |
| `service.SettleBilling()` | `service/billing.go` | 计费结算 |
| `service.PostTextConsumeQuota()` | `service/text_quota.go` | 文本请求的实际计费计算与结算 |
| `service.TaskPollingLoop()` | `service/task_polling.go` | 异步任务轮询主循环 |
| `model.InitChannelCache()` | `model/channel_cache.go` | 初始化渠道内存缓存 |
| `model.SyncChannelCache()` | `model/channel_cache.go` | 定时同步渠道缓存 |
| `common.InitRedisClient()` | `common/redis.go` | 初始化 Redis 连接 |
| `common.InitEnv()` | `common/init.go` | 初始化全局环境变量 |

---

## 6. 外部依赖

### 6.1 数据库

| 数据库 | 驱动 | 用途 | 备注 |
|---|---|---|---|
| **SQLite** | `glebarez/sqlite` | 主数据库（默认） | 通过 `SQLITE_PATH` 配置 |
| **MySQL** | `gorm.io/driver/mysql` | 主数据库（可选） | 通过 `SQL_DSN` 配置 |
| **PostgreSQL** | `gorm.io/driver/postgres` | 主数据库（可选） | 通过 `SQL_DSN` 配置 |

数据库类型检测和初始化在 `model/main.go` 的 `InitDB()` 中完成。通过 `common.UsingPostgreSQL` / `common.UsingMySQL` / `common.UsingSQLite` 标志位进行数据库特异性分支。

**日志数据库**：支持独立的日志数据库（通过 `LOG_SQL_DSN` 配置），与主数据库可以是不同类型。

### 6.2 缓存

| 组件 | 客户端 | 用途 | 必需？ |
|---|---|---|---|
| **Redis** | `go-redis/v8` | 分布式缓存、限流计数器、渠道亲和性缓存 | 可选 |
| **内存缓存** | `common/` 自实现 | 渠道列表、用户信息、Token 信息、配置项 | 始终启用 |
| **混合缓存** | `pkg/cachex/` | 内存 + Redis 双层缓存抽象 | — |
| **磁盘缓存** | `common/disk_cache.go` | 文件级别缓存 | 可选 |

**缓存同步机制**：`model.SyncChannelCache()` 定期（默认 60 秒）从数据库重新加载渠道信息到内存，通过 `channelSyncLock` (sync.RWMutex) 保证并发安全。

### 6.3 消息队列

**项目不使用消息队列**。异步任务通过数据库轮询实现（`service/task_polling.go` 中每 15 秒查询一次未完成任务）。

### 6.4 第三方 API / 服务

#### AI 提供商（36 个渠道适配器）

| 提供商 | 适配器目录 | API 类型常量 |
|---|---|---|
| OpenAI | `relay/channel/openai/` | `APITypeOpenAI` |
| Anthropic (Claude) | `relay/channel/claude/` | `APITypeAnthropic` |
| Google Gemini | `relay/channel/gemini/` | `APITypeGemini` |
| AWS Bedrock | `relay/channel/aws/` | `APITypeAws` |
| DeepSeek | `relay/channel/deepseek/` | `APITypeDeepSeek` |
| 阿里云 (通义) | `relay/channel/ali/` | `APITypeAli` |
| 百度 (文心) | `relay/channel/baidu/` | `APITypeBaidu` |
| 腾讯 (混元) | `relay/channel/tencent/` | `APITypeTencent` |
| 讯飞 (星火) | `relay/channel/xunfei/` | `APITypeXunfei` |
| 智谱 (GLM) | `relay/channel/zhipu/` | `APITypeZhipu` |
| Moonshot | `relay/channel/moonshot/` | `APITypeMoonshot` |
| SiliconFlow | `relay/channel/siliconflow/` | `APITypeSiliconFlow` |
| OpenRouter | `relay/channel/openai/`（复用） | `APITypeOpenRouter` |
| xAI (Grok) | `relay/channel/xai/` | `APITypeXai` |
| Codex | `relay/channel/codex/` | `APITypeCodex` |
| Ollama | `relay/channel/ollama/` | `APITypeOllama` |
| ... | ... | ... |

#### 支付服务

| 服务 | 控制器 | 备注 |
|---|---|---|
| Stripe | `controller/topup_stripe.go`, `controller/subscription_payment_stripe.go` | 国际支付 |
| Creem | `controller/topup_creem.go`, `controller/subscription_payment_creem.go` | 支付 |
| Waffo | `controller/topup_waffo.go` | 支付 |
| Waffo Pancake | `controller/topup_waffo_pancake.go` | 支付 |
| 易支付 (Epay) | `controller/topup.go`, `service/epay.go` | 国内支付 |

#### OAuth 提供商

| 提供商 | 文件 |
|---|---|
| GitHub | `oauth/github.go` |
| Discord | `oauth/discord.go` |
| OIDC (通用) | `oauth/oidc.go` |
| LinuxDo | `oauth/linuxdo.go` |
| 微信 | `controller/wechat.go` |
| Telegram | `controller/telegram.go` |
| 自定义 OAuth | `oauth/generic.go`, `oauth/registry.go` |

#### 其他外部服务

| 服务 | 用途 | 配置 |
|---|---|---|
| **Cloudflare Turnstile** | 人机验证 | `middleware/turnstile-check.go` |
| **Pyroscope** | 性能分析 | `common/pyro.go`，通过 `PYROSCOPE_URL` 配置 |
| **Umami / Google Analytics** | 前端埋点 | 通过 `UMAMI_WEBSITE_ID` / `GOOGLE_ANALYTICS_ID` 注入 |
| **Uptime Kuma** | 状态监控 | `controller/uptime_kuma.go` |

---

## 7. 架构风险和不清晰的地方

### 7.1 已识别的架构风险

#### (1) Controller 层直接调用 Relay 层

`controller/relay.go` 中的 `Relay()` 函数直接调用 `relay.TextHelper()` 等函数，绕过了 service 层。这意味着：
- 中继逻辑的计费/配额操作分散在 relay 层和 service 层
- 难以对中继流程进行单元测试
- relay 层需要回调 service 层（`service.PostTextConsumeQuota`），形成**双向依赖**

**影响**：中等。当前通过接口（`BillingSettler`）部分缓解，但耦合仍然较深。

#### (2) 渠道缓存的并发安全

`model/channel_cache.go` 使用 `sync.RWMutex` 保护 `group2model2channels` 和 `channelsIDM` 两个全局 map。但：
- `InitChannelCache()` 中对 `channelsIDM` 的更新是**先删除旧值再写入新值**（`delete(channelsIDM, i)` 循环），在写锁期间如果其他 goroutine 正在读取旧引用，可能读到不完整的数据
- 渠道的多 key 轮询索引 `MultiKeyPollingIndex` 在缓存刷新时需要从旧缓存迁移，增加了复杂度

**影响**：低。有锁保护，但实现较为脆弱。

#### (3) 环境变量配置过多

`.env.example` 列出了大量环境变量配置，但没有统一的配置验证机制。错误的环境变量值只在运行时才会暴露（如 Redis 连接失败、数据库连接字符串解析失败）。

**影响**：低。但增加了部署时的出错概率。

#### (4) 异步任务轮询效率

`service/task_polling.go` 中 `TaskPollingLoop()` 每 15 秒全量查询未完成任务（`GetAllUnFinishSyncTasks`）。当未完成任务量大时：
- 每次查询都会产生数据库压力
- 没有增量查询或事件驱动机制

**影响**：中等。对于大规模部署，可能需要引入消息队列或变更通知。

#### (5) 双前端维护成本

系统同时维护 Default（React 19 + Rsbuild）和 Classic（React 19 + Rsbuild + Semi Design）两套前端，通过 `go:embed` 嵌入同一二进制。这带来：
- 双倍的前端维护工作
- 功能可能在两套前端之间不同步
- 构建产物体积增大

**影响**：中等。推测 Classic 前端处于维护模式，但未找到明确的废弃计划。

### 7.2 不清晰的地方

#### (1) 多节点部署机制

代码中多次出现 `common.IsMasterNode` 判断（如 `main.go` 中只有主节点执行 Midjourney 任务轮询），但**多节点的具体协调机制不清晰**：
- 节点间如何发现和通信？
- 主节点故障时如何切换？
- `NODE_TYPE` 环境变量如何影响行为？

**推测**：可能是简单的主从模式，通过 Redis 或数据库实现简单协调，但缺乏明确的文档。

#### (2) `model/main.go` 中的数据库迁移策略

代码中有 `model.CheckSetup()` 和大量 AutoMigrate 调用，但**没有版本化的迁移文件**。所有 schema 变更通过 GORM AutoMigrate 自动执行：
- 无法回滚迁移
- 无法在迁移前备份
- 多节点同时 AutoMigrate 可能产生竞态

**已确认**：项目确实没有独立的数据库迁移策略。

#### (3) `relay/channel/openai/` 的复用范围

OpenRouter、Xinference、LiteMoonshot 等多个提供商**复用了 OpenAI 适配器**（`relay/relay_adaptor.go` 中 `case constant.APITypeOpenRouter: return &openai.Adaptor{}`）。这意味着：
- OpenAI 适配器需要处理所有兼容提供商的差异
- 新增 OpenAI 兼容提供商时，可能需要修改共享的 OpenAI 适配器

**影响**：中等。可能导致 OpenAI 适配器逐渐膨胀。

#### (4) `setting/` 的分包策略

`setting/` 下有 `billing_setting/`、`model_setting/`、`operation_setting/`、`performance_setting/` 等子目录，但也有直接放在 `setting/` 根目录的文件（如 `auto_group.go`、`chat.go`、`rate_limit.go`）。**分包标准不清晰**——有些按功能域分包，有些直接放在根目录。

#### (5) 错误处理的一致性

`types.NewAPIError` 提供了统一的错误类型，但并非所有层都一致使用：
- controller 层有时直接调用 `c.JSON()` 返回错误
- service 层有时返回 `error`，有时返回 `*types.NewAPIError`
- relay 层统一使用 `*types.NewAPIError`

**影响**：低。但增加了错误处理的复杂度。

---

## 附录 A：请求生命周期中的 Context Key 流转

```
TokenAuth() 写入:
  ├─ ContextKeyUserId
  ├─ ContextKeyTokenId
  ├─ ContextKeyTokenKey
  ├─ ContextKeyTokenGroup
  ├─ ContextKeyTokenUnlimited
  ├─ ContextKeyTokenModelLimit
  └─ ContextKeyTokenSpecificChannelId

Distribute() 写入:
  ├─ ContextKeyOriginalModel
  ├─ ContextKeyChannelId
  ├─ ContextKeyChannelType
  ├─ ContextKeyChannelBaseUrl
  ├─ ContextKeyChannelKey
  ├─ ContextKeyUsingGroup
  └─ ContextKeyAutoGroup

Relay 层写入:
  ├─ ContextKeyRelayMode
  └─ 各种请求特定的上下文
```

## 附录 B：数据库兼容性变量

**文件**：`model/main.go`

```go
var commonGroupCol string   // PostgreSQL: "group", MySQL/SQLite: `group`
var commonKeyCol string     // PostgreSQL: "key",   MySQL/SQLite: `key`
var commonTrueVal string    // PostgreSQL: "true",  MySQL/SQLite: "1"
var commonFalseVal string   // PostgreSQL: "false", MySQL/SQLite: "0"
```

使用示例（跨库兼容的 raw SQL）：

```go
DB.Raw(fmt.Sprintf("SELECT %s, SUM(quota) FROM log GROUP BY %s", commonGroupCol, commonGroupCol))
```

## 附录 C：前端主题切换机制

**文件**：`router/web-router.go`, `main.go`

```go
// main.go 中通过 go:embed 嵌入两套前端产物
//go:embed web/default/dist
var buildFS embed.FS

//go:embed web/classic/dist
var classicBuildFS embed.FS

// router/web-router.go 中根据 THEME 环境变量选择
func SetWebRouter(router *gin.Engine, assets ThemeAssets) {
    theme := os.Getenv("THEME") // "default" 或 "classic"
    // ...
}
```

## 附录 D：关键配置项速查

| 环境变量 | 默认值 | 说明 |
|---|---|---|
| `PORT` | 3000 | HTTP 监听端口 |
| `SQL_DSN` | 空（使用 SQLite） | 数据库连接字符串 |
| `SQLITE_PATH` | `one-api.db` | SQLite 文件路径 |
| `LOG_SQL_DSN` | 空（与主库相同） | 日志数据库连接字符串 |
| `REDIS_CONN_STRING` | 空 | Redis 连接字符串 |
| `SYNC_FREQUENCY` | 60 | 缓存同步频率（秒） |
| `SESSION_SECRET` | 随机生成 | Session 加密密钥 |
| `THEME` | `default` | 前端主题 |
| `NODE_TYPE` | 空（普通节点） | 节点类型（`master` 为主节点） |
| `GIN_MODE` | `release` | Gin 框架模式 |
| `DEBUG` | false | 调试模式 |
| `RELAY_TIMEOUT` | 0（不限制） | 上游请求超时（秒） |
| `STREAMING_TIMEOUT` | 120 | 流式无响应超时（秒） |
| `BATCH_UPDATE_ENABLED` | false | 批量更新配额 |
| `ENABLE_PPROF` | false | 启用 pprof 性能分析 |
| `ERROR_LOG_ENABLED` | false | 启用错误日志记录 |
| `FORCE_STREAM_OPTION` | false | 强制启用 StreamOptions |
| `TASK_TIMEOUT_MINUTES` | 0（不限制） | 异步任务超时时间 |
