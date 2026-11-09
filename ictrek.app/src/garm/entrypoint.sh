#!/bin/sh
# Render the garm config from the packaged template, then exec the server.
# Secrets are generated once and kept in the app storage.
set -eu

DATA_DIR="${VOS_APP_STORAGE_PATH:-/data}/garm"
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

sed \
	-e "s|__BASE_PATH__|${BASE_PATH}|g" \
	-e "s|__JWT_SECRET__|${JWT_SECRET}|g" \
	-e "s|__DB_PASSPHRASE__|${DB_PASSPHRASE}|g" \
	-e "s|__LOG_LEVEL__|${LOG_LEVEL}|g" \
	/etc/garm/config.toml.tpl >"${CONF_DIR}/config.toml"

exec /bin/garm --config "${CONF_DIR}/config.toml"
