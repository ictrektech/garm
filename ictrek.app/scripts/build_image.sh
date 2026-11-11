#!/usr/bin/env bash
# 构建并推送 garm 镜像。推送 tag 与飞书发布表的值完全相同，格式 <arch>_v<版本>。
# 一台构建机只构建一种架构：amd64 在 x86_64 主机构建，arm64 在 arm64 主机构建。
#
# 用法：build_image.sh [--version <version>] [--arch amd|arm] [--no-push] [--no-feishu]
#   --arch       目标架构，默认取本机架构；必须与本机架构一致
#   --no-push    只在本机构建并加载镜像，不推送仓库
#   --no-feishu  推送成功后跳过写入飞书发布表
# 环境：
#   PACKAGE_VERSION  版本覆盖，未指定时由 version.sh 生成
#   IMAGE_REPO       镜像仓库前缀，默认 swr.cn-southwest-2.myhuaweicloud.com/ictrek/garm
#   BUILDER          buildx builder 名称，默认用当前默认 builder
#   FEISHU_CONFIG_FILE  飞书凭据文件，默认 ~/.feishu.json
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
app_dir="$(cd "${script_dir}/.." && pwd)"
repo_root="$(git -C "${script_dir}" rev-parse --show-toplevel)"

IMAGE_REPO="${IMAGE_REPO:-swr.cn-southwest-2.myhuaweicloud.com/ictrek/garm}"
BUILDER="${BUILDER:-}"
PACKAGE_VERSION="${PACKAGE_VERSION:-}"
BASE_PATH="$(cat "${app_dir}/src/garm/base-path")"
# 可选：构建期源加速，留空时用官方源。
APK_MIRROR="${APK_MIRROR:-}"
NPM_REGISTRY="${NPM_REGISTRY:-}"
# 未设置时用公共代理。国内直连 github.com 常超时，单次克隆会等上百秒。
# 需要直连时显式传 GITHUB_PROXY= （空值）。
GITHUB_PROXY="${GITHUB_PROXY-https://gh-proxy.com}"
arch=""
no_push=0
no_feishu=0

die() { echo "build_image.sh: $*" >&2; exit 1; }

# 把本机架构的 tag 写入 ictrek.app/image-tags，安装包从该文件取镜像引用。
record_image_tag() {
  local key="$1" value="$2"
  local file="${app_dir}/image-tags" tmp
  tmp="$(mktemp)"
  if [[ -f "${file}" ]]; then
    grep -v "^${key}=" "${file}" > "${tmp}" || true
  fi
  printf '%s=%s\n' "${key}" "${value}" >> "${tmp}"
  LC_ALL=C sort "${tmp}" > "${file}"
  rm -f "${tmp}"
  echo "build_image.sh: recorded ${key}=${value} in ictrek.app/image-tags"
}

host_arch() {
  case "$(uname -m)" in
    x86_64|amd64) echo "amd" ;;
    aarch64|arm64) echo "arm" ;;
    *) die "unsupported machine: $(uname -m)" ;;
  esac
}

platform_arch() {
  case "$1" in
    amd) echo "amd64" ;;
    arm) echo "arm64" ;;
    *) die "unsupported arch: $1" ;;
  esac
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version) PACKAGE_VERSION="${2:-}"; shift 2 ;;
    --arch) arch="${2:-}"; shift 2 ;;
    --no-push) no_push=1; shift ;;
    --no-feishu) no_feishu=1; shift ;;
    -h|--help) sed -n '2,13p' "$0"; exit 0 ;;
    *) die "unknown argument: $1" ;;
  esac
done

cd "${repo_root}"

# 镜像内容来自工作区，有未提交改动时镜像与 <GITSHA> 不符。
# vendor/ 与 webapp/node_modules 不参与检查：前者由 go mod vendor 生成，
# 后者被 .gitignore 忽略，二者在 NFS 上会显著拖慢工作区扫描。
git diff --cached --quiet || die "worktree has staged changes"
git diff --quiet -- . ':(exclude)vendor' ':(exclude)webapp/node_modules' \
  || die "worktree has unstaged changes (vendor and node_modules excluded)"

if [[ -n "${PACKAGE_VERSION}" ]]; then
  [[ "${PACKAGE_VERSION}" =~ ^[0-9]+\.[0-9]+\.[0-9]+-([0-9]{8})\.([0-9]+)\.([0-9A-Za-z]+)$ ]] \
    || die "invalid version: ${PACKAGE_VERSION}"
  version="${PACKAGE_VERSION}"
  date_part="${BASH_REMATCH[1]}"
else
  eval "$("${script_dir}/version.sh")"
  version="${VERSION}"
  date_part="${DATE}"
fi

if [[ -z "${arch}" ]]; then
  arch="$(host_arch)"
fi
case "${arch}" in
  amd|arm) ;;
  *) die "invalid arch: ${arch} (use amd or arm)" ;;
esac
[[ "${arch}" == "$(host_arch)" ]] \
  || die "arch ${arch} does not match this machine ($(uname -m)); build each platform on its own host"
platform="linux/$(platform_arch "${arch}")"

# 推送 tag 与飞书发布表的值完全相同；本地构建用不带仓库前缀的 tag。
if (( no_push )); then
  image="$("${script_dir}/image-ref.sh" --version "${version}" --arch "${arch}")"
else
  image="$("${script_dir}/image-ref.sh" --version "${version}" --arch "${arch}" --repo "${IMAGE_REPO}")"
fi
tag="${image##*:}"

ref="$(git rev-parse HEAD)"
builder_arg=()
mirror_args=()
[[ -z "${APK_MIRROR}" ]] || mirror_args+=(--build-arg "APK_MIRROR=${APK_MIRROR}")
[[ -z "${NPM_REGISTRY}" ]] || mirror_args+=(--build-arg "NPM_REGISTRY=${NPM_REGISTRY}")
[[ -z "${GITHUB_PROXY}" ]] || mirror_args+=(--build-arg "GITHUB_PROXY=${GITHUB_PROXY}")
[[ -z "${BUILDER}" ]] || builder_arg=(--builder "${BUILDER}")

echo "build_image.sh: building ${image} for ${platform} as ${version}"

docker buildx build "${builder_arg[@]}" \
  --platform "${platform}" \
  "${mirror_args[@]}" \
  --provenance=false \
  --build-arg "GARM_REF=${ref}" \
  --build-arg "GARM_BASE_PATH=${BASE_PATH}" \
  --tag "${image}" \
  --load \
  .

if (( no_push )); then
  echo "build_image.sh: built ${image} (not pushed)"
  exit 0
fi

docker push "${image}"
# 先确认仓库里真的有该 tag，再写发布表。
docker manifest inspect "${image}" >/dev/null
echo "build_image.sh: pushed ${image}"

# 记录本机架构的 tag。安装包从这里取引用，只改 ictrek.app/ 时不用重建镜像。
record_image_tag "${arch}" "${tag}"

if (( no_feishu )); then
  echo "build_image.sh: Feishu sheet update skipped (--no-feishu)"
  echo "build_image.sh: commit ictrek.app/image-tags before publishing"
  exit 0
fi

# amd 与 arm 分别落在固定的工作表里，值就是推送 tag。
case "${arch}" in
  amd) sheet="AMD_with_cuda" ;;
  arm) sheet="ARM_without_cuda" ;;
esac
command -v python3 >/dev/null 2>&1 || die "python3 is required to update the Feishu sheet"
python3 "${script_dir}/feishu_sheet.py" \
  --sheet "${sheet}" \
  --component "garm" \
  --image "${IMAGE_REPO}" \
  --value "${tag}" \
  --date "${date_part}" \
  || die "cannot record ${tag} in sheet ${sheet}"

echo "build_image.sh: commit ictrek.app/image-tags before publishing"
