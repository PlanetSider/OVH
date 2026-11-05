package exchange

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
	// 内嵌 tzdata：容器里常常没有 /usr/share/zoneinfo，
	// 没有它 LoadLocation("Asia/Shanghai") 会失败，"0 点刷新"就退化成固定时区。
	_ "time/tzdata"

	"github.com/ovh-buy/server/internal/app"
)

// Service 持有汇率快照并负责每天 0 点刷新。
//
// 生命周期跟着进程走：main 里 Init 一次，之后 handlers / monitor 通过包级
// Current() / CNY() 读取，不往 app.State 里塞字段（State 已被大量结构体嵌入，
// 加字段会牵连所有构造点；monitor 的 SetMonitorRef 也是同一套做法）。
type Service struct {
	mu          sync.RWMutex
	state       *app.State
	snap        Snapshot
	fetchedAt   time.Time
	lastAttempt time.Time
	lastErr     error
	loc         *time.Location
}

var (
	svcMu sync.RWMutex
	svc   *Service
)

// Init 创建并启动汇率服务。重复调用只替换实例，旧实例的循环会在下一次
// 唤醒时读到新 svc 之前继续跑——测试里会这么用，生产只调用一次。
func Init(state *app.State) *Service {
	s := &Service{state: state, loc: resolveLocation()}
	svcMu.Lock()
	svc = s
	svcMu.Unlock()

	next := nextMidnight(time.Now(), s.loc)
	state.Logger.Info(fmt.Sprintf("汇率服务启动：刷新时区 %s，下次刷新 %s",
		s.loc.String(), formatTime(next, s.loc)), "exchange")
	go s.run()
	return s
}

// Current 返回当前实例；未初始化时为 nil。
func Current() *Service {
	svcMu.RLock()
	defer svcMu.RUnlock()
	return svc
}

// CurrentSnapshot 返回当前快照副本；服务未启动（测试、极早期请求）时
// 返回 inactive 空快照，前端据此不显示人民币。
func CurrentSnapshot() Snapshot {
	s := Current()
	if s == nil {
		return Snapshot{}
	}
	return s.Snapshot()
}

// CNY 包级便捷换算。服务未启动、汇率失效、币种不在表内都返回 false，
// 调用方要做的只有一件事：不显示。
func CNY(currency string, amount float64) (float64, bool) {
	s := Current()
	if s == nil {
		return 0, false
	}
	return s.CNY(currency, amount)
}

// CNY 把 amount（单位 currency）换算成人民币。
// 只做乘法：不做四舍五入以外的任何加工，四舍五入留给 FormatCNY。
func (s *Service) CNY(currency string, amount float64) (float64, bool) {
	if amount <= 0 {
		return 0, false
	}
	c := strings.ToUpper(strings.TrimSpace(currency))
	if c == "" {
		return 0, false
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.snap.Active || time.Since(s.fetchedAt) >= RateTTL {
		return 0, false
	}
	rate := s.snap.Rates[c]
	if rate <= 0 {
		return 0, false
	}
	return amount * rate, true
}

// Snapshot 返回给前端的快照副本。NextAt 每次现算，避免长时间运行后
// 界面显示"下次刷新"停在上一次的时间。
func (s *Service) Snapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := Snapshot{
		Active:    s.snap.Active && time.Since(s.fetchedAt) < RateTTL,
		Base:      s.snap.Base,
		UpdatedAt: s.snap.FetchedAt,
		FetchedAt: s.snap.FetchedAt,
		Provider:  s.snap.Provider,
		NextAt:    nextMidnight(time.Now(), s.loc),
	}
	if len(s.snap.Rates) > 0 {
		out.Rates = make(map[string]float64, len(s.snap.Rates))
		for k, v := range s.snap.Rates {
			out.Rates[k] = v
		}
	}
	return out
}

// run 启动时先拉一次（不等到 0 点），之后每天本地 00:00 刷新；
// 失败则最多每小时重试一次，但不会越过下一个 0 点。
func (s *Service) run() {
	s.refresh()
	for {
		time.Sleep(s.nextWait())
		s.refresh()
	}
}

func (s *Service) nextWait() time.Duration {
	wait := time.Until(nextMidnight(time.Now(), s.loc))

	s.mu.RLock()
	failed := s.lastErr != nil
	s.mu.RUnlock()

	if failed && wait > retryDelay {
		wait = retryDelay
	}
	if wait < time.Minute {
		wait = time.Minute
	}
	return wait
}

func (s *Service) refresh() {
	start := time.Now()
	res, err := fetchRates(context.Background())

	s.mu.Lock()
	s.lastAttempt = start
	if err != nil {
		s.lastErr = err
		// 保留旧 rates 但置 inactive：Active=false 时 CNY() 一律拒绝，
		// 旧值只用于日志诊断，绝不流到界面。
		s.snap.Active = false
		s.mu.Unlock()
		s.state.Logger.Warn("汇率刷新失败："+err.Error()+"（人民币参考价暂停显示，1 小时后重试）", "exchange")
		return
	}

	ratios := buildCNYRatios(res.Rates)
	s.lastErr = nil
	s.fetchedAt = start
	s.snap = Snapshot{
		Active:    true,
		Base:      "USD",
		Rates:     ratios,
		UpdatedAt: start,
		FetchedAt: start,
		Provider:  res.Provider,
	}
	s.mu.Unlock()

	s.state.Logger.Info(fmt.Sprintf(
		"汇率已更新（%s）：1 EUR = ¥%.2f，1 USD = ¥%.2f，1 CAD = ¥%.2f（下次 %s）",
		res.Provider, ratios["EUR"], ratios["USD"], ratios["CAD"],
		formatTime(nextMidnight(time.Now(), s.loc), s.loc)), "exchange")
}

// resolveLocation 解析刷新时区。EXCHANGE_TZ 控制；无法解析时退到固定 +8，
// 而不是静默用 UTC——按 UTC 0 点刷新在国内部署时是早上 8 点，等于功能没生效。
func resolveLocation() *time.Location {
	name := strings.TrimSpace(os.Getenv("EXCHANGE_TZ"))
	if name == "" {
		name = DefaultTZ
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return time.FixedZone("CST", 8*3600)
	}
	return loc
}
