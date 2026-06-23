# Kling 与即梦(Jimeng) 视频生成 — 详细设计文档

本文档基于对 outer/video-router.go 中 /kling/v1 和 /jimeng 路由的代码阅读，详细梳理其设计原理、请求转换机制、适配器实现及完整代码路径。

---

## 目录

1. [整体架构概览](#1-整体架构概览)
2. [路由定义与中间件链](#2-路由定义与中间件链)
3. [请求转换中间件详解](#3-请求转换中间件详解)
4. [TaskAdaptor 适配器接口](#4-taskadaptor-适配器接口)
5. [Kling 适配器实现](#5-kling-适配器实现)
6. [即梦(Jimeng) 适配器实现](#6-即梦jimeng-适配器实现)
7. [任务提交核心流程 (RelayTaskSubmit)](#7-任务提交核心流程)
8. [任务查询流程 (RelayTaskFetch)](#8-任务查询流程)
9. [任务轮询与状态更新](#9-任务轮询与状态更新)
10. [计费模型](#10-计费模型)
11. [视频代理下载 (VideoProxy)](#11-视频代理下载)
12. [关键代码索引表](#12-关键代码索引表)

---

## 1. 整体架构概览

Kling 和即梦的视频生成采用 **统一的 Task 异步任务框架**：

`
客户端请求 (Kling/即梦原生格式)
  │
  ▼
[请求转换中间件] ─── 将原生格式转为统一的 /v1/video/generations 格式
  │
  ▼
[TokenAuth] ─── 认证鉴权
  │
  ▼
[Distribute] ─── 渠道选择与分发
  │
  ▼
[controller.RelayTask] ─── 入口控制器
  │
  ▼
[relay.RelayTaskSubmit] ─── 核心提交逻辑
  │  ├─ GetTaskAdaptor(platform) → 选择 Kling/Jimeng 适配器
  │  ├─ adaptor.ValidateRequestAndSetAction()
  │  ├─ adaptor.EstimateBilling() → 计费估算
  │  ├─ adaptor.BuildRequestBody() → 构建上游请求体
  │  ├─ adaptor.BuildRequestURL() → 构建上游 URL
  │  ├─ adaptor.BuildRequestHeader() → 签名/鉴权
  │  ├─ adaptor.DoRequest() → 发送请求
  │  └─ adaptor.DoResponse() → 解析响应
  │
  ▼
[任务入库] ─── model.Task.Insert()
`

**核心设计理念**：中间件层负责将不同平台的原生 API 格式统一转换为内部标准格式，适配器层负责与各上游平台的通信细节。这种分层使得新增平台只需实现 TaskAdaptor 接口 + 一个请求转换中间件即可接入。

---

## 2. 路由定义与中间件链

**文件**：outer/video-router.go（共 58 行）

### 2.1 Kling 原生路由（第 38-46 行）

`go
klingV1Router := router.Group("/kling/v1")                                    // L38
klingV1Router.Use(middleware.RouteTag("relay"))                               // L39
klingV1Router.Use(middleware.KlingRequestConvert(), middleware.TokenAuth(),
    middleware.Distribute())                                                   // L40-41
{
    klingV1Router.POST("/videos/text2video",   controller.RelayTask)          // L43
    klingV1Router.POST("/videos/image2video",  controller.RelayTask)          // L44
    klingV1Router.GET("/videos/text2video/:task_id",  controller.RelayTaskFetch) // L45
    klingV1Router.GET("/videos/image2video/:task_id", controller.RelayTaskFetch) // L46
}
`

**中间件链执行顺序**：
1. RouteTag("relay") — 标记该请求为 relay 类型，用于日志/统计
2. KlingRequestConvert() — **请求转换**：将 Kling 原生请求体转换为统一格式，并将 URL 重写为 /v1/video/generations
3. TokenAuth() — Token 认证
4. Distribute() — 渠道选择与负载分发

**关键点**：KlingRequestConvert 在 TokenAuth 之前执行，这意味着 URL 重写在认证之前完成，后续中间件看到的是转换后的统一路径。

### 2.2 即梦(Jimeng) 原生路由（第 49-56 行）

`go
jimengOfficialGroup := router.Group("jimeng")                                 // L49
jimengOfficialGroup.Use(middleware.RouteTag("relay"))                          // L50
jimengOfficialGroup.Use(middleware.JimengRequestConvert(), middleware.TokenAuth(),
    middleware.Distribute())                                                   // L51-52
{
    // Maps to: /?Action=CVSync2AsyncSubmitTask&Version=2022-08-31
    //      and /?Action=CVSync2AsyncGetResult&Version=2022-08-31
    jimengOfficialGroup.POST("/", controller.RelayTask)                       // L55
}
`

**即梦路由的特殊性**：即梦官方 API 使用 **单一路由 + Action 查询参数** 区分提交和查询操作：
- ?Action=CVSync2AsyncSubmitTask → 提交视频生成任务
- ?Action=CVSync2AsyncGetResult → 查询任务结果

JimengRequestConvert 中间件会根据 Action 参数将请求分别重写为：
- 提交：POST /v1/video/generations
- 查询：GET /v1/video/generations/{task_id} 并修改 HTTP 方法为 GET

---

## 3. 请求转换中间件详解

### 3.1 Kling 请求转换

**文件**：middleware/kling_adapter.go（共 47 行）

`
输入 (Kling 原生格式):
{
    "model_name": "kling-v1",        // 或 "model" 字段
    "prompt": "一只猫在草地上奔跑",
    "image": "https://example.com/img.jpg",
    ...其他 Kling 特有字段
}

输出 (统一格式):
{
    "model": "kling-v1",
    "prompt": "一只猫在草地上奔跑",
    "metadata": { ...原始请求的所有字段 }
}
`

**执行逻辑**（KlingRequestConvert，第 14-47 行）：

1. **解析原始请求体**（L17-20）：使用 common.UnmarshalBodyReusable 解析，该函数会缓存 body 以供后续重读
2. **提取 model 和 prompt**（L22-28）：兼容 model_name 和 model 两种字段名
3. **构造统一请求**（L30-34）：将 model/prompt 提到顶层，其余字段放入 metadata
4. **重写请求体和路径**（L39-42）：
   - c.Request.Body 替换为新的 JSON body
   - c.Request.URL.Path 重写为 /v1/video/generations
5. **设置 action**（L43-45）：如果请求中没有 image 字段，设置 ction = TaskActionTextGenerate（文生视频）

### 3.2 即梦请求转换

**文件**：middleware/jimeng_adapter.go（共 61 行）

`
输入 (即梦官方格式 — 提交):
POST /jimeng/?Action=CVSync2AsyncSubmitTask&Version=2022-08-31
{
    "req_key": "jimeng_v30",
    "prompt": "一只猫在草地上奔跑",
    "image_urls": ["https://..."],
    ...
}

输入 (即梦官方格式 — 查询):
POST /jimeng/?Action=CVSync2AsyncGetResult&Version=2022-08-31
{
    "task_id": "task_xxxxx",
    ...
}
`

**执行逻辑**（JimengRequestConvert，第 16-61 行）：

1. **验证 Action 参数**（L18-21）：必须提供 Action 查询参数，否则返回 400
2. **解析原始请求体**（L24-28）：解析即梦原生格式
3. **提取 model 和 prompt**（L29-30）：从 eq_key 和 prompt 字段提取
4. **构造统一请求**（L32-37）：同 Kling，顶层 model/prompt + metadata
5. **设置 action**（L46-48）：如果没有 image 字段，设为 TaskActionTextGenerate
6. **路径重写**（L50）：默认重写为 /v1/video/generations
7. **查询分支处理**（L52-60）：当 Action == "CVSync2AsyncGetResult" 时：
   - 验证 	ask_id 必填
   - 路径重写为 /v1/video/generations/{task_id}
   - HTTP 方法改为 GET
   - 在 context 中设置 elay_mode = RelayModeVideoFetchByID

**与 Kling 的关键差异**：
- 即梦用单路由 + Action 参数区分提交/查询，Kling 用不同路径
- 即梦查询时需要改写 HTTP Method（POST → GET）
- 即梦查询需要设置 elay_mode 以走正确的 Fetch 逻辑

---

## 4. TaskAdaptor 适配器接口

**文件**：elay/channel/adapter.go（第 34-78 行）

`go
type TaskAdaptor interface {
    Init(info *relaycommon.RelayInfo)
    ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError

    // 计费
    EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64
    AdjustBillingOnSubmit(info *relaycommon.RelayInfo, taskData []byte) map[string]float64
    AdjustBillingOnComplete(task *model.Task, taskResult *relaycommon.TaskInfo) int

    // 请求构建
    BuildRequestURL(info *relaycommon.RelayInfo) (string, error)
    BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error
    BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error)

    // 请求执行
    DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error)
    DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, err *dto.TaskError)

    GetModelList() []string
    GetChannelName() string

    // 轮询
    FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error)
    ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error)
}
`

**平台路由选择**（elay/relay_adaptor.go，GetTaskAdaptor 函数，第 163-190 行）：

`go
case constant.ChannelTypeKling:   // = 50
    return &kling.TaskAdaptor{}
case constant.ChannelTypeJimeng:  // = 51
    return &taskjimeng.TaskAdaptor{}
`

---

## 5. Kling 适配器实现

**文件**：elay/channel/task/kling/adaptor.go（共 416 行）

### 5.1 核心结构体

`go
type TaskAdaptor struct {                              // L120
    taskcommon.BaseBilling                            // 嵌入默认计费实现
    ChannelType int
    apiKey      string    // 格式: "access_key|access_key|secret_key"
    baseURL     string
}
`

### 5.2 请求构建

**BuildRequestURL**（第 138-147 行）：
- TaskActionGenerate（图生视频）→ {baseURL}/v1/videos/image2video
- TaskActionTextGenerate（文生视频）→ {baseURL}/v1/videos/text2video
- 如果是 New API 中继（sk- 前缀），路径前加 /kling

**BuildRequestHeader**（第 150-160 行）：
- New API 中继：直接用 Bearer {apiKey}
- 原生 Kling：调用 createJWTToken() 生成 JWT Token

**JWT Token 生成**（第 329-349 行）：
- apiKey 格式：ccess_key|access_key|secret_key
- 使用 HS256 签名算法
- Claims：iss=accessKey, exp=now+1800(30分钟), 
bf=now-5
- Header：	yp=JWT

### 5.3 请求体构建

**BuildRequestBody**（第 163-192 行）：
1. 从 context 获取已解析的 TaskSubmitReq
2. 调用 convertToRequestPayload 转换为 Kling 原生格式
3. JSON 序列化

**convertToRequestPayload**（第 295-323 行）：
`go
requestPayload {
    Prompt:         req.Prompt,
    Image:          req.Image,
    Mode:           "std",                    // 默认标准模式
    Duration:       "5",                      // 默认5秒
    AspectRatio:    a.getAspectRatio(size),   // 尺寸映射
    ModelName:      upstreamModelName,        // e.g. "kling-v1"
    Model:          upstreamModelName,        // 兼容字段
    CfgScale:       0.5,                      // 默认引导系数
}
// 通过 UnmarshalMetadata 合并用户传入的额外字段（camera_control, dynamic_masks 等）
`

**尺寸映射**（getAspectRatio，第 325-334 行）：
- 1024x1024, 512x512 → 1:1
- 1280x720, 1920x1080 → 16:9
- 720x1280, 1080x1920 → 9:16

### 5.4 响应解析

**DoResponse**（第 195-222 行）：
1. 读取响应体
2. 解析为 esponsePayload 结构体
3. 从 data.task_id 提取上游任务 ID
4. 返回任务 ID 和原始响应数据

**FetchTask**（第 239-269 行，轮询用）：
- TaskActionGenerate → GET {baseURL}/v1/videos/image2video/{task_id}
- 其他 → GET {baseURL}/v1/videos/text2video/{task_id}
- 鉴权同 BuildRequestHeader（JWT）

**ParseTaskResult**（第 352-387 行）：
- submitted → TaskStatusSubmitted
- processing → TaskStatusInProgress
- succeed → TaskStatusSuccess（提取第一个视频 URL 和 token 消耗）
- ailed → TaskStatusFailure

### 5.5 OpenAI Video 格式转换

**ConvertToOpenAIVideo**（第 389-416 行）：
将 Kling 原生响应转换为 OpenAI Video API 兼容的 OpenAIVideo 格式，包括状态映射、进度、时长和错误信息。

### 5.6 支持的模型

**GetModelList**（第 275-277 行）：
`go
return []string{"kling-v1", "kling-v1-6", "kling-v2-master"}
`

---

## 6. 即梦(Jimeng) 适配器实现

**文件**：elay/channel/task/jimeng/adaptor.go（共 480 行）

### 6.1 核心结构体

`go
type TaskAdaptor struct {                              // L96
    taskcommon.BaseBilling
    ChannelType int
    accessKey   string    // 从 apiKey 拆分
    secretKey   string    // 从 apiKey 拆分
    baseURL     string
}
`

**Init**（第 103-112 行）：
- apiKey 格式：ccess_key|access_key|secret_key
- 与 Kling 不同，即梦在 Init 阶段就拆分 key

### 6.2 请求构建

**BuildRequestURL**（第 121-126 行）：
- New API 中继：{baseURL}/jimeng/?Action=CVSync2AsyncSubmitTask&Version=2022-08-31
- 原生即梦：{baseURL}/?Action=CVSync2AsyncSubmitTask&Version=2022-08-31

**BuildRequestHeader**（第 129-140 行）：
- New API 中继：Bearer {apiKey}
- 原生即梦：**HMAC-SHA256 签名**（火山引擎 V4 签名算法）

### 6.3 签名算法

**signRequest**（第 300-380 行）：
即梦使用火山引擎标准的 HMAC-SHA256 请求签名：

`
1. 生成时间戳: x-date, short-date
2. 计算请求体 SHA256 哈希
3. 构建规范化查询字符串（排序）
4. 构建规范化请求头
5. 拼接规范化请求 (CanonicalRequest)
6. 计算 StringToSign
7. 派生签名密钥: secretKey → date → region → service → "request"
8. 计算 HMAC-SHA256 签名
9. 设置 Authorization 头
`

**签名参数**：
- Region: cn-north-1
- Service: cv
- 签名算法: HMAC-SHA256

### 6.4 请求体构建

**BuildRequestBody**（第 143-225 行）：
即梦的请求体构建比 Kling 复杂得多：

1. **Multipart 表单支持**（L153-193）：支持通过 OpenAI SDK 的 input_reference 字段上传图片
   - 单张图片 → TaskActionGenerate（图生视频）
   - 多张图片 → TaskActionFirstTailGenerate（首尾帧生成）
   - 图片转 base64 上传
   - 单文件限制 4.7MB（MaxFileSize，L74）

2. **URL 图片处理**（L196-225）：如果图片是 HTTP URL，先下载再转 base64

3. **convertToRequestPayload**（L385-437）：
   `go
   requestPayload {
       ReqKey:           upstreamModelName,  // e.g. "jimeng_v30"
       Prompt:           prompt,
       Frames:           121,                // 默认5秒 (24*5+1)
       BinaryDataBase64: [...],              // base64 图片数据
       ImageUrls:        [...],              // HTTP 图片 URL
   }
   `

4. **即梦视频 3.0 ReqKey 自动转换**（L417-437）：
   - jimeng_v30_pro → jimeng_ti2v_v30_pro（固定）
   - 多图：jimeng_v30* → jimeng_i2v_first_tail_v30*（首尾帧）
   - 单图：jimeng_v30* → jimeng_i2v_first_v30*（图生视频）
   - 无图：jimeng_v30* → jimeng_t2v_v30*（文生视频）

### 6.5 响应解析

**DoResponse**（第 228-256 行）：
1. 解析提交响应，提取 data.task_id
2. 如果 code != 0，返回错误

**FetchTask**（第 271-302 行，轮询用）：
- URL：{baseURL}/?Action=CVSync2AsyncGetResult&Version=2022-08-31
- Method: POST
- Body: {"task_id": "xxx"}
- 鉴权：同提交（HMAC 签名）

**ParseTaskResult**（第 439-461 行）：
- code == 10000 → 成功码
- status == "in_queue" → TaskStatusQueued
- status == "done" → TaskStatusSuccess，提取 ideo_url

### 6.6 支持的模型

**GetModelList**（第 288-290 行）：
`go
return []string{"jimeng_v30", "jimeng_v30p", "jimeng_v30_pro"}
`

---

## 7. 任务提交核心流程

**文件**：elay/relay_task.go，RelayTaskSubmit 函数（第 144-258 行）

### 流程步骤

| 步骤 | 行号 | 说明 |
|------|------|------|
| 1 | 145 | info.InitChannelMeta(c) — 初始化渠道元数据 |
| 2 | 148-151 | 确定 platform（从 context 读取，Kling=50, Jimeng=51） |
| 3 | 152-155 | GetTaskAdaptor(platform) — 获取对应适配器 |
| 4 | 156 | daptor.Init(info) — 初始化适配器（设置 apiKey、baseURL） |
| 5 | 157-158 | daptor.ValidateRequestAndSetAction() — 验证请求 |
| 6 | 162-170 | 确定模型名称 + 应用渠道模型映射 |
| 7 | 175-176 | 预生成公开 task ID |
| 8 | 181-185 | 计算基础模型价格（ModelPriceHelperPerCall） |
| 9 | 190-194 | daptor.EstimateBilling() — 估算附加倍率 |
| 10 | 197-202 | 应用 OtherRatios 到基础额度 |
| 11 | 206-209 | **预扣费**（PreConsumeBilling） |
| 12 | 214-216 | daptor.BuildRequestBody() |
| 13 | 220-228 | daptor.DoRequest() — 发送请求到上游 |
| 14 | 238-240 | daptor.DoResponse() — 解析响应 |
| 15 | 244-249 | daptor.AdjustBillingOnSubmit() — 提交后计费调整 |
| 16 | 252-257 | 返回 TaskSubmitResult |

### 控制器层逻辑

**文件**：controller/relay.go，RelayTask 函数（第 486-605 行）

`go
func RelayTask(c *gin.Context) {
    relayInfo, err := relaycommon.GenRelayInfo(c, types.RelayFormatTask, nil, nil)  // L488
    relay.ResolveOriginTask(c, relayInfo)  // L497 — 处理 remix 场景

    for ; retryParam.GetRetry() <= common.RetryTimes; retryParam.IncreaseRetry() {  // L518
        // 选择渠道
        channel, channelErr = getChannel(c, relayInfo, retryParam)  // L531
        // 提交任务
        result, taskErr = relay.RelayTaskSubmit(c, relayInfo)  // L551
        if taskErr == nil { break }
        // 失败重试
        if !shouldRetryTaskRelay(...) { break }
    }

    if taskErr == nil {
        service.SettleBilling(c, relayInfo, result.Quota)  // L576 — 结算
        task := model.InitTask(result.Platform, relayInfo) // L581 — 创建任务
        task.Insert()  // L597 — 入库
    }
}
`

**重试机制**：提交失败后自动重试，最多 common.RetryTimes 次，每次选择不同的渠道。

---

## 8. 任务查询流程

**文件**：elay/relay_task.go，RelayTaskFetch 函数（第 287-307 行）

### 查询路由分发

`go
var fetchRespBuilders = map[int]func(c *gin.Context) (respBody []byte, taskResp *dto.TaskError){  // L281
    relayconstant.RelayModeSunoFetchByID:  sunoFetchByIDRespBodyBuilder,
    relayconstant.RelayModeSunoFetch:      sunoFetchRespBodyBuilder,
    relayconstant.RelayModeVideoFetchByID: videoFetchByIDRespBodyBuilder,  // 即梦查询走这条
}
`

**通用视频查询**（ideoFetchByIDRespBodyBuilder，第 362-416 行）：

1. 从 URL 参数或 context 获取 	ask_id（L363-365）
2. 从数据库查询任务（L368）
3. **实时拉取**：对 Gemini/Vertex 渠道，直接从上游拉最新状态（L382）
4. **OpenAI Video 格式**：调用 daptor.ConvertToOpenAIVideo() 转换（L394-395）
5. **通用 TaskDto 格式**：返回标准任务信息（L407-411）

**即梦查询的特殊路径**：
由于即梦的查询请求在 JimengRequestConvert 中间件里已被重写为 GET /v1/video/generations/{task_id}，且设置了 elay_mode = RelayModeVideoFetchByID，所以会走 ideoFetchByIDRespBodyBuilder 路径，从数据库读取任务状态返回。

**Kling 查询路径**：
Kling 的 GET 请求路径如 /kling/v1/videos/text2video/:task_id，经过 KlingRequestConvert 重写后变为 /v1/video/generations/:task_id，走通用的 RelayTaskFetch → ideoFetchByIDRespBodyBuilder 路径。

---

## 9. 任务轮询与状态更新

**文件**：controller/task_video.go（共 313 行）

### 轮询入口

`go
func UpdateVideoTaskAll(ctx context.Context, platform constant.TaskPlatform,
    taskChannelM map[int][]string, taskM map[string]*model.Task) error  // L24
`

### 轮询流程

1. 按渠道分组处理待轮询任务（L25-28）
2. 获取渠道信息和适配器（L34-47）
3. 逐个任务调用 updateVideoSingleTask（L53-55）

**单任务更新**（updateVideoSingleTask，第 60-259 行）：

1. **获取上游最新状态**：调用 daptor.FetchTask()（L88-93）
2. **解析响应**：尝试 New API 格式 → 适配器原生格式（L100-113）
3. **状态机更新**（L127-233）：

| 上游状态 | 任务状态 | 进度 |
|----------|----------|------|
| submitted | SUBMITTED | 10% |
| queued / in_queue | QUEUED | 20% |
| processing / in_progress | IN_PROGRESS | 30% |
| succeed / done / success | SUCCESS | 100% |
| failed | FAILURE | 100% |

4. **计费结算**（SUCCESS 分支，L145-229）：
   - 计算实际 token 消耗 × 模型倍率 × 分组倍率
   - 与预扣费比较，补扣或退还差额
   - 记录消费日志

5. **任务失败退款**（FAILURE 分支，L235-245）：
   - 退还预扣费额度
   - 防止重复退还（检查 preStatus）

---

## 10. 计费模型

### Kling 和即梦的计费策略

Kling 和即梦都嵌入了 	askcommon.BaseBilling，使用默认的计费实现：

`go
// relay/channel/task/taskcommon/helpers.go
type BaseBilling struct{}

func (BaseBilling) EstimateBilling(...)  { return nil }          // 无附加倍率
func (BaseBilling) AdjustBillingOnSubmit(...) { return nil }     // 无提交后调整
func (BaseBilling) AdjustBillingOnComplete(...) { return 0 }     // 保持预扣费
`

**计费流程**：
1. **基础价格**：通过 ModelPriceHelperPerCall 根据模型名查询配置的价格
2. **预扣费**：提交前按基础价格预扣
3. **完成结算**：轮询到成功后，按实际 token 消耗计算最终费用
   - Kling：从 data.final_unit_deduction 提取 token 消耗（第 378-384 行）
   - 即梦：固定 token 消耗（未从响应中提取）
4. **差额处理**：补扣或退还

---

## 11. 视频代理下载

**文件**：controller/video_proxy.go（共 207 行）

**路由**：GET /v1/videos/:task_id/content

**执行逻辑**（VideoProxy，第 31-157 行）：

1. 根据 task_id 和 user_id 查询任务
2. 验证任务状态为 SUCCESS
3. 根据渠道类型选择视频 URL 获取策略：
   - **Gemini/Vertex**：特殊 API 调用获取视频
   - **OpenAI/Sora**：直接代理上游 /v1/videos/{id}/content
   - **其他（含 Kling/即梦）**：使用任务存储的 ResultURL（L124）
4. SSRF 防护检查
5. 代理转发视频流到客户端

**Kling/即梦的视频 URL 来源**：
- 上游任务成功后，适配器解析出视频 URL
- 存储在 	ask.PrivateData.ResultURL 中
- 视频代理直接使用该 URL 代理下载

---

## 12. 关键代码索引表

### 路由层

| 文件 | 行号 | 函数/结构 | 说明 |
|------|------|-----------|------|
| outer/video-router.go | 38-46 | Kling 路由组 | 定义 4 条 Kling 原生路由 |
| outer/video-router.go | 49-56 | Jimeng 路由组 | 定义 1 条即梦原生路由（Action 分发） |

### 中间件层

| 文件 | 行号 | 函数 | 说明 |
|------|------|------|------|
| middleware/kling_adapter.go | 14-47 | KlingRequestConvert() | Kling 请求转换中间件 |
| middleware/jimeng_adapter.go | 16-61 | JimengRequestConvert() | 即梦请求转换中间件 |

### 控制器层

| 文件 | 行号 | 函数 | 说明 |
|------|------|------|------|
| controller/relay.go | 486-605 | RelayTask() | 任务提交入口 |
| controller/relay.go | 471-484 | RelayTaskFetch() | 任务查询入口 |
| controller/video_proxy.go | 31-157 | VideoProxy() | 视频代理下载 |
| controller/task_video.go | 24-28 | UpdateVideoTaskAll() | 任务轮询入口 |
| controller/task_video.go | 60-259 | updateVideoSingleTask() | 单任务状态更新 |

### Relay 层

| 文件 | 行号 | 函数 | 说明 |
|------|------|------|------|
| elay/relay_task.go | 144-258 | RelayTaskSubmit() | 核心提交逻辑 |
| elay/relay_task.go | 287-307 | RelayTaskFetch() | 核心查询逻辑 |
| elay/relay_task.go | 362-416 | ideoFetchByIDRespBodyBuilder() | 视频查询响应构建 |
| elay/relay_task.go | 38-140 | ResolveOriginTask() | 处理 remix 任务 |
| elay/relay_adaptor.go | 163-190 | GetTaskAdaptor() | 适配器工厂 |

### 适配器层

| 文件 | 行号 | 函数/结构 | 说明 |
|------|------|-----------|------|
| elay/channel/adapter.go | 34-78 | TaskAdaptor 接口 | 适配器接口定义 |
| elay/channel/task/kling/adaptor.go | 120-125 | kling.TaskAdaptor | Kling 适配器结构 |
| elay/channel/task/kling/adaptor.go | 138-147 | BuildRequestURL | 构建上游 URL |
| elay/channel/task/kling/adaptor.go | 150-160 | BuildRequestHeader | JWT 鉴权 |
| elay/channel/task/kling/adaptor.go | 163-192 | BuildRequestBody | 构建请求体 |
| elay/channel/task/kling/adaptor.go | 195-222 | DoResponse | 解析提交响应 |
| elay/channel/task/kling/adaptor.go | 239-269 | FetchTask | 轮询上游状态 |
| elay/channel/task/kling/adaptor.go | 329-349 | createJWTToken | JWT Token 生成 |
| elay/channel/task/kling/adaptor.go | 352-387 | ParseTaskResult | 解析轮询结果 |
| elay/channel/task/jimeng/adaptor.go | 96-101 | jimeng.TaskAdaptor | 即梦适配器结构 |
| elay/channel/task/jimeng/adaptor.go | 121-126 | BuildRequestURL | 构建上游 URL |
| elay/channel/task/jimeng/adaptor.go | 129-140 | BuildRequestHeader | HMAC 签名鉴权 |
| elay/channel/task/jimeng/adaptor.go | 143-225 | BuildRequestBody | 构建请求体（含图片处理） |
| elay/channel/task/jimeng/adaptor.go | 228-256 | DoResponse | 解析提交响应 |
| elay/channel/task/jimeng/adaptor.go | 271-302 | FetchTask | 轮询上游状态 |
| elay/channel/task/jimeng/adaptor.go | 300-380 | signRequest | HMAC-SHA256 签名 |
| elay/channel/task/jimeng/adaptor.go | 385-437 | convertToRequestPayload | 请求转换（含 ReqKey 映射） |
| elay/channel/task/jimeng/adaptor.go | 439-461 | ParseTaskResult | 解析轮询结果 |

### 通用工具

| 文件 | 行号 | 函数 | 说明 |
|------|------|------|------|
| elay/channel/task/taskcommon/helpers.go | 22-33 | UnmarshalMetadata | 元数据 JSON 转换 |
| elay/channel/task/taskcommon/helpers.go | 96-101 | BaseBilling | 默认空计费实现 |
| elay/channel/api_request.go | 535-557 | DoTaskApiRequest | 通用 HTTP 请求执行 |
| elay/common/relay_utils.go | 198-222 | ValidateBasicTaskRequest | 通用任务请求验证 |

### 常量定义

| 文件 | 行号 | 常量 | 值 |
|------|------|------|------|
| constant/channel.go | 50 | ChannelTypeKling | 50 |
| constant/channel.go | 51 | ChannelTypeJimeng | 51 |

---

## 附录：Kling 与即梦实现对比

| 维度 | Kling | 即梦(Jimeng) |
|------|-------|-------------|
| **渠道类型** | 50 | 51 |
| **认证方式** | JWT (HS256) | HMAC-SHA256 (火山引擎 V4) |
| **apiKey 格式** | access_key|secret_key|access_key|secret_key | access_key|secret_key|access_key|secret_key |
| **请求格式** | Kling 原生 JSON | 火山引擎 CV API JSON |
| **提交 URL** | /v1/videos/text2video 或 image2video | /?Action=CVSync2AsyncSubmitTask |
| **查询 URL** | GET /v1/videos/{type}/{task_id} | POST /?Action=CVSync2AsyncGetResult |
| **查询方式** | GET 请求，路径区分类型 | POST 请求 + task_id body |
| **图片输入** | 单张 image URL | base64 / URL，支持多张 |
| **视频时长** | 字符串 "5" / "10" | Frames 整数 (121/241) |
| **尺寸映射** | 尺寸→宽高比(1:1, 16:9, 9:16) | 直接传递 aspect_ratio |
| **模型名映射** | 直接使用 | ReqKey 自动转换(v30系列) |
| **成功标识** | data.task_status == "succeed" | data.status == "done" |
| **token 消耗** | data.final_unit_deduction | 未提取(固定) |
| **支持的模型** | kling-v1, kling-v1-6, kling-v2-master | jimeng_v30, jimeng_v30p, jimeng_v30_pro |
| **文件大小限制** | 无特殊限制 | 4.7MB (即梦平台限制) |
| **New API 中继** | 路径前缀 /kling | 路径前缀 /jimeng |
