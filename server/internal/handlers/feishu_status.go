package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ovh-buy/server/internal/app"
	"github.com/ovh-buy/server/internal/feishu"
)

// GetFeishuStatus GET /api/feishu/status
//
// 设置页用它显示长连接到底连上没有。飞书是服务端主动连出去的长连接,
// 没有入站请求,连接断了不会有任何别的迹象 —— 命令、文本下单、卡片按钮
// 会一起静默失效,所以这里必须能看出"在不在连着"。
func GetFeishuStatus(_ *app.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"status":  feishu.CurrentStatus(),
		})
	}
}

// SendFeishuTestMessage POST /api/feishu/test-send
//
// 设置页的"测试发送"按钮。返回具体原因而不是静默失败:
// 未就绪/未绑定/发送报错三种情况用户要做的动作完全不同。
func SendFeishuTestMessage(_ *app.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := feishu.SendTestNow(); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "success"})
	}
}
