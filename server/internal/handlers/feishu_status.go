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
