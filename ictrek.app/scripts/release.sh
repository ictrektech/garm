#!/usr/bin/env bash
# 改了 garm 代码时的第一步：构建并推送本机架构的镜像，写飞书发布表。
# 之后提交 ictrek.app/image-tags 与代码，再运行 update_version.sh 打 tag 并推送。
# 只改 ictrek.app/ 时不运行本脚本，直接运行 update_version.sh。
#
# 用法：release.sh [--no-push] [--no-feishu] [--dry-run]
#   --no-push    只构建本机架构的本地镜像，不推送仓库
#   --no-feishu  跳过写入飞书发布表
#   --dry-run    只按现有 ictrek.app/image-tags 出包，不建镜像
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

if (( dry_run )); then
  "${script_dir}/package.sh"
  echo "release.sh: dry run, image build skipped"
  exit 0
fi

# vendor/ 与 webapp/node_modules 不参与检查：前者由 go mod vendor 生成，
# 后者被 .gitignore 忽略，二者在 NFS 上会显著拖慢工作区扫描。
git diff --cached --quiet || die "worktree has staged changes"
git diff --quiet -- . ':(exclude)vendor' ':(exclude)webapp/node_modules' \
  || die "worktree has unstaged changes (vendor and node_modules excluded)"

build_args=()
if (( no_feishu )); then
  build_args+=(--no-feishu)
fi

if (( no_push )); then
  "${script_dir}/build_image.sh" --no-push "${build_args[@]}"
else
  "${script_dir}/build_image.sh" "${build_args[@]}"
fi

echo
echo "release.sh: 下一步"
echo "  1. 提交 ictrek.app/image-tags 和代码改动"
echo "  2. 在另一种架构的机器上运行本脚本，或使用已推送的 tag"
echo "  3. ./ictrek.app/scripts/update_version.sh"
