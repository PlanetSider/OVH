package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ovh-buy/server/internal/types"
)

// 自动付款是显式开关:旧页面带 autoPay:false 照常下单,autoPay:true 同样接受并保留。
func TestAutoPayRequestsAreAccepted(t *testing.T) {
	state, mon := newWatchTestMonitor(t)
	state.Accounts = []types.OVHAccount{{ID: "eu", Endpoint: "ovh-eu", Zone: "BATCH-TEST"}}
	// 订阅接口先探测通知通道;用本机假 webhook 让请求走到保存逻辑。
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer hook.Close()
	cfg := state.Config.Get()
	cfg.NotifyWebhookURL = hook.URL
	state.Config.Set(cfg)
	router := gin.New()
	router.POST("/queue", AddQueueItem(state))
	router.POST("/monitor", AddSubscription(state, mon))
	router.GET("/monitor", GetSubscriptions(state, mon))
	send := func(method, path, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, r)
		return w
	}
	w := send(http.MethodPost, "/queue", `{"account_id":"eu","planCode":"plan","datacenter":"gra","autoPay":true}`)
	if w.Code != http.StatusOK || len(state.Queue) != 1 || !state.Queue[0].AutoPay {
		t.Fatalf("autoPay:true must be accepted and kept: %d %s", w.Code, w.Body.String())
	}
	w = send(http.MethodPost, "/monitor", `{"planCode":"plan","autoOrder":true,"autoOrderAccountId":"eu","autoPay":true}`)
	if w.Code != http.StatusOK {
		t.Fatalf("subscription autoPay:true must be accepted: %d %s", w.Code, w.Body.String())
	}
	w = send(http.MethodGet, "/monitor", "")
	if w.Code != http.StatusOK {
		t.Fatalf("read subscriptions: %d %s", w.Code, w.Body.String())
	}
	var subs []struct {
		AutoPay bool `json:"autoPay"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &subs); err != nil || len(subs) != 1 || !subs[0].AutoPay {
		t.Fatalf("subscription autoPay not kept: %v %s", err, w.Body.String())
	}
}
