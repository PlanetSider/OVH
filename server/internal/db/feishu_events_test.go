package db

import "testing"

// 飞书事件幂等：同一条 event 只认领一次，重放返回 false；
// 没有 event_id 的事件按首次处理，不能把正常消息挡掉。
func TestFeishuEventClaimIsIdempotent(t *testing.T) {
	database, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer database.Close()

	claimed, err := database.TryClaimFeishuEvent("evt_001")
	if err != nil || !claimed {
		t.Fatalf("首次认领应成功: claimed=%v err=%v", claimed, err)
	}

	claimed, err = database.TryClaimFeishuEvent("evt_001")
	if err != nil {
		t.Fatalf("重复认领报错: %v", err)
	}
	if claimed {
		t.Fatal("同一个 event_id 第二次不应认领成功")
	}

	claimed, err = database.TryClaimFeishuEvent("evt_002")
	if err != nil || !claimed {
		t.Fatalf("不同 event_id 应独立认领: claimed=%v err=%v", claimed, err)
	}

	// 没有 event_id：按首次处理，且不写表
	claimed, err = database.TryClaimFeishuEvent("")
	if err != nil || !claimed {
		t.Fatalf("空 event_id 应按首次处理: claimed=%v err=%v", claimed, err)
	}
	var n int
	if err := database.Get(&n, `SELECT COUNT(*) FROM feishu_events`); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 2 {
		t.Fatalf("幂等表应只有 2 条记录，实际 %d", n)
	}
}

func TestCleanupFeishuEvents(t *testing.T) {
	database, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer database.Close()

	if _, err := database.TryClaimFeishuEvent("old"); err != nil {
		t.Fatalf("claim: %v", err)
	}
	// 把所有记录的时间改到过去，再按一个更晚的时间点清理
	if _, err := database.Exec(`UPDATE feishu_events SET created_at = '2020-01-01T00:00:00Z'`); err != nil {
		t.Fatalf("update: %v", err)
	}
	removed, err := database.CleanupFeishuEvents("2021-01-01T00:00:00Z")
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if removed != 1 {
		t.Fatalf("应删除 1 条，实际 %d", removed)
	}
}
