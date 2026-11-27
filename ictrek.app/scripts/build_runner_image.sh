#!/usr/bin/env bash
# 构建 garm-runner 镜像。V-Scheduler provider（garm-provider-vscheduler）在节点上
# 跑的就是它。入口脚本与底包都在 contrib/garm-provider-vscheduler/runner-image/。
#
# 用法：build_runner_image.sh --target amd|arm --version <底包版本>
#                             [--runner-base gitea|github] [--no-push] [--no-feishu] [--force]
#       build_runner_image.sh --latest --version <底包版本> [--runner-base gitea|github]
#       build_runner_image.sh --list
#
# tag 是 <amd|arm>_v<底包版本>_<内容哈希8位>，例 arm_v6.0.1_3f9a1c2b。
# 内容哈希覆盖 Dockerfile、entrypoint.sh 与底包引用。仓库里其他代码怎么改都不影响
# 这个 tag，所以每次发版都可以无脑跑一遍：tag 已存在就直接跳过。
#
# 与 build_image.sh 的差异：本镜像没有 RUN 步骤，不要求本机架构匹配，
# 一台机器可以交叉构建另一个架构。构建后仍校验实际架构与 --target 一致。
#
# 推完镜像后要人工把 garm 池的 image 改成仓库名。provider 会按池的 os_arch 补上
# amd_ 或 arm_ 前缀的 tag（见 garm-provider-vscheduler 的 image_tag 配置）。
# 用 --list 查仓库里已有的 tag。
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "${script_dir}/../.." && pwd)"
image_dir="${repo_root}/contrib/garm-provider-vscheduler/runner-image"

IMAGE_REPO="${IMAGE_REPO:-swr.cn-southwest-2.myhuaweicloud.com/ictrek/garm-runner}"
target=""
BASE_VERSION=""
runner_base="gitea"
no_push=0
no_feishu=0
force=0
latest=0
list_tags=0

die() { echo "build_runner_image.sh: $*" >&2; exit 1; }

while [[ $# -gt 0 ]]; do
  case "$1" in
    --target) target="${2:-}"; shift 2 ;;
    --version) BASE_VERSION="${2:-}"; shift 2 ;;
    --runner-base) runner_base="${2:-}"; shift 2 ;;
    --no-push) no_push=1; shift ;;
    --no-feishu) no_feishu=1; shift ;;
    --force) force=1; shift ;;
    --latest) latest=1; shift ;;
    --list) list_tags=1; shift ;;
    -h|--help) awk 'NR>1 && /^#/ {sub(/^# ?/, ""); print; next} NR>1 {exit}' "$0"; exit 0 ;;
    *) die "unknown argument: $1" ;;
  esac
done

cd "${repo_root}"

# --list 只读仓库，不构建。凭据取自 docker login 的配置。
if (( list_tags )); then
  python3 - "${IMAGE_REPO}" <<'PY'
import base64, json, os, sys, urllib.error, urllib.parse, urllib.request

repo = sys.argv[1]
host, _, path = repo.partition("/")
config_dir = os.environ.get("DOCKER_CONFIG") or os.path.join(os.path.expanduser("~"), ".docker")
config_file = os.path.join(config_dir, "config.json")
with open(config_file, encoding="utf-8") as handle:
    auth = json.load(handle).get("auths", {}).get(host, {}).get("auth")
if not auth:
    raise SystemExit("registry_tags: %s has no credentials for %s" % (config_file, host))
user, _, password = base64.b64decode(auth).decode().partition(":")

# Ask the registry how to authenticate instead of hardcoding one vendor.
realm = service = None
try:
    with urllib.request.urlopen("https://%s/v2/" % host, timeout=30) as response:
        service = "" if response.status == 200 else None
except urllib.error.HTTPError as exc:
    header = exc.headers.get("Www-Authenticate", "")
    fields = {}
    if header.lower().startswith("bearer "):
        for part in header[7:].split(","):
            key, _, value = part.strip().partition("=")
            fields[key.strip()] = value.strip().strip('"')
    realm, service = fields.get("realm"), fields.get("service")

if realm:
    query = urllib.parse.urlencode({"service": service or "", "scope": "repository:%s:pull" % path})
    request = urllib.request.Request(realm + "?" + query)
    request.add_header("Authorization", "Basic " + base64.b64encode(("%s:%s" % (user, password)).encode()).decode())
    with urllib.request.urlopen(request, timeout=30) as response:
        payload = json.load(response)
    token = payload.get("token") or payload.get("access_token")
else:
    token = base64.b64encode(("%s:%s" % (user, password)).encode()).decode()

request = urllib.request.Request("https://%s/v2/%s/tags/list?n=1000" % (host, path))
request.add_header("Authorization", ("Bearer " if realm else "Basic ") + token)
with urllib.request.urlopen(request, timeout=30) as response:
    tags = [tag for tag in json.load(response).get("tags") or [] if tag]

if not tags:
    raise SystemExit("registry_tags: %s has no tags" % repo)

# 有版本号的在前，按版本降序；临时标签（latest、旧的调试 tag）放最后。
versioned = sorted((t for t in tags if "_v" in t), key=lambda t: t.split("_v", 1)[1], reverse=True)
others = sorted((t for t in tags if "_v" not in t), reverse=True)
for tag in versioned + others:
    print(tag)
PY
  exit 0
fi

[[ "${BASE_VERSION}" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] \
  || die "pass --version <底包版本>，例如 6.0.1"

# 底包版本由 --version 表达，脚本不解析镜像，升级底包时人工改 --version。
case "${runner_base}" in
  gitea) base_image="docker.io/gitea/runner:latest" ;;
  github) base_image="ghcr.io/actions/actions-runner:latest" ;;
  *) die "--runner-base must be gitea or github" ;;
esac

# 内容哈希只覆盖镜像的实际输入。garm 的 Go 代码变了不会改变它，
# 所以「内容没变就跳过」是准的。底包用 :latest 引用，上游换了内容时
# 需要人工加 --force。
content_hash() {
  { cat "${image_dir}/Dockerfile" "${image_dir}/entrypoint.sh"; printf '%s' "${base_image}"; } \
    | sha256sum | cut -c1-8
}
hash="$(content_hash)"

# --latest 只动 tag，不构建：把同版本的 amd 与 arm 清单合到一个 latest 下。
if (( latest )); then
  if (( no_push )); then
    die "--latest 会改仓库里的 latest，去掉 --no-push"
  fi
  tags=()
  missing=()
  for arch in amd arm; do
    candidate="${IMAGE_REPO}:${arch}_v${BASE_VERSION}_${hash}"
    if docker buildx imagetools inspect "${candidate}" >/dev/null 2>&1; then
      tags+=("${candidate}")
    else
      missing+=("${arch}")
    fi
  done
  (( ${#tags[@]} > 0 )) || die "no tag for ${BASE_VERSION}_${hash}; check --runner-base (now ${runner_base})"
  if (( ${#missing[@]} > 0 )); then
    echo "build_runner_image.sh: 缺 ${missing[*]} 的该版本 tag，latest 将是单架构" >&2
  fi
  docker buildx imagetools create --tag "${IMAGE_REPO}:latest" "${tags[@]}"
  echo "build_runner_image.sh: ${IMAGE_REPO}:latest -> ${tags[*]}"
  exit 0
fi

case "${target}" in
  amd) platform=amd64; sheet=AMD_with_cuda ;;
  arm) platform=arm64; sheet=ARM_without_cuda ;;
  *) die "select --target amd or arm" ;;
esac

image_tag="${target}_v${BASE_VERSION}_${hash}"
published="${IMAGE_REPO}:${image_tag}"
image="${published}"
if (( no_push )); then
  image="garm-runner:${image_tag}"
elif (( ! force )) && docker buildx imagetools inspect "${published}" >/dev/null 2>&1; then
  echo "build_runner_image.sh: ${published} 已存在，内容未变，跳过（要重建加 --force）"
  if (( ! no_feishu )); then
    # 上一次可能推送成功但写表失败。给出补登记命令，别让表里少一条。
    echo "build_runner_image.sh: 补登记：python3 ${script_dir}/feishu_sheet.py \\"
    echo "  --sheet ${sheet} --component garm-runner --image ${IMAGE_REPO} \\"
    echo "  --value ${image_tag} --date $(date +%Y%m%d)"
  fi
  exit 0
fi

# --force rewrites a tag that nodes may already have cached.
if (( force && ! no_push )); then
  echo "build_runner_image.sh: --force 覆盖同名 tag；已缓存该 tag 的节点不会更新，优先改 --version" >&2
fi

docker buildx build \
  --platform "linux/${platform}" --load --provenance=false --sbom=false \
  --build-arg "RUNNER_BASE=${base_image}" \
  --label "org.opencontainers.image.version=${BASE_VERSION}" \
  --label "org.opencontainers.image.revision=$(git rev-parse --short=8 HEAD 2>/dev/null || echo unknown)" \
  --label "org.opencontainers.image.base.name=${base_image}" \
  --tag "${image}" "${image_dir}"

actual="$(docker image inspect "${image}" --format '{{.Architecture}}')"
[[ "${actual}" == "${platform}" ]] || die "built ${actual}, expected ${platform}"
entrypoint="$(docker image inspect "${image}" --format '{{json .Config.Entrypoint}}')"
[[ "${entrypoint}" == '["/entrypoint.sh"]' ]] || die "unexpected entrypoint: ${entrypoint}"

# 只登记，不作为发版门禁：runner 镜像的重建节奏与 garm 发版不同，
# 按日期核对会让没重建 runner 的发版失败。
record_sheet() {
  python3 "${script_dir}/feishu_sheet.py" \
    --sheet "${sheet}" \
    --component "garm-runner" \
    --image "${IMAGE_REPO}" \
    --value "${image_tag}" \
    --date "$(date +%Y%m%d)" \
    || die "cannot record ${image_tag} in sheet ${sheet}"
}

if (( no_push )); then
  echo "build_runner_image.sh: built ${image} (not pushed)"
else
  docker push "${image}"
  docker manifest inspect "${image}" >/dev/null
  echo "build_runner_image.sh: pushed ${published}"
  if (( ! no_feishu )); then
    record_sheet
  fi
  echo "build_runner_image.sh: 池的 image 填 ${IMAGE_REPO}，provider 会按 os_arch 补 ${target}_v${BASE_VERSION}_${hash}"
fi
