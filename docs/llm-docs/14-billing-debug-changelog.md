# 计费调试日志实现 — 变更说明

> 为计费系统增加独立调试日志开关，详细记录计费计算的每一个细节。
> 不修改数据库，通过环境变量 `BILLING_DEBUG` 或运行时 POST API 控制开关。

---

## 1. 变更概览

| 维度 | 说明 |
|------|------|
| 变更类型 | 新增功能（日志埋点） |
| 影响范围 | 计费全链路（预扣费 → 结算 → 退款） |
| 数据库变更 | 无 |
| 配置变更 | 新增环境变量 `BILLING_DEBUG` |
| 性能影响 | 开关关闭时约 1ns/次（atomic load），可忽略 |
| 兼容性 | 完全向后兼容，开关默认关闭 |

---

## 2. 新增文件（3 个）

### 2.1 `common/billing_debug.go`
- **职责**：计费调试日志开关（`atomic.Int32`）
- **关键函数**：
  - `IsBillingDebugEnabled() bool` — 读取开关状态
  - `SetBillingDebugEnabled(enabled bool)` — 运行时热切换
- **初始化**：`init()` 从环境变量 `BILLING_DEBUG=true` 或 `BILLING_DEBUG=1` 读取初始值

### 2.2 `logger/billing_debug.go`
- **职责**：计费调试日志输出函数
- **关键函数**：
  - `BillingDebugMap(ctx *gin.Context, fields map[string]interface{})` — HTTP handler 中使用
  - `BillingDebugMapWithCtx(ctx context.Context, fields map[string]interface{})` — 异步任务中使用
  - `BillingDebugf(ctx *gin.Context, format string, args ...interface{})` — 格式化字符串版本
  - `BillingDebugfWithCtx(ctx context.Context, format string, args ...interface{})` — 异步格式化版本
- **日志格式**：`[BILLING_DEBUG] 2006/01/02 - 15:04:05 | request-id | file.go:42 | key1=val1 key2=val2`
- **输出目标**：与系统日志共用 `gin.DefaultWriter`，通过 `common.LogWriterMu` 保护并发写入

### 2.3 `controller/billing_debug.go`
- **职责**：计费调试日志开关的 API 接口
- **接口**：
  - `GetBillingDebug(c *gin.Context)` — GET 查询开关状态
  - `SetBillingDebug(c *gin.Context)` — POST 设置开关状态

---

## 3. 修改文件（9 个）

### 3.1 `router/api-router.go`
新增路由组 `/api/debug/billing`（需管理员权限）：
```
GET  /api/debug/billing  → GetBillingDebug
POST /api/debug/billing  → SetBillingDebug  body: {"enabled": true/false}
```

### 3.2 `relay/helper/price.go`
在价格计算的三个核心函数中添加埋点：

| 函数 | stage 标识 | 说明 |
|------|-----------|------|
| `ModelPriceHelper` | `pre_consume_start` | 预扣费开始，记录模型和 token 信息 |
| `ModelPriceHelper` | `ratio_loaded` | 倍率加载完成（仅 usePrice 模式） |
| `ModelPriceHelper` | `pre_consume_calculated` | 预扣费额度计算完成 |
| `ModelPriceHelperPerCall` | `per_call_start` | 按次计费开始 |
| `ModelPriceHelperPerCall` | `per_call_calculated` | 按次计费额度计算完成 |
| `modelPriceHelperTiered` | `tiered_expr_start` | 阶梯表达式计算开始 |
| `modelPriceHelperTiered` | `tiered_expr_result` | 阶梯表达式计算结果 |
| `modelPriceHelperTiered` | `tiered_pre_consume_calculated` | 阶梯预扣费计算完成 |

### 3.3 `service/billing_session.go`
在计费会话生命周期的关键节点添加埋点：

| 方法 | stage 标识 | 说明 |
|------|-----------|------|
| `NewBillingSession` | `session_create` | 会话创建，记录用户、偏好、模型 |
| `preConsume` | `pre_consume_executed` | 预扣费执行完成 |
| `Settle` | `settle_start` | 结算开始，记录预扣/实际/资金来源 |
| `Settle` | `settle_funding` | 资金来源结算开始 |
| `Settle` | `settle_funding_error` | 资金来源结算失败 |
| `Settle` | `settle_funding_done` | 资金来源结算完成 |
| `Refund` | `refund_start` | 退款开始 |

### 3.4 `service/billing.go`
| 函数 | stage 标识 | 说明 |
|------|-----------|------|
| `SettleBilling` | `settle_billing_start` | 计费结算入口 |

### 3.5 `service/text_quota.go`
| 函数 | stage 标识 | 说明 |
|------|-----------|------|
| `PostTextConsumeQuota` | `text_quota_calculated` | 文本计费汇总（含所有 token 类型和倍率） |
| `PostTextConsumeQuota` | `tiered_settle_applied` | 阶梯计费结算已应用 |

### 3.6 `service/task_billing.go`
| 函数 | stage 标识 | 说明 |
|------|-----------|------|
| `RecalculateTaskQuota` | `task_recalculate` | 异步任务差额结算 |

### 3.7 `service/task_polling.go`
| 函数 | stage 标识 | 说明 |
|------|-----------|------|
| `settleTaskBillingOnComplete` | `task_settle_on_complete` | 任务完成时的计费调整 |

### 3.8 `service/funding_source.go`
在钱包和订阅两种资金来源的所有操作中添加埋点：

| 方法 | stage 标识 | 说明 |
|------|-----------|------|
| `WalletFunding.PreConsume` | `wallet_pre_consume` | 钱包预扣费 |
| `WalletFunding.Settle` | `wallet_settle` | 钱包结算 |
| `WalletFunding.Refund` | `wallet_refund` | 钱包退款 |
| `SubscriptionFunding.PreConsume` | `subscription_pre_consume` | 订阅预扣费 |
| `SubscriptionFunding.Settle` | `subscription_settle` | 订阅结算 |
| `SubscriptionFunding.Refund` | `subscription_refund` | 订阅退款 |

### 3.9 `service/quota.go`
| 函数 | stage 标识 | 说明 |
|------|-----------|------|
| `PostConsumeQuota` | `post_consume_quota` | 旧路径计费（无 BillingSession 时的回退） |
| `PreWssConsumeQuota` | `wss_pre_consume_calculated` | WebSocket 实时音频计费 |

### 3.10 `service/pre_consume_quota.go`
| 函数 | stage 标识 | 说明 |
|------|-----------|------|
| `ReturnPreConsumedQuota` | `return_pre_consumed` | 旧路径预扣费退还 |

### 3.11 `service/violation_fee.go`
| 函数 | stage 标识 | 说明 |
|------|-----------|------|
| `ChargeViolationFeeIfNeeded` | `violation_fee_start` | 违规扣费开始 |
| `ChargeViolationFeeIfNeeded` | `violation_fee_error` | 违规扣费失败 |
| `ChargeViolationFeeIfNeeded` | `violation_fee_charged` | 违规扣费成功 |

---

## 4. 计费链路完整覆盖

### 4.1 文本请求（同步计费）

```
请求到达
  │
  ├─ ModelPriceHelper         → pre_consume_start / ratio_loaded / pre_consume_calculated
  ├─ PreConsumeBilling        → session_create / pre_consume_executed
  │   └─ WalletFunding/SubscriptionFunding.PreConsume → wallet_pre_consume / subscription_pre_consume
  │
  ├─ [上游请求处理]
  │
  ├─ PostTextConsumeQuota     → text_quota_calculated / tiered_settle_applied
  └─ SettleBilling            → settle_billing_start
      └─ BillingSession.Settle → settle_start / settle_funding / settle_funding_done
          └─ WalletFunding/SubscriptionFunding.Settle → wallet_settle / subscription_settle
```

### 4.2 视频/图片任务（两阶段计费）

```
任务提交
  │
  ├─ ModelPriceHelperPerCall  → per_call_start / per_call_calculated
  ├─ PreConsumeBilling        → session_create / pre_consume_executed
  │
  ├─ [任务轮询等待]
  │
  └─ settleTaskBillingOnComplete → task_settle_on_complete
      └─ RecalculateTaskQuota    → task_recalculate
          └─ taskAdjustFunding   → wallet_settle / subscription_settle
```

### 4.3 请求失败退款

```
请求失败
  ├─ BillingSession.Refund    → refund_start
  │   └─ WalletFunding/SubscriptionFunding.Refund → wallet_refund / subscription_refund
  │
  └─ [旧路径] ReturnPreConsumedQuota → return_pre_consumed
      └─ PostConsumeQuota             → post_consume_quota
```

### 4.4 WebSocket 实时音频

```
WebSocket 连接
  └─ PreWssConsumeQuota       → wss_pre_consume_calculated
      └─ PostConsumeQuota     → post_consume_quota
```

### 4.5 违规扣费

```
请求返回违规标记
  └─ ChargeViolationFeeIfNeeded → violation_fee_start / violation_fee_charged / violation_fee_error
      └─ PostConsumeQuota       → post_consume_quota
```

---

## 5. 使用方式

### 5.1 启动时开启
```bash
BILLING_DEBUG=true ./new-api
# 或
BILLING_DEBUG=1 ./new-api
```

### 5.2 运行时切换
```bash
# 开启
curl -X POST http://localhost:3000/api/debug/billing \
  -H "Authorization: Bearer <admin-token>" \
  -H "Content-Type: application/json" \
  -d '{"enabled": true}'

# 关闭
curl -X POST http://localhost:3000/api/debug/billing \
  -H "Authorization: Bearer <admin-token>" \
  -H "Content-Type: application/json" \
  -d '{"enabled": false}'

# 查询状态
curl http://localhost:3000/api/debug/billing \
  -H "Authorization: Bearer <admin-token>"
```

### 5.3 日志输出示例

```
[BILLING_DEBUG] 2026/06/16 - 10:30:15 | req-abc123 | price.go:71 | stage=pre_consume_start model=gpt-4o prompt_tokens=1500 max_tokens=500
[BILLING_DEBUG] 2026/06/16 - 10:30:15 | req-abc123 | price.go:130 | stage=ratio_loaded use_price=true model_price=0.005 group_ratio=1
[BILLING_DEBUG] 2026/06/16 - 10:30:15 | req-abc123 | price.go:184 | stage=pre_consume_calculated pre_consumed_quota=2500 free_model=false
[BILLING_DEBUG] 2026/06/16 - 10:30:15 | req-abc123 | billing_session.go:382 | stage=session_create user_id=42 pre_consumed_quota=2500 billing_preference=wallet_first model=gpt-4o
[BILLING_DEBUG] 2026/06/16 - 10:30:15 | req-abc123 | funding_source.go:39 | stage=wallet_pre_consume user_id=42 amount=2500
[BILLING_DEBUG] 2026/06/16 - 10:30:15 | req-abc123 | billing_session.go:267 | stage=pre_consume_executed user_id=42 effective_quota=2500 funding_source=wallet trusted=false
[BILLING_DEBUG] 2026/06/16 - 10:30:16 | req-abc123 | text_quota.go:333 | stage=text_quota_calculated model=gpt-4o prompt_tokens=1500 completion_tokens=800 cache_tokens=0 image_tokens=0 audio_tokens=0 model_ratio=5 completion_ratio=1 cache_ratio=0.5 group_ratio=1 model_price=0 quota=843750
[BILLING_DEBUG] 2026/06/16 - 10:30:16 | req-abc123 | billing.go:36 | stage=settle_billing_start actual_quota=843750
[BILLING_DEBUG] 2026/06/16 - 10:30:16 | req-abc123 | billing_session.go:50 | stage=settle_start actual_quota=843750 pre_consumed=2500 funding_source=wallet funding_settled=false token_consumed=0
[BILLING_DEBUG] 2026/06/16 - 10:30:16 | req-abc123 | billing_session.go:65 | stage=settle_funding funding_source=wallet delta=841250
[BILLING_DEBUG] 2026/06/16 - 10:30:16 | req-abc123 | funding_source.go:55 | stage=wallet_settle user_id=42 delta=841250
[BILLING_DEBUG] 2026/06/16 - 10:30:16 | req-abc123 | billing_session.go:80 | stage=settle_funding_done funding_source=wallet delta=841250
```

---

## 6. stage 标识速查表

| stage | 文件 | 说明 |
|-------|------|------|
| `pre_consume_start` | `relay/helper/price.go` | 预扣费价格计算开始 |
| `ratio_loaded` | `relay/helper/price.go` | 倍率/价格加载完成 |
| `pre_consume_calculated` | `relay/helper/price.go` | 预扣费额度计算完成 |
| `per_call_start` | `relay/helper/price.go` | 按次计费开始 |
| `per_call_calculated` | `relay/helper/price.go` | 按次计费额度计算完成 |
| `tiered_expr_start` | `relay/helper/price.go` | 阶梯表达式计算开始 |
| `tiered_expr_result` | `relay/helper/price.go` | 阶梯表达式计算结果 |
| `tiered_pre_consume_calculated` | `relay/helper/price.go` | 阶梯预扣费计算完成 |
| `session_create` | `service/billing_session.go` | 计费会话创建 |
| `pre_consume_executed` | `service/billing_session.go` | 预扣费执行完成 |
| `settle_start` | `service/billing_session.go` | 结算开始 |
| `settle_funding` | `service/billing_session.go` | 资金来源结算开始 |
| `settle_funding_error` | `service/billing_session.go` | 资金来源结算失败 |
| `settle_funding_done` | `service/billing_session.go` | 资金来源结算完成 |
| `refund_start` | `service/billing_session.go` | 退款开始 |
| `settle_billing_start` | `service/billing.go` | 计费结算入口 |
| `text_quota_calculated` | `service/text_quota.go` | 文本计费汇总 |
| `tiered_settle_applied` | `service/text_quota.go` | 阶梯计费结算已应用 |
| `task_recalculate` | `service/task_billing.go` | 异步任务差额结算 |
| `task_settle_on_complete` | `service/task_polling.go` | 任务完成计费调整 |
| `wallet_pre_consume` | `service/funding_source.go` | 钱包预扣费 |
| `wallet_settle` | `service/funding_source.go` | 钱包结算 |
| `wallet_refund` | `service/funding_source.go` | 钱包退款 |
| `subscription_pre_consume` | `service/funding_source.go` | 订阅预扣费 |
| `subscription_settle` | `service/funding_source.go` | 订阅结算 |
| `subscription_refund` | `service/funding_source.go` | 订阅退款 |
| `post_consume_quota` | `service/quota.go` | 旧路径计费 |
| `wss_pre_consume_calculated` | `service/quota.go` | WebSocket 实时音频计费 |
| `return_pre_consumed` | `service/pre_consume_quota.go` | 旧路径预扣费退还 |
| `violation_fee_start` | `service/violation_fee.go` | 违规扣费开始 |
| `violation_fee_error` | `service/violation_fee.go` | 违规扣费失败 |
| `violation_fee_charged` | `service/violation_fee.go` | 违规扣费成功 |

---

## 7. 变更统计

| 类型 | 数量 |
|------|------|
| 新增文件 | 3 |
| 修改文件 | 9 |
| 新增代码行 | ~232 |
| 埋点位置 | 30 |
| stage 标识 | 30 |
| 数据库变更 | 0 |
| 配置表变更 | 0 |
