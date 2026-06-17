package controller

import (
	"fmt"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

// GetBillingDebug 获取计费调试日志开关状态
func GetBillingDebug(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"enabled": common.IsBillingDebugEnabled(),
	})
}

// SetBillingDebug 设置计费调试日志开关
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

	status := "disabled"
	if req.Enabled {
		status = "enabled"
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": fmt.Sprintf("billing debug log %s", status),
		"enabled": common.IsBillingDebugEnabled(),
	})
}
