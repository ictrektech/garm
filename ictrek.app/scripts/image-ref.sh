#!/usr/bin/env bash
# 生成本地构建的镜像引用，按 CPU 架构加前缀，避免同一台机器上的 amd/arm 镜像混淆。
#
# 格式：garm:<arch>_v<版本>，示例 garm:amd_v0.2.1_20261110.1.e8478193
#   <arch>   amd 表示 x86_64，arm 表示 aarch64 或 arm64
#
# 用法：image-ref.sh --version <version> [--arch amd|arm]
# 环境：IMAGE_ARCH  可选，覆盖自动识别的架构
set -euo pipefail

version=""
arch="${IMAGE_ARCH:-}"

die() { echo "image-ref.sh: $*" >&2; exit 1; }

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version) version="${2:-}"; shift 2 ;;
    --arch) arch="${2:-}"; shift 2 ;;
    -h|--help) sed -n '2,9p' "$0"; exit 0 ;;
    *) die "unknown argument: $1" ;;
  esac
done

[[ "${version}" =~ ^[0-9]+\.[0-9]+\.[0-9]+-[0-9]{8}\.[0-9]+\.[0-9A-Za-z]+$ ]] \
  || die "invalid version: ${version}"

if [[ -z "${arch}" ]]; then
  case "$(uname -m)" in
    x86_64|amd64) arch="amd" ;;
    aarch64|arm64) arch="arm" ;;
    *) die "unsupported machine: $(uname -m)" ;;
  esac
fi

echo "garm:${arch}_v${version//-/_}"
