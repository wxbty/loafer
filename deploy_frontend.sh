#!/bin/bash

set -e

echo "开始部署前端..."

cd /srv/zfei/projects/loafer/frontend

echo "清理旧构建..."
sudo rm -rf dist

echo "构建前端..."
npm run build

echo "部署到 /opt/loafer/frontend-dist..."
sudo rm -rf /opt/loafer/frontend-dist
sudo cp -r dist /opt/loafer/frontend-dist
sudo chown -R root:root /opt/loafer/frontend-dist

echo "前端部署完成！"
