#!/bin/sh
# Render the garm config from the packaged template, then exec the server.
# Secrets are generated once and kept in the app storage.
set -eu

DATA_DIR="/data"
CONF_DIR="${DATA_DIR}"
SECRETS="${CONF_DIR}/secrets.env"

mkdir -p "${CONF_DIR}"

if [ ! -f "${SECRETS}" ]; then
	umask 077
	{
		echo "JWT_SECRET=$(head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n')"
		echo "DB_PASSPHRASE=$(head -c 16 /dev/urandom | od -An -tx1 | tr -d ' \n')"
	} >"${SECRETS}"
fi
. "${SECRETS}"

BASE_PATH="$(cat /etc/garm/base-path)"
LOG_LEVEL="${GARM_LOG_LEVEL:-info}"
LXD_SOCKET="${GARM_LXD_SOCKET_PATH:-/var/snap/lxd/common/lxd/unix.socket}"
LXD_INSTANCE_TYPE="${GARM_LXD_INSTANCE_TYPE:-container}"
LXD_PROJECT="${GARM_LXD_PROJECT:-default}"

sed \
	-e "s|__BASE_PATH__|${BASE_PATH}|g" \
	-e "s|__JWT_SECRET__|${JWT_SECRET}|g" \
	-e "s|__DB_PASSPHRASE__|${DB_PASSPHRASE}|g" \
	-e "s|__LOG_LEVEL__|${LOG_LEVEL}|g" \
	/etc/garm/config.toml.tpl >"${CONF_DIR}/config.toml"

# Provider config is rewritten on every start so that changing the exposed
# settings takes effect without touching the image.
cat >"${CONF_DIR}/garm-provider-lxd.toml" <<EOF
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

# The V-Scheduler provider is listed whenever its binary ships with the image,
# so the web UI can store the service token. Once the config file exists it is
# the source of truth: the UI owns it, and this script only seeds a missing one.
SCHEDULER_PROVIDER_BIN="/opt/garm/providers.d/garm-provider-vscheduler"
SCHEDULER_JSON="${CONF_DIR}/garm-provider-vscheduler.json"

if [ -x "${SCHEDULER_PROVIDER_BIN}" ]; then
	if [ ! -f "${SCHEDULER_JSON}" ]; then
		(
			umask 077
			cat >"${SCHEDULER_JSON}" <<EOF
{
  "base_url": "${SCHEDULER_BASE_URL:-http://v-scheduler:38120/api/v1}",
  "token": "${SCHEDULER_TOKEN:-}",
  "container_port": ${SCHEDULER_CONTAINER_PORT:-8080},
  "image_tag": "${GARM_RUNNER_IMAGE_TAG:-}",
  "image_repository": "${GARM_RUNNER_IMAGE_REPO:-swr.cn-southwest-2.myhuaweicloud.com/ictrek/garm-runner}"
}
EOF
		)
	fi
	cat >>"${CONF_DIR}/config.toml" <<EOF

[[provider]]
  name = "vscheduler"
  provider_type = "external"
  description = "V-Scheduler managed nodes"
  [provider.external]
    interface_version = "v0.1.1"
    provider_executable = "${SCHEDULER_PROVIDER_BIN}"
    config_file = "${SCHEDULER_JSON}"
EOF
fi

exec /bin/garm --config "${CONF_DIR}/config.toml"
