# 计费调试日志独立开关设计方案

> 为计费系统添加独立的调试日志开关，不依赖全局 DEBUG 开关，可详细输出计费计算的每一个细节。

---

## 1. 设计目标

1. **独立开关**：通过配置文件或环境变量单独控制计费日志，不影响其他模块的日志输出
2. **详细输出**：记录计费计算的每一步，包括：
   - 输入参数（token 数量、倍率、价格等）
   - 中间计算结果
   - 缓存命中/未命中
   - 数据库操作（扣费、退款、差额结算）
   - 最终结果
3. **代码定位**：每条日志包含源文件路径和行号
4. **格式一致**：日志格式与系统原有日志（`logger.logHelper`）保持一致，仅新增文件行号字段
5. **性能友好**：开关关闭时零开销（运行时 atomic 判断）

---

## 2. 系统原有日志格式分析

### 2.1 `logger.logHelper` 格式（`logger/logger.go`）

```
[LEVEL] 2006/01/02 - 15:04:05 | request-id | message
```

**代码**：
```go
fmt.Fprintf(writer, "[%s] %v | %s | %s \n", level, now.Format("2006/01/02 - 15:04:05"), id, msg)
```

**实际输出示例**：
```
[INFO] 2026/06/16 - 15:30:09 | abc-123-request | 预扣费后补扣费：＄0.001000（实际消耗：＄0.002000，预扣费：＄0.001000）
[ERR] 2026/06/16 - 15:30:09 | SYSTEM | error settling billing: connection refused
[DEBUG] 2026/06/16 - 15:30:09 | abc-123-request | model_price_helper_result: ...
```

### 2.2 `common.SysLog` 格式（`common/sys_log.go`）

```
[SYS] 2006/01/02 - 15:04:05 | message
```

### 2.3 计费调试日志格式（新增）

```
[BILLING_DEBUG] 2006/01/02 - 15:04:05 | request-id | file.go:line | message
```

**与原格式的差异**：
- 新增 `[BILLING_DEBUG]` 级别标识
- 在 `request-id` 和 `message` 之间插入 `file.go:line`
- 其余格式（时间戳格式、分隔符）完全一致

---

## 3. 实现方案

### 3.1 配置层设计

**文件**：`setting/billing_setting/billing_debug.go`（新建）

```go
package billing_setting

import (
    "github.com/QuantumNous/new-api/setting/config"
)

type BillingDebugConfig struct {
    Enabled bool `json:"enabled"`
}

var billingDebug = BillingDebugConfig{
    Enabled: false,
}

func init() {
    config.GlobalConfig.Register("billing_debug", &billingDebug)
}

func IsBillingDebugEnabled() bool {
    return billingDebug.Enabled
}
```

**配置方式**：
- 前端：系统设置 → 计费设置 → 计费调试日志开关
- 数据库：`configs` 表中 `billing_debug` 键

### 3.2 日志工具函数

**文件**：`logger/billing_debug.go`（新建）

```go
package logger

import (
    "fmt"
    "path/filepath"
    "runtime"
    "time"

    "github.com/QuantumNous/new-api/common"
    "github.com/QuantumNous/new-api/setting/billing_setting"
    "github.com/gin-gonic/gin"
)

// BillingDebugf 输出计费调试日志，格式与 logHelper 一致，额外包含 file:line。
// 格式：[BILLING_DEBUG] 2006/01/02 - 15:04:05 | request-id | file.go:42 | message
func BillingDebugf(ctx interface{}, format string, args ...interface{}) {
    if !billing_setting.IsBillingDebugEnabled() {
        return
    }
    msg := format
    if len(args) > 0 {
        msg = fmt.Sprintf(format, args...)
    }
    billingDebugLog(ctx, msg)
}

// BillingDebugMap 输出计费调试日志（键值对格式）。
// 格式：[BILLING_DEBUG] 2006/01/02 - 15:04:05 | request-id | file.go:42 | key1=val1 key2=val2
func BillingDebugMap(ctx interface{}, fields map[string]interface{}) {
    if !billing_setting.IsBillingDebugEnabled() {
        return
    }
    msg := ""
    for k, v := range fields {
        if msg != "" {
            msg += " "
        }
        msg += fmt.Sprintf("%s=%v", k, v)
    }
    billingDebugLog(ctx, msg)
}

// billingDebugLog 内部实现：获取调用者信息并输出日志。
// 跳过 2 层调用栈（billingDebugLog → BillingDebugf/BillingDebugMap → 调用者）。
func billingDebugLog(ctx interface{}, msg string) {
    // 获取调用者信息（skip=2：billingDebugLog → BillingDebugf/BillingDebugMap → 调用者）
    pc, file, line, ok := runtime.Caller(2)
    if !ok {
        file = "unknown"
        line = 0
    }
    _ = pc // funcName 可按需启用
    fileName := filepath.Base(file)
    fileLine := fmt.Sprintf("%s:%d", fileName, line)

    // 获取 request-id，与 logHelper 逻辑保持一致
    var id interface{} = "SYSTEM"
    if ctx != nil {
        if gCtx, ok := ctx.(*gin.Context); ok && gCtx != nil {
            if requestID := gCtx.Value(common.RequestIdKey); requestID != nil {
                id = requestID
            }
        }
    }

    now := time.Now()
    // 输出格式与 logHelper 完全一致，仅在 request-id 和 message 之间插入 file:line
    common.LogWriterMu.RLock()
    _, _ = fmt.Fprintf(gin.DefaultWriter, "[BILLING_DEBUG] %v | %v | %s | %s \n",
        now.Format("2006/01/02 - 15:04:05"), id, fileLine, msg)
    common.LogWriterMu.RUnlock()
}
```

**关键设计**：
- 使用 `gin.DefaultWriter` 而非 `fmt.Printf`，与系统日志输出目标一致（stdout + 日志文件）
- 使用 `common.LogWriterMu` 保护并发写入
- 时间戳格式 `2006/01/02 - 15:04:05` 与系统完全一致
- 接受 `interface{}` 类型的 ctx 参数，兼容 `*gin.Context` 和 `context.Context`

### 3.3 埋点位置清单

#### 3.3.1 预扣费阶段

**文件**：`relay/helper/price.go` → `ModelPriceHelper`

```go
// 在函数开头添加
logger.BillingDebugMap(c, map[string]interface{}{
    "stage": "pre_consume_start",
    "model": info.OriginModelName,
    "prompt_tokens": promptTokens,
    "max_tokens": meta.MaxTokens,
})

// 在获取倍率后添加
logger.BillingDebugMap(c, map[string]interface{}{
    "stage": "ratio_loaded",
    "model_ratio": modelRatio,
    "completion_ratio": completionRatio,
    "cache_ratio": cacheRatio,
    "group_ratio": groupRatioInfo.GroupRatio,
    "use_price": usePrice,
    "model_price": modelPrice,
})

// 在计算预扣费后添加
logger.BillingDebugMap(c, map[string]interface{}{
    "stage": "pre_consume_calculated",
    "pre_consumed_quota": preConsumedQuota,
    "free_model": freeModel,
})
```

**文件**：`relay/helper/price.go` → `modelPriceHelperTiered`

```go
// 在表达式执行前添加
logger.BillingDebugMap(c, map[string]interface{}{
    "stage": "tiered_expr_start",
    "model": info.OriginModelName,
    "expr": exprStr,
    "prompt_tokens": promptTokens,
    "estimated_completion": estimatedCompletionTokens,
})

// 在表达式执行后添加
logger.BillingDebugMap(c, map[string]interface{}{
    "stage": "tiered_expr_result",
    "raw_cost": rawCost,
    "matched_tier": trace.MatchedTier,
    "quota_before_group": quotaBeforeGroup,
    "group_ratio": groupRatioInfo.GroupRatio,
    "pre_consumed_quota": preConsumedQuota,
})
```

#### 3.3.2 计费会话创建

**文件**：`service/billing_session.go` → `NewBillingSession`

```go
// 在函数开头添加
logger.BillingDebugMap(c, map[string]interface{}{
    "stage": "session_create",
    "user_id": relayInfo.UserId,
    "pre_consumed_quota": preConsumedQuota,
    "billing_preference": pref,
})

// 在选择资金来源后添加
logger.BillingDebugMap(c, map[string]interface{}{
    "stage": "funding_selected",
    "funding_source": session.funding.Source(),
    "user_quota": relayInfo.UserQuota,
})

// 在预扣费执行后添加
logger.BillingDebugMap(c, map[string]interface{}{
    "stage": "pre_consume_executed",
    "funding_source": session.funding.Source(),
    "actual_pre_consumed": session.preConsumedQuota,
    "trusted": session.trusted,
})
```

#### 3.3.3 结算阶段

**文件**：`service/billing.go` → `SettleBilling`

```go
// 在函数开头添加
logger.BillingDebugMap(ctx, map[string]interface{}{
    "stage": "settle_start",
    "actual_quota": actualQuota,
    "pre_consumed": relayInfo.Billing.GetPreConsumedQuota(),
    "delta": actualQuota - relayInfo.Billing.GetPreConsumedQuota(),
})
```

**文件**：`service/billing_session.go` → `Settle`

```go
// 在资金来源调整前添加
logger.BillingDebugMap(nil, map[string]interface{}{
    "stage": "settle_funding",
    "funding_source": s.funding.Source(),
    "delta": delta,
    "funding_settled": s.fundingSettled,
})

// 在资金来源调整后添加
logger.BillingDebugMap(nil, map[string]interface{}{
    "stage": "settle_funding_done",
    "funding_source": s.funding.Source(),
    "delta": delta,
    "error": err,
})

// 在令牌额度调整前添加
logger.BillingDebugMap(nil, map[string]interface{}{
    "stage": "settle_token",
    "token_id": s.relayInfo.TokenId,
    "delta": delta,
    "is_playground": s.relayInfo.IsPlayground,
})
```

#### 3.3.4 退款阶段

**文件**：`service/billing_session.go` → `Refund`

```go
// 在退款开始时添加
logger.BillingDebugMap(c, map[string]interface{}{
    "stage": "refund_start",
    "user_id": s.relayInfo.UserId,
    "token_consumed": s.tokenConsumed,
    "funding_source": s.funding.Source(),
})
```

#### 3.3.5 文本计费计算

**文件**：`service/text_quota.go` → `PostTextConsumeQuota`

```go
// 在 calculateTextQuota 调用后添加
logger.BillingDebugMap(ctx, map[string]interface{}{
    "stage": "text_quota_calculated",
    "model": summary.ModelName,
    "prompt_tokens": summary.PromptTokens,
    "completion_tokens": summary.CompletionTokens,
    "cache_tokens": summary.CacheTokens,
    "cache_creation_tokens": summary.CacheCreationTokens,
    "image_tokens": summary.ImageTokens,
    "audio_tokens": summary.AudioTokens,
    "model_ratio": summary.ModelRatio,
    "completion_ratio": summary.CompletionRatio,
    "cache_ratio": summary.CacheRatio,
    "group_ratio": summary.GroupRatio,
    "model_price": summary.ModelPrice,
    "quota": summary.Quota,
    "is_tiered": tieredBillingApplied,
})
```

#### 3.3.6 异步任务计费

**文件**：`service/task_billing.go` → `RecalculateTaskQuota`

```go
// 在函数开头添加
logger.BillingDebugMap(ctx, map[string]interface{}{
    "stage": "task_recalculate",
    "task_id": task.TaskID,
    "actual_quota": actualQuota,
    "pre_consumed": task.Quota,
    "delta": actualQuota - task.Quota,
    "reason": reason,
})
```

**文件**：`service/task_polling.go` → `settleTaskBillingOnComplete`

```go
// 在函数开头添加
logger.BillingDebugMap(ctx, map[string]interface{}{
    "stage": "task_settle_on_complete",
    "task_id": task.TaskID,
    "status": taskResult.Status,
    "total_tokens": taskResult.TotalTokens,
    "per_call_billing": bc.PerCallBilling,
})
```

#### 3.3.7 资金来源操作

**文件**：`service/funding_source.go`

```go
// WalletFunding.PreConsume
logger.BillingDebugMap(nil, map[string]interface{}{
    "stage": "wallet_pre_consume",
    "user_id": w.userId,
    "amount": amount,
})

// WalletFunding.Settle
logger.BillingDebugMap(nil, map[string]interface{}{
    "stage": "wallet_settle",
    "user_id": w.userId,
    "delta": delta,
})

// WalletFunding.Refund
logger.BillingDebugMap(nil, map[string]interface{}{
    "stage": "wallet_refund",
    "user_id": w.userId,
    "consumed": w.consumed,
})

// SubscriptionFunding.PreConsume
logger.BillingDebugMap(nil, map[string]interface{}{
    "stage": "subscription_pre_consume",
    "user_id": s.userId,
    "request_id": s.requestId,
    "amount": s.amount,
})

// SubscriptionFunding.Settle
logger.BillingDebugMap(nil, map[string]interface{}{
    "stage": "subscription_settle",
    "subscription_id": s.subscriptionId,
    "delta": delta,
})
```

---

## 4. 日志输出示例

### 4.1 文本请求（倍率模式）

```
[BILLING_DEBUG] 2026/06/16 - 10:30:15 | req-abc123 | price.go:85 | stage=pre_consume_start model=gpt-4o prompt_tokens=1000 max_tokens=500
[BILLING_DEBUG] 2026/06/16 - 10:30:15 | req-abc123 | price.go:120 | stage=ratio_loaded model_ratio=1.25 completion_ratio=1 cache_ratio=0 group_ratio=1 use_price=false model_price=0
[BILLING_DEBUG] 2026/06/16 - 10:30:15 | req-abc123 | price.go:150 | stage=pre_consume_calculated pre_consumed_quota=937500 free_model=false
[BILLING_DEBUG] 2026/06/16 - 10:30:15 | req-abc123 | billing_session.go:280 | stage=session_create user_id=123 pre_consumed_quota=937500 billing_preference=subscription_first
[BILLING_DEBUG] 2026/06/16 - 10:30:15 | req-abc123 | billing_session.go:310 | stage=funding_selected funding_source=wallet user_quota=10000000
[BILLING_DEBUG] 2026/06/16 - 10:30:15 | req-abc123 | funding_source.go:45 | stage=wallet_pre_consume user_id=123 amount=937500
[BILLING_DEBUG] 2026/06/16 - 10:30:15 | req-abc123 | billing_session.go:320 | stage=pre_consume_executed funding_source=wallet actual_pre_consumed=937500 trusted=false
...（上游 API 调用）...
[BILLING_DEBUG] 2026/06/16 - 10:30:16 | req-abc123 | text_quota.go:400 | stage=text_quota_calculated model=gpt-4o prompt_tokens=1000 completion_tokens=450 cache_tokens=0 model_ratio=1.25 completion_ratio=1 group_ratio=1 quota=843750
[BILLING_DEBUG] 2026/06/16 - 10:30:16 | req-abc123 | billing.go:35 | stage=settle_start actual_quota=843750 pre_consumed=937500 delta=-93750
[BILLING_DEBUG] 2026/06/16 - 10:30:16 | req-abc123 | billing_session.go:45 | stage=settle_funding funding_source=wallet delta=-93750 funding_settled=false
[BILLING_DEBUG] 2026/06/16 - 10:30:16 | req-abc123 | funding_source.go:65 | stage=wallet_settle user_id=123 delta=-93750
[BILLING_DEBUG] 2026/06/16 - 10:30:16 | req-abc123 | billing_session.go:55 | stage=settle_funding_done funding_source=wallet delta=-93750 error=<nil>
[BILLING_DEBUG] 2026/06/16 - 10:30:16 | req-abc123 | billing_session.go:60 | stage=settle_token token_id=456 delta=-93750 is_playground=false
[BILLING_DEBUG] 2026/06/16 - 10:30:16 | req-abc123 | billing_session.go:70 | stage=settle_token_done token_id=456 delta=-93750 error=<nil>
```

### 4.2 视频任务（两阶段计费）

```
[BILLING_DEBUG] 2026/06/16 - 10:35:00 | req-xyz789 | price.go:200 | stage=pre_consume_start model=wan2.7-t2v
[BILLING_DEBUG] 2026/06/16 - 10:35:00 | req-xyz789 | price.go:250 | stage=pre_consume_calculated model_price=0.082 quota=41000 group_ratio=1
[BILLING_DEBUG] 2026/06/16 - 10:35:00 | req-xyz789 | billing_session.go:280 | stage=session_create user_id=123 pre_consumed_quota=41000 billing_preference=wallet_first
...（任务提交）...
[BILLING_DEBUG] 2026/06/16 - 10:35:01 | req-xyz789 | task_billing.go:200 | stage=task_submit task_id=task_789 quota=41000 model_price=0.082
...（轮询阶段，任务完成）...
[BILLING_DEBUG] 2026/06/16 - 10:40:00 | SYSTEM | task_polling.go:400 | stage=task_settle_on_complete task_id=task_789 status=SUCCESS total_tokens=0
[BILLING_DEBUG] 2026/06/16 - 10:40:00 | SYSTEM | task_polling.go:410 | stage=task_adaptor_result task_id=task_789 actual_quota=61500
[BILLING_DEBUG] 2026/06/16 - 10:40:00 | SYSTEM | task_billing.go:180 | stage=task_recalculate task_id=task_789 actual_quota=61500 pre_consumed=41000 delta=20500 reason=adaptor计费调整
[BILLING_DEBUG] 2026/06/16 - 10:40:00 | SYSTEM | task_billing.go:100 | stage=task_funding_adjusted task_id=task_789 billing_source=wallet delta=20500 error=<nil>
[BILLING_DEBUG] 2026/06/16 - 10:40:00 | SYSTEM | funding_source.go:65 | stage=wallet_settle user_id=123 delta=20500
```

---

## 5. 配置界面设计

### 5.1 前端配置页面

**路径**：系统设置 → 计费设置 → 调试日志

**UI 组件**：
- 开关（Switch）：启用/禁用计费调试日志
- 说明文字：启用后将输出详细的计费计算日志，包含源文件和行号信息

### 5.2 数据库存储

**表**：`configs`

| key | value | 说明 |
|-----|-------|------|
| `billing_debug` | `{"enabled": true}` | 计费调试日志配置 |

---

## 6. 性能考虑

### 6.1 开关关闭时的开销

`IsBillingDebugEnabled()` 内部读取数据库配置或 atomic 变量，开销极小。

### 6.2 日志输出性能

- 使用 `gin.DefaultWriter` 输出，与系统日志共用 writer 和日志文件
- 通过 `common.LogWriterMu` 保护并发写入
- 生产环境建议关闭，仅在调试时开启

### 6.3 敏感信息处理

- 不输出用户额度、令牌 Key 等敏感信息
- 仅输出计算相关的数值和状态

---

## 7. 实施步骤

### 第一步：创建配置和工具函数（1小时）

1. 创建 `setting/billing_setting/billing_debug.go`
2. 创建 `logger/billing_debug.go`

### 第二步：埋点预扣费阶段（2小时）

1. `relay/helper/price.go` → `ModelPriceHelper`
2. `relay/helper/price.go` → `modelPriceHelperTiered`
3. `relay/helper/price.go` → `ModelPriceHelperPerCall`

### 第三步：埋点计费会话（2小时）

1. `service/billing_session.go` → `NewBillingSession`
2. `service/billing_session.go` → `Settle`
3. `service/billing_session.go` → `Refund`

### 第四步：埋点结算阶段（2小时）

1. `service/billing.go` → `SettleBilling`
2. `service/text_quota.go` → `PostTextConsumeQuota`

### 第五步：埋点异步任务（2小时）

1. `service/task_billing.go` → `RecalculateTaskQuota`
2. `service/task_polling.go` → `settleTaskBillingOnComplete`

### 第六步：埋点资金来源和数据库（1小时）

1. `service/funding_source.go`
2. `model/user.go`（相关函数）

### 第七步：测试和文档（1小时）

1. 单元测试验证开关控制
2. 集成测试验证完整日志输出
3. 更新文档

---

## 8. 扩展建议

### 8.1 日志级别

可以进一步细分为：
- `BILLING_DEBUG`：详细调试信息
- `BILLING_INFO`：关键计费事件（预扣费、结算、退款）
- `BILLING_WARN`：异常情况（退款失败、差额过大）

### 8.2 日志聚合

可以将计费日志发送到 ELK、Grafana 等系统进行聚合分析：
- 统计每个模型的计费分布
- 监控差额结算的频率和金额
- 检测异常计费模式
