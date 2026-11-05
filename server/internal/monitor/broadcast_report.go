package monitor

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ovh-buy/server/internal/types"
)

// 抢购战报：过去 24 小时内成功下单的统计，按 OVH 账号分组。
//
// 口径：
//   - 只统计 status=success 的历史；失败单不进战报（失败原因各不同，
//     堆在一起只会让"今天抢到几台"这个信息被淹没）。
//   - "未支付"按 OVH 订单状态判断，缺省/未知状态一律算未支付——
//     宁可提醒用户"还有单没付"，也不要把一单未付款说成已完成。
//   - 金额用不含税价（billing 的口径），并附人民币参考价。
//     多笔订单币种不一致时金额显示"暂不可用"，不做汇率合并——那会造一个
//     谁也对不上的数。

type battleReportAccount struct {
	Name           string
	Successful     int
	Unpaid         int
	Paid           int
	UnpaidAmount   float64
	PaidAmount     float64
	UnpaidCurrency string
	PaidCurrency   string
	UnpaidKnown    bool
	PaidKnown      bool
}

// BuildBattleReport 生成战报文本；没有历史时也会给出全 0 的完整结构，
// 避免用户以为"战报坏了"。
func (m *Monitor) BuildBattleReport() string {
	if m == nil || m.state == nil {
		return ""
	}
	m.state.HistoryMu.Lock()
	history := append([]types.PurchaseHistoryEntry(nil), m.state.History...)
	m.state.HistoryMu.Unlock()

	accountNames := make(map[string]string)
	for _, entry := range history {
		id := strings.TrimSpace(entry.AccountID)
		if id == "" {
			continue
		}
		if _, ok := accountNames[id]; !ok {
			accountNames[id] = m.accountName(id)
		}
	}
	return formatBattleReport(history, time.Now(), accountNames)
}

func formatBattleReport(history []types.PurchaseHistoryEntry, now time.Time, accountNames map[string]string) string {
	accounts := make(map[string]*battleReportAccount)
	cutoff := now.Add(-24 * time.Hour)
	for _, entry := range history {
		if strings.ToLower(strings.TrimSpace(entry.Status)) != "success" {
			continue
		}
		when, ok := types.ParseTS(entry.PurchaseTime)
		if !ok || when.Before(cutoff) || when.After(now) {
			continue
		}
		id := strings.TrimSpace(entry.AccountID)
		name := strings.TrimSpace(accountNames[id])
		if name == "" {
			name = id
		}
		if name == "" {
			name = "未指定账户"
		}
		account := accounts[id]
		if account == nil {
			account = &battleReportAccount{Name: name, UnpaidKnown: true, PaidKnown: true}
			accounts[id] = account
		}
		account.Successful++
		amount, currency, known := battleAmountOf(entry)
		if isUnpaidBattleStatus(entry.OrderStatus) {
			account.Unpaid++
			if !known {
				account.UnpaidKnown = false
			} else {
				account.UnpaidAmount += amount
				account.UnpaidCurrency = mergeBattleCurrency(account.UnpaidCurrency, currency)
			}
			continue
		}
		account.Paid++
		if !known {
			account.PaidKnown = false
		} else {
			account.PaidAmount += amount
			account.PaidCurrency = mergeBattleCurrency(account.PaidCurrency, currency)
		}
	}

	keys := make([]string, 0, len(accounts))
	for id := range accounts {
		keys = append(keys, id)
	}
	sort.Slice(keys, func(i, j int) bool {
		left, right := accounts[keys[i]], accounts[keys[j]]
		if left.Name != right.Name {
			return left.Name < right.Name
		}
		return keys[i] < keys[j]
	})

	lines := []string{"🏆 抢购战报"}
	if len(keys) == 0 {
		lines = append(lines, "", "OVH 账号：暂无",
			"过去 24 小时成功下单：0",
			"未支付订单：0 单",
			"未支付金额：0",
			"已支付订单：0 单",
			"已支付金额：0")
	}
	for _, id := range keys {
		account := accounts[id]
		lines = append(lines, "", "OVH 账号："+account.Name,
			fmt.Sprintf("过去 24 小时成功下单：%d", account.Successful),
			fmt.Sprintf("未支付订单：%d 单", account.Unpaid),
			"未支付金额："+battleAmount(account.Unpaid, account.UnpaidKnown, account.UnpaidAmount, account.UnpaidCurrency),
			fmt.Sprintf("已支付订单：%d 单", account.Paid),
			"已支付金额："+battleAmount(account.Paid, account.PaidKnown, account.PaidAmount, account.PaidCurrency))
	}
	return strings.Join(lines, "\n")
}

// battleAmountOf 取一笔订单的不含税金额。缺价格或币种时返回 known=false，
// 由调用方把"金额"标成暂不可用，而不是当 0 累加。
func battleAmountOf(entry types.PurchaseHistoryEntry) (float64, string, bool) {
	if entry.Price == nil || entry.Price.WithoutTax == nil {
		return 0, "", false
	}
	currency := strings.ToUpper(strings.TrimSpace(entry.Price.CurrencyCode))
	if currency == "" {
		return 0, "", false
	}
	return *entry.Price.WithoutTax, currency, true
}

// isUnpaidBattleStatus OVH 的订单状态不统一（历史数据里大小写、
// 下划线、连字符都出现过），先归一化再判断。
func isUnpaidBattleStatus(status string) bool {
	normalized := strings.ToLower(strings.TrimSpace(status))
	normalized = strings.ReplaceAll(normalized, " ", "")
	normalized = strings.ReplaceAll(normalized, "_", "")
	normalized = strings.ReplaceAll(normalized, "-", "")
	switch normalized {
	case "", "notpaid", "unpaid", "paymentpending", "pendingpayment",
		"waitingpayment", "pending", "created", "new":
		return true
	default:
		return false
	}
}

// mergeBattleCurrency 多笔金额只有在币种一致时才能相加；
// 不一致置空 → battleAmount 显示"暂不可用"。
func mergeBattleCurrency(current, next string) string {
	next = strings.ToUpper(strings.TrimSpace(next))
	if current == "" {
		return next
	}
	if next == "" || current != next {
		return ""
	}
	return current
}

func battleAmount(count int, known bool, amount float64, currency string) string {
	if count == 0 {
		return "0"
	}
	if !known || currency == "" {
		return "暂不可用"
	}
	return broadcastMoney(currency, amount, "")
}
