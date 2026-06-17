package common

import (
	"os"
	"strings"
	"sync/atomic"
)

// billingDebugEnabled 使用 atomic 存储，避免锁竞争。
// 0 = 关闭, 1 = 开启。
// 启动时从环境变量 BILLING_DEBUG 读取初始值，运行时可通过 SetBillingDebugEnabled 热切换。
var billingDebugEnabled atomic.Int32

func init() {
	if strings.EqualFold(os.Getenv("BILLING_DEBUG"), "true") ||
		os.Getenv("BILLING_DEBUG") == "1" {
		billingDebugEnabled.Store(1)
		SysLog("billing debug log enabled via BILLING_DEBUG env")
	}
}

// IsBillingDebugEnabled 返回计费调试日志是否开启。
func IsBillingDebugEnabled() bool {
	return billingDebugEnabled.Load() == 1
}

// SetBillingDebugEnabled 设置计费调试日志开关（运行时热切换）。
func SetBillingDebugEnabled(enabled bool) {
	if enabled {
		billingDebugEnabled.Store(1)
	} else {
		billingDebugEnabled.Store(0)
	}
}
