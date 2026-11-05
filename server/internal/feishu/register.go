package feishu

// 扫码创建机器人(飞书 PersonalAgent 设备码注册协议)。
//
// 这不是普通 OAuth:这里的"客户端"是飞书应用本身,扫码者在飞书里确认
// "创建机器人"后,返回的是应用凭据(client_id / client_secret) + 扫码者 open_id。
//
// 三步:
//   init  探测当前环境支持哪些认证方式(必须含 client_secret)
//   begin 创建扫码会话,拿 device_code + 二维码地址
//   poll  用 device_code 换凭据
//
// 三条纪律:
//  1. device_code 只活在服务端内存,绝不下发前端;
//  2. 注册端点只允许两个常量,外部只能传 sessionId;
//  3. poll 的 4xx 里如果带结构化 error,那是状态机的一部分,不是网关故障。

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	feishuRegistrationURL = "https://accounts.feishu.cn/oauth/v1/app/registration"
	larkRegistrationURL   = "https://accounts.larksuite.com/oauth/v1/app/registration"

	// maxResponseBytes 上限:注册接口只会回一个几百字节的 JSON,
	// 1MB 足够,防止对端异常时把响应整个读进内存。
	maxResponseBytes = 1 << 20

	// 飞书不同版本返回 expires_in / expire_in 两种字段名,两个都要读。
	// 默认 600 秒,和 playbook 一致;超过 1800 视为异常值。
	defaultExpiresSeconds = 600
	maxExpiresSeconds     = 1800
	defaultIntervalSecond = 5
	maxIntervalSecond     = 60
)

// 扫码会话的状态。前端只需按 status 分支。
const (
	StatusPending  = "pending"
	StatusComplete = "complete"
	StatusDenied   = "denied"
	StatusExpired  = "expired"
	StatusError    = "error"
)

// 二维码地址允许的 host。前端也要做同一份校验,但后端才是最终执行者。
var allowedQRHosts = map[string]bool{
	"accounts.feishu.cn":     true,
	"accounts.larksuite.com": true,
	"open.feishu.cn":         true,
	"open.larksuite.com":     true,
}

// Session 一次扫码注册的会话。只存内存:进程重启后二维码本来就该重扫。
type Session struct {
	ID         string
	DeviceCode string
	Endpoint   string
	Interval   time.Duration
	ExpiresAt  time.Time
	NextPollAt time.Time
}

// StartReply 交给前端的非敏感信息。device_code 不在其中。
type StartReply struct {
	SessionID string `json:"sessionId"`
	// VerificationURIComplete 自带 user_code 的完整二维码地址。
	VerificationURIComplete string `json:"verificationUriComplete"`
	ExpiresIn               int    `json:"expiresIn"`
	Interval                int    `json:"interval"`
}

// PollReply poll 的结果。AppSecret 只给 handler 落库用,handler 回前端前必须删掉。
type PollReply struct {
	Status     string `json:"status"`
	RetryAfter int    `json:"retryAfter,omitempty"`
	AppID      string `json:"appId,omitempty"`
	AppSecret  string `json:"appSecret,omitempty"`
	OpenID     string `json:"openId,omitempty"`
	Domain     string `json:"domain,omitempty"`
	Message    string `json:"message,omitempty"`
}

// RegistrationService 管理扫码会话。所有方法可并发调用。
type RegistrationService struct {
	mu       sync.Mutex
	sessions map[string]*Session
	now      func() time.Time
	client   *http.Client
	// 生产固定为上面两个常量;留字段是为了测试能指向 httptest。
	feishuURL string
	larkURL   string
}

// NewRegistrationService 创建服务。http.Client 单独一个:注册接口访问的是
// 飞书账号系统,和 OVH 的代理/指纹链路无关,不能复用 netfp 的 transport。
func NewRegistrationService() *RegistrationService {
	return &RegistrationService{
		sessions:  make(map[string]*Session),
		now:       time.Now,
		client:    &http.Client{Timeout: 10 * time.Second},
		feishuURL: feishuRegistrationURL,
		larkURL:   larkRegistrationURL,
	}
}

type registrationResponse struct {
	DeviceCode              string   `json:"device_code"`
	VerificationURI         string   `json:"verification_uri"`
	VerificationURIComplete string   `json:"verification_uri_complete"`
	ExpiresIn               int      `json:"expires_in"`
	ExpireIn                int      `json:"expire_in"`
	Interval                int      `json:"interval"`
	ClientID                string   `json:"client_id"`
	ClientSecret            string   `json:"client_secret"`
	SupportedAuthMethods    []string `json:"supported_auth_methods"`
	UserInfo                *struct {
		OpenID      string `json:"open_id"`
		TenantBrand string `json:"tenant_brand"`
	} `json:"user_info"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

// expiresSeconds 兼容 expires_in / expire_in,并夹到合理区间。
func expiresSeconds(r *registrationResponse) int {
	v := r.ExpiresIn
	if v <= 0 {
		v = r.ExpireIn
	}
	if v <= 0 || v > maxExpiresSeconds {
		return defaultExpiresSeconds
	}
	return v
}

func clampInterval(v int) int {
	if v <= 0 {
		return defaultIntervalSecond
	}
	if v > maxIntervalSecond {
		return maxIntervalSecond
	}
	return v
}

// Start 走 init + begin,创建一次扫码会话。
func (s *RegistrationService) Start(ctx context.Context) (*StartReply, error) {
	// 每次 start 顺手清掉过期会话,避免长期运行攒垃圾。
	s.mu.Lock()
	now := s.now()
	for id, sess := range s.sessions {
		if now.After(sess.ExpiresAt) {
			delete(s.sessions, id)
		}
	}
	s.mu.Unlock()

	initResp, err := s.call(ctx, s.feishuURL, url.Values{"action": {"init"}})
	if err != nil {
		return nil, fmt.Errorf("初始化注册失败: %w", err)
	}
	if initResp.Error != "" {
		return nil, fmt.Errorf("初始化注册失败: %s", firstNonEmpty(initResp.ErrorDescription, initResp.Error))
	}
	if !containsStr(initResp.SupportedAuthMethods, "client_secret") {
		return nil, errors.New("当前租户不支持 client_secret 注册方式,请改用手动填写 App ID/Secret")
	}

	beginResp, err := s.call(ctx, s.feishuURL, url.Values{
		"action":            {"begin"},
		"archetype":         {"PersonalAgent"},
		"auth_method":       {"client_secret"},
		"request_user_info": {"open_id"},
	})
	if err != nil {
		return nil, fmt.Errorf("创建扫码会话失败: %w", err)
	}
	if beginResp.Error != "" {
		return nil, fmt.Errorf("创建扫码会话失败: %s", firstNonEmpty(beginResp.ErrorDescription, beginResp.Error))
	}
	if beginResp.DeviceCode == "" {
		return nil, errors.New("飞书未返回 device_code")
	}

	qrURL, err := ValidateVerificationURL(firstNonEmpty(beginResp.VerificationURIComplete, beginResp.VerificationURI))
	if err != nil {
		return nil, err
	}

	id, err := randomID()
	if err != nil {
		return nil, err
	}
	expires := expiresSeconds(beginResp)
	interval := clampInterval(beginResp.Interval)

	s.mu.Lock()
	s.sessions[id] = &Session{
		ID:         id,
		DeviceCode: beginResp.DeviceCode,
		Endpoint:   s.feishuURL,
		Interval:   time.Duration(interval) * time.Second,
		ExpiresAt:  now.Add(time.Duration(expires) * time.Second),
		NextPollAt: now.Add(time.Duration(interval) * time.Second),
	}
	s.mu.Unlock()

	return &StartReply{
		SessionID:               id,
		VerificationURIComplete: qrURL,
		ExpiresIn:               expires,
		Interval:                interval,
	}, nil
}

// Poll 用会话里的 device_code 问一次上游。节流由 NextPollAt 控制:
// 前端比建议间隔更勤,也不会把请求打到飞书。
func (s *RegistrationService) Poll(ctx context.Context, sessionID string) (*PollReply, error) {
	s.mu.Lock()
	sess, ok := s.sessions[sessionID]
	if !ok {
		s.mu.Unlock()
		return nil, errors.New("扫码会话不存在或已用完,请重新生成二维码")
	}
	now := s.now()
	if now.After(sess.ExpiresAt) {
		delete(s.sessions, sessionID)
		s.mu.Unlock()
		return &PollReply{Status: StatusExpired, Message: "二维码已过期"}, nil
	}
	if now.Before(sess.NextPollAt) {
		wait := int(sess.NextPollAt.Sub(now).Seconds())
		if wait < 1 {
			wait = 1
		}
		s.mu.Unlock()
		return &PollReply{Status: StatusPending, RetryAfter: wait}, nil
	}
	endpoint := sess.Endpoint
	deviceCode := sess.DeviceCode
	s.mu.Unlock()

	resp, err := s.call(ctx, endpoint, url.Values{
		"action":      {"poll"},
		"device_code": {deviceCode},
	})
	if err != nil {
		// 网络错误不算会话失效:保留会话,让前端隔几秒重试。
		s.setNextPoll(sessionID, 5*time.Second)
		return &PollReply{Status: StatusPending, RetryAfter: 5}, nil
	}

	// 已拿到凭据:成功优先级最高,不再看 error 字段。
	if resp.ClientID != "" && resp.ClientSecret != "" {
		s.mu.Lock()
		delete(s.sessions, sessionID)
		s.mu.Unlock()
		domain := "feishu"
		openID := ""
		if resp.UserInfo != nil {
			openID = strings.TrimSpace(resp.UserInfo.OpenID)
			if strings.EqualFold(resp.UserInfo.TenantBrand, "lark") {
				domain = "lark"
			}
		}
		return &PollReply{
			Status:    StatusComplete,
			AppID:     resp.ClientID,
			AppSecret: resp.ClientSecret,
			OpenID:    openID,
			Domain:    domain,
		}, nil
	}

	// 海外租户可能先在 feishu 端点建会话,轮询时才暴露 tenant_brand=lark。
	// 此时把会话端点切过去,下一轮用 larksuite.com 继续 poll。
	//
	// 这一步必须排在状态机之前:poll 返回的 error 仍可能是
	// authorization_pending,先进状态机就再也不会切端点了。
	if resp.UserInfo != nil && strings.EqualFold(resp.UserInfo.TenantBrand, "lark") && endpoint == s.feishuURL {
		s.mu.Lock()
		if cur, ok := s.sessions[sessionID]; ok {
			cur.Endpoint = s.larkURL
			cur.NextPollAt = s.now()
		}
		s.mu.Unlock()
		return &PollReply{Status: StatusPending, RetryAfter: 1}, nil
	}

	switch strings.ToLower(strings.TrimSpace(resp.Error)) {
	case "", "authorization_pending":
		s.bumpPoll(sessionID, 0)
		return &PollReply{Status: StatusPending, RetryAfter: s.retryAfter(sessionID)}, nil
	case "slow_down":
		// playbook:间隔 +5 秒,上限 60。加到 60 后继续按 60 等。
		s.bumpPoll(sessionID, 5*time.Second)
		return &PollReply{Status: StatusPending, RetryAfter: s.retryAfter(sessionID)}, nil
	case "access_denied":
		s.dropSession(sessionID)
		return &PollReply{Status: StatusDenied, Message: "你已取消授权"}, nil
	case "expired_token":
		s.dropSession(sessionID)
		return &PollReply{Status: StatusExpired, Message: "二维码已过期"}, nil
	}

	// 剩余情况都归为「注册服务出错」:保留会话让用户重试,
	// 不把一次网关抖动变成"二维码作废"。
	s.setNextPoll(sessionID, 5*time.Second)
	return &PollReply{
		Status:  StatusError,
		Message: firstNonEmpty(resp.ErrorDescription, resp.Error, "注册服务返回未知错误"),
	}, nil
}

func (s *RegistrationService) dropSession(id string) {
	s.mu.Lock()
	delete(s.sessions, id)
	s.mu.Unlock()
}

func (s *RegistrationService) setNextPoll(id string, d time.Duration) {
	s.mu.Lock()
	if sess, ok := s.sessions[id]; ok {
		sess.NextPollAt = s.now().Add(d)
	}
	s.mu.Unlock()
}

// bumpPoll 在当前间隔上加 delta(slow_down 用),按新间隔安排下一次。
func (s *RegistrationService) bumpPoll(id string, delta time.Duration) {
	s.mu.Lock()
	if sess, ok := s.sessions[id]; ok {
		if delta > 0 {
			sess.Interval += delta
			if sess.Interval > maxIntervalSecond*time.Second {
				sess.Interval = maxIntervalSecond * time.Second
			}
		}
		sess.NextPollAt = s.now().Add(sess.Interval)
	}
	s.mu.Unlock()
}

func (s *RegistrationService) retryAfter(id string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	if !ok {
		return 5
	}
	wait := int(sess.Interval.Seconds())
	if wait < 1 {
		wait = 5
	}
	return wait
}

// call 发一次 form 请求。非 2xx 但 body 里有结构化 error 时照样返回响应,
// 由调用方按状态机处理 —— 把 authorization_pending 当网关错误会让扫码永远卡住。
func (s *RegistrationService) call(ctx context.Context, endpoint string, values url.Values) (*registrationResponse, error) {
	if endpoint != s.feishuURL && endpoint != s.larkURL {
		return nil, errors.New("不受信任的飞书注册端点")
	}
	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, endpoint, strings.NewReader(values.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxResponseBytes {
		return nil, errors.New("注册服务响应过大")
	}

	var out registrationResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("注册服务返回了非 JSON 响应(HTTP %d)", resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if out.Error == "" {
			return nil, fmt.Errorf("注册服务 HTTP %d", resp.StatusCode)
		}
		return &out, nil
	}
	return &out, nil
}

// ValidateVerificationURL 校验二维码地址。
//
// 这里挡的是钓鱼:如果飞书(或中间人)返回一个 https://evil.com?... 的地址,
// 用户扫了就在别人的页面上确认授权。所以只认 https + 精确 host + 无 userinfo。
func ValidateVerificationURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("飞书未返回二维码地址")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", errors.New("二维码地址不合法")
	}
	if u.Scheme != "https" {
		return "", errors.New("二维码地址必须是 https")
	}
	if u.User != nil {
		return "", errors.New("二维码地址不能包含用户信息")
	}
	if u.Port() != "" && u.Port() != "443" {
		return "", errors.New("二维码地址端口不合法")
	}
	if !allowedQRHosts[strings.ToLower(u.Hostname())] {
		return "", errors.New("二维码地址域名不在白名单内")
	}
	// 用 url 解析 + Query().Set,不要字符串拼接(地址里可能已有查询参数)。
	q := u.Query()
	q.Set("from", "onboard")
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func randomID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func containsStr(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}
