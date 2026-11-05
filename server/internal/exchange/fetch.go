package exchange

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// provider 一个汇率源的描述。顺序即优先级：前面的失败再试后面的。
type provider struct {
	name string
	url  string
}

// fetchResult 拉取成功后的中间结果。
type fetchResult struct {
	Provider  string
	Rates     map[string]float64 // USD 基准原始汇率
	UpdatedAt time.Time          // provider 自报的数据时间（拿不到时用拉取时间）
}

// providers 返回本次运行可用的汇率源。
//
// 默认两个免费源：api.exchangerate.fun 是 playbook 里用的那个；
// open.er-api.com 是它的公开镜像，格式几乎一致，一个挂了还能顶上。
// 配了 EXCHANGE_API_KEY 时再把 exchangerate-api v6 排到最前面——付费源通常更新更稳。
func providers() []provider {
	list := []provider{
		{name: "exchangerate.fun", url: "https://api.exchangerate.fun/latest?base=USD"},
		{name: "open.er-api.com", url: "https://open.er-api.com/v6/latest/USD"},
	}
	if key := strings.TrimSpace(os.Getenv("EXCHANGE_API_KEY")); key != "" {
		list = append([]provider{{
			name: "exchangerate-api",
			url:  "https://v6.exchangerate-api.com/v6/" + url.PathEscape(key) + "/latest/USD",
		}}, list...)
	}
	// EXCHANGE_BASE_URL 用于自建/内网镜像：填一个与 open.er-api.com 同格式的地址即可。
	if custom := strings.TrimSpace(os.Getenv("EXCHANGE_BASE_URL")); custom != "" {
		list = append([]provider{{name: "custom", url: custom}}, list...)
	}
	return list
}

// httpClient 汇率服务的 HTTP 客户端。
//
// 故意不复用 ovh 的 netfp 客户端：那套传输带浏览器指纹和用户代理，
// 是为"让 OVH 认为是真人"设计的，拿去请求汇率源既没必要也会让代理故障
// 牵连到展示层。这里只认 EXCHANGE_HTTP_PROXY 一个显式代理开关，
// 不读系统 HTTP_PROXY（容器里常有全局变量，会把汇率请求也拽进代理）。
var httpClient = &http.Client{Timeout: 20 * time.Second}

func init() {
	if raw := strings.TrimSpace(os.Getenv("EXCHANGE_HTTP_PROXY")); raw != "" {
		u, err := url.Parse(raw)
		if err == nil && u.Host != "" {
			httpClient.Transport = &http.Transport{
				Proxy: http.ProxyURL(u),
				DialContext: (&net.Dialer{
					Timeout:   10 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext,
				TLSHandshakeTimeout: 10 * time.Second,
			}
		}
	}
}

// providerResponse 兼容三个源的字段差异。
// exchangerate.fun 给 timestamp，er-api 给 time_last_update_unix；
// 两者都可能是 0（镜像没带时间），此时退回拉取时刻。
type providerResponse struct {
	Rates     map[string]float64 `json:"rates"`
	Base      string             `json:"base"`
	BaseCode  string             `json:"base_code"`
	Timestamp int64              `json:"timestamp"`
	LastUnix  int64              `json:"time_last_update_unix"`
	Result    string             `json:"result"`
	ErrorType string             `json:"error-type"`
}

// fetchRates 依次尝试各源，返回第一个可用的结果。
// 任何一个源返回了 rates 但缺 CNY，都算失败——没有 CNY 的汇率快照
// 在这个功能里没有意义，留着只会让调用方以为"汇率可用"。
func fetchRates(_ context.Context) (*fetchResult, error) {
	var errs []string
	for _, p := range providers() {
		res, err := fetchOne(p)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", p.name, err))
			continue
		}
		return res, nil
	}
	return nil, errors.New("所有汇率源都失败: " + strings.Join(errs, "; "))
}

func fetchOne(p provider) (*fetchResult, error) {
	req, err := http.NewRequest(http.MethodGet, p.url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "ovh-buy-exchange/1.0")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var pr providerResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&pr); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	if pr.Result != "" && pr.Result != "success" {
		return nil, fmt.Errorf("provider 返回 %q (%s)", pr.Result, pr.ErrorType)
	}
	if len(pr.Rates) == 0 {
		return nil, errors.New("响应里没有 rates")
	}
	usd := strings.ToUpper(strings.TrimSpace(firstNonEmpty(pr.Base, pr.BaseCode, "USD")))
	if usd != "USD" {
		return nil, fmt.Errorf("汇率基准是 %s，期望 USD", usd)
	}
	if pr.Rates["CNY"] <= 0 {
		return nil, errors.New("缺少 CNY 汇率")
	}

	updated := time.Now()
	if ts := firstNonZero(pr.Timestamp, pr.LastUnix); ts > 0 {
		// 有些源给毫秒（13 位），按位数判断，避免把 2026 年当成 1970 年。
		if ts > 1e12 {
			updated = time.UnixMilli(ts)
		} else {
			updated = time.Unix(ts, 0)
		}
	}
	return &fetchResult{Provider: p.name, Rates: pr.Rates, UpdatedAt: updated.UTC()}, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func firstNonZero(vals ...int64) int64 {
	for _, v := range vals {
		if v > 0 {
			return v
		}
	}
	return 0
}
