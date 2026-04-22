# OVH 服务器抢购与监控平台

一个面向 OVH 独立服务器抢购、库存监控、多账户管理和服务器运维控制的完整平台。

项目基于 `coolci/OVH-BUY` 的分支演进而来，当前版本重点增强了以下能力：
- 多 OVH 账户管理与切换
- 抢购队列与自动重试
- 服务器补货监控与自动下单
- VPS 补货通知
- Telegram 与飞书应用机器人双通道通知/交互
- OVH 服务器控制面板
- Docker 一键部署与 GitHub 自动构建镜像

## 功能概览

### 1. 抢购队列
- 添加服务器型号到抢购队列
- 支持多机房优先级选择
- 支持配置选项下单
- 支持数量控制与自动支付
- 支持暂停、恢复、编辑、清空队列
- 支持多账户隔离与全部账户视图

### 2. 服务器库存监控
- 监控指定服务器型号在多个机房的库存变化
- 支持配置级监控
- 支持库存变化通知
- 支持自动加入抢购队列
- 支持监控历史记录查看

### 3. VPS 补货通知
- 监控 OVH VPS 套餐库存
- 支持不同子公司与机房订阅
- 支持补货/下架通知

### 4. 多账户管理
- 保存多个 OVH API 账户
- 快速切换当前操作账户
- 账户连接状态检查
- 支持按账户分离抢购队列与历史记录

### 5. 消息通知与机器人交互
- Telegram 通知
- Telegram Webhook 按钮交互
- 飞书应用机器人私聊消息接收
- 飞书交互卡片下单
- 飞书用户绑定持久化
- 飞书与 Telegram 并存发送

### 6. 服务器控制面板
- 查看服务器列表与详情
- 查看任务、网络、监控、硬件、IP 等信息
- 支持重装系统、切换启动模式等常见操作

## 技术栈

### 前端
- React 18
- TypeScript
- Vite
- Tailwind CSS
- shadcn/ui
- Framer Motion
- Axios
- React Router
- TanStack Query

### 后端
- Python 3.11+
- Flask
- flask-cors
- python-dotenv
- requests
- ovh SDK
- tenacity
- SQLite（主持久化存储）

### 部署
- Docker
- Docker Compose
- Nginx
- GitHub Actions

## 项目结构

```text
.
├─ backend/                 # Flask 后端
│  ├─ app.py                # 主服务与大部分 API
│  ├─ api_auth_middleware.py
│  ├─ api_key_config.py
│  ├─ feishu_utils.py       # 飞书应用机器人能力
│  ├─ server_monitor.py     # 服务器监控逻辑
│  ├─ sqlite_storage.py     # SQLite 持久化与数据访问层
│  ├─ telegram_utils.py     # Telegram 工具
│  └─ requirements.txt
├─ src/                     # React 前端
│  ├─ components/
│  ├─ context/
│  ├─ pages/
│  ├─ utils/
│  └─ config/
├─ nginx/                   # Nginx 配置
├─ .github/workflows/       # GitHub Actions
├─ Dockerfile.hub           # GHCR / Docker 镜像构建文件
├─ docker-compose.yml       # Docker Compose 示例
└─ README.md
```

## 运行方式

## Docker Compose 部署

最推荐的运行方式是直接使用 Docker。

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

说明：
- `ghcr.io/planetsider/ovh:beta` 对应 GitHub Actions 在 `main` 分支自动构建的镜像
- 如果你发布了版本标签 `v*`，也可以使用对应版本标签或 `latest`
- 镜像内会自带一份默认 `.env` 模板，生产环境仍建议通过容器环境变量显式覆盖关键配置

## 本地开发

### 前端

```bash
npm install
npm run dev
```

默认端口：

```text
http://localhost:19999
```

### 后端

建议使用虚拟环境：

```bash
cd backend
python -m venv venv
venv\Scripts\activate
pip install -r requirements.txt
python app.py
```

默认后端端口：

```text
http://localhost:19998
```

## 关键环境变量

- `API_SECRET_KEY`
  - 后端 API 访问密钥
  - 前端设置页需要填写相同值才能访问 API

- `ENABLE_API_KEY_AUTH`
  - 是否启用 API Key 校验
  - 生产环境建议设置为 `true`

- `VITE_API_URL`
  - 前端请求后端 API 的基础地址
  - 容器部署推荐 `/api`

- `TZ`
  - 时区，默认 `Asia/Shanghai`

## 首次使用流程

### 1. 进入系统
- 打开页面后，先到“系统设置”填写 `API_SECRET_KEY`

### 2. 配置 OVH 账户
- 到“API账户管理”中添加 OVH API 凭据：
  - `APP KEY`
  - `APP SECRET`
  - `CONSUMER KEY`
  - `ENDPOINT`
  - `ZONE`

### 3. 验证服务器列表
- 到“服务器列表”页面拉取可售服务器

### 4. 创建抢购任务
- 到“抢购队列”添加任务

### 5. 配置通知通道（可选）
- Telegram Bot
- 飞书应用机器人

## Telegram 配置

系统支持 Telegram 通知和 Telegram Webhook 按钮交互。

在“系统设置”中可以配置：
- `Telegram Bot Token`
- `Telegram Chat ID`
- Telegram Webhook

支持的能力：
- 监控通知
- 下单结果通知
- Telegram 按钮直接加入抢购队列
- 文本命令快速下单

## 飞书应用机器人配置

系统支持飞书应用机器人私聊通知与交互卡片。

在“系统设置”中可以配置：
- `飞书 App ID`
- `飞书 App Secret`
- `Verification Token`
- `Encrypt Key`
- 是否启用飞书通道

### 飞书回调地址
- 事件回调：`/api/feishu/events`
- 卡片回调：`/api/feishu/card-action`

### 飞书当前支持
- 私聊机器人接收文本命令
- 私聊机器人自动绑定用户
- 飞书交互卡片多步下单
- 飞书测试交互卡片
- 补货通知转飞书交互卡片

### 使用说明
1. 在飞书开放平台创建应用机器人
2. 开启消息相关权限
3. 配置事件订阅与卡片回调地址
4. 发布应用版本
5. 用户先私聊机器人一次，系统会自动绑定

## GitHub 自动构建镜像

仓库已包含 GitHub Actions 镜像构建工作流：

- 工作流文件：`.github/workflows/dockerhub.yml`
- 构建文件：`Dockerfile.hub`
- 推送目标：`ghcr.io/planetsider/ovh`

### 触发条件
- 推送到 `main`
- 推送标签 `v*`
- 手动触发 `workflow_dispatch`

### 权限要求
- 工作流使用内置的 `GITHUB_TOKEN` 推送到 GitHub Container Registry
- 仓库 Actions 需要具备 `packages: write` 权限

### 默认镜像标签策略
- 推送到 `main`
  - `beta`
  - `sha-<commit>`
- 推送 `v*` 标签
  - 对应 tag
  - `latest`
  - `sha-<commit>`

### 配置步骤
1. 确认仓库启用了 GitHub Actions
2. 确认 Actions 对 Packages 有写入权限
3. 推送到 `main` 或创建版本标签
4. 在 `Actions` 页面查看构建结果
5. 在 GitHub Packages / GHCR 中查看镜像

## 数据存储说明

后端当前以 SQLite 作为主持久化存储，数据库默认位于：

- `data/app.db`

当前运行期主读写都已切换到 SQLite，包括：

- 系统配置
- 日志
- API 账户
- 抢购队列
- 抢购历史
- 服务器监控订阅
- 服务器列表与快照
- 飞书绑定、机器人账户选择、服务器别名、机器人待处理动作
- 配置绑定狙击任务
- VPS 订阅

旧 JSON 文件仅保留为历史迁移来源，默认路径包括：

- `backend/data/config.json`
- `backend/data/accounts.json`
- `backend/data/queue.json`
- `backend/data/history.json`
- `backend/data/subscriptions.json`
- `backend/data/vps_subscriptions.json`
- `backend/data/feishu_users.json`

运行容器时建议挂载：
- `data/`
- `logs/`
- `cache/`

## 当前验证状态

已完成的本地验证包括：

- `python -m py_compile backend/app.py backend/sqlite_storage.py`
- 安装 `backend/requirements.txt` 后执行后端初始化 smoke test
- 使用 Flask `test_client()` 验证核心 API 读写链路

已通过的接口覆盖：

- `/api/settings`
- `/api/queue`
- `/api/queue/paged`
- `/api/queue/all`
- `/api/purchase-history`
- `/api/config-sniper/tasks`
- `/api/vps-monitor/subscriptions`
- `/api/cache/info`

当前结论：SQLite 主存储方案已经能在本地真实初始化并通过核心 API 验证。

## 注意事项

- `backend/app.py` 是项目核心主文件，功能较多，修改前建议先备份或分支开发
- 飞书私聊通知依赖用户先私聊机器人建立绑定
- Telegram 与飞书目前可并存，不会互相替代
- 服务器列表首次拉取可能较慢，属于正常现象
- 多账户场景下，抢购队列、历史记录与通知行为要特别留意当前账户上下文

## 许可证

MIT License
