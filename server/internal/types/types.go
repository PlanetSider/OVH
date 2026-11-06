package types

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	AppKey      string `json:"appKey"`
	AppSecret   string `json:"appSecret"`
	ConsumerKey string `json:"consumerKey"`
	Endpoint    string `json:"endpoint"`
	TgToken     string `json:"tgToken"`
	TgChatID    string `json:"tgChatId"`
	IAM         string `json:"iam"`
	Zone        string `json:"zone"`

	// NotifyWebhookURL 第二条通知通道:一个接收 JSON POST 的地址(钉钉/飞书/Bark/自建都行)。
	// 补货监控的全部价值就是"有货那一刻你能收到消息",单通道意味着 Telegram 一挂就全盲。
	NotifyWebhookURL string `json:"notifyWebhookUrl,omitempty"`

	// DefaultRetryInterval 新建抢购任务的默认重试间隔(秒)。
	// 网页弹窗、TG /buy、上架通知里的一键下单按钮不显式指定时都用它。
	// 以前四条入队路径各写各的(30 / 30 / 30,前端弹窗还显示 60),用户既改不了也对不上。
	DefaultRetryInterval int `json:"defaultRetryInterval,omitempty"`
	// QuickOrderRetryInterval 监控触发的自动下单(/watch 自动抢)用的重试间隔(秒)。
	// 单独一个值是因为场景不同:货刚出现那一刻要抢,窗口可能只有几十秒,
	// 所以默认比普通任务激进得多;但太密会吃 OVH 的 429,这里交给用户自己权衡。
	QuickOrderRetryInterval int `json:"quickOrderRetryInterval,omitempty"`

	// DisplayMode 是否显示人民币参考价:"both"(原币 + ≈ ¥) / "off"(只看原币)。
	//
	// 存进 Config 而不是前端 localStorage,是因为同一份判据还要被后端通知
	// (TG / 飞书 / webhook)使用 —— 界面上关了人民币、通知里却还带着,用户
	// 无法解释这种不一致。空值/未知值一律按 "both" 处理,兼容旧配置。
	DisplayMode string `json:"displayMode,omitempty"`

	// —— 飞书机器人(长连接,双向) ——
	//
	// 和 NotifyWebhookURL 的飞书群机器人是两条完全不同的通道:
	//   - Webhook:只出,往群里发文本,零配置;
	//   - 应用:能进能出,私聊收命令、卡片带按钮、回调触发下单。
	// 所以凭据单独存:App Secret 是能冒充机器人发消息、甚至触发下单的密钥,
	// 和 OVH 凭据同级,必须走 secret.Encrypt。
	FeishuAppID       string `json:"feishuAppId,omitempty"`
	FeishuAppSecret   string `json:"feishuAppSecret,omitempty"`
	FeishuDomain      string `json:"feishuDomain,omitempty"`      // feishu | lark
	FeishuBoundOpenID string `json:"feishuBoundOpenId,omitempty"` // 唯一被授权的私聊接收人
	// FeishuEnabled 临时停用开关。凭据还在、连接不建,用于排查问题时不丢配置。
	FeishuEnabled bool `json:"feishuEnabled,omitempty"`

	// ServerChanURL Server酱³ 的完整推送地址,形如
	// https://<uid>.push.ft07.com/send/<sendkey>.send
	// 参数是 title/desp,不是通用 JSON,所以不能塞进 NotifyWebhookURL。
	ServerChanURL string `json:"serverChanUrl,omitempty"`
	// MeowNickname MeoW 推送昵称,消息发到 https://api.chuckfang.com/<昵称>。
	MeowNickname string `json:"meowNickname,omitempty"`

	// —— 通道开关 ——
	// 用 *bool 而不是 bool：老配置里没有这几个字段，nil 必须按「开」处理，
	// 否则升级后已经填好的通道会全部静默失效。
	//
	// 为什么要独立开关：填了地址 ≠ 想收消息。临时停一个通道（比如 MeoW
	// 每天额度用完、公司 webhook 在维护）时，用户不该被迫删掉配置再重填。
	WebhookEnabled    *bool `json:"webhookEnabled,omitempty"`
	ServerChanEnabled *bool `json:"serverChanEnabled,omitempty"`
	MeowEnabled       *bool `json:"meowEnabled,omitempty"`

	// —— 定时播报（每日一次，把当前任务/订阅/战报发到所有已配置的通知通道） ——
	//
	// 为什么用 *bool 而不是 bool：旧配置里根本没有这些键，读出来是零值 false。
	// 用 bool 就分不清"升级后还没配"和"用户主动关掉了"。
	// 默认姿态：分类开关 nil = 开（打开总开关就该收到完整清单），
	// 总开关 / 启动播报 nil = 关（升级不能突然开始发消息）。
	TaskBroadcastEnabled        *bool  `json:"taskBroadcastEnabled,omitempty"`
	TaskBroadcastTime           string `json:"taskBroadcastTime,omitempty"` // HH:MM，按 Asia/Shanghai
	TaskBroadcastQueueEnabled   *bool  `json:"taskBroadcastQueueEnabled,omitempty"`
	TaskBroadcastMonitorEnabled *bool  `json:"taskBroadcastMonitorEnabled,omitempty"`
	TaskBroadcastVPSEnabled     *bool  `json:"taskBroadcastVpsEnabled,omitempty"`
	TaskBroadcastReportEnabled  *bool  `json:"taskBroadcastReportEnabled,omitempty"`
	// TaskBroadcastOnStartup 服务启动时补发一次队列/监控/VPS 清单。
	// 独立于每日开关：容器和自更新会频繁重启，默认关，需要的人自己打开。
	TaskBroadcastOnStartup *bool `json:"taskBroadcastOnStartup,omitempty"`
}

// DisplayMode 的合法取值。
const (
	DisplayModeBoth = "both"
	DisplayModeOff  = "off"
)

// 重试间隔的默认值与合法区间(秒)。
const (
	DefaultTaskRetryInterval  = 60
	DefaultQuickRetryInterval = 2
	MinRetryInterval          = 1
	MaxRetryInterval          = 86400
)

// ClampRetryInterval 把重试间隔夹到合法区间;<= 0 视为"没设",退回 fallback。
//
// 处理器、入队路径、设置保存都走这一个函数。0 必须兜住:处理器的就绪判断是
// `now - last >= interval`,间隔为 0 时恒真,任务会每秒重试一次把 OVH 刷到 429 ——
// 旧库里 retry_interval 列后加的行、任何忘了设这个字段的入队路径都会踩到。
// —— 下单规模上限 ——
//
// 这几个数原来只写在 telegram 包里,于是只有 TG 那条路受保护。
// 网页端建任务的循环是 `for dc { for i < qty { POST /queue } }`,两个上界都没有:
// 数量填 9999、选 5 个机房 = 约 5 万次串行请求,浏览器卡死、库里塞 5 万条任务,
// 而每一条都是一次真实的下单尝试。多打一个数字的代价不该是这个。
//
// 放在 types 里是为了让所有入队路径共用同一份约束 —— 之前"能力在一条路径有、
// 另一条没有"已经出过好几次问题了。
const (
	// MaxOrderQuantity 单个机房最多下几台。
	// 没有上限时 "planCode 4000000000" 会让入队方先把 40 亿个 QueueItem
	// append 进一个切片 —— 进程当场 OOM 被杀。
	MaxOrderQuantity = 20
	// MaxOrderFanout 一次操作最多创建多少个抢购任务。
	// 不指定机房时任务数 = 配置数 × 机房数 × 数量,很容易远超用户直觉。
	MaxOrderFanout = 60
	// MaxQueueItems 队列里最多存多少条任务。
	// 这是最后一道闸:不管任务从哪条路进来(网页 / 快速下单 / TG / 监控自动下单),
	// 超过这个数一律拒绝。每条任务都是一次真实下单尝试,正常用法离这个数很远。
	MaxQueueItems = 500
)

func ClampRetryInterval(v, fallback int) int {
	if v <= 0 {
		v = fallback
	}
	if v < MinRetryInterval {
		return MinRetryInterval
	}
	if v > MaxRetryInterval {
		return MaxRetryInterval
	}
	return v
}

// NormalizeDisplayMode 只认 off / both，其余（含空/旧配置）一律回退 both。
// 保存设置、读取配置、通知路径共用同一个判据。
func NormalizeDisplayMode(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case DisplayModeOff:
		return DisplayModeOff
	default:
		return DisplayModeBoth
	}
}

// DefaultTaskBroadcastTime 每日播报的默认时间（北京时间 09:00）。
const DefaultTaskBroadcastTime = "09:00"

// flagDefault 把"没配过"归到调用方指定的默认值。
// nil 一律视为未配置——旧配置里没有这些键，反序列化后就是 nil。
func flagDefault(v *bool, def bool) bool {
	if v == nil {
		return def
	}
	return *v
}

// IsWebhookEnabled 通用 Webhook 开关（默认开）。
// 只要地址非空且开关为开才会真的发；临时停用时保留配置。
func (c Config) IsWebhookEnabled() bool {
	return flagDefault(c.WebhookEnabled, true)
}

func (c Config) IsServerChanEnabled() bool {
	return flagDefault(c.ServerChanEnabled, true)
}

func (c Config) IsMeowEnabled() bool {
	return flagDefault(c.MeowEnabled, true)
}

// IsTaskBroadcastEnabled 每日定时播报总开关（默认关）。
func (c Config) IsTaskBroadcastEnabled() bool {
	return flagDefault(c.TaskBroadcastEnabled, false)
}

// IsTaskBroadcastOnStartup 启动补发开关（默认关）。
// 与每日开关解耦：想每次重启确认"在盯什么"的人可以只开这个。
func (c Config) IsTaskBroadcastOnStartup() bool {
	return flagDefault(c.TaskBroadcastOnStartup, false)
}

// 分类开关默认开：总开关是用户唯一需要理解的门，打开后不应再怀疑
// "为什么只发了队列、没发监控"。
func (c Config) IsTaskBroadcastQueueEnabled() bool {
	return flagDefault(c.TaskBroadcastQueueEnabled, true)
}

func (c Config) IsTaskBroadcastMonitorEnabled() bool {
	return flagDefault(c.TaskBroadcastMonitorEnabled, true)
}

func (c Config) IsTaskBroadcastVPSEnabled() bool {
	return flagDefault(c.TaskBroadcastVPSEnabled, true)
}

func (c Config) IsTaskBroadcastReportEnabled() bool {
	return flagDefault(c.TaskBroadcastReportEnabled, true)
}

// NormalizeTaskBroadcastTime 校验并规范化 "HH:MM"。
// 空值给默认时间；非法值返回错误，由调用方决定是拒绝保存还是回退默认。
func NormalizeTaskBroadcastTime(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return DefaultTaskBroadcastTime, nil
	}
	parts := strings.Split(value, ":")
	if len(parts) != 2 {
		return "", fmt.Errorf("时间格式必须是 HH:MM")
	}
	hour, hourErr := strconv.Atoi(parts[0])
	minute, minuteErr := strconv.Atoi(parts[1])
	if hourErr != nil || minuteErr != nil || hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return "", fmt.Errorf("时间必须在 00:00 到 23:59 之间")
	}
	return fmt.Sprintf("%02d:%02d", hour, minute), nil
}

// 飞书域名。海外租户(tenant_brand=lark)要用 larksuite.com 的接口,
// 但连接层和扫码注册都不看这个值决定"用哪个端点"——它只决定消息 API 的 host。
const (
	FeishuDomainFeishu = "feishu"
	FeishuDomainLark   = "lark"
)

// NormalizeFeishuDomain 只认 lark,其余(含空)一律 feishu。
func NormalizeFeishuDomain(s string) string {
	if strings.EqualFold(strings.TrimSpace(s), FeishuDomainLark) {
		return FeishuDomainLark
	}
	return FeishuDomainFeishu
}

// FeishuConfigured 凭据是否齐全。连接层、设置页、扫码结果页共用这一份判据。
// 注意:已绑定接收人(FeishuBoundOpenID)不在这里判断——没绑定也能连上,
// 只是收不到授权命令。
func (c Config) FeishuConfigured() bool {
	return strings.TrimSpace(c.FeishuAppID) != "" && strings.TrimSpace(c.FeishuAppSecret) != ""
}

// DefaultConfig 默认配置
func DefaultConfig() Config {
	return Config{
		Endpoint:                "ovh-eu",
		IAM:                     "go-ovh-ie",
		Zone:                    "IE",
		DefaultRetryInterval:    DefaultTaskRetryInterval,
		QuickOrderRetryInterval: DefaultQuickRetryInterval,
		DisplayMode:             DisplayModeBoth,
		FeishuDomain:            FeishuDomainFeishu,
		TaskBroadcastTime:       DefaultTaskBroadcastTime,
	}
}

// LogEntry 日志条目（字段名与前端 JSON 结构一致）
type LogEntry struct {
	ID        string `json:"id"`
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"`
	Message   string `json:"message"`
	Source    string `json:"source"`
}

// Stats 对应 /api/stats 响应
type Stats struct {
	ActiveQueues          int  `json:"activeQueues"`
	TotalServers          int  `json:"totalServers"`
	AvailableServers      int  `json:"availableServers"`
	PurchaseSuccess       int  `json:"purchaseSuccess"`
	PurchaseFailed        int  `json:"purchaseFailed"`
	QueueProcessorRunning bool `json:"queueProcessorRunning"`
	MonitorRunning        bool `json:"monitorRunning"`
}

// OVHAccount OVH 账户凭据。多账户场景下每条记录代表一个 OVH 账户。
type OVHAccount struct {
	ID          string `json:"id"`       // UUID
	Name        string `json:"name"`     // 用户起的名字（"主号" / "小号 A"）
	Endpoint    string `json:"endpoint"` // ovh-eu / ovh-us / ovh-ca
	Zone        string `json:"zone"`     // IE/FR/DE/US/CA/...
	AppKey      string `json:"appKey"`
	AppSecret   string `json:"appSecret"`
	ConsumerKey string `json:"consumerKey"`
	IAM         string `json:"iam"`       // go-ovh-<zone-lower>
	IsDefault   bool   `json:"isDefault"` // 默认账户（未指定时 fallback 用它）
	CreatedAt   string `json:"createdAt"`

	// ProxyURL 这个账户的出站代理。空 = 直连。
	//
	//	http://user:pass@host:port
	//	socks5://user:pass@host:port
	//
	// 为什么要按账户隔离出口:OVH 的限流是按来源 IP 算的,多个账户共用一个出口时
	// 一个账户被限流会把其它账户一起拖下水 —— 而这恰好发生在补货那一刻。
	//
	// 带凭据,所以和 AppSecret 一样加密落盘;GetAccounts 回前端时打码。
	ProxyURL string `json:"proxyUrl,omitempty"`

	// Fingerprint 出站指纹配置名(见 internal/netfp.Profiles)。空 = default。
	//
	// 注意它能做到的程度有限:Go 标准库不允许控制 JA3 的主要构成要素
	// (套件顺序被忽略、TLS 1.3 套件不可配、扩展顺序固定),
	// 所以这里改的是 TLS 版本区间、ALPN/h2、以及 UA 这类头。详见 netfp 包的说明。
	Fingerprint string `json:"fingerprint,omitempty"`
}

// QueueItem 抢购队列项
type QueueItem struct {
	Duration      string   `json:"duration,omitempty"` // 购买时长；空值兼容旧任务，按 1 个月下单。
	ID            string   `json:"id"`
	AccountID     string   `json:"accountId"` // 该任务下单时用的 OVH 账户
	PlanCode      string   `json:"planCode"`
	Datacenter    string   `json:"datacenter"`
	Options       []string `json:"options"`
	Status        string   `json:"status"` // running / pending / paused / completed
	CreatedAt     string   `json:"createdAt"`
	UpdatedAt     string   `json:"updatedAt"`
	RetryInterval int      `json:"retryInterval"`
	RetryCount    int      `json:"retryCount"`
	// FailureCount 只统计"真的向 OVH 提交过并失败"的次数;无货的空轮不算。
	// MaxRetries 封顶用它而不是 RetryCount —— 抢购的常态就是绝大多数轮次都无货,
	// 拿轮次封顶会让任务在还没真正试过几次时就被判死。
	FailureCount  int     `json:"failureCount,omitempty"`
	MaxRetries    int     `json:"maxRetries,omitempty"`
	LastCheckTime float64 `json:"lastCheckTime"`
	QuickOrder    bool    `json:"quickOrder,omitempty"`
	Priority      int     `json:"priority,omitempty"`
	FromTelegram  bool    `json:"fromTelegram,omitempty"`
	// AutoPay 下单成功后让 OVH 用账户默认支付方式自动付款
	// (checkout 的 autoPayWithPreferredPaymentMethod,schema 描述:
	// "order will be automatically paid with preferred payment method")。
	// 默认 false:自动扣钱必须是用户显式打开的开关,不能是隐含行为。
	AutoPay            bool   `json:"autoPay,omitempty"`
	Coupon             string `json:"coupon,omitempty"`
	Preorder           bool   `json:"preorder,omitempty"` // 本地预订:等账户销售目录上架后再尝试下单
	ConfigSniperTaskID string `json:"configSniperTaskId,omitempty"`
}

// PriceInfo 价格信息
type PriceInfo struct {
	WithTax      *float64 `json:"withTax"`
	WithoutTax   *float64 `json:"withoutTax"`
	Tax          *float64 `json:"tax"`
	CurrencyCode string   `json:"currencyCode"`
}

// PurchaseHistoryEntry 抢购历史
type PurchaseHistoryEntry struct {
	Duration       string   `json:"duration,omitempty"` // 购买时长；空值兼容旧任务，按 1 个月下单。
	ID             string   `json:"id"`
	AccountID      string   `json:"accountId"` // 哪个账户买的
	TaskID         string   `json:"taskId"`
	PlanCode       string   `json:"planCode"`
	Datacenter     string   `json:"datacenter"`
	Options        []string `json:"options"`
	Status         string   `json:"status"` // success / failed
	OrderID        string   `json:"orderId"`
	OrderURL       string   `json:"orderUrl"`
	ErrorMessage   *string  `json:"errorMessage"`
	CouponWarning  string   `json:"couponWarning,omitempty"`
	PurchaseTime   string   `json:"purchaseTime"`
	AttemptCount   int      `json:"attemptCount"`
	ExpirationTime string   `json:"expirationTime,omitempty"`
	// RetractionTime 订单的撤销权截止时间（billing.Order.retractionDate）。
	// 单独开一个字段而不是塞进 ExpirationTime：retractionDate 是"多久内可无理由撤单"，
	// expirationDate 是"订单未付款何时作废"，语义不同，混用会让用户把撤销期当成付款截止期。
	RetractionTime string     `json:"retractionTime,omitempty"`
	Price          *PriceInfo `json:"price,omitempty"`
	// Timing 这一单每个阶段花了多久。抢购输了之后唯一有用的信息就是"慢在哪一步" ——
	// 是 OVH 的库存接口慢、还是自己这台机器建购物车慢、还是最后 checkout 排队了。
	Timing  []PhaseTiming `json:"timing,omitempty"`
	TotalMs int64         `json:"totalMs,omitempty"`
	// OrderStatus OVH 侧的订单状态(billing.order.OrderStatusEnum):
	// notPaid / checking / delivering / delivered / cancelled / cancelling /
	// documentsRequested / unknown。来自 GET /me/order/{orderId}/status,
	// 三区都有。"下单成功"≠"已付款",没有它用户永远不知道订单到底付了没。
	OrderStatus string `json:"orderStatus,omitempty"`
	// OrderStatusAt 上次刷新状态的时间,给节流和"这是多久以前的状态"用
	OrderStatusAt string `json:"orderStatusAt,omitempty"`
}

// PhaseTiming 抢购链路上一个阶段的墙钟耗时
type PhaseTiming struct {
	Name string `json:"name"`
	Ms   int64  `json:"ms"`
}

// Datacenter 服务器目录中单个机房可用性
type Datacenter struct {
	Datacenter   string `json:"datacenter"`
	Availability string `json:"availability"`
	DCName       string `json:"dcName,omitempty"`
	Region       string `json:"region,omitempty"`
}

// ServerOption 选项标签
type ServerOption struct {
	Label     string `json:"label"`
	Value     string `json:"value"`
	Family    string `json:"family,omitempty"`
	IsDefault bool   `json:"isDefault,omitempty"`
}

// ServerPlan 服务器目录项
type ServerPlan struct {
	PlanCode         string         `json:"planCode"`
	Name             string         `json:"name"`
	Description      string         `json:"description"`
	CPU              string         `json:"cpu"`
	Memory           string         `json:"memory"`
	Storage          string         `json:"storage"`
	Bandwidth        string         `json:"bandwidth"`
	VrackBandwidth   string         `json:"vrackBandwidth"`
	Datacenters      []Datacenter   `json:"datacenters"`
	DefaultOptions   []ServerOption `json:"defaultOptions"`
	AvailableOptions []ServerOption `json:"availableOptions"`
}

// SubscriptionHistoryEntry 监控订阅的历史记录条目
type SubscriptionHistoryEntry struct {
	Timestamp  string                 `json:"timestamp"`
	Datacenter string                 `json:"datacenter"`
	Status     string                 `json:"status"`
	ChangeType string                 `json:"changeType"`
	OldStatus  interface{}            `json:"oldStatus"`
	Config     map[string]interface{} `json:"config,omitempty"`
}

// Subscription 监控订阅（跨账户共享列表;auto-order 触发时按 AutoOrderAccountID 下单）
type Subscription struct {
	Duration           string                     `json:"duration,omitempty"` // 购买时长；空值兼容旧任务，按 1 个月下单。
	PlanCode           string                     `json:"planCode"`
	Datacenters        []string                   `json:"datacenters"`
	NotifyAvailable    bool                       `json:"notifyAvailable"`
	NotifyUnavailable  bool                       `json:"notifyUnavailable"`
	LastStatus         map[string]string          `json:"lastStatus"`
	CreatedAt          string                     `json:"createdAt"`
	History            []SubscriptionHistoryEntry `json:"history"`
	ServerName         string                     `json:"serverName,omitempty"`
	AutoOrder          bool                       `json:"autoOrder,omitempty"`
	Quantity           int                        `json:"quantity,omitempty"`
	AutoOrderAccountID string                     `json:"autoOrderAccountId,omitempty"` // 空 = 触发时只通知不下单
	// AutoPay 下单成功后用默认支付方式自动付款(显式开关,默认关)
	AutoPay bool `json:"autoPay,omitempty"`
	// Paused 暂停监控:检查循环跳过,订阅与历史保留
	Paused bool `json:"paused,omitempty"`
	// Options 只盯这套配置(addon planCode 列表,如 ram-64g / softraid-2x480ssd)。
	//
	// 空 = 盯这个 planCode 的**全部**配置,也是一直以来的行为。
	//
	// 为什么需要它:一个 planCode 底下往往有好几套内存/存储组合,监控是按
	// planCode 做的,通知和自动下单则是**按配置逐套触发**。于是
	// "自动抢 1 台"在三套配置同时补货时会下三次单(还要再乘以机房数) ——
	// 用户想要的往往是"只盯 64G + 2x480SSD 那套"。
	// 没有这个字段之前,他没有任何办法表达这件事。
	Options []string `json:"options,omitempty"`
}

// VPSSubscription VPS 监控订阅
type VPSSubscription struct {
	Duration           string                   `json:"duration,omitempty"` // 购买时长；空值兼容旧任务，按 1 个月下单。
	Revision           uint64                   `json:"-"`
	ID                 string                   `json:"id"`
	PlanCode           string                   `json:"planCode"`
	OvhSubsidiary      string                   `json:"ovhSubsidiary"`
	Datacenters        []string                 `json:"datacenters"`
	MonitorLinux       bool                     `json:"monitorLinux"`
	MonitorWindows     bool                     `json:"monitorWindows"`
	NotifyAvailable    bool                     `json:"notifyAvailable"`
	NotifyUnavailable  bool                     `json:"notifyUnavailable"`
	LastStatus         map[string]string        `json:"lastStatus"`
	History            []map[string]interface{} `json:"history"`
	CreatedAt          string                   `json:"createdAt"`
	AutoOrderAccountID string                   `json:"autoOrderAccountId,omitempty"` // 空 = 触发时只通知不下单
	// AutoOrder 有货时是否真的下单。和 AutoOrderAccountID 分开:
	// 只填账户不代表要下单,用户可能只是想让通知里带上"用哪个账户能买"。
	AutoOrder bool `json:"autoOrder,omitempty"`
	// Quantity 每次下单几台
	Quantity int `json:"quantity,omitempty"`
	// AutoPay 下单成功后用 OVH 默认支付方式自动付款(用户显式开关)
	AutoPay bool `json:"autoPay,omitempty"`
	// OS 装什么系统。空 = 从购物车允许的镜像中选择。
	// VPS 和独服不同:系统是下单时就要定的配置项,不是买完再装。
	OS string `json:"os,omitempty"`
}

// CacheInfo 服务器列表缓存信息
type CacheInfo struct {
	Cached             bool     `json:"cached"`
	UsingExpiredCache  bool     `json:"usingExpiredCache"`
	CacheAgeMinutes    int      `json:"cacheAgeMinutes"`
	Timestamp          *float64 `json:"timestamp"`
	CacheAge           *int     `json:"cacheAge"`
	CacheDuration      int      `json:"cacheDuration"`
	NextAutoRefresh    *float64 `json:"nextAutoRefresh"`
	AutoRefreshEnabled bool     `json:"autoRefreshEnabled"`
}

// NowISO 返回 ISO8601 时间（与 datetime.now().isoformat() 一致）
func NowISO() string {
	return time.Now().Format(NowISOLayout)
}

// NowISOLayout 是 NowISO 的布局。**注意它没有时区偏移** —— 这不是 RFC3339。
// 用 time.Parse(time.RFC3339, ...) 去解它一定失败,而失败通常被 `err == nil &&`
// 这类写法静默吞掉,于是整段逻辑变成永不生效的死代码。
// 实际踩到的:订单状态刷新的两处节流(2 分钟最小间隔 / 30 天上限)全废,
// 每轮都对所有未终态订单打 OVH;队列按创建时间排序也退化成了不排序。
// 解析自家时间戳一律走 ParseTS。
const NowISOLayout = "2006-01-02T15:04:05.000000"

// ParseTS 解析本项目自己写出来的时间戳。
//
// 历史上存过两种格式:NowISO(无时区,本地时间)和 time.RFC3339Nano,
// 库里两种都有,所以解析必须两种都认。第二个返回值为 false 表示确实解不出来,
// 调用方要显式决定"解不出来时怎么办",不要再写成静默跳过。
func ParseTS(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, NowISOLayout, "2006-01-02T15:04:05"} {
		// NowISO 没带时区,按本地时区解 —— 它本来就是 time.Now() 的本地时间
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}
