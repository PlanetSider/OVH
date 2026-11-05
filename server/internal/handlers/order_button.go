package handlers

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/ovh-buy/server/internal/app"
	"github.com/ovh-buy/server/internal/monitor"
	"github.com/ovh-buy/server/internal/ovh"
	"github.com/ovh-buy/server/internal/types"
)

// EnqueueFromButton 处理一次「一键下单」按钮点击，返回给用户的回复文本。
//
// 通道无关：按钮 value 里只有 UUID，完整的 planCode/机房/配置/账户都从
// telegram_order_buttons 表里取。Telegram 与飞书点的是同一行，
// ClaimTelegramButton 的 UPDATE ... WHERE used_at = 0 是原子的，
// 所以同一条通知在两个通道各点一次，也只有一次能真正入队
// —— 这正是跨通道去重需要的那把锁。
func EnqueueFromButton(state *app.State, mon *monitor.Monitor, buttonID string) string {
	buttonID = strings.TrimSpace(buttonID)
	if buttonID == "" {
		return "按钮参数缺失。"
	}
	btn, ok, err := state.DB.ClaimTelegramButton(buttonID)
	if err != nil {
		state.Logger.Error("按钮认领失败: "+err.Error(), "notify")
		return "⚠️ 读取按钮失败，请重试。"
	}
	if !ok {
		// 区分「不存在」和「已消费」：前者让用户等新通知，后者要告诉他这是防重设计
		if _, exists, _ := state.DB.GetTelegramButton(buttonID); exists {
			return "这个按钮已经用过了（一次性按钮，防止重复下单）。"
		}
		return "按钮已失效，请等下一条通知。"
	}

	var options []string
	if strings.TrimSpace(btn.Options) != "" {
		_ = json.Unmarshal([]byte(btn.Options), &options)
	}
	// 老按钮可能没存 options，退回监控进程里的缓存（与 Telegram 同一条兜底）
	if len(options) == 0 && mon != nil {
		if cached := mon.OptionsCacheLookup(btn.PlanCode + "|" + btn.Datacenter); len(cached) > 0 {
			options = cached
		}
	}
	var configInfo map[string]interface{}
	if strings.TrimSpace(btn.ConfigInfo) != "" {
		_ = json.Unmarshal([]byte(btn.ConfigInfo), &configInfo)
	}
	duration, _ := configInfo["duration"].(string)

	acc, hasAcc := state.FindAccount(btn.AccountID)
	if !hasAcc && btn.AccountID != "" {
		// 按钮里的账户已被删除：不能直接失败（用户还有别的账户可下），
		// 但必须留痕 —— 退回默认账户很可能就是跨区下错单的那一刻。
		state.Logger.Warn("按钮记录的账户已不存在，退回默认账户: "+btn.AccountID, "notify")
		acc, hasAcc = state.FindAccount("")
	}
	if !hasAcc {
		_ = state.DB.UnclaimTelegramButton(buttonID)
		state.Logger.Warn("一键下单被拒绝：系统里没有任何 OVH 账户", "notify")
		return "❌ 未配置任何 OVH 账户，无法下单。请先在控制台添加账户。"
	}

	accSub := strings.ToUpper(strings.TrimSpace(acc.Zone))
	if accSub == "" {
		accSub = ovh.DefaultSubsidiaryForEndpoint(acc.Endpoint)
	}
	accLabel := fmt.Sprintf("%s（子公司 %s / %s 区）", acc.Name, accSub, ovh.SubsidiaryRegion(accSub))
	if btn.AccountID == "" {
		accLabel += "［默认账户］"
	}

	item := types.QueueItem{
		ID:            uuid.NewString(),
		AccountID:     acc.ID,
		PlanCode:      btn.PlanCode,
		Duration:      duration,
		Datacenter:    btn.Datacenter,
		Options:       options,
		Status:        "running",
		CreatedAt:     types.NowISO(),
		UpdatedAt:     types.NowISO(),
		RetryInterval: state.Config.RetryInterval(),
		LastCheckTime: 0,
		FromTelegram:  true,
	}
	// 入队 + 落库统一走 EnqueueItems：失败会自动撤回内存里的那条，这里把按钮也还回去
	if err := state.EnqueueItems([]types.QueueItem{item}, false); err != nil {
		_ = state.DB.UnclaimTelegramButton(buttonID)
		state.Logger.Error("一键下单落库失败,已撤回: "+err.Error(), "notify")
		return "❌ 任务没能写进数据库，已撤回（避免出现重启就消失的假任务）：\n" + err.Error()
	}

	optsStr := strings.Join(options, ", ")
	if optsStr == "" {
		optsStr = "无（默认配置）"
	}
	state.Logger.Info(fmt.Sprintf("通过按钮添加到队列: %s@%s, 配置选项: %s, 账户: %s",
		btn.PlanCode, btn.Datacenter, optsStr, accLabel), "notify")
	return fmt.Sprintf("✅ 已加入抢购队列\n\n型号: %s\n机房: %s\n配置: %s\n账户: %s\n\n"+
		"系统会一直重试到抢到为止。\n查看 /queue · 取消 /cancel %s",
		btn.PlanCode, strings.ToUpper(btn.Datacenter), optsStr, accLabel, shortID(item.ID))
}
