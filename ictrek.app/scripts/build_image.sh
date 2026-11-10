#!/usr/bin/env bash
# 构建并推送 garm 镜像。推送用 manifest list，tag 取版本号的 <DATE>.<GITSHA> 两段。
# 本地构建（--no-push）的 tag 带架构前缀，由 image-ref.sh 生成。
#
# 用法：build_image.sh [--version <version>] [--no-push] [--no-feishu]
#   --no-push    只在本机构建并加载镜像，不推送仓库
#   --no-feishu  推送成功后跳过写入飞书发布表
# 环境：
#   PACKAGE_VERSION  版本覆盖，未指定时由 version.sh 生成
#   IMAGE_REPO       镜像仓库前缀，默认 swr.cn-southwest-2.myhuaweicloud.com/ictrek/garm
#   PLATFORMS        推送时的构建平台，默认 linux/amd64,linux/arm64
#   BUILDER          buildx builder 名称，默认用当前默认 builder
#   FEISHU_CONFIG_FILE  飞书凭据文件，默认 ~/.feishu.json
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
app_dir="$(cd "${script_dir}/.." && pwd)"
repo_root="$(git -C "${script_dir}" rev-parse --show-toplevel)"

IMAGE_REPO="${IMAGE_REPO:-swr.cn-southwest-2.myhuaweicloud.com/ictrek/garm}"
PLATFORMS="${PLATFORMS:-linux/amd64,linux/arm64}"
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
    --version) PACKAGE_VERSION="${2:-}"; shift 2 ;;
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
  image_tag="${BASH_REMATCH[1]}.${BASH_REMATCH[3]}"
else
  eval "$("${script_dir}/version.sh")"
  version="${VERSION}"
  image_tag="${IMAGE_TAG}"
  date_part="${image_tag%%.*}"
fi

ref="$(git rev-parse HEAD)"
builder_arg=()
mirror_args=()
[[ -z "${APK_MIRROR}" ]] || mirror_args+=(--build-arg "APK_MIRROR=${APK_MIRROR}")
[[ -z "${NPM_REGISTRY}" ]] || mirror_args+=(--build-arg "NPM_REGISTRY=${NPM_REGISTRY}")
[[ -z "${GITHUB_PROXY}" ]] || mirror_args+=(--build-arg "GITHUB_PROXY=${GITHUB_PROXY}")

if (( no_push )); then
  # 单平台才能 --load，本地 tag 不带仓库前缀，避免误以为镜像已推送。
  # tag 带架构前缀，便于在 docker images 中区分 amd 与 arm。
  image="$("${script_dir}/image-ref.sh" --version "${version}")"
  platform_args=()
  output_args=(--load)
  echo "build_image.sh: building ${image} for the local platform as ${version}"
else
  image="${IMAGE_REPO}:${image_tag}"
  platform_args=(--platform "${PLATFORMS}")
  output_args=(--push)
  [[ -z "${BUILDER}" ]] || builder_arg=(--builder "${BUILDER}")
  echo "build_image.sh: building ${image} for ${PLATFORMS} as ${version}"
fi

docker buildx build "${builder_arg[@]}" \
  "${platform_args[@]}" \
  "${mirror_args[@]}" \
  --build-arg "GARM_REF=${ref}" \
  --build-arg "GARM_BASE_PATH=${BASE_PATH}" \
  --tag "${image}" \
  "${output_args[@]}" \
  .

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
  docker manifest inspect "${image}" >/dev/null
  echo "build_image.sh: pushed ${image}"
  if (( no_feishu )); then
    echo "build_image.sh: Feishu sheet update skipped (--no-feishu)"
  else
    command -v python3 >/dev/null 2>&1 || die "python3 is required to update the Feishu sheet"
    case "${PLATFORMS}" in
      *amd64*) record_sheet "AMD_with_cuda" "amd_v${version//-/_}" ;;
    esac
    case "${PLATFORMS}" in
      *arm64*) record_sheet "ARM_without_cuda" "arm_v${version//-/_}" ;;
    esac
  fi
fi
