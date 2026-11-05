package handlers

import (
	"strings"
	"testing"
	"time"

	"github.com/ovh-buy/server/internal/feishu"
)

// 手动填写的应用没有"扫码即绑定"，绑定码是唯一的信任边界：
// 能生成它 = 已登录控制台；能用它换绑定 = 那个私聊确实是控制台主人的。
func TestFeishuBindingFlow(t *testing.T) {
	st, mon := newWatchTestMonitor(t)
	cfg := st.Config.Get()
	cfg.FeishuEnabled = true
	cfg.FeishuAppID = "cli_test"
	cfg.FeishuAppSecret = "secret"
	cfg.FeishuBoundOpenID = ""
	if err := st.Config.Set(cfg); err != nil {
		t.Fatalf("set config: %v", err)
	}

	// 未绑定时即使发命令也只回绑定引导，不能执行任何队列操作
	reply := HandleFeishuMessage(st, mon, feishu.Message{
		EventID: "ev-1", ChatType: "p2p", UserID: "ou_owner", Text: "/queue",
	})
	if !strings.Contains(reply, "绑定") {
		t.Fatalf("unbound user should be guided to bind, got %q", reply)
	}

	code, err := newFeishuBindingCode()
	if err != nil {
		t.Fatalf("generate code: %v", err)
	}
	if err := st.DB.SetKV(feishuBindingKey, feishuBindingRecord{
		Code:      code,
		ExpiresAt: time.Now().Add(time.Minute).Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("store code: %v", err)
	}

	// 错码不消耗、不绑定
	reply = HandleFeishuMessage(st, mon, feishu.Message{
		EventID: "ev-2", ChatType: "p2p", UserID: "ou_owner", Text: "/bind 000000",
	})
	if !strings.Contains(reply, "不对") {
		t.Fatalf("wrong code should be rejected, got %q", reply)
	}
	if st.Config.Get().FeishuBoundOpenID != "" {
		t.Fatal("wrong code must not bind")
	}

	// 正确码绑定成功
	reply = HandleFeishuMessage(st, mon, feishu.Message{
		EventID: "ev-3", ChatType: "p2p", UserID: "ou_owner", Text: "/bind " + code,
	})
	if !strings.Contains(reply, "绑定成功") {
		t.Fatalf("correct code should bind, got %q", reply)
	}
	if got := st.Config.Get().FeishuBoundOpenID; got != "ou_owner" {
		t.Fatalf("bound open id = %q", got)
	}

	// 绑定码一次性：已绑定后再用同一码必须被挡（不允许顶替操作者）
	reply = HandleFeishuMessage(st, mon, feishu.Message{
		EventID: "ev-4", ChatType: "p2p", UserID: "ou_owner", Text: "/bind " + code,
	})
	if !strings.Contains(reply, "已经绑定") {
		t.Fatalf("re-binding must be refused, got %q", reply)
	}

	// 换个人发命令：拒绝
	reply = HandleFeishuMessage(st, mon, feishu.Message{
		EventID: "ev-5", ChatType: "p2p", UserID: "ou_other", Text: "/queue",
	})
	if !strings.Contains(reply, "不是") {
		t.Fatalf("other user should be rejected, got %q", reply)
	}

	// 群聊静默：任何人不该在群里触发队列操作
	if reply = HandleFeishuMessage(st, mon, feishu.Message{
		EventID: "ev-6", ChatType: "group", UserID: "ou_owner", Text: "/cancel all",
	}); reply != "" {
		t.Fatalf("group chat must be ignored, got %q", reply)
	}

	// 同一事件重放：幂等，第二次不回
	dup := feishu.Message{EventID: "ev-7", ChatType: "p2p", UserID: "ou_owner", Text: "/queue"}
	if reply = HandleFeishuMessage(st, mon, dup); reply == "" {
		t.Fatal("first delivery should reply")
	}
	if reply = HandleFeishuMessage(st, mon, dup); reply != "" {
		t.Fatalf("duplicate event must be silent, got %q", reply)
	}
}
