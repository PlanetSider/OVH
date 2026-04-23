# OVH 多账户抢购、监控与服务器控制平台

这是一个基于 `coolci/OVH-BUY` 深度演进的 OVH 面板项目。

相较原始 fork，本项目已经不再只是“服务器补货提醒 + 简单抢购”的薄扩展，而是围绕以下目标做了系统级重构：

- 多 OVH API 账户长期运行
- 抢购、监控、通知按账户隔离
- 服务器列表刷新与业务任务解耦
- SQLite 统一持久化
- 独立登录页与访问密钥保护
- 已购服务器资产缓存与服务器控制面板
- Telegram / 飞书双通道通知与交互

## 项目定位

本项目面向以下场景：

- 多个 OVH 账户同时管理
- 对独立服务器和 VPS 进行补货监控
- 有货后自动加入抢购队列或自动下单
- 统一查看不同账户的任务状态
- 对已购服务器执行日常控制与维护操作

## 相对原 fork 的核心变化

下面这部分是本仓库最重要的内容：它说明了本项目相对 `coolci/OVH-BUY` 到底改了什么。

### 1. 从单账户偏向改为多账户优先架构

原始 fork 的很多能力默认以“当前唯一账户”展开，而本项目已经将主要业务拆成两类：

- 主刷新链路
- 按账户独立链路

当前规则如下：

- 服务器列表自动刷新：只由主刷新账户执行
- 实时可用性自动刷新：只由主刷新账户执行
- 抢购队列：按账户独立执行
- 服务器监控：按账户独立运行
- VPS 补货通知：按账户独立订阅、独立线程、独立启停
- 服务器控制页的已购服务器资产刷新：遍历所有账户定时刷新

这意味着：

- 可售服务器目录不再被每个账户重复拉取
- 真正与账户绑定的任务不会混在一起
- 当前页面选择的账户，只影响当前账户上下文，不会误伤主刷新链路

### 2. 持久化从 JSON 文件堆叠演进为 SQLite 主存储

原项目大量依赖 JSON 文件读写；本项目已经把运行期主数据读写切换到 SQLite。

当前 SQLite 主存储覆盖：

- 系统配置
- API 账户
- 抢购队列
- 抢购历史
- 服务器监控订阅
- VPS 订阅
- 服务器列表快照
- 实时可用性快照
- 已购服务器资产缓存
- 飞书绑定与机器人辅助状态
- 日志与部分业务元数据

旧 JSON 文件现在只作为历史迁移来源保留，不再是主读写路径。

### 3. 增加主刷新账户模型

这是相对 fork 的关键架构变化之一。

系统新增 `primaryRefreshAccountId`，用于明确谁负责：

- 可售服务器列表刷新
- 实时可用性刷新

这样做的目的：

- 避免多账户重复刷新同一份目录数据
- 降低无意义的 API 请求量
- 将“目录刷新”和“任务执行”从语义上分开

### 4. VPS 监控从全局实例改为按账户独立实例

原本更接近全局单实例模型，本项目已改为：

- 每个账户独立订阅列表
- 每个账户独立运行状态
- 每个账户独立启动与停止
- 每个账户独立历史与通知上下文

这使 VPS 补货通知和抢购队列、服务器监控在账户语义上保持一致。

### 5. 新增服务器控制面板与资产缓存链路

本项目不仅关注“买到之前”，也关注“买到之后”。

新增能力包括：

- 查看当前账户下已购服务器
- 切换 API 账户查看不同账户资产
- 查看服务器详情、任务、网络、监控、硬件等信息
- 执行常见控制动作
- 对已购服务器资产进行后台定时刷新缓存

这部分能力在原 fork 中并不是核心结构。

### 6. 增加独立登录页与 API 密钥访问保护

本项目引入了真正的前端访问门禁，而不是把访问控制散落在设置页或提示弹层里。

当前登录链路包括：

- 独立 `/login` 页面
- `PasswordGate` 路由守卫
- `/api/auth/check` 校验接口
- 退出登录能力
- 本地访问密钥缓存清理

这让面板访问控制更接近可部署产品，而不是开发期工具页。

### 7. 账户标签、别名与展示语义做了系统整理

本项目不再只把账户 ID 当作唯一展示信息。

当前账户展示分为两套语义：

- 用户可读场景：优先 `alias`
- 管理 / 日志 / 调试场景：`alias | email | id`

同时，服务器别名策略也从“写入缓存事实数据”改为“读取时动态叠加展示层别名”，避免缓存和展示元数据耦合。

### 8. 通知通道从单一消息推送扩展为双通道交互

本项目保留并增强了 Telegram，并新增或强化了飞书应用机器人链路。

支持：

- Telegram 通知
- Telegram Webhook 按钮交互
- 飞书应用机器人私聊消息
- 飞书交互卡片
- 飞书用户绑定持久化
- 双通道并存发送

## 当前主要能力

### 抢购队列

- 创建抢购任务
- 选择多个机房并保留优先顺序
- 配置选项下单
- 数量控制
- 自动支付
- 暂停、恢复、编辑、清空
- 当前账户 / 全部账户视图
- 仪表盘摘要展示

### 服务器监控

- 监控指定服务器型号库存变化
- 监控指定机房或所有机房
- 有货提醒 / 无货提醒
- 可选自动下单
- 监控历史记录
- 当前账户 / 全部账户视图

### VPS 补货通知

- 监控 VPS 型号库存
- 支持不同 OVH 子公司
- 支持指定数据中心或所有数据中心
- 有货提醒 / 无货提醒
- 按账户独立订阅与独立运行
- 当前账户 / 全部账户视图

### 多账户管理

- 保存多个 OVH API 账户
- 切换当前操作账户
- 查看账户状态
- 设置主刷新账户
- 在仪表盘、监控页、队列页显示账户归属

### 服务器控制

- 查看已购服务器
- 按账户切换服务器视图
- 查看任务、网络、监控、硬件、IP 等信息
- 执行常见服务器控制操作
- 已购服务器资产后台自动刷新

### 访问控制与通知

- 独立登录页
- API 访问密钥保护
- Telegram 状态显示
- 飞书绑定状态显示
- Telegram / 飞书双通道通知

## 架构说明

### 前端

技术栈：

- React 18
- TypeScript
- Vite
- Tailwind CSS
- shadcn/ui
- Framer Motion
- Axios
- React Router

前端当前主要分层：

- `src/pages/`
  - 页面级业务，例如仪表盘、抢购队列、服务器监控、VPS 监控、服务器控制、系统设置
- `src/components/`
  - 布局、侧边栏、登录守卫、局部复用 UI
- `src/context/APIContext.tsx`
  - 当前账户、账户列表、认证状态、系统设置上下文
- `src/utils/apiClient.ts`
  - API 请求封装，携带访问密钥和当前账户上下文

### 后端

技术栈：

- Python 3.11+
- Flask
- requests
- ovh SDK
- tenacity
- SQLite

后端当前主要结构：

- `backend/app.py`
  - 主服务、主要 API、业务调度入口
- `backend/sqlite_storage.py`
  - SQLite 持久化层
- `backend/server_monitor.py`
  - 服务器库存监控逻辑
- `backend/api_auth_middleware.py`
  - API 密钥访问控制
- `backend/feishu_utils.py`
  - 飞书应用机器人能力

### 缓存与数据边界

当前大致分为三类数据：

- 目录级缓存
  - 服务器列表
  - 实时可用性快照
  - 由主刷新账户负责刷新

- 账户级业务数据
  - 抢购队列
  - 服务器监控订阅
  - VPS 订阅
  - 抢购历史
  - 按账户隔离

- 资产级缓存
  - 已购服务器资产
  - 为服务器控制页服务
  - 定期遍历所有账户刷新

## 项目结构

```text
.
├─ backend/
│  ├─ app.py
│  ├─ api_auth_middleware.py
│  ├─ api_key_config.py
│  ├─ feishu_utils.py
│  ├─ server_monitor.py
│  ├─ sqlite_storage.py
│  ├─ telegram_utils.py
│  └─ requirements.txt
├─ src/
│  ├─ components/
│  ├─ context/
│  ├─ pages/
│  ├─ utils/
│  └─ config/
├─ nginx/
├─ .github/workflows/
├─ Dockerfile.hub
├─ docker-compose.yml
└─ README.md
```

## 部署

### Docker Compose

推荐直接使用 Docker 部署。

示例：

```yaml
version: "3.8"
services:
  ovh-app:
    image: ghcr.io/planetsider/ovh:beta
    container_name: OVH
    pull_policy: always
    ports:
      - "20000:80"
    environment:
      - DEBUG=false
      - VITE_API_URL=/api
      - API_SECRET_KEY=请替换为你自己的访问密钥
      - ENABLE_API_KEY_AUTH=true
    volumes:
      - ./data:/app/backend/data
      - ./logs:/app/backend/logs
      - ./cache:/app/backend/cache
    restart: unless-stopped
```

启动：

```bash
docker compose up -d
```

访问：

```text
http://你的服务器IP:20000
```

### 本地开发

前端：

```bash
npm install
npm run dev
```

后端：

```bash
cd backend
python -m venv venv
venv\Scripts\activate
pip install -r requirements.txt
python app.py
```

## 关键环境变量

- `API_SECRET_KEY`
  - 面板访问密钥
  - 前端登录页和 API 调用都会依赖它

- `ENABLE_API_KEY_AUTH`
  - 是否启用 API 密钥校验
  - 生产环境建议开启

- `VITE_API_URL`
  - 前端请求后端 API 的基础地址
  - 容器部署推荐 `/api`

- `TZ`
  - 时区，默认可设为 `Asia/Shanghai`

## 首次使用建议流程

1. 以 Docker 或本地方式启动项目
2. 打开页面，先用 `API_SECRET_KEY` 登录面板
3. 进入 `API账户管理` 添加一个或多个 OVH API 账户
4. 在 `系统设置` 中设置主刷新账户
5. 检查 `服务器列表` 与 `实时可用性` 是否能正常加载
6. 在 `抢购队列`、`服务器监控`、`VPS补货通知` 中创建任务
7. 如果需要通知，配置 Telegram 或飞书
8. 如果需要管理已购服务器，进入 `服务器控制`

## Telegram 与飞书

### Telegram

支持：

- 普通消息通知
- Webhook 按钮交互
- 从通知直接加入抢购队列

系统设置中可配置：

- `Telegram Bot Token`
- `Telegram Chat ID`
- Telegram Webhook

### 飞书

支持：

- 飞书应用机器人私聊消息
- 飞书用户绑定
- 飞书交互卡片
- 多步交互下单

系统设置中可配置：

- `飞书 App ID`
- `飞书 App Secret`
- `Verification Token`
- `Encrypt Key`
- 是否启用飞书通道

回调地址：

- `/api/feishu/events`
- `/api/feishu/card-action`

## 机器人命令

项目当前支持在 Telegram 和飞书私聊中使用一组统一的命令。

这些命令主要面向：

- 查看当前账户状态
- 查看多账户概览
- 查看监控和抢购任务
- 切换机器人当前使用的 API 账户
- 为服务器绑定或移除别名
- 通过文本命令直接下单

### 基础命令

- `/help`
  - 查看所有可用命令说明

- `/status`
  - 查看当前 API 账户概览
  - 包括账户、服务器、监控和抢购摘要

- `/status-all`
  - 查看所有 API 账户概览

- `/monitor`
  - 查看当前 API 账户下的服务器监控任务

- `/monitor-all`
  - 查看所有 API 账户下的服务器监控任务

- `/autobuy`
  - 查看当前 API 账户下的抢购任务

- `/autobuy-all`
  - 查看所有 API 账户下的抢购任务

### 账户切换命令

- `/switch`
  - 按顺序循环切换机器人当前使用的 API 账户

- `/switch <邮箱>`
  - 切换到指定邮箱对应的 API 账户

说明：

- 机器人命令有自己的当前账户上下文
- 它和网页端当前选中的账户不是同一个状态源
- 因此 `/switch` 只影响机器人会话中的当前账户

### 服务器别名命令

- `/alias`
  - 为服务器绑定别名
  - 会先让你选择账户，再选择服务器并输入别名

- `/unalias`
  - 移除服务器别名

- `/alias-list`
  - 查看当前已绑定的所有服务器别名

### 服务器操作命令

- `/reboot <服务器自定义名称>`
  - 在所有 API 账户下搜索匹配服务器
  - 找到后进入重启确认流程

示例：

```text
/reboot Web-01
```

### 文本下单命令

除了斜杠命令之外，机器人还支持直接发送简化下单文本。

支持格式：

```text
<planCode>
<planCode> <datacenter>
<planCode> <quantity>
<planCode> <datacenter> <quantity>
```

示例：

```text
24sk202
24sk202 rbx
24sk202 1
24sk202 rbx 1
```

含义：

- 只写 `planCode`
  - 使用当前账户尝试下单，机房由系统按默认逻辑处理

- `planCode + datacenter`
  - 指定机房下单

- `planCode + quantity`
  - 指定数量下单

- `planCode + datacenter + quantity`
  - 指定机房和数量下单

说明：

- 这些文本命令会走当前机器人账户上下文
- 如果格式错误，机器人会返回帮助信息
- 如果需要先查看当前机器人账户，可以先执行 `/status`

## 数据存储

当前 SQLite 数据库默认位于：

- `data/app.db`

部署时建议挂载以下目录：

- `data/`
- `logs/`
- `cache/`

## 适合维护者注意的点

- `backend/app.py` 仍然是当前主要聚合文件，历史原因导致职责较多
- 当前系统已经明显偏向多账户架构，新增功能时需要先判断它属于：
  - 主刷新链路
  - 当前账户链路
  - 全账户遍历链路
- 不要把展示层元数据和事实缓存混写
- 与账户相关的接口，优先确认是否需要支持：
  - 当前账户视图
  - 全部账户视图

## 许可证

MIT License
