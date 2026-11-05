package handlers

import (
	"github.com/ovh-buy/server/internal/app"
	"github.com/ovh-buy/server/internal/feishu"
	"github.com/ovh-buy/server/internal/monitor"
)

// HandleFeishuCardAction 处理一次飞书卡片按钮点击，返回要回发的文本（空 = 不回）。
//
// 与消息入口共用同一套安全边界：只认绑定过的 open_id；事件 ID 幂等。
// 按钮 value 里是 {"a":"add_to_queue","u":"<uuid>"}，u 与 Telegram 按钮是同一个
// 一次性 nonce，所以跨通道重复点击只会成功一次。
func HandleFeishuCardAction(state *app.State, mon *monitor.Monitor, act feishu.CardAction) string {
	if state == nil {
		return ""
	}
	cfg := state.Config.Get()
	if !cfg.FeishuEnabled || !cfg.FeishuConfigured() {
		return ""
	}
	if cfg.FeishuBoundOpenID == "" || act.OpenID != cfg.FeishuBoundOpenID {
		return "⛔ 你没有权限操作这个按钮。"
	}
	if act.EventID != "" && state.DB != nil {
		claimed, err := state.DB.TryClaimFeishuEvent(act.EventID)
		if err != nil {
			state.Logger.Error("飞书卡片事件幂等写入失败: "+err.Error(), "feishu")
			return "⚠️ 内部错误，请稍后重试。"
		}
		if !claimed {
			return ""
		}
	}

	action, _ := act.Value["a"].(string)
	id, _ := act.Value["u"].(string)
	switch action {
	case "add_to_queue":
		return EnqueueFromButton(state, mon, id)
	default:
		return "未知操作。"
	}
}
