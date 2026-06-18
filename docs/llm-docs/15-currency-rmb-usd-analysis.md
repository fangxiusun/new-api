# 人民币与美元逻辑关系 — 深度分析文档

> 本文档深入分析系统中钱包、支付、充值、扣费涉及的所有货币逻辑，
> 理清人民币（CNY）和美元（USD）在系统内部的完整关系链。

---

## 1. 核心概念：三层货币体系

系统存在三层货币概念，理解它们的区别是理解整个系统的前提。

### 1.1 内部计量单位：Quota（额度点数）

**定义位置**：`common/constants.go`

```go
var QuotaPerUnit = 500 * 1000.0 // 500,000
```

- **Quota 是系统内部唯一的计量单位**，所有余额、扣费、充值都以 Quota 存储和计算
- 换算关系：**500,000 Quota = $1 USD**
- 数据库中用户余额（`users.quota`）、令牌余额（`tokens.remain_quota`）、充值记录（`topups.amount`）都以 Quota 为单位

### 1.2 展示货币：Display Currency（纯前端转换）

**定义位置**：`setting/operation_setting/general_setting.go`

```go
const (
    QuotaDisplayTypeUSD    = "USD"
    QuotaDisplayTypeCNY    = "CNY"
    QuotaDisplayTypeTokens = "TOKENS"
    QuotaDisplayTypeCustom = "CUSTOM"
)
```

- 展示货币**仅影响前端显示**，不影响任何后端计算和存储
- 转换公式：`展示金额 = Quota ÷ QuotaPerUnit × usdExchangeRate`
- 后端日志（`logger.FormatQuota`）也会跟随此配置切换显示

### 1.3 支付货币：用户实际支付金额

- 用户实际支付的金额，由 `Price`（易支付）或 `StripeUnitPrice`（Stripe）决定
- 与展示货币**可以不同**

---

## 2. 关键配置变量一览

| 变量 | 文件 | 默认值 | 含义 |
|------|------|--------|------|
| `QuotaPerUnit` | `common/constants.go` | 500,000 | 1 USD = 500,000 Quota（代码硬编码） |
| `USDExchangeRate` | `operation_setting/payment_setting_old.go` | 7.3 | 1 USD = ? 展示货币（前端可配置） |
| `Price` | `operation_setting/payment_setting_old.go` | 7.3 | 充值 1 USD 信用额需支付的金额（易支付） |
| `StripeUnitPrice` | `setting/payment_stripe.go` | 8.0 | 充值 1 USD 信用额需支付的美元（Stripe） |
| `USD2RMB` | `setting/ratio_setting/model_ratio.go` | 7.3 | 模型默认比率中 RMB→USD 的换算常量（代码硬编码） |

### 2.1 关键区分：`Price` vs `USDExchangeRate`

这两个变量默认值都是 7.3，但含义完全不同：

| | `Price` | `USDExchangeRate` |
|---|---------|-------------------|
| **用途** | 充值定价（用户实际付多少钱买 1 USD 信用额） | 展示换算（1 USD 显示为多少展示货币） |
| **影响范围** | 用户实际支付金额 | 前端显示的数字 |
| **可独立修改** | ✅ 前端可配 | ✅ 前端可配 |
| **举例** | Price=5 表示充 $1 只需付 ¥5 | Rate=7.3 表示 $1 显示为 ¥7.30 |

**两者的比值决定了充值的"折扣率"**：
- `Price / USDExchangeRate = 1` → 无折扣，充多少显示多少
- `Price / USDExchangeRate < 1` → 有折扣，实际支付 < 显示金额
- `Price / USDExchangeRate > 1` → 溢价，实际支付 > 显示金额

---

## 3. 问题 1：前端如何实现人民币显示？

### 3.1 配置方式

管理员在 **系统设置 → 运营设置** 中设置：
- `quota_display_type` = `"CNY"`
- `usd_exchange_rate` = `7.3`（可调整）

### 3.2 后端返回数据

**文件**：`controller/misc.go`

```go
"quota_per_unit":    common.QuotaPerUnit,                    // 500000
"quota_display_type": operation_setting.GetQuotaDisplayType(), // "CNY"
"usd_exchange_rate": operation_setting.USDExchangeRate,         // 7.3
"price":             operation_setting.Price,                   // 7.3
```

### 3.3 前端转换逻辑

**文件**：`web/default/src/lib/currency.ts`

核心函数 `formatCurrencyFromUSD(amountUSD)`：

```
1. 读取配置：quotaDisplayType="CNY", usdExchangeRate=7.3
2. 构建 DisplayMeta: { kind: "currency", symbol: "¥", exchangeRate: 7.3 }
3. value = amountUSD × 7.3
4. 返回 "¥{value}"
```

**Quota → 展示金额的完整链路**：

```
数据库 Quota → ÷ QuotaPerUnit(500,000) → USD → × usdExchangeRate(7.3) → 展示金额
例: 3,650,000 Quota → ÷ 500,000 = $7.30 → × 7.3 = ¥53.29
```

### 3.4 结论

**✅ 只需配置即可**。将 `quota_display_type` 设为 `"CNY"`，系统自动完成：
- 前端所有金额展示转换为 ¥
- 后端日志（`FormatQuota`）转换为 ¥
- 模型定价页面转换为 ¥
- 无需修改任何代码

---

## 4. 问题 2：人民币充值是否所见即所得？

### 4.1 充值完整数据流

以 **易支付 + CNY 展示 + Price=7.3 + USDExchangeRate=7.3** 为例：

```
┌─────────────┐     ┌──────────────┐     ┌──────────────┐     ┌──────────┐
│  前端预设    │     │  前端显示     │     │  后端计算     │     │  用户体验  │
│ amount=100  │ ──→ │ ¥100         │ ──→ │              │     │          │
│ (USD单位)   │     │ (×7.3显示)   │     │              │     │          │
└─────────────┘     └──────────────┘     └──────────────┘     └──────────┘

1. 管理员配置 amount_options = [1, 5, 10, 50, 100]（单位：USD）
2. 前端显示：formatCurrencyFromUSD(100) = "¥730"（100 × 7.3）
3. 用户看到预设按钮 "¥730"，点击选择
4. 前端传 amount=100 给后端（仍是 USD 单位）
5. 后端 getPayMoney(100) = 100 × Price(7.3) × discount = ¥730
6. 前端显示 "需支付 ¥730"
7. 用户支付 ¥730
8. 回调成功：quotaToAdd = 100 × 500,000 = 50,000,000 Quota
9. 用户余额增加 50,000,000 Quota = $100 USD
10. 前端显示余额增加：$100 × 7.3 = ¥730
```

**当 Price = USDExchangeRate = 7.3 时**：
- 预设显示 ¥730 → 实际支付 ¥730 → 余额增加 ¥730
- ✅ **所见即所得**

### 4.2 当 Price ≠ USDExchangeRate 时

**场景：Price=5, USDExchangeRate=7.3**

```
1. 预设 amount=100，前端显示 "¥730"（100 × 7.3）
2. 后端 getPayMoney(100) = 100 × 5 = ¥500
3. 前端显示 "需支付 ¥500"
4. 用户支付 ¥500
5. 余额增加 $100 = ¥730
```

- 预设显示 ¥730，但实际只需付 ¥500
- 用户付 ¥500 获得 ¥730 的余额
- ⚠️ **不是所见即所得**，但对用户有利（有折扣）

**场景：Price=10, USDExchangeRate=7.3**

```
1. 预设 amount=100，前端显示 "¥730"（100 × 7.3）
2. 后端 getPayMoney(100) = 100 × 10 = ¥1000
3. 前端显示 "需支付 ¥1000"
4. 用户支付 ¥1000
5. 余额增加 $100 = ¥730
```

- ⚠️ **不是所见即所得**，用户付 ¥1000 只获得 ¥730 的余额

### 4.3 结论

**✅ 当 Price = USDExchangeRate 时（默认都是 7.3），充值是所见即所得的。**

管理员可通过独立调整这两个参数实现不同的充值策略：

| Price | USDExchangeRate | 效果 |
|-------|-----------------|------|
| 7.3 | 7.3 | 1:1 充值，所见即所得 |
| 5 | 7.3 | 充值折扣，充 ¥730 只需付 ¥500 |
| 10 | 7.3 | 充值溢价，充 ¥730 需付 ¥1000 |

---

## 5. 问题 3：模型价格能否直接配置人民币金额？

### 5.1 当前模型价格体系

系统有**两种定价模式**：

#### 模式 A：倍率定价（model_ratio）

```go
// 实际扣费 = token数 × modelRatio × completionRatio × groupRatio
// 单位：USD / 1K tokens
"gpt-4o": 5,           // $10/1M tokens → ratio=5
"claude-3-opus": 7.5,  // $15/1M tokens → ratio=7.5
```

扣费公式（`service/text_quota.go`）：
```
quota = (promptTokens × modelRatio + completionTokens × modelRatio × completionRatio) × groupRatio
```

#### 模式 B：固定价格（model_price）

```go
// 按次计费，每次请求固定价格
"dall-e-3": 0.04,      // $0.04 / 次
"suno_music": 0.1,     // $0.10 / 次
```

扣费公式（`relay/helper/price.go`）：
```
quota = modelPrice × QuotaPerUnit × groupRatio
```

### 5.2 默认模型价格中的 RMB 处理

**文件**：`setting/ratio_setting/model_ratio.go`

```go
const (
    USD2RMB = 7.3   // 硬编码
    USD     = 500    // $1 = 500 ratio units
    RMB     = USD / USD2RMB  // ≈ 68.49
)
```

部分国内模型使用 RMB 定价：

```go
"ERNIE-4.0-8K":    0.120 * RMB,  // ￥0.12/1K tokens → ratio ≈ 8.22
"ERNIE-3.5-8K":    0.012 * RMB,  // ￥0.012/1K tokens → ratio ≈ 0.82
"glm-4v":          0.05 * RMB,   // ￥0.05/1K tokens → ratio ≈ 3.42
"qwen-turbo":      0.8572,       // ￥0.012/1K tokens（直接换算好）
```

**这里的 `* RMB` 是代码编写时的一次性换算**，不是运行时动态转换。

### 5.3 管理员如何配置模型价格？

管理员通过 **系统设置 → 分组与模型定价** 的 JSON 编辑器配置：

```json
{
  "model_price": {
    "my-custom-model": 0.05
  },
  "model_ratio": {
    "my-custom-model": 2.5
  }
}
```

**这些值的单位始终是 USD**：
- `model_price`: USD/次（固定价格模式）
- `model_ratio`: USD/1K tokens 的比例系数

### 5.4 能否直接配置人民币金额？

**❌ 当前不能直接配置人民币金额**。

原因：
1. 后端 `model_price` 和 `model_ratio` 的存储和计算**始终以 USD 为单位**
2. 前端 JSON 编辑器直接写入值，不做货币转换
3. 计费引擎（`relay/helper/price.go`、`service/text_quota.go`）直接使用这些值乘以 `QuotaPerUnit`

**如果管理员想配置人民币价格，需要手动换算**：

```
USD价格 = RMB价格 ÷ 7.3
```

例如：想设置某模型 ￥0.10/1K tokens：
```
model_ratio = 0.10 ÷ 7.3 × 500 ≈ 6.85
```

或使用固定价格模式，设置 $0.0137/次：
```json
{ "model_price": { "my-model": 0.0137 } }
```

### 5.5 前端定价页面的展示

**文件**：`web/default/src/features/pricing/lib/price.ts`

定价页面的展示逻辑：

```ts
// 基础价格（USD）
priceInUSD = calculateTokenPrice(model, type, groupRatio)

// 展示价格
formatCurrencyFromUSD(priceInUSD / tokenUnitDivisor)
// → CNY模式下自动 × 7.3 显示为 ¥
```

所以管理员在 CNY 模式下看到的模型价格已经是 ¥，但配置时仍需输入 USD 值。

---

## 6. 完整数据流图

### 6.1 充值流

```
用户选择"充 ¥X"
    │
    ├─ 前端: amount = X ÷ usdExchangeRate (转回 USD)
    │   实际上 presetValue 本身就是 USD，X = presetValue × usdExchangeRate
    │
    ├─ 后端 getPayMoney(amount):
    │   payMoney = amount × Price × topupGroupRatio × discount
    │   (易支付: payMoney 单位是人民币元)
    │
    ├─ 用户支付 payMoney 元
    │
    └─ 回调成功:
        quotaToAdd = amount × QuotaPerUnit
        用户余额 += quotaToAdd
```

### 6.2 计费流

```
请求到达
    │
    ├─ 预扣费:
    │   modelPrice(USD) × QuotaPerUnit × groupRatio = preQuota
    │   用户余额 -= preQuota
    │
    ├─ 实际计费:
    │   token数 × modelRatio × completionRatio × groupRatio = actualQuota
    │   或 modelPrice × QuotaPerUnit × groupRatio = actualQuota
    │
    └─ 结算:
        delta = actualQuota - preQuota
        用户余额 += delta (正数补扣，负数退还)
```

### 6.3 展示流

```
Quota 值 (数据库)
    │
    ├─ ÷ QuotaPerUnit = USD 金额
    │
    ├─ × usdExchangeRate = 展示金额
    │
    └─ 加货币符号: ¥ / $ / ¤ / Tokens
```

---

## 7. 汇率设置为 1 的影响分析

如果将 `usd_exchange_rate` 设置为 `1`：

| 场景 | 效果 |
|------|------|
| 余额显示 | $100 显示为 "$100" → CNY模式下显示为 "¥100"（实际是 $100 的购买力） |
| 充值显示 | 预设 amount=100 显示为 "¥100" |
| 充值支付 | getPayMoney(100) = 100 × Price(7.3) = ¥730 |
| 模型定价 | $0.002/1K tokens 显示为 "¥0.002"（实际价值 ¥0.0146） |
| 日志 | FormatQuota 将 $100 格式化为 "¥100" |

**结论**：设置 `usd_exchange_rate=1` 会导致**显示金额与实际价值严重不匹配**，不建议这样做。

如果同时将 `Price` 也改为 `1`：
- 充 ¥100 只需付 ¥1
- 但余额显示 ¥100 实际只有 $1 的购买力
- 系统不会报错，但会造成用户理解混乱

---

## 8. 各支付渠道对比

| 渠道 | amount 单位 | 充值公式 | 实际到账 |
|------|------------|---------|---------|
| 易支付 | 展示货币(USD) | amount × Price | amount × QuotaPerUnit |
| Stripe | 展示货币(USD) | amount × StripeUnitPrice | Money × QuotaPerUnit |
| Waffo | 展示货币(USD) | — | amount × QuotaPerUnit |
| Waffo Pancake | 展示货币(USD) | amount × Price × Ratio | amount × QuotaPerUnit |
| Creem | 直接额度 | 产品价格 | Amount(直接是 Quota) |
| 兑换码 | — | — | 码面值(Quota) |

---

## 9. 总结

| 问题 | 结论 |
|------|------|
| 前端人民币展示 | ✅ 只需配置 `quota_display_type="CNY"` 即可，无需改代码 |
| 充值所见即所得 | ✅ 当 `Price = USDExchangeRate` 时（默认均为 7.3），所见即所得 |
| 直接配置人民币价格 | ❌ 不能。模型价格始终以 USD 为单位存储和计算，需手动 ÷ 7.3 换算 |
