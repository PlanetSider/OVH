package handlers

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/ovh-buy/server/internal/app"
	"github.com/ovh-buy/server/internal/feishu"
	"github.com/ovh-buy/server/internal/ovh"
	"github.com/ovh-buy/server/internal/telegram"
	"github.com/ovh-buy/server/internal/types"
)

// GetSettings GET /api/settings
func GetSettings(state *app.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, state.Config.Get())
	}
}

// SaveSettings POST /api/settings
func SaveSettings(state *app.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		var newCfg types.Config
		if err := c.ShouldBindJSON(&newCfg); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": ovh.Explain(err)})
			return
		}

		prev := state.Config.Get()

		// 凭据去空白（前端粘贴时常带空格/换行，会导致 OVH 签名失败 "Invalid signature"）
		newCfg.AppKey = strings.TrimSpace(newCfg.AppKey)
		newCfg.AppSecret = strings.TrimSpace(newCfg.AppSecret)
		newCfg.ConsumerKey = strings.TrimSpace(newCfg.ConsumerKey)
		newCfg.TgToken = strings.TrimSpace(newCfg.TgToken)
		newCfg.TgChatID = strings.TrimSpace(newCfg.TgChatID)
		// 同样去空白:webhook 地址末尾带个换行,POST 出去就是 DNS 解析失败
		newCfg.NotifyWebhookURL = strings.TrimSpace(newCfg.NotifyWebhookURL)
		// 在保存这一步就把地址挡下来。放过去的话,用户要等到真有货那一刻
		// 才会发现通知发不出去 —— 而那正是唯一不能出错的时刻。
		if newCfg.NotifyWebhookURL != "" {
			u, err := url.Parse(newCfg.NotifyWebhookURL)
			if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
				c.JSON(http.StatusBadRequest, gin.H{"status": "error",
					"message": "通知 Webhook 地址不合法,必须是完整的 http:// 或 https:// 地址", "code": "E3CAC8031"})
				return
			}
		}
		// Server酱³ 是 title/desp 表单协议,和上面的通用 JSON webhook 不是一回事,
		// 所以单独一个字段、单独验一遍:填错时同样要在这里挡下来。
		newCfg.ServerChanURL = strings.TrimSpace(newCfg.ServerChanURL)
		if newCfg.ServerChanURL != "" {
			u, err := url.Parse(newCfg.ServerChanURL)
			if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
				c.JSON(http.StatusBadRequest, gin.H{"status": "error",
					"message": "Server酱³ 地址不合法,必须是完整的 http:// 或 https:// 地址", "code": "E3CAC8032"})
				return
			}
		}
		newCfg.MeowNickname = strings.TrimSpace(newCfg.MeowNickname)
		if strings.ContainsAny(newCfg.MeowNickname, "/\\?#") {
			c.JSON(http.StatusBadRequest, gin.H{"status": "error",
				"message": "MeoW 昵称不能包含 / \\ ? #", "code": "E3CAC8033"})
			return
		}
		// 默认值兜底
		if newCfg.Endpoint == "" {
			newCfg.Endpoint = "ovh-eu"
		}
		if newCfg.Zone == "" {
			newCfg.Zone = "IE"
		}
		// 前端留空 / 传 0 = 用默认;超出区间夹回来。存进去的永远是明确的数
		newCfg.DefaultRetryInterval = types.ClampRetryInterval(newCfg.DefaultRetryInterval, types.DefaultTaskRetryInterval)
		newCfg.QuickOrderRetryInterval = types.ClampRetryInterval(newCfg.QuickOrderRetryInterval, types.DefaultQuickRetryInterval)
		// 汇率展示开关只认 off / both;前端没传(旧版前端)时按 both,保持升级前行为。
		newCfg.DisplayMode = types.NormalizeDisplayMode(newCfg.DisplayMode)

		// 飞书凭据同样去空白:App ID/Secret 复制时常带首尾空格,
		// 放过去会变成连接层的 "permission denied",日志里却像权限问题。
		newCfg.FeishuAppID = strings.TrimSpace(newCfg.FeishuAppID)
		newCfg.FeishuAppSecret = strings.TrimSpace(newCfg.FeishuAppSecret)
		newCfg.FeishuBoundOpenID = strings.TrimSpace(newCfg.FeishuBoundOpenID)
		newCfg.FeishuDomain = types.NormalizeFeishuDomain(newCfg.FeishuDomain)

		// 扫码注册成功后凭据已经写进库了,而扫码流程按设计不回传 App Secret,
		// 所以表单里这一格是空的。用户接着点「保存设置」时不能让空串把刚拿到的
		// 凭据抹掉 —— 那会表现为"扫码成功,保存后机器人又不工作了"。
		// 想禁用机器人请用 feishuEnabled=false(或未来单独的移除接口)。
		if newCfg.FeishuAppID == "" {
			newCfg.FeishuAppID = prev.FeishuAppID
		}
		if newCfg.FeishuAppSecret == "" {
			newCfg.FeishuAppSecret = prev.FeishuAppSecret
		}
		// 绑定同理：解绑走独立的 /api/feishu/unbind，不能让"保存设置"顺手清掉
		// 扫码或 /bind 刚写进去的 open_id。
		if newCfg.FeishuBoundOpenID == "" {
			newCfg.FeishuBoundOpenID = prev.FeishuBoundOpenID
		}

		if err := state.Config.Set(newCfg); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": ovh.Explain(err)})
			return
		}
		state.Logger.Info("API settings updated in config.json", "system")

		// Token 变了就得重拉长轮询。
		//
		// 收 update 只有这一条路,而循环是拿着旧 Token 在跑的 ——
		// 不重启的话用户换完 Token 保存,界面一切正常,但从此一条命令、
		// 一个按钮都收不到,而且没有任何地方会提示他。
		// 首次填 Token 同理:启动时没有 Token,poller 根本没起来。
		if newCfg.TgToken != prev.TgToken {
			state.Logger.Info("Telegram Token 已变更,重启长轮询", "telegram")
			go RestartPoller(state)
		}

		// 飞书凭据/域名/开关变了 → 让连接层按新配置重建。
		// 不重连的话用户换完 App Secret 保存,界面显示成功,实际还拿着旧凭据,
		// 而且不会有任何报错(旧长连接可能仍然活着)。
		if newCfg.FeishuAppID != prev.FeishuAppID ||
			newCfg.FeishuAppSecret != prev.FeishuAppSecret ||
			newCfg.FeishuDomain != prev.FeishuDomain ||
			newCfg.FeishuEnabled != prev.FeishuEnabled ||
			newCfg.FeishuBoundOpenID != prev.FeishuBoundOpenID {
			state.Logger.Info("飞书机器人配置已变更,重建长连接", "feishu")
			// 和 Telegram 保存 Token 后发测试消息对齐:让用户立刻知道"这条路通了"。
			// 连接是异步建立的,此刻还没有 Channel,所以只登记意图,连上后再发。
			feishu.QueueTestMessage()
			go feishu.Reconfigure()
		}

		// TG 配置变更 → 同步发一条测试消息
		if newCfg.TgToken != "" && newCfg.TgChatID != "" {
			changed := newCfg.TgToken != prev.TgToken || newCfg.TgChatID != prev.TgChatID
			if changed || prev.TgToken == "" || prev.TgChatID == "" {
				state.Logger.Info("Telegram Token或Chat ID已更新/设置。尝试发送Telegram测试消息到 Chat ID: "+newCfg.TgChatID, "")
				if telegram.SendMessage(state, "OVH 控制台: Telegram 通知已成功配置 (来自 Go 后端测试)", nil) {
					state.Logger.Info("Telegram 测试消息发送成功。", "")
				} else {
					state.Logger.Warn("Telegram 测试消息发送失败。请检查 Token 和 Chat ID 以及后端日志。", "")
				}
			} else {
				state.Logger.Info("Telegram 配置未更改，跳过测试消息。", "")
			}
		} else {
			state.Logger.Info("未配置 Telegram Token 或 Chat ID，跳过测试消息。", "")
		}

		c.JSON(http.StatusOK, gin.H{"status": "success"})
	}
}

// VerifyAuth POST /api/verify-auth
func VerifyAuth(state *app.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		client, err := ovhClientFor(state, c)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"valid": false})
			return
		}
		var me map[string]interface{}
		if err := client.Get("/me", &me); err != nil {
			state.Logger.Error("Authentication verification failed: "+err.Error(), "system")
			c.JSON(http.StatusOK, gin.H{"valid": false})
			return
		}
		c.JSON(http.StatusOK, gin.H{"valid": true})
	}
}

// EndpointConfig GET /api/endpoint-config
func EndpointConfig(state *app.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg := state.Config.Get()
		c.JSON(http.StatusOK, gin.H{
			"endpoint": cfg.Endpoint,
			"zone":     cfg.Zone,
		})
	}
}
