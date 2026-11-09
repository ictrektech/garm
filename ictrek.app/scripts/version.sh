#!/usr/bin/env bash
# 生成 VOS 包版本，输出 KEY=VALUE。
#
# 格式：<核心版本>-<DATE>.<N>.<GITSHA>，示例 0.2.1-20261109.1.158fe569
#   <核心版本>  取 HEAD 上最近的稳定上游 tag，去掉 v
#   <DATE>      Asia/Shanghai 的 YYYYMMDD
#   <N>         远端同名日期 tag 的最大序号加一；失败不消耗序号
#   <GITSHA>    git-sha.sh 输出
#
# 用法：version.sh [--offline]
#   --offline  改用本地 tag 计算 <N>，用于无网络时预览，不用于正式发布
set -euo pipefail

offline=0
for arg in "$@"; do
  case "${arg}" in
    --offline) offline=1 ;;
    -h|--help) sed -n '2,14p' "$0"; exit 0 ;;
    *) echo "version.sh: unknown argument: ${arg}" >&2; exit 1 ;;
  esac
done

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(git -C "${script_dir}" rev-parse --show-toplevel)"

core="$(git -C "${repo_root}" describe --tags --abbrev=0 \
  --match 'v[0-9]*' --exclude '*-*' HEAD | sed 's/^v//')"
if ! [[ "${core}" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "version.sh: bad core version from git describe: ${core}" >&2
  exit 1
fi

date="$(TZ='Asia/Shanghai' date +%Y%m%d)"
sha="$("${script_dir}/git-sha.sh")"
if [[ "${sha}" == "unknown" ]]; then
  echo "version.sh: cannot resolve HEAD sha" >&2
  exit 1
fi

core_re="${core//./\\.}"
if (( offline )); then
  last="$(git -C "${repo_root}" tag -l "v${core}-${date}.*" \
    | sed -nE "s#^v${core_re}-${date}\.([0-9]+)\..*#\1#p" | sort -n | tail -1)"
else
  last="$(git -C "${repo_root}" ls-remote --tags origin "refs/tags/v${core}-${date}.*" \
    | sed -nE "s#.*refs/tags/v${core_re}-${date}\.([0-9]+)\..*#\1#p" | sort -n | tail -1)"
fi
n=$(( ${last:-0} + 1 ))

version="${core}-${date}.${n}.${sha}"
image_tag="${date}.${sha}"

echo "CORE=${core}"
echo "DATE=${date}"
echo "N=${n}"
echo "SHA=${sha}"
echo "VERSION=${version}"
echo "IMAGE_TAG=${image_tag}"
