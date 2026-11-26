#!/bin/sh
# Bootstrap one runner from V_SCHEDULER_BACKEND_CONFIG_B64.
#
# V-Scheduler passes no volumes, no command and no other environment variable, so
# every bootstrap value GARM has for this instance arrives inside that one blob.
set -eu

config="$(printf '%s' "${V_SCHEDULER_BACKEND_CONFIG_B64:-e30=}" | base64 -d)"

# shortcut: flat string fields only; switch to jq if the config gains objects.
field() {
	printf '%s' "$config" | grep -o "\"$1\": *\"[^\"]*\"" | head -n1 | sed -e 's/.*: *"//' -e 's/"$//'
}

metadata_url="$(field metadata_url)"
callback_url="$(field callback_url)"
instance_token="$(field instance_token)"
if [ -z "$metadata_url" ] || [ -z "$instance_token" ]; then
	echo "V_SCHEDULER_BACKEND_CONFIG_B64 has no metadata_url or instance_token" >&2
	exit 1
fi

# The GitHub base image ships curl, the Gitea one busybox wget. Both helpers keep
# the response in a temp file, so a failed retry never emits partial bytes.
fetch() {
	url="$1"
	if command -v curl >/dev/null 2>&1; then
		curl --fail --silent --show-error --retry 5 --retry-delay 5 --retry-connrefused \
			-H 'Accept: application/json' \
			-H "Authorization: Bearer ${instance_token}" \
			"$url"
		return
	fi
	out="$(mktemp)"
	tries=0
	until wget --quiet --timeout=20 --output-document="$out" \
		--header='Accept: application/json' \
		--header="Authorization: Bearer ${instance_token}" \
		"$url"; do
		tries=$((tries + 1))
		if [ "$tries" -ge 5 ]; then
			rm -f "$out"
			return 1
		fi
		sleep 5
	done
	cat "$out"
	rm -f "$out"
}

post() {
	url="$1"
	body="$2"
	if command -v curl >/dev/null 2>&1; then
		curl --fail --silent --show-error --retry 5 --retry-delay 5 --retry-connrefused \
			-H 'Accept: application/json' \
			-H "Authorization: Bearer ${instance_token}" \
			--data "$body" "$url" >/dev/null
		return
	fi
	tries=0
	until wget --quiet --timeout=20 --output-document=/dev/null \
		--header='Accept: application/json' \
		--header="Authorization: Bearer ${instance_token}" \
		--post-data="$body" "$url"; do
		tries=$((tries + 1))
		if [ "$tries" -ge 5 ]; then
			return 1
		fi
		sleep 5
	done
}

# GARM learns that the runner bootstrapped from this callback, not from the
# provider. Without it the instance stays pending and gets recycled. The status
# machine only allows pending -> installing -> idle.
notify_idle() {
	agent_id="$1"
	status_url="${callback_url%/}/status"
	# A missed callback must not kill the runner.
	post "$status_url" '{"status":"installing","message":"runner registered"}' || true
	payload="{\"status\":\"idle\",\"message\":\"runner container is up\""
	if [ -n "$agent_id" ]; then
		payload="$payload,\"agent_id\":$agent_id"
	fi
	payload="$payload}"
	post "$status_url" "$payload" || true
}

if [ -e /home/runner/run.sh ]; then
	# GitHub: GARM generated the JIT files, run.sh only reads those three.
	# The actions-runner image keeps the runner checkout in /home/runner.
	cd /home/runner
	fetch "${metadata_url}/credentials/runner" >.runner
	fetch "${metadata_url}/credentials/credentials" >.credentials
	fetch "${metadata_url}/credentials/credentials_rsaparams" >.credentials_rsaparams
	notify_idle ""
	exec ./run.sh
fi

# Gitea: no JIT config, so register with the one-time token. Same calls as
# internal/templates/userdata/gitea_linux_userdata.tmpl. The image's own run.sh
# registers when GITEA_RUNNER_REGISTRATION_TOKEN is set.
token="$(fetch "${metadata_url}/runner-registration-token/" | sed -e 's/^"//' -e 's/"$//')"
export GITEA_INSTANCE_URL="$(field repo_url)"
export GITEA_RUNNER_REGISTRATION_TOKEN="$token"
export GITEA_RUNNER_NAME="$(field runner_name)"
export GITEA_RUNNER_LABELS="$(field labels)"
# One job per container, like every other GARM runner.
export GITEA_RUNNER_EPHEMERAL=1

if [ -x /usr/local/bin/run.sh ]; then
	/sbin/tini -- /usr/local/bin/run.sh &
else
	gitea-runner daemon &
fi
runner_pid=$!

tries=0
while [ ! -s /data/.runner ] && [ "$tries" -lt 90 ]; do
	sleep 2
	tries=$((tries + 1))
done
if [ -s /data/.runner ]; then
	notify_idle "$(sed -n 's/.*"id": *\([0-9][0-9]*\).*/\1/p' /data/.runner | head -n1)"
fi

wait "$runner_pid"

# GARM reaps the instance when the runner disappears from the forge. Exiting here
# would make --restart unless-stopped restart the container at once, and the
# registration token is already used, so the restart can only fail. Stay alive.
exec sleep 2147483647
