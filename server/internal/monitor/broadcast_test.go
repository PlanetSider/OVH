package monitor

import (
	"strings"
	"testing"
	"time"

	"github.com/ovh-buy/server/internal/types"
)

func TestNormalizeTaskBroadcastTime(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{"empty uses default", "", "09:00", false},
		{"padded", "08:05", "08:05", false},
		{"unpadded hour", "9:05", "09:05", false},
		{"midnight", "00:00", "00:00", false},
		{"last minute", "23:59", "23:59", false},
		{"missing colon", "0900", "", true},
		{"hour overflow", "24:00", "", true},
		{"minute overflow", "12:60", "", true},
		{"negative", "-1:00", "", true},
		{"non numeric", "ab:cd", "", true},
		{"trailing colon", "12:", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := types.NormalizeTaskBroadcastTime(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q, got %q", tc.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.in, err)
			}
			if got != tc.want {
				t.Fatalf("NormalizeTaskBroadcastTime(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestNextDailyBroadcastAt(t *testing.T) {
	loc := broadcastLocation()
	// 北京时间 08:30，今天的 09:00 还没到 → 今天。
	now := time.Date(2026, 11, 6, 8, 30, 0, 0, loc)
	if got, want := nextDailyBroadcastAt(now, "09:00", loc), time.Date(2026, 11, 6, 9, 0, 0, 0, loc); !got.Equal(want) {
		t.Fatalf("before target: got %s, want %s", got, want)
	}
	// 已过 → 明天。
	now = time.Date(2026, 11, 6, 10, 0, 0, 0, loc)
	if got, want := nextDailyBroadcastAt(now, "09:00", loc), time.Date(2026, 11, 7, 9, 0, 0, 0, loc); !got.Equal(want) {
		t.Fatalf("after target: got %s, want %s", got, want)
	}
	// 正好等于 → 明天（否则当分钟会连发）。
	now = time.Date(2026, 11, 6, 9, 0, 0, 0, loc)
	if got, want := nextDailyBroadcastAt(now, "09:00", loc), time.Date(2026, 11, 7, 9, 0, 0, 0, loc); !got.Equal(want) {
		t.Fatalf("exactly target: got %s, want %s", got, want)
	}
	// 非法时间回退到默认 09:00，而不是 panic。
	now = time.Date(2026, 11, 6, 8, 0, 0, 0, loc)
	if got, want := nextDailyBroadcastAt(now, "99:99", loc), time.Date(2026, 11, 6, 9, 0, 0, 0, loc); !got.Equal(want) {
		t.Fatalf("invalid time: got %s, want %s", got, want)
	}
	// 跨月边界。
	now = time.Date(2026, 11, 30, 23, 0, 0, 0, loc)
	if got, want := nextDailyBroadcastAt(now, "09:00", loc), time.Date(2026, 12, 1, 9, 0, 0, 0, loc); !got.Equal(want) {
		t.Fatalf("month boundary: got %s, want %s", got, want)
	}
}

func TestSplitBroadcastTextKeepsRunesIntact(t *testing.T) {
	text := strings.Repeat("测", 10) // 30 bytes
	chunks := splitBroadcastText(text, 9)
	if len(chunks) != 4 {
		t.Fatalf("expected 4 chunks, got %d", len(chunks))
	}
	for _, chunk := range chunks {
		if len(chunk) > 9 {
			t.Fatalf("chunk exceeds limit: %d bytes", len(chunk))
		}
	}
	if joined := strings.Join(chunks, ""); joined != text {
		t.Fatalf("rejoined text differs")
	}
}

func TestFormatBroadcastItemsSplitsLongLists(t *testing.T) {
	items := make([]broadcastItem, 0, 40)
	for i := 0; i < 40; i++ {
		items = append(items, broadcastItem{
			AccountID:   "acc1",
			Account:     "主账号",
			Model:       strings.Repeat("机", 30),
			PlanCode:    "24sk602",
			Datacenters: "GRA",
			Monthly:     "€42.99 ≈ ¥307",
		})
	}
	messages := formatBroadcastItems(items, func(part, total int) string {
		return broadcastCategoryHeader("抢购任务", len(items), part, total)
	})
	if len(messages) < 2 {
		t.Fatalf("expected multiple parts, got %d", len(messages))
	}
	if !strings.Contains(messages[0], "（1/") {
		head := messages[0]
		if len(head) > 80 {
			head = head[:80]
		}
		t.Fatalf("first message missing part index: %q", head)
	}
	for _, msg := range messages {
		// 允许标题预留的少量溢出，但不允许接近整条消息上限的暴涨。
		if len(msg) > broadcastMessageMaxBytes+broadcastPartTitleReserve {
			t.Fatalf("message too large: %d bytes", len(msg))
		}
	}
}

func TestAggregateQueueTasks(t *testing.T) {
	items := []types.QueueItem{
		{AccountID: "a1", PlanCode: "24sk602", Datacenter: "gra", Options: []string{"ram-64g"}, Status: "running"},
		{AccountID: "a1", PlanCode: "24sk602", Datacenter: "GRA", Options: []string{"ram-64g"}, Status: "pending"},
		{AccountID: "a1", PlanCode: "24sk602", Datacenter: "bhz", Options: []string{"ram-64g"}, Status: "paused"},
		{AccountID: "a1", PlanCode: "24sk602", Datacenter: "gra", Options: []string{"ram-32g"}, Status: "running"},
		{AccountID: "a1", PlanCode: "24sk602", Datacenter: "gra", Options: []string{"ram-64g"}, Status: "completed"},
		{AccountID: "a1", PlanCode: "24sk602", Datacenter: "gra", Options: []string{"ram-64g"}, Status: "failed"},
	}
	groups := aggregateQueueTasks(items)
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d: %+v", len(groups), groups)
	}
	byKey := map[string]queueBroadcastGroup{}
	for _, group := range groups {
		byKey[strings.Join(group.Options, ",")] = group
	}
	sixtyFour, ok := byKey["ram-64g"]
	if !ok {
		t.Fatalf("missing ram-64g group: %+v", groups)
	}
	if sixtyFour.DatacenterCounts["GRA"] != 2 || sixtyFour.DatacenterCounts["BHZ"] != 1 {
		t.Fatalf("unexpected datacenter counts: %+v", sixtyFour.DatacenterCounts)
	}
	if _, ok := byKey["ram-32g"]; !ok {
		t.Fatalf("missing ram-32g group: %+v", groups)
	}
}

func TestIsUnpaidBattleStatus(t *testing.T) {
	unpaid := []string{"", "notPaid", "unpaid", "paymentPending", "waiting-payment", "PENDING", "created", "new"}
	for _, status := range unpaid {
		if !isUnpaidBattleStatus(status) {
			t.Fatalf("%q should be treated as unpaid", status)
		}
	}
	paid := []string{"delivered", "cancelled", "cancelling", "checking", "documentsRequested"}
	for _, status := range paid {
		if isUnpaidBattleStatus(status) {
			t.Fatalf("%q should not be treated as unpaid", status)
		}
	}
}

func TestFormatBattleReportWindowAndAmounts(t *testing.T) {
	now := time.Now()
	amount := 42.99
	paidAmount := 10.5
	history := []types.PurchaseHistoryEntry{
		{
			AccountID: "a1", Status: "success",
			PurchaseTime: now.Add(-1 * time.Hour).Format(time.RFC3339),
			OrderStatus:  "notPaid",
			Price:        &types.PriceInfo{WithoutTax: &amount, CurrencyCode: "EUR"},
		},
		{
			AccountID: "a1", Status: "success",
			PurchaseTime: now.Add(-2 * time.Hour).Format(time.RFC3339),
			OrderStatus:  "delivered",
			Price:        &types.PriceInfo{WithoutTax: &paidAmount, CurrencyCode: "EUR"},
		},
		{
			// 窗口外，不能计入。
			AccountID: "a1", Status: "success",
			PurchaseTime: now.Add(-30 * time.Hour).Format(time.RFC3339),
			OrderStatus:  "delivered",
			Price:        &types.PriceInfo{WithoutTax: &amount, CurrencyCode: "EUR"},
		},
		{
			// 失败单不进战报。
			AccountID: "a1", Status: "failed",
			PurchaseTime: now.Format(time.RFC3339),
		},
	}
	text := formatBattleReport(history, now, map[string]string{"a1": "主账号"})
	if !strings.Contains(text, "OVH 账号：主账号") {
		t.Fatalf("missing account label: %s", text)
	}
	if !strings.Contains(text, "未支付订单：1 单") || !strings.Contains(text, "已支付订单：1 单") {
		t.Fatalf("unexpected order counts: %s", text)
	}
	if !strings.Contains(text, "过去 24 小时成功下单：2") {
		t.Fatalf("unexpected success count: %s", text)
	}
	if !strings.Contains(text, "€42.99") || !strings.Contains(text, "€10.50") {
		t.Fatalf("unexpected amounts: %s", text)
	}
}
