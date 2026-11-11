#!/usr/bin/env bash
# 发布入口：算版本、出包、打 tag、推送。不构建镜像。
# 只改 ictrek.app/ 时用本脚本；改了 garm 代码时先在两台构建机跑 build_image.sh。
#
# 用法：update_version.sh [--no-push]
#   --no-push   只出包和本地 tag，不推送远端
# 前置条件：
#   - 工作区干净。
#   - ictrek.app/image-tags 存在 amd 与 arm 两行，且两行版本一致。
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
app_dir="$(cd "${script_dir}/.." && pwd)"
repo_root="$(git -C "${app_dir}" rev-parse --show-toplevel)"

no_push=0

die() { echo "update_version.sh: $*" >&2; exit 1; }

while [[ $# -gt 0 ]]; do
  case "$1" in
    --no-push) no_push=1; shift ;;
    -h|--help) sed -n '2,9p' "$0"; exit 0 ;;
    *) die "unknown argument: $1" ;;
  esac
done

cd "${repo_root}"

git diff --cached --quiet || die "worktree has staged changes"
git diff --quiet -- . ':(exclude)vendor' ':(exclude)webapp/node_modules' \
  || die "worktree has unstaged changes (vendor and node_modules excluded)"

tags_file="${app_dir}/image-tags"
[[ -f "${tags_file}" ]] \
  || die "missing ictrek.app/image-tags; run build_image.sh on both hosts first"

amd_tag="$(sed -n 's/^amd=//p' "${tags_file}" | head -n 1)"
arm_tag="$(sed -n 's/^arm=//p' "${tags_file}" | head -n 1)"
[[ -n "${amd_tag}" ]] || die "no amd tag in ictrek.app/image-tags"
[[ -n "${arm_tag}" ]] || die "no arm tag in ictrek.app/image-tags"
# 两行版本必须相同，否则安装包会引用缺失的镜像。
amd_version="${amd_tag#amd_v}"
arm_version="${arm_tag#arm_v}"
[[ "${amd_version}" == "${arm_version}" ]] \
  || die "image versions differ (amd=${amd_version}, arm=${arm_version}); rebuild on both hosts"

eval "$("${script_dir}/version.sh")" || die "cannot compute version, see the error above"
tag="vos-garm-v${VERSION}"
branch="$(git branch --show-current)"
[[ -n "${branch}" ]] || die "publish requires a named branch"

if git rev-parse -q --verify "refs/tags/${tag}" >/dev/null; then
  die "tag already exists: ${tag}"
fi
if git ls-remote --exit-code --tags origin "refs/tags/${tag}" >/dev/null 2>&1; then
  die "remote tag already exists: ${tag}"
fi

"${script_dir}/package.sh" --version "${VERSION}"

git tag -a "${tag}" -m "VOS garm ${VERSION}"

if (( no_push )); then
  echo "update_version.sh: version ${VERSION}"
  echo "update_version.sh: tag     ${tag} (local only)"
  exit 0
fi

git push origin "${branch}" "${tag}"

echo "update_version.sh: version ${VERSION}"
echo "update_version.sh: pushed  ${branch} and ${tag}"
echo "update_version.sh: check GitHub Actions and the VOS App Store"
