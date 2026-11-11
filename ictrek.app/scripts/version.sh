#!/usr/bin/env bash
# 生成 VOS 包版本，输出 KEY=VALUE。
#
# 格式：<核心版本>-<DATE>.<N>.<GITSHA>，示例 0.2.1-20261109.1.158fe569
#   <核心版本>  取 HEAD 上最近的稳定上游 tag，去掉 v
#   <DATE>      Asia/Shanghai 的 YYYYMMDD
#   <N>         远端同名日期 tag 的最大序号加一；失败不消耗序号
#   <GITSHA>    最近一次改动 ictrek.app/ 以外路径的提交，git-sha.sh 输出。
#               只改 ictrek.app/ 时它停在上一个代码提交，与镜像 tag 的 SHA 相同。
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

if ! core="$(git -C "${repo_root}" describe --tags --abbrev=0 \
  --match 'v[0-9]*' --exclude '*-*' HEAD 2>/dev/null | sed 's/^v//')"; then
  echo "version.sh: no stable upstream tag reachable from HEAD" >&2
  echo "version.sh: run 'git fetch upstream --tags' first" >&2
  exit 1
fi
if ! [[ "${core}" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "version.sh: bad core version from git describe: ${core}" >&2
  exit 1
fi

date="$(TZ='Asia/Shanghai' date +%Y%m%d)"
# 镜像内容只由 ictrek.app/ 以外的路径决定，SHA 取这些路径的最后一次提交。
code_sha="$(git -C "${repo_root}" log -1 --format=%H -- . ':(exclude)ictrek.app' 2>/dev/null || true)"
[[ -n "${code_sha}" ]] || code_sha="$(git -C "${repo_root}" rev-parse HEAD)"
sha="$(GIT_SHA_BASE="${code_sha}" "${script_dir}/git-sha.sh")"
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
# 镜像 tag 的后缀，不含架构前缀；完整 tag 为 <arch>_<IMAGE_TAG>。
image_tag="v${version//-/_}"

echo "CORE=${core}"
echo "DATE=${date}"
echo "N=${n}"
echo "SHA=${sha}"
echo "VERSION=${version}"
echo "IMAGE_TAG=${image_tag}"
