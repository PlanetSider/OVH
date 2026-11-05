package handlers

import (
	"fmt"
	"strings"

	"github.com/ovh-buy/server/internal/app"
	"github.com/ovh-buy/server/internal/monitor"
	"github.com/ovh-buy/server/internal/telegram"
	"github.com/ovh-buy/server/internal/types"
)

// feishuCommandNote 是 /help 末尾针对飞书通道的说明。
//
// 不在 helpText() 里直接写，因为那段文本 Telegram 也在用；飞书没有 inline
// keyboard，/watch 的配置选择、/accounts 的切换键盘都无法呈现，得提前说清楚，
// 否则用户发 /watch 会得到一句"请按下面的按钮选择"，然后什么按钮都没有。
const feishuCommandNote = `

—— 飞书通道 ——
命令：/help /status /queue /cancel /watch /unwatch /interval /subs /recent /accounts
下单：直接发「型号 机房 数量」，例 24sk602 gra 2；多账户用 @序号 指定
监控：/watch 24sk602 gra x2（带 x 才会自动下单）`

// BotCommandReply 是各 IM 通道共用的「纯文本命令」实现。
//
// 与 telegram_commands.go 里 handleCommand 的分工：
//   - handleCommand 负责 Telegram 专有的交互（按钮式 /watch、/accounts 切换键盘），
//     它自己发消息；
//   - 这里只解析命令并返回文本，由调用方决定怎么发（Telegram 走 SendReply，
//     飞书走 channel.Send）。
//
// 两边复用同一批 *Text 函数（helpText/statusText/queueText/…），命令语义只有一份，
// 不会出现"Telegram 上能 /queue、飞书上是另一套"的漂移。
func BotCommandReply(state *app.State, mon *monitor.Monitor, text string) (string, bool) {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "/") {
		return "", false
	}
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return "", false
	}
	cmd := strings.ToLower(strings.TrimPrefix(fields[0], "/"))
	// /status@bot_name 是群里的写法，去掉 @ 后缀
	if i := strings.Index(cmd, "@"); i >= 0 {
		cmd = cmd[:i]
	}
	args := fields[1:]

	switch cmd {
	case "start", "help", "h", "?":
		return helpText() + feishuCommandNote, true
	case "status", "s":
		return statusText(state, mon), true
	case "queue", "q":
		return queueText(state), true
	case "cancel":
		return cancelText(state, args), true
	case "interval", "iv":
		return intervalText(state, args), true
	case "subs", "sub":
		return subsText(state, mon), true
	case "recent", "history":
		return recentText(state), true
	case "accounts", "acc":
		return accountsTextPlain(state), true
	case "watch", "w":
		// 飞书没有按钮流程：带参数直接建订阅；不带参数回用法。
		return watchText(state, mon, args), true
	case "unwatch", "uw":
		return unwatchText(state, mon, args), true
	default:
		return "❓ 不认识的命令: /" + cmd + "\n\n发 /help 看能用什么。", true
	}
}

// accountsTextPlain 是 /accounts 的纯文本版本。
//
// Telegram 的 accountsText 在账户数 ≥2 时会改发一排切换按钮并返回空串；
// 飞书没有按钮，就只列出账户和当前生效的那个，切换动作留给网页。
func accountsTextPlain(state *app.State) string {
	accs := listAccounts(state)
	if len(accs) == 0 {
		return "还没有配置 OVH 账户。请到网页控制台「设置 → 账户」添加。"
	}
	cur, explicit := telegram.ActiveAccount(state)

	var b strings.Builder
	fmt.Fprintf(&b, "OVH 账户（%d 个）\n\n", len(accs))
	for _, acc := range accs {
		mark := "  "
		if acc.ID == cur.ID {
			mark = "✅"
		}
		line := fmt.Sprintf("%s %s · %s/%s", mark, telegram.AccountLabel(acc), acc.Endpoint, acc.Zone)
		if acc.IsDefault {
			line += " · 默认"
		}
		b.WriteString(line + "\n")
	}
	if explicit {
		b.WriteString("\n当前指定账户：" + telegram.AccountLabel(cur))
	} else {
		b.WriteString("\n当前使用默认账户：" + telegram.AccountLabel(cur))
	}
	b.WriteString("\n切换账户请到网页控制台操作。")
	return b.String()
}

// BotTextOrder 解析并执行一条「文本下单」消息（与 Telegram 同一套语法）：
//
//	<型号> [机房] [数量] [配置] [@账户]
//
// 与 Telegram 的差别只有确认环节。Telegram 在没有写 @账户 时会发一个带按钮的
// 确认卡片；飞书目前没有按钮通道，所以这里只在"同一区有多个账户"时停下来，
// 要求用 @序号 指定 —— 单一候选直接下单，和 Telegram 里显式写 @账户 时跳过确认
// 是同一个理由：抢购最不能忍的就是多一次往返。
//
// 返回 (reply, true) 表示这条消息确实是下单格式。
func BotTextOrder(state *app.State, mon *monitor.Monitor, text string) (string, bool) {
	o := telegram.ParseOrderMessage(strings.TrimSpace(text))
	if o == nil {
		return "", false
	}
	if o.Error != "" {
		return o.Error, true
	}

	// 显式写了 @账户：照做，不再拦
	if ref := strings.TrimSpace(o.AccountRef); ref != "" {
		accs, errMsg := resolveAccountRef(state, mon, ref, o.PlanCode)
		if errMsg != "" {
			return "❌ " + errMsg, true
		}
		return runOrder(state, o, accs), true
	}

	ra := resolveOrderAccount(state, mon, o.PlanCode)
	if ra.Account.ID == "" {
		return "❌ 无法下单\n\n" + ra.Reason, true
	}
	if len(ra.Ambiguous) > 1 {
		return ambiguousAccountText(state, o, ra), true
	}

	reply := runOrder(state, o, []types.OVHAccount{ra.Account})
	if !ra.Confident {
		// 判不出来时也下单（目录没热起来就拒绝，等于让一次抖动毁掉一次补货），
		// 但必须提示：用错区的账户不会报错，只会一直抢不到。
		reply = "⚠️ 没能确认这个型号属于哪个区，按当前账户下单。\n" +
			"用错区不会报错、只会一直抢不到。\n\n" + reply
	}
	return reply, true
}

// ambiguousAccountText 同一区有多个账户时的提示：列出可选的全局序号。
//
// 序号必须和 /accounts 的顺序一致（resolveAccountRef 按 listAccounts 的下标解析），
// 所以这里遍历全量账户找属于 Ambiguous 的那几个，而不是用 Ambiguous 自己的下标。
func ambiguousAccountText(state *app.State, o *telegram.OrderInfo, ra resolvedAccount) string {
	all := listAccounts(state)
	var b strings.Builder
	fmt.Fprintf(&b, "⚠️ %s 在 %d 个账户里都能买，先指定一个（避免落错区白抢）：\n\n",
		o.PlanCode, len(ra.Ambiguous))
	for i, acc := range all {
		for _, cand := range ra.Ambiguous {
			if acc.ID == cand.ID {
				fmt.Fprintf(&b, "  @%d  %s\n", i+1, telegram.AccountLabel(acc))
				break
			}
		}
	}
	fmt.Fprintf(&b, "\n重发并在末尾加 @序号，例如：%s", orderExample(o))
	if o.Datacenter == "" {
		b.WriteString("\n（忘了写机房的话，重发时补上：gra / rbx / bhs …）")
	}
	return b.String()
}

// orderExample 拼一条"重发示例"：只带上用户原本写过的参数，别替人脑补。
func orderExample(o *telegram.OrderInfo) string {
	ex := o.PlanCode
	if o.Datacenter != "" {
		ex += " " + o.Datacenter
	}
	if o.Quantity > 1 {
		ex += fmt.Sprintf(" %d", o.Quantity)
	}
	return ex + " @1"
}
