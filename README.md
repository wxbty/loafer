# Loafer Agent

全自动项目开发平台，基于 Go + Vue 3。通过 Claude Code CLI 拆解复杂项目为模块化任务，自动执行 TDD 流水线（需求→测试→实现→运行→重构），实现从需求到部署的全链路自动化。

## 架构概览

```
┌──────────────┐    ┌──────────────────────────────────────────┐
│   Frontend   │───▶│              Backend (Go)                │
│  Vue 3 + Vite│    │  Gin + GORM + MySQL                      │
│  xterm.js    │◀───│  WebSocket Terminal + SSE Pipeline        │
└──────────────┘    │                    │                      │
                    │     ┌──────────────┼──────────────┐      │
                    │     ▼              ▼              ▼      │
                    │  Claude CLI    Deploy Service  Playwright│
                    │  (PTY/Pool)    (SSH/Nginx)     (E2E Test)│
                    └──────────────────────────────────────────┘
```

## 技术栈

- **后端**: Go 1.21+ / Gin / GORM / MySQL 8.0+
- **前端**: Vue 3 / TypeScript / Vite 5 / Tailwind CSS / xterm.js
- **CLI 引擎**: Claude Code CLI (PTY 进程管理 + 会话池)
- **部署**: Nginx 反向代理 / SSH 远程部署
- **测试**: Playwright E2E / Go 单元测试

## 核心功能

1. **CC终端**: Claude Code CLI 进程管理（PTY）、会话池、WebSocket 实时终端
2. **流水线引擎**: 计划生成 → 模块分解 → 编码实现 → 部署 → 测试验证，支持后台执行与断线重连
3. **TDD 流水线**: 5 阶段断言驱动（需求→测试→实现→运行→重构）
4. **项目管理**: 项目 CRUD、工作目录管理、Git 操作、Gitee 仓库自动创建
5. **部署服务**: SSH 远程构建、Nginx 配置、端口自动分配、进程管理
6. **测试自动化**: Playwright 集成测试、API 脚本生成与执行、截图验证

## 项目结构

```
loafer/
├── backend-go/               # Go 后端
│   ├── cmd/server/main.go    # 入口
│   ├── internal/
│   │   ├── config/           # 配置加载（环境变量）
│   │   ├── db/               # 数据库初始化（GORM AutoMigrate）
│   │   ├── model/            # 数据模型（14+ 表）
│   │   ├── handler/          # HTTP 处理器
│   │   ├── engine/           # 核心引擎
│   │   │   ├── cli/          # Claude CLI 进程管理
│   │   │   ├── executor/     # 任务/测试执行器
│   │   │   └── plan/         # 计划生成器
│   │   ├── service/          # 业务服务（部署/SSH/Nginx/Gitee）
│   │   ├── middleware/        # 认证中间件
│   │   └── router/           # 路由注册
│   └── go.mod
├── frontend/                 # Vue 3 前端
│   ├── src/
│   │   ├── views/            # 页面组件
│   │   ├── components/       # UI 组件
│   │   ├── api/              # API 调用
│   │   └── utils/            # 工具函数（SSE/WebSocket）
│   └── package.json
├── setup-server.sh           # 服务器初始化脚本
├── deploy-local.sh           # 本地部署脚本（构建+重启）
├── deploy_frontend.sh        # 前端部署脚本
└── nginx-local.conf          # Nginx 配置参考
```

## 快速开始（开发环境）

### 前置条件

- Go 1.21+
- MySQL 8.0+
- Node.js 18+
- Claude Code CLI（`npm install -g @anthropic-ai/claude-code`）

### 1. 数据库准备

```bash
# 创建数据库（GORM 启动时自动建表，无需手动执行 SQL）
mysql -u root -p -e "CREATE DATABASE IF NOT EXISTS loafer CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;"
```

### 2. 配置环境变量

```bash
export DB_HOST=127.0.0.1
export DB_PORT=3306
export DB_NAME=loafer
export DB_USERNAME=root
export DB_PASSWORD=your_password
export SERVER_PORT=9080
export JWT_SECRET=your_jwt_secret_at_least_32_chars
export APP_AUTH_USERNAME=admin
export APP_AUTH_PASSWORD=your_password
```

### 3. 启动服务

```bash
# 后端
cd backend-go && go run ./cmd/server/

# 前端（另一个终端）
cd frontend && npm install && npm run dev
```

### 4. 访问

- 前端: http://localhost:5173
- 后端 API: http://localhost:9080/api/

## 生产环境部署

### 1. 服务器初始化

在目标服务器上执行 `setup-server.sh`，自动完成以下操作：

```bash
# 通过环境变量传入数据库密码（必须）
MYSQL_PASS=your_mysql_password ssh root@your_server 'bash -s' < setup-server.sh
```

脚本完成 7 步初始化：
1. 创建 `loafer` 数据库
2. 安装/检查 Nginx
3. 配置防火墙（端口 40410-40500, 9080）
4. 创建部署目录（`/opt/loafer/`）
5. 安装 Node.js 20.x + Playwright
6. 安装 Go 1.24
7. 检查 Claude Code CLI

### 2. 上传代码

```bash
# 在服务器上 clone 代码
git clone https://github.com/wxbty/loafer.git /srv/loafer
cd /srv/loafer
```

### 3. 部署后端

```bash
# 设置应用环境变量
export APP_ENV_VARS="DB_HOST=127.0.0.1 DB_PORT=3306 DB_USERNAME=root DB_PASSWORD=your_password JWT_SECRET=your_jwt_secret APP_AUTH_PASSWORD=your_password"

# 构建并启动（拉代码 + Go 编译 + 前端构建 + 重启）
./deploy-local.sh all

# 或仅后端
./deploy-local.sh backend

# 或仅前端
./deploy-local.sh frontend
```

### 4. 服务管理

```bash
./deploy-local.sh start     # 启动
./deploy-local.sh stop      # 停止
./deploy-local.sh restart   # 重启
./deploy-local.sh status    # 查看状态
```

### 5. Nginx 配置

参考 `nginx-local.conf`，或使用部署脚本自动生成：

```bash
DEPLOY_INIT_NGINX=true \
FRONTEND_PORT=9081 \
BACKEND_PORT=9080 \
NGINX_CONF_PATH=/etc/nginx/conf.d/loafer.conf \
./deploy-local.sh all
```

Nginx 配置要点：
- 前端静态资源：`/` → `/opt/loafer/frontend-dist/`
- API 代理：`/api/` → `http://127.0.0.1:9080/api/`（SSE 需关闭 buffering）
- WebSocket 代理：`/ws` → `http://127.0.0.1:9080`（需 upgrade 头）

## 环境变量参考

### 必须配置

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `DB_HOST` | MySQL 主机地址 | `127.0.0.1` |
| `DB_PORT` | MySQL 端口 | `3306` |
| `DB_NAME` | 数据库名 | `loafer` |
| `DB_USERNAME` | 数据库用户名 | `root` |
| `DB_PASSWORD` | 数据库密码 | **无（必须设置）** |
| `JWT_SECRET` | JWT 签名密钥（≥32 字符） | **无（必须设置）** |
| `APP_AUTH_PASSWORD` | 管理员登录密码 | **无（必须设置）** |

### 服务配置

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `SERVER_PORT` | 后端监听端口 | `9080` |
| `APP_AUTH_USERNAME` | 管理员用户名 | `admin` |
| `JWT_EXPIRATION` | JWT 过期时间（毫秒） | `315360000000`（10年） |
| `SESSION_POOL_MAX_SIZE` | Claude CLI 会话池大小 | `5` |
| `SESSION_POOL_IDLE_TIMEOUT` | 会话空闲超时（分钟） | `40` |

### 基础设施配置

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `INFRA_SERVER_HOST` | 部署服务器 IP | **无（部署功能需要）** |
| `INFRA_SSH_USER` | SSH 用户名 | `root` |
| `INFRA_SSH_KEY_PATH` | SSH 私钥文件路径 | **无（与 PEM 二选一）** |
| `INFRA_SSH_PEM` | SSH 私钥内容（直接传入） | **无（与路径二选一）** |
| `INFRA_PORT_RANGE_START` | 项目端口范围起始 | `40410` |
| `INFRA_PORT_RANGE_END` | 项目端口范围结束 | `40500` |
| `INFRA_DEPLOY_BASE_DIR` | 部署目录 | `/opt/loafer/projects` |
| `INFRA_PROJECT_BASE_DIR` | 工作目录 | `/opt/loafer/workspace` |

### Gitee 集成

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `GITEE_ACCESS_TOKEN` | Gitee 个人访问令牌 | **无** |
| `GITEE_API_BASE_URL` | Gitee API 地址 | `https://gitee.com/api/v5` |

### 数据库连接池

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `DB_MAX_OPEN_CONNS` | 最大连接数 | `20` |
| `DB_MAX_IDLE_CONNS` | 最大空闲连接 | `5` |
| `DB_CONN_MAX_LIFETIME` | 连接最大生命周期（分钟） | `30` |
| `DB_CONN_MAX_IDLE_TIME` | 连接最大空闲时间（分钟） | `5` |

### Playwright 测试

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `PLAYWRIGHT_BINARY` | Playwright 可执行路径 | `npx` |
| `PLAYWRIGHT_HEADLESS` | 无头模式 | `true` |
| `PLAYWRIGHT_TIMEOUT` | 超时秒数 | `120` |
| `PLAYWRIGHT_BASE_URL` | 测试目标 URL | **无** |

## 数据库

项目使用 GORM AutoMigrate 自动建表，启动后端时自动创建/更新表结构，无需手动执行 SQL 脚本。

主要数据表：

| 表名 | 说明 |
|------|------|
| `project` | 项目 |
| `module` | 模块（基础架构/业务） |
| `task` | 任务 |
| `slice_history` | 分片执行历史 |
| `execution_plan` | 执行计划 |
| `deployment` | 部署记录 |
| `project_database` | 项目数据库供给记录 |
| `system_config` | 系统配置 |

每个项目还会自动创建独立数据库 `loafer_proj_<id>`，由 `DatabaseProvisioner` 管理。

## License

MIT
