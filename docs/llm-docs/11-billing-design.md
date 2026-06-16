# 计费系统设计与使用文档

> 本文档基于代码实际实现编写，涵盖计费系统的完整设计、配置方式和执行流程。

---

## 1. 计费系统总览

### 1.1 核心概念

| 概念 | 说明 |
|------|------|
| **Quota（配额）** | 系统内部计费单位，`QuotaPerUnit = 500,000`，即 `$0.002 = 1 Quota`（`common/constants.go:62`） |
| **倍率（Ratio）** | 将模型原始价格转换为 Quota 的乘数，`1 = $0.002/1K tokens` |
| **固定价格（ModelPrice）** | 直接以美元定价，绕过倍率体系，适用于按次/按量计费 |
| **分层表达式（TieredExpr）** | 基于表达式的动态计费，支持阶梯定价、缓存差异化、音频/图片独立计价 |
| **预扣费（PreConsume）** | 请求到达时按估算 token 数预扣，完成后差额结算 |
| **资金来源（FundingSource）** | 钱包（Wallet）或订阅（Subscription），由用户偏好决定 |

### 1.2 三种计费模式

| 模式 | 标识 | 说明 | 配置位置 |
|------|------|------|----------|
| **倍率计费** | `ratio`（默认） | 使用 ModelRatio × CompletionRatio × CacheRatio 等倍率 | `ratio_setting/model_ratio.go` |
| **固定价格计费** | `ratio` + `ModelPrice` | 模型配置了 `model_price`，直接以美元 × QuotaPerUnit 计费 | `ratio_setting/model_ratio.go:GetModelPrice` |
| **分层表达式计费** | `tiered_expr` | 使用 expr-lang 表达式，支持阶梯、缓存、音频差异化 | `billing_setting/tiered_billing.go` |

---

## 2. 计费类别详解

### 2.1 文本 Token 计费

**代码路径**：`service/text_quota.go` → `calculateTextQuota` → `PostTextConsumeQuota`

#### 2.1.1 倍率模式计算公式

```
quota = (promptTokens × modelRatio + completionTokens × completionRatio × modelRatio 
        + cacheReadTokens × cacheRatio × modelRatio 
        + cacheCreationTokens × cacheCreationRatio × modelRatio
        + imageTokens × imageRatio × modelRatio
        + audioTokens × audioRatio × modelRatio
        + audioCompletionTokens × audioCompletionRatio × modelRatio)
        × groupRatio
```

**关键字段说明**（`service/text_quota.go:textQuotaSummary`）：

| 字段 | 说明 |
|------|------|
| `ModelRatio` | 模型基础倍率（`ratio_setting.GetModelRatio`） |
| `CompletionRatio` | 输出 token 倍率（`ratio_setting.GetCompletionRatio`） |
| `CacheRatio` | 缓存读取倍率（`ratio_setting.GetCacheRatio`） |
| `CacheCreationRatio` | 缓存创建倍率（5分钟 TTL） |
| `CacheCreationRatio1h` | 缓存创建倍率（1小时 TTL，Claude 专用，= CacheCreationRatio × 1.6） |
| `ImageRatio` | 图片 token 倍率 |
| `AudioRatio` | 音频输入 token 倍率 |
| `AudioCompletionRatio` | 音频输出 token 倍率 |
| `GroupRatio` | 分组倍率（`ratio_setting.GetGroupRatio`） |

#### 2.1.2 固定价格模式计算公式

```
quota = modelPrice × QuotaPerUnit × groupRatio
```

当模型配置了 `model_price` 时，直接以美元价格 × QuotaPerUnit（500,000）× 分组倍率计算。

#### 2.1.3 分层表达式模式

详见 `pkg/billingexpr/expr.md`。核心特点：
- 表达式系数是 **$/1M tokens** 的真实价格
- 转换公式：`quota = exprOutput / 1,000,000 × QuotaPerUnit × groupRatio`
- 支持 `p`（输入）、`c`（输出）、`cr`（缓存读取）、`cc`（缓存创建）、`img`（图片）、`ai`（音频输入）、`ao`（音频输出）等变量
- `p` 和 `c` 会自动排除被表达式单独计价的子类别（AST 自省机制）

### 2.2 图片生成计费

**代码路径**：`relay/helper/price.go` → `ModelPriceHelperPerCall`

图片模型（如 DALL-E、Midjourney）使用按次计费：
```
quota = modelPrice × QuotaPerUnit × groupRatio
```

### 2.3 音频计费

**代码路径**：`service/text_quota.go` → `calculateTextQuota`

音频 token 通过 `AudioRatio` 和 `AudioCompletionRatio` 独立计价。当表达式使用 `ai`/`ao` 变量时，自动从 `p`/`c` 中扣除。

### 2.4 工具调用计费

**代码路径**：`service/tool_billing.go` → `ComputeToolCallQuota`

支持的工具：
- `web_search_preview` / `web_search`：网页搜索
- `file_search`：文件搜索
- `image_generation`：图片生成（按次）

计算公式：
```
quota = pricePer1K × callCount / 1000 × QuotaPerUnit × groupRatio
```

### 2.5 异步任务计费（视频/音频生成）

**代码路径**：
- 提交：`relay/relay_task.go` → `RelayTaskSubmit`
- 轮询：`service/task_polling.go` → `UpdateVideoTasks`
- 结算：`service/task_billing.go` → `RecalculateTaskQuota`

**两阶段计费流程**：

1. **提交阶段**：按估算参数（时长、分辨率）预扣费
2. **完成阶段**：任务状态变为 `SUCCESS` 时，通过 `settleTaskBillingOnComplete` 进行差额结算

差额结算优先级（`service/task_polling.go:settleTaskBillingOnComplete`）：
1. `adaptor.AdjustBillingOnComplete` 返回正数 → 使用 adaptor 计算的额度
2. `taskResult.TotalTokens > 0` → 按 token 重算
3. 都不满足 → 保持预扣额度不变

---

## 3. 前端配置指南

### 3.1 倍率计费配置

**路径**：系统设置 → 分组与模型定价设置

| 配置项 | 说明 | 数据库键 |
|--------|------|----------|
| 模型倍率 | `model_ratio` JSON | `model_ratio` |
| 输出倍率 | `completion_ratio` JSON | `completion_ratio` |
| 缓存读取倍率 | `cache_ratio` JSON | `cache_ratio` |
| 缓存创建倍率 | `create_cache_ratio` JSON | `create_cache_ratio` |
| 图片倍率 | `image_ratio` JSON | `image_ratio` |
| 音频倍率 | `audio_ratio` JSON | `audio_ratio` |
| 音频输出倍率 | `audio_completion_ratio` JSON | `audio_completion_ratio` |
| 分组倍率 | `group_ratio` JSON | `group_ratio` |

### 3.2 固定价格配置

**路径**：系统设置 → 分组与模型定价设置 → 模型价格

配置 `model_price` JSON，格式：`{"model-name": 0.015}`（单位：美元/1K tokens 或美元/次）。

### 3.3 分层表达式配置

**路径**：系统设置 → 分组与模型定价设置 → 分层计费编辑器

1. 设置 `billing_mode` 为 `tiered_expr`
2. 在 `billing_expr` 中配置表达式

示例：
```
tier("standard", len <= 200000 ? p * 0.8 + c * 3.2 : p * 1.2 + c * 4.8, "long", len > 200000 ? p * 1.2 + c * 4.8 : nil)
```

### 3.4 工具调用价格配置

**路径**：系统设置 → 运营设置 → 工具价格

配置 `tool_price` JSON，支持按模型前缀覆盖。

### 3.5 分组倍率配置

**路径**：系统设置 → 分组与模型定价设置 → 分组倍率

配置不同用户分组的倍率乘数。

---

## 4. 请求计费完整流程

### 4.1 文本请求计费流程

```
请求到达
  │
  ▼
controller/relay.go:Relay()
  │
  ├─ helper.GetAndValidateRequest()         // 解析请求
  ├─ relaycommon.GenRelayInfo()             // 构建 RelayInfo
  ├─ service.EstimateRequestToken()         // 估算 prompt tokens
  │
  ▼
relay/helper/price.go:ModelPriceHelper()
  │
  ├─ ratio_setting.GetModelPrice()          // 检查固定价格
  ├─ billing_setting.GetBillingMode()       // 检查计费模式
  │
  ├─ [倍率模式]
  │   ├─ ratio_setting.GetModelRatio()
  │   ├─ ratio_setting.GetCompletionRatio()
  │   ├─ ratio_setting.GetCacheRatio()
  │   └─ 计算 preConsumedQuota
  │
  ├─ [分层表达式模式]
  │   ├─ billing_setting.GetBillingExpr()
  │   ├─ billingexpr.RunExprWithRequest()
  │   └─ 创建 BillingSnapshot
  │
  ▼
service/billing.go:PreConsumeBilling()
  │
  ├─ NewBillingSession()                    // 创建计费会话
  │   ├─ 检查用户计费偏好（subscription_first/wallet_first/...）
  │   ├─ tryWallet() → WalletFunding.PreConsume()
  │   └─ trySubscription() → SubscriptionFunding.PreConsume()
  │
  ▼
relay:TextHelper()                          // 调用上游 API
  │
  ▼
service/text_quota.go:PostTextConsumeQuota()
  │
  ├─ calculateTextQuota()                   // 计算实际配额
  │   ├─ 处理 token 统计
  │   ├─ [分层表达式] TryTieredSettle()
  │   └─ [倍率/固定价格] 标准计算
  │
  ├─ calculateTextToolCallSurcharge()       // 工具调用附加费
  │
  ├─ service/billing.go:SettleBilling()
  │   ├─ BillingSession.Settle()
  │   │   ├─ FundingSource.Settle(delta)    // 调整资金来源
  │   │   └─ model.IncreaseTokenQuota()     // 调整令牌额度
  │   └─ 发送额度通知
  │
  └─ model.RecordConsumeLog()               // 记录消费日志
```

### 4.2 异步任务计费流程

```
任务提交
  │
  ▼
controller/relay.go:RelayTask()
  │
  ├─ helper.ModelPriceHelperPerCall()       // 按次计费价格计算
  ├─ service/billing.go:PreConsumeBilling() // 预扣费
  │
  ▼
relay/relay_task.go:RelayTaskSubmit()
  │
  ├─ adaptor.DoRequest()                    // 提交到上游
  ├─ service/billing.go:SettleBilling()     // 结算（提交时的预扣费）
  └─ task.Insert()                          // 保存任务
  │
  ▼
轮询循环（每15秒）
  │
  ▼
service/task_polling.go:UpdateVideoTasks()
  │
  ├─ adaptor.FetchTask()                    // 查询上游状态
  ├─ adaptor.ParseTaskResult()
  │
  ├─ [SUCCESS]
  │   ├─ settleTaskBillingOnComplete()
  │   │   ├─ adaptor.AdjustBillingOnComplete()
  │   │   ├─ RecalculateTaskQuota()
  │   │   │   ├─ taskAdjustFunding()        // 调整资金来源
  │   │   │   └─ taskAdjustTokenQuota()     // 调整令牌额度
  │   │   └─ RecalculateTaskQuotaByTokens()
  │   └─ RecordTaskBillingLog()
  │
  └─ [FAILURE]
      └─ RefundTaskQuota()                  // 退还预扣费
```

---

## 5. 资金来源与计费偏好

### 5.1 FundingSource 接口

**代码路径**：`service/funding_source.go`

```go
type FundingSource interface {
    Source() string          // "wallet" 或 "subscription"
    PreConsume(amount int) error
    Settle(delta int) error
    Refund() error
}
```

### 5.2 计费偏好模式

**代码路径**：`service/billing_session.go:NewBillingSession`

| 模式 | 说明 |
|------|------|
| `subscription_first` | 优先使用订阅，不足时回退到钱包（默认） |
| `wallet_first` | 优先使用钱包，不足时回退到订阅 |
| `subscription_only` | 仅使用订阅 |
| `wallet_only` | 仅使用钱包 |

### 5.3 信任额度旁路

当用户额度 > `trustQuota`（10 × QuotaPerUnit）且令牌额度也充足时，预扣费金额为 0，减少数据库写入。

---

## 6. 关键数据库表结构

### 6.1 用户表（users）

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | int | 用户 ID |
| `quota` | int | 用户剩余额度（Quota） |
| `used_quota` | int | 已使用额度 |
| `request_count` | int | 请求次数 |

### 6.2 令牌表（tokens）

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | int | 令牌 ID |
| `key` | varchar | 令牌 Key |
| `quota` | int | 令牌剩余额度 |
| `used_quota` | int | 已使用额度 |

### 6.3 渠道表（channels）

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | int | 渠道 ID |
| `used_quota` | int | 渠道已使用额度 |
| `type` | int | 渠道类型 |

### 6.4 消费日志表（logs）

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | int | 日志 ID |
| `user_id` | int | 用户 ID |
| `channel_id` | int | 渠道 ID |
| `model_name` | varchar | 模型名称 |
| `quota` | int | 消费额度 |
| `prompt_tokens` | int | 输入 token 数 |
| `completion_tokens` | int | 输出 token 数 |
| `token_name` | varchar | 令牌名称 |
| `other` | text | JSON 扩展字段（倍率、价格等） |

### 6.5 任务表（tasks）

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | int | 任务 ID |
| `task_id` | varchar | 公开任务 ID |
| `user_id` | int | 用户 ID |
| `quota` | int | 预扣费额度 |
| `status` | varchar | 任务状态 |
| `platform` | varchar | 任务平台 |
| `private_data` | text | JSON（含 BillingContext、BillingSource 等） |

---

## 7. 关键配置文件

| 文件 | 说明 |
|------|------|
| `setting/ratio_setting/model_ratio.go` | 模型倍率、价格、完成比等配置 |
| `setting/ratio_setting/group_ratio.go` | 分组倍率配置 |
| `setting/ratio_setting/cache_ratio.go` | 缓存倍率配置 |
| `setting/billing_setting/tiered_billing.go` | 分层表达式计费配置 |
| `setting/operation_setting/` | 运营设置（工具价格、免费模型预扣等） |
| `common/constants.go` | QuotaPerUnit 定义 |
| `pkg/billingexpr/` | 分层表达式引擎 |

---

## 8. 常见问题

### Q1: 为什么预扣费和实际扣费不一致？

预扣费基于估算的 token 数（prompt_tokens + estimated_max_tokens），实际扣费基于上游返回的真实 token 数。系统通过差额结算自动补扣或退还。

### Q2: 视频任务为什么需要两阶段计费？

视频生成是异步任务，提交时无法知道实际时长。系统在提交时按估算参数预扣费，任务完成时根据实际参数（时长、分辨率）进行差额结算。

### Q3: 分组倍率如何影响计费？

分组倍率作为最终乘数应用于所有计费模式：`finalQuota = baseQuota × groupRatio`。不同用户分组可以有不同的倍率。

### Q4: 订阅计费和钱包计费有什么区别？

- **钱包计费**：直接扣减用户余额，支持任意金额
- **订阅计费**：扣减订阅额度，有预扣费机制和 requestId 幂等保护
