// Package feishu 负责飞书自建应用机器人:扫码注册(register.go)、长连接(bot.go)。
//
// 只有这个包 import larksuite/oapi-sdk-go —— 业务层(handlers/命令/卡片)
// 通过 bot.go 暴露的少数函数与它交互,避免 SDK 类型渗到整个项目。
package feishu

import (
	"context"
	"fmt"
	"sync"
	"time"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	"github.com/larksuite/oapi-sdk-go/v3/channel"
	channeltypes "github.com/larksuite/oapi-sdk-go/v3/channel/types"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"

	"github.com/ovh-buy/server/internal/app"
)

const (
	openBaseFeishu = "https://open.feishu.cn"
	openBaseLark   = "https://open.larksuite.com"
)

// Bot 管理一条飞书长连接的生命周期。
//
// SDK 的连接是"一次性"的:连上过再断开,这个 ws.Client 就作废了,
// 想重连必须新建 client + channel。所以配置变更/断开重连一律走 Reconfigure,
// 而不是在旧对象上再 Start 一次。
type Bot struct {
	mu      sync.Mutex
	state   *app.State
	ch      channeltypes.Channel
	cancel  context.CancelFunc
	done    chan struct{}
	started bool
}

var (
	botMu sync.Mutex
	bot   *Bot
)

// Init 在 main 里调用一次:登记配置变更钩子,并按当前配置尝试连接。
func Init(state *app.State) {
	botMu.Lock()
	if bot == nil {
		bot = &Bot{state: state}
		SetReconfigureHook(bot.Reconfigure)
	}
	b := bot
	botMu.Unlock()

	b.Start()
}

// Start 按当前配置建立长连接。配置不全(未启用/缺凭据)是正常状态,只记日志。
func (b *Bot) Start() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.started {
		return
	}
	if b.state == nil {
		return
	}

	cfg := b.state.Config.Get()
	if !cfg.FeishuEnabled || !cfg.FeishuConfigured() {
		b.state.Logger.Info("飞书机器人未启用或凭据不全,跳过长连接", "feishu")
		return
	}

	openBase := openBaseFeishu
	if cfg.FeishuDomain == "lark" {
		openBase = openBaseLark
	}

	// 日志级别只开 error:SDK 的 info 会把每条收发都打出来,
	// 抢购时一次下单能刷几十行,埋掉我们自己的日志。
	client := lark.NewClient(cfg.FeishuAppID, cfg.FeishuAppSecret,
		lark.WithOpenBaseUrl(openBase),
		lark.WithLogLevel(larkcore.LogLevelError),
	)
	wsClient := larkws.NewClient(cfg.FeishuAppID, cfg.FeishuAppSecret,
		larkws.WithDomain(openBase),
		larkws.WithLogLevel(larkcore.LogLevelError),
	)
	ch := channel.NewChannel(client, wsClient)

	ch.OnReady(func() {
		b.state.Logger.Info("飞书长连接已就绪", "feishu")
	})
	ch.OnReconnecting(func() {
		b.state.Logger.Warn("飞书长连接正在重连", "feishu")
	})
	ch.OnReconnected(func() {
		b.state.Logger.Info("飞书长连接已重连", "feishu")
	})
	ch.OnDisconnected(func() {
		b.state.Logger.Warn("飞书长连接已断开", "feishu")
	})
	ch.OnError(func(err error) {
		b.state.Logger.Warn(fmt.Sprintf("飞书长连接错误: %v", err), "feishu")
	})

	// 消息处理：归一化后交给上层（handlers）解析命令，再把回复发回去。
	// 这里只做"投递 + 回发"，命令语义全在 handlers，避免 SDK 类型往业务层渗。
	ch.OnMessage(func(ctx context.Context, msg *channeltypes.NormalizedMessage) error {
		if msg == nil {
			return nil
		}
		reply := clampReply(dispatchMessage(ctx, msg))
		if reply == "" {
			return nil
		}
		if err := sendText(ctx, ch, msg.ChatID, msg.MessageID, reply); err != nil {
			b.state.Logger.Warn("飞书回复失败: "+err.Error(), "feishu")
		}
		return nil
	})
	ch.OnCardAction(func(ctx context.Context, event *channeltypes.CardActionEvent) error {
		reply := clampReply(dispatchCardAction(ctx, event))
		if reply == "" || event == nil {
			return nil
		}
		if err := sendText(ctx, ch, event.ChatID, event.MessageID, reply); err != nil {
			b.state.Logger.Warn("飞书卡片回调回复失败: "+err.Error(), "feishu")
		}
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	b.cancel = cancel
	b.done = done
	b.ch = ch
	b.started = true

	go func() {
		defer close(done)
		err := ch.Start(ctx)
		b.mu.Lock()
		// 只有还是当前这条连接时才清状态:Reconfigure 可能已经换上新连接了。
		if b.done == done {
			b.started = false
			b.cancel = nil
			b.ch = nil
		}
		b.mu.Unlock()
		if err != nil && ctx.Err() == nil {
			// ctx 没被取消却退出,说明是 SDK 判定的终态错误。
			// 不自动无限重试:凭据错了重试一万次也没用,
			// 让用户改完配置后由 Reconfigure 触发。
			b.state.Logger.Warn(fmt.Sprintf("飞书长连接退出: %v", err), "feishu")
		}
	}()
}

// Stop 断开当前长连接并等它真正退出。
func (b *Bot) Stop() {
	b.mu.Lock()
	if !b.started {
		b.mu.Unlock()
		return
	}
	cancel, done := b.cancel, b.done
	b.started = false
	b.cancel = nil
	b.ch = nil
	b.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		// SDK 的 Stop 没有强制终止语义,超时只能放弃等待,
		// 下一次 Reconfigure 会建新连接,旧 goroutine 由 ctx 取消收尾。
	}
}

// Reconfigure 配置变更后重建连接:旧连接一律丢弃,新配置重新连。
func (b *Bot) Reconfigure() {
	b.Stop()
	b.Start()
}
