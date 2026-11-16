#!/bin/bash

set -e

echo "Building GARM SPA (SvelteKit)..."

# Navigate to webapp directory
cd webapp

# 资源路径前缀。平台把应用挂在 /app/<app-id> 下，缺这个前缀时 SPA 会去请求
# /ui/_app/...，经 Traefik 无路由可匹配（404），平台 iframe 里白屏。
# 生产镜像由 Dockerfile 的 GARM_BASE_PATH 构建参数注入；本地构建取下面默认值，
# 也可用 VITE_GARM_BASE_PATH 覆盖（例如置空做脱离平台的调试）。
# 判断「是否设置」而不是「是否为空」：置空是有效用法。
if [ -z "${VITE_GARM_BASE_PATH+set}" ] && [ -f ../ictrek.app/src/garm/base-path ]; then
	VITE_GARM_BASE_PATH="$(cat ../ictrek.app/src/garm/base-path)"
	export VITE_GARM_BASE_PATH
fi
echo "SPA base path: ${VITE_GARM_BASE_PATH:-<empty>}"

# Install dependencies if node_modules doesn't exist
npm install

# Build the SPA
echo "Building SPA..."
npm run build
echo "SPA built successfully!"
