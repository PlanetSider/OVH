package handlers

import (
	"strings"

	"github.com/ovh-buy/server/internal/app"
	"github.com/ovh-buy/server/internal/feishu"
	"github.com/ovh-buy/server/internal/monitor"
)

// HandleFeishuMessage 处理一条飞书入站消息，返回要回发的文本（空串 = 不回）。
//
// 安全边界：
//   - 只处理私聊（p2p）。群里任何人 @ 机器人都能触发命令，而 /cancel 会改队列；
//   - 必须由绑定过的 open_id 发出。未绑定时一律拒绝，不做"第一个说话的人自动绑定"
//     —— 绑定的动作只能从网页设置页发起；
//   - 事件 ID 幂等：长连接断线重放不会把 /cancel 执行两次。
func HandleFeishuMessage(state *app.State, mon *monitor.Monitor, in feishu.Message) string {
	if state == nil {
		return ""
	}
	cfg := state.Config.Get()
	if !cfg.FeishuEnabled || !cfg.FeishuConfigured() {
		return ""
	}
	// 群聊直接忽略：这里没有 @ 判定，宁可静默也不要让群里的人碰到队列操作
	if in.ChatType != "" && in.ChatType != "p2p" {
		return ""
	}

	// 幂等放在绑定检查之前：绑定请求也是入站消息，断线重放不能消费两次。
	if in.EventID != "" && state.DB != nil {
		claimed, err := state.DB.TryClaimFeishuEvent(in.EventID)
		if err != nil {
			state.Logger.Error("飞书事件幂等写入失败: "+err.Error(), "feishu")
			return "⚠️ 内部错误，请稍后重试。"
		}
		if !claimed {
			return ""
		}
	}

	text := strings.TrimSpace(in.Text)
	if cfg.FeishuBoundOpenID == "" {
		return handleFeishuBinding(state, in.UserID, text)
	}
	if in.UserID != cfg.FeishuBoundOpenID {
		state.Logger.Warn("拒绝未授权的飞书消息: open_id="+in.UserID, "feishu")
		return "⛔ 你不是这台控制台绑定的操作者，无法执行命令。"
	}

	if text == "" {
		return ""
	}
	// 已绑定后再发 /bind 没有语义：换人必须先解绑，否则"知道码的人"能顶替操作者。
	if strings.HasPrefix(strings.ToLower(text), "/bind") {
		return "已经绑定过了。想更换绑定人，请先在网页控制台解除绑定。"
	}
	state.Logger.Info("收到飞书命令: "+truncate(text, 80), "feishu")

	if reply, ok := BotCommandReply(state, mon, text); ok {
		return reply
	}
	// 不是命令就按文本下单格式再试一次；两条路径与 Telegram 完全一致
	if reply, ok := BotTextOrder(state, mon, text); ok {
		return reply
	}
	return "没看懂这条消息。\n\n下单格式：<型号> [机房] [数量] [配置] [@账户]\n" +
		"例：24sk602 gra 2\n\n发 /help 看完整说明。"
}

// handleFeishuBinding 处理"还没绑定操作者"阶段的消息。
//
// 这阶段只接受 /bind <6位码>：绑定码由网页设置页生成，只有能登录控制台的人
// 才拿得到 —— 这是"加机器人的人"和"控制台主人"之间唯一可信的一步。
// 不做"第一个说话的人自动绑定"：机器人被拉进别人的私聊时，那等于把下单权送人。
func handleFeishuBinding(state *app.State, openID, text string) string {
	fields := strings.Fields(text)
	if len(fields) == 2 {
		if cmd := strings.ToLower(strings.TrimPrefix(fields[0], "/")); cmd == "bind" {
			if reason := claimFeishuBindingCode(state, fields[1]); reason != "" {
				return reason
			}
			cfg := state.Config.Get()
			cfg.FeishuBoundOpenID = openID
			if err := state.Config.Set(cfg); err != nil {
				state.Logger.Error("保存飞书绑定失败: "+err.Error(), "feishu")
				return "⚠️ 绑定写库失败，请稍后重试。"
			}
			state.Logger.Info("飞书已绑定操作者: "+openID, "feishu")
			return "✅ 绑定成功。以后只有这个私聊能发命令。发 /help 看能做什么。"
		}
	}
	if strings.HasPrefix(strings.ToLower(text), "/help") {
		return "这台机器人还没有绑定操作者。\n\n到网页控制台 → 设置 → 飞书机器人 → 点「生成绑定码」，再发 /bind <6位码>。"
	}
	return "这台机器人还没有绑定操作者。\n\n到网页控制台 → 设置 → 飞书机器人 → 点「生成绑定码」，再发 /bind <6位码>。"
}
