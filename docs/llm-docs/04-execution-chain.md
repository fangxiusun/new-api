# New API — 启动与核心业务链路追踪

> 生成时间：2026-06-16
> 追踪方式：基于源码逐函数阅读，引用实际文件路径和函数签名

---

## 目录

1. [程序入口与启动流程](#1-程序入口与启动流程)
2. [初始化流程详解](#2-初始化流程详解)
3. [路由/中间件/后台任务注册](#3-路由中间件后台任务注册)
4. [核心链路一：AI 聊天中继请求](#4-核心链路一ai-聊天中继请求)
5. [核心链路二：异步任务提交与轮询](#5-核心链路二异步任务提交与轮询)
6. [关键函数速查表](#6-关键函数速查表)

---

## 1. 程序入口与启动流程

### 1.1 入口函数

**文件**：`main.go` → `func main()`

程序启动后按以下顺序执行：

```
main()
  │
  ├── [1] InitResources()                    ← 所有资源初始化
  │     ├── godotenv.Load(".env")            ← 加载 .env 文件
  │     ├── common.InitEnv()                 ← 解析环境变量和命令行参数
  │     ├── logger.SetupLogger()             ← 初始化日志系统
  │     ├── ratio_setting.InitRatioSettings() ← 初始化模型定价比例
  │     ├── service.InitHttpClient()         ← 初始化 HTTP 客户端（带超时/代理配置）
  │     ├── service.InitTokenEncoders()      ← 初始化 Token 编码器（tiktoken 等）
  │     ├── model.InitDB()                   ← 初始化主数据库连接
  │     ├── model.CheckSetup()               ← 检查系统是否已初始化
  │     ├── model.InitOptionMap()            ← 加载配置项到内存
  │     ├── common.CleanupOldCacheFiles()    ← 清理旧磁盘缓存
  │     ├── model.GetPricing()               ← 加载模型定价
  │     ├── model.InitLogDB()                ← 初始化日志数据库（可独立配置）
  │     ├── common.InitRedisClient()         ← 初始化 Redis（可选）
  │     ├── perfmetrics.Init()               ← 初始化性能指标采集
  │     ├── common.StartSystemMonitor()       ← 启动系统资源监控
  │     ├── i18n.Init()                      ← 初始化后端国际化
  │     └── oauth.LoadCustomProviders()      ← 加载自定义 OAuth 提供商
  │
  ├── [2] 后台任务注入
  │     ├── service.GetTaskAdaptorFunc = ...  ← 注入任务适配器工厂（打破循环依赖）
  │     ├── service.StartCodexCredentialAutoRefreshTask()
  │     ├── service.StartSubscriptionQuotaResetTask()
  │     └── controller.StartChannelUpstreamModelUpdateTask()
  │
  ├── [3] 条件性后台任务
  │     ├── [if IsMasterNode] gopool.Go(UpdateMidjourneyTaskBulk)
  │     ├── [if IsMasterNode] gopool.Go(UpdateTaskBulk)       ← 通用任务轮询
  │     ├── [if BATCH_UPDATE_ENABLED] model.InitBatchUpdater()
  │     ├── [if ENABLE_PPROF] http.ListenAndServe(:8005)
  │     └── common.StartPyroScope()          ← Pyroscope 性能分析（可选）
  │
  ├── [4] 创建 Gin 服务器
  │     ├── gin.New() + CustomRecovery
  │     ├── middleware.RequestId()
  │     ├── middleware.PoweredBy()
  │     ├── middleware.I18n()
  │     ├── middleware.SetUpLogger()
  │     └── sessions.Sessions("session", cookieStore)
  │
  ├── [5] 注入前端分析脚本
  │     ├── InjectUmamiAnalytics()
  │     └── InjectGoogleAnalytics()
  │
  ├── [6] 注册路由
  │     └── router.SetRouter(server, assets)
  │
  └── [7] 启动 HTTP 服务器
        └── server.Run(":" + port)           ← 默认 :3000
```

### 1.2 关键启动节点说明

#### `InitResources()` — 全部资源初始化

**文件**：`main.go:140` → `func InitResources() error`

这是启动阶段最重要的函数，负责所有外部资源的连接和内部状态的初始化。如果任何关键步骤失败（数据库、Redis 连接测试），程序会直接 Fatal 退出。

#### `common.InitEnv()` — 环境变量解析

**文件**：`common/init.go:27` → `func InitEnv()`

解析命令行参数和所有环境变量：

- `flag.Parse()` → 处理 `--port`、`--log-dir`、`--version`、`--help`
- `--version` / `--help` 时直接 `os.Exit(0)`
- `SESSION_SECRET` / `CRYPTO_SECRET` → 会话和加密密钥
- `SQLITE_PATH` → SQLite 文件路径
- `DEBUG` / `MEMORY_CACHE_ENABLED` / `NODE_TYPE` / `NODE_NAME` → 运行模式
- `TLS_INSECURE_SKIP_VERIFY` → TLS 跳过验证
- `SYNC_FREQUENCY` / `BATCH_UPDATE_INTERVAL` → 缓存同步频率
- `RELAY_TIMEOUT` / `RELAY_IDLE_CONN_TIMEOUT` → 上游请求超时
- 各种速率限制参数（`GLOBAL_API_RATE_LIMIT` 等）

#### `model.InitDB()` — 数据库初始化

**文件**：`model/main.go` → `func InitDB() error`

```
InitDB()
  ├── chooseDB("SQL_DSN", false)
  │     ├── DSN 以 "postgres://" 开头 → PostgreSQL
  │     ├── DSN 以 "local" 开头       → SQLite
  │     ├── DSN 有值（其他）           → MySQL
  │     └── DSN 为空                   → SQLite（默认）
  │
  ├── 设置连接池参数（MaxIdleConns / MaxOpenConns / MaxLifetime）
  ├── [MySQL] checkMySQLChineseSupport() ← 校验字符集支持中文
  ├── DB.AutoMigrate(所有模型)           ← 自动迁移表结构
  │     ├── User, Channel, Token, Option, Log
  │     ├── Redemption, Task, Midjourney
  │     ├── Ability, Pricing, Subscription...
  │     └── Setup, VendorMeta, ModelMeta...
  ├── createRootAccountIfNeed()          ← 无用户时创建 root/123456
  └── [if LOG_SQL_DSN] InitLogDB()       ← 独立日志数据库
```

#### `model.CheckSetup()` — 系统初始化状态检查

**文件**：`model/main.go` → `func CheckSetup()`

```
CheckSetup()
  ├── GetSetup() → 查询 setups 表
  │     ├── 记录存在   → constant.Setup = true（已初始化）
  │     └── 记录不存在 → 检查 RootUserExists()
  │           ├── root 用户存在   → 创建 setup 记录，Setup = true
  │           └── root 用户不存在 → Setup = false（等待向导）
```

---

## 2. 初始化流程详解

### 2.1 缓存初始化

**文件**：`model/channel_cache.go` → `func InitChannelCache()`

启动时将全量渠道数据加载到内存：

```
InitChannelCache()
  ├── DB.Find(&channels)             ← 查询所有渠道
  ├── DB.Find(&abilities)            ← 查询所有能力映射
  ├── 构建 newGroup2model2channels    ← group → model → []channelId
  ├── 按 priority 排序               ← 高优先级在前
  ├── 处理多 key 渠道                 ← 解析 JSON key 列表
  └── 加写锁替换全局变量
      ├── group2model2channels       ← 渠道选择的核心索引
      └── channelsIDM                ← channelId → *Channel 映射
```

### 2.2 配置项加载

**文件**：`model/option.go` → `func InitOptionMap()`

```
InitOptionMap()
  ├── 设置默认配置值（文件上传权限、登录开关等）
  ├── DB.Find(&options)              ← 从 options 表加载所有配置
  ├── 覆盖默认值
  ├── 触发 setting 子模块初始化
  │     ├── operation_setting.InitOperationSettings()
  │     ├── system_setting.InitSystemSettings()
  │     ├── performance_setting.InitPerformanceSettings()
  │     └── config.InitConfigSettings()
  └── 解密敏感配置值
```

### 2.3 Redis 初始化

**文件**：`common/redis.go` → `func InitRedisClient() error`

```
InitRedisClient()
  ├── [if REDIS_CONN_STRING 为空] → RedisEnabled = false, return nil
  ├── redis.ParseURL(connString)
  ├── 设置连接池大小（REDIS_POOL_SIZE，默认 10）
  ├── Ping 测试（5 秒超时）
  └── 失败 → FatalLog 退出
```

---

## 3. 路由/中间件/后台任务注册

### 3.1 路由注册总入口

**文件**：`router/main.go` → `func SetRouter(router *gin.Engine, assets ThemeAssets)`

```
SetRouter()
  ├── SetApiRouter(router)           ← /api/* 管理/业务路由
  ├── SetDashboardRouter(router)     ← /api/dashboard/* 仪表盘
  ├── SetRelayRouter(router)         ← /v1/* AI 中继路由
  ├── SetVideoRouter(router)         ← /v1/videos/* 视频路由
  └── SetWebRouter(router, assets)   ← 前端静态文件
```

### 3.2 中继路由注册（核心）

**文件**：`router/relay-router.go` → `func SetRelayRouter(router *gin.Engine)`

路由注册和中间件编排如下：

```
全局中间件:
  ├── middleware.CORS()
  ├── middleware.DecompressRequestMiddleware()
  ├── middleware.BodyStorageCleanup()
  └── middleware.StatsMiddleware()

/v1/models 路由组:
  ├── RouteTag("relay")
  └── TokenAuth()
      ├── GET /v1/models           → controller.ListModels
      └── GET /v1/models/:model    → controller.RetrieveModel

/v1 路由组（核心中继）:
  ├── RouteTag("relay")
  ├── SystemPerformanceCheck()
  ├── TokenAuth()
  ├── ModelRequestRateLimit()
  └── Distribute()                  ← 渠道分配中间件
      ├── POST /v1/chat/completions  → controller.Relay(OpenAI)
      ├── POST /v1/completions       → controller.Relay(OpenAI)
      ├── POST /v1/messages          → controller.Relay(Claude)
      ├── POST /v1/responses         → controller.Relay(OpenAIResponses)
      ├── POST /v1/embeddings        → controller.Relay(Embedding)
      ├── POST /v1/images/generations → controller.Relay(Image)
      ├── POST /v1/audio/speech      → controller.Relay(Audio)
      ├── POST /v1/audio/transcriptions → controller.Relay(Audio)
      ├── POST /v1/rerank            → controller.Relay(Rerank)
      └── POST /v1/models/*path      → controller.Relay(Gemini)

/v1beta 路由组（Gemini 原生）:
  ├── RouteTag("relay")
  ├── SystemPerformanceCheck()
  ├── TokenAuth()
  ├── ModelRequestRateLimit()
  ├── Distribute()
  └── POST /v1beta/models/*path → controller.Relay(Gemini)

/mj 路由组（Midjourney）:
  ├── RouteTag("relay")
  ├── SystemPerformanceCheck()
  ├── TokenAuth()
  ├── Distribute()
  └── POST /mj/submit/* → controller.RelayMidjourney

/suno 路由组:
  ├── TokenAuth() + Distribute()
  └── POST /suno/submit/:action → controller.RelayTask

/pg 路由组（Playground）:
  ├── UserAuth() + Distribute()
  └── POST /pg/chat/completions → controller.Playground
```

### 3.3 后台任务注册

**文件**：`main.go`

| 任务 | 启动方式 | 条件 | 文件 |
|---|---|---|---|
| `model.SyncChannelCache(freq)` | `go` goroutine | `MemoryCacheEnabled` | `model/channel_cache.go` |
| `model.SyncOptions(freq)` | `go` goroutine | 始终 | `main.go:86` |
| `model.UpdateQuotaData()` | `go` goroutine | 始终 | `main.go:89` |
| `controller.AutomaticallyUpdateChannels(freq)` | `go` goroutine | `CHANNEL_UPDATE_FREQUENCY` 非空 | `main.go:91` |
| `controller.AutomaticallyTestChannels()` | 启动时 | 始终 | `main.go:99` |
| `service.StartCodexCredentialAutoRefreshTask()` | 启动时 | 始终 | `main.go:102` |
| `service.StartSubscriptionQuotaResetTask()` | 启动时 | 始终 | `main.go:105` |
| `controller.UpdateMidjourneyTaskBulk()` | `gopool.Go` | `IsMasterNode && UpdateTask` | `main.go:119` |
| `service.TaskPollingLoop()` | `gopool.Go` | `IsMasterNode && UpdateTask` | `service/task_polling.go` |
| `model.InitBatchUpdater()` | 启动时 | `BATCH_UPDATE_ENABLED` | `main.go:125` |
| `common.StartPyroScope()` | 启动时 | 可选 | `common/pyro.go` |

---

## 4. 核心链路一：AI 聊天中继请求

以 `POST /v1/chat/completions` 为例，完整追踪从 HTTP 请求到响应的每一步。

### Step 1: Gin 接收请求

**文件**：`main.go:163`

```go
server.Run(":" + port)
```

Gin 框架接收 HTTP 请求，按注册的中间件链和路由表匹配。

### Step 2: 全局中间件

请求依次经过：

| 顺序 | 中间件 | 文件 | 作用 |
|---|---|---|---|
| 1 | `gin.CustomRecovery` | `main.go:147` | panic 恢复 |
| 2 | `middleware.RequestId()` | `middleware/request-id.go` | 生成唯一请求 ID |
| 3 | `middleware.PoweredBy()` | `middleware/header_nav.go` | 设置响应头 |
| 4 | `middleware.I18n()` | `middleware/i18n.go` | 注入 i18n 翻译函数到 Context |

### Step 3: 路由组中间件

**文件**：`router/relay-router.go`

请求匹配到 `/v1` 路由组后，依次经过：

```
middleware.RouteTag("relay")         ← 标记路由类型（用于日志分类）
middleware.SystemPerformanceCheck()  ← 检查系统负载是否过高
middleware.TokenAuth()               ← 验证 API Token
middleware.ModelRequestRateLimit()   ← 模型级别请求限流
middleware.Distribute()              ← 渠道分配（选择上游渠道）
```

### Step 4: TokenAuth — Token 认证

**文件**：`middleware/auth.go:286` → `func TokenAuth()`

```
TokenAuth()
  ├── 兼容多种 Key 来源:
  │     ├── Authorization: Bearer sk-xxx
  │     ├── x-api-key (Anthropic 格式)
  │     ├── x-goog-api-key (Gemini 格式)
  │     ├── ?key=xxx (Gemini query 参数)
  │     ├── Sec-WebSocket-Protocol (WebSocket)
  │     └── mj-api-secret (Midjourney)
  │
  ├── 解析 Key: strings.TrimPrefix("sk-") → split by "-"
  ├── model.ValidateUserToken(key)
  │     ├── 查询 token 表（按 Key 字段）
  │     ├── 检查状态、过期时间、剩余额度
  │     └── 返回 *model.Token
  │
  ├── 检查 Token IP 白名单
  ├── model.GetUserCache(token.UserId)
  │     └── 检查用户状态（是否被禁用）
  │
  ├── 检查 Token 分组权限
  │     └── service.GetUserUsableGroups()
  │
  ├── 设置 Context:
  │     ├── ContextKeyUserId = token.UserId
  │     ├── ContextKeyTokenId = token.Id
  │     ├── ContextKeyTokenKey = token.Key
  │     ├── ContextKeyTokenGroup = token.Group
  │     ├── ContextKeyTokenUnlimited = token.UnlimitedQuota
  │     ├── ContextKeyTokenModelLimit (map[string]bool)
  │     ├── ContextKeyUserGroup = user.Group
  │     ├── ContextKeyTokenSpecificChannelId (如果有)
  │     └── ContextKeyRequestStartTime = time.Now()
  │
  └── c.Next() → 进入下一个中间件
```

### Step 5: Distribute — 渠道分配

**文件**：`middleware/distributor.go` → `func Distribute()`

这是中继链路中**最关键的中间件**，负责选择一个可用的上游渠道：

```
Distribute()
  ├── getModelRequest(c) → 从请求体解析 model 字段
  │
  ├── [如果 Token 指定渠道] → model.GetChannelById()
  │
  ├── [如果 Token 有模型白名单] → 检查 model 是否在白名单内
  │
  ├── service.GetPreferredChannelByAffinity(c, model, group)
  │     └── 检查渠道亲和性缓存（之前请求粘性到某个渠道）
  │
  ├── [auto 分组] → service.GetUserAutoGroup() → 跨分组选择
  │
  ├── service.CacheGetRandomSatisfiedChannel(retryParam)
  │     ├── 从 group2model2channels 缓存中查找
  │     ├── 按 priority 排序选择
  │     ├── model.IsChannelSatisfy() 检查渠道是否满足条件
  │     ├── 支持跨分组重试（cross_group_retry）
  │     └── 返回 *model.Channel
  │
  ├── middleware.SetupContextForSelectedChannel(c, channel, model)
  │     ├── 设置 ContextKeyChannelId
  │     ├── 设置 ContextKeyChannelType
  │     ├── 设置 ContextKeyChannelBaseUrl
  │     ├── 设置 ContextKeyChannelKey（API Key）
  │     ├── 设置 ContextKeyOriginalModel（原始模型名）
  │     ├── 处理模型名映射（ModelMapping）
  │     ├── 设置 ContextKeyUsingGroup
  │     └── 设置 StatusCodeMapping
  │
  └── c.Next()
```

### Step 6: controller.Relay — 中继总入口

**文件**：`controller/relay.go:96` → `func Relay(c *gin.Context, relayFormat types.RelayFormat)`

这是中继请求的**总调度函数**，协调验证、计费、格式转换、上游调用和结算：

```
Relay(c, RelayFormatOpenAI)
  │
  ├── [Step 6a] helper.GetAndValidateRequest(c, relayFormat)
  │     └── 根据 relayFormat 解析请求体为对应的 DTO
  │           └── OpenAI 格式 → dto.GeneralOpenAIRequest
  │
  ├── [Step 6b] relaycommon.GenRelayInfo(c, relayFormat, request, nil)
  │     └── genBaseRelayInfo(c, request)
  │           ├── 从 Context 读取所有中间件设置的信息
  │           ├── 构建 RelayInfo 对象
  │           │     ├── UserId, TokenId, TokenKey, TokenGroup
  │           │     ├── OriginModelName, UsingGroup, UserGroup
  │           │     ├── RelayMode = Path2RelayMode(path)
  │           │     ├── IsStream = request.IsStream()
  │           │     └── StartTime
  │           └── InitRequestConversionChain()
  │
  ├── [Step 6c] 敏感词检查（可选）
  │     └── service.CheckSensitiveText()
  │
  ├── [Step 6d] Token 估算
  │     └── service.EstimateRequestToken(c, meta, relayInfo)
  │           └── 计算 prompt 大约 token 数（用于预扣费）
  │
  ├── [Step 6e] 计算价格
  │     ├── service.GetModelRatio()     → 模型比例
  │     ├── service.GetGroupRatio()     → 分组比例
  │     └── service.GetCompletionRatio() → 补全比例
  │
  ├── [Step 6f] 预扣配额
  │     └── service.PreConsumeBilling(c, preConsumedQuota, relayInfo)
  │           ├── NewBillingSession(c, relayInfo, preConsumedQuota)
  │           │     ├── [如果用户有订阅] → SubscriptionFunding
  │           │     │     └── model.PreConsumeUserSubscription()
  │           │     └── [否则] → WalletFunding
  │           │           └── model.DecreaseUserQuota()
  │           └── relayInfo.Billing = session
  │
  ├── [Step 6g] 重试循环
  │     └── for retry <= RetryTimes:
  │           ├── getChannel(c, relayInfo, retryParam)
  │           │     └── middleware.SetupContextForSelectedChannel()
  │           │
  │           └── relayHandler(c, info)
  │                 └── [进入 relay 层处理]
  │
  └── [Step 6h] 错误处理
        └── 如果有错误，格式化响应（OpenAI/Claude/Gemini 格式）
```

### Step 7: relayHandler — 按模式分发

**文件**：`controller/relay.go:34` → `func relayHandler(c *gin.Context, info *RelayInfo)`

```
relayHandler()
  ├── RelayModeImagesGenerations/Edits  → relay.ImageHelper()
  ├── RelayModeAudioSpeech/Transcription/Translation → relay.AudioHelper()
  ├── RelayModeRerank                   → relay.RerankHelper()
  ├── RelayModeEmbeddings               → relay.EmbeddingHelper()
  ├── RelayModeResponses/Compact        → relay.ResponsesHelper()
  └── [default: ChatCompletions]        → relay.TextHelper()  ← 最常用的路径
```

### Step 8: relay.TextHelper — 文本中继核心

**文件**：`relay/compatible_handler.go` → `func TextHelper(c, info)`

这是**最核心的函数**，处理聊天补全请求的完整生命周期：

```
TextHelper(c, info)
  │
  ├── [8a] 初始化
  │     ├── info.InitChannelMeta(c)     ← 从 Context 加载渠道元信息
  │     ├── DeepCopy(request)           ← 深拷贝请求（避免修改原始数据）
  │     └── helper.ModelMappedHelper()  ← 应用模型名映射
  │
  ├── [8b] 创建适配器
  │     ├── adaptor := GetAdaptor(info.ApiType)  ← 工厂函数
  │     │     └── switch apiType:
  │     │           case APITypeOpenAI    → &openai.Adaptor{}
  │     │           case APITypeAnthropic → &claude.Adaptor{}
  │     │           case APITypeGemini    → &gemini.Adaptor{}
  │     │           case APITypeDeepSeek  → &deepseek.Adaptor{}
  │     │           ...（共 37 种）
  │     └── adaptor.Init(info)
  │
  ├── [8c] 请求格式转换
  │     ├── [PassThrough 模式] → 直接透传原始请求体
  │     └── [正常模式]
  │           ├── adaptor.ConvertOpenAIRequest(c, info, request)
  │           │     └── 将 OpenAI 格式转换为目标提供商格式
  │           ├── 处理 SystemPrompt 覆盖
  │           ├── common.Marshal(convertedRequest) → JSON
  │           ├── RemoveDisabledFields()           ← 移除不支持的字段
  │           ├── ApplyParamOverride()             ← 应用参数覆盖
  │           └── NewOutboundJSONBody(jsonData)     ← 构建请求体
  │
  ├── [8d] 发送上游请求
  │     ├── adaptor.DoRequest(c, info, requestBody)
  │     │     └── http.NewRequest + http.Client.Do()
  │     └── 检查 HTTP 状态码
  │           └── [非 200] → service.RelayErrorHandler()
  │
  ├── [8e] 处理上游响应
  │     └── adaptor.DoResponse(c, httpResp, info)
  │           ├── [流式] → 逐块读取 SSE → 转换格式 → 写入响应
  │           │     └── 返回累积的 Usage
  │           └── [非流式] → 读取完整响应 → 转换格式 → 写入响应
  │                 └── 返回 Usage
  │
  └── [8f] 计费结算
        ├── service.PostTextConsumeQuota(c, info, usage, nil)
        │     ├── calculateTextQuotaSummary()       ← 计算各项 token 和费用
        │     │     ├── PromptTokens / CompletionTokens / CacheTokens
        │     │     ├── ImageTokens / AudioTokens
        │     │     ├── ModelRatio × GroupRatio × CompletionRatio
        │     │     └── 最终 Quota
        │     ├── [如果有分层计费] → TryTieredSettle()
        │     ├── model.UpdateUserUsedQuotaAndRequestCount()
        │     ├── model.UpdateChannelUsedQuota()
        │     ├── SettleBilling(ctx, relayInfo, quota)
        │     │     ├── relayInfo.Billing.Settle(actualQuota)
        │     │     │     ├── Funding.Settle(delta)   ← 调整钱包/订阅
        │     │     │     └── model.Decrease/IncreaseTokenQuota()
        │     │     └── 发送额度通知
        │     └── model.RecordLog(c, log)            ← 记录使用日志
        │
        └── [如果有音频 token] → service.PostAudioConsumeQuota()
```

### 完整链路时序图

```
Client           Router      Middleware          Controller     Relay          Service        Model     Upstream
  │                │             │                   │             │              │             │          │
  │ POST /v1/chat/ │             │                   │             │              │             │          │
  │ completions    │             │                   │             │              │             │          │
  │───────────────►│             │                   │             │              │             │          │
  │                │ RouteTag    │                   │             │              │             │          │
  │                │────────────►│                   │             │              │             │          │
  │                │             │ TokenAuth()       │             │              │             │          │
  │                │             │──ValidateUserToken─────────────────────────────►│             │          │
  │                │             │◄───────────────────────────────────────────────│             │          │
  │                │             │ Distribute()      │             │              │             │          │
  │                │             │──CacheGetRandomSatisfiedChannel────────────────►│             │          │
  │                │             │◄──────────────────────────────────────────────│             │          │
  │                │             │                   │             │              │             │          │
  │                │             │ Context 已填充     │             │              │             │          │
  │                │─────────────────────────────────►│             │              │             │          │
  │                │             │                   │ Relay()     │              │             │          │
  │                │             │                   │             │              │             │          │
  │                │             │                   │ PreConsumeBilling           │             │          │
  │                │             │                   │────────────────────────────►│             │          │
  │                │             │                   │◄────────────────────────────│             │          │
  │                │             │                   │             │              │             │          │
  │                │             │                   │ relayHandler│              │             │          │
  │                │             │                   │────────────►│              │             │          │
  │                │             │                   │             │ TextHelper() │             │          │
  │                │             │                   │             │              │             │          │
  │                │             │                   │             │ ConvertReq   │             │          │
  │                │             │                   │             │ DoRequest()  │             │          │
  │                │             │                   │             │──────────────────────────────────────►│
  │                │             │                   │             │◄──────────────────────────────────────│
  │                │             │                   │             │              │             │          │
  │                │             │                   │             │ DoResponse() │             │          │
  │◄───────────────────────────────────────────────────────────── │ (SSE stream) │             │          │
  │                │             │                   │             │              │             │          │
  │                │             │                   │             │ PostTextConsumeQuota        │          │
  │                │             │                   │             │──────────────►│             │          │
  │                │             │                   │             │ SettleBilling│             │          │
  │                │             │                   │             │──────────────►│             │          │
  │                │             │                   │             │              │ UpdateQuota │          │
  │                │             │                   │             │              │────────────►│          │
  │                │             │                   │             │              │ RecordLog   │          │
  │                │             │                   │             │              │────────────►│          │
```

---

## 5. 核心链路二：异步任务提交与轮询

以视频生成任务 `POST /v1/videos/submit` 为例。

### Step 1-5: 与聊天中继相同

TokenAuth → Distribute → controller.RelayTask() → 重试循环

### Step 6: controller.RelayTask — 任务提交入口

**文件**：`controller/relay.go` → `func RelayTask(c *gin.Context)`

```
RelayTask(c)
  │
  ├── [6a] 解析请求 + 构建 RelayInfo
  │     ├── helper.GetAndValidateRequest(c, RelayFormatTask)
  │     └── relaycommon.GenRelayInfo(c, RelayFormatTask, request, nil)
  │
  ├── [6b] relay.ResolveOriginTask(c, info)
  │     └── 如果是 remix/continuation，锁定到原始任务的渠道
  │
  ├── [6c] 重试循环 for retry <= RetryTimes:
  │     ├── getChannel(c, relayInfo, retryParam)
  │     ├── c.Request.Body = bodyStorage（重置请求体）
  │     └── relay.RelayTaskSubmit(c, relayInfo)
  │           ├── [6c.1] taskAdaptor := GetTaskAdaptor(platform)
  │           ├── [6c.2] taskAdaptor.Init(info)
  │           ├── [6c.3] taskAdaptor.ValidateRequestAndSetAction()
  │           ├── [6c.4] taskAdaptor.EstimateBilling()
  │           │     └── 返回 OtherRatios（时长、分辨率等）
  │           ├── [6c.5] service.PreConsumeBilling()
  │           │     └── ForcePreConsume = true（全额预扣）
  │           ├── [6c.6] taskAdaptor.DoRequest()
  │           │     └── 发送到上游 API
  │           ├── [6c.7] taskAdaptor.DoResponse()
  │           │     └── 返回 TaskSubmitResult{UpstreamTaskID, TaskData, Quota}
  │           └── [6c.8] taskAdaptor.AdjustBillingOnSubmit()
  │                 └── 根据上游返回的实际参数调整计费
  │
  ├── [6d] 成功后:
  │     ├── service.SettleBilling(c, relayInfo, result.Quota)
  │     ├── service.LogTaskConsumption(c, relayInfo)
  │     ├── model.InitTask(platform, relayInfo)
  │     │     └── 构建 Task 对象
  │     │           ├── Platform, TaskID, Action
  │     │           ├── UserId, ChannelId, ModelName
  │     │           ├── Status = TaskStatusSubmitted
  │     │           └── Quota
  │     └── task.Insert()               ← 写入 tasks 表
  │
  └── [6e] 失败 → respondTaskError(c, taskErr)
```

### Step 7: 后台轮询 — TaskPollingLoop

**文件**：`service/task_polling.go` → `func TaskPollingLoop()`

```
TaskPollingLoop()                     ← main.go → UpdateTaskBulk() 启动
  │
  └── for {                           ← 无限循环
        time.Sleep(15s)
        │
        ├── sweepTimedOutTasks(ctx)   ← 清理超时任务
        │     ├── model.GetTimedOutUnfinishedTasks(cutoff, 100)
        │     ├── task.Status = TaskStatusFailure
        │     ├── task.UpdateWithStatus(oldStatus)  ← CAS 更新
        │     └── RefundTaskQuota()   ← 退还预扣费
        │
        ├── model.GetAllUnFinishSyncTasks(limit)
        │     └── 查询 status IN (submitted, processing) 的任务
        │
        ├── 按 platform 分组
        │
        └── for platform, tasks := range platformTask:
              ├── GetTaskAdaptorFunc(platform)
              │     └── relay.GetTaskAdaptor()
              │           ├── TaskPlatformSuno → &suno.TaskAdaptor{}
              │           ├── ChannelTypeAli   → &taskali.TaskAdaptor{}
              │           ├── ChannelTypeKling → &kling.TaskAdaptor{}
              │           └── ...（10+ 个任务平台）
              │
              ├── 按 channelId 分组
              │
              └── for channelId, taskIds:
                    ├── model.GetChannelById(channelId)
                    ├── adaptor.Init(info)
                    │
                    └── for each task:
                          ├── adaptor.FetchTask(baseURL, key, body, proxy)
                          │     └── HTTP 查询上游任务状态
                          │
                          ├── adaptor.ParseTaskResult(body)
                          │     └── 返回 TaskInfo{Status, Progress, Result}
                          │
                          ├── task.UpdateWithStatus(oldStatus)  ← CAS
                          │
                          └── [if 终态]:
                                ├── adaptor.AdjustBillingOnComplete()
                                │     └── 返回实际配额（差额结算）
                                └── service.SettleBilling()
```

### 任务状态机

```
                    ┌──────────┐
                    │ submitted│ ← task.Insert()
                    └────┬─────┘
                         │
              轮询 FetchTask()
                         │
           ┌─────────────┼──────────────┐
           │             │              │
     ┌─────▼─────┐ ┌─────▼─────┐ ┌──────▼─────┐
     │ processing │ │  failure  │ │  success   │
     └─────┬─────┘ └───────────┘ └────────────┘
           │              ▲              ▲
     轮询继续             │              │
           │         超时清理            │
           └────────── sweepTimedOutTasks()
                   CAS 更新防止重复处理
```

---

## 6. 关键函数速查表

### 启动阶段

| 函数 | 文件 | 职责 |
|---|---|---|
| `main()` | `main.go` | 程序入口 |
| `InitResources()` | `main.go` | 全部资源初始化 |
| `common.InitEnv()` | `common/init.go` | 环境变量解析 |
| `model.InitDB()` | `model/main.go` | 数据库初始化 + AutoMigrate |
| `model.CheckSetup()` | `model/main.go` | 系统初始化状态检查 |
| `model.InitOptionMap()` | `model/option.go` | 配置项加载 |
| `model.InitChannelCache()` | `model/channel_cache.go` | 渠道缓存初始化 |
| `common.InitRedisClient()` | `common/redis.go` | Redis 初始化 |
| `i18n.Init()` | `i18n/i18n.go` | 国际化初始化 |

### 路由阶段

| 函数 | 文件 | 职责 |
|---|---|---|
| `router.SetRouter()` | `router/main.go` | 路由总入口 |
| `router.SetRelayRouter()` | `router/relay-router.go` | 中继路由注册 |
| `router.SetApiRouter()` | `router/api-router.go` | 业务 API 路由注册 |

### 中间件阶段

| 函数 | 文件 | 职责 |
|---|---|---|
| `middleware.TokenAuth()` | `middleware/auth.go:286` | Token 认证 |
| `middleware.Distribute()` | `middleware/distributor.go` | 渠道分配 |
| `middleware.ModelRequestRateLimit()` | `middleware/model-rate-limit.go` | 模型限流 |
| `middleware.SystemPerformanceCheck()` | `middleware/performance.go` | 系统性能检查 |

### 中继阶段

| 函数 | 文件 | 职责 |
|---|---|---|
| `controller.Relay()` | `controller/relay.go:96` | 中继总入口 |
| `controller.relayHandler()` | `controller/relay.go:34` | 按 RelayMode 分发 |
| `relay.TextHelper()` | `relay/compatible_handler.go` | 文本中继核心 |
| `relay.ImageHelper()` | `relay/image_handler.go` | 图片中继 |
| `relay.AudioHelper()` | `relay/audio_handler.go` | 音频中继 |
| `relay.GeminiHelper()` | `relay/gemini_handler.go` | Gemini 格式中继 |
| `relay.ClaudeHelper()` | `relay/claude_handler.go` | Claude 格式中继 |
| `relay.EmbeddingHelper()` | `relay/embedding_handler.go` | 嵌入向量中继 |
| `relay.ResponsesHelper()` | `relay/responses_handler.go` | Responses API 中继 |
| `relay.GetAdaptor()` | `relay/relay_adaptor.go` | 适配器工厂 |
| `relaycommon.GenRelayInfo()` | `relay/common/relay_info.go:537` | 构建 RelayInfo |

### 计费阶段

| 函数 | 文件 | 职责 |
|---|---|---|
| `service.PreConsumeBilling()` | `service/billing.go` | 预扣费入口 |
| `service.NewBillingSession()` | `service/billing_session.go` | 创建计费会话 |
| `service.SettleBilling()` | `service/billing.go` | 结算入口 |
| `service.PostTextConsumeQuota()` | `service/text_quota.go` | 文本计费计算+结算 |
| `BillingSession.Settle()` | `service/billing_session.go` | 执行结算（资金+令牌） |
| `BillingSession.Refund()` | `service/billing_session.go` | 退款 |
| `WalletFunding.PreConsume()` | `service/funding_source.go` | 钱包预扣 |
| `SubscriptionFunding.PreConsume()` | `service/funding_source.go` | 订阅预扣 |

### 任务阶段

| 函数 | 文件 | 职责 |
|---|---|---|
| `controller.RelayTask()` | `controller/relay.go` | 任务提交入口 |
| `relay.RelayTaskSubmit()` | `relay/relay_task.go` | 任务提交核心 |
| `relay.ResolveOriginTask()` | `relay/relay_task.go` | 解析原始任务（remix） |
| `service.TaskPollingLoop()` | `service/task_polling.go` | 任务轮询主循环 |
| `service.sweepTimedOutTasks()` | `service/task_polling.go` | 超时任务清理 |
| `relay.GetTaskAdaptor()` | `relay/relay_adaptor.go` | 任务适配器工厂 |

### 渠道选择

| 函数 | 文件 | 职责 |
|---|---|---|
| `service.CacheGetRandomSatisfiedChannel()` | `service/channel_select.go` | 渠道选择算法 |
| `service.GetPreferredChannelByAffinity()` | `service/channel_affinity.go` | 渠道亲和性 |
| `model.IsChannelSatisfy()` | `model/channel_satisfy.go` | 渠道条件检查 |
| `model.CacheGetChannel()` | `model/channel_cache.go` | 从缓存获取渠道 |
| `middleware.SetupContextForSelectedChannel()` | `middleware/distributor.go` | 设置渠道上下文 |
