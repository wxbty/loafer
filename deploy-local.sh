#!/bin/bash
# =============================================================================
# 服务器本地部署：在服务器上直接拉取代码并构建，然后重启应用
# 用法: ./deploy-local.sh [backend|bd|frontend|ft|all|start|stop|restart|status]
# =============================================================================
#
# 环境变量注入（通过 APP_ENV_VARS 传入，格式: KEY1=VALUE1 KEY2=VALUE2）:
#   APP_ENV_VARS="DB_HOST=localhost DB_PORT=3306" ./deploy-local.sh all
#   脚本会自动导出为环境变量并传给 Go 二进制
#
# =============================================================================

set -e

PROJECT_DIR="${DEPLOY_PROJECT_DIR:-/srv/zfei/projects/loafer}"

# 加载项目级默认端口配置（由端口申请自动生成，命令行传入可覆盖）
if [ -f "${PROJECT_DIR}/.claude/deploy.env" ]; then
  source "${PROJECT_DIR}/.claude/deploy.env"
fi

FRONTEND_DIR="${DEPLOY_FRONTEND_DIR:-/opt/loafer/frontend-dist}"
GO_PATH="${DEPLOY_GO:-}"
APP_BIN="${DEPLOY_APP_BIN:-loafer-agent}"
APP_NAME="${DEPLOY_APP_NAME:-loafer-agent}"
SERVER_PORT="${DEPLOY_SERVER_PORT:-9080}"
DEPLOY_VERBOSE="${DEPLOY_VERBOSE:-0}"

# 应用环境变量参数（由部署按钮传入，格式: KEY1=VALUE1 KEY2=VALUE2）
# 兼容旧 Java 时代的 JVM_OPTS_ENV 变量名
if [ -n "${JVM_OPTS_ENV:-}" ] && [ -z "${APP_ENV_VARS:-}" ]; then
  APP_ENV_VARS="${JVM_OPTS_ENV}"
fi
APP_ENV_VARS="${APP_ENV_VARS:-}"

# 项目端口分配（由部署按钮传入）
# FRONTEND_PORT: 前端端口，用于 Nginx 配置
# BACKEND_PORT: 后端端口，用于后端应用监听
FRONTEND_PORT="${FRONTEND_PORT:-}"
BACKEND_PORT="${BACKEND_PORT:-}"
NGINX_CONF_PATH="${NGINX_CONF_PATH:-}"

# 实际使用的后端端口：优先使用 BACKEND_PORT，否则使用 SERVER_PORT
EFFECTIVE_BACKEND_PORT="${BACKEND_PORT:-${SERVER_PORT}}"

# 前端 API 地址配置（部署时注入到 .env.production）
FRONTEND_API_URL="${DEPLOY_FRONTEND_API_URL:-}"
FRONTEND_WS_URL="${DEPLOY_FRONTEND_WS_URL:-}"

# 自动探测 Node.js/npm 路径
NODE_PATH="${DEPLOY_NODE:-}"
if [ -z "$NODE_PATH" ]; then
  for p in /usr/local/node18/bin /usr/local/node/bin /usr/bin; do
    if [ -x "$p/node" ]; then
      NODE_PATH="$p"
      break
    fi
  done
fi
if [ -n "$NODE_PATH" ]; then
  export PATH="$NODE_PATH:$PATH"
fi

ACTION=""
MODE="${1:-all}"
case "$MODE" in
  backend|bd) MODE=backend ;;
  frontend|ft) MODE=frontend ;;
  all) MODE=all ;;
  start) ACTION=start; MODE=service ;;
  stop) ACTION=stop; MODE=service ;;
  restart) ACTION=restart; MODE=service ;;
  status) ACTION=status; MODE=service ;;
  -h|--help)
    echo "用法: $0 [backend|bd|frontend|ft|all|start|stop|restart|status]"
    echo ""
    echo "部署模式："
    echo "  backend/bd   - 仅拉代码并构建后端（Go），重启应用"
    echo "  frontend/ft  - 拉代码并构建前端，仅更新 ${FRONTEND_DIR}"
    echo "  all          - 拉代码并构建前后端，重启应用（默认）"
    echo ""
    echo "服务管理："
    echo "  start        - 启动应用（不拉代码不构建）"
    echo "  stop         - 停止应用"
    echo "  restart      - 重启应用（不拉代码不构建）"
    echo "  status       - 查看应用状态"
    echo ""
    echo "可用环境变量参数（可选）："
    echo "  DEPLOY_PROJECT_DIR   项目目录（默认: ${PROJECT_DIR}）"
    echo "  DEPLOY_FRONTEND_DIR  前端发布目录（默认: ${FRONTEND_DIR}）"
    echo "  DEPLOY_GO            Go 二进制路径（默认自动探测）"
    echo "  DEPLOY_NODE          Node.js 路径（默认自动探测）"
    echo "  DEPLOY_APP_BIN       Go 二进制文件名（默认: ${APP_BIN}）"
    echo "  DEPLOY_APP_NAME      进程匹配名（默认: ${APP_NAME}）"
    echo "  DEPLOY_SERVER_PORT   应用端口（默认: ${SERVER_PORT}）"
    echo "  DEPLOY_FRONTEND_API_URL  前端 API 地址（可选，默认使用相对路径）"
    echo "  DEPLOY_FRONTEND_WS_URL   前端 WebSocket 地址（可选，默认使用相对路径）"
    echo "  DEPLOY_VERBOSE       设为 1 输出详细构建日志"
    echo "  DEPLOY_INIT_NGINX    设为 true 部署时同步 nginx 配置（默认: false）"
    echo "  APP_ENV_VARS         应用环境变量（格式: KEY1=VALUE1 KEY2=VALUE2）"
    echo ""
    echo "应用环境变量（Go 读取）："
    echo "  DB_HOST              数据库地址（默认: 127.0.0.1）"
    echo "  DB_PORT              数据库端口（默认: 3306）"
    echo "  DB_NAME              数据库名（默认: loafer）"
    echo "  DB_USERNAME          数据库用户名（默认: root）"
    echo "  DB_PASSWORD          数据库密码"
    echo "  SERVER_PORT          服务端口（默认: 9080）"
    echo "  JWT_SECRET           JWT 密钥"
    echo "  APP_AUTH_USERNAME    认证用户名（默认: admin）"
    echo "  APP_AUTH_PASSWORD    认证密码"
    echo ""
    echo "Gitee 自动创建仓库配置："
    echo "  GITEE_ACCESS_TOKEN   Gitee 个人访问令牌（在 Gitee 设置-私人令牌中创建）"
    echo "  GITEE_API_BASE_URL   Gitee API 基础URL（默认: https://gitee.com/api/v5）"
    echo ""
    echo "示例："
    echo "  $0 stop                           # 停止应用"
    echo "  $0 start                          # 启动应用"
    echo "  $0 restart                        # 重启应用"
    echo "  $0 status                         # 查看状态"
    exit 0
    ;;
  *)
    echo "未知模式: $MODE (可选: backend/bd, frontend/ft, all, start, stop, restart, status)"
    exit 1
    ;;
esac

RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m'
info()  { echo -e "${BLUE}[INFO]${NC} $1"; }
ok()    { echo -e "${GREEN}[OK]${NC} $1"; }
err()   { echo -e "${RED}[ERROR]${NC} $1"; }

# =============================================================================
# 将 APP_ENV_VARS 导出为环境变量，并添加端口配置
# =============================================================================
export_app_env_vars() {
  # 处理 APP_ENV_VARS 中的参数
  if [ -n "$APP_ENV_VARS" ]; then
    for kv in $APP_ENV_VARS; do
      if [ -n "$kv" ]; then
        export "$kv"
      fi
    done
  fi

  # 添加项目端口分配参数
  if [ -n "$FRONTEND_PORT" ]; then
    export FRONTEND_PORT="${FRONTEND_PORT}"
  fi
  if [ -n "$BACKEND_PORT" ]; then
    export BACKEND_PORT="${BACKEND_PORT}"
    export SERVER_PORT="${BACKEND_PORT}"
  else
    export SERVER_PORT="${SERVER_PORT}"
  fi

  info "应用环境变量已导出（SERVER_PORT=${SERVER_PORT}）" >&2
}

# 生成启动命令的内联环境前缀：确保 INFRA_SSH_KEY_PATH 一定随进程启动传递，
# 不依赖 export 传递链（历史上经 export 注入的变量在多级 sudo/bash/nohup 下曾丢失）。
infra_start_env() {
  local pem="${INFRA_SSH_KEY_PATH:-${INFRA_SSH_KEY_PATH_DEFAULT:-/srv/zfei/sdata/loafer/dev01.pem}}"
  if [ -z "${INFRA_SSH_PEM:-}" ] && [ -f "$pem" ]; then
    echo "INFRA_SSH_KEY_PATH=$pem"
  fi
}

# =============================================================================
# 校验是否已注入数据库相关的环境变量
# =============================================================================
check_db_env_vars() {
  if [ -z "${APP_ENV_VARS:-}" ]; then
    err "未通过 APP_ENV_VARS 注入数据库参数，启动后将使用默认数据库地址，可能连不上 DB"
    echo "请使用以下方式注入数据库参数：" >&2
    echo "  1) export APP_ENV_VARS=\"DB_HOST=127.0.0.1 DB_USERNAME=root DB_PASSWORD=xxx\" && $0 backend|all|start|restart" >&2
    echo "  2) 本地开发可使用封装脚本: ./deploy-with-bak-db.sh backend|all|start|restart" >&2
    exit 1
  fi

  local has_db_config=false
  case " $APP_ENV_VARS " in
    *" DB_HOST="*)
      has_db_config=true
      ;;
  esac

  if [ "$has_db_config" = false ]; then
    err "APP_ENV_VARS 中未找到数据库地址参数（DB_HOST）"
    echo "当前 APP_ENV_VARS: ${APP_ENV_VARS}" >&2
    echo "请注入数据库参数后重试，例如：" >&2
    echo "  export APP_ENV_VARS=\"DB_HOST=127.0.0.1 DB_USERNAME=root DB_PASSWORD=xxx\" && $0 backend|all|start|restart" >&2
    exit 1
  fi
}

# =============================================================================
# 服务管理函数
# =============================================================================
get_app_pid() {
  local pid=""
  # 优先通过端口查找（使用实际后端端口）
  pid=$(sudo lsof -ti :${EFFECTIVE_BACKEND_PORT} 2>/dev/null || true)
  if [ -n "$pid" ]; then
    echo "$pid"
    return 0
  fi
  # 其次通过进程名查找
  pid=$(sudo pgrep -f "${APP_NAME}" 2>/dev/null | head -n1 || true)
  if [ -n "$pid" ]; then
    echo "$pid"
    return 0
  fi
  return 1
}

stop_app() {
  info "停止应用..."
  local stopped=false

  # 方法1: 通过端口查找进程（使用实际后端端口）
  local port_pid=$(sudo lsof -ti :${EFFECTIVE_BACKEND_PORT} 2>/dev/null || true)
  if [ -n "$port_pid" ]; then
    info "找到占用端口 ${EFFECTIVE_BACKEND_PORT} 的进程 PID: ${port_pid}"
    sudo kill "$port_pid" 2>/dev/null || true
    for i in 1 2 3 4 5; do
      if ! sudo kill -0 "$port_pid" 2>/dev/null; then
        ok "进程 ${port_pid} 已停止"
        stopped=true
        break
      fi
      sleep 1
    done
    if [ "$stopped" = false ]; then
      info "强制终止进程 ${port_pid}"
      sudo kill -9 "$port_pid" 2>/dev/null || true
      stopped=true
    fi
  fi

  # 方法2: 通过进程名查找
  local name_pids=$(sudo pgrep -f "${APP_NAME}" 2>/dev/null || true)
  if [ -n "$name_pids" ]; then
    info "找到匹配 ${APP_NAME} 的进程: ${name_pids}"
    for pid in $name_pids; do
      sudo kill "$pid" 2>/dev/null || true
    done
    sleep 2
    for pid in $name_pids; do
      if sudo kill -0 "$pid" 2>/dev/null; then
        sudo kill -9 "$pid" 2>/dev/null || true
      fi
    done
    stopped=true
  fi

  # 确保端口已释放
  for i in 1 2 3; do
    if ! sudo lsof -ti :${EFFECTIVE_BACKEND_PORT} >/dev/null 2>&1; then
      info "端口 ${EFFECTIVE_BACKEND_PORT} 已释放"
      break
    fi
    sleep 1
  done

  if [ "$stopped" = true ]; then
    ok "应用已停止"
  else
    info "未找到运行中的应用"
  fi
}

start_app() {
  info "启动应用..."

  # 检查是否已在运行
  local existing_pid=$(get_app_pid)
  if [ -n "$existing_pid" ]; then
    err "应用已在运行中 (PID: ${existing_pid})"
    return 1
  fi

  # 检查二进制文件
  if [ ! -f "${PROJECT_DIR}/backend-go/${APP_BIN}" ]; then
    err "未找到二进制文件: ${PROJECT_DIR}/backend-go/${APP_BIN}"
    return 1
  fi

  cd "${PROJECT_DIR}/backend-go"

  # 导出应用环境变量
  export_app_env_vars

  # INFRA_SSH_KEY_PATH 以 inline env 前缀传递，确保启动环境不丢失
  env $(infra_start_env) nohup "./${APP_BIN}" > ../app.log 2>&1 &
  local new_pid=$!
  info "已启动新进程 PID: ${new_pid}"

  info "等待应用就绪..."
  sleep 5

  for i in 1 2 3 4 5 6 7 8; do
    local code=$(curl -sS -o /dev/null -m 5 --connect-timeout 5 -w '%{http_code}' "http://127.0.0.1:${EFFECTIVE_BACKEND_PORT}/api/" 2>/dev/null || echo "000")
    if [ "$code" != "000" ]; then
      ok "应用已启动，端口 ${EFFECTIVE_BACKEND_PORT} 响应正常 (HTTP: ${code})"
      return 0
    fi
    info "等待中... ($i/8)"
    sleep 3
  done

  err "应用启动超时，请检查 ${PROJECT_DIR}/app.log"
  return 1
}

show_status() {
  info "应用状态检查"
  echo "----------------------------------------"

  local pid=$(get_app_pid)
  if [ -n "$pid" ]; then
    echo -e "状态:    ${GREEN}运行中${NC}"
    echo "PID:     ${pid}"
    echo "端口:    ${EFFECTIVE_BACKEND_PORT}"

    # 显示进程详情
    local proc_info=$(sudo ps -p "$pid" -o pid,ppid,%cpu,%mem,etime,args --no-headers 2>/dev/null || true)
    if [ -n "$proc_info" ]; then
      echo "详情:    ${proc_info}"
    fi

    # 测试端口响应
    local code=$(curl -sS -o /dev/null -m 3 --connect-timeout 3 -w '%{http_code}' "http://127.0.0.1:${EFFECTIVE_BACKEND_PORT}/api/" 2>/dev/null || echo "000")
    if [ "$code" != "000" ]; then
      echo -e "健康检查: ${GREEN}正常 (HTTP ${code})${NC}"
    else
      echo -e "健康检查: ${RED}无响应${NC}"
    fi
  else
    echo -e "状态:    ${RED}未运行${NC}"
    echo "端口:    ${EFFECTIVE_BACKEND_PORT} (未监听)"
  fi

  echo "----------------------------------------"
}

info "部署模式: ${MODE}"
info "项目目录: ${PROJECT_DIR}"
info "构建日志: $([ "${DEPLOY_VERBOSE}" = "1" ] && echo "详细" || echo "静默")"

# 显示端口配置信息
if [ -n "${FRONTEND_PORT}" ] || [ -n "${BACKEND_PORT}" ]; then
  info "项目端口分配: FRONTEND_PORT=${FRONTEND_PORT:-未设置}, BACKEND_PORT=${BACKEND_PORT:-未设置}"
  [ -n "${NGINX_CONF_PATH}" ] && info "Nginx 配置路径: ${NGINX_CONF_PATH}"
fi

# -----------------------------------------------------------------------------
# 服务管理模式处理（不拉代码不构建）
# -----------------------------------------------------------------------------
if [ "${MODE}" = "service" ]; then
  info "服务管理模式: ${ACTION}"
  cd "${PROJECT_DIR}" 2>/dev/null || true

  case "${ACTION}" in
    start)
      check_db_env_vars
      start_app
      exit $?
      ;;
    stop)
      stop_app
      exit 0
      ;;
    restart)
      check_db_env_vars
      stop_app
      sleep 2
      start_app
      exit $?
      ;;
    status)
      show_status
      exit 0
      ;;
    *)
      err "未知服务操作: ${ACTION}"
      exit 1
      ;;
  esac
fi

# -----------------------------------------------------------------------------
# 设置区域与编码
# -----------------------------------------------------------------------------
export LANG="${DEPLOY_LANG:-en_US.UTF-8}"
export LC_ALL="${DEPLOY_LC_ALL:-$LANG}"

log() {
  echo "[INFO] $1"
}

log "开始本地部署脚本"
log "PROJECT_DIR=${PROJECT_DIR}"
log "MODE=${MODE}, PORT=${EFFECTIVE_BACKEND_PORT}"
log "项目端口: FRONTEND=${FRONTEND_PORT:-未设置}, BACKEND=${BACKEND_PORT:-未设置}"
log "当前用户: $(whoami)"
log "当前目录: $(pwd)"
log "PATH=${PATH}"

if [ ! -d "${PROJECT_DIR}" ]; then
  err "目录不存在: ${PROJECT_DIR}"
  echo "提示: 请确认项目目录路径是否正确"
  exit 1
fi

cd "${PROJECT_DIR}"
log "切换目录后: $(pwd)"
log "目录内容预览:"
ls -la | sed -n '1,40p'

if [ ! -d ".git" ] && [ -d "source/.git" ]; then
  log "检测到 ${PROJECT_DIR}/source 为 git 仓库，切换到 source 目录继续部署"
  cd source
  log "切换后目录: $(pwd)"
fi

if [ ! -d ".git" ]; then
  err "当前目录不是 git 仓库"
  echo "当前目录: $(pwd)"
  echo "目录内容（前40行）:"
  ls -la | sed -n '1,40p'
  echo "建议:"
  echo "  1) 若代码在 ${PROJECT_DIR}/source，请保留该目录结构后重试"
  echo "  2) 或设置 DEPLOY_PROJECT_DIR 为实际 git 仓库目录"
  echo "  3) 或执行 'git clone <repo> <git目录>'"
  exit 1
fi

log "检测到 git 仓库"
log "git remote -v:"
git remote -v || true
log "git status -sb:"
git status -sb || true
log "当前分支: $(git symbolic-ref --short HEAD 2>/dev/null || git rev-parse --abbrev-ref HEAD)，当前提交: $(git rev-parse --short HEAD)"

# 拉取最新代码
log "拉取最新代码..."
git fetch origin 2>/dev/null || true
git pull origin "$(git symbolic-ref --short HEAD 2>/dev/null || echo master)" 2>&1 || {
  err "git pull 失败，请手动解决冲突后重试"
  exit 1
}
log "拉取后提交: $(git rev-parse --short HEAD)"

if [ "${MODE}" = "frontend" ] || [ "${MODE}" = "all" ]; then
  if [ ! -d "frontend" ]; then
    err "未找到 frontend 目录"
    exit 1
  fi
  log "开始构建前端"
  echo "构建前端..."
  # 注入前端环境变量配置
  if [ -n "$FRONTEND_API_URL" ] || [ -n "$FRONTEND_WS_URL" ]; then
    log "创建 .env.production 文件，API=${FRONTEND_API_URL:-未设置}, WS=${FRONTEND_WS_URL:-未设置}"
    cat > frontend/.env.production << EOF
# 自动生成 - 部署时注入
VITE_API_BASE_URL=${FRONTEND_API_URL}
VITE_WS_BASE_URL=${FRONTEND_WS_URL}
EOF
  fi
  # 仅在依赖清单变更或 node_modules 缺失时执行 npm ci，避免每次部署重复安装 400M+ 依赖
  need_npm_ci=false
  if [ ! -d "frontend/node_modules" ]; then
    need_npm_ci=true
  elif [ ! -f "frontend/node_modules/.package-lock.json" ]; then
    need_npm_ci=true
  elif [ "frontend/package-lock.json" -nt "frontend/node_modules/.package-lock.json" ]; then
    need_npm_ci=true
  fi

  if [ "$need_npm_ci" = true ]; then
    log "检测到前端依赖需要更新，执行 npm ci（超时 300s）..."
    NODE_OPTIONS="${NODE_OPTIONS:-} --max-old-space-size=1024" timeout 300 bash -c 'cd frontend && npm ci'
  else
    log "前端依赖未变更，跳过 npm ci"
  fi

  log "执行 Vite 生产构建（超时 300s）..."
  NODE_OPTIONS="${NODE_OPTIONS:-} --max-old-space-size=1024" timeout 300 bash -c 'cd frontend && npm run build'
  sudo rm -rf "${FRONTEND_DIR}"
  sudo mkdir -p "${FRONTEND_DIR}"
  sudo cp -r frontend/dist/. "${FRONTEND_DIR}/"
  echo "前端静态文件已更新: ${FRONTEND_DIR}"
  src_hash="$( (cd frontend/dist && shasum index.html 2>/dev/null | awk '{print $1}') || true )"
  dst_hash="$( (cd "${FRONTEND_DIR}" && shasum index.html 2>/dev/null | awk '{print $1}') || true )"
  src_js="$( (cd frontend/dist/assets && ls -1 *.js 2>/dev/null | head -n 3 | tr '\n' ',' | sed 's/,$//') || true )"
  dst_js="$( (cd "${FRONTEND_DIR}/assets" && ls -1 *.js 2>/dev/null | head -n 3 | tr '\n' ',' | sed 's/,$//') || true )"
  log "前端验收: git=$(git rev-parse --short HEAD)"
  log "前端验收: dist/index.html sha1=${src_hash}"
  log "前端验收: publish/index.html sha1=${dst_hash}"
  log "前端验收: dist/assets(js 示例)=${src_js}"
  log "前端验收: publish/assets(js 示例)=${dst_js}"
  if [ -n "${src_hash}" ] && [ -n "${dst_hash}" ] && [ "${src_hash}" != "${dst_hash}" ]; then
    echo "警告: 前端发布目录与构建产物 index.html 指纹不一致，请检查拷贝权限/挂载路径"
  fi
  log "前端构建与拷贝完成"
fi

if [ "${MODE}" = "frontend" ]; then
  ok "前端模式部署完成（已更新前端目录: ${FRONTEND_DIR}）"
  exit 0
fi

# 后端/全量部署前必须注入数据库环境变量，避免使用默认地址启动失败
check_db_env_vars

# 构建前检查可用内存（含 swap），低于 400MB 时警告（Go 编译比 Maven 需求低）
check_available_memory() {
  local avail_kb=$(grep MemAvailable /proc/meminfo | awk '{print $2}')
  local swap_free_kb=$(grep SwapFree /proc/meminfo | awk '{print $2}')
  local total_avail_mb=$(( (avail_kb + swap_free_kb) / 1024 ))
  if [ "$total_avail_mb" -lt 400 ]; then
    err "可用内存不足 ${total_avail_mb}MB（需 >= 400MB），构建可能导致系统假死"
    echo "建议：1) 停止不必要的进程释放内存 2) 扩容至 8GB 3) 增加 swap" >&2
    exit 1
  fi
  info "可用内存: ${total_avail_mb}MB (RAM+Swap)，满足构建最低要求"
}
check_available_memory

log "开始构建后端（Go）"

# —— 构建前校验：确保 backend-go 目录存在 ——
if [ ! -d "backend-go" ]; then
  err "未找到 backend-go 目录，请确认代码已同步"
  echo "当前提交: $(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
  exit 1
fi

if [ ! -f "backend-go/go.mod" ]; then
  err "backend-go/go.mod 不存在，当前检出可能过旧。请先同步/拉取最新代码后再部署。"
  echo "当前提交: $(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
  exit 1
fi

echo "构建后端（Go）..."

# 查找 Go 二进制
GO_BIN=go
[ -n "${GO_PATH}" ] && GO_BIN="${GO_PATH}"
if [ "${GO_BIN}" = "go" ]; then
  for p in \
    /usr/local/go/bin/go \
    /usr/lib/go/bin/go \
    /usr/bin/go \
    /snap/go/current/bin/go
  do
    [ -x "$p" ] && GO_BIN="$p" && break
  done
fi
if ! command -v "${GO_BIN}" >/dev/null 2>&1 && [ ! -x "${GO_BIN}" ]; then
  err "未找到可用的 Go，请安装 Go 1.21+ 或设置 DEPLOY_GO=/usr/local/go/bin/go"
  exit 1
fi
log "使用 Go: ${GO_BIN}"
"${GO_BIN}" version

# 设置 Go 代理（国内环境加速模块下载）
export GOPROXY="${GOPROXY:-https://goproxy.cn,direct}"
log "GOPROXY=${GOPROXY}"

# 进入 backend-go 目录构建
cd backend-go

# 下载依赖
log "下载 Go 依赖..."
if [ "${DEPLOY_VERBOSE:-0}" = "1" ]; then
  "${GO_BIN}" mod download -x
else
  "${GO_BIN}" mod download
fi

# 构建
log "执行 Go 构建..."
GO_BUILD_ARGS="build -o ${APP_BIN} ./cmd/server/"
if [ "${DEPLOY_VERBOSE:-0}" = "1" ]; then
  log "DEPLOY_VERBOSE=1，输出详细构建日志"
  GO_BUILD_ARGS="build -v -o ${APP_BIN} ./cmd/server/"
fi

# shellcheck disable=SC2086
"${GO_BIN}" ${GO_BUILD_ARGS}

if [ ! -f "${APP_BIN}" ]; then
  err "构建失败：未找到输出二进制 ${APP_BIN}"
  exit 1
fi

# 确保二进制可执行
chmod +x "${APP_BIN}"

log "后端构建完成: backend-go/${APP_BIN} ($(stat -c%s "${APP_BIN}" 2>/dev/null || stat -f%z "${APP_BIN}" 2>/dev/null || echo '?') bytes)"

# 回到项目根目录
cd "${PROJECT_DIR}"

# 停止旧进程
log "检查并停止旧进程..."
OLD_PID=""
# 方法1: 通过端口查找进程（使用实际后端端口）
OLD_PID=$(sudo lsof -ti :${EFFECTIVE_BACKEND_PORT} 2>/dev/null || true)
if [ -n "$OLD_PID" ]; then
  log "找到占用端口 ${EFFECTIVE_BACKEND_PORT} 的进程 PID: ${OLD_PID}"
  sudo kill "$OLD_PID" 2>/dev/null || true
  for i in 1 2 3 4 5; do
    if ! sudo kill -0 "$OLD_PID" 2>/dev/null; then
      log "旧进程已停止"
      break
    fi
    [ $i -lt 5 ] && sleep 1
  done
  sudo kill -9 "$OLD_PID" 2>/dev/null || true
fi

# 方法2: 通过进程名查找
OLD_PIDS=$(sudo pgrep -f "${APP_NAME}" 2>/dev/null || true)
if [ -n "$OLD_PIDS" ]; then
  log "找到匹配 ${APP_NAME} 的进程: ${OLD_PIDS}"
  for pid in $OLD_PIDS; do
    sudo kill "$pid" 2>/dev/null || true
  done
  sleep 2
  for pid in $OLD_PIDS; do
    sudo kill -9 "$pid" 2>/dev/null || true
  done
fi

# 确保端口已释放
for i in 1 2 3 4 5; do
  if ! sudo lsof -ti :${EFFECTIVE_BACKEND_PORT} >/dev/null 2>&1; then
    log "端口 ${EFFECTIVE_BACKEND_PORT} 已释放"
    break
  fi
  [ $i -lt 5 ] && sleep 2
done

# 确认旧进程已完全退出，避免新旧并存耗尽内存
for i in 1 2 3 4 5; do
  if ! sudo pgrep -f "${APP_NAME}" >/dev/null 2>&1; then
    log "旧进程已完全退出"
    break
  fi
  log "等待旧进程退出... ($i/5)"
  sleep 2
done

# 导出应用环境变量
export_app_env_vars

# 启动新进程（INFRA_SSH_KEY_PATH 以 inline env 前缀传递，确保启动环境不丢失）
cd "${PROJECT_DIR}/backend-go"
env $(infra_start_env) nohup "./${APP_BIN}" > ../app.log 2>&1 &
echo "已启动新进程，等待就绪..."
cd "${PROJECT_DIR}"
sleep 8
log "最近日志预览:"
sed -n '1,80p' app.log || true

for i in 1 2 3 4 5 6 7 8 9 10; do
  code_root="$(curl -sS -o /dev/null -m 5 --connect-timeout 5 -w '%{http_code}' "http://127.0.0.1:${EFFECTIVE_BACKEND_PORT}/" || true)"
  code_api="$(curl -sS -o /dev/null -m 5 --connect-timeout 5 -w '%{http_code}' "http://127.0.0.1:${EFFECTIVE_BACKEND_PORT}/api/" || true)"
  log "探活第 ${i} 次: / -> ${code_root}, /api/ -> ${code_api}"
  if [ "${code_root}" != "000" ] || [ "${code_api}" != "000" ]; then
    echo "应用已在端口 ${EFFECTIVE_BACKEND_PORT} 响应（HTTP: /=${code_root}, /api/=${code_api}）"
    break
  fi
  [ $i -lt 10 ] && sleep 3
done

# -----------------------------------------------------------------------------
# 自动部署 Nginx 配置
# -----------------------------------------------------------------------------
DEPLOY_INIT_NGINX="${DEPLOY_INIT_NGINX:-false}"

# 检查是否需要动态生成 nginx 站点配置
if [ "${DEPLOY_INIT_NGINX}" = "true" ]; then
  if [ -n "${NGINX_CONF_PATH}" ] && [ -n "${FRONTEND_PORT}" ] && [ -n "${BACKEND_PORT}" ]; then
    log "动态生成 Nginx 站点配置: ${NGINX_CONF_PATH}"
    log "前端端口: ${FRONTEND_PORT}, 后端端口: ${BACKEND_PORT}"

    # 直接覆盖配置文件（不创建备份）
    sudo tee "${NGINX_CONF_PATH}" > /dev/null << 'NGINX_EOF'
# Claude Sprint 前端站点配置
# 自动生成于部署时

server {
    listen FRONTEND_PORT_PLACEHOLDER;
    server_name localhost;

    root /opt/claude_sprint/frontend-dist;
    index index.html;
    client_max_body_size 2g;

    # 前端静态资源
    location / {
        try_files $uri $uri/ /index.html;

        # 静态资源缓存优化
        expires 1h;
        add_header Cache-Control "public, immutable";
    }

    # —— REST API 代理（不含 WebSocket）——
    location /api/ {
        charset utf-8;
        source_charset utf-8;

        proxy_pass http://127.0.0.1:BACKEND_PORT_PLACEHOLDER/api/;

        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # HTTP/1.1 保持连接
        proxy_http_version 1.1;
        proxy_set_header Connection "";

        # SSE / 长耗时流式接口配置
        proxy_buffering off;
        proxy_cache off;
        proxy_request_buffering off;
        proxy_set_header X-Accel-Buffering no;

        # 超时设置
        proxy_connect_timeout 60s;
        proxy_send_timeout 600s;
        proxy_read_timeout 600s;

        # 流式响应禁用 gzip
        gzip off;
    }

    # —— WebSocket 专用代理（关键优化）——
    location /ws {
        proxy_pass http://127.0.0.1:BACKEND_PORT_PLACEHOLDER;

        # WebSocket 必需头部
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;

        # 超时设置：WebSocket 长连接需要更长超时
        proxy_connect_timeout 7d;
        proxy_send_timeout 7d;
        proxy_read_timeout 7d;

        # 禁用缓冲：WebSocket 需要实时传输
        proxy_buffering off;
        proxy_request_buffering off;
        proxy_cache off;

        # 禁用 gzip
        gzip off;

        # SSE 流式响应兼容
        proxy_set_header X-Accel-Buffering no;
    }

    # —— 健康检查端点 ——
    location /health {
        access_log off;
        return 200 "OK\n";
        add_header Content-Type text/plain;
    }
}
NGINX_EOF

    # 替换端口占位符
    sudo sed -i "s/FRONTEND_PORT_PLACEHOLDER/${FRONTEND_PORT}/g" "${NGINX_CONF_PATH}"
    sudo sed -i "s/BACKEND_PORT_PLACEHOLDER/${BACKEND_PORT}/g" "${NGINX_CONF_PATH}"

    log "已生成 Nginx 配置: ${NGINX_CONF_PATH}"
    log "配置内容预览:"
    sudo grep -E "listen|proxy_pass" "${NGINX_CONF_PATH}" | head -10

    # 测试并重载
    if sudo nginx -t 2>/dev/null; then
      sudo nginx -s reload 2>/dev/null || true
      log "Nginx 配置已重载"
    else
      echo "警告: nginx 配置测试失败，请手动检查"
    fi
  else
    log "未提供 NGINX_CONF_PATH / FRONTEND_PORT / BACKEND_PORT，跳过 nginx 配置部署"
  fi
else
  log "DEPLOY_INIT_NGINX 未设置为 true，跳过 nginx 配置部署"
fi

# 最终检查
for i in 1 2 3; do
  code_root="$(curl -sS -o /dev/null -m 5 --connect-timeout 5 -w '%{http_code}' "http://127.0.0.1:${EFFECTIVE_BACKEND_PORT}/" || true)"
  code_api="$(curl -sS -o /dev/null -m 5 --connect-timeout 5 -w '%{http_code}' "http://127.0.0.1:${EFFECTIVE_BACKEND_PORT}/api/" || true)"
  if [ "${code_root}" != "000" ] || [ "${code_api}" != "000" ]; then
    ok "部署完成"
    exit 0
  fi
  [ $i -lt 3 ] && sleep 2
done

err "端口 ${EFFECTIVE_BACKEND_PORT} 暂未响应，请检查 ${PROJECT_DIR}/app.log"
exit 1
