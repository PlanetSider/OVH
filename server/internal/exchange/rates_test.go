package exchange

import (
	"math"
	"testing"
	"time"
)

func TestCnyPerUnitCrossRate(t *testing.T) {
	usd := map[string]float64{"CNY": 6.6696, "USD": 1, "EUR": 0.890729, "GBP": 0.742097}

	euro, ok := cnyPerUnit(usd, "EUR")
	if !ok {
		t.Fatal("EUR 应该能换算")
	}
	// 1 EUR = 1/0.890729 USD = 6.6696/0.890729 CNY
	want := 6.6696 / 0.890729
	if math.Abs(euro-want) > 1e-9 {
		t.Fatalf("EUR 交叉汇率 = %v，期望 %v", euro, want)
	}

	// 直接把 usdRates[EUR] 当人民币汇率是最常见的错法：那会得到 0.89（比 1 USD 还便宜）。
	if euro <= 1 {
		t.Fatalf("EUR 兑 CNY 应该远大于 1，得到 %v（疑似把 USD 基准当成了 CNY 汇率）", euro)
	}

	usdRate, ok := cnyPerUnit(usd, "USD")
	if !ok || math.Abs(usdRate-6.6696) > 1e-9 {
		t.Fatalf("USD 兑 CNY = %v (ok=%v)，期望 6.6696", usdRate, ok)
	}
}

func TestCnyPerUnitRejectsInvalidInput(t *testing.T) {
	cases := map[string]struct {
		rates    map[string]float64
		currency string
	}{
		"缺 CNY":    {map[string]float64{"EUR": 0.9}, "EUR"},
		"币种汇率为零":   {map[string]float64{"CNY": 6.6, "EUR": 0}, "EUR"},
		"币种汇率缺失":   {map[string]float64{"CNY": 6.6}, "EUR"},
		"空币种":      {map[string]float64{"CNY": 6.6}, ""},
		"CNY 自身为零": {map[string]float64{"CNY": 0, "EUR": 0.9}, "EUR"},
		"负数汇率":     {map[string]float64{"CNY": 6.6, "EUR": -1}, "EUR"},
	}
	for name, c := range cases {
		if v, ok := cnyPerUnit(c.rates, c.currency); ok {
			t.Errorf("%s: 应该拒绝，却返回 %v", name, v)
		}
	}
}

func TestBuildCNYRatiosOnlySupportedCurrencies(t *testing.T) {
	got := buildCNYRatios(map[string]float64{
		"CNY": 7, "USD": 1, "EUR": 0.9, "JPY": 150, "KRW": 1350,
	})

	if _, ok := got["JPY"]; ok {
		t.Error("JPY 不在 SupportedCurrencies，不应出现")
	}
	if math.Abs(got["USD"]-7) > 1e-9 {
		t.Errorf("USD = %v，期望 7", got["USD"])
	}
	if math.Abs(got["EUR"]-7.0/0.9) > 1e-9 {
		t.Errorf("EUR = %v，期望 %v", got["EUR"], 7.0/0.9)
	}
}

func TestFormatCNY(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{0, ""},
		{-10, ""},
		{0.4, "¥0"},
		{1.4, "¥1"},
		{307.5, "¥308"},
		{999.4, "¥999"},
		{1234, "¥1,234"},
		{1234567.8, "¥1,234,568"},
	}
	for _, c := range cases {
		if got := FormatCNY(c.in); got != c.want {
			t.Errorf("FormatCNY(%v) = %q，期望 %q", c.in, got, c.want)
		}
	}
}

func TestNextMidnight(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Skipf("加载时区失败（无 tzdata）：%v", err)
	}

	// 23:30 → 次日 00:00
	late := time.Date(2026, 11, 5, 23, 30, 0, 0, loc)
	if got, want := nextMidnight(late, loc), time.Date(2026, 11, 6, 0, 0, 0, 0, loc); !got.Equal(want) {
		t.Errorf("nextMidnight(23:30) = %v，期望 %v", got, want)
	}

	// 中午 → 也是次日 00:00（不是"今天 0 点"）
	noon := time.Date(2026, 11, 6, 12, 0, 0, 0, loc)
	if got, want := nextMidnight(noon, loc), time.Date(2026, 11, 7, 0, 0, 0, 0, loc); !got.Equal(want) {
		t.Errorf("nextMidnight(12:00) = %v，期望 %v", got, want)
	}

	// 跨月
	endOfMonth := time.Date(2026, 11, 30, 22, 0, 0, 0, loc)
	if got, want := nextMidnight(endOfMonth, loc), time.Date(2026, 12, 1, 0, 0, 0, 0, loc); !got.Equal(want) {
		t.Errorf("nextMidnight(11-30 22:00) = %v，期望 %v", got, want)
	}
}

func TestSnapshotString(t *testing.T) {
	if got := (Snapshot{}).String(); got != "inactive" {
		t.Errorf("空快照 = %q，期望 inactive", got)
	}
	s := Snapshot{Active: true, Provider: "p", Rates: map[string]float64{"USD": 7}, UpdatedAt: time.Unix(0, 0)}
	if got := s.String(); got == "" || got[0:7] != "active(" {
		t.Errorf("活跃快照 = %q", got)
	}
}
