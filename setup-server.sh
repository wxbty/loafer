#!/bin/bash
# =============================================================================
# Loafer 平台服务器初始化脚本
# 在目标服务器上执行，完成以下操作：
#   1. 创建 loafer 数据库
#   2. 安装/检查 Nginx
#   3. 配置防火墙开放端口 40410-40500
#   4. 创建部署目录结构
#   5. 安装 Node.js（用于前端构建和 Playwright）
#   6. 安装 Go（用于后端编译）
#   7. 配置 Claude Code CLI
# 用法: ssh root@your_server 'bash -s' < setup-server.sh
# =============================================================================

set -e

echo "===== Loafer 平台服务器初始化 ====="

# ===== 1. 创建 loafer 数据库 =====
echo "[1/7] 创建 loafer 数据库..."
MYSQL_HOST="${MYSQL_HOST:-127.0.0.1}"
MYSQL_PORT="${MYSQL_PORT:-3306}"
MYSQL_USER="${MYSQL_USER:-root}"
MYSQL_PASS="${MYSQL_PASS:?请通过环境变量设置 MYSQL_PASS}"

mysql -h${MYSQL_HOST} -P${MYSQL_PORT} -u${MYSQL_USER} -p${MYSQL_PASS} -e "
  CREATE DATABASE IF NOT EXISTS loafer CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
  SELECT 'Database loafer is ready' AS status;
" 2>/dev/null && echo "✓ loafer 数据库已创建" || echo "⚠ 数据库创建失败，请检查 MySQL 连接"

# ===== 2. 安装/检查 Nginx =====
echo "[2/7] 检查 Nginx..."
if command -v nginx &>/dev/null; then
  echo "✓ Nginx 已安装: $(nginx -v 2>&1)"
else
  echo "  安装 Nginx..."
  if command -v apt &>/dev/null; then
    apt update -qq && apt install -y -qq nginx
  elif command -v yum &>/dev/null; then
    yum install -y nginx
  else
    echo "⚠ 无法自动安装 Nginx，请手动安装"
  fi
  systemctl enable nginx
  systemctl start nginx
  echo "✓ Nginx 安装完成"
fi

# 确保 Nginx 加载 conf.d 目录
NGINX_CONF="/etc/nginx/nginx.conf"
if ! grep -q "include /etc/nginx/conf.d/\*.conf;" "$NGINX_CONF"; then
  echo "  配置 Nginx 加载 conf.d 目录..."
  sed -i '/http {/a \    include /etc/nginx/conf.d/*.conf;' "$NGINX_CONF"
  systemctl reload nginx
fi

# ===== 3. 配置防火墙 =====
echo "[3/7] 配置防火墙..."
# 开放端口范围 40410-40500
if command -v firewall-cmd &>/dev/null; then
  firewall-cmd --permanent --add-port=40410-40500/tcp
  firewall-cmd --permanent --add-port=9080/tcp
  firewall-cmd --reload
  echo "✓ firewall-cmd 已开放端口 40410-40500, 9080"
elif command -v ufw &>/dev/null; then
  for port in 40410 40411 40412 40413 40414 40415 9080; do
    ufw allow ${port}/tcp
  done
  echo "✓ ufw 已开放关键端口"
else
  echo "⚠ 未找到防火墙工具，请手动开放端口 40410-40500 和 9080"
fi

# ===== 4. 创建部署目录结构 =====
echo "[4/7] 创建部署目录..."
mkdir -p /opt/loafer/projects
mkdir -p /opt/loafer/workspace
mkdir -p /opt/loafer/frontend-dist
mkdir -p /etc/nginx/conf.d
mkdir -p /var/log/loafer
echo "✓ 部署目录已创建:
  - /opt/loafer/projects (项目部署)
  - /opt/loafer/workspace (项目工作区)
  - /opt/loafer/frontend-dist (前端构建产物)
  - /etc/nginx/conf.d (Nginx站点配置)
  - /var/log/loafer (日志)"

# ===== 5. 安装 Node.js =====
echo "[5/7] 检查 Node.js..."
if command -v node &>/dev/null; then
  echo "✓ Node.js 已安装: $(node -v)"
else
  echo "  安装 Node.js 20.x..."
  if command -v apt &>/dev/null; then
    curl -fsSL https://deb.nodesource.com/setup_20.x | bash -
    apt install -y nodejs
  elif command -v yum &>/dev/null; then
    curl -fsSL https://rpm.nodesource.com/setup_20.x | bash -
    yum install -y nodejs
  fi
  echo "✓ Node.js 安装完成: $(node -v)"
fi

# 安装 npm 全局依赖
echo "  检查全局 npm 包..."
npm list -g playwright 2>/dev/null || npm install -g playwright
echo "✓ Playwright 已安装"

# ===== 6. 安装 Go =====
echo "[6/7] 检查 Go..."
if command -v go &>/dev/null; then
  echo "✓ Go 已安装: $(go version)"
else
  echo "  安装 Go 1.24..."
  cd /tmp
  wget -q https://go.dev/dl/go1.24.0.linux-amd64.tar.gz
  rm -rf /usr/local/go
  tar -C /usr/local -xzf go1.24.0.linux-amd64.tar.gz
  echo 'export PATH=$PATH:/usr/local/go/bin' >> /etc/profile
  export PATH=$PATH:/usr/local/go/bin
  rm -f go1.24.0.linux-amd64.tar.gz
  echo "✓ Go 安装完成: $(go version)"
fi

# 设置 GOPROXY
export GOPROXY=https://goproxy.cn,direct
grep -q "GOPROXY" /etc/profile || echo 'export GOPROXY=https://goproxy.cn,direct' >> /etc/profile
echo "✓ GOPROXY 已设置为 goproxy.cn"

# ===== 7. 检查 Claude Code CLI =====
echo "[7/7] 检查 Claude Code CLI..."
if command -v claude &>/dev/null; then
  echo "✓ Claude Code CLI 已安装"
else
  echo "⚠ Claude Code CLI 未安装"
  echo "  请手动安装: npm install -g @anthropic-ai/claude-code"
  echo "  安装后运行: claude auth login"
fi

# ===== 完成 =====
echo ""
echo "===== 服务器初始化完成 ====="
echo "数据库: ${MYSQL_HOST}:${MYSQL_PORT}/loafer"
echo "Nginx: $(nginx -v 2>&1)"
echo "Node.js: $(node -v 2>/dev/null || echo '未安装')"
echo "Go: $(go version 2>/dev/null || echo '未安装')"
echo "端口范围: 40410-40500, 9080"
echo ""
echo "下一步:"
echo "  1. 将 PEM 密钥写入 /root/.ssh/loafer_rsa"
echo "  2. 上传代码到服务器"
echo "  3. 运行 ./deploy-local.sh all 部署应用"
