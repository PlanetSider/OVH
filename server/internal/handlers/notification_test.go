package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ovh-buy/server/internal/notify"
	"github.com/ovh-buy/server/internal/types"
)

func TestNotificationUsesActualDeliveryWithoutSecondWebhook(t *testing.T) {
	for _, tc := range []struct {
		name, body        string
		status, delivered int
		ok                bool
	}{
		{"accepted", `{"code":0,"msg":"success"}`, 200, 1, true},
		{"business rejected", `{"code":19002,"msg":"msg_type need"}`, 500, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var requests atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); io.WriteString(w, tc.body) }))
			defer srv.Close()
			st, _ := newWatchTestMonitor(t)
			st.Config.Set(types.Config{NotifyWebhookURL: srv.URL})
			router := gin.New()
			router.POST("/test-notification", TestNotification(st))
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest("POST", "/test-notification", nil))
			var result struct {
				Delivered int              `json:"delivered"`
				Channels  []notify.Channel `json:"channels"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			// 通道顺序固定为 Telegram / Webhook / 飞书 / Server酱³ / MeoW；没配的必须如实
			// 回报"未配置"，不能因为它们不参与本次投递就从列表里消失（那样前端会以为通道不存在）。
			if response.Code != tc.status || result.Delivered != tc.delivered || len(result.Channels) != 5 ||
				result.Channels[1].OK != tc.ok || !result.Channels[1].Configured ||
				result.Channels[2].Name != "飞书" || result.Channels[2].Configured ||
				result.Channels[3].Name != "Server酱³" || result.Channels[3].Configured ||
				result.Channels[4].Name != "MeoW" || result.Channels[4].Configured {
				t.Fatalf("incorrect delivery result: %d %+v", response.Code, result)
			}
			if !tc.ok && !strings.Contains(result.Channels[1].Detail, "19002") {
				t.Fatal("business rejection reason missing")
			}
			if requests.Load() != 1 {
				t.Fatalf("test must send exactly once, got %d", requests.Load())
			}
		})
	}
}
