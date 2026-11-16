#!/usr/bin/env bash
# scripts/vos-dev.sh — 在 VOS 平台内联调 garm（免 push / 免推镜像 / 免重装应用）
#
# 一条命令起两套热重载：
#   后端 —— dev 容器（源码挂载 + air），改 .go 自动重编译
#   前端 —— 宿主 vite dev server，改 .svelte 毫秒级 HMR
# 再装 Traefik 临时路由（/ui 走 vite，其余走 garm），前台跟随日志，Ctrl-C 统一收尾。
# 平台上已安装的 garm 容器全程不动。
#
# 用法：
#   make vos-dev                热重载模式
#   make vos-dev-debug          断点模式：dlv 监听宿主机 :39097，无热重载
#   make vos-dev-down           撤临时路由 + 停容器 + 停 vite
#   scripts/vos-dev.sh status   查看容器/健康/路由
#   scripts/vos-dev.sh logs     跟随容器日志
#   scripts/vos-dev.sh route-off 只撤临时路由
#
# 可覆盖的环境变量：
#   VOS_DEV_PORT(38097)              dev 容器宿主端口（容器内仍是 9997）
#   VOS_DEV_WEB_PORT(5173)           vite dev server 端口（宿主）
#   VOS_DEV_WEB(1)                   0=不起 vite，直接用二进制里嵌的旧前端产物
#   VOS_DEV_DELVE_PORT(39097)        dlv 宿主端口
#   DEV_DATA_DIR(../../.dev-data)    sqlite 与 config.toml 的宿主目录
#   GARM_LOG_LEVEL(debug)            日志级别
#   VOS_DEV_DEBUG(0)                 1=dlv 模式
#   VOS_HOST_GW_IP(172.17.0.1)       Traefik 回源宿主的网关 IP
#   VOS_TRAEFIK_CONTAINER(traefik)   Traefik 容器名
#   VOS_DEV_HEALTH_TIMEOUT(300)      等待容器健康的秒数
#   GARM_LXD_SOCKET_PATH(/var/snap/lxd/common/lxd/unix.socket)  宿主 LXD socket
#   GARM_LXD_INSTANCE_TYPE(container)  LXD 实例形态：container / virtual-machine
#   GARM_LXD_PROJECT(default)         LXD 项目名
#
# provider 二进制（garm-provider-lxd）首次启动时从已安装的 garm 容器或本地镜像
# 提取到 .dev-providers/。拿不到时 garm 以无 provider 模式运行，VOS ensure 会报
# no such provider lxd。
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

APP_ID="com.ictrek.garm"
BASE_PATH="$(cat ictrek.app/src/garm/base-path)"
DEV_PORT="${VOS_DEV_PORT:-38097}"
WEB_PORT="${VOS_DEV_WEB_PORT:-5173}"
DELVE_PORT="${VOS_DEV_DELVE_PORT:-39097}"
WEB_ENABLED="${VOS_DEV_WEB:-1}"
WEB_LOG="${TMPDIR:-/tmp}/garm-vos-dev-web.log"
WEB_PID=""
HOST_GW="${VOS_HOST_GW_IP:-172.17.0.1}"
# Traefik 回源和 VOS SSO 的 userinfo 都走同一个网关，由 HOST_GW 统一派生。
# compose 里的默认值只是手动 docker compose 使用时的兜底。
export GARM_VOS_USERINFO_URL="${GARM_VOS_USERINFO_URL:-http://${HOST_GW}:1182/v1000/oauth2/userinfo}"
TRAEFIK_CONTAINER="${VOS_TRAEFIK_CONTAINER:-traefik}"
ROUTE_DIR="${VOS_TRAEFIK_DYNAMIC_DIR:-/etc/traefik/dynamic-conf}"
ROUTE_FILE="${ROUTE_DIR}/${APP_ID}.dev.yml"
ROUTE_TMPL="docker/dev/traefik.dev.yml.tmpl"
CONTAINER="${VOS_DEV_CONTAINER_NAME:-garm-vos-dev}"
SERVICE="garm"
PROVIDER_DIR=".dev-providers"
PROVIDER_BIN="garm-provider-lxd"
LXD_SOCKET="${GARM_LXD_SOCKET_PATH:-/var/snap/lxd/common/lxd/unix.socket}"
COMPOSE_FILES=(-f docker/dev/docker-compose.dev.yml -f docker/dev/docker-compose.vos-dev.yml)

log() { printf '\033[36m[vos-dev]\033[0m %s\n' "$*"; }
warn() { printf '\033[33m[vos-dev] %s\033[0m\n' "$*" >&2; }
die() { printf '\033[31m[vos-dev][ERROR]\033[0m %s\n' "$*" >&2; exit 1; }

compose() {
	docker compose "${COMPOSE_FILES[@]}" "$@"
}

# 宿主 LXD socket 存在时才挂载；否则 pool 里没有可用 provider。
maybe_add_lxd() {
	if [ -S "$LXD_SOCKET" ]; then
		COMPOSE_FILES+=(-f docker/dev/docker-compose.lxd.yml)
		return 0
	fi
	warn "没有 LXD socket（$LXD_SOCKET）：provider 连不上 LXD，跑不了 runner 实例"
}

# provider 二进制不在仓库里。优先从运行中的已安装 garm 容器复制，其次从本地镜像。
prepare_provider() {
	local target="${PROVIDER_DIR}/${PROVIDER_BIN}"
	if [ -x "$target" ]; then
		return 0
	fi
	mkdir -p "$PROVIDER_DIR"
	local container
	container="$(docker ps --format '{{.Names}}' | grep -m1 'ictrek-garm' || true)"
	if [ -n "$container" ] &&
		docker cp "${container}:/opt/garm/providers.d/${PROVIDER_BIN}" "$target" >/dev/null 2>&1; then
		chmod +x "$target"
		log "已从 ${container} 复制 ${PROVIDER_BIN} 到 ${PROVIDER_DIR}/"
		return 0
	fi
	local image
	image="$(docker images --format '{{.Repository}}:{{.Tag}}' | grep -m1 'ictrek/garm' || true)"
	if [ -n "$image" ]; then
		docker create --name garm-dev-provider-extract "$image" >/dev/null 2>&1 || true
		if docker cp "garm-dev-provider-extract:/opt/garm/providers.d/${PROVIDER_BIN}" "$target" >/dev/null 2>&1; then
			chmod +x "$target"
			log "已从镜像 ${image} 提取 ${PROVIDER_BIN}"
		fi
		docker rm -f garm-dev-provider-extract >/dev/null 2>&1 || true
	fi
	if [ -x "$target" ]; then
		return 0
	fi
	warn "拿不到 ${PROVIDER_BIN}：garm 会以无 provider 模式启动，VOS ensure 会报 no such provider lxd"
	warn "手动放一份：cp <已安装容器>:/opt/garm/providers.d/${PROVIDER_BIN} ${PROVIDER_DIR}/"
	return 0
}

port_in_use() { (exec 3<>"/dev/tcp/127.0.0.1/$1") 2>/dev/null; }

ensure_port_free() {
	if port_in_use "$1"; then
		die "端口 $1 已被占用（多半是残留容器）。先释放，或换端口：VOS_DEV_PORT=38098 make vos-dev"
	fi
}

cmd_route_on() {
	if ! docker ps --format '{{.Names}}' | grep -qx "$TRAEFIK_CONTAINER"; then
		warn "未发现 Traefik 容器 ${TRAEFIK_CONTAINER}：临时路由未写入，只能直连 http://127.0.0.1:${DEV_PORT}${BASE_PATH}/"
		return 0
	fi
	# 前端没起时，/ui 也指回后端，由二进制里的嵌入产物提供页面
	local ui_port="$WEB_PORT"
	[ "$WEB_ENABLED" = "1" ] || ui_port="$DEV_PORT"
	if ! sed -e "s|__HOST__|${HOST_GW}|g" -e "s|__PORT__|${DEV_PORT}|g" \
		-e "s|__WEB_PORT__|${ui_port}|g" -e "s|__BASE_PATH__|${BASE_PATH}|g" "$ROUTE_TMPL" |
		docker exec -i "$TRAEFIK_CONTAINER" sh -c "cat > '${ROUTE_FILE}'"; then
		warn "写入临时路由失败，只能直连 http://127.0.0.1:${DEV_PORT}${BASE_PATH}/"
		return 0
	fi
	# Traefik 的 file provider 是异步 watch，等它加载完再宣布就绪
	sleep 2
	log "已写入临时路由 ${ROUTE_FILE}（/ui → 127.0.0.1:${ui_port}，其余 → 127.0.0.1:${DEV_PORT}；route-off 即回退）"
}

cmd_route_off() {
	docker ps --format '{{.Names}}' | grep -qx "$TRAEFIK_CONTAINER" || return 0
	docker exec -i "$TRAEFIK_CONTAINER" sh -c "rm -f '${ROUTE_FILE}'" 2>/dev/null || true
	log "已删除临时路由，流量回到平台上已安装的应用"
}

# 容器健康状态；没有 healthcheck 或容器不在时报 none。
container_health() {
	docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}' "$CONTAINER" 2>/dev/null || echo none
}

wait_healthy() {
	local timeout="${VOS_DEV_HEALTH_TIMEOUT:-300}" waited=0 status="" state=""
	while [ "$waited" -lt "$timeout" ]; do
		state="$(docker inspect -f '{{.State.Status}}' "$CONTAINER" 2>/dev/null || echo missing)"
		case "$state" in
		missing)
			printf '\n'
			warn "容器 ${CONTAINER} 不存在，启动失败"
			return 1
			;;
		exited | dead)
			printf '\n'
			warn "容器 ${CONTAINER} 已退出（入口脚本报错），看日志：docker logs ${CONTAINER}"
			return 1
			;;
		esac
		status="$(container_health)"
		if [ "$status" = "healthy" ]; then
			printf '\n'
			return 0
		fi
		printf '\r\033[36m[vos-dev]\033[0m 等待 dev 容器就绪（首次要编译 garm）… %ss/%ss [%s]' \
			"$waited" "$timeout" "$status"
		sleep 5
		waited=$((waited + 5))
	done
	printf '\n'
	return 1
}

# 前端产物是 go:embed 进二进制的。构建时没带平台前缀，SPA 就会去请求
# /ui/_app/...，Traefik 匹配不到，平台 iframe 白屏。
check_webui_assets() {
	# 前端走 dev server 时不看嵌入产物
	if [ "$WEB_ENABLED" = "1" ]; then
		return 0
	fi
	local idx="webapp/assets/index.html"
	if [ ! -f "$idx" ]; then
		warn "没有前端产物（$idx）：平台里会白屏。先跑 make build-webui"
		return 0
	fi
	grep -q "${BASE_PATH}/ui/" "$idx" ||
		warn "前端产物缺少平台前缀 ${BASE_PATH}（平台里会白屏）：先跑 make build-webui"
}

# 前端 HMR：宿主上跑 vite dev server，带平台前缀，API 反代到 dev 容器。
start_web() {
	if [ "$WEB_ENABLED" != "1" ]; then
		log "VOS_DEV_WEB=0：前端用二进制里的嵌入产物，改前端要 make build-webui"
		return 0
	fi
	command -v npm >/dev/null || die "缺少 npm：先装 Node.js（或 VOS_DEV_WEB=0 make vos-dev）"
	[ -d webapp/node_modules ] || die "webapp/node_modules 不存在：先 cd webapp && npm install"
	ensure_port_free "$WEB_PORT"

	log "启动前端 dev server（vite :${WEB_PORT}，日志 ${WEB_LOG}）"
	VITE_GARM_BASE_PATH="$BASE_PATH" \
		VITE_GARM_API_URL="http://127.0.0.1:${DEV_PORT}" \
		npm --prefix webapp run dev -- --port "$WEB_PORT" >"$WEB_LOG" 2>&1 &
	WEB_PID=$!

	local waited=0
	while [ "$waited" -lt 60 ]; do
		if curl -sf -o /dev/null -m 2 "http://127.0.0.1:${WEB_PORT}${BASE_PATH}/ui/"; then
			log "前端就绪（改 .svelte 立即生效，页面不整刷）"
			return 0
		fi
		if ! kill -0 "$WEB_PID" 2>/dev/null; then
			warn "vite 已退出，看日志：${WEB_LOG}"
			return 1
		fi
		sleep 2
		waited=$((waited + 2))
	done
	warn "vite 60s 内未就绪，看日志：${WEB_LOG}"
	return 1
}

stop_web() {
	if [ -n "${WEB_PID:-}" ] && kill -0 "$WEB_PID" 2>/dev/null; then
		kill "$WEB_PID" 2>/dev/null || true
		wait "$WEB_PID" 2>/dev/null || true
	fi
	WEB_PID=""
	# npm 会再拉起子进程，按命令行兜底
	pkill -f "vite dev --host 0.0.0.0 --port ${WEB_PORT}" 2>/dev/null || true
	return 0
}

cleanup_on_exit() {
	local rc=$?
	# 收尾期间忽略重复信号，避免 compose down 中途被杀
	trap '' INT TERM
	printf '\n'
	log "退出联调：撤路由 + 停前端 + 停容器"
	set +e
	cmd_route_off
	stop_web
	compose down
	return "$rc"
}

cmd_up() {
	command -v docker >/dev/null || die "缺少 docker"
	[ -f "$ROUTE_TMPL" ] || die "缺少路由模板 $ROUTE_TMPL"
	check_webui_assets
	ensure_port_free "$DEV_PORT"
	prepare_provider
	maybe_add_lxd

	if [ "${VOS_DEV_DEBUG:-0}" = "1" ]; then
		log "启动 dev 容器（debug 模式：dlv 在 127.0.0.1:${DELVE_PORT}，改代码需手动重启）"
	else
		log "启动 dev 容器（热重载：保存 .go 即重编译，日志就是 garm 日志）"
	fi
	compose up -d --build

	if ! wait_healthy; then
		warn "dev 容器未就绪，**未**切换平台路由（平台仍走已安装应用）"
		warn "容器保留运行以便排查：make vos-dev-logs；修好后重跑 make vos-dev"
		exit 1
	fi
	log "dev 容器健康（http://127.0.0.1:${DEV_PORT}${BASE_PATH}/api/）"

	if ! start_web; then
		warn "前端 dev server 没起来：/ui 回落到二进制里的嵌入产物（无 HMR）"
		WEB_ENABLED=0
	fi

	cmd_route_on

	log "就绪：在 VOS 平台侧边栏打开 GARM（或刷新平台内 iframe）"
	log "  改 .go → air 重编译；改 webapp/src/*.svelte → HMR 立即生效"
	log "  数据目录：${DEV_DATA_DIR:-.dev-data}（sqlite + 渲染后的 config.toml）"
	if [ "${VOS_DEV_DEBUG:-0}" = "1" ]; then
		log "  断点：dlv --headless --listen=:${DELVE_PORT} --api-version=2 --accept-multiclient"
		log "        客户端 attach 到 localhost:${DELVE_PORT} 后下断点"
	fi
	log "  Ctrl-C 退出联调（撤路由 + 停前端 + 停容器）；下面开始跟随日志"
	printf '\n'

	trap 'cleanup_on_exit' EXIT
	compose logs -f "$SERVICE"
}

cmd_down() {
	set +e
	cmd_route_off
	stop_web
	compose down
	log "dev 容器已停止"
}

cmd_status() {
	echo "── dev 容器 ${CONTAINER} ──"
	local health="未运行"
	if docker inspect "$CONTAINER" >/dev/null 2>&1; then
		health="$(container_health)"
		compose ps "$SERVICE" 2>/dev/null
	fi
	echo "  健康：${health}"
	if [ "$health" = "healthy" ]; then
		echo "  API：http://127.0.0.1:${DEV_PORT}${BASE_PATH}/api/"
	fi
	echo "── provider ──"
	if [ -x "${PROVIDER_DIR}/${PROVIDER_BIN}" ]; then
		echo "  已就绪：${PROVIDER_DIR}/${PROVIDER_BIN}"
	else
		echo "  缺失（VOS ensure 会报 no such provider lxd）"
	fi
	if [ -S "$LXD_SOCKET" ]; then
		echo "  LXD socket：${LXD_SOCKET}"
	else
		echo "  LXD socket 不存在：${LXD_SOCKET}"
	fi
	echo "── 前端 dev server :${WEB_PORT} ──"
	if port_in_use "$WEB_PORT"; then
		echo "  运行中（改 .svelte 走 HMR，日志 ${WEB_LOG}）"
	else
		echo "  未运行（/ui 走二进制里的嵌入产物，改前端要 make build-webui）"
	fi
	echo "── 临时 Traefik 路由 ${ROUTE_FILE} ──"
	docker exec -i "$TRAEFIK_CONTAINER" sh -c "test -f '${ROUTE_FILE}' && cat '${ROUTE_FILE}' || echo '  （未启用 → 平台走已安装应用）'" 2>/dev/null ||
		echo "  （traefik 容器不可用）"
}

case "${1:-up}" in
up) cmd_up ;;
down) cmd_down ;;
route-off) cmd_route_off ;;
status) cmd_status ;;
logs) compose logs -f "$SERVICE" ;;
-h | --help | help)
	awk 'NR > 1 { if ($0 ~ /^#/) print; else exit }' "$0"
	;;
*) die "未知子命令：$1（up|down|route-off|status|logs）" ;;
esac
