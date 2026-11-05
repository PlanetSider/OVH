package handlers

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// 定时播报时间在保存时就挡下来：存进一个非法值时，每日循环会静默回退到
// 默认时间，而设置页显示的是用户填的字符串——两边对不上，且没有任何报错。
func TestSaveSettingsValidatesBroadcastTime(t *testing.T) {
	gin.SetMode(gin.TestMode)
	st, _ := newWatchTestMonitor(t)

	post := func(body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/settings", bytes.NewBufferString(body))
		c.Request.Header.Set("Content-Type", "application/json")
		SaveSettings(st)(c)
		return w
	}

	for _, bad := range []string{"25:00", "09:60", "0900", "abc", "12:"} {
		w := post(`{"taskBroadcastTime":"` + bad + `"}`)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("time %q: expected 400, got %d: %s", bad, w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), "定时播报时间不合法") {
			t.Fatalf("time %q: unexpected body %s", bad, w.Body.String())
		}
	}

	// 合法但未补零的写法保存后要变成规整的 HH:MM
	w := post(`{"taskBroadcastTime":"8:05"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if got := st.Config.Get().TaskBroadcastTime; got != "08:05" {
		t.Fatalf("stored time = %q, want 08:05", got)
	}
}
