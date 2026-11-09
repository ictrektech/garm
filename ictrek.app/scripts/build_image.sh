#!/usr/bin/env bash
# 构建并推送 garm 镜像。镜像 tag 取版本号的 <DATE>.<GITSHA> 两段。
#
# 用法：build_image.sh [--version <version>]
# 环境：
#   PACKAGE_VERSION  版本覆盖，未指定时由 version.sh 生成
#   IMAGE_REPO       镜像仓库前缀，默认 swr.cn-southwest-2.myhuaweicloud.com/ictrek/garm
#   PLATFORMS        构建平台，默认 linux/amd64,linux/arm64
#   BUILDER          buildx builder 名称，默认用当前默认 builder
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
app_dir="$(cd "${script_dir}/.." && pwd)"
repo_root="$(git -C "${script_dir}" rev-parse --show-toplevel)"

IMAGE_REPO="${IMAGE_REPO:-swr.cn-southwest-2.myhuaweicloud.com/ictrek/garm}"
PLATFORMS="${PLATFORMS:-linux/amd64,linux/arm64}"
BUILDER="${BUILDER:-}"
PACKAGE_VERSION="${PACKAGE_VERSION:-}"
BASE_PATH="$(cat "${app_dir}/src/garm/base-path")"

die() { echo "build_image.sh: $*" >&2; exit 1; }

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version) PACKAGE_VERSION="${2:-}"; shift 2 ;;
    -h|--help) sed -n '2,10p' "$0"; exit 0 ;;
    *) die "unknown argument: $1" ;;
  esac
done

cd "${repo_root}"

# 镜像内容来自工作区，有未提交改动时镜像与 <GITSHA> 不符。
git diff --quiet && git diff --cached --quiet || die "worktree is not clean"

if [[ -n "${PACKAGE_VERSION}" ]]; then
  [[ "${PACKAGE_VERSION}" =~ ^[0-9]+\.[0-9]+\.[0-9]+-([0-9]{8})\.([0-9]+)\.([0-9A-Za-z]+)$ ]] \
    || die "invalid version: ${PACKAGE_VERSION}"
  version="${PACKAGE_VERSION}"
  image_tag="${BASH_REMATCH[1]}.${BASH_REMATCH[3]}"
else
  eval "$("${script_dir}/version.sh")"
  version="${VERSION}"
  image_tag="${IMAGE_TAG}"
fi

ref="$(git rev-parse HEAD)"
image="${IMAGE_REPO}:${image_tag}"
echo "build_image.sh: building ${image} for ${PLATFORMS} as ${version}"

builder_arg=()
[[ -z "${BUILDER}" ]] || builder_arg=(--builder "${BUILDER}")

docker buildx build "${builder_arg[@]}" \
  --platform "${PLATFORMS}" \
  --build-arg "GARM_REF=${ref}" \
  --build-arg "GARM_BASE_PATH=${BASE_PATH}" \
  --tag "${image}" \
  --push \
  .

docker manifest inspect "${image}" >/dev/null
echo "build_image.sh: pushed ${image}"
