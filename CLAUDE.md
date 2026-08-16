# Agent 约定

## 工作流约定

### 任务完成即提交推送

每完成一个独立任务（功能开发、Bug 修复、重构等），**必须立即**执行 git commit 和 push，不要积攒多个任务后一次性提交。

流程：
1. 完成代码编写和测试
2. `git add -A`
3. `git commit -m "<type>: <简要描述>"`
4. `git push origin master`

提交信息规范：
- `feat:` 新功能
- `fix:` 修复 Bug
- `refactor:` 重构
- `docs:` 文档变更
- `chore:` 构建/工具变更

### 开发语言与目录

- 开发语言固定为 `go+reactjs`，无需用户填写
- 工作目录固定为 `/srv/zfei/projects/{slugified_name}`
- 数据库参数通过 `APP_ENV_VARS` 或 `JVM_OPTS_ENV` 环境变量注入

### 部署运行时契约

生成的后端必须以环境变量方式读取运行配置，部署时由 loafer 注入：

- `PORT`：后端监听端口（必填，nginx 按此端口反代；启动参数 `--port` 也会同时传入）
- `JWT_SECRET`：JWT 签名密钥（由 loafer 派生，重部署保持稳定）
- `APP_ENV_VARS`：项目独立数据库的 MySQL DSN（`user:pass@tcp(host:port)/loafer_proj_<id>?...`）
- `DB_PATH`：SQLite 类项目的库文件绝对路径（位于部署目录内）

后端进程日志写入部署目录 `backend.log`。

### 代码风格

- 后端: Go + Gin + GORM，遵循 Go 官方代码规范
- 前端: Vue 3 + TypeScript + Element Plus，使用 `<script setup>` 语法
- 提交前确保 `go build ./...` 和 `npm run build` 均通过

### 部署

- 部署脚本: `deploy-local.sh`，支持 `all`/`backend`/`frontend` 三种模式
- 端口范围: 40410-40500
- 前端通过 Nginx 转发到构建产物和后端端口
