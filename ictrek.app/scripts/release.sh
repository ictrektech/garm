#!/usr/bin/env bash
# Run locally after both architecture images have been published and recorded.
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "${script_dir}/../.."
version="${1:-}"
if [[ ! "${version}" =~ ^[0-9]+\.[0-9]+\.[0-9]+-([0-9]{8})\.([0-9]+)\.([0-9A-Za-z]+)$ ]]; then
  echo "usage: release.sh <version>" >&2
  exit 1
fi
date="${BASH_REMATCH[1]}"
sha="${BASH_REMATCH[3]}"
[[ -z "$(git status --porcelain)" ]] || { echo "worktree must be clean" >&2; exit 1; }
[[ "$(git rev-parse HEAD)" == "${sha}"* ]] || { echo "version must identify HEAD" >&2; exit 1; }

trigger="vos-garm-v${version}"
public="v${version}"
for tag in "${trigger}" "${public}"; do
  if git show-ref --verify --quiet "refs/tags/${tag}"; then
    echo "tag already exists: ${tag}" >&2
    exit 1
  fi
done
remote_tags="$(git ls-remote --tags origin "refs/tags/${trigger}" "refs/tags/${public}")"
[[ -z "${remote_tags}" ]] || { echo "release tag already exists on origin" >&2; exit 1; }

if [[ -z "${FEISHU_CONFIG_FILE:-}" && -r "${HOME}/.feishu.components.json" ]]; then
  export FEISHU_CONFIG_FILE="${HOME}/.feishu.components.json"
fi
python3 "${script_dir}/feishu_sheet.py" --read --sheet AMD_with_cuda --component garm --date "${date}" --value "amd_v${version//-/_}"
python3 "${script_dir}/feishu_sheet.py" --read --sheet ARM_without_cuda --component garm --date "${date}" --value "arm_v${version//-/_}"

git tag -a "${trigger}" -m "VOS GARM ${version}"
git push origin "refs/tags/${trigger}"
