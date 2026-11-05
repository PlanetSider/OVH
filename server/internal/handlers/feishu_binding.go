package handlers

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ovh-buy/server/internal/app"
)

// 飞书绑定码：手动填写 App ID/Secret 的应用没有"扫码即绑定"这一步，
// 需要一个让用户证明"这个私聊是我发的"的短流程。
//
// 只存在一处：kv 里一条记录，覆盖式写入。这是有意的 ——
// 单实例单操作者场景，旧码本来就该被新码顶掉（多发几次也不会出现两个有效码）。
const feishuBindingKey = "feishu:binding"

// 10 分钟：够复制粘贴，又短到泄露窗口可接受。
const feishuBindingTTL = 10 * time.Minute

type feishuBindingRecord struct {
	Code      string `json:"code"`
	ExpiresAt string `json:"expiresAt"` // RFC3339
}

func newFeishuBindingCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

// CreateFeishuBindingCode 生成一次性绑定码（POST /api/feishu/binding-code）。
//
// 不走白名单：接口在鉴权组内，能调它的前提是已经持有 API Key。
func CreateFeishuBindingCode(state *app.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg := state.Config.Get()
		if !cfg.FeishuConfigured() {
			c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "先填写 App ID / App Secret 再生成绑定码。"})
			return
		}
		code, err := newFeishuBindingCode()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "生成绑定码失败: " + err.Error()})
			return
		}
		rec := feishuBindingRecord{
			Code:      code,
			ExpiresAt: time.Now().Add(feishuBindingTTL).Format(time.RFC3339),
		}
		if err := state.DB.SetKV(feishuBindingKey, rec); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "保存绑定码失败: " + err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "success", "code": rec.Code, "expiresAt": rec.ExpiresAt})
	}
}

// UnbindFeishu 解除接收人绑定（POST /api/feishu/unbind）。
//
// 解绑必须是独立动作，而不是「保存设置时把表单里的 open_id 清空」：
// 设置页每次保存都提交整个表单，若允许空值清绑定，任何一次无关的保存
// （改个重试间隔）都可能顺手把绑定删掉，用户只会看到机器人突然不认人了。
func UnbindFeishu(state *app.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg := state.Config.Get()
		if cfg.FeishuBoundOpenID != "" {
			cfg.FeishuBoundOpenID = ""
			if err := state.Config.Set(cfg); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "解除绑定失败: " + err.Error()})
				return
			}
			state.Logger.Info("飞书接收人已解绑", "feishu")
		}
		c.JSON(http.StatusOK, gin.H{"status": "success"})
	}
}

// claimFeishuBindingCode 校验并消费一个绑定码。
// 返回空串表示成功；否则返回给用户的失败原因。
func claimFeishuBindingCode(state *app.State, input string) string {
	var rec feishuBindingRecord
	ok, err := state.DB.GetKV(feishuBindingKey, &rec)
	if err != nil {
		state.Logger.Error("读取飞书绑定码失败: "+err.Error(), "feishu")
		return "⚠️ 读取绑定码失败，请稍后重试。"
	}
	if !ok || rec.Code == "" {
		return "还没有生成绑定码。请到网页控制台 → 设置 → 飞书机器人，点「生成绑定码」。"
	}
	if exp, err := time.Parse(time.RFC3339, rec.ExpiresAt); err == nil && time.Now().After(exp) {
		_ = state.DB.DeleteKV(feishuBindingKey)
		return "绑定码已过期，请到控制台重新生成。"
	}
	if input != rec.Code {
		return "绑定码不对，请检查控制台上的 6 位数字。"
	}
	// 先清码再写绑定：清码失败时不继续，避免同一个码还能再用一次。
	if err := state.DB.DeleteKV(feishuBindingKey); err != nil {
		state.Logger.Error("清除飞书绑定码失败: "+err.Error(), "feishu")
		return "⚠️ 绑定码作废失败，请重试。"
	}
	return ""
}
