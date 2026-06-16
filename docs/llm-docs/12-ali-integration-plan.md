# 阿里云模型接入与计费实现计划

> 基于 `docs/reference/pricing/ali.csv` 分析，规划阿里云模型的接入和计费实现方案。

---

## 1. 模型清单分析

### 1.1 文本模型（按 Token 计费）

| 模型名称 | 输入价格（¥/百万tokens） | 输出价格 | 合同折扣 | 限时折扣 | 版本 |
|----------|------------------------|----------|----------|----------|------|
| qwen3.7-max | 12 | 36 | 0.x | 0.5 | qwen3.7-max-2026-05-20, qwen3.7-max-2026-06-08, qwen3.7-max-preview (only-think) |
| qwen3.7-plus | 2/6（阶梯） | 8/24（阶梯） | 0.x | 0.8 | qwen3.7-plus-2026-05-26 |
| qwen3.6-plus | 2/8（阶梯） | 12/48（阶梯） | 0.x | 1 | qwen3.6-plus-2026-04-02 |
| qwen3.6-flash | 1.2/4.8（阶梯） | 7.2/28.8（阶梯） | 0.x | 1 | qwen3.6-flash-2026-04-16 |
| qwen3.5-flash | 0.2/0.8/1.2（阶梯） | 2/8/12（阶梯） | 0.x | 1 | qwen3.5-flash-2026-02-23 |
| qwen-flash-character | 0.25 | 1.5 | 0.x | 1 | - |
| qwen-plus-character | 0.8 | 2 | 0.x | 1 | - |

**阶梯定价规则**：
- qwen3.7-plus: 0 < Token ≤ 256K → 基础价；256K < Token ≤ 1M → 3倍价
- qwen3.6-plus: 0 < Token ≤ 256K → 基础价；256K < Token ≤ 1M → 4倍价
- qwen3.6-flash: 0 < Token ≤ 256K → 基础价；256K < Token ≤ 1M → 4倍价
- qwen3.5-flash: 0 < Token ≤ 128K → 基础价；128K < Token ≤ 256K → 4倍价；256K < Token ≤ 1M → 6倍价

### 1.2 全模态模型（按 Token 计费，区分音频/文本+图片+视频）

| 模型名称 | 场景 | 输入价格 | 输出价格 | 合同折扣 | 限时折扣 |
|----------|------|----------|----------|----------|----------|
| qwen3.5-omni-flash | 音频输入→文本+音频输出 | 18 | 72 | 0.x | 1 |
| qwen3.5-omni-flash | 文本/图片/视频输入→文本输出 | 2.2 | 13.3 | 0.x | 1 |
| qwen3.5-omni-plus | 音频输入→文本+音频输出 | 53 | 213 | 0.x | 1 |
| qwen3.5-omni-plus | 文本/图片/视频输入→文本输出 | 7 | 40 | 0.x | 1 |
| qwen3.5-omni-plus-realtime | 音频输入→文本+音频输出 | 80 | 300 | 0.x | 1 |
| qwen3.5-omni-plus-realtime | 文本/图片/视频输入→文本输出 | 10 | 60 | 0.x | 1 |
| qwen3.5-omni-flash-realtime | 音频输入→文本+音频输出 | 27 | 107 | 0.x | 1 |
| qwen3.5-omni-flash-realtime | 文本/图片/视频输入→文本输出 | 3.3 | 20 | 0.x | 1 |

### 1.3 图片模型（按张计费）

| 模型名称 | 价格（¥/张） | 合同折扣 | 限时折扣 |
|----------|-------------|----------|----------|
| wan2.7-image-pro | 0.5 | 0.x | 1 |
| wan2.7-image | 0.2 | 0.x | 1 |
| qwen-image-2.0-pro | 0.5 | 0.x | 1 |
| qwen-image-2.0 | 0.2 | 0.x | 1 |
| qwen-image-edit-max | 0.5 | 0.x | 1 |
| qwen-image-edit-plus | 0.2 | 0.x | 1 |
| qwen-image-edit | 0.3 | 0.x | 1 |
| qwen-mt-image | 0.003 | 0.x | 1 |

### 1.4 视频模型（按秒计费，区分 720P/1080P）

| 模型名称 | 分辨率 | 输入价格 | 输出价格 | 合同折扣 | 限时折扣 |
|----------|--------|----------|----------|----------|----------|
| wan2.7-t2v | 720P | - | 0.6 | 0.x | 1 |
| wan2.7-t2v | 1080P | - | 1 | 0.x | 1 |
| wan2.7-r2v | 720P | 0.6 | 0.6 | 0.x | 1 |
| wan2.7-r2v | 1080P | 1 | 1 | 0.x | 1 |
| wan2.7-i2v | 720P | - | 0.6 | 0.x | 1 |
| wan2.7-i2v | 1080P | - | 1 | 0.x | 1 |
| wan2.7-videoedit | 720P | 0.6 | 0.6 | 0.x | 1 |
| wan2.7-videoedit | 1080P | 1 | 1 | 0.x | 1 |
| happyhorse-1.0-t2v | 720P | - | 0.9 | 0.x | 1 |
| happyhorse-1.0-t2v | 1080P | - | 1.6 | 0.x | 1 |
| happyhorse-1.0-r2v | 720P | - | 0.9 | 0.x | 1 |
| happyhorse-1.0-r2v | 1080P | - | 1.6 | 0.x | 1 |
| happyhorse-1.0-i2v | 720P | - | 0.9 | 0.x | 1 |
| happyhorse-1.0-i2v | 1080P | - | 1.6 | 0.x | 1 |
| happyhorse-1.0-video-edit | 720P | 0.9 | 0.9 | 0.x | 1 |
| happyhorse-1.0-video-edit | 1080P | 1.6 | 1.6 | 0.x | 1 |

---

## 2. 价格转换逻辑

### 2.1 币种转换

阿里云价格为人民币（¥），系统内部使用美元（$）。

**转换公式**：
```
美元价格 = 人民币价格 / USD2RMB（7.3）
```

**参考**：`setting/ratio_setting/model_ratio.go:50` 定义 `USD2RMB = 7.3`

### 2.2 Token 单位转换

阿里云价格为 ¥/百万tokens，系统内部倍率为 `$0.002/1K tokens`。

**文本模型倍率计算**：
```
modelRatio = (人民币价格 / 7.3) / 0.002
```

示例（qwen3.7-max 输入 12¥/百万tokens）：
```
modelRatio = (12 / 7.3) / 0.002 = 821.92
```

### 2.3 固定价格计算

对于按次/按秒计费的模型，使用 `model_price`（美元价格）。

**图片模型**：
```
modelPrice = 人民币价格 / 7.3  （单位：美元/张）
```

**视频模型**：
```
modelPrice = 人民币价格 / 7.3  （单位：美元/秒）
```

---

## 3. 需要修改的文件清单

### 3.1 渠道适配器（新增阿里云渠道）

**文件**：`relay/channel/ali/`（新建目录）

| 文件 | 说明 |
|------|------|
| `relay/channel/ali/relay.go` | 阿里云 relay 适配器 |
| `relay/channel/ali/task.go` | 阿里云异步任务适配器（视频生成） |
| `relay/channel/ali/adaptor.go` | 适配器注册 |

**修改理由**：需要实现阿里云 API 格式转换、任务提交/查询、结果解析。

### 3.2 渠道类型注册

**文件**：`constant/channel.go`

修改内容：
- 添加 `ChannelTypeAli = XX` 常量
- 添加 `ChannelBaseURLs[ChannelTypeAli]` 映射

**修改理由**：注册阿里云渠道类型。

### 3.3 任务平台注册

**文件**：`constant/task.go`

修改内容：
- 添加 `TaskPlatformAli TaskPlatform = "ali"` 常量

**修改理由**：支持阿里云异步任务平台。

### 3.4 模型倍率配置

**文件**：`setting/ratio_setting/model_ratio.go`

修改内容：
- 在 `defaultModelRatio` 中添加阿里云文本模型倍率
- 在 `defaultCompletionRatio` 中添加输出倍率
- 在 `defaultCacheRatio` 中添加缓存倍率（如有）

**示例配置**：
```go
// qwen3.7-max: 12¥/百万tokens输入, 36¥/百万tokens输出
"qwen3.7-max": 12 / 7.3 / 0.002,  // ≈ 821.92
// 输出倍率: 36/12 = 3
// qwen3.7-plus: 阶梯定价，需要使用 tiered_expr
```

**修改理由**：将阿里云价格转换为系统内部倍率。

### 3.5 分层表达式配置（阶梯定价模型）

**文件**：`setting/billing_setting/tiered_billing.go`（运行时配置，通过前端设置）

对于有阶梯定价的模型（qwen3.7-plus、qwen3.6-plus、qwen3.6-flash、qwen3.5-flash），需要使用分层表达式：

```javascript
// qwen3.7-plus 示例
tier("standard", len <= 256000 ? p * (2/7.3/0.002) + c * (8/7.3/0.002) : nil, 
     "long", len > 256000 ? p * (6/7.3/0.002) + c * (24/7.3/0.002) : nil)
```

**修改理由**：阶梯定价无法用单一倍率表示，必须使用分层表达式。

### 3.6 图片模型价格配置

**文件**：`setting/ratio_setting/model_ratio.go`（运行时配置，通过前端设置）

配置 `model_price`：
```json
{
  "wan2.7-image-pro": 0.5/7.3,
  "wan2.7-image": 0.2/7.3,
  "qwen-image-2.0-pro": 0.5/7.3,
  "qwen-image-2.0": 0.2/7.3,
  "qwen-image-edit-max": 0.5/7.3,
  "qwen-image-edit-plus": 0.2/7.3,
  "qwen-image-edit": 0.3/7.3,
  "qwen-mt-image": 0.003/7.3
}
```

**修改理由**：图片模型按张计费，使用固定价格模式。

### 3.7 视频模型价格配置

**文件**：`setting/ratio_setting/model_ratio.go`（运行时配置，通过前端设置）

配置 `model_price`（美元/秒）：
```json
{
  "wan2.7-t2v": 0.6/7.3,
  "wan2.7-r2v": 0.6/7.3,
  "wan2.7-i2v": 0.6/7.3,
  "wan2.7-videoedit": 0.6/7.3,
  "happyhorse-1.0-t2v": 0.9/7.3,
  "happyhorse-1.0-r2v": 0.9/7.3,
  "happyhorse-1.0-i2v": 0.9/7.3,
  "happyhorse-1.0-video-edit": 0.9/7.3
}
```

**注意**：视频模型需要区分 720P/1080P 价格，通过 `OtherRatios` 实现。

### 3.8 视频任务适配器实现

**文件**：`relay/channel/ali/task.go`

需要实现：
- `TaskPollingAdaptor` 接口
- `AdjustBillingOnComplete` 方法：根据实际时长计算最终价格

**关键逻辑**：
```go
func (a *AliAdaptor) AdjustBillingOnComplete(task *model.Task, taskResult *relaycommon.TaskInfo) int {
    // 1. 获取实际时长（秒）
    duration := taskResult.Duration
    
    // 2. 获取分辨率倍率（720P=1, 1080P=1.67）
    sizeRatio := task.PrivateData.BillingContext.OtherRatios["size"]
    
    // 3. 计算实际价格
    actualPrice := modelPrice * float64(duration) * sizeRatio
    
    // 4. 转换为 Quota
    actualQuota := int(actualPrice * common.QuotaPerUnit * groupRatio)
    
    return actualQuota
}
```

**修改理由**：视频计费必须在任务完成时才能确定实际价格（因为需要实际时长）。

### 3.9 全模态模型处理

**文件**：`relay/channel/ali/relay.go`

全模态模型（qwen3.5-omni-*）需要根据输入类型选择不同的价格：
- 音频输入 → 使用音频价格（18¥/百万tokens）
- 文本/图片/视频输入 → 使用文本价格（2.2¥/百万tokens）

**实现方式**：
1. 在请求中检测输入类型（通过 `content` 字段的 `type` 判断）
2. 设置不同的 `modelRatio` 或使用分层表达式的 `param()` 函数

**修改理由**：同一模型不同输入类型价格差异很大，需要动态选择。

---

## 4. 数据库变更

### 4.1 模型配置表

无需新增表，通过前端配置 `model_ratio`、`model_price`、`billing_mode`、`billing_expr` 等 JSON 字段即可。

### 4.2 渠道配置

在 `channels` 表中新增阿里云渠道记录，配置：
- `type`: 阿里云渠道类型 ID
- `base_url`: 阿里云 API 地址
- `key`: API Key
- `models`: 支持的模型列表

---

## 5. 实施步骤

### 第一阶段：基础接入（1-2天）

1. 创建 `relay/channel/ali/` 目录和基础适配器
2. 注册渠道类型和任务平台
3. 实现基本的文本模型 relay

### 第二阶段：计费配置（1天）

1. 配置文本模型倍率（qwen3.7-max、qwen-flash-character 等无阶梯模型）
2. 配置阶梯定价模型的分层表达式
3. 配置图片模型固定价格

### 第三阶段：视频任务（2-3天）

1. 实现视频任务提交适配器
2. 实现任务轮询和状态解析
3. 实现 `AdjustBillingOnComplete` 差额结算逻辑
4. 测试 720P/1080P 不同分辨率的计费

### 第四阶段：全模态支持（1-2天）

1. 实现输入类型检测
2. 动态选择音频/文本价格
3. 测试 realtime 变体

### 第五阶段：测试与优化（1-2天）

1. 单元测试：验证价格计算正确性
2. 集成测试：验证完整请求流程
3. 性能测试：验证高并发下的计费准确性

---

## 6. 风险与注意事项

### 6.1 价格精度风险

- 人民币/美元汇率（7.3）可能波动，建议支持配置化
- 小数精度问题：使用 `shopspring/decimal` 库避免浮点误差

### 6.2 视频计费时序风险

- 视频任务可能长时间处于 `IN_PROGRESS` 状态
- 需要设置合理的超时时间（`constant.TaskTimeoutMinutes`）
- 超时任务需要正确退款

### 6.3 阶梯定价边界条件

- Token 数量恰好等于阶梯边界时的处理
- 需要明确是 `<=` 还是 `<`

### 6.4 全模态模型混合输入

- 同一请求可能包含音频+文本+图片
- 需要明确如何拆分计费（按比例？按类型？）

### 6.5 折扣叠加

- 合同折扣和限时折扣是否叠加？
- 建议文档明确折扣计算顺序

---

## 7. 测试用例

### 7.1 文本模型测试

| 模型 | 输入tokens | 输出tokens | 预期输入价格 | 预期输出价格 |
|------|-----------|-----------|-------------|-------------|
| qwen3.7-max | 1000 | 500 | 12/7.3/0.002 × 1000 | 36/7.3/0.002 × 500 |
| qwen3.7-plus（短文本） | 100000 | 50000 | 2/7.3/0.002 × 100000 | 8/7.3/0.002 × 50000 |
| qwen3.7-plus（长文本） | 500000 | 100000 | 6/7.3/0.002 × 500000 | 24/7.3/0.002 × 100000 |

### 7.2 图片模型测试

| 模型 | 图片数量 | 预期价格 |
|------|---------|---------|
| wan2.7-image-pro | 1 | 0.5/7.3 × QuotaPerUnit |
| wan2.7-image | 5 | 0.2/7.3 × 5 × QuotaPerUnit |

### 7.3 视频模型测试

| 模型 | 时长 | 分辨率 | 预期价格 |
|------|------|--------|---------|
| wan2.7-t2v | 10秒 | 720P | 0.6/7.3 × 10 × QuotaPerUnit |
| wan2.7-t2v | 10秒 | 1080P | 1/7.3 × 10 × QuotaPerUnit |
| happyhorse-1.0-t2v | 5秒 | 720P | 0.9/7.3 × 5 × QuotaPerUnit |

---

## 8. 汇率配置深度分析

### 8.1 系统中的三个汇率变量

| 变量 | 文件位置 | 类型 | 默认值 | 用途 |
|------|----------|------|--------|------|
| `USD2RMB` | `setting/ratio_setting/model_ratio.go:13` | **编译时常量** | 7.3 | 仅用于定义中文模型的**默认倍率**（ERNIE、GLM 等） |
| `USDExchangeRate` | `setting/operation_setting/payment_setting_old.go:18` | **运行时变量** | 7.3 | 仅用于**展示**（日志、API 响应中的货币转换） |
| `CustomCurrencyExchangeRate` | `setting/operation_setting/general_setting.go:22` | **运行时变量** | 1.0 | 自定义货币展示汇率 |

### 8.2 关键结论：汇率不参与实际计费计算

**代码证据分析**：

1. **`USD2RMB = 7.3`（编译时常量）**
   - 仅在 `defaultModelRatio` 初始化时使用，用于定义中文模型的默认倍率
   - 示例：`"ERNIE-4.0-8K": 0.120 * RMB`（其中 `RMB = USD / USD2RMB`）
   - **不可从前端修改**，是硬编码常量

2. **`USDExchangeRate = 7.3`（运行时变量）**
   - 可通过前端「系统设置 → 运营设置」修改
   - 存储在 `options` 表，键为 `USDExchangeRate`
   - **仅用于展示**，不参与计费计算
   - 使用位置：
     - `controller/billing.go:50` — 额度查询响应中的货币转换
     - `controller/billing.go:96` — 使用量查询响应中的货币转换
     - `controller/misc.go:95` — 系统信息 API 返回
     - `logger/logger.go:128,154` — 日志中额度的格式化显示

3. **实际计费使用的是什么？**
   - 倍率模式：使用 `modelRatio`（前端配置的 JSON）
   - 固定价格模式：使用 `modelPrice`（前端配置的 JSON）
   - 这些值**直接就是计费乘数**，不经过任何汇率转换

### 8.3 将 USDExchangeRate 设置为 1 的影响

**影响范围**：

| 场景 | 影响 |
|------|------|
| 实际计费计算 | ❌ **无影响** — 计费不使用此变量 |
| 前端额度显示（CNY 模式） | ✅ **有影响** — 显示的人民币金额会变化 |
| 日志中额度显示（CNY 模式） | ✅ **有影响** — 日志中的货币金额会变化 |
| API 响应中的额度 | ✅ **有影响** — `/v1/dashboard/billing/subscription` 等接口 |

**示例**：
- 假设用户有 1,000,000 Quota
- `USDExchangeRate = 7.3` 时，CNY 显示：`1,000,000 / 500,000 × 7.3 = ¥14.6`
- `USDExchangeRate = 1` 时，CNY 显示：`1,000,000 / 500,000 × 1 = ¥2.0`
- 实际计费扣费金额**完全相同**

### 8.4 对阿里云模型接入的启示

**正确的配置方式**：

1. **文本模型倍率**：直接在前端配置 `ModelRatio` JSON，**不需要除以汇率**
   - 例：qwen3.7-max 输入 12¥/百万tokens
   - 倍率 = `12 / 7.3 / 0.002 = 821.92`（这里的 7.3 是代码中的 `USD2RMB` 常量，不是前端配置的 `USDExchangeRate`）
   - 或者更简单：直接在前端输入 `821.92` 作为倍率

2. **固定价格模型**：直接在前端配置 `ModelPrice` JSON，单位是**美元**
   - 例：wan2.7-image 0.2¥/张
   - 价格 = `0.2 / 7.3 = 0.0274` 美元
   - 在前端输入 `0.0274`

3. **汇率配置建议**：
   - 如果希望前端显示人民币：设置 `QuotaDisplayType = "CNY"`，`USDExchangeRate = 7.3`
   - 如果希望前端显示美元：设置 `QuotaDisplayType = "USD"`
   - **不建议将 USDExchangeRate 设为 1**，因为这会导致 CNY 显示模式下的金额显示错误（显示的是美元数值但标注为 ¥）

### 8.5 代码位置总结

| 文件 | 行号 | 说明 |
|------|------|------|
| `setting/ratio_setting/model_ratio.go` | 13-16 | `USD2RMB`、`USD`、`RMB` 常量定义 |
| `setting/operation_setting/payment_setting_old.go` | 18 | `USDExchangeRate` 变量定义 |
| `model/option.go` | 82 | `USDExchangeRate` 初始化到 OptionMap |
| `model/option.go` | 397-398 | `USDExchangeRate` 更新处理 |
| `controller/billing.go` | 50, 96 | 额度展示中的汇率使用 |
| `logger/logger.go` | 128, 154 | 日志格式化中的汇率使用 |
| `controller/misc.go` | 95 | 系统信息 API 返回汇率 |
