package handlers

// 扫码创建飞书机器人。
//
// 两个接口:
//   POST /api/feishu/registration/start  创建会话,回二维码地址
//   GET  /api/feishu/registration/:id    轮询状态,成功时凭据落库并重连
//
// 安全边界:
//   - 两个接口都在 Settings 组里,需要 X-API-Key,不能进白名单;
//   - device_code 只活在服务端内存,响应里没有它;
//   - App Secret 落库后回前端只给 appSecretConfigured:true;
//   - 所有响应 Cache-Control: no-store。

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/ovh-buy/server/internal/app"
	"github.com/ovh-buy/server/internal/feishu"
	"github.com/ovh-buy/server/internal/types"
)

// 进程级单例:会话是内存态,和 State 的生命周期一致。
var feishuRegistration = feishu.NewRegistrationService()

func noStore(c *gin.Context) {
	c.Header("Cache-Control", "no-store, no-cache, must-revalidate")
	c.Header("Pragma", "no-cache")
}

// StartFeishuRegistration POST /api/feishu/registration/start
func StartFeishuRegistration(state *app.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		noStore(c)
		reply, err := feishuRegistration.Start(c.Request.Context())
		if err != nil {
			state.Logger.Warn("飞书扫码注册初始化失败: "+err.Error(), "feishu")
			c.JSON(http.StatusBadGateway, gin.H{
				"status":  "error",
				"message": err.Error(),
			})
			return
		}
		state.Logger.Info("已创建飞书扫码注册会话", "feishu")
		c.JSON(http.StatusOK, gin.H{
			"status":                  "success",
			"sessionId":               reply.SessionID,
			"verificationUriComplete": reply.VerificationURIComplete,
			"expiresIn":               reply.ExpiresIn,
			"interval":                reply.Interval,
		})
	}
}

// PollFeishuRegistration GET /api/feishu/registration/:id
func PollFeishuRegistration(state *app.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		noStore(c)
		id := strings.TrimSpace(c.Param("id"))
		if id == "" {
			c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "缺少会话 id"})
			return
		}
		reply, err := feishuRegistration.Poll(c.Request.Context(), id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"status": "error", "message": err.Error()})
			return
		}

		switch reply.Status {
		case feishu.StatusComplete:
			// 四件事:落库 -> 绑定扫码者 -> 重连 -> 返回脱敏信息。
			// 落库失败不能回 complete,否则用户以为建好了,重启后配置却没了。
			cfg := state.Config.Get()
			cfg.FeishuAppID = strings.TrimSpace(reply.AppID)
			cfg.FeishuAppSecret = strings.TrimSpace(reply.AppSecret)
			cfg.FeishuDomain = types.NormalizeFeishuDomain(reply.Domain)
			if openID := strings.TrimSpace(reply.OpenID); openID != "" {
				cfg.FeishuBoundOpenID = openID
			}
			cfg.FeishuEnabled = true
			if err := state.Config.Set(cfg); err != nil {
				state.Logger.Error("飞书凭据保存失败: "+err.Error(), "feishu")
				c.JSON(http.StatusInternalServerError, gin.H{
					"status":  "error",
					"message": "扫码成功,但凭据保存失败:" + err.Error(),
				})
				return
			}
			// 扫码创建后同样自动发一条确认:用户刚扫完码,这是唯一能证明
			// "长连接真的通了"的反馈。
			feishu.QueueTestMessage()
			feishu.Reconfigure()
			state.Logger.Info("飞书机器人凭据已保存,域名="+cfg.FeishuDomain, "feishu")
			c.JSON(http.StatusOK, gin.H{
				"status":              "success",
				"state":               reply.Status,
				"appId":               cfg.FeishuAppID,
				"appSecretConfigured": true,
				"domain":              cfg.FeishuDomain,
				"bound":               cfg.FeishuBoundOpenID != "",
				// 回填给设置页表单：扫码后用户可能马上点「保存设置」，
				// 表单里若没有 open_id，那次保存会把刚写的绑定抹掉。
				"boundOpenId": cfg.FeishuBoundOpenID,
			})

		case feishu.StatusPending:
			retryAfter := reply.RetryAfter
			if retryAfter <= 0 {
				retryAfter = 5
			}
			c.JSON(http.StatusOK, gin.H{
				"status":     "success",
				"state":      reply.Status,
				"retryAfter": retryAfter,
			})

		case feishu.StatusDenied, feishu.StatusExpired:
			c.JSON(http.StatusOK, gin.H{
				"status":  "success",
				"state":   reply.Status,
				"message": reply.Message,
			})

		default:
			c.JSON(http.StatusOK, gin.H{
				"status":  "success",
				"state":   reply.Status,
				"message": reply.Message,
			})
		}
	}
}
