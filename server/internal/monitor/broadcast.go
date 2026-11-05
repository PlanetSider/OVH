package monitor

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ovh-buy/server/internal/catalog"
	"github.com/ovh-buy/server/internal/exchange"
	"github.com/ovh-buy/server/internal/notify"
	"github.com/ovh-buy/server/internal/types"
	"github.com/ovh-buy/server/internal/vps"
)

// 定时播报把「当前在盯什么」一次性发到所有已配置的通知通道。
//
// 三条设计约束：
//  1. 只看不写：不创建任务、不改订阅、不碰队列状态。播报失败最多是少一条消息。
//  2. 价格走公开目录（catalog.PriceForOptions），不走 7 步询价：播报是清单，
//     不是下单前核价；询价每个任务要 2-5 秒，几十个任务会把每日循环拖成几分钟。
//  3. 分片发送：Telegram/飞书都有单条长度上限，多账户多任务必然超。

const (
	// broadcastMessageMaxBytes 单条消息正文上限（字节）。
	// 2500 远小于 Telegram 的 4096，留出标题、分片后缀和 emoji 余量。
	broadcastMessageMaxBytes = 2500
	// broadcastPartTitleReserve 分片标题 "（1/3）" 之类的预留空间。
	broadcastPartTitleReserve = 32
	// broadcastMaxMessages 单次播报最多发几条，防止几十个订阅把渠道刷爆
	// （MeoW 的硬限制是每分钟 15 条）。
	broadcastMaxMessages = 8
)

// TaskBroadcastSelection 本次要发哪几类。
type TaskBroadcastSelection struct {
	Queue   bool
	Monitor bool
	VPS     bool
	Report  bool
}

type broadcastField struct {
	Label string
	Value string
}

type broadcastItem struct {
	AccountID   string
	Account     string
	Model       string
	PlanCode    string
	Memory      string
	Storage     string
	Bandwidth   string
	Datacenters string
	Monthly     string
	Install     string
	Total       string
	Extra       []broadcastField
	AutoPay     bool
}

type queueBroadcastGroup struct {
	AccountID        string
	PlanCode         string
	Options          []string
	AutoPay          bool
	DatacenterCounts map[string]int
}

func formatBroadcastItem(item broadcastItem) string {
	lines := []string{
		"型号：" + displayOrUnavailable(item.Model),
		"Plan Code：" + displayOrUnavailable(item.PlanCode),
		"内存：" + displayOrUnavailable(item.Memory),
		"数据盘：" + displayOrUnavailable(item.Storage),
		"带宽：" + displayOrUnavailable(item.Bandwidth),
		"数据中心：" + displayOrUnavailable(item.Datacenters),
		"月费：" + displayOrUnavailable(item.Monthly),
		"安装费：" + displayOrUnavailable(item.Install),
		"总价：" + displayOrUnavailable(item.Total),
	}
	for _, field := range item.Extra {
		if strings.TrimSpace(field.Label) == "" {
			continue
		}
		lines = append(lines, field.Label+"："+displayOrUnavailable(field.Value))
	}
	lines = append(lines, "自动付款："+boolLabel(item.AutoPay))
	return strings.Join(lines, "\n")
}

// splitBroadcastText 按字节上限切分，保证不切坏 UTF-8 字符。
func splitBroadcastText(text string, maxBytes int) []string {
	if maxBytes < 1 {
		return []string{text}
	}
	chunks := make([]string, 0, len(text)/maxBytes+1)
	for start := 0; start < len(text); {
		end := start
		for end < len(text) {
			_, size := utf8.DecodeRuneInString(text[end:])
			if end+size-start > maxBytes {
				break
			}
			end += size
		}
		if end == start {
			// 单字符超过上限：硬切一个字符，避免死循环。
			_, size := utf8.DecodeRuneInString(text[start:])
			end = start + size
		}
		chunks = append(chunks, text[start:end])
		start = end
	}
	return chunks
}

func broadcastCategoryHeader(title string, taskCount, part, total int) string {
	title = strings.TrimSpace(title)
	if total > 1 {
		title = fmt.Sprintf("%s（%d/%d）", title, part, total)
	}
	return fmt.Sprintf("%s\n\n正在执行任务数：%d\n\n", title, taskCount)
}

func formatBroadcastCategory(title string, items []broadcastItem) []string {
	return formatBroadcastItems(items, func(part, total int) string {
		return broadcastCategoryHeader(title, len(items), part, total)
	})
}

func formatBroadcastItems(items []broadcastItem, header func(part, total int) string) []string {
	if len(items) == 0 {
		return nil
	}
	type accountGroup struct {
		key   string
		name  string
		items []broadcastItem
	}
	groups := make([]accountGroup, 0)
	groupIndexes := make(map[string]int)
	for _, item := range items {
		accountID := strings.TrimSpace(item.AccountID)
		groupKey := "id:" + accountID
		if accountID == "" {
			groupKey = "name:" + strings.TrimSpace(item.Account)
		}
		index, ok := groupIndexes[groupKey]
		if !ok {
			index = len(groups)
			groupIndexes[groupKey] = index
			groups = append(groups, accountGroup{key: groupKey, name: item.Account})
		}
		groups[index].items = append(groups[index].items, item)
	}

	baseHeader := header(0, 0)
	maxBodyBytes := broadcastMessageMaxBytes - len(baseHeader) - broadcastPartTitleReserve
	if maxBodyBytes < 256 {
		maxBodyBytes = 256
	}
	bodies := make([]string, 0, 1)
	currentBody := ""
	currentAccountKey := ""
	for _, group := range groups {
		accountLine := "OVH 账号：" + displayOrUnavailable(group.name)
		for taskIndex, item := range group.items {
			task := fmt.Sprintf("任务%d：\n%s", taskIndex+1, formatBroadcastItem(item))
			block := task
			if currentBody == "" || currentAccountKey != group.key {
				block = accountLine + "\n" + task
			}
			// 单个任务本身就超长：按字节硬切，标题只在第一片出现。
			if len(block) > maxBodyBytes {
				if currentBody != "" {
					bodies = append(bodies, currentBody)
					currentBody = ""
				}
				parts := splitBroadcastText(block, maxBodyBytes)
				for i, part := range parts {
					if i == 0 {
						bodies = append(bodies, part)
					} else {
						bodies = append(bodies, "（续）\n"+part)
					}
				}
				currentAccountKey = ""
				continue
			}
			if currentBody == "" {
				currentBody = block
			} else if len(currentBody)+2+len(block) > maxBodyBytes {
				bodies = append(bodies, currentBody)
				currentBody = accountLine + "\n" + task
			} else {
				currentBody += "\n\n" + block
			}
			currentAccountKey = group.key
		}
	}
	if currentBody != "" {
		bodies = append(bodies, currentBody)
	}
	if len(bodies) == 0 {
		return nil
	}
	out := make([]string, 0, len(bodies))
	for i, body := range bodies {
		out = append(out, header(i+1, len(bodies))+body)
	}
	return out
}

// BuildTaskBroadcastMessages 构建本次播报的文本（不含战报）。
func (m *Monitor) BuildTaskBroadcastMessages(ctx context.Context, selection TaskBroadcastSelection) []string {
	if m == nil || m.state == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	messages := make([]string, 0, 8)
	if selection.Queue {
		messages = append(messages, m.buildQueueBroadcastMessages(ctx)...)
	}
	if selection.Monitor {
		messages = append(messages, m.buildMonitorBroadcastMessages(ctx)...)
	}
	if selection.VPS {
		messages = append(messages, m.buildVPSBroadcastMessages(ctx)...)
	}
	return messages
}

func (m *Monitor) buildQueueBroadcastMessages(ctx context.Context) []string {
	if m == nil || m.state == nil {
		return nil
	}
	m.state.QueueMu.Lock()
	items := append([]types.QueueItem(nil), m.state.Queue...)
	m.state.QueueMu.Unlock()

	groups := aggregateQueueTasks(items)
	entries := make([]broadcastItem, 0, len(groups))
	for _, group := range groups {
		if ctx.Err() != nil {
			break
		}
		plan := m.serverPlan(group.PlanCode)
		memory, storage, bandwidth := planSpecifications(plan, group.Options)
		dcs := sortedDatacenterCounts(group.DatacenterCounts)
		monthly, install, total := m.broadcastPriceParts(ctx, group.AccountID, group.PlanCode, group.Options)
		extra := []broadcastField{
			{Label: "抢购数量", Value: formatDatacenterCounts(group.DatacenterCounts)},
		}
		entries = append(entries, broadcastItem{
			AccountID: group.AccountID, Account: m.accountName(group.AccountID),
			Model: planName(plan, group.PlanCode), PlanCode: group.PlanCode,
			Memory: memory, Storage: storage, Bandwidth: bandwidth,
			Datacenters: strings.Join(dcs, "，"), Monthly: monthly, Install: install, Total: total,
			Extra: extra, AutoPay: group.AutoPay,
		})
	}
	if len(entries) == 0 {
		return nil
	}
	return formatBroadcastCategory("🛒 抢购任务", entries)
}

func (m *Monitor) buildMonitorBroadcastMessages(ctx context.Context) []string {
	if m == nil || m.state == nil {
		return nil
	}
	entries := make([]broadcastItem, 0)
	for _, sub := range m.Snapshot() {
		if sub == nil || ctx.Err() != nil {
			break
		}
		plan := m.serverPlan(sub.PlanCode)
		memory, storage, bandwidth := planSpecifications(plan, sub.Options)
		dcs := normalizedDatacenters(sub.Datacenters)
		accountID := sub.AutoOrderAccountID
		if accountID == "" {
			accountID = m.defaultAccountID()
		}
		monthly, install, total := m.broadcastPriceParts(ctx, accountID, sub.PlanCode, sub.Options)
		entries = append(entries, broadcastItem{
			AccountID: accountID, Account: m.accountName(accountID),
			Model: firstNonEmpty(sub.ServerName, planName(plan, sub.PlanCode)), PlanCode: sub.PlanCode,
			Memory: memory, Storage: storage, Bandwidth: bandwidth,
			Datacenters: displayDatacenters(dcs), Monthly: monthly, Install: install, Total: total,
			Extra: []broadcastField{
				{Label: "有货提醒", Value: boolLabel(sub.NotifyAvailable)},
				{Label: "无货提醒", Value: boolLabel(sub.NotifyUnavailable)},
				{Label: "有货自动下单", Value: boolLabel(sub.AutoOrder)},
			},
			AutoPay: sub.AutoPay,
		})
	}
	if len(entries) == 0 {
		return nil
	}
	return formatBroadcastCategory("🖥️ 独服监控", entries)
}

func (m *Monitor) buildVPSBroadcastMessages(ctx context.Context) []string {
	if m == nil || m.state == nil {
		return nil
	}
	entries := make([]broadcastItem, 0)
	for _, sub := range m.state.VPSSubscriptionsSnapshot() {
		if ctx.Err() != nil {
			break
		}
		accountID := sub.AutoOrderAccountID
		if accountID == "" {
			accountID = m.defaultAccountID()
		}
		model := m.vpsBroadcastModel(sub.OvhSubsidiary, sub.PlanCode)
		dcs := normalizedDatacenters(sub.Datacenters)
		quantity := sub.Quantity
		if quantity < 1 {
			quantity = 1
		}
		entries = append(entries, broadcastItem{
			AccountID: accountID, Account: m.accountName(accountID),
			Model: firstNonEmpty(model.Name, sub.PlanCode), PlanCode: sub.PlanCode,
			Memory: "—", Storage: "—", Bandwidth: "—",
			Datacenters: displayDatacenters(dcs), Monthly: displayOrUnavailable(model.Price),
			Install: "—", Total: displayOrUnavailable(model.Price),
			Extra: []broadcastField{
				{Label: "有货提醒", Value: boolLabel(sub.NotifyAvailable)},
				{Label: "无货提醒", Value: boolLabel(sub.NotifyUnavailable)},
				{Label: "Linux 系统", Value: boolLabel(sub.MonitorLinux)},
				{Label: "Windows 系统", Value: boolLabel(sub.MonitorWindows)},
				{Label: "有货自动下单", Value: boolLabel(sub.AutoOrder)},
				{Label: "数量", Value: strconv.Itoa(quantity)},
			},
			AutoPay: sub.AutoPay,
		})
	}
	if len(entries) == 0 {
		return nil
	}
	return formatBroadcastCategory("🖥️ VPS 监控", entries)
}

// broadcastPriceParts 用公开目录算价（含税），并附人民币参考价。
// 目录里查不到的 addon 会让报价偏低，必须显式标注——播报里的数字会被
// 用户拿去算预算，不能给一个"看起来精确"的假数。
func (m *Monitor) broadcastPriceParts(ctx context.Context, accountID, planCode string, options []string) (string, string, string) {
	const unavailable = "暂不可用"
	if ctx != nil && ctx.Err() != nil {
		return unavailable, unavailable, unavailable
	}
	planPrice, err := catalog.PriceForOptions(m.state, accountID, planCode, options)
	if err != nil {
		return unavailable, unavailable, unavailable
	}
	monthly := broadcastMoney(planPrice.Currency, planPrice.Monthly, "/月")
	install := "无"
	if planPrice.Install > 0 {
		install = broadcastMoney(planPrice.Currency, planPrice.Install, "")
	}
	total := broadcastMoney(planPrice.Currency, planPrice.Monthly+planPrice.Install, "")
	if planPrice.Partial {
		monthly += "（部分 addon 未计入）"
	}
	return monthly, install, total
}

// broadcastMoney 原币 + （汇率可用时）人民币参考价。
// 汇率不可用只显示原币，绝不回退成 ¥0 —— 和通知文案的口径保持一致。
func broadcastMoney(currency string, amount float64, unit string) string {
	base := formatAmount(currency, amount) + unit
	if cny, ok := exchange.CNY(currency, amount); ok {
		return base + " ≈ " + exchange.FormatCNY(cny) + unit
	}
	return base
}

// SendTaskBroadcast 构建并发送一次播报，返回成功发出的条数。
func (m *Monitor) SendTaskBroadcast(ctx context.Context, selection TaskBroadcastSelection) int {
	if m == nil || m.state == nil {
		return 0
	}
	if ctx == nil {
		ctx = context.Background()
	}
	messages := m.BuildTaskBroadcastMessages(ctx, selection)
	if selection.Report {
		if report := m.BuildBattleReport(); report != "" {
			messages = append(messages, report)
		}
	}
	if len(messages) == 0 {
		return 0
	}
	if len(messages) > broadcastMaxMessages {
		messages = append(messages[:broadcastMaxMessages],
			"…内容过多，完整清单请到网页控制台查看。")
	}
	sent := 0
	for i, message := range messages {
		if ctx.Err() != nil {
			break
		}
		if notify.Broadcast(m.state, message, nil) > 0 {
			sent++
		}
		// 渠道限速：MeoW 3 秒 1 条，Telegram 也别连着轰。
		if i < len(messages)-1 {
			if !waitBroadcast(ctx, 500*time.Millisecond) {
				break
			}
		}
	}
	return sent
}

// SendStartupTaskBroadcast 启动补发（独立开关，默认关）。
// 只发队列/监控/VPS 清单，不发战报——战报是"过去 24 小时"的日报，
// 和"现在在盯什么"不是同一件事。
func (m *Monitor) SendStartupTaskBroadcast(ctx context.Context) int {
	if m == nil || m.state == nil {
		return 0
	}
	if !m.state.Config.Get().IsTaskBroadcastOnStartup() {
		return 0
	}
	return m.SendTaskBroadcast(ctx, TaskBroadcastSelection{Queue: true, Monitor: true, VPS: true})
}

// RunTaskBroadcastLoop 每日播报循环。
//
// 每分钟醒一次而不是 sleep 到目标时刻：配置可能被用户随时改，
// 睡死一整天会让"改成 21:00"要等到明天才生效。
// 停机错过的当天不补发——补发会和第二天的内容重复。
func (m *Monitor) RunTaskBroadcastLoop(ctx context.Context) {
	if m == nil || m.state == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	loc := broadcastLocation()
	for {
		cfg := m.state.Config.Get()
		if !cfg.IsTaskBroadcastEnabled() {
			if !waitBroadcast(ctx, time.Minute) {
				return
			}
			continue
		}
		timeText, err := types.NormalizeTaskBroadcastTime(cfg.TaskBroadcastTime)
		if err != nil {
			timeText = types.DefaultTaskBroadcastTime
		}
		next := nextDailyBroadcastAt(time.Now(), timeText, loc)
		wait := time.Until(next)
		if wait > time.Minute {
			wait = time.Minute
		}
		if wait < 0 {
			wait = 0
		}
		if !waitBroadcast(ctx, wait) {
			return
		}
		if time.Now().In(loc).Before(next) {
			continue
		}
		current := m.state.Config.Get()
		currentTime, currentErr := types.NormalizeTaskBroadcastTime(current.TaskBroadcastTime)
		if currentErr != nil {
			currentTime = types.DefaultTaskBroadcastTime
		}
		if !current.IsTaskBroadcastEnabled() || currentTime != timeText {
			continue
		}
		m.SendTaskBroadcast(ctx, scheduledBroadcastSelection(current))
	}
}

func scheduledBroadcastSelection(cfg types.Config) TaskBroadcastSelection {
	return TaskBroadcastSelection{
		Queue:   cfg.IsTaskBroadcastQueueEnabled(),
		Monitor: cfg.IsTaskBroadcastMonitorEnabled(),
		VPS:     cfg.IsTaskBroadcastVPSEnabled(),
		Report:  cfg.IsTaskBroadcastReportEnabled(),
	}
}

func waitBroadcast(ctx context.Context, duration time.Duration) bool {
	if duration <= 0 {
		return ctx.Err() == nil
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// broadcastLocation 播报固定按北京时间。
// 不跟随容器 TZ：用户把 TZ 改成 UTC 时，"每天 09:00"突然变成北京时间 17:00，
// 而设置页写的是 09:00，这种错位没人能自己发现。
func broadcastLocation() *time.Location {
	if loc, err := time.LoadLocation("Asia/Shanghai"); err == nil {
		return loc
	}
	return time.FixedZone("CST", 8*60*60)
}

func nextDailyBroadcastAt(now time.Time, hhmm string, loc *time.Location) time.Time {
	if loc == nil {
		loc = now.Location()
	}
	normalized, err := types.NormalizeTaskBroadcastTime(hhmm)
	if err != nil {
		normalized = types.DefaultTaskBroadcastTime
	}
	parts := strings.Split(normalized, ":")
	hour, _ := strconv.Atoi(parts[0])
	minute, _ := strconv.Atoi(parts[1])
	localNow := now.In(loc)
	next := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), hour, minute, 0, 0, loc)
	if !next.After(localNow) {
		next = next.Add(24 * time.Hour)
	}
	return next
}

// ---- 以下为播报专用的小工具（monitor 包内原本没有） ----

func (m *Monitor) serverPlan(planCode string) *types.ServerPlan {
	if m == nil || m.state == nil {
		return nil
	}
	m.state.ServerPlansMu.RLock()
	defer m.state.ServerPlansMu.RUnlock()
	for _, plan := range m.state.ServerPlans {
		if strings.EqualFold(plan.PlanCode, planCode) {
			copyPlan := plan
			return &copyPlan
		}
	}
	return nil
}

func planName(plan *types.ServerPlan, fallback string) string {
	if plan != nil && strings.TrimSpace(plan.Name) != "" {
		return strings.TrimSpace(plan.Name)
	}
	return fallback
}

func planValue(plan *types.ServerPlan, getter func(types.ServerPlan) string) string {
	if plan == nil {
		return ""
	}
	return getter(*plan)
}

// planSpecifications 从计划默认规格出发，把用户选中的 addon 覆盖到内存/存储/带宽。
// 订阅本身不存这三项（Options 里是 addon code），必须推导。
func planSpecifications(plan *types.ServerPlan, options []string) (string, string, string) {
	memory := planValue(plan, func(p types.ServerPlan) string { return p.Memory })
	storage := planValue(plan, func(p types.ServerPlan) string { return p.Storage })
	bandwidth := planValue(plan, func(p types.ServerPlan) string { return p.Bandwidth })
	if plan == nil {
		return memory, storage, bandwidth
	}
	for _, selected := range options {
		for _, option := range plan.AvailableOptions {
			if !strings.EqualFold(strings.TrimSpace(selected), strings.TrimSpace(option.Value)) &&
				!strings.EqualFold(strings.TrimSpace(selected), strings.TrimSpace(option.Label)) {
				continue
			}
			family := strings.ToLower(option.Family)
			label := option.Label
			switch {
			case strings.Contains(family, "memory") || strings.Contains(family, "ram"):
				memory = firstNonEmpty(catalog.FormatMemoryDisplay(label), label)
			case strings.Contains(family, "storage") || strings.Contains(family, "disk") || strings.Contains(family, "drive"):
				storage = firstNonEmpty(catalog.FormatStorageDisplay(label), label)
			case strings.Contains(family, "bandwidth") || strings.Contains(family, "network") || strings.Contains(family, "traffic"):
				bandwidth = label
			}
		}
	}
	return memory, storage, bandwidth
}

func (m *Monitor) vpsBroadcastModel(subsidiary, planCode string) vps.Model {
	models, err := vps.Models(subsidiary)
	if err != nil {
		return vps.Model{}
	}
	for _, model := range models {
		if strings.EqualFold(strings.TrimSpace(model.PlanCode), strings.TrimSpace(planCode)) {
			return model
		}
	}
	return vps.Model{}
}

func (m *Monitor) accountName(accountID string) string {
	if m == nil || m.state == nil {
		return accountID
	}
	if account, ok := m.state.FindAccount(accountID); ok {
		return firstNonEmpty(account.Name, account.ID)
	}
	return firstNonEmpty(accountID, "未指定账户")
}

func (m *Monitor) defaultAccountID() string {
	if m == nil || m.state == nil {
		return ""
	}
	if account, ok := m.state.FindAccount(""); ok {
		return account.ID
	}
	return ""
}

func normalizedDatacenters(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToUpper(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func sortedDatacenterCounts(values map[string]int) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func formatDatacenterCounts(values map[string]int) string {
	keys := sortedDatacenterCounts(values)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s %d 台", key, values[key]))
	}
	return strings.Join(parts, "，")
}

func displayDatacenters(values []string) string {
	if len(values) == 0 {
		return "全部"
	}
	return strings.Join(values, "，")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func displayOrUnavailable(value string) string {
	if value = strings.TrimSpace(value); value != "" {
		return value
	}
	return "暂不可用"
}

func boolLabel(value bool) string {
	if value {
		return "是"
	}
	return "否"
}

func aggregateQueueTasks(items []types.QueueItem) []queueBroadcastGroup {
	groups := make(map[string]*queueBroadcastGroup)
	for _, item := range items {
		if !isActiveQueueStatus(item.Status) {
			continue
		}
		options := append([]string(nil), item.Options...)
		key := queueBroadcastKey(item.AccountID, item.PlanCode, options, item.AutoPay)
		group := groups[key]
		if group == nil {
			group = &queueBroadcastGroup{
				AccountID: item.AccountID, PlanCode: item.PlanCode, Options: options,
				AutoPay: item.AutoPay, DatacenterCounts: map[string]int{},
			}
			groups[key] = group
		}
		dc := strings.ToUpper(strings.TrimSpace(item.Datacenter))
		if dc == "" {
			dc = "未知"
		}
		group.DatacenterCounts[dc]++
	}
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]queueBroadcastGroup, 0, len(keys))
	for _, key := range keys {
		group := *groups[key]
		group.Options = append([]string(nil), group.Options...)
		out = append(out, group)
	}
	return out
}

func queueBroadcastKey(accountID, planCode string, options []string, autoPay bool) string {
	return strings.Join([]string{
		strings.TrimSpace(accountID), strings.TrimSpace(planCode),
		strings.Join(options, "\x00"), strconv.FormatBool(autoPay),
	}, "\x01")
}

func isActiveQueueStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "running", "pending", "paused":
		return true
	default:
		return false
	}
}
