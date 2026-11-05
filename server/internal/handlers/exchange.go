package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ovh-buy/server/internal/exchange"
)

// GetExchangeRates GET /api/exchange/rates
//
// 只返回后台服务的当前快照，不在这里触发抓取：
//   - 前端缓存 30 分钟，但用户刷新页面可能更频繁，不能把请求配额打给免费 provider；
//   - 抓取失败时快照本身就是 inactive，前端据此不显示人民币 ——
//     "不显示"永远优于"显示一个错的数字"。
func GetExchangeRates() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, exchange.CurrentSnapshot())
	}
}
