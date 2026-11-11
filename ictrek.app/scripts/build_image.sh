#!/usr/bin/env bash
# 在对应架构的构建机上运行。每个 profile 发布独立镜像到 SWR。
# 用法：build_image.sh --target amd|arm --version <version> [--no-push] [--no-feishu]
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
app_dir="$(cd "${script_dir}/.." && pwd)"
repo_root="$(cd "${app_dir}/.." && pwd)"

IMAGE_REPO="${IMAGE_REPO:-swr.cn-southwest-2.myhuaweicloud.com/ictrek/garm}"
target=""
BUILDER="${BUILDER:-}"
PACKAGE_VERSION="${PACKAGE_VERSION:-}"
BASE_PATH="$(cat "${app_dir}/src/garm/base-path")"
# 可选：构建期源加速，留空时用官方源。
APK_MIRROR="${APK_MIRROR:-}"
NPM_REGISTRY="${NPM_REGISTRY:-}"
# 未设置时用公共代理。国内直连 github.com 常超时，单次克隆会等上百秒。
# 需要直连时显式传 GITHUB_PROXY= （空值）。
GITHUB_PROXY="${GITHUB_PROXY-https://gh-proxy.com}"
no_push=0
no_feishu=0

die() { echo "build_image.sh: $*" >&2; exit 1; }

while [[ $# -gt 0 ]]; do
  case "$1" in
    --target) target="${2:-}"; shift 2 ;;
    --version) PACKAGE_VERSION="${2:-}"; shift 2 ;;
    --no-push) no_push=1; shift ;;
    --no-feishu) no_feishu=1; shift ;;
    -h|--help) sed -n '2,13p' "$0"; exit 0 ;;
    *) die "unknown argument: $1" ;;
  esac
done

cd "${repo_root}"

case "${target}:$(uname -m)" in
  amd:x86_64|amd:amd64) platform=amd64; sheet=AMD_with_cuda ;;
  arm:aarch64|arm:arm64) platform=arm64; sheet=ARM_without_cuda ;;
  *) die "select --target amd or arm on a matching native build host" ;;
esac

[[ "${PACKAGE_VERSION}" =~ ^[0-9]+\.[0-9]+\.[0-9]+-([0-9]{8})\.([0-9]+)\.([0-9A-Za-z]+)$ ]] \
  || die "pass --version from the local release checkout"
date_part="${BASH_REMATCH[1]}"
ref="${BASH_REMATCH[3]}"
version="${PACKAGE_VERSION}"
image_tag="${target}_v${version//-/_}"
builder_arg=()
mirror_args=()
[[ -z "${APK_MIRROR}" ]] || mirror_args+=(--build-arg "APK_MIRROR=${APK_MIRROR}")
[[ -z "${NPM_REGISTRY}" ]] || mirror_args+=(--build-arg "NPM_REGISTRY=${NPM_REGISTRY}")
[[ -z "${GITHUB_PROXY}" ]] || mirror_args+=(--build-arg "GITHUB_PROXY=${GITHUB_PROXY}")

image="${IMAGE_REPO}:${image_tag}"
if (( no_push )); then
  image="garm:${image_tag}"
fi
[[ -z "${BUILDER}" ]] || builder_arg=(--builder "${BUILDER}")

docker buildx build "${builder_arg[@]}" "${mirror_args[@]}" \
  --platform "linux/${platform}" --load --provenance=false --sbom=false \
  --build-arg "GARM_REF=${ref}" \
  --build-arg "GARM_SOURCE_VERSION=${version}" \
  --build-arg "GARM_BASE_PATH=${BASE_PATH}" \
  --tag "${image}" .

actual="$(docker image inspect "${image}" --format '{{.Architecture}}')"
[[ "${actual}" == "${platform}" ]] || die "built ${actual}, expected ${platform}"
docker run --rm --entrypoint /bin/sh "${image}" -c '/bin/garm --help >/dev/null'

# 把镜像 tag 写进 VOS 发布表。amd 与 arm 分别落在固定的工作表里，
# 写入失败直接退出，避免发布记录与实际镜像不一致。
record_sheet() {
  local sheet="$1" value="$2"
  python3 "${script_dir}/feishu_sheet.py" \
    --sheet "${sheet}" \
    --component "garm" \
    --image "${IMAGE_REPO}" \
    --value "${value}" \
    --date "${date_part}" \
    || die "cannot record ${value} in sheet ${sheet}"
}

if (( no_push )); then
  echo "build_image.sh: built ${image} (not pushed)"
else
  docker push "${image}"
  docker manifest inspect "${image}" >/dev/null
  echo "build_image.sh: pushed ${image}"
  if (( ! no_feishu )); then
    record_sheet "${sheet}" "${image_tag}"
  fi
fi
