# Video API 接口实现详细分析

本文档以 `router/video-router.go` 为起点，详细梳理视频相关 API 接口的完整调用路径、实现逻辑及计费流程。

---

## 一、路由定义总览

**入口文件**：`router/video-router.go`

该文件定义了三大类路由组：

### 1.1 OpenAI 兼容路由（`/v1`）

| 路由 | 方法 | 处理函数 | 中间件 |
|------|------|----------|--------|
| `/v1/videos/:task_id/content` | GET | `VideoProxy` | `TokenOrUserAuth` |
| `/v1/video/generations` | POST | `RelayTask` | `TokenAuth`, `Distribute` |
| `/v1/video/generations/:task_id` | GET | `RelayTaskFetch` | `TokenAuth`, `Distribute` |
| `/v1/videos/:video_id/remix` | POST | `RelayTask` | `TokenAuth`, `Distribute` |
| `/v1/videos` | POST | `RelayTask` | `TokenAuth`, `Distribute` |
| `/v1/videos/:task_id` | GET | `RelayTaskFetch` | `TokenAuth`, `Distribute` |

### 1.2 Kling 原生路由（`/kling/v1`）

| 路由 | 方法 | 处理函数 | 中间件 |
|------|------|----------|--------|
| `/kling/v1/videos/text2video` | POST | `RelayTask` | `KlingRequestConvert`, `TokenAuth`, `Distribute` |
| `/kling/v1/videos/image2video` | POST | `RelayTask` | `KlingRequestConvert`, `TokenAuth`, `Distribute` |
| `/kling/v1/videos/text2video/:task_id` | GET | `RelayTaskFetch` | `KlingRequestConvert`, `TokenAuth`, `Distribute` |
| `/kling/v1/videos/image2video/:task_id` | GET | `RelayTaskFetch` | `KlingRequestConvert`, `TokenAuth`, `Distribute` |

### 1.3 即梦原生路由（`/jimeng`）

| 路由 | 方法 | 处理函数 | 中间件 |
|------|------|----------|--------|
| `/jimeng/` | POST | `RelayTask` | `JimengRequestConvert`, `TokenAuth`, `Distribute` |

---

## 二、核心接口详细实现流程

### 2.1 视频任务提交接口（`RelayTask`）

**文件**：`controller/relay.go:485` → `relay/relay_task.go:120`（`RelayTaskSubmit`）

#### 调用链路

`
HTTP POST /v1/videos
  → middleware.TokenAuth()                    // JWT 认证，提取用户信息
  → middleware.Distribute()                   // 分组分发，确定 channel_id
  → controller.RelayTask()
    → relaycommon.GenRelayInfo()              // 生成 RelayInfo，RelayFormat = Task
    → relay.ResolveOriginTask()               // 处理 remix 场景（锁定原始任务渠道）
    → relay.RelayTaskSubmit()                 // 核心提交逻辑
      → info.InitChannelMeta()                // 初始化渠道元数据
      → GetTaskPlatform()                     // 确定平台（sora/kling/jimeng/...）
      → GetTaskAdaptor()                      // 获取平台适配器
      → adaptor.Init()
      → adaptor.ValidateRequestAndSetAction() // 验证请求 & 设置 action
      → helper.ModelMappedHelper()            // 模型映射
      → model.GenerateTaskID()                // 预生成公开 task ID
      → helper.ModelPriceHelperPerCall()      // 【计费】计算基础价格
      → adaptor.EstimateBilling()             // 【计费】计算附加倍率（时长/分辨率）
      → service.PreConsumeBilling()           // 【计费】预扣费
      → adaptor.BuildRequestBody()            // 构建上游请求体
      → adaptor.DoRequest()                   // 发送请求到上游
      → adaptor.DoResponse()                  // 解析上游响应
      → adaptor.AdjustBillingOnSubmit()        // 【计费】提交后计费调整
    → service.SettleBilling()                  // 【计费】结算差额
    → service.LogTaskConsumption()             // 记录消费日志
    → task.Insert()                            // 持久化任务到数据库
`

#### 详细逻辑说明

**1. RelayInfo 生成**（`relay/common/relay_info.go`）：
- 根据 `types.RelayFormatTask` 格式生成基础信息
- 初始化 `TaskRelayInfo` 结构

**2. 平台识别**（`relay/relay_adaptor.go:127`）：
- 从 context 中获取 `channel_type` 或 `platform` 字符串
- 通过 `GetTaskAdaptor()` 映射到具体适配器：
  - `ChannelTypeOpenAI` / `ChannelTypeSora` → `tasksora.TaskAdaptor`
  - `ChannelTypeKling` → `kling.TaskAdaptor`
  - `ChannelTypeJimeng` → `taskjimeng.TaskAdaptor`
  - `ChannelTypeVertexAi` → `taskvertex.TaskAdaptor`
  - `ChannelTypeGemini` → `taskGemini.TaskAdaptor`
  - `ChannelTypeAli` → `taskali.TaskAdaptor`
  - `ChannelTypeVidu` → `taskVidu.TaskAdaptor`
  - `ChannelTypeDoubaoVideo` / `ChannelTypeVolcEngine` → `taskdoubao.TaskAdaptor`
  - `ChannelTypeMiniMax` → `hailuo.TaskAdaptor`

**3. 请求验证**：各适配器实现 `ValidateRequestAndSetAction`，例如：
- Sora：支持 JSON 和 multipart/form-data 格式
- Kling：解析 `model_name`/`model`、`prompt`、`image` 等字段
- Jimeng：解析 `req_key`、`prompt`、`action` 等字段

**4. 重试机制**：控制器层包含重试循环（最多 `common.RetryTimes` 次），每次重试：
- 重新获取 channel
- 重新读取 request body
- 调用 `RelayTaskSubmit`

---

### 2.2 视频任务查询接口（`RelayTaskFetch`）

**文件**：`controller/relay.go:470` → `relay/relay_task.go:287`（`RelayTaskFetch`）

#### 调用链路

`
HTTP GET /v1/videos/:task_id
  → middleware.TokenAuth()
  → middleware.Distribute()
  → controller.RelayTaskFetch()
    → relaycommon.GenRelayInfo()
    → relay.RelayTaskFetch()
      → videoFetchByIDRespBodyBuilder()       // 构建查询响应
        → model.GetByTaskId()                 // 从数据库查询任务
        → tryRealtimeFetch()                  // Gemini/Vertex 实时查询上游
        → adaptor.ConvertToOpenAIVideo()      // 转换为 OpenAI Video 格式
`

#### 详细逻辑说明

**1. 路由模式匹配**：`Path2RelayMode` 将 `/v1/videos/:task_id` 映射为 `RelayModeVideoFetchByID`

**2. 实时查询**（`tryRealtimeFetch`）：
- 仅对 Gemini/Vertex 类型渠道生效
- 直接调用上游 API 获取最新任务状态
- 更新数据库中的任务状态和结果 URL
- 通过 CAS（`UpdateWithStatus`）防止并发覆盖

**3. 格式转换**：
- OpenAI Video API 路径（`/v1/videos/`）：调用 `ConvertToOpenAIVideo`
- 通用路径：返回 `TaskDto` 格式

---

### 2.3 视频内容代理接口（`VideoProxy`）

**文件**：`controller/video_proxy.go:33`

#### 调用链路

`
HTTP GET /v1/videos/:task_id/content
  → middleware.TokenOrUserAuth()              // 支持 session 或 token 认证
  → controller.VideoProxy()
    → model.GetByTaskId()                     // 查询任务
    → model.CacheGetChannel()                 // 获取渠道信息
    → service.GetHttpClientWithProxy()        // 创建 HTTP 客户端
    → 根据渠道类型解析视频 URL
    → http.Get(videoURL)                      // 代理获取视频内容
    → io.Copy(response)                       // 流式返回
`

#### 详细逻辑说明

**1. 渠道类型适配**：
- **Gemini**：调用 `getGeminiVideoURL()` 解析视频 URL，使用存储的 API Key
- **VertexAi**：调用 `getVertexVideoURL()` 解析视频 URL
- **OpenAI/Sora**：拼接 `{baseURL}/v1/videos/{upstreamTaskID}/content`，使用渠道 Key
- **其他**：从 `task.PrivateData.ResultURL` 获取

**2. Data URL 处理**：支持 `data:` 前缀的 Base64 编码视频数据

**3. 安全检查**：
- SSRF 防护：`common.ValidateURLWithFetchSetting`
- 私有 IP 检查
- 域名/端口过滤

---

## 三、中间件实现

### 3.1 KlingRequestConvert

**文件**：`middleware/kling_adapter.go:14`

将 Kling 原生请求转换为统一格式：
- 解析 `model_name`/`model`、`prompt` 字段
- 将原始请求作为 `metadata` 附加
- 重写请求路径为 `/v1/video/generations`
- 设置 `action`（有 image 则为 `generate`，否则为 `text_generate`）

### 3.2 JimengRequestConvert

**文件**：`middleware/jimeng_adapter.go:15`

将即梦原生请求转换为统一格式：
- 解析 `Action` 查询参数（`CVSync2AsyncSubmitTask` 或 `CVSync2AsyncGetResult`）
- 解析 `req_key`、`prompt` 字段
- 提交时重写路径为 `/v1/video/generations`
- 查询时重写路径为 `/v1/video/generations/{task_id}`，并设置 `relay_mode` 为 `RelayModeVideoFetchByID`

---

## 四、计费系统详细流程

### 4.1 计费架构概览

计费系统采用 **预扣费 + 结算/退款** 模型：

`
请求进入
  ↓
计算预扣费额度（ModelPriceHelperPerCall）
  ↓
创建 BillingSession（NewBillingSession）
  ├── 选择资金来源（钱包/订阅）
  ├── 预扣令牌额度（PreConsumeTokenQuota）
  └── 预扣资金（funding.PreConsume）
  ↓
请求成功
  ├── SettleBilling（差额结算）
  └── LogTaskConsumption（记录日志）

请求失败
  ├── BillingSession.Refund（退还预扣费）
  └── ChargeViolationFeeIfNeeded（违规扣费）
`

### 4.2 计费配置获取

**文件**：`relay/helper/price.go`

#### 4.2.1 按次计费（`ModelPriceHelperPerCall`）

用于视频/图像等异步任务：

`
1. 获取分组倍率（HandleGroupRatio）
   → ratio_setting.GetGroupRatio(group)
   → ratio_setting.GetGroupGroupRatio(userGroup, group)

2. 获取模型价格
   → ratio_setting.GetModelPrice(modelName, true)  // true = 按次价格
   → 若未配置，尝试 ratio_setting.GetDefaultModelPriceMap()
   → 若仍未配置，尝试 ratio_setting.GetModelRatio()

3. 计算预扣费额度
   if usePrice:
     quota = modelPrice * QuotaPerUnit * groupRatio
   else:
     quota = modelRatio / 2 * QuotaPerUnit * groupRatio  // 按量计费取一半作为预扣

4. 免费模型判断
   if !EnableFreeModelPreConsume && (groupRatio == 0 || price == 0):
     quota = 0, freeModel = true
`

#### 4.2.2 附加倍率（`EstimateBilling`）

各适配器实现 `EstimateBilling` 方法，根据用户请求参数计算附加倍率：

- **Sora 适配器**（`relay/channel/task/sora/adaptor.go:98`）：
  - `seconds`：视频时长（默认 4 秒），直接作为倍率
  - `size`：分辨率，`1792x1024` 或 `1024x1792` 时倍率为 1.666667，其他为 1

- **Kling/Vertex/Gemini 等**：继承 `taskcommon.BaseBilling`，返回 nil（无附加倍率）

- **Ali 适配器**（`relay/channel/task/ali/adaptor.go:349`）：根据分辨率和时长计算倍率

#### 4.2.3 倍率应用

`go
// relay/relay_task.go:197
if !common.StringsContains(constant.TaskPricePatches, modelName) {
    for _, ra := range info.PriceData.OtherRatios {
        if ra != 1.0 {
            info.PriceData.Quota = int(float64(info.PriceData.Quota) * ra)
        }
    }
}
`

`TaskPricePatches`（通过环境变量 `TASK_PRICE_PATCHES` 配置）中的模型跳过倍率应用，直接使用基础价格。

### 4.3 预扣费流程

**文件**：`service/billing.go:19` → `service/billing_session.go`

#### 4.3.1 创建 BillingSession（`NewBillingSession`）

`
1. 确定计费偏好（billing_preference）
   - subscription_only: 仅订阅
   - wallet_only: 仅钱包
   - wallet_first: 钱包优先，不足时回退到订阅
   - subscription_first: 订阅优先，不足时回退到钱包

2. 钱包路径（tryWallet）
   → model.GetUserQuota() 检查余额
   → 验证 userQuota >= preConsumedQuota
   → session.preConsume()

3. 订阅路径（trySubscription）
   → model.HasActiveUserSubscription() 检查订阅
   → session.preConsume()
`

#### 4.3.2 执行预扣费（`BillingSession.preConsume`）

`
1. 信任额度检查（shouldTrust）
   - ForcePreConsume = true 时跳过信任检查（异步任务强制预扣）
   - 仅钱包路径支持信任旁路
   - 条件：tokenQuota > trustQuota && userQuota > trustQuota

2. 预扣令牌额度（PreConsumeTokenQuota）
   → model.PreConsumeTokenQuota()

3. 预扣资金来源（funding.PreConsume）
   - 钱包：model.DecreaseUserQuota()
   - 订阅：model.PreConsumeUserSubscription()

4. 失败回滚
   - 若资金预扣失败，回滚令牌额度
`

### 4.4 结算流程

**文件**：`service/billing.go:34`

任务成功后调用 `SettleBilling`：

`
1. 计算差额 = actualQuota - preConsumedQuota
   - delta > 0: 补扣费
   - delta < 0: 退还多扣费用
   - delta == 0: 无需调整

2. 调用 BillingSession.Settle(actualQuota)
   → funding.Settle(delta)                      // 调整资金来源
   → model.DecreaseTokenQuota / IncreaseTokenQuota  // 调整令牌额度

3. 发送额度通知
`

### 4.5 退款流程

**文件**：`service/billing_session.go:106`

请求失败时调用 `BillingSession.Refund`：

`
1. 幂等检查：settled || refunded || fundingSettled 时跳过

2. 异步执行（gopool.Go）
   → funding.Refund()                           // 退还资金来源
     - 钱包：model.IncreaseUserQuota(consumed)
     - 订阅：model.RefundSubscriptionPreConsume(requestId)（带重试）
   → model.IncreaseTokenQuota(tokenConsumed)    // 退还令牌额度
`

### 4.6 任务完成时的计费调整

**文件**：`service/task_polling.go:276`（`settleTaskBillingOnComplete`）

轮询发现任务完成时执行：

`
1. 按次计费检查（PerCallBilling）
   → 若为按次计费，跳过差额结算

2. adaptor.AdjustBillingOnComplete()
   → 适配器返回正数时，使用该值进行差额结算

3. token 重算（RecalculateTaskQuotaByTokens）
   → 若上游返回 totalTokens，按 token 重算实际费用
   → actualQuota = totalTokens * modelRatio * groupRatio * otherMultiplier

4. 无调整时保持预扣额度
`

### 4.7 任务失败退款

**文件**：`service/task_billing.go:152`（`RefundTaskQuota`）

`
1. 退还资金来源（taskAdjustFunding）
   - 订阅：model.PostConsumeUserSubscriptionDelta(-quota)
   - 钱包：model.IncreaseUserQuota(quota)

2. 退还令牌额度（taskAdjustTokenQuota）
   → model.IncreaseTokenQuota()

3. 记录退款日志（model.RecordTaskBillingLog）
`

### 4.8 违规扣费

**文件**：`service/violation_fee.go:104`

当检测到 CSAM 违规时额外扣费：

`
1. 判断是否需要扣费（shouldChargeViolationFee）
   → error.code == ViolationFeeGrokCSAM 或包含 CSAM 标记

2. 计算违规费用
   → feeQuota = ViolationDeductionAmount * QuotaPerUnit * groupRatio

3. 执行扣费（PostConsumeQuota）

4. 记录消费日志
`

---

## 五、周期性自动调用（任务轮询）

### 5.1 轮询启动入口

**入口文件**：`controller/task.go:19`

`go
func UpdateTaskBulk() {
    service.TaskPollingLoop()
}
`

系统启动时调用 `UpdateTaskBulk()` 启动轮询循环。

### 5.2 轮询主循环

**文件**：`service/task_polling.go:91`（`TaskPollingLoop`）

`
每 15 秒执行一次：

1. sweepTimedOutTasks()                    // 清理超时任务
   → model.GetTimedOutUnfinishedTasks(cutoff, 100)
   → 超时时间：TaskTimeoutMinutes 分钟
   → 使用 CAS 更新防止并发冲突
   → 退还预扣费（RefundTaskQuota）

2. model.GetAllUnFinishSyncTasks(limit)    // 获取未完成任务

3. 按平台分组（platformTask）

4. 按渠道分组（taskChannelM）

5. DispatchPlatformUpdate()                // 分发到平台特定更新逻辑
   - Midjourney: 预留入口
   - Suno: UpdateSunoTasks()
   - 其他（视频）: UpdateVideoTasks()
`

### 5.3 视频任务更新

**文件**：`service/task_polling.go:203`（`updateVideoSingleTask`）

`
1. 获取渠道信息和适配器

2. 调用 adaptor.FetchTask() 查询上游状态
   → 每个任务间隔 1 秒（避免触发上游限流）

3. 解析上游响应
   → 尝试解析为 New API 格式
   → 失败则调用 adaptor.ParseTaskResult()

4. 更新任务状态
   - SUBMITTED  → progress = "10%"
   - QUEUED     → progress = "20%"
   - IN_PROGRESS → progress = "30%"
   - SUCCESS    → progress = "100%", 记录结果 URL
   - FAILURE    → progress = "100%", 记录失败原因

5. CAS 更新（UpdateWithStatus）
   → 防止并发覆盖

6. 计费处理
   - 成功：settleTaskBillingOnComplete()
   - 失败：RefundTaskQuota()
`

### 5.4 Suno 任务更新

**文件**：`service/task_polling.go:134`（`updateSunoTasks`）

- 批量查询：`adaptor.FetchTask(ids=[...])`
- 按任务 ID 匹配更新
- 失败时退还预扣费

---

## 六、支持的视频平台适配器

| 平台 | 适配器路径 | 特殊计费逻辑 |
|------|-----------|-------------|
| OpenAI/Sora | `relay/channel/task/sora/` | 时长和分辨率倍率 |
| Kling | `relay/channel/task/kling/` | 无附加倍率 |
| 即梦(Jimeng) | `relay/channel/task/jimeng/` | 无附加倍率 |
| Vertex AI | `relay/channel/task/vertex/` | 无附加倍率 |
| Gemini | `relay/channel/task/gemini/` | 无附加倍率 |
| 阿里(Ali) | `relay/channel/task/ali/` | 分辨率和时长倍率 |
| Vidu | `relay/channel/task/vidu/` | 无附加倍率 |
| 豆包(Doubao) | `relay/channel/task/doubao/` | 无附加倍率 |
| 海螺(MiniMax) | `relay/channel/task/hailuo/` | 无附加倍率 |

---

## 七、关键数据结构

### 7.1 Task 模型

**文件**：`model/task.go:44`

| 字段 | 类型 | 说明 |
|------|------|------|
| ID | int64 | 主键 |
| TaskID | string | 对外公开的 task_xxxx ID |
| Platform | TaskPlatform | 平台标识 |
| UserId | int | 用户 ID |
| Group | string | 分组 |
| ChannelId | int | 渠道 ID |
| Quota | int | 预扣费额度 |
| Action | string | 任务类型 |
| Status | TaskStatus | 任务状态 |
| FailReason | string | 失败原因 |
| PrivateData | TaskPrivateData | 内部私有数据（含计费上下文） |
| Data | json.RawMessage | 原始响应数据 |

### 7.2 TaskBillingContext

**文件**：`model/task.go:78`

记录任务提交时的计费参数快照，用于轮询阶段重新计算额度：

| 字段 | 类型 | 说明 |
|------|------|------|
| ModelPrice | float64 | 模型单价 |
| GroupRatio | float64 | 分组倍率 |
| ModelRatio | float64 | 模型倍率 |
| OtherRatios | map[string]float64 | 附加倍率（时长、分辨率等） |
| OriginModelName | string | 模型名称 |
| PerCallBilling | bool | 是否按次计费（跳过轮询阶段差额结算） |

### 7.3 PriceData

价格计算结果结构，包含模型价格、分组倍率、附加倍率、预扣额度等。

---

## 八、计费流程总结图

`
┌───────────────────────────────────────────────────────────────────────┐
│                       视频任务提交流程                                  │
├───────────────────────────────────────────────────────────────────────┤
│                                                                       │
│  HTTP Request                                                          │
│       ↓                                                               │
│  [1] 解析请求 & 验证（ValidateRequestAndSetAction）                    │
│       ↓                                                               │
│  [2] 确定平台 & 适配器（GetTaskPlatform / GetTaskAdaptor）             │
│       ↓                                                               │
│  [3] 价格计算                                                          │
│       ├── ModelPriceHelperPerCall() → 基础价格                         │
│       └── EstimateBilling() → 附加倍率（时长/分辨率）                   │
│       ↓                                                               │
│  [4] 预扣费（PreConsumeBilling）                                       │
│       ├── 选择资金来源（钱包/订阅）                                     │
│       ├── 预扣令牌额度                                                  │
│       └── 预扣资金                                                      │
│       ↓                                                               │
│  [5] 发送请求到上游（DoRequest）                                       │
│       ↓                                                               │
│  [6] 解析响应（DoResponse）                                            │
│       ↓                                                               │
│  [7] 提交后计费调整（AdjustBillingOnSubmit）                           │
│       ↓                                                               │
│  [8] 结算（SettleBilling）                                             │
│       ├── 成功：差额结算（补扣/退还）                                   │
│       └── 失败：退还预扣费（Refund）                                   │
│       ↓                                                               │
│  [9] 记录日志 & 持久化任务                                             │
│                                                                       │
└───────────────────────────────────────────────────────────────────────┘
`

`
┌───────────────────────────────────────────────────────────────────────┐
│                       任务轮询流程（每15秒）                            │
├───────────────────────────────────────────────────────────────────────┤
│                                                                       │
│  sweepTimedOutTasks()        ← 清理超时任务，退还预扣费                │
│       ↓                                                               │
│  GetAllUnFinishSyncTasks()   ← 获取未完成任务                          │
│       ↓                                                               │
│  按平台/渠道分组                                                        │
│       ↓                                                               │
│  updateVideoSingleTask()     ← 逐个查询上游状态                        │
│       ├── adaptor.FetchTask()                                          │
│       ├── adaptor.ParseTaskResult()                                    │
│       ├── CAS 更新数据库                                               │
│       └── 计费处理                                                      │
│           ├── 成功 → settleTaskBillingOnComplete()                      │
│           │    ├── AdjustBillingOnComplete()                            │
│           │    └── RecalculateTaskQuotaByTokens()                       │
│           └── 失败 → RefundTaskQuota()                                  │
│                                                                       │
└───────────────────────────────────────────────────────────────────────┘
`

---

## 九、关键文件索引

| 功能 | 文件路径 |
|------|----------|
| 路由定义 | `router/video-router.go` |
| 控制器（RelayTask） | `controller/relay.go:485` |
| 控制器（RelayTaskFetch） | `controller/relay.go:470` |
| 控制器（VideoProxy） | `controller/video_proxy.go:33` |
| 任务提交核心逻辑 | `relay/relay_task.go:120` |
| 任务查询逻辑 | `relay/relay_task.go:287` |
| 平台适配器注册 | `relay/relay_adaptor.go:135` |
| 价格计算（按次） | `relay/helper/price.go:184` |
| 价格计算（按量） | `relay/helper/price.go:67` |
| 计费入口 | `service/billing.go:19,34` |
| BillingSession | `service/billing_session.go` |
| 资金来源实现 | `service/funding_source.go` |
| 任务计费辅助 | `service/task_billing.go` |
| 违规扣费 | `service/violation_fee.go` |
| 任务轮询主循环 | `service/task_polling.go:91` |
| 任务模型定义 | `model/task.go:44` |
| 中间件-Kling适配 | `middleware/kling_adapter.go` |
| 中间件-Jimeng适配 | `middleware/jimeng_adapter.go` |
| 任务适配器基类 | `relay/channel/task/taskcommon/helpers.go` |
| Sora 适配器 | `relay/channel/task/sora/adaptor.go` |
| Kling 适配器 | `relay/channel/task/kling/adaptor.go` |
| Jimeng 适配器 | `relay/channel/task/jimeng/adaptor.go` |
| Gemini 适配器 | `relay/channel/task/gemini/adaptor.go` |
| Vertex 适配器 | `relay/channel/task/vertex/adaptor.go` |
| Ali 适配器 | `relay/channel/task/ali/adaptor.go` |
| 任务控制器-轮询启动 | `controller/task.go:19` |
