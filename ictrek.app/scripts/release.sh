#!/usr/bin/env bash
# 本地发布：算版本、出镜像、出包、建 tag、写飞书发布表，然后打印推送命令。
# 不推送 git，推送由人执行。推 tag 会触发 .github/workflows/vos-release.yml。
#
# 用法：release.sh [--no-push] [--no-feishu] [--dry-run]
#   --no-push    只构建本地镜像，不推送仓库；打出的包引用本地 tag
#   --no-feishu  跳过写入飞书发布表
#   --dry-run    只出包，跳过镜像与 tag，用于预览
# 环境：
#   FEISHU_CONFIG_FILE  飞书凭据文件，默认 ~/.feishu.json
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(git -C "${script_dir}" rev-parse --show-toplevel)"

no_push=0
no_feishu=0
dry_run=0

die() { echo "release.sh: $*" >&2; exit 1; }

while [[ $# -gt 0 ]]; do
  case "$1" in
    --no-push) no_push=1; shift ;;
    --no-feishu) no_feishu=1; shift ;;
    --dry-run) dry_run=1; shift ;;
    -h|--help) sed -n '2,10p' "$0"; exit 0 ;;
    *) die "unknown argument: $1" ;;
  esac
done

cd "${repo_root}"

# vendor/ 与 webapp/node_modules 不参与检查：前者由 go mod vendor 生成，
# 后者被 .gitignore 忽略，二者在 NFS 上会显著拖慢工作区扫描。
git diff --cached --quiet || die "worktree has staged changes"
git diff --quiet -- . ':(exclude)vendor' ':(exclude)webapp/node_modules' \
  || die "worktree has unstaged changes (vendor and node_modules excluded)"

# 让本地看到远端 tag，便于同名检查。不得用带 --force 的通配 refspec，
# 远端无匹配 tag 时 git 会删除本地同名 tag。
git fetch --tags origin >/dev/null 2>&1 || die "cannot fetch tags from origin"

version_out="$("${script_dir}/version.sh")" || die "cannot compute version, see the error above"
eval "${version_out}"
tag="vos-garm-v${VERSION}"
image_repo="${IMAGE_REPO:-swr.cn-southwest-2.myhuaweicloud.com/ictrek/garm}"
branch="$(git branch --show-current)"

# 同一提交可能被多个架构各构建一次。tag 与架构无关，已存在且指向 HEAD 时复用，
# 指向其他提交时才视为冲突。
tag_exists=0
if git rev-parse -q --verify "refs/tags/${tag}" >/dev/null; then
  tag_sha="$(git rev-list -n1 "refs/tags/${tag}")"
  head_sha="$(git rev-parse HEAD)"
  if [[ "${tag_sha}" != "${head_sha}" ]]; then
    die "tag already exists: ${tag} (points to ${tag_sha:0:8}, HEAD is ${head_sha:0:8}); delete it or use a new build date"
  fi
  tag_exists=1
  echo "release.sh: tag ${tag} already exists for this commit, reusing it"
fi

build_args=(--version "${VERSION}")
if (( no_feishu )); then
  build_args+=(--no-feishu)
fi

if (( dry_run )); then
  echo "release.sh: dry run, skipping image build and tag"
elif (( no_push )); then
  "${script_dir}/build_image.sh" --no-push "${build_args[@]}"
  export IMAGE_REF="$("${script_dir}/image-ref.sh" --version "${VERSION}")"
else
  "${script_dir}/build_image.sh" "${build_args[@]}"
fi

"${script_dir}/package.sh" --version "${VERSION}"
image="${IMAGE_REF:-${image_repo}:${IMAGE_TAG}}"

if (( dry_run )); then
  echo "release.sh: version ${VERSION} (dry run, no tag created)"
  exit 0
fi

if (( tag_exists )); then
  echo "release.sh: tag ${tag} already created, skipping"
else
  git tag -a "${tag}" -m "VOS garm ${VERSION}"
fi

echo
echo "release.sh: version ${VERSION}"
echo "release.sh: image   ${image}"
echo "release.sh: tag     ${tag} (local only)"
echo
echo "Push to publish:"
echo "  git push origin ${branch} ${tag}"
