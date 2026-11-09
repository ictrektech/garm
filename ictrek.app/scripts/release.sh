#!/usr/bin/env bash
# 本地发布：算版本、出镜像、出包、建 tag，然后打印推送命令。
# 不推送 git，推送由人执行。推 tag 会触发 .github/workflows/vos-release.yml。
#
# 用法：release.sh [--skip-image] [--dry-run]
#   --skip-image  镜像已推送，只校验并出包
#   --dry-run     只算版本并出包，跳过镜像校验与 tag，用于预览包内容
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(git -C "${script_dir}" rev-parse --show-toplevel)"

skip_image=0
dry_run=0

die() { echo "release.sh: $*" >&2; exit 1; }

while [[ $# -gt 0 ]]; do
  case "$1" in
    --skip-image) skip_image=1; shift ;;
    --dry-run) dry_run=1; shift ;;
    -h|--help) sed -n '2,9p' "$0"; exit 0 ;;
    *) die "unknown argument: $1" ;;
  esac
done

cd "${repo_root}"

git diff --quiet && git diff --cached --quiet || die "worktree is not clean"

# 序号从远端 tag 反推，必须先取回。
git fetch --no-tags --force origin 'refs/tags/*:refs/tags/*' >/dev/null 2>&1 \
  || die "cannot fetch tags from origin"

eval "$("${script_dir}/version.sh")"
tag="vos-garm-v${VERSION}"
image="${IMAGE_REPO:-swr.cn-southwest-2.myhuaweicloud.com/ictrek/garm}:${IMAGE_TAG}"
branch="$(git branch --show-current)"

if git rev-parse -q --verify "refs/tags/${tag}" >/dev/null; then
  die "tag already exists: ${tag}; delete it or use a new build date"
fi

if (( dry_run )); then
  echo "release.sh: dry run, skipping image build and tag"
else
  if (( skip_image )); then
    docker manifest inspect "${image}" >/dev/null \
      || die "image not found: ${image}"
  else
    "${script_dir}/build_image.sh" --version "${VERSION}"
  fi
fi

"${script_dir}/package.sh" --version "${VERSION}"

if (( dry_run )); then
  echo "release.sh: version ${VERSION} (dry run, no tag created)"
  exit 0
fi

git tag -a "${tag}" -m "VOS garm ${VERSION}"

echo
echo "release.sh: version ${VERSION}"
echo "release.sh: image   ${image}"
echo "release.sh: tag     ${tag} (local only)"
echo
echo "Push to publish:"
echo "  git push origin ${branch} ${tag}"
