# OVH 控制台（PlanetSider fork）

[gokele/ovh](https://github.com/gokele/ovh) 的个人 fork。上游的全部功能都保留，本仓库额外加入了**人民币参考价**和**飞书机器人**两套能力。

- 上游原始中文文档（550 行）已被这份 fork 说明替换；英文文档 [README.en.md](README.en.md) 仍是上游原文，内容对应上游版本。
- 许可：AGPL-3.0，见 [LICENSE](LICENSE)。

## 这是什么

OVHcloud 独立服务器 / VPS / Eco 系列的**抢购 + 补货监控 + 生命周期管理**控制台：

- **抢购队列**：planCode × 机房 × 数量一个任务，支持暂停/恢复/编辑、优惠码、预上架预订、fail-fast、失败重试间隔
- **补货监控**：服务器与 VPS，按机房/配置/数量/时长过滤，可自动入队下单
- **已购服务器管理**：30+ 生命周期操作（重启、重装、rescue、BIOS、启动模式、网络、反代、DDoS、MRTG、OL A、IPMI、合同期……）
- **多 OVH 账户**：EU / US / CA 三区，账户维度隔离目录与下单，凭据 AES-256-GCM 加密落库
- **通知通道**：Telegram、Webhook、飞书
- **Telegram 机器人**：长轮询、文本下单、一键下单按钮（一次性 nonce + update_id 幂等）
- **在线自更新**：SHA256 校验 + 失败回滚（容器内自动禁用）

技术栈：后端 Go 1.26 + Gin + SQLite（cgo / purego 双驱动），前端 Vite 7 + React 18 + TypeScript + TanStack Router/Query + Tailwind。

## 本 fork 新增

### 1. 人民币参考价

在原来的外币金额后面追加一个 `≈¥xxx` 的参考金额，**不改变结算币种**：

```
€42.99/月 · ≈¥300/月
42.99 GBP/月 · ≈¥390/月（按区域估算）
```

- 汇率来源 `api.exchangerate.fun`（USD 基准），失败自动回退 `open.er-api.com`；也可用 `EXCHANGE_API_KEY` 走 exchangerate-api 商业接口
- 每日本地 0 点刷新（`EXCHANGE_TZ`，默认 `Asia/Shanghai`）；超过 48 小时未更新视为不可用，此时只显示原币
- 三档币种来源：OVH 询价/目录明确返回的币种（不加标注）→ 按 OVH 子公司推断（标注「按区域估算」）→ 完全未知（不显示人民币）
- 设置 → 价格显示：`原币 + 人民币参考价` / `只显示原币`；关闭后前端不再请求汇率，通知里也不带人民币
- 覆盖位置：服务器列表与详情、抢购队列弹窗、购买历史、合同价格、VPS 页面、补货通知文案

> 人民币只是参考价。OVH 始终按账户币种结算，发卡行还可能另收货币转换费。

### 2. 飞书机器人

与 Telegram 机器人能力对齐，但走**长连接**，不需要公网 IP 或反向代理：

- **扫码创建应用**：设置 → 飞书机器人 → 扫码，用飞书扫码完成应用创建，App ID / Secret 自动回填（PersonalAgent 设备码流程）；也可以手动填写自建应用的凭据
- **仅私聊（DM）**：群里不响应，避免把操作权暴露给无关成员
- **绑定接收人**：网页生成 6 位绑定码，私聊机器人发 `/bind <码>`；解绑只能从网页操作
- **命令**：`/help`、`/status`、`/queue`、`/cancel`、`/interval`、`/watch`、`/unwatch`、`/accounts`、`/subs`、`/recent`，语义与 Telegram 版一致
- **文本下单**：与 Telegram 相同的格式，例如 `24sk602 gra 2 ram-64g,softraid-2x480ssd`
- **补货告警卡片**：应用内私聊发卡片，带「立即下单」按钮；按钮与 Telegram 共用同一个一次性 nonce，跨通道只会成功一次
- **幂等**：`feishu_events` 表按 event_id 去重，飞书事件重试不会重复执行

### 3. 其他改动

- 通知通道从 2 个扩到 3 个（Telegram / Webhook / 飞书），`/api/notify/test` 与设置页同步显示
- 数据库新增 `feishu_events` 表，启动时自动迁移，对老库安全
- 汇率状态单独存 kv（`exchange:rates`），不挤占设置项，避免后台刷新和保存设置互相覆盖

## 部署

### 方式一：Docker Compose（推荐）

镜像由本仓库的 GitHub Actions 自动构建并推到 GHCR：`ghcr.io/planetsider/ovh-console:latest`。

```bash
git clone https://github.com/PlanetSider/OVH.git
cd OVH
cp .env.example .env
# 编辑 .env，至少把 API_SECRET_KEY 换成一串随机值
#   生成：openssl rand -hex 32

docker compose up -d
```

访问 `http://127.0.0.1:20000`（默认只绑回环，远程访问请放在反向代理后面）。

```bash
docker compose logs -f          # 日志
docker compose pull && docker compose up -d   # 更新
docker compose down             # 停止（数据在 ./data，不会丢）
```

> 镜像包首次推送后默认是 private。到 GitHub → 个人/组织 → Packages → `ovh` → Package settings 里把可见性改成 Public，否则 `docker compose pull` 需要先 `docker login ghcr.io`。

### 方式二：源码运行

依赖：Go 1.26+、Node.js 20+。

```bash
# 1. 前端构建（产物直接写入 server/web，后端用 -tags ui 把它 embed 进二进制）
cd web
npm ci
npm run build

# 2. 后端构建
cd ../server
go build -tags ui -o ovh-console .

# 3. 运行（Linux/macOS 示例；Windows 用 ovh-console.exe）
./ovh-console
```

默认监听 `http://127.0.0.1:19998`，首次访问用 `API_SECRET_KEY` 登录（未设置时默认 `123456`，仅限本机调试）。

不带 `-tags ui` 构建的是纯 API 二进制，前端需要单独用 `npm run dev` 起 Vite（默认 `19997`）。

### 环境变量

| 变量 | 必填 | 说明 |
| --- | --- | --- |
| `API_SECRET_KEY` | 是 | 访问密码。这扇门后面能用 OVH 账户下单、重装服务器 |
| `OVH_DB_KEY` | 否 | 数据库加密密钥。留空会在首次启动时生成并写入 `data/.env`；**丢了它，已加密的凭据再也解不开** |
| `PORT` | 否 | 容器内监听端口，默认 `20000`（源码模式默认 `19998`） |
| `DATA_DIR` | 否 | 数据目录，容器内 `/data` |
| `BIND_ADDR` | 否 | compose 的宿主机绑定地址，默认 `127.0.0.1` |
| `HOST_PORT` | 否 | compose 的宿主机端口，默认 `20000` |
| `TZ` | 否 | 容器时区，默认 `Asia/Shanghai` |
| `PUID` / `PGID` | 否 | 容器内文件属主，默认 `10001` |
| `CORS_ALLOWED_ORIGINS` | 否 | 额外的 CORS 来源（放在反代后面时用） |
| `TRUSTED_PROXIES` | 否 | 信任的反代网段，例如 `172.16.0.0/12` |
| `EXCHANGE_TZ` | 否 | 汇率每日刷新时区，默认 `Asia/Shanghai` |
| `EXCHANGE_API_KEY` | 否 | 走 exchangerate-api 商业接口的 Key |
| `EXCHANGE_BASE_URL` | 否 | 自定义汇率镜像地址（优先级最高） |
| `EXCHANGE_HTTP_PROXY` | 否 | 汇率请求专用的 HTTP 代理；不设则直连，**不会**复用 OVH 的代理配置 |

飞书的 App ID / Secret / 域名 / 绑定人都在网页设置里配置，不走环境变量。

## 飞书机器人配置

扫码方式会自动完成第 1–2 步；手动方式需要在 [open.feishu.cn](https://open.feishu.cn) 创建企业自建应用。

1. **创建应用 / 扫码**：设置 → 飞书机器人 → 扫码创建，或手动填 App ID 与 App Secret
2. **开启机器人能力**：应用功能 → 机器人
3. **申请权限**：
   - `im:message.p2p_msg:readonly`（接收私聊消息）
   - `im:message:send_as_bot`（以机器人身份发消息）
4. **事件订阅**：选择「使用长连接接收事件」，订阅
   - `im.message.receive_v1`
   - `card.action.trigger`（卡片按钮回调）
5. **创建版本并发布**：权限和事件变更都要发布新版本才生效
6. **绑定**：网页设置里生成绑定码 → 飞书私聊机器人发 `/bind 123456`

> 长连接由服务端主动连飞书，因此不需要公网 IP、域名或 HTTPS 证书；但应用必须已发布，否则收不到事件。

## 镜像与 CI

- `.github/workflows/docker.yml`：push 到 `main` 构建 `latest` / `main` / `sha-<短SHA>`；push `v*` tag 额外打语义版本号；也支持手动触发
- `.github/workflows/ci.yml`：纯手动触发的检查流水线（gofmt / vet / test / 前端 tsc + build / 联网契约测试），不产生产物
- 构建为 `linux/amd64` + `linux/arm64` 双架构，缓存走 GitHub Actions cache

## 数据与备份

所有状态都在 `DATA_DIR`（默认 `/data`）一个目录里：SQLite 数据库、数据库加密密钥（`data/.env`）、目录缓存、日志。

备份就备份这个目录。删掉它等于已保存的 OVH 凭据、Telegram Token、飞书 Secret 全部作废。

## 与上游同步 / 升级

- 基线版本：`VERSION` 文件（当前 `0.4.16`）
- 数据库结构通过 `schema.sql` 的 `IF NOT EXISTS` + 增量列迁移向前兼容，升级不需要手工改库
- 想回到纯上游：`git remote add upstream https://github.com/gokele/ovh && git fetch upstream`，或直接部署上游镜像 `ghcr.io/gokele/ovh:latest`

## 许可

AGPL-3.0。上游版权归 [gokele/ovh](https://github.com/gokele/ovh) 作者所有；本 fork 的修改同样以 AGPL-3.0 发布。
