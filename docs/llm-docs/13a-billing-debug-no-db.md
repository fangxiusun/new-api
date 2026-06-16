# 计费调试日志开关 — 无数据库方案

> 本文档是 `13-billing-debug-log-plan.md` 的替代方案，不修改数据库，仅使用环境变量或运行时 POST 请求控制开关。

---

## 1. 设计目标

- **零数据库变更**：不修改 `configs` 表结构，不新增任何数据库字段
- **环境变量控制**：启动时通过环境变量 `BILLING_DEBUG` 设置初始值
- **运行时热切换**：通过 POST API 在运行时开启/关闭，无需重启服务
- **内存存储**：开关状态仅存在于内存中，服务重启后恢复为环境变量值
- **格式一致**：日志格式与系统原有日志（`logger.logHelper`）保持一致，仅新增文件行号字段

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

### 3.1 核心实现：原子标志 + 环境变量

**新建文件**：`common/billing_debug.go`

```go
package common

import (
    "os"
    "strings"
    "sync/atomic"
)

// billingDebugEnabled 使用 atomic 存储，避免锁竞争
// 0 = 关闭, 1 = 开启
var billingDebugEnabled atomic.Int32

func init() {
    // 启动时从环境变量读取
    if strings.EqualFold(os.Getenv("BILLING_DEBUG"), "true") ||
       os.Getenv("BILLING_DEBUG") == "1" {
        billingDebugEnabled.Store(1)
    }
}

// IsBillingDebugEnabled 返回计费调试日志是否开启
func IsBillingDebugEnabled() bool {
    return billingDebugEnabled.Load() == 1
}

// SetBillingDebugEnabled 设置计费调试日志开关（运行时热切换）
func SetBillingDebugEnabled(enabled bool) {
    if enabled {
        billingDebugEnabled.Store(1)
    } else {
        billingDebugEnabled.Store(0)
    }
}
```

**优势**：
- `atomic.Int32` 的 `Load()` 开销约 1ns，比 `sync.RWMutex.RLock()` 快 10 倍以上
- 无锁竞争，适合高并发热路径
- 无需依赖任何外部包

### 3.2 日志工具函数

**新建文件**：`logger/billing_debug.go`

```go
package logger

import (
    "fmt"
    "path/filepath"
    "runtime"
    "time"

    "github.com/QuantumNous/new-api/common"
    "github.com/gin-gonic/gin"
)

// BillingDebugf 输出计费调试日志，格式与 logHelper 一致，额外包含 file:line。
// 格式：[BILLING_DEBUG] 2006/01/02 - 15:04:05 | request-id | file.go:42 | message
func BillingDebugf(ctx interface{}, format string, args ...interface{}) {
    if !common.IsBillingDebugEnabled() {
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
    if !common.IsBillingDebugEnabled() {
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

### 3.3 运行时控制 API

**修改文件**：`controller/option.go`（或新建 `controller/debug.go`）

```go
// POST /api/debug/billing
// Body: {"enabled": true}
// Auth: 需要管理员权限
func SetBillingDebug(c *gin.Context) {
    var req struct {
        Enabled bool `json:"enabled"`
    }
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{
            "success": false,
            "message": "invalid request body",
        })
        return
    }

    common.SetBillingDebugEnabled(req.Enabled)

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": fmt.Sprintf("billing debug log %s",
            map[bool]string{true: "enabled", false: "disabled"}[req.Enabled]),
        "enabled": common.IsBillingDebugEnabled(),
    })
}

// GET /api/debug/billing
// Auth: 需要管理员权限
func GetBillingDebug(c *gin.Context) {
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "enabled": common.IsBillingDebugEnabled(),
    })
}
```

**修改文件**：`router/xxx.go`（路由注册）

```go
// 在管理员路由组中添加
adminRouter.POST("/debug/billing", controller.SetBillingDebug)
adminRouter.GET("/debug/billing", controller.GetBillingDebug)
```

---

## 4. 使用方式

### 4.1 环境变量控制（启动时）

```bash
# 开启
BILLING_DEBUG=true ./new-api

# 或
BILLING_DEBUG=1 ./new-api

# 关闭（默认）
BILLING_DEBUG=false ./new-api
```

### 4.2 POST 请求控制（运行时）

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

### 4.3 Docker 环境

```yaml
# docker-compose.yml
services:
  new-api:
    environment:
      - BILLING_DEBUG=true
```

---

## 5. 日志输出示例

### 5.1 文本请求（倍率模式）

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

### 5.2 视频任务（两阶段计费）

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

## 6. 性能分析

### 6.1 开关关闭时的开销

```go
// 每次调用的开销：
func IsBillingDebugEnabled() bool {
    return billingDebugEnabled.Load() == 1  // atomic load: ~1ns
}
```

**对比**：
| 方案 | 开关关闭时开销 | 说明 |
|------|---------------|------|
| `atomic.Int32`（本方案） | ~1ns | 无锁，CPU cache 友好 |
| `sync.RWMutex` | ~20ns | 需要锁操作 |
| `map` 读取 | ~50ns | 需要 hash 计算 |
| 数据库查询 | ~1ms | 需要网络 IO |

**结论**：即使在高频调用路径上（如每个请求调用 10 次），额外开销也仅 ~10ns，完全可以忽略。

### 6.2 开关开启时的开销

- `runtime.Caller()`：~200ns（获取调用者信息）
- `fmt.Sprintf()`：~100ns（格式化字符串）
- `fmt.Fprintf()`：~500ns（输出到 writer）
- 总计：~800ns/条日志

**建议**：仅在调试时开启，生产环境保持关闭。

---

## 7. 与数据库方案的对比

| 特性 | 数据库方案 | 环境变量+API 方案（本方案） |
|------|-----------|---------------------------|
| 数据库变更 | 需要 | ❌ 不需要 |
| 持久化 | ✅ 重启后保持 | ❌ 重启后恢复环境变量值 |
| 运行时切换 | ✅ 前端 UI | ✅ POST API |
| 多实例同步 | ✅ 数据库共享 | ❌ 每个实例独立 |
| 性能开销 | ~1ms（数据库读取） | ~1ns（atomic 读取） |
| 实现复杂度 | 中等 | 低 |

**适用场景**：
- 单实例部署：推荐本方案
- 多实例部署且需要统一控制：推荐数据库方案
- 临时调试：推荐本方案（无需修改数据库）

---

## 8. 实施步骤

### 第一步：创建核心文件（30分钟）

1. 创建 `common/billing_debug.go`（atomic 标志 + 环境变量读取）
2. 创建 `logger/billing_debug.go`（日志工具函数）

### 第二步：添加 API 接口（30分钟）

1. 在 `controller/` 中添加 `SetBillingDebug` 和 `GetBillingDebug`
2. 在路由中注册接口（管理员权限）

### 第三步：埋点（与数据库方案相同）

参考 `13-billing-debug-log-plan.md` 的 3.3 节，埋点位置完全相同。

### 第四步：测试（30分钟）

1. 单元测试：验证环境变量读取、atomic 标志切换
2. 集成测试：验证 API 接口控制
3. 性能测试：验证开关关闭时的开销

---

## 9. 扩展建议

### 9.1 多级别日志

```go
// 支持不同的调试级别
const (
    BillingDebugLevelOff    = 0
    BillingDebugLevelBasic  = 1  // 基本计费流程
    BillingDebugLevelDetail = 2  // 详细计算过程
    BillingDebugLevelTrace  = 3  // 完整追踪（含数据库操作）
)

var billingDebugLevel atomic.Int32

func GetBillingDebugLevel() int {
    return int(billingDebugLevel.Load())
}

func SetBillingDebugLevel(level int) {
    billingDebugLevel.Store(int32(level))
}

// 使用示例
func BillingDebugDetail(ctx interface{}, format string, args ...interface{}) {
    if common.GetBillingDebugLevel() < BillingDebugLevelDetail {
        return
    }
    // ... 调用 billingDebugLog
}
```

### 9.2 结构化日志（JSON 格式）

```go
// 输出 JSON 格式，便于日志采集系统解析
func BillingDebugJSON(ctx interface{}, fields map[string]interface{}) {
    if !common.IsBillingDebugEnabled() {
        return
    }

    pc, file, line, _ := runtime.Caller(1)

    fields["_timestamp"] = time.Now().Format(time.RFC3339Nano)
    fields["_file"] = filepath.Base(file)
    fields["_line"] = line
    fields["_func"] = runtime.FuncForPC(pc).Name()

    // 获取 request-id
    if gCtx, ok := ctx.(*gin.Context); ok && gCtx != nil {
        if requestID := gCtx.Value(common.RequestIdKey); requestID != nil {
            fields["_request_id"] = requestID
        }
    }

    jsonBytes, _ := common.Marshal(fields)
    common.LogWriterMu.RLock()
    fmt.Fprintln(gin.DefaultWriter, string(jsonBytes))
    common.LogWriterMu.RUnlock()
}
```
