package feishu

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	channeltypes "github.com/larksuite/oapi-sdk-go/v3/channel/types"
)

// SendAlert 把一条补货通知发到绑定的飞书私聊。
//
// replyMarkup 是 Telegram 用的 inline keyboard；这里会把每个按钮的
// callback_data 原样放进飞书卡片按钮的 callback value。两边共享同一个 UUID，
// 所以同一条通知在 Telegram 和飞书各点一次，只有一次能真正入队
// （服务端 telegram_order_buttons 的原子 claim 挡住第二次）。
func SendAlert(message string, replyMarkup map[string]interface{}) error {
	botMu.Lock()
	b := bot
	botMu.Unlock()
	if b == nil {
		return errors.New("飞书机器人未初始化")
	}
	return b.sendAlert(message, replyMarkup)
}

func (b *Bot) sendAlert(message string, replyMarkup map[string]interface{}) error {
	if b == nil || b.state == nil {
		return errors.New("飞书机器人未初始化")
	}
	cfg := b.state.Config.Get()
	if !cfg.FeishuEnabled || !cfg.FeishuConfigured() {
		return errors.New("飞书机器人未启用或凭据不全")
	}
	if cfg.FeishuBoundOpenID == "" {
		return errors.New("飞书还没有绑定接收人")
	}

	b.mu.Lock()
	ch := b.ch
	b.mu.Unlock()
	if ch == nil {
		return errors.New("飞书长连接未就绪")
	}

	card, err := buildAlertCard(message, replyMarkup)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// UserID 传 open_id：SDK 默认 receive_id_type=open_id
	if _, err := ch.Send(ctx, &channeltypes.SendInput{UserID: cfg.FeishuBoundOpenID, Card: card}); err != nil {
		return fmt.Errorf("send feishu card: %w", err)
	}
	return nil
}

// markupJSON 只取 inline keyboard 里飞书需要的那两个字段。
//
// replyMarkup 里的 keyboard 是 monitor 包内的一个未导出结构体切片，
// 跨包没法直接断言，先转一圈 JSON 反而最稳（也是它最终发给 Telegram 的形态）。
type markupJSON struct {
	InlineKeyboard [][]struct {
		Text         string `json:"text"`
		CallbackData string `json:"callback_data"`
	} `json:"inline_keyboard"`
}

func buildAlertCard(message string, replyMarkup map[string]interface{}) (string, error) {
	elements := []map[string]interface{}{
		{
			"tag":  "div",
			"text": map[string]interface{}{"tag": "plain_text", "content": message},
		},
	}

	if replyMarkup != nil {
		raw, err := json.Marshal(replyMarkup)
		if err == nil {
			var mk markupJSON
			if json.Unmarshal(raw, &mk) == nil {
				for _, row := range mk.InlineKeyboard {
					for _, btn := range row {
						value := map[string]interface{}{}
						// callback_data 是 {"a":"add_to_queue","u":"<uuid>"}
						if btn.CallbackData != "" {
							if err := json.Unmarshal([]byte(btn.CallbackData), &value); err != nil {
								// 解析不了就不带行为，宁可按钮点了没反应也不要发错参数
								value = map[string]interface{}{}
							}
						}
						elements = append(elements, map[string]interface{}{
							"tag":  "button",
							"text": map[string]interface{}{"tag": "plain_text", "content": btn.Text},
							"type": "primary",
							"behaviors": []map[string]interface{}{
								{"type": "callback", "value": value},
							},
						})
					}
				}
			}
		}
	}

	card := map[string]interface{}{
		"schema": "2.0",
		"config": map[string]interface{}{"update_multi": true},
		"header": map[string]interface{}{
			"title":    map[string]interface{}{"tag": "plain_text", "content": "OVH 补货提醒"},
			"template": "blue",
		},
		"body": map[string]interface{}{"elements": elements},
	}
	out, err := json.Marshal(card)
	if err != nil {
		return "", fmt.Errorf("marshal alert card: %w", err)
	}
	return string(out), nil
}
