// Package feishu 负责飞书自建应用机器人:扫码注册(register.go)、长连接(bot.go)。
//
// 只有这个包 import larksuite/oapi-sdk-go —— 业务层(handlers/命令/卡片)
// 通过 bot.go 暴露的少数函数与它交互,避免 SDK 类型渗到整个项目。
package feishu

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	"github.com/larksuite/oapi-sdk-go/v3/channel"
	channeltypes "github.com/larksuite/oapi-sdk-go/v3/channel/types"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	larkevent "github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
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
	mu          sync.Mutex
	state       *app.State
	ch          channeltypes.Channel
	cancel      context.CancelFunc
	done        chan struct{}
	started     bool
	connected   bool
	connectedAt time.Time
	lastEventAt time.Time
	lastError   string
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
	// 这一步不能省：SDK 的 ws 客户端在 eventHandler == nil 时会把所有事件直接丢掉
	// （ws/client_message.go 的 handleDataFrame 第一行判断），而 channel 的
	// OnMessage/OnCardAction 又要用 wsClient.EventHandler() 拿同一个 dispatcher
	// 去注册 handler。漏了它的症状很迷惑：长连接显示"已就绪"，但消息和卡片
	// 回调一条都收不到。
	dispatcher := larkevent.NewEventDispatcher("", "")
	wsClient := larkws.NewClient(cfg.FeishuAppID, cfg.FeishuAppSecret,
		larkws.WithDomain(openBase),
		larkws.WithEventHandler(dispatcher),
		larkws.WithLogLevel(larkcore.LogLevelError),
	)
	ch := channel.NewChannel(client, wsClient)

	ch.OnReady(func() {
		b.markConnected()
		b.state.Logger.Info("飞书长连接已就绪", "feishu")
		// 配置刚变更/刚扫码创建时,这条连接是异步起来的:保存那一刻 ch 还是 nil,
		// 直接发必然失败。所以由保存方登记意图,这里连上了再兑现。
		if takeTestMessage() {
			go b.sendTestMessage(ch)
		}
	})
	ch.OnReconnecting(func() {
		b.state.Logger.Warn("飞书长连接正在重连", "feishu")
	})
	ch.OnReconnected(func() {
		b.markConnected()
		b.state.Logger.Info("飞书长连接已重连", "feishu")
	})
	ch.OnDisconnected(func() {
		b.markDisconnected()
		b.state.Logger.Warn("飞书长连接已断开", "feishu")
	})
	ch.OnError(func(err error) {
		b.markError(err)
		b.state.Logger.Warn(fmt.Sprintf("飞书长连接错误: %v", err), "feishu")
	})

	// 消息处理：归一化后交给上层（handlers）解析命令，再把回复发回去。
	// 这里只做"投递 + 回发"，命令语义全在 handlers，避免 SDK 类型往业务层渗。
	ch.OnMessage(func(ctx context.Context, msg *channeltypes.NormalizedMessage) error {
		if msg == nil {
			return nil
		}
		b.markEvent()
		// 这条日志是排查"发消息没反应"的入口:能看到消息到了,才谈得上命令解析;
		// 看不到就说明长连接/事件订阅/权限那一段没通。
		b.state.Logger.Info(fmt.Sprintf("收到飞书消息: chat=%s type=%s user=%s text=%q",
			clipRunes(msg.ChatID, 14), msg.ChatType, clipRunes(msg.UserID, 14), clipRunes(msg.Content, 40)), "feishu")
		reply := clampReply(dispatchMessage(ctx, msg))
		if reply == "" {
			b.state.Logger.Warn("飞书消息没有产生回复(未绑定/非私聊/未授权/重复事件/处理器未注册)", "feishu")
			return nil
		}
		if err := sendText(ctx, ch, msg.ChatID, msg.MessageID, reply); err != nil {
			b.state.Logger.Warn("飞书回复失败: "+err.Error(), "feishu")
		}
		return nil
	})
	ch.OnCardAction(func(ctx context.Context, event *channeltypes.CardActionEvent) error {
		if event != nil {
			b.markEvent()
		}
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
			b.connected = false
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
	b.connected = false
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

// BotStatus 是设置页「连接状态」卡片读的快照。
//
// 和 Telegram 那边不同:飞书走长连接,没有 offset / 最后轮询这种概念,
// 能说明"它到底在不在收消息"的只有连接状态、最后连接时间和最后事件时间。
type BotStatus struct {
	Enabled     bool   `json:"enabled"`
	Configured  bool   `json:"configured"`
	Bound       bool   `json:"bound"`
	Started     bool   `json:"started"`
	Connected   bool   `json:"connected"`
	ConnectedAt string `json:"connectedAt,omitempty"`
	LastEventAt string `json:"lastEventAt,omitempty"`
	LastError   string `json:"lastError,omitempty"`
	AppID       string `json:"appId,omitempty"`
	Domain      string `json:"domain,omitempty"`
}

// CurrentStatus 返回包级 bot 的状态快照。bot 还没初始化(或未配置)时返回空快照。
func CurrentStatus() BotStatus {
	botMu.Lock()
	b := bot
	botMu.Unlock()
	if b == nil {
		return BotStatus{}
	}
	return b.status()
}

func (b *Bot) status() BotStatus {
	if b == nil || b.state == nil {
		return BotStatus{}
	}
	b.mu.Lock()
	started, connected := b.started, b.connected
	connectedAt, lastEventAt, lastError := b.connectedAt, b.lastEventAt, b.lastError
	b.mu.Unlock()

	cfg := b.state.Config.Get()
	return BotStatus{
		Enabled:     cfg.FeishuEnabled,
		Configured:  cfg.FeishuConfigured(),
		Bound:       strings.TrimSpace(cfg.FeishuBoundOpenID) != "",
		Started:     started,
		Connected:   connected,
		ConnectedAt: formatStatusTime(connectedAt),
		LastEventAt: formatStatusTime(lastEventAt),
		LastError:   lastError,
		AppID:       cfg.FeishuAppID,
		Domain:      cfg.FeishuDomain,
	}
}

func (b *Bot) markConnected() {
	b.mu.Lock()
	b.connected = true
	b.connectedAt = time.Now()
	// 连上就把上一次的错误清掉,否则界面会一直显示一个早就过去的故障。
	b.lastError = ""
	b.mu.Unlock()
}

func (b *Bot) markDisconnected() {
	b.mu.Lock()
	b.connected = false
	b.mu.Unlock()
}

func (b *Bot) markError(err error) {
	if err == nil {
		return
	}
	b.mu.Lock()
	b.lastError = err.Error()
	b.mu.Unlock()
}

func (b *Bot) markEvent() {
	b.mu.Lock()
	b.lastEventAt = time.Now()
	b.mu.Unlock()
}

// testMessageText 是连接建立后自动发的那条确认消息。
// 和 Telegram 保存 Token 后发的测试消息对齐:让用户立刻知道"这条路通了",
// 而不是等到下一次补货没收到才怀疑配置。
const testMessageText = "OVH 控制台：飞书机器人已连接。\n发 /help 查看可用命令。"

var (
	testMu      sync.Mutex
	pendingTest bool
)

// QueueTestMessage 登记"下一条连接就绪时发一条测试消息"。
//
// 只登记不发送:配置变更触发的是异步重连,此刻还没有可用的 Channel。
func QueueTestMessage() {
	testMu.Lock()
	pendingTest = true
	testMu.Unlock()
}

func takeTestMessage() bool {
	testMu.Lock()
	defer testMu.Unlock()
	if !pendingTest {
		return false
	}
	pendingTest = false
	return true
}

func (b *Bot) sendTestMessage(ch channeltypes.Channel) {
	if err := b.sendTestCore(ch); err != nil {
		b.state.Logger.Warn("飞书测试消息未发送: "+err.Error(), "feishu")
	}
}

// sendTestCore 是"配置变更后自动发"和网页"测试发送"按钮共用的发送逻辑。
func (b *Bot) sendTestCore(ch channeltypes.Channel) error {
	cfg := b.state.Config.Get()
	openID := strings.TrimSpace(cfg.FeishuBoundOpenID)
	if openID == "" {
		// 手动填凭据但还没绑定的人会走到这里:连上了却没有收件人。
		return fmt.Errorf("还没有绑定接收人")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := ch.Send(ctx, &channeltypes.SendInput{
		UserID:  openID,
		MsgType: "text",
		Text:    testMessageText,
	}); err != nil {
		return err
	}
	b.state.Logger.Info("飞书测试消息已发送", "feishu")
	return nil
}

// SendTestNow 立刻发一条测试消息（网页设置页的"测试发送"按钮）。
//
// 和自动测试消息的区别:自动那条要等 OnReady,用户看不到进度;
// 这里把"连接未就绪/未绑定/发送失败"的原因直接带回页面,便于自查。
func SendTestNow() error {
	botMu.Lock()
	b := bot
	botMu.Unlock()
	if b == nil {
		return fmt.Errorf("飞书机器人未初始化")
	}
	return b.sendTestNow()
}

func (b *Bot) sendTestNow() error {
	b.mu.Lock()
	ch := b.ch
	connected := b.connected
	b.mu.Unlock()
	if ch == nil || !connected {
		return fmt.Errorf("长连接尚未就绪，请等几秒后重试；若一直未就绪，看上面的「连接状态」与 lastError")
	}
	return b.sendTestCore(ch)
}

// clipRunes 截断日志里的长字符串,避免一条消息刷满日志。
func clipRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func formatStatusTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}
