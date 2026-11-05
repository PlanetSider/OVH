package feishu

import (
	"context"
	"fmt"

	channeltypes "github.com/larksuite/oapi-sdk-go/v3/channel/types"
)

// sendText 回复一条纯文本消息。
//
// replyTo 非空时作为「回复某条消息」发送（飞书里会带引用条），
// 命令回复带上它，用户能一眼看出这句是回哪条，比新开一条更清楚。
func sendText(ctx context.Context, ch channeltypes.Channel, chatID, replyTo, text string) error {
	if ch == nil || chatID == "" || text == "" {
		return nil
	}
	_, err := ch.Send(ctx, &channeltypes.SendInput{
		ChatID:         chatID,
		Text:           text,
		ReplyMessageID: replyTo,
	})
	if err != nil {
		return fmt.Errorf("send feishu text: %w", err)
	}
	return nil
}
