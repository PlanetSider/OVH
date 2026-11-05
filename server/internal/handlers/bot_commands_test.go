package handlers

import (
	"strings"
	"testing"

	"github.com/ovh-buy/server/internal/app"
)

// 命令路由的纯逻辑测试：只覆盖不依赖 DB/OVH 的分支。
func TestBotCommandReplyRouting(t *testing.T) {
	state := &app.State{}

	if _, ok := BotCommandReply(state, nil, "hello"); ok {
		t.Fatal("普通文本不应被识别为命令")
	}
	if _, ok := BotCommandReply(state, nil, "   "); ok {
		t.Fatal("空白文本不应被识别为命令")
	}

	reply, ok := BotCommandReply(state, nil, "/help")
	if !ok {
		t.Fatal("/help 应被识别")
	}
	if !strings.Contains(reply, "飞书通道") {
		t.Fatalf("/help 应说明飞书通道能力: %q", reply)
	}

	reply, ok = BotCommandReply(state, nil, "/watch 24sk602 gra")
	if !ok || !strings.Contains(reply, "监控") {
		t.Fatalf("/watch 在 mon 为空时应给出说明: ok=%v reply=%q", ok, reply)
	}

	reply, ok = BotCommandReply(state, nil, "/nope")
	if !ok || !strings.Contains(reply, "不认识的命令") {
		t.Fatalf("未知命令应有提示: ok=%v reply=%q", ok, reply)
	}

	// /help@some_bot 这类群写法要去掉 @ 后缀
	if _, ok := BotCommandReply(state, nil, "/help@ovh_bot"); !ok {
		t.Fatal("@bot 后缀的命令应被识别")
	}
}
