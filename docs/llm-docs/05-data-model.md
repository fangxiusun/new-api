# New API — 数据模型说明

> 生成时间：2026-06-16
> 追踪方式：基于 model/*.go 源码逐结构体阅读
> 所有模型均通过 GORM AutoMigrate 自动建表，无独立迁移文件

---

## 1. 数据库类型

| 数据库 | 驱动 | 连接方式 | 默认？ |
|---|---|---|---|
| **SQLite** | `glebarez/sqlite` | `SQLITE_PATH` 环境变量（默认 `one-api.db`） | 是 |
| **MySQL** | `gorm.io/driver/mysql` | `SQL_DSN=mysql://...` | 否 |
| **PostgreSQL** | `gorm.io/driver/postgres` | `SQL_DSN=postgres://...` | 否 |

数据库类型通过 `SQL_DSN` 前缀自动判断（`model/main.go:chooseDB()`）：
- `postgres://` / `postgresql://` → PostgreSQL
- `local` → SQLite
- 其他非空值 → MySQL
- 空值 → SQLite

**日志数据库**：支持通过 `LOG_SQL_DSN` 配置独立的日志数据库，类型可以与主库不同。日志表（`logs`、`quota_data` 等）写入 `LOG_DB`，其他表写入 `DB`。

**跨库兼容标志**（`model/main.go`）：
```go
var commonGroupCol string   // PostgreSQL: "group", MySQL/SQLite: `group`
var commonKeyCol string     // PostgreSQL: "key",   MySQL/SQLite: `key`
var commonTrueVal string    // PostgreSQL: "true",  MySQL/SQLite: "1"
var commonFalseVal string   // PostgreSQL: "false", MySQL/SQLite: "0"
```

---

## 2. ORM / SQL / Migration 使用方式

### ORM：GORM v2

所有数据库操作通过 GORM v2 进行，主要模式：

| 模式 | 使用场景 | 示例 |
|---|---|---|
| **GORM 链式 API** | 常规 CRUD | `DB.Where(...).Find(&users)` |
| **原生 SQL** | 复杂查询/统计 | `DB.Raw(fmt.Sprintf("SELECT %s ...", commonGroupCol))` |
| **Upsert** | 性能指标写入 | `DB.Clauses(clause.OnConflict{...}).Create(...)` |
| **事务** | 充值/兑换码 | `DB.Begin()` / `tx.Commit()` |

### Migration：GORM AutoMigrate

**文件**：`model/main.go:InitDB()`

```go
DB.AutoMigrate(
    &User{}, &Channel{}, &Token{}, &Option{}, &Log{},
    &Redemption{}, &Midjourney{}, &Ability{}, &Pricing{},
    &TopUp{}, &Checkin{}, &Task{}, &Setup{}, &QuotaData{},
    &PasskeyCredential{}, &TwoFA{}, &TwoFABackupCode{},
    &SubscriptionPlan{}, &SubscriptionOrder{}, &UserSubscription{},
    &SubscriptionPreConsumeRecord{},
    &Vendor{}, &Model{}, &PrefillGroup{}, &PerfMetric{},
    &CustomOAuthProvider{}, &UserOAuthBinding{},
)
```

特点：
- **无版本化迁移**：所有 schema 变更通过 AutoMigrate 自动执行
- **仅追加列**：AutoMigrate 只会添加新列/索引，不会删除列或修改列类型
- **SQLite 列迁移受限**：通过 `sqliteColumnDef` 辅助结构实现 `ALTER TABLE ... ADD COLUMN`
- **MySQL 字符集检查**：启动时 `checkMySQLChineseSupport()` 校验字符集支持中文
- **decimal 迁移**：`migrateColumnToDecimal()` 辅助函数处理 float → decimal 的列类型迁移

---

## 3. 主要实体/表

### 3.1 核心实体总览

| # | 表名 | 结构体 | 文件 | 说明 |
|---|---|---|---|---|
| 1 | `users` | `User` | `user.go` | 用户账户 |
| 2 | `tokens` | `Token` | `token.go` | API Token（密钥） |
| 3 | `channels` | `Channel` | `channel.go` | AI 渠道（上游提供商配置） |
| 4 | `abilities` | `Ability` | `ability.go` | 渠道-模型能力映射 |
| 5 | `logs` | `Log` | `log.go` | 使用日志（消费/充值/管理/系统/错误/退款/登录） |
| 6 | `tasks` | `Task` | `task.go` | 异步任务（视频/音乐生成等） |
| 7 | `midjourneys` | `Midjourney` | `midjourney.go` | Midjourney 图像任务 |
| 8 | `subscription_plans` | `SubscriptionPlan` | `subscription.go` | 订阅计划定义 |
| 9 | `subscription_orders` | `SubscriptionOrder` | `subscription.go` | 订阅订单 |
| 10 | `user_subscriptions` | `UserSubscription` | `subscription.go` | 用户订阅实例 |
| 11 | `subscription_pre_consume_records` | `SubscriptionPreConsumeRecord` | `subscription.go` | 订阅预扣记录 |
| 12 | `top_ups` | `TopUp` | `topup.go` | 充值记录 |
| 13 | `redemptions` | `Redemption` | `redemption.go` | 兑换码 |
| 14 | `options` | `Option` | `option.go` | 系统配置项（KV 存储） |
| 15 | `checkins` | `Checkin` | `checkin.go` | 用户签到记录 |
| 16 | `quota_data` | `QuotaData` | `usedata.go` | 用量统计数据（仪表盘） |
| 17 | `setups` | `Setup` | `setup.go` | 系统初始化状态 |
| 18 | `passkey_credentials` | `PasskeyCredential` | `passkey.go` | WebAuthn/Passkey 凭证 |
| 19 | `two_f_as` | `TwoFA` | `twofa.go` | TOTP 两步验证设置 |
| 20 | `two_f_a_backup_codes` | `TwoFABackupCode` | `twofa.go` | 2FA 备用码 |
| 21 | `models` | `Model` | `model_meta.go` | 模型元数据（名称/描述/图标/标签） |
| 22 | `vendors` | `Vendor` | `vendor_meta.go` | 供应商元数据（名称/图标） |
| 23 | `prefill_groups` | `PrefillGroup` | `prefill_group.go` | 预填充分组模板 |
| 24 | `perf_metrics` | `PerfMetric` | `perf_metric.go` | 性能指标聚合数据 |
| 25 | `custom_oauth_providers` | `CustomOAuthProvider` | `custom_oauth_provider.go` | 自定义 OAuth 提供商配置 |
| 26 | `user_oauth_bindings` | `UserOAuthBinding` | `user_oauth_binding.go` | 用户-OAuth 绑定关系 |

### 3.2 核心表字段详解

#### ① `users` — 用户表

**文件**：`model/user.go` → `type User struct`

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| `id` | int | PK, auto | 用户 ID |
| `username` | varchar | unique, index | 用户名（最大 20 字符） |
| `password` | varchar | not null | bcrypt 哈希后的密码 |
| `display_name` | varchar | index | 显示名称 |
| `role` | int | default 1 | 角色：1=普通用户, 10=管理员, 100=Root |
| `status` | int | default 1 | 状态：1=启用, 2=禁用 |
| `email` | varchar(50) | index | 邮箱地址 |
| `github_id` | varchar | index | GitHub OAuth ID |
| `discord_id` | varchar | index | Discord OAuth ID |
| `oidc_id` | varchar | index | OIDC OAuth ID |
| `wechat_id` | varchar | index | 微信 OpenID |
| `telegram_id` | varchar | index | Telegram User ID |
| `linux_do_id` | varchar | index | LinuxDo OAuth ID |
| `access_token` | char(32) | uniqueIndex | 管理用 Access Token |
| `quota` | int | default 0 | 剩余配额（单位：分） |
| `used_quota` | int | default 0 | 已使用配额 |
| `request_count` | int | default 0 | 总请求次数 |
| `group` | varchar(64) | default 'default' | 用户分组（决定可用渠道和定价） |
| `aff_code` | varchar(32) | uniqueIndex | 邀请码 |
| `aff_count` | int | default 0 | 邀请人数 |
| `aff_quota` | int | default 0 | 邀请奖励剩余配额 |
| `aff_history` | int | default 0 | 邀请奖励历史配额 |
| `inviter_id` | int | index | 邀请人用户 ID |
| `setting` | text | — | 用户个性化设置（JSON） |
| `stripe_customer` | varchar(64) | index | Stripe Customer ID |
| `remark` | varchar(255) | — | 管理员备注 |
| `created_at` | int64 | autoCreateTime | 创建时间 |
| `last_login_at` | int64 | default 0 | 最后登录时间 |
| `deleted_at` | datetime | index | 软删除时间 |

#### ② `tokens` — API Token 表

**文件**：`model/token.go` → `type Token struct`

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| `id` | int | PK, auto | Token ID |
| `user_id` | int | index | 所属用户 ID |
| `key` | varchar(128) | uniqueIndex | Token 密钥（`sk-` 前缀 + 随机串） |
| `status` | int | default 1 | 状态：1=启用, 2=禁用, 3=已过期 |
| `name` | varchar | index | Token 名称 |
| `created_time` | int64 | bigint | 创建时间 |
| `accessed_time` | int64 | bigint | 最后访问时间 |
| `expired_time` | int64 | bigint, default -1 | 过期时间（-1=永不过期） |
| `remain_quota` | int | default 0 | 剩余配额（-1=无限制） |
| `unlimited_quota` | bool | — | 是否无限制配额 |
| `model_limits_enabled` | bool | — | 是否启用模型白名单 |
| `model_limits` | text | — | 模型白名单（JSON 数组或逗号分隔） |
| `allow_ips` | varchar | default '' | IP 白名单（CIDR 格式，逗号分隔） |
| `used_quota` | int | default 0 | 已使用配额 |
| `group` | varchar | default '' | Token 分组（覆盖用户分组） |
| `cross_group_retry` | bool | — | 是否启用跨分组重试 |
| `deleted_at` | datetime | index | 软删除时间 |

#### ③ `channels` — AI 渠道表

**文件**：`model/channel.go` → `type Channel struct`

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| `id` | int | PK, auto | 渠道 ID |
| `type` | int | default 0 | 提供商类型（`constant.ChannelType*`） |
| `key` | varchar | not null | API Key（支持多 key 逗号分隔） |
| `openai_organization` | varchar | — | OpenAI Organization ID |
| `test_model` | varchar | — | 测试用模型名 |
| `status` | int | default 1 | 状态：1=启用, 2=禁用, 3=自动禁用 |
| `name` | varchar | index | 渠道名称 |
| `weight` | uint | default 0 | 权重（用于加权随机选择） |
| `created_time` | int64 | bigint | 创建时间 |
| `test_time` | int64 | bigint | 最后测试时间 |
| `response_time` | int | — | 响应时间（毫秒） |
| `base_url` | varchar | default '' | 上游 API 基地址 |
| `other` | text | — | 其他配置（如 Azure 版本等） |
| `balance` | float64 | — | 渠道余额（USD） |
| `balance_updated_time` | int64 | bigint | 余额更新时间 |
| `models` | text | — | 支持的模型列表（逗号分隔） |
| `group` | varchar(64) | default 'default' | 所属分组 |
| `used_quota` | int64 | bigint, default 0 | 已使用配额 |
| `model_mapping` | text | — | 模型名映射（JSON: `{"gpt-4": "gpt-4-0613"}`） |
| `status_code_mapping` | varchar(1024) | — | 上游状态码映射 |
| `priority` | int64 | bigint, default 0 | 优先级（越高越优先） |
| `auto_ban` | int | default 1 | 是否自动禁用（连续失败时） |
| `tag` | varchar | index | 渠道标签（用于分组管理） |
| `setting` | text | — | 渠道额外设置（JSON, `dto.ChannelSettings`） |
| `param_override` | text | — | 请求参数覆盖（JSON） |
| `header_override` | text | — | 请求头覆盖（JSON） |
| `remark` | varchar(255) | — | 管理员备注 |
| `channel_info` | JSON | — | 多 key 模式元信息（`ChannelInfo`） |
| `settings` | text | — | 其他设置（Azure 版本等，`dto.ChannelOtherSettings`） |

**嵌套结构 `ChannelInfo`**：

| 字段 | 说明 |
|---|---|
| `is_multi_key` | 是否多 key 模式 |
| `multi_key_size` | key 数量 |
| `multi_key_status_list` | key 状态映射（index → status） |
| `multi_key_disabled_reason` | key 禁用原因 |
| `multi_key_disabled_time` | key 禁用时间 |
| `multi_key_polling_index` | 轮询模式当前索引 |
| `multi_key_mode` | 多 key 模式：`polling`（轮询）或 `backup`（备用） |

#### ④ `abilities` — 渠道-模型能力映射表

**文件**：`model/ability.go` → `type Ability struct`

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| `group` | varchar(64) | **复合 PK** | 分组名 |
| `model` | varchar(255) | **复合 PK** | 模型名 |
| `channel_id` | int | **复合 PK**, index | 渠道 ID |
| `enabled` | bool | — | 是否启用 |
| `priority` | int64 | bigint, default 0, index | 优先级 |
| `weight` | uint | default 0, index | 权重 |
| `tag` | varchar | index | 标签 |

这是渠道选择的核心索引表，复合主键 `(group, model, channel_id)` 唯一确定一条能力映射。

#### ⑤ `logs` — 使用日志表

**文件**：`model/log.go` → `type Log struct`

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| `id` | int | PK, auto | 日志 ID |
| `user_id` | int | index | 用户 ID |
| `created_at` | int64 | bigint, 复合索引 | 创建时间 |
| `type` | int | 复合索引 | 日志类型（0=未知, 1=充值, 2=消费, 3=管理, 4=系统, 5=错误, 6=退款, 7=登录） |
| `content` | text | — | 日志内容 |
| `username` | varchar | index | 用户名 |
| `token_name` | varchar | index | Token 名称 |
| `model_name` | varchar(64) | index | 模型名 |
| `quota` | int | default 0 | 消耗/充值配额 |
| `prompt_tokens` | int | default 0 | Prompt Token 数 |
| `completion_tokens` | int | default 0 | Completion Token 数 |
| `use_time` | int | default 0 | 请求耗时（毫秒） |
| `is_stream` | bool | — | 是否流式请求 |
| `channel` | int | index | 渠道 ID |
| `channel_name` | varchar | — | 渠道名称（计算列，`gorm:"->"` 只读） |
| `token_id` | int | default 0, index | Token ID |
| `group` | varchar | index | 使用的分组 |
| `ip` | varchar | index | 客户端 IP |
| `request_id` | varchar(64) | index | 请求 ID |
| `upstream_request_id` | varchar(128) | index | 上游请求 ID |
| `other` | text | — | 其他信息（JSON，含详细计费参数） |

#### ⑥ `tasks` — 异步任务表

**文件**：`model/task.go` → `type Task struct`

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| `id` | int64 | PK, auto | 自增主键 |
| `created_at` | int64 | index | 创建时间 |
| `updated_at` | int64 | — | 更新时间 |
| `task_id` | varchar(191) | index | 对外任务 ID（`task_` + 随机串） |
| `platform` | varchar(30) | index | 任务平台（`suno`、`kling` 等） |
| `user_id` | int | index | 用户 ID |
| `group` | varchar(50) | — | 计费分组 |
| `channel_id` | int | index | 渠道 ID |
| `quota` | int | — | 消耗配额 |
| `action` | varchar(40) | index | 任务类型（`song`、`lyrics`、`video` 等） |
| `status` | varchar(20) | index | 任务状态 |
| `fail_reason` | text | — | 失败原因 |
| `submit_time` | int64 | index | 提交时间 |
| `start_time` | int64 | index | 开始执行时间 |
| `finish_time` | int64 | index | 完成时间 |
| `progress` | varchar(20) | index | 进度百分比 |
| `properties` | JSON | — | 任务属性（input、模型名等） |
| `private_data` | JSON | — | 内部数据（key、上游 task ID、计费上下文等，**不返回给用户**） |
| `data` | JSON | — | 任务结果数据 |

**任务状态枚举**：`NOT_START` → `SUBMITTED` → `QUEUED` → `IN_PROGRESS` → `SUCCESS` / `FAILURE`

#### ⑦ `subscription_plans` — 订阅计划表

**文件**：`model/subscription.go` → `type SubscriptionPlan struct`

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| `id` | int | PK, auto | 计划 ID |
| `title` | varchar(128) | not null | 计划标题 |
| `subtitle` | varchar(255) | default '' | 副标题 |
| `price_amount` | decimal(10,6) | not null, default 0 | 价格金额 |
| `currency` | varchar(8) | not null, default 'USD' | 货币代码 |
| `duration_unit` | varchar(16) | not null, default 'month' | 时长单位（year/month/day/hour/custom） |
| `duration_value` | int | not null, default 1 | 时长值 |
| `custom_seconds` | int64 | bigint, default 0 | 自定义时长（秒） |
| `enabled` | bool | default true | 是否启用 |
| `sort_order` | int | default 0 | 排序权重 |
| `allow_balance_pay` | bool | default true | 允许余额支付 |
| `stripe_price_id` | varchar(128) | — | Stripe Price ID |
| `creem_product_id` | varchar(128) | — | Creem Product ID |
| `waffo_pancake_product_id` | varchar(128) | — | Waffo Pancake Product ID |
| `max_purchase_per_user` | int | default 0 | 每用户最大购买次数（0=不限） |
| `upgrade_group` | varchar(64) | default '' | 购买后升级的用户分组 |
| `total_amount` | int64 | bigint, default 0 | 总配额（0=不限） |
| `quota_reset_period` | varchar(16) | default 'never' | 配额重置周期（never/daily/weekly/monthly/custom） |
| `quota_reset_custom_seconds` | int64 | bigint, default 0 | 自定义重置周期（秒） |
| `created_at` / `updated_at` | int64 | bigint | 时间戳 |

#### ⑧ `subscription_orders` — 订阅订单表

**文件**：`model/subscription.go` → `type SubscriptionOrder struct`

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| `id` | int | PK, auto | 订单 ID |
| `user_id` | int | index | 用户 ID |
| `plan_id` | int | index | 计划 ID |
| `money` | float64 | — | 支付金额 |
| `trade_no` | varchar(255) | unique, index | 交易号 |
| `payment_method` | varchar(50) | — | 支付方式 |
| `payment_provider` | varchar(50) | default '' | 支付提供商 |
| `status` | varchar | — | 订单状态 |
| `create_time` | int64 | — | 创建时间 |
| `complete_time` | int64 | — | 完成时间 |
| `provider_payload` | text | — | 支付商原始回调数据 |

#### ⑨ `user_subscriptions` — 用户订阅实例表

**文件**：`model/subscription.go` → `type UserSubscription struct`

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| `id` | int | PK, auto | 订阅 ID |
| `user_id` | int | index, 复合索引 | 用户 ID |
| `plan_id` | int | index | 计划 ID |
| `amount_total` | int64 | bigint, default 0 | 订阅总配额 |
| `amount_used` | int64 | bigint, default 0 | 已使用配额 |
| `start_time` | int64 | bigint | 开始时间 |
| `end_time` | int64 | bigint, index, 复合索引 | 结束时间 |
| `status` | varchar(32) | index, 复合索引 | 状态（active/expired/cancelled） |
| `source` | varchar(32) | default 'order' | 来源（order=购买, admin=管理员分配） |
| `last_reset_time` | int64 | bigint, default 0 | 上次配额重置时间 |
| `next_reset_time` | int64 | bigint, default 0, index | 下次配额重置时间 |
| `upgrade_group` | varchar(64) | default '' | 订阅期间的用户分组 |
| `prev_user_group` | varchar(64) | default '' | 订阅前的用户分组（用于恢复） |
| `created_at` / `updated_at` | int64 | bigint | 时间戳 |

#### ⑩ `top_ups` — 充值记录表

**文件**：`model/topup.go` → `type TopUp struct`

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| `id` | int | PK, auto | 记录 ID |
| `user_id` | int | index | 用户 ID |
| `amount` | int64 | — | 充值配额数量 |
| `money` | float64 | — | 支付金额 |
| `trade_no` | varchar(255) | unique, index | 交易号 |
| `payment_method` | varchar(50) | — | 支付方式 |
| `payment_provider` | varchar(50) | default '' | 支付提供商 |
| `create_time` | int64 | — | 创建时间 |
| `complete_time` | int64 | — | 完成时间 |
| `status` | varchar | — | 状态 |

#### ⑪ `redemptions` — 兑换码表

**文件**：`model/redemption.go` → `type Redemption struct`

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| `id` | int | PK, auto | 兑换码 ID |
| `user_id` | int | — | 创建者用户 ID |
| `key` | char(32) | uniqueIndex | 兑换码 |
| `status` | int | default 1 | 状态：1=未使用, 2=已使用 |
| `name` | varchar | index | 兑换码名称 |
| `quota` | int | default 100 | 兑换配额 |
| `created_time` | int64 | bigint | 创建时间 |
| `redeemed_time` | int64 | bigint | 兑换时间 |
| `used_user_id` | int | — | 使用者用户 ID |
| `expired_time` | int64 | bigint | 过期时间（0=不过期） |
| `deleted_at` | datetime | index | 软删除时间 |

#### ⑫ `options` — 系统配置项表

**文件**：`model/option.go` → `type Option struct`

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| `key` | varchar | **PK** | 配置键 |
| `value` | text | — | 配置值 |

KV 存储，所有系统配置项通过此表持久化。启动时加载到 `common.OptionMap`。

#### 其他辅助表

| 表 | 结构体 | 文件 | 说明 |
|---|---|---|---|
| `midjourneys` | `Midjourney` | `midjourney.go` | Midjourney 图像任务（旧版，与 tasks 表并存） |
| `checkins` | `Checkin` | `checkin.go` | 签到记录（`user_id` + `checkin_date` 联合唯一） |
| `quota_data` | `QuotaData` | `usedata.go` | 用量统计聚合（按模型/用户/时间） |
| `setups` | `Setup` | `setup.go` | 系统初始化状态（仅一条记录） |
| `passkey_credentials` | `PasskeyCredential` | `passkey.go` | WebAuthn 凭证（`user_id` 唯一） |
| `two_f_as` | `TwoFA` | `twofa.go` | TOTP 2FA 设置（`user_id` 唯一） |
| `two_f_a_backup_codes` | `TwoFABackupCode` | `twofa.go` | 2FA 备用码 |
| `models` | `Model` | `model_meta.go` | 模型元数据（名称/描述/图标/标签/供应商） |
| `vendors` | `Vendor` | `vendor_meta.go` | 供应商元数据 |
| `prefill_groups` | `PrefillGroup` | `prefill_group.go` | 预填充分组模板（JSON items） |
| `perf_metrics` | `PerfMetric` | `perf_metric.go` | 性能指标聚合（按模型/分组/时间桶，Upsert） |
| `custom_oauth_providers` | `CustomOAuthProvider` | `custom_oauth_provider.go` | 自定义 OAuth 配置（含访问策略 JSON） |
| `user_oauth_bindings` | `UserOAuthBinding` | `user_oauth_binding.go` | 用户-OAuth 绑定（`user_id` + `provider_id` 唯一） |
| `subscription_pre_consume_records` | `SubscriptionPreConsumeRecord` | `subscription.go` | 订阅预扣记录（幂等退款用） |

---

## 4. 实体之间关系

### 4.1 ER 关系图

```
┌──────────┐     1:N     ┌──────────┐     1:N     ┌──────────────┐
│  users   │────────────►│  tokens  │             │ subscriptions│
│          │             └──────────┘             │   _plans     │
│          │     1:N     ┌──────────┐             └──────┬───────┘
│          │────────────►│   logs   │                    │ 1:N
│          │             └──────────┘                    ▼
│          │     1:N     ┌──────────┐     N:1    ┌──────────────┐
│          │────────────►│   tasks  │◄───────────│  channels    │
│          │             └──────────┘            │              │
│          │     1:N     ┌──────────┐            │              │
│          │────────────►│ top_ups  │            │              │
│          │             └──────────┘            │              │
│          │     1:N     ┌──────────┐            │              │
│          │────────────►│redemption│            │              │
│          │             └──────────┘            │              │
│          │     1:1     ┌──────────┐            │              │
│          │────────────►│ two_f_as │            │              │
│          │             └──────────┘            │              │
│          │     1:1     ┌──────────────────┐    │              │
│          │────────────►│passkey_credentials│   │              │
│          │             └──────────────────┘    │              │
│          │     1:N     ┌───────────────────┐   │              │
│          │────────────►│ user_oauth_bindings│  │              │
│          │             └────────┬──────────┘   │              │
└──────────┘                      │ N:1          │              │
                                  ▼              │              │
                    ┌─────────────────────┐      │              │
                    │custom_oauth_providers│      │              │
                    └─────────────────────┘      └──────────────┘
                                                        │
                                                        │ 1:N
                                                        ▼
                                                 ┌──────────┐
                                                 │abilities │ ← 复合 PK: (group, model, channel_id)
                                                 └──────────┘
                                                        │
                                                        │ N:1
                                                        ▼
┌──────────┐     1:N     ┌──────────────────┐    ┌──────────┐
│  users   │────────────►│user_subscriptions│    │  models  │
└──────────┘             └────────┬─────────┘    └────┬─────┘
                                  │ N:1                │ N:1
                                  ▼                    ▼
                         ┌──────────────────┐    ┌──────────┐
                         │subscription_plans│    │ vendors  │
                         └──────────────────┘    └──────────┘
```

### 4.2 关系说明

| 关系 | 类型 | 外键 | 说明 |
|---|---|---|---|
| User → Token | 一对多 | `token.user_id → user.id` | 一个用户可创建多个 API Token |
| User → Log | 一对多 | `log.user_id → user.id` | 一个用户有多条使用日志 |
| User → Task | 一对多 | `task.user_id → user.id` | 一个用户可提交多个异步任务 |
| Channel → Task | 一对多 | `task.channel_id → channel.id` | 一个渠道可处理多个任务 |
| User → TopUp | 一对多 | `topup.user_id → user.id` | 一个用户可有多条充值记录 |
| User → Redemption | 一对多 | `redemption.used_user_id → user.id` | 一个用户可使用多个兑换码 |
| User → Checkin | 一对多 | `checkin.user_id → user.id` | 一个用户可多次签到 |
| User → QuotaData | 一对多 | `quota_data.user_id → user.id` | 用量统计 |
| User → UserSubscription | 一对多 | `user_subscription.user_id → user.id` | 一个用户可有多个订阅 |
| SubscriptionPlan → SubscriptionOrder | 一对多 | `order.plan_id → plan.id` | 一个计划可有多个订单 |
| SubscriptionPlan → UserSubscription | 一对多 | `sub.plan_id → plan.id` | 一个计划可有多个用户订阅 |
| User → TwoFA | 一对一 | `twofa.user_id → user.id`（unique） | 一个用户一个 2FA 设置 |
| User → PasskeyCredential | 一对一 | `passkey.user_id → user.id`（unique） | 一个用户一个 Passkey |
| User → UserOAuthBinding | 一对多 | `binding.user_id → user.id` | 一个用户可绑定多个 OAuth |
| CustomOAuthProvider → UserOAuthBinding | 一对多 | `binding.provider_id → provider.id` | 一个提供商可有多个绑定 |
| Channel → Ability | 一对多 | `ability.channel_id → channel.id` | 一个渠道可有多条能力映射 |
| Vendor → Model | 一对多 | `model.vendor_id → vendor.id` | 一个供应商可有多个模型 |

### 4.3 非外键约束的关系

以下关系**未在数据库层定义外键约束**，仅通过应用层维护一致性：

- `log.channel_id → channel.id`（日志表的渠道 ID 不是外键）
- `ability.(group, model)` 与 `channel.group` 和 `channel.models` 的对应关系
- `user.group` 与渠道选择中的分组匹配

---

## 5. 数据读写的主要入口

### 5.1 高频读取路径

| 操作 | 函数 | 文件 | 缓存策略 |
|---|---|---|---|
| 获取渠道缓存 | `model.CacheGetChannel(id)` | `model/channel_cache.go` | 内存缓存（`channelsIDM`） |
| 渠道选择索引 | `group2model2channels` | `model/channel_cache.go` | 内存缓存，定时同步 |
| 获取用户缓存 | `model.GetUserCache(userId)` | `model/user_cache.go` | 内存缓存 |
| 获取 Token | `model.ValidateUserToken(key)` | `model/token.go` + `token_cache.go` | 内存缓存 |
| 获取配置项 | `common.OptionMap[key]` | `model/option.go` | 内存缓存（`OptionMap`） |
| 订阅计划 | `model.GetSubscriptionPlanById(id)` | `model/subscription.go` | HybridCache（内存+Redis） |

### 5.2 高频写入路径

| 操作 | 函数 | 文件 | 写入方式 |
|---|---|---|---|
| 记录消费日志 | `model.RecordConsumeLog()` | `model/log.go` | 直接写入（支持批量更新） |
| 更新用户配额 | `model.UpdateUserUsedQuotaAndRequestCount()` | `model/user.go` | 直接更新 |
| 更新渠道配额 | `model.UpdateChannelUsedQuota()` | `model/channel.go` | 直接更新 |
| 减少用户配额 | `model.DecreaseUserQuota()` | `model/user.go` | 直接更新（可批量） |
| 更新 Token 配额 | `model.DecreaseTokenQuota()` / `IncreaseTokenQuota()` | `model/token.go` | 直接更新 |
| 写入任务 | `task.Insert()` | `model/task.go` | 直接写入 |
| 更新任务状态 | `task.UpdateWithStatus(oldStatus)` | `model/task.go` | CAS 更新（乐观锁） |
| 写入性能指标 | `UpsertPerfMetric()` | `model/perf_metric.go` | Upsert（`ON CONFLICT DO UPDATE`） |
| 保存订阅预扣记录 | `model.PreConsumeUserSubscription()` | `model/subscription.go` | 事务写入 |

### 5.3 批量更新机制

**文件**：`model/main.go` 中的 `InitBatchUpdater()`

当 `BATCH_UPDATE_ENABLED=true` 时，用户配额和渠道配额的更新会先缓存在内存中，每隔 `BATCH_UPDATE_INTERVAL` 秒批量写入数据库，减少数据库压力。

---

## 6. 潜在的数据一致性风险

### 6.1 配额扣减的竞态条件

**风险等级**：中

**位置**：`service/pre_consume_quota.go` → `PreConsumeQuota()`

```
1. GetUserQuota(userId)        ← 读取用户配额
2. 判断 quota >= preConsumedQuota
3. DecreaseUserQuota(userId)   ← 扣减用户配额
```

步骤 1 和 3 之间存在时间窗口。在高并发场景下，同一用户的多个请求可能同时通过步骤 2 的检查，导致超额扣减。

**现有缓解**：
- `BillingSession` 使用 `sync.Mutex` 保护单次请求的结算
- Token 配额和用户配额分别扣减，提供双重保护
- 信任额度机制（`trustQuota`）在额度充足时跳过预扣

**残余风险**：多请求并发时仍可能短暂超额，但结算时会修正差额。

### 6.2 渠道缓存与数据库的不一致

**风险等级**：低

**位置**：`model/channel_cache.go`

渠道缓存每 60 秒从数据库全量同步一次。在此期间：
- 新增/修改/删除的渠道不会立即生效
- 多节点部署时，各节点的缓存刷新时间不同步

**现有缓解**：
- 写操作后主动调用 `InvalidateChannelCache()` 清除缓存
- `SyncChannelCache()` 定时全量刷新

**残余风险**：管理员修改渠道配置后有最多 60 秒的延迟窗口。

### 6.3 异步任务状态更新的 CAS 竞争

**风险等级**：低

**位置**：`model/task.go` → `UpdateWithStatus(oldStatus)`

任务轮询和超时清理可能同时操作同一任务。项目使用 CAS（Compare-And-Swap）更新防止状态冲突：

```go
func (t *Task) UpdateWithStatus(oldStatus TaskStatus) (bool, error) {
    result := DB.Where("id = ? AND status = ?", t.ID, oldStatus).Updates(t)
    return result.RowsAffected > 0, nil
}
```

**评估**：实现正确，CAS 失败时跳过更新，避免重复处理。

### 6.4 订阅预扣与退款的幂等性

**风险等级**：低

**位置**：`model/subscription.go` → `PreConsumeUserSubscription()` / `RefundSubscriptionPreConsume()`

订阅预扣使用 `requestId` 作为幂等键，退款函数在事务中执行并最多重试 3 次（`service/funding_source.go:refundWithRetry()`）。

**评估**：幂等性设计正确，但 `IncreaseUserQuota()` 不是幂等操作（注释中明确说明），需要依赖上层逻辑防止重复退款。

### 6.5 多节点批量更新的数据丢失

**风险等级**：中

**位置**：`model/main.go` → `InitBatchUpdater()`

当 `BATCH_UPDATE_ENABLED=true` 时，配额更新先缓存在内存中。如果进程异常退出：
- 内存中未写入的更新会丢失
- 多节点各自维护独立的批量更新队列

**建议**：生产环境谨慎使用批量更新，或确保进程优雅退出时刷新队列。

### 6.6 软删除的唯一索引问题

**风险等级**：低

**位置**：多表使用 `gorm.DeletedAt` 软删除

部分表（如 `tokens`、`redemptions`）对 `key` 字段设置了 `uniqueIndex`。在软删除后，如果 `deleted_at` 被设置，GORM 的软删除机制会将 `deleted_at` 纳入唯一索引考虑。但 SQLite 和 MySQL 的行为不同：
- SQLite：`NULL` 值在 UNIQUE 约束中被视为不同
- MySQL：同上
- PostgreSQL：同上

**评估**：当前实现通过 `gorm.DeletedAt`（指针类型，`NULL` 表示未删除）正确处理了这个问题。

---

## 附录：内存缓存结构

以下全局变量在进程内存中维护，不持久化：

| 变量 | 类型 | 文件 | 说明 |
|---|---|---|---|
| `group2model2channels` | `map[string]map[string][]int` | `model/channel_cache.go` | group → model → []channelId（按优先级排序） |
| `channelsIDM` | `map[int]*Channel` | `model/channel_cache.go` | channelId → *Channel（全量渠道映射） |
| `common.OptionMap` | `map[string]string` | `common/` | 系统配置项 KV |
| `userCache` | `map[int]*UserBase` | `model/user_cache.go` | userId → *UserBase |
| `tokenKeyCache` | `map[string]*Token` | `model/token_cache.go` | tokenKey → *Token |
| `pricingMap` | `[]Pricing` | `model/pricing.go` | 模型定价列表（内存缓存） |
| `modelEnableGroups` | `map[string][]string` | `model/pricing.go` | model → enabled groups |
| `modelQuotaTypeMap` | `map[string]int` | `model/pricing.go` | model → quota type |
