#!/bin/sh
# garm dev 容器入口：用 VOS 交付里的模板渲染 config.toml，再启动热重载。
#
# 与 ictrek.app/src/garm/entrypoint.sh 的差异：
#   - provider 二进制由宿主机通过 /opt/garm/providers.d 挂载进来。没有该二进制时
#     截掉 [[provider]] 段，否则 garm 启动校验会失败；有则按交付脚本生成配置。
#   - 末尾 exec air（或 dlv），而不是 /bin/garm
#
# VOS_DEV_DEBUG=1 时改为编译带调试符号的二进制，用 dlv 在 :2345 等客户端连接。
set -eu

DATA_DIR=/data
SECRETS="${DATA_DIR}/secrets.env"
PROVIDER_BIN=/opt/garm/providers.d/garm-provider-lxd

mkdir -p "${DATA_DIR}"

if [ ! -f "${SECRETS}" ]; then
	umask 077
	{
		echo "JWT_SECRET=$(head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n')"
		echo "DB_PASSPHRASE=$(head -c 16 /dev/urandom | od -An -tx1 | tr -d ' \n')"
	} >"${SECRETS}"
fi
. "${SECRETS}"

BASE_PATH="$(cat /etc/garm/base-path)"
LOG_LEVEL="${GARM_LOG_LEVEL:-debug}"
LXD_SOCKET="${GARM_LXD_SOCKET_PATH:-/var/snap/lxd/common/lxd/unix.socket}"
LXD_INSTANCE_TYPE="${GARM_LXD_INSTANCE_TYPE:-container}"
LXD_PROJECT="${GARM_LXD_PROJECT:-default}"

# 与 ictrek.app/src/garm/entrypoint.sh 保持一致：每次启动重写，改环境变量即生效。
if [ -x "${PROVIDER_BIN}" ]; then
	cat >"${DATA_DIR}/garm-provider-lxd.toml" <<EOF
unix_socket_path = "${LXD_SOCKET}"
include_default_profile = false
instance_type = "${LXD_INSTANCE_TYPE}"
secure_boot = false
project_name = "${LXD_PROJECT}"
url = ""
client_certificate = ""
client_key = ""
tls_server_certificate = ""

[image_remotes]
  [image_remotes.ubuntu]
  addr = "https://cloud-images.ubuntu.com/releases"
  public = true
  protocol = "simplestreams"
  skip_verify = false
  [image_remotes.ubuntu_daily]
  addr = "https://cloud-images.ubuntu.com/daily"
  public = true
  protocol = "simplestreams"
  skip_verify = false
  [image_remotes.images]
  addr = "https://images.lxd.canonical.com"
  public = true
  protocol = "simplestreams"
  skip_verify = false
EOF
fi

sed \
	-e "s|__BASE_PATH__|${BASE_PATH}|g" \
	-e "s|__JWT_SECRET__|${JWT_SECRET}|g" \
	-e "s|__DB_PASSPHRASE__|${DB_PASSPHRASE}|g" \
	-e "s|__LOG_LEVEL__|${LOG_LEVEL}|g" \
	/etc/garm/config.toml.tpl >"${DATA_DIR}/config.toml.tmp"

if [ -x "${PROVIDER_BIN}" ]; then
	mv "${DATA_DIR}/config.toml.tmp" "${DATA_DIR}/config.toml"
	PROVIDER_STATE="lxd（${PROVIDER_BIN}）"
else
	# 没有 provider 二进制时，[[provider]] 段会让启动校验失败
	sed '/^\[\[provider\]\]/,$d' "${DATA_DIR}/config.toml.tmp" >"${DATA_DIR}/config.toml"
	rm -f "${DATA_DIR}/config.toml.tmp"
	PROVIDER_STATE="无（VOS ensure 会报 no such provider lxd）"
fi

echo "[garm-dev] config: ${DATA_DIR}/config.toml (base_path=${BASE_PATH}, log_level=${LOG_LEVEL})"
echo "[garm-dev] provider: ${PROVIDER_STATE}"

if [ "${VOS_DEV_DEBUG:-0}" = "1" ]; then
	echo "[garm-dev] 编译 -N -l 调试二进制，dlv 监听 :2345（无热重载）"
	go build -mod=vendor -gcflags="all=-N -l" \
		-tags osusergo,netgo,sqlite_omit_load_extension \
		-ldflags "-X github.com/cloudbase/garm/util/appdefaults.Version=dev" \
		-o /tmp/garm ./cmd/garm
	# 子命令 exec 紧跟二进制路径：放在 --continue 之后会被当成它的取值。
	exec dlv exec /tmp/garm \
		--headless --listen=:2345 --api-version=2 --accept-multiclient \
		--check-go-version=false --continue \
		-- --config "${DATA_DIR}/config.toml"
fi

exec air -c /app/docker/dev/air.toml
