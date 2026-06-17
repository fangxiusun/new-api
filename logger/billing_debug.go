package logger

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

// BillingDebugf 输出计费调试日志（格式化字符串），格式与 logHelper 一致，额外包含 file:line。
// 格式：[BILLING_DEBUG] 2006/01/02 - 15:04:05 | request-id | file.go:42 | message
func BillingDebugf(ctx *gin.Context, format string, args ...interface{}) {
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
func BillingDebugMap(ctx *gin.Context, fields map[string]interface{}) {
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
// skip=2：billingDebugLog → BillingDebugf/BillingDebugMap → 调用者
func billingDebugLog(ctx *gin.Context, msg string) {
	pc, file, line, ok := runtime.Caller(2)
	if !ok {
		file = "unknown"
		line = 0
	}
	_ = pc
	fileLine := fmt.Sprintf("%s:%d", filepath.Base(file), line)

	// 获取 request-id，与 logHelper 逻辑保持一致
	var id interface{} = "SYSTEM"
	if ctx != nil {
		if requestID := ctx.Value(common.RequestIdKey); requestID != nil {
			id = requestID
		}
	}

	now := time.Now()
	// 输出格式与 logHelper 完全一致，仅在 request-id 和 message 之间插入 file:line
	common.LogWriterMu.RLock()
	_, _ = fmt.Fprintf(gin.DefaultWriter, "[BILLING_DEBUG] %v | %v | %s | %s \n",
		now.Format("2006/01/02 - 15:04:05"), id, fileLine, msg)
	common.LogWriterMu.RUnlock()
}

// BillingDebugfWithCtx 支持 context.Context 的版本，用于异步任务等无 gin.Context 的场景。
func BillingDebugfWithCtx(ctx context.Context, format string, args ...interface{}) {
	if !common.IsBillingDebugEnabled() {
		return
	}
	msg := format
	if len(args) > 0 {
		msg = fmt.Sprintf(format, args...)
	}
	billingDebugLogWithContext(ctx, msg)
}

// BillingDebugMapWithCtx 支持 context.Context 的版本。
func BillingDebugMapWithCtx(ctx context.Context, fields map[string]interface{}) {
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
	billingDebugLogWithContext(ctx, msg)
}

func billingDebugLogWithContext(ctx context.Context, msg string) {
	pc, file, line, ok := runtime.Caller(2)
	if !ok {
		file = "unknown"
		line = 0
	}
	_ = pc
	fileLine := fmt.Sprintf("%s:%d", filepath.Base(file), line)

	var id interface{} = "SYSTEM"
	if ctx != nil {
		if requestID := ctx.Value(common.RequestIdKey); requestID != nil {
			id = requestID
		}
	}

	now := time.Now()
	common.LogWriterMu.RLock()
	_, _ = fmt.Fprintf(gin.DefaultWriter, "[BILLING_DEBUG] %v | %v | %s | %s \n",
		now.Format("2006/01/02 - 15:04:05"), id, fileLine, msg)
	common.LogWriterMu.RUnlock()
}
