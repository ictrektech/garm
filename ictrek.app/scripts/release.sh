#!/usr/bin/env bash
# 本地发布：算版本、出镜像、出包、建 tag、发飞书，然后打印推送命令。
# 不推送 git，推送由人执行。推 tag 会触发 .github/workflows/vos-release.yml。
#
# 用法：release.sh [--no-push] [--no-feishu] [--dry-run]
#   --no-push    只构建本地镜像，不推送仓库；打出的包引用本地 tag
#   --no-feishu  不发飞书通知
#   --dry-run    只出包，跳过镜像与 tag，用于预览
# 环境：
#   FEISHU_WEBHOOK  飞书机器人 webhook 地址，未设置时跳过通知
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

git diff --quiet && git diff --cached --quiet || die "worktree is not clean"

# 让本地看到远端 tag，便于同名检查。不得用带 --force 的通配 refspec，
# 远端无匹配 tag 时 git 会删除本地同名 tag。
git fetch --tags origin >/dev/null 2>&1 || die "cannot fetch tags from origin"

version_out="$("${script_dir}/version.sh")" || die "cannot compute version, see the error above"
eval "${version_out}"
tag="vos-garm-v${VERSION}"
image_repo="${IMAGE_REPO:-swr.cn-southwest-2.myhuaweicloud.com/ictrek/garm}"
branch="$(git branch --show-current)"

if git rev-parse -q --verify "refs/tags/${tag}" >/dev/null; then
  die "tag already exists: ${tag}; delete it or use a new build date"
fi

if (( dry_run )); then
  echo "release.sh: dry run, skipping image build and tag"
elif (( no_push )); then
  "${script_dir}/build_image.sh" --no-push --version "${VERSION}"
  export IMAGE_REF="garm:${IMAGE_TAG}"
else
  "${script_dir}/build_image.sh" --version "${VERSION}"
fi

"${script_dir}/package.sh" --version "${VERSION}"
package="${script_dir}/../dist/garm_${VERSION}_pull.tar"
image="${IMAGE_REF:-${image_repo}:${IMAGE_TAG}}"

if (( dry_run )); then
  echo "release.sh: version ${VERSION} (dry run, no tag created)"
  exit 0
fi

git tag -a "${tag}" -m "VOS garm ${VERSION}"

send_feishu() {
  local webhook="${FEISHU_WEBHOOK:-}"
  if [[ -z "${webhook}" ]]; then
    echo "release.sh: FEISHU_WEBHOOK not set, skipping notification"
    return 0
  fi
  local text payload
  text="$(printf 'GARM VOS 发布\n版本: %s\n镜像: %s\n包: %s\n分支: %s\n提交: %s' \
    "${VERSION}" "${image}" "${package}" "${branch}" "$(git rev-parse --short HEAD)")"
  payload="$(printf '{"msg_type":"text","content":{"text":"%s"}}' "${text//$'\n'/\\n}")"
  if ! curl --fail -sS -X POST "${webhook}" -H "Content-Type: application/json" \
    --data "${payload}" >/dev/null; then
    echo "release.sh: Feishu notification failed" >&2
  fi
}

if (( no_feishu )); then
  echo "release.sh: Feishu notification skipped (--no-feishu)"
else
  send_feishu
fi

echo
echo "release.sh: version ${VERSION}"
echo "release.sh: image   ${image}"
echo "release.sh: tag     ${tag} (local only)"
echo
echo "Push to publish:"
echo "  git push origin ${branch} ${tag}"
