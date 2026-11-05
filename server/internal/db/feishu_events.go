package db

import (
	"fmt"
	"time"
)

// TryClaimFeishuEvent 幂等认领一个飞书事件。
//
// claimed=true 表示这条消息是第一次处理；false 表示平台重推或长连接断线重连后的重放。
// 飞书长连接在 ack 丢失时会把同一个 event 再推一遍，命令里有 /cancel 这类会改队列的操作，
// 重复执行必须挡住。与 telegram_updates 同一套 INSERT OR IGNORE + RowsAffected 判定。
//
// 没有 event_id 的事件（理论上不会出现）不写表，按首次处理，避免把正常消息挡掉。
func (db *DB) TryClaimFeishuEvent(eventID string) (claimed bool, err error) {
	if eventID == "" {
		return true, nil
	}
	res, err := db.Exec(
		`INSERT OR IGNORE INTO feishu_events (event_id, created_at) VALUES (?, ?)`,
		eventID, time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return false, fmt.Errorf("claim feishu event: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// CleanupFeishuEvents 删除 created_at < beforeISO 的旧记录，防止表无限增长。
func (db *DB) CleanupFeishuEvents(beforeISO string) (int64, error) {
	res, err := db.Exec(`DELETE FROM feishu_events WHERE created_at < ?`, beforeISO)
	if err != nil {
		return 0, fmt.Errorf("cleanup feishu events: %w", err)
	}
	return res.RowsAffected()
}
