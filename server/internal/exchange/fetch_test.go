package exchange

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// serveJSON 起一个只回固定 body 的测试服务器。
func serveJSON(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestFetchOneExchangerateFunFormat(t *testing.T) {
	srv := serveJSON(t, http.StatusOK,
		`{"timestamp":1762345600000,"base":"USD","rates":{"CNY":6.6696,"EUR":0.890729}}`)

	res, err := fetchOne(provider{name: "exchangerate.fun", url: srv.URL})
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if res.Rates["CNY"] != 6.6696 {
		t.Errorf("CNY = %v，期望 6.6696", res.Rates["CNY"])
	}
	if res.Provider != "exchangerate.fun" {
		t.Errorf("Provider = %q", res.Provider)
	}
	// 13 位时间戳按毫秒解析，不能被当成 1970 年
	if got := res.UpdatedAt.UnixMilli(); got != 1762345600000 {
		t.Errorf("UpdatedAt = %d，期望 1762345600000", got)
	}
}

func TestFetchOneErApiFormat(t *testing.T) {
	srv := serveJSON(t, http.StatusOK,
		`{"result":"success","base_code":"USD","time_last_update_unix":1762345600,"rates":{"CNY":7.1}}`)

	res, err := fetchOne(provider{name: "open.er-api.com", url: srv.URL})
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if res.Rates["CNY"] != 7.1 {
		t.Errorf("CNY = %v，期望 7.1", res.Rates["CNY"])
	}
	if got := res.UpdatedAt.Unix(); got != 1762345600 {
		t.Errorf("UpdatedAt = %d，期望 1762345600（10 位按秒）", got)
	}
}

func TestFetchOneRejectsBadPayloads(t *testing.T) {
	cases := map[string]string{
		"没有 rates":       `{"base":"USD","rates":{}}`,
		"基准不是 USD":      `{"base":"EUR","rates":{"CNY":7}}`,
		"缺 CNY":          `{"base":"USD","rates":{"EUR":0.9}}`,
		"CNY 为零":         `{"base":"USD","rates":{"CNY":0}}`,
		"provider 报错":    `{"result":"error","error-type":"invalid-key"}`,
		"不是 JSON":        `not json`,
	}
	for name, body := range cases {
		srv := serveJSON(t, http.StatusOK, body)
		if _, err := fetchOne(provider{name: "t", url: srv.URL}); err == nil {
			t.Errorf("%s: 应该失败", name)
		}
	}
}

func TestFetchOneHTTPErrorIncludesStatus(t *testing.T) {
	srv := serveJSON(t, http.StatusTooManyRequests, `rate limited`)
	_, err := fetchOne(provider{name: "t", url: srv.URL})
	if err == nil {
		t.Fatal("429 应该返回错误")
	}
	if !containsStr(err.Error(), "429") {
		t.Errorf("错误里应包含 429，得到 %v", err)
	}
}

func containsStr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
