// Package exchange 维护"外币 → 人民币"的参考汇率，只用于展示层的副标注。
//
// 三条铁律，和项目里"币种缺失就留空、绝不冒充欧元"是同一套思路：
//   - 拉不到汇率就 active=false，调用方不显示人民币，绝不拿旧汇率冒充最新；
//   - 换算只做乘法，不参与询价/下单/库存判定；
//   - 汇率超过 RateTTL 直接失效，宁可不显示，也不给一个过期的数字。
package exchange

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultTZ 默认刷新时区。必须显式：容器默认 UTC，按 UTC 0 点刷新
	// 在国内部署时其实是早上 8 点，用户看到的是"汇率整天没变"。
	DefaultTZ = "Asia/Shanghai"
	// RateTTL 汇率超过这个时长就不再用于换算。
	// 一天一刷的前提下，48h 意味着"至少连续两次刷新失败"，这时沉默比旧值诚实。
	RateTTL = 48 * time.Hour
	// retryDelay 拉取失败后的重试间隔（不越过下一个 0 点）。
	retryDelay = time.Hour
)

// SupportedCurrencies 是 OVH 各子公司可能用到的币种，与
// internal/monitor/price.go 的 subsidiaryCurrency 同一维度。
// 表外的币种即使汇率源里有值也不做换算——显示层没有对应的符号规则，
// 多换一个反而会出现"¥ 有值、原币没符号"的怪组合。
var SupportedCurrencies = []string{
	"EUR", "USD", "GBP", "PLN", "MAD", "TND", "XOF", "CAD", "AUD", "INR", "SGD",
}

// Snapshot 是给前端用的一次汇率快照。
// Rates 的含义是"1 单位该币种 = N 人民币"，前端拿到后直接乘，不需要知道 USD 基准。
type Snapshot struct {
	Active    bool               `json:"active"`
	Base      string             `json:"base,omitempty"`
	Rates     map[string]float64 `json:"rates,omitempty"`
	UpdatedAt time.Time          `json:"updatedAt,omitempty"`
	FetchedAt time.Time          `json:"fetchedAt,omitempty"`
	Provider  string             `json:"provider,omitempty"`
	NextAt    time.Time          `json:"nextAt,omitempty"`
}

// cnyPerUnit 从 USD 基准的原始汇率算出"1 单位该币种 = ? CNY"。
//
// 交叉汇率：1 EUR = 1/usdRates[EUR] USD = usdRates[CNY]/usdRates[EUR] CNY。
// 直接把 usdRates[EUR] 当人民币汇率用是最常见的错法（EUR 实际比 USD 贵，会算反）。
func cnyPerUnit(usdRates map[string]float64, currency string) (float64, bool) {
	c := strings.ToUpper(strings.TrimSpace(currency))
	if c == "" {
		return 0, false
	}
	cny := usdRates["CNY"]
	rate := usdRates[c]
	if cny <= 0 || rate <= 0 {
		return 0, false
	}
	return cny / rate, true
}

func buildCNYRatios(usdRates map[string]float64) map[string]float64 {
	out := make(map[string]float64, len(SupportedCurrencies))
	for _, c := range SupportedCurrencies {
		if v, ok := cnyPerUnit(usdRates, c); ok {
			out[c] = v
		}
	}
	return out
}

// FormatCNY 参考价统一取整。
//
// 保留两位小数会制造"同价不同 ¥"的假精度：€42.99 与 €43.00 都是 ≈ ¥333.5x，
// 四舍五入后一个 334 一个 334 还好，差一分钱就变成 333/334，看起来像两档价格。
func FormatCNY(v float64) string {
	if v <= 0 {
		return ""
	}
	n := int64(math.Round(v))
	s := strconv.FormatInt(n, 10)
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	if neg {
		return "-¥" + b.String()
	}
	return "¥" + b.String()
}

// nextMidnight 返回 loc 时区下"明天 00:00"。用 Date(y,m,d+1) 而不是
// Truncate(24h)：后者在非 UTC 时区、夏令时切换日都会偏。
func nextMidnight(now time.Time, loc *time.Location) time.Time {
	n := now.In(loc)
	return time.Date(n.Year(), n.Month(), n.Day()+1, 0, 0, 0, 0, loc)
}

// formatTime 用于日志：把时间按刷新时区渲染，避免部署在 UTC 的机器上
// 日志里出现"下次刷新 16:00"这种要用户自己换算的时间。
func formatTime(t time.Time, loc *time.Location) string {
	if t.IsZero() {
		return "-"
	}
	return t.In(loc).Format("2006-01-02 15:04")
}

// displayMode 的判据统一在 types.NormalizeDisplayMode：保存设置、通知、
// 前端三处必须同一套规则，放在这里多一份实现迟早漂移。

// String 给日志用的简短描述。
func (s Snapshot) String() string {
	if !s.Active {
		return "inactive"
	}
	return fmt.Sprintf("active(%s, %d currencies, updated %s)",
		s.Provider, len(s.Rates), s.UpdatedAt.Format(time.RFC3339))
}
