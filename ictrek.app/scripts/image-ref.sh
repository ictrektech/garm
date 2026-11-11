#!/usr/bin/env bash
# 生成 garm 镜像引用，tag 带架构前缀，与飞书发布表的值一致。
#
# 格式：<仓库>:<arch>_v<版本>，示例 swr.cn-southwest-2.myhuaweicloud.com/ictrek/garm:amd_v0.2.1_20261110.5.f827f4df
#   <arch>   amd 表示 x86_64，arm 表示 aarch64 或 arm64
#
# 用法：image-ref.sh --version <version> [--arch amd|arm] [--repo <仓库>]
#   --repo    镜像仓库前缀，默认 garm（本地构建用，不带仓库前缀）
# 环境：IMAGE_ARCH  可选，覆盖自动识别的架构
set -euo pipefail

version=""
arch="${IMAGE_ARCH:-}"
repo="garm"

die() { echo "image-ref.sh: $*" >&2; exit 1; }

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version) version="${2:-}"; shift 2 ;;
    --arch) arch="${2:-}"; shift 2 ;;
    --repo) repo="${2:-}"; shift 2 ;;
    -h|--help) sed -n '2,9p' "$0"; exit 0 ;;
    *) die "unknown argument: $1" ;;
  esac
done

[[ "${version}" =~ ^[0-9]+\.[0-9]+\.[0-9]+-[0-9]{8}\.[0-9]+\.[0-9A-Za-z]+$ ]] \
  || die "invalid version: ${version}"
[[ -n "${repo}" ]] || die "empty repo"

if [[ -z "${arch}" ]]; then
  case "$(uname -m)" in
    x86_64|amd64) arch="amd" ;;
    aarch64|arm64) arch="arm" ;;
    *) die "unsupported machine: $(uname -m)" ;;
  esac
fi
case "${arch}" in
  amd|arm) ;;
  *) die "invalid arch: ${arch}" ;;
esac

echo "${repo}:${arch}_v${version//-/_}"
