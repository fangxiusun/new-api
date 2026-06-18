# Ali 渠道代码深度分析与新模型接入指南

> 生成时间: 2026-06-17
> 基于代码库最新状态分析

---

## 目录

1. [Ali 渠道架构总览](#1-ali-渠道架构总览)
2. [现有代码结构详解](#2-现有代码结构详解)
3. [文本模型接入方法](#3-文本模型接入方法)
4. [图片模型接入方法](#4-图片模型接入方法)
5. [音频模型接入方法](#5-音频模型接入方法)
6. [视频模型接入方法](#6-视频模型接入方法)
7. [Ali 自定义 API 可行性分析](#7-ali-自定义-api-可行性分析)
8. [代码修改清单汇总](#8-代码修改清单汇总)

---

## 1. Ali 渠道架构总览

Ali 渠道在本系统中分为 **两套独立的 Adaptor**：

| 层级 | 路径 | 职责 | 接口 |
|------|------|------|------|
| **标准 Relay Adaptor** | `relay/channel/ali/` | 处理文本、图片、Embedding、Rerank 等同步/半同步请求 | `channel.Adaptor` |
| **Task Adaptor** | `relay/channel/task/ali/` | 处理视频生成等异步任务请求 | `channel.TaskAdaptor` |

### 请求分发流程

```
用户请求 → router/ → middleware/Distribute() → controller/Relay 或 controller/RelayTask
                                                      │
                                          ┌───────────┴───────────┐
                                          ▼                       ▼
                                   relay/Relay()           relay/RelayTaskSubmit()
                                          │                       │
                                   GetAdaptor(17)         GetTaskAdaptor("17")
                                          │                       │
                                          ▼                       ▼
                                   ali.Adaptor{}          task/ali.TaskAdaptor{}
```

**关键映射关系**（`relay/relay_adaptor.go`）：
- `constant.ChannelTypeAli = 17` → `ali.Adaptor{}` （标准 relay）
- `constant.TaskPlatform("17")` → `taskali.TaskAdaptor{}` （任务 relay）

---

## 2. 现有代码结构详解

### 2.1 标准 Relay Adaptor (`relay/channel/ali/`)

| 文件 | 职责 | 关键函数/类型 |
|------|------|---------------|
| `adaptor.go` | 核心适配器：路由分发、请求转换、响应处理 | `Adaptor.GetRequestURL()`, `Adaptor.ConvertOpenAIRequest()`, `Adaptor.ConvertImageRequest()`, `Adaptor.DoResponse()` |
| `constants.go` | 模型列表和渠道名 | `ModelList`, `ChannelName` |
| `dto.go` | 阿里原生请求/响应 DTO | `AliChatRequest`, `AliImageRequest`, `AliResponse`, `AliUsage`, `AliEmbeddingRequest`, `AliRerankRequest` |
| `text.go` | 文本请求转换 | `requestOpenAI2Ali()` — 修正 TopP 范围 |
| `image.go` | 图片生成/编辑请求转换与响应处理 | `oaiImage2AliImageRequest()`, `oaiFormEdit2AliImageEdit()`, `aliImageHandler()`, `asyncTaskWait()` |
| `image_wan.go` | Wan 模型专用图片编辑 | `oaiFormEdit2WanxImageEdit()`, `isOldWanModel()`, `isWanModel()` |
| `rerank.go` | Rerank 请求转换与响应处理 | `ConvertRerankRequest()`, `RerankHandler()` |

### 2.2 Task Adaptor (`relay/channel/task/ali/`)

| 文件 | 职责 | 关键函数/类型 |
|------|------|---------------|
| `adaptor.go` | 视频任务全流程：验证→构建→提交→轮询→计费 | `TaskAdaptor.ValidateRequestAndSetAction()`, `TaskAdaptor.BuildRequestBody()`, `TaskAdaptor.DoResponse()`, `TaskAdaptor.FetchTask()`, `TaskAdaptor.ParseTaskResult()`, `TaskAdaptor.EstimateBilling()`, `TaskAdaptor.ConvertToOpenAIVideo()` |
| `constants.go` | 视频模型列表 | `ModelList` (wan2.5-i2v-preview, wan2.2-i2v-flash 等) |

### 2.3 现有支持的模型

**标准 Relay（文本/图片/Embedding/Rerank）**（`relay/channel/ali/constants.go`）：
```
qwen-turbo, qwen-plus, qwen-max, qwen-max-longcontext,
qwq-32b, qwen3-235b-a22b, text-embedding-v1, gte-rerank-v2
```

**Task Relay（视频）**（`relay/channel/task/ali/constants.go`）：
```
wan2.5-i2v-preview, wan2.2-i2v-flash, wan2.2-i2v-plus,
wanx2.1-i2v-plus, wanx2.1-i2v-turbo
```

**图片模型**（硬编码在 `adaptor.go` 的 `syncModels` 变量中 + `model_setting` 配置）：
```
z-image, qwen-image, wan2.6
```

---

## 3. 文本模型接入方法

### 3.1 现状分析

文本模型通过**标准 OpenAI 兼容模式**接入，阿里百炼的接口为：
```
POST {baseURL}/compatible-mode/v1/chat/completions
```

现有的 `requestOpenAI2Ali()` 函数（`relay/channel/ali/text.go`）只做了一个转换：将 TopP 限制在 `(0.001, 0.999)` 范围内。其他字段直接透传。

### 3.2 需要新增的文本模型

根据 `docs/reference/pricing/ali.csv`：

| 模型名 | 版本名 | 类型 | 特殊要求 |
|--------|--------|------|----------|
| `qwen3.7-max` | qwen3.7-max-2026-05-20 等 | 文本 | 无 |
| `qwen3.7-plus` | qwen3.7-plus-2026-05-26 | 文本（分段计费） | Token 分段定价 |
| `qwen3.6-plus` | qwen3.6-plus-2026-04-02 | 文本（分段计费） | Token 分段定价 |
| `qwen3.6-flash` | qwen3.6-flash-2026-04-16 | 文本（分段计费） | Token 分段定价 |
| `qwen3.5-flash` | qwen3.5-flash-2026-02-23 | 文本（分段计费） | Token 分段定价 |
| `qwen-flash-character` | — | 文本 | 无 |
| `qwen-plus-character` | — | 文本 | 无 |
| `qwen3.5-omni-flash` | qwen3.5-omni-flash-2026-03-15 | 全模态（音频+文本/图片/视频） | 双计费模式 |
| `qwen3.5-omni-plus` | qwen3.5-omni-plus-2026-03-15 | 全模态 | 双计费模式 |
| `qwen3.5-omni-plus-realtime` | — | 全模态实时 | 双计费模式 |
| `qwen3.5-omni-flash-realtime` | — | 全模态实时 | 双计费模式 |

### 3.3 接入步骤

#### 步骤 1：更新 `relay/channel/ali/constants.go`

在 `ModelList` 中添加新模型名：

```go
var ModelList = []string{
    // 已有
    "qwen-turbo", "qwen-plus", "qwen-max", "qwen-max-longcontext",
    "qwq-32b", "qwen3-235b-a22b", "text-embedding-v1", "gte-rerank-v2",
    // 新增文本模型
    "qwen3.7-max",
    "qwen3.7-plus",
    "qwen3.6-plus",
    "qwen3.6-flash",
    "qwen3.5-flash",
    "qwen-flash-character",
    "qwen-plus-character",
    // 全模态模型（文本模式下走 chat/completions）
    "qwen3.5-omni-flash",
    "qwen3.5-omni-plus",
    "qwen3.5-omni-plus-realtime",
    "qwen3.5-omni-flash-realtime",
}
```

#### 步骤 2：计费配置

- **普通文本模型**：在管理后台的 **模型比率（ModelRatio）** 中配置输入/输出价格比
- **分段计费模型**（qwen3.7-plus 等）：需要使用系统的 **阶梯计费表达式**（参见 `pkg/billingexpr/expr.md`），根据 Token 数量分段定价
- **全模态模型**：需根据输入类型（音频 vs 文本/图片/视频）使用不同的价格，在前端配置两组价格或使用表达式

> **无需修改代码**：文本模型走的是标准 OpenAI 兼容格式，系统会自动将请求转发到 `/compatible-mode/v1/chat/completions`。只需在后台添加模型名和价格配置即可。

#### 步骤 3：Anthropic Messages 兼容

如果需要通过 Claude 格式调用这些模型（`supportsAliAnthropicMessages`），需要在环境变量 `ALI_ANTHROPIC_MESSAGES_MODELS` 中添加模型名模式，或修改默认值。

---

## 4. 图片模型接入方法

### 4.1 现状分析

图片模型分两类处理（`relay/channel/ali/adaptor.go` 的 `GetRequestURL`）：

| 类型 | 判断逻辑 | 上游 API |
|------|----------|----------|
| **同步图片模型** | `isSyncImageModel()` 匹配 | `POST /api/v1/services/aigc/multimodal-generation/generation` |
| **异步图片模型** | 默认 | `POST /api/v1/services/aigc/text2image/image-synthesis`（文生图）或 `/api/v1/services/aigc/image-generation/generation`（wan2.7+）或 `/api/v1/services/aigc/image2image/image-synthesis`（旧 wan 图生图） |

同步/异步判断来源：`setting/model_setting/qwen.go` 的 `IsSyncImageModel()` 函数，通过配置文件中的 `SyncImageModels` 列表匹配。

### 4.2 需要新增的图片模型

| 模型名 | 类型 | 计费方式 | 特殊说明 |
|--------|------|----------|----------|
| `wan2.7-image-pro` | 异步/同步（待确认） | 按张 (0.5元/张) | 新一代 wan 图片模型 |
| `wan2.7-image` | 异步/同步（待确认） | 按张 (0.2元/张) | 新一代 wan 图片模型 |
| `qwen-image-2.0-pro` | 同步（推测） | 按张 (0.5元/张) | qwen 系列图片模型 |
| `qwen-image-2.0` | 同步（推测） | 按张 (0.2元/张) | qwen 系列图片模型 |
| `qwen-image-edit-max` | 同步（推测） | 按张 (0.5元/张) | 图片编辑 |
| `qwen-image-edit-plus` | 同步（推测） | 按张 (0.2元/张) | 图片编辑 |
| `qwen-image-edit` | 同步（推测） | 按张 (0.3元/张) | 图片编辑 |
| `qwen-mt-image` | 同步（推测） | 按张 (0.003元/张) | 翻译图片模型（极低价） |

### 4.3 接入步骤

#### 步骤 1：确定同步/异步模式

需要查阅阿里百炼 API 文档确认每个模型的 API 调用方式：

- 如果是**同步模型**（直接返回结果）：加入 `model_setting` 的 `SyncImageModels` 列表
- 如果是**异步模型**（返回 task_id，需轮询）：走默认异步流程

#### 步骤 2：更新 `relay/channel/ali/adaptor.go`

**如果新模型使用现有的 API 端点**（如 `text2image/image-synthesis` 或 `multimodal-generation/generation`）：

只需更新 `syncModels` 列表（如果需要同步模式）：
```go
var syncModels = []string{
    "z-image", "qwen-image", "wan2.6",
    "wan2.7-image", "wan2.7-image-pro",  // 新增
    "qwen-image-2.0", "qwen-image-2.0-pro",  // 新增
}
```

**如果新模型使用了不同的 API 端点**：

需要修改 `GetRequestURL()` 中的 `RelayModeImagesGenerations` 和 `RelayModeImagesEdits` 分支，添加新的 URL 构造逻辑。

#### 步骤 3：更新 `isWanModel()` 和 `isOldWanModel()`

`relay/channel/ali/image_wan.go` 中的判断逻辑需要更新，确保 wan2.7 系列被正确识别：

```go
func isOldWanModel(modelName string) bool {
    return strings.Contains(modelName, "wan") &&
        !lo.SomeBy([]string{"wan2.6", "wan2.7"}, func(v string) bool { return strings.Contains(modelName, v) })
}

func isWanModel(modelName string) bool {
    return strings.Contains(modelName, "wan")
}
```

> `isOldWanModel` 已经排除了 wan2.7，但 `isWanModel` 只检查 "wan" 前缀，wan2.7-image 会被匹配。需确认 wan2.7-image 的图片编辑是否走 wan 的 API 路径。

#### 步骤 4：DTO 适配

如果新模型的请求/响应格式与现有的 `AliImageRequest` 不同，需要在 `relay/channel/ali/dto.go` 中添加新的 DTO 结构。

#### 步骤 5：计费配置

- **按张计费**：在管理后台配置模型的 `ModelRatio`，系统会通过 `info.PriceData.AddOtherRatio("n", count)` 自动乘以图片数量
- 对于极低价模型如 `qwen-mt-image`（0.003元/张），需要注意价格精度

---

## 5. 音频模型接入方法

### 5.1 现状分析

当前 Ali 渠道的 `ConvertAudioRequest` 方法返回 `errors.New("not implemented")`（`relay/channel/ali/adaptor.go`），即**音频功能完全未实现**。

系统的音频处理通过以下路由进入：
```
POST /v1/audio/transcriptions  → RelayFormatOpenAIAudio → RelayModeAudioTranscription
POST /v1/audio/translations    → RelayFormatOpenAIAudio → RelayModeAudioTranslation
POST /v1/audio/speech          → RelayFormatOpenAIAudio → RelayModeSpeech
```

### 5.2 Ali 音频 API 分析

根据 `docs/reference/pricing/ali.csv`，omni 模型（qwen3.5-omni-flash/plus）支持音频输入和音频输出：

- **输入**：音频 → 输出：文本+音频（输出文本不计费）
- **输入**：文本/图片/视频 → 输出：文本

这不同于标准的 Whisper/TTS API，而是通过 DashScope 的 chat completions 接口实现的全模态交互。

### 5.3 接入方案

#### 方案 A：通过 Chat Completions 接入（推荐）

Omni 模型的音频能力本质上是通过 chat completions 接口实现的，不是独立的 audio API。用户可以通过标准的 `/v1/chat/completions` 调用，传入音频内容（base64 或 URL）。

**需要的改动**：
1. 在 `ModelList` 中添加 omni 模型（已在文本模型部分完成）
2. 在后台配置特殊的音频输入/输出价格比率
3. 无需修改 `ConvertAudioRequest`

#### 方案 B：实现独立的 Audio API（如果需要兼容 OpenAI 标准格式）

如果要支持标准的 `/v1/audio/speech` 和 `/v1/audio/transcriptions` 接口，需要：

1. **在 `relay/channel/ali/adaptor.go` 中实现 `ConvertAudioRequest`**：
   - 参考阿里 DashScope 的 Paraformer（语音识别）和 CosyVoice（语音合成）API
   - 将 OpenAI 标准格式转换为阿里原生格式

2. **添加音频相关的 DTO**：
   - TTS 请求/响应结构
   - ASR 请求/响应结构

3. **更新 `GetRequestURL` 添加音频路由**：
   ```go
   case constant.RelayModeSpeech:
       fullRequestURL = fmt.Sprintf("%s/api/v1/services/aigc/text2speech/speech-synthesis", info.ChannelBaseUrl)
   case constant.RelayModeAudioTranscription:
       fullRequestURL = fmt.Sprintf("%s/api/v1/services/audio/asr/transcription", info.ChannelBaseUrl)
   ```

4. **更新 `DoResponse` 添加音频响应处理**

#### 建议

对于 omni 模型，采用**方案 A** 即可满足需求（通过 chat completions 调用）。如果需要独立的 TTS/ASR 服务（如 CosyVoice、Paraformer），再实现方案 B。

---

## 6. 视频模型接入方法

### 6.1 现状分析

视频模型通过 **Task Adaptor**（`relay/channel/task/ali/`）处理，流程为：

```
POST /v1/video/generations → RelayTask → task/ali.TaskAdaptor
                                          ├── ValidateRequestAndSetAction()
                                          ├── EstimateBilling()  → 计算预扣费
                                          ├── BuildRequestBody() → 构建阿里视频请求
                                          ├── DoRequest()        → 提交到阿里
                                          ├── DoResponse()       → 解析 task_id
                                          └── [后台轮询]
                                               ├── FetchTask()         → 查询任务状态
                                               ├── ParseTaskResult()   → 解析结果
                                               └── AdjustBillingOnComplete() → 结算
```

### 6.2 需要新增的视频模型

| 模型名 | 类型 | 计费方式 | API 端点 |
|--------|------|----------|----------|
| `wan2.7-t2v` | 文生视频 | 按秒 720P: 0.6元/s, 1080P: 1元/s | 待确认 |
| `wan2.7-r2v` | 参考生视频 | 按秒 720P: 0.6元/s, 1080P: 1元/s | 待确认 |
| `wan2.7-i2v` | 图生视频 | 按秒 720P: 0.6元/s, 1080P: 1元/s | 待确认 |
| `wan2.7-videoedit` | 视频编辑 | 按秒 720P: 0.6元/s, 1080P: 1元/s | 待确认 |
| `happyhorse-1.0-t2v` | 文生视频（小马） | 按秒 720P: 0.9元/s, 1080P: 1.6元/s | 待确认 |
| `happyhorse-1.0-r2v` | 参考生视频（小马） | 按秒 | 待确认 |
| `happyhorse-1.0-i2v` | 图生视频（小马） | 按秒 | 待确认 |
| `happyhorse-1.0-video-edit` | 视频编辑（小马） | 按秒 | 待确认 |

### 6.3 接入步骤

#### 步骤 1：更新 `relay/channel/task/ali/constants.go`

```go
var ModelList = []string{
    // 已有
    "wan2.5-i2v-preview", "wan2.2-i2v-flash", "wan2.2-i2v-plus",
    "wanx2.1-i2v-plus", "wanx2.1-i2v-turbo",
    // 新增 wan2.7 系列
    "wan2.7-t2v",
    "wan2.7-r2v",
    "wan2.7-i2v",
    "wan2.7-videoedit",
    // 新增小马系列
    "happyhorse-1.0-t2v",
    "happyhorse-1.0-r2v",
    "happyhorse-1.0-i2v",
    "happyhorse-1.0-video-edit",
}
```

#### 步骤 2：更新 `convertToAliRequest()`

`relay/channel/task/ali/adaptor.go` 的 `convertToAliRequest()` 函数需要处理新模型的特殊参数：

- `wan2.7-t2v`：文生视频，使用 `prompt` + `size`（尺寸格式如 "1920*1080"）
- `wan2.7-i2v`：图生视频，使用 `prompt` + `img_url`
- `wan2.7-r2v`：参考生视频，需要 `img_url`（参考图）
- `wan2.7-videoedit`：视频编辑，需要视频 URL
- `happyhorse-*`：小马系列，API 格式可能与 wan 不同

关键修改点：
```go
func (a *TaskAdaptor) convertToAliRequest(info *relaycommon.RelayInfo, req relaycommon.TaskSubmitReq) (*AliVideoRequest, error) {
    // 已有逻辑...

    // 新增：wan2.7 文生视频使用 Size 字段（格式如 "1920*1080"）
    // 新增：wan2.7 参考生视频需要处理参考图
    // 新增：小马系列模型可能需要不同的 API 路径

    // 需要确认：wan2.7 系列是否使用相同的 /api/v1/services/aigc/video-generation/generation 端点
}
```

#### 步骤 3：更新 `BuildRequestURL()`

如果新模型使用不同的 API 端点：

```go
func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
    model := info.UpstreamModelName

    // 已有逻辑
    if strings.Contains(model, "t2v") {
        return fmt.Sprintf("%s/api/v1/services/aigc/video-generation/generation", a.baseURL), nil
    }

    // 新增：wan2.7 系列
    if strings.HasPrefix(model, "wan2.7") {
        // 需要确认 wan2.7 的 API 端点
        return fmt.Sprintf("%s/api/v1/services/aigc/video-generation/generation", a.baseURL), nil
    }

    // 新增：小马系列
    if strings.HasPrefix(model, "happyhorse") {
        // 需要确认 happyhorse 的 API 端点
        return fmt.Sprintf("%s/api/v1/services/aigc/video-generation/generation", a.baseURL), nil
    }
}
```

#### 步骤 4：更新 `EstimateBilling()` 和 `ProcessAliOtherRatios()`

`ProcessAliOtherRatios()` 中的 `aliRatios` 映射表需要添加新模型的分辨率价格比：

```go
var aliRatios = map[string]map[string]float64{
    // 已有...

    // 新增 wan2.7 系列
    "wan2.7-t2v": {
        "720P":  1,
        "1080P": 1 / 0.6,  // 1080P 价格 / 720P 价格
    },
    "wan2.7-i2v": {
        "720P":  1,
        "1080P": 1 / 0.6,
    },
    "wan2.7-r2v": {
        "720P":  1,
        "1080P": 1 / 0.6,
    },
    "wan2.7-videoedit": {
        "720P":  1,
        "1080P": 1 / 0.6,
    },
    // 新增小马系列
    "happyhorse-1.0-t2v": {
        "720P":  1,
        "1080P": 1.6 / 0.9,
    },
    // ... 其他小马模型
}
```

> 注意：小马系列 (happyhorse) 在 CSV 中标注的是"6折"，与 wan 系列的"7折"不同。这个折扣在**系统外部**处理（管理员手动计算后配置到系统中的 ModelRatio），不影响代码逻辑。

#### 步骤 5：确认 API 兼容性

需要确认以下问题（可能需要查阅阿里百炼文档）：

1. **wan2.7 系列的 API 端点**是否与现有 wan2.2/wan2.5 一致？
2. **happyhorse 系列的 API 端点**是什么？是否与 wan 系列通用？
3. **wan2.7-videoedit** 的请求格式是什么？是否需要视频 URL 输入？
4. **wan2.7-r2v**（参考生视频）的输入格式是什么？

---

## 7. Ali 自定义 API 可行性分析

### 7.1 什么是"自定义 API"

在本项目中，"自定义 API"指**直接暴露厂商原生格式的 API 端点**，而非通过 OpenAI 兼容格式中转。已有先例：

| 渠道 | 自定义路由 | 实现方式 |
|------|-----------|----------|
| **Kling** | `/kling/v1/videos/text2video`, `/kling/v1/videos/image2video` | `middleware.KlingRequestConvert()` 将 Kling 原生格式转换为统一 Task 格式 |
| **Jimeng** | `jimeng/` (Action=CVSync2AsyncSubmitTask) | `middleware.JimengRequestConvert()` 将即梦原生格式转换为统一 Task 格式 |

### 7.2 实现模式分析

Kling 和 Jimeng 的自定义 API 都遵循**同一个模式**：

```
1. 注册原生路由 (router/video-router.go)
2. 编写转换中间件 (middleware/xxx_adapter.go)
   - 解析原生请求格式
   - 提取 model、prompt 等关键字段
   - 构建统一的 { model, prompt, metadata } 格式
   - 重写 request body 和 URL path
3. 路由到标准的 controller.RelayTask 处理
4. Task Adaptor 中的 BuildRequestBody() 从 metadata 恢复原生参数
```

核心代码流程（以 Kling 为例）：

```go
// middleware/kling_adapter.go
func KlingRequestConvert() func(c *gin.Context) {
    return func(c *gin.Context) {
        // 1. 解析原生请求
        var originalReq map[string]interface{}
        common.UnmarshalBodyReusable(c, &originalReq)

        // 2. 构建统一格式
        unifiedReq := map[string]interface{}{
            "model":    model,
            "prompt":   prompt,
            "metadata": originalReq,  // 原始参数保留在 metadata 中
        }

        // 3. 重写 body 和 path
        c.Request.Body = io.NopCloser(bytes.NewBuffer(jsonData))
        c.Request.URL.Path = "/v1/video/generations"

        c.Next()
    }
}
```

```go
// relay/channel/task/kling/adaptor.go
func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
    req, _ := relaycommon.GetTaskRequest(c)
    r := requestPayload{...}

    // 从 metadata 恢复原生参数
    taskcommon.UnmarshalMetadata(req.Metadata, &r)
    // ...
}
```

### 7.3 Ali 自定义 API 的可行性

**完全可行**，且有明确的参考实现。

#### 方案：Ali Video 原生 API 中间件

##### 路由设计

```go
// router/video-router.go
aliOfficialGroup := router.Group("/ali/v1")
aliOfficialGroup.Use(middleware.RouteTag("relay"))
aliOfficialGroup.Use(middleware.AliVideoRequestConvert(), middleware.TokenAuth(), middleware.Distribute())
{
    // 视频生成
    aliOfficialGroup.POST("/services/aigc/video-generation/generation", controller.RelayTask)
    aliOfficialGroup.POST("/services/aigc/text2video/video-generation", controller.RelayTask)

    // 任务查询
    aliOfficialGroup.GET("/tasks/:task_id", controller.RelayTaskFetch)
}
```

##### 中间件设计

```go
// middleware/ali_video_adapter.go
func AliVideoRequestConvert() func(c *gin.Context) {
    return func(c *gin.Context) {
        var originalReq map[string]interface{}
        if err := common.UnmarshalBodyReusable(c, &originalReq); err != nil {
            c.Next()
            return
        }

        // 从阿里原生请求中提取 model 和 prompt
        model, _ := originalReq["model"].(string)
        input, _ := originalReq["input"].(map[string]interface{})
        prompt, _ := input["prompt"].(string)

        unifiedReq := map[string]interface{}{
            "model":    model,
            "prompt":   prompt,
            "metadata": originalReq,
        }

        jsonData, _ := json.Marshal(unifiedReq)
        c.Request.Body = io.NopCloser(bytes.NewBuffer(jsonData))
        c.Request.URL.Path = "/v1/video/generations"
        c.Set(common.KeyRequestBody, jsonData)

        // 处理任务查询
        if c.Request.Method == http.MethodGet {
            taskID := c.Param("task_id")
            c.Request.URL.Path = "/v1/video/generations/" + taskID
            c.Set("task_id", taskID)
            c.Set("relay_mode", relayconstant.RelayModeVideoFetchByID)
        }

        c.Next()
    }
}
```

##### TaskAdaptor 中恢复原生参数

在 `task/ali/adaptor.go` 的 `convertToAliRequest()` 中，需要从 metadata 恢复阿里原生参数：

```go
func (a *TaskAdaptor) convertToAliRequest(info *relaycommon.RelayInfo, req relaycommon.TaskSubmitReq) (*AliVideoRequest, error) {
    aliReq := &AliVideoRequest{
        Model: info.UpstreamModelName,
        Input: AliVideoInput{
            Prompt: req.Prompt,
        },
    }

    // 从 metadata 恢复阿里原生参数
    if err := taskcommon.UnmarshalMetadata(req.Metadata, aliReq); err != nil {
        return nil, err
    }

    return aliReq, nil
}
```

#### 方案：Ali Image 原生 API 中间件

同理，图片生成也可以实现原生 API 中间件：

```go
// router/video-router.go 或新建 router/ali-router.go
aliImageGroup := router.Group("/ali/v1")
aliImageGroup.Use(middleware.AliImageRequestConvert(), middleware.TokenAuth(), middleware.Distribute())
{
    aliImageGroup.POST("/services/aigc/text2image/image-synthesis", controller.Relay)
    aliImageGroup.POST("/services/aigc/multimodal-generation/generation", controller.Relay)
    aliImageGroup.POST("/services/aigc/image-generation/generation", controller.Relay)
}
```

### 7.4 自定义 API 的价值

| 优势 | 说明 |
|------|------|
| **降低迁移成本** | 已有阿里 SDK 的用户可以直接切换，无需改代码 |
| **支持高级参数** | 阿里原生的 `template`、`audio`、`enable_sequential` 等参数可以通过 metadata 原样传递 |
| **保持兼容性** | 系统内部仍然走统一的 Task 流程，计费和轮询逻辑不变 |

| 挑战 | 说明 |
|------|------|
| **路由维护** | 需要跟踪阿里 API 的路由变更 |
| **文档同步** | 需要维护两套 API 文档（OpenAI 兼容 + 阿里原生） |
| **签名认证** | 如果要支持阿里的 HMAC 签名认证（类似 Jimeng），需要额外实现签名逻辑 |

### 7.5 与现有 Jimeng 实现的对比

Jimeng 使用 HMAC-SHA256 签名认证（`relay/channel/task/jimeng/adaptor.go` 的 `signRequest()`），而 Ali 视频使用标准 Bearer Token 认证，**认证更简单**。

Jimeng 的路由是 `POST /jimeng/`，通过 query 参数 `Action` 区分提交和查询。Ali 可以设计为 RESTful 风格，更直观。

---

## 8. 代码修改清单汇总

### 8.1 必须修改的文件

| 文件 | 修改内容 | 涉及模型类型 |
|------|----------|-------------|
| `relay/channel/ali/constants.go` | `ModelList` 添加新文本模型 | 文本 |
| `relay/channel/task/ali/constants.go` | `ModelList` 添加新视频模型 | 视频 |
| `relay/channel/task/ali/adaptor.go` | `convertToAliRequest()` 处理新模型参数 | 视频 |
| `relay/channel/task/ali/adaptor.go` | `BuildRequestURL()` 处理新模型端点 | 视频 |
| `relay/channel/task/ali/adaptor.go` | `ProcessAliOtherRatios()` 添加新模型分辨率比 | 视频 |
| 管理后台配置 | 添加新模型的 ModelRatio | 所有 |

### 8.2 可能需要修改的文件

| 文件 | 修改内容 | 前提条件 |
|------|----------|----------|
| `relay/channel/ali/adaptor.go` | `syncModels` 列表更新 | 新图片模型使用同步模式 |
| `relay/channel/ali/adaptor.go` | `GetRequestURL()` 新增 API 端点 | 新模型使用不同端点 |
| `relay/channel/ali/image_wan.go` | `isOldWanModel()` 更新 | wan2.7 使用不同 API |
| `relay/channel/ali/dto.go` | 新增 DTO 结构 | 新模型请求格式不同 |
| `setting/model_setting/qwen.go` | `SyncImageModels` 配置 | 新同步图片模型 |
| `relay/channel/ali/adaptor.go` | `ConvertAudioRequest()` 实现 | 需要独立 TTS/ASR |

### 8.3 自定义 API 所需的新文件

| 文件 | 内容 |
|------|------|
| `middleware/ali_video_adapter.go` | Ali 视频原生 API 转换中间件 |
| `middleware/ali_image_adapter.go` | Ali 图片原生 API 转换中间件（可选） |
| `router/ali-router.go` 或修改 `router/video-router.go` | 注册 Ali 原生 API 路由 |

### 8.4 待确认事项

以下是需要进一步确认的技术细节：

1. **wan2.7 系列的 API 端点**：是否与 wan2.2/wan2.5 使用相同的 DashScope 端点？
2. **happyhorse 系列的 API 端点**：是独立端点还是复用 wan 的端点？
3. **wan2.7-r2v 的请求格式**：参考生视频的输入参数是什么？
4. **wan2.7-videoedit 的请求格式**：视频编辑需要哪些输入？
5. **新图片模型的同步/异步模式**：wan2.7-image、qwen-image-2.0 等是同步返回还是异步轮询？
6. **Ali 视频 API 是否有 HMAC 签名需求**：当前 task/ali 使用 Bearer Token，但某些 API 可能需要签名
7. **Omni 模型的音频计费**：CSV 中音频和文本/图片/视频的单价差异很大，如何在系统中区分输入类型进行计费？

---

## 附录：关键代码位置速查

| 功能 | 文件路径 | 行号/函数 |
|------|----------|-----------|
| Adaptor 注册 | `relay/relay_adaptor.go` | `GetAdaptor()` / `GetTaskAdaptor()` |
| 渠道类型常量 | `constant/channel.go` | `ChannelTypeAli = 17` |
| 标准 Adaptor 接口 | `relay/channel/adapter.go` | `Adaptor` interface |
| Task Adaptor 接口 | `relay/channel/adapter.go` | `TaskAdaptor` interface |
| 图片请求转换 | `relay/channel/ali/image.go` | `oaiImage2AliImageRequest()` |
| 图片响应处理 | `relay/channel/ali/image.go` | `aliImageHandler()` |
| 视频请求构建 | `relay/channel/task/ali/adaptor.go` | `convertToAliRequest()` |
| 视频计费估算 | `relay/channel/task/ali/adaptor.go` | `EstimateBilling()` |
| 分辨率价格比 | `relay/channel/task/ali/adaptor.go` | `ProcessAliOtherRatios()` |
| 任务状态解析 | `relay/channel/task/ali/adaptor.go` | `ParseTaskResult()` |
| Kling 中间件参考 | `middleware/kling_adapter.go` | `KlingRequestConvert()` |
| Jimeng 中间件参考 | `middleware/jimeng_adapter.go` | `JimengRequestConvert()` |
| 路由注册参考 | `router/video-router.go` | `SetVideoRouter()` |
