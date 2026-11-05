package feishu

import (
	"context"
	"sync"

	channeltypes "github.com/larksuite/oapi-sdk-go/v3/channel/types"
)

// Message 是从飞书事件归一化出来的一条入站消息。
//
// 业务层（handlers）只依赖这个结构，不直接碰 SDK 类型 —— 这样命令/卡片逻辑
// 可以单测，也不会因为 SDK 升级到处改。
type Message struct {
	EventID   string // 事件唯一 ID，用于幂等
	MessageID string
	ChatID    string
	ChatType  string // "p2p" / "group"
	UserID    string // 发送者 open_id
	Text      string
}

var (
	handlerMu sync.RWMutex
	handler   func(context.Context, Message) string
)

// SetMessageHandler 注册消息处理器：入参一条消息，返回要回发的文本；空串表示不回。
//
// 处理函数在 SDK 的读协程里同步执行，必须快速返回 —— 下单、重启这类耗时动作用
// goroutine 自己后续补发，不要在这里等。
func SetMessageHandler(h func(context.Context, Message) string) {
	handlerMu.Lock()
	handler = h
	handlerMu.Unlock()
}

// dispatchMessage 把 SDK 的归一化消息交给上层的处理器。
func dispatchMessage(ctx context.Context, msg *channeltypes.NormalizedMessage) string {
	if msg == nil {
		return ""
	}
	handlerMu.RLock()
	h := handler
	handlerMu.RUnlock()
	if h == nil {
		return ""
	}
	return h(ctx, Message{
		EventID:   msg.EventID,
		MessageID: msg.MessageID,
		ChatID:    msg.ChatID,
		ChatType:  msg.ChatType,
		UserID:    msg.UserID,
		Text:      msg.Content,
	})
}

// maxReplyRunes 单条回复的字符上限。
//
// 飞书单条文本技术上能到几十 KB，但/queue、/subs 这类列表一旦很长，
// 手机上就是一屏刷不完的竖排；截断到 3000 字与 Telegram 的 3800 字节限制接近，
// 保持两个通道的阅读体验一致。
const maxReplyRunes = 3000

func clampReply(s string) string {
	if s == "" {
		return ""
	}
	r := []rune(s)
	if len(r) <= maxReplyRunes {
		return s
	}
	return string(r[:maxReplyRunes]) + "\n…(内容过长已截断，完整信息请看网页控制台)"
}

// CardAction 一次卡片按钮回调（告警卡片上的「立即下单」等）。
type CardAction struct {
	EventID   string
	MessageID string
	ChatID    string
	OpenID    string
	Value     map[string]interface{}
}

var (
	cardHandlerMu sync.RWMutex
	cardHandler   func(context.Context, CardAction) string
)

// SetCardActionHandler 注册卡片回调处理器：入参一次点击，返回要回发的文本；空串表示不回。
func SetCardActionHandler(h func(context.Context, CardAction) string) {
	cardHandlerMu.Lock()
	cardHandler = h
	cardHandlerMu.Unlock()
}

func dispatchCardAction(ctx context.Context, ev *channeltypes.CardActionEvent) string {
	if ev == nil {
		return ""
	}
	cardHandlerMu.RLock()
	h := cardHandler
	cardHandlerMu.RUnlock()
	if h == nil {
		return ""
	}
	return h(ctx, CardAction{
		EventID:   ev.EventID,
		MessageID: ev.MessageID,
		ChatID:    ev.ChatID,
		OpenID:    ev.Operator.OpenID,
		Value:     ev.Action.Value,
	})
}
