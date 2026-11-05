package feishu

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// ---- 二维码地址白名单 ----

func TestValidateVerificationURL(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		wantErr bool
		wantHas string
	}{
		{"feishu 合法", "https://accounts.feishu.cn/oauth/device?user_code=ABC", false, "from=onboard"},
		{"lark 合法", "https://accounts.larksuite.com/oauth/device?user_code=ABC", false, "from=onboard"},
		{"open.feishu 合法", "https://open.feishu.cn/page/launcher?user_code=ABC", false, "from=onboard"},
		{"http 拒绝", "http://accounts.feishu.cn/x", true, ""},
		{"非法 host", "https://evil.example.com/x", true, ""},
		{"userinfo 拒绝", "https://user:pass@accounts.feishu.cn/x", true, ""},
		{"非 443 端口", "https://accounts.feishu.cn:8443/x", true, ""},
		{"空地址", "", true, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ValidateVerificationURL(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("期望报错,实际通过: %s", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("不应报错: %v", err)
			}
			if !strings.Contains(got, tc.wantHas) {
				t.Fatalf("结果缺少 %q: %s", tc.wantHas, got)
			}
			u, _ := url.Parse(got)
			if u.Query().Get("from") != "onboard" {
				t.Fatalf("from 参数未设置: %s", got)
			}
			// 原有参数必须保留(用 url 而不是字符串拼接)。
			if u.Query().Get("user_code") != "ABC" {
				t.Fatalf("原有参数丢失: %s", got)
			}
		})
	}
}

// ---- expires_in / expire_in 兼容 ----

func TestExpiresSecondsCompat(t *testing.T) {
	cases := []struct {
		name string
		resp registrationResponse
		want int
	}{
		{"expires_in 生效", registrationResponse{ExpiresIn: 120}, 120},
		{"只有 expire_in", registrationResponse{ExpireIn: 300}, 300},
		{"都缺省回退 600", registrationResponse{}, 600},
		{"负值回退 600", registrationResponse{ExpiresIn: -5}, 600},
		{"超过 1800 回退 600", registrationResponse{ExpiresIn: 99999}, 600},
		{"两个都有取 expires_in", registrationResponse{ExpiresIn: 60, ExpireIn: 300}, 60},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := expiresSeconds(&tc.resp); got != tc.want {
				t.Fatalf("expiresSeconds = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestClampInterval(t *testing.T) {
	if got := clampInterval(0); got != defaultIntervalSecond {
		t.Fatalf("0 应回退默认值, got %d", got)
	}
	if got := clampInterval(120); got != maxIntervalSecond {
		t.Fatalf("超上限应夹到 60, got %d", got)
	}
	if got := clampInterval(10); got != 10 {
		t.Fatalf("合法值不应改, got %d", got)
	}
}

// ---- 注册流程 ----

// fakeRegistration 起一个假的注册端点:按 form 里的 action 分支返回。
func fakeRegistration(t *testing.T, handler func(action string, r *http.Request) (int, any)) (*httptest.Server, *RegistrationService) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		action := r.PostFormValue("action")
		status, body := handler(action, r)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(body)
	}))
	t.Cleanup(srv.Close)
	s := NewRegistrationService()
	s.feishuURL = srv.URL + "/feishu"
	s.larkURL = srv.URL + "/lark"
	return srv, s
}

func forcePollable(t *testing.T, s *RegistrationService) {
	t.Helper()
	s.mu.Lock()
	for _, sess := range s.sessions {
		sess.NextPollAt = time.Now().Add(-time.Second)
	}
	s.mu.Unlock()
}

func TestStartRequiresClientSecret(t *testing.T) {
	_, s := fakeRegistration(t, func(action string, _ *http.Request) (int, any) {
		return http.StatusOK, map[string]any{"supported_auth_methods": []string{"urn:ietf:params:oauth:grant-type:device_code"}}
	})
	if _, err := s.Start(context.Background()); err == nil {
		t.Fatal("不支持 client_secret 时应报错")
	}
}

func TestStartAndPollComplete(t *testing.T) {
	pollCalls := 0
	_, s := fakeRegistration(t, func(action string, _ *http.Request) (int, any) {
		switch action {
		case "init":
			return http.StatusOK, map[string]any{"supported_auth_methods": []string{"client_secret"}}
		case "begin":
			return http.StatusOK, map[string]any{
				"device_code":               "dev-1",
				"verification_uri_complete": "https://accounts.feishu.cn/oauth/device?user_code=ABC",
				"expire_in":                 300, // 故意只给旧字段名
				"interval":                  5,
			}
		case "poll":
			pollCalls++
			if pollCalls == 1 {
				return http.StatusBadRequest, map[string]any{"error": "authorization_pending"}
			}
			return http.StatusOK, map[string]any{
				"client_id":     "cli_abc",
				"client_secret": "secret_xyz",
				"user_info":     map[string]any{"open_id": "ou_123", "tenant_brand": "lark"},
			}
		}
		return http.StatusBadRequest, map[string]any{"error": "unexpected_action"}
	})

	start, err := s.Start(context.Background())
	if err != nil {
		t.Fatalf("start 失败: %v", err)
	}
	if start.SessionID == "" || start.ExpiresIn != 300 {
		t.Fatalf("start 结果异常: %+v", start)
	}
	if strings.Contains(start.VerificationURIComplete, "dev-1") {
		t.Fatal("device_code 不能出现在二维码地址里")
	}

	forcePollable(t, s)
	rep, err := s.Poll(context.Background(), start.SessionID)
	if err != nil {
		t.Fatalf("poll 失败: %v", err)
	}
	if rep.Status != StatusPending {
		t.Fatalf("首次 poll 应 pending, got %+v", rep)
	}
	// pending 后要等 interval;测试里强制放开
	forcePollable(t, s)
	rep, err = s.Poll(context.Background(), start.SessionID)
	if err != nil {
		t.Fatalf("poll 失败: %v", err)
	}
	if rep.Status != StatusComplete {
		t.Fatalf("应 complete, got %+v", rep)
	}
	if rep.AppID != "cli_abc" || rep.AppSecret != "secret_xyz" || rep.OpenID != "ou_123" {
		t.Fatalf("凭据解析错误: %+v", rep)
	}
	if rep.Domain != "lark" {
		t.Fatalf("tenant_brand=lark 应映射为 lark 域名, got %q", rep.Domain)
	}
	// 完成后会话必须删除,防止重复取凭据
	if _, err := s.Poll(context.Background(), start.SessionID); err == nil {
		t.Fatal("完成后会话应失效")
	}
}

func TestPollSlowDownIncreasesInterval(t *testing.T) {
	_, s := fakeRegistration(t, func(action string, _ *http.Request) (int, any) {
		switch action {
		case "init":
			return http.StatusOK, map[string]any{"supported_auth_methods": []string{"client_secret"}}
		case "begin":
			return http.StatusOK, map[string]any{
				"device_code":               "dev-2",
				"verification_uri_complete": "https://accounts.feishu.cn/x?user_code=ABC",
				"interval":                  5,
			}
		}
		return http.StatusBadRequest, map[string]any{"error": "slow_down"}
	})
	start, err := s.Start(context.Background())
	if err != nil {
		t.Fatalf("start 失败: %v", err)
	}
	forcePollable(t, s)
	if _, err := s.Poll(context.Background(), start.SessionID); err != nil {
		t.Fatalf("poll 失败: %v", err)
	}
	s.mu.Lock()
	iv := s.sessions[start.SessionID].Interval
	s.mu.Unlock()
	if iv != 10*time.Second {
		t.Fatalf("slow_down 应把间隔 +5s, got %v", iv)
	}
}

func TestPollDeniedDeletesSession(t *testing.T) {
	_, s := fakeRegistration(t, func(action string, _ *http.Request) (int, any) {
		switch action {
		case "init":
			return http.StatusOK, map[string]any{"supported_auth_methods": []string{"client_secret"}}
		case "begin":
			return http.StatusOK, map[string]any{
				"device_code":               "dev-3",
				"verification_uri_complete": "https://accounts.feishu.cn/x?user_code=ABC",
			}
		}
		return http.StatusForbidden, map[string]any{"error": "access_denied"}
	})
	start, err := s.Start(context.Background())
	if err != nil {
		t.Fatalf("start 失败: %v", err)
	}
	forcePollable(t, s)
	rep, err := s.Poll(context.Background(), start.SessionID)
	if err != nil {
		t.Fatalf("poll 失败: %v", err)
	}
	if rep.Status != StatusDenied {
		t.Fatalf("应 denied, got %+v", rep)
	}
	if _, err := s.Poll(context.Background(), start.SessionID); err == nil {
		t.Fatal("denied 后会话应删除")
	}
}

func TestPollThrottleDoesNotHitUpstream(t *testing.T) {
	calls := 0
	_, s := fakeRegistration(t, func(action string, _ *http.Request) (int, any) {
		switch action {
		case "init":
			return http.StatusOK, map[string]any{"supported_auth_methods": []string{"client_secret"}}
		case "begin":
			return http.StatusOK, map[string]any{
				"device_code":               "dev-4",
				"verification_uri_complete": "https://accounts.feishu.cn/x?user_code=ABC",
				"interval":                  30,
			}
		}
		calls++
		return http.StatusOK, map[string]any{"error": "authorization_pending"}
	})
	start, err := s.Start(context.Background())
	if err != nil {
		t.Fatalf("start 失败: %v", err)
	}
	// 不 force:NextPollAt 还在 30 秒后
	rep, err := s.Poll(context.Background(), start.SessionID)
	if err != nil {
		t.Fatalf("poll 失败: %v", err)
	}
	if rep.Status != StatusPending || rep.RetryAfter < 20 {
		t.Fatalf("应返回 pending + 剩余秒数, got %+v", rep)
	}
	if calls != 0 {
		t.Fatalf("节流期内不应请求上游, got %d 次", calls)
	}
}

func TestPollLarkTenantSwitchesEndpoint(t *testing.T) {
	larkCalls := 0
	_, s := fakeRegistration(t, func(action string, r *http.Request) (int, any) {
		switch action {
		case "init":
			return http.StatusOK, map[string]any{"supported_auth_methods": []string{"client_secret"}}
		case "begin":
			return http.StatusOK, map[string]any{
				"device_code":               "dev-5",
				"verification_uri_complete": "https://accounts.feishu.cn/x?user_code=ABC",
			}
		}
		if strings.HasSuffix(r.URL.Path, "/lark") {
			larkCalls++
			return http.StatusOK, map[string]any{
				"client_id":     "cli_lark",
				"client_secret": "secret_lark",
			}
		}
		// feishu 端点只暴露租户品牌
		return http.StatusOK, map[string]any{
			"error":     "authorization_pending",
			"user_info": map[string]any{"tenant_brand": "lark"},
		}
	})
	start, err := s.Start(context.Background())
	if err != nil {
		t.Fatalf("start 失败: %v", err)
	}
	forcePollable(t, s)
	rep, err := s.Poll(context.Background(), start.SessionID)
	if err != nil {
		t.Fatalf("poll 失败: %v", err)
	}
	if rep.Status != StatusPending {
		t.Fatalf("切端点后应 pending, got %+v", rep)
	}
	s.mu.Lock()
	ep := s.sessions[start.SessionID].Endpoint
	s.mu.Unlock()
	if ep != s.larkURL {
		t.Fatalf("会话端点应切到 lark, got %s", ep)
	}

	forcePollable(t, s)
	rep, err = s.Poll(context.Background(), start.SessionID)
	if err != nil {
		t.Fatalf("poll 失败: %v", err)
	}
	if rep.Status != StatusComplete || rep.AppID != "cli_lark" {
		t.Fatalf("lark 端点应完成注册, got %+v", rep)
	}
	if larkCalls != 1 {
		t.Fatalf("lark 端点应被调用 1 次, got %d", larkCalls)
	}
}

func TestCallRejectsUnknownEndpoint(t *testing.T) {
	s := NewRegistrationService()
	_, err := s.call(context.Background(), "https://evil.example.com/x", url.Values{})
	if err == nil {
		t.Fatal("未知端点应被拒绝")
	}
}

// ---- 上游错误处理 ----

func TestPollNetworkErrorKeepsSessionPending(t *testing.T) {
	_, s := fakeRegistration(t, func(action string, _ *http.Request) (int, any) {
		switch action {
		case "init":
			return http.StatusOK, map[string]any{"supported_auth_methods": []string{"client_secret"}}
		case "begin":
			return http.StatusOK, map[string]any{
				"device_code":               "dev-6",
				"verification_uri_complete": "https://accounts.feishu.cn/x?user_code=ABC",
			}
		}
		return http.StatusInternalServerError, "not json"
	})
	start, err := s.Start(context.Background())
	if err != nil {
		t.Fatalf("start 失败: %v", err)
	}
	forcePollable(t, s)
	rep, err := s.Poll(context.Background(), start.SessionID)
	if err != nil {
		t.Fatalf("poll 不应返回错误: %v", err)
	}
	if rep.Status != StatusPending {
		t.Fatalf("网关错误应保持 pending 让用户重试, got %+v", rep)
	}
	s.mu.Lock()
	_, still := s.sessions[start.SessionID]
	s.mu.Unlock()
	if !still {
		t.Fatal("网络类错误不应删除会话")
	}
}

func TestStartNon2xxWithErrorIsNotGatewayError(t *testing.T) {
	_, s := fakeRegistration(t, func(action string, _ *http.Request) (int, any) {
		// init 返回 400 + 结构化 error:应原样报错而不是被当成 JSON 失败
		return http.StatusBadRequest, map[string]any{"error": "invalid_request", "error_description": "bad archetype"}
	})
	_, err := s.Start(context.Background())
	if err == nil {
		t.Fatal("应报错")
	}
	if !strings.Contains(err.Error(), "bad archetype") {
		t.Fatalf("错误信息应带上游描述, got %v", err)
	}
}
