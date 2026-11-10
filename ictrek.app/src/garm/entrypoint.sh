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

exec /bin/garm --config "${CONF_DIR}/config.toml"
