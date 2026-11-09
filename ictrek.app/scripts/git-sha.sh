#!/usr/bin/env bash
# ===========================================================================
# git-sha.sh - 生成可用作 SemVer 预发布段的 Git 短 SHA
# ===========================================================================
# 来源：/data/xjx/workspace/vai/docs/ictrek-monorepo/scripts/git-sha.sh
# 规范：/data/xjx/workspace/vai/docs/ictrek-monorepo/docs/version.md 第 3 节
#
# 为什么需要它：SemVer 2.0.0 §9 禁止数字标识符带前导零。当短 SHA 恰好是
# 全数字且首位为 0（如 02537684）时，直接拼进版本号会得到非法版本。本脚本
# 从起始长度截取，逐位延长直到出现 a~f，从而始终输出字母数字标识符。
#
# 用法：git-sha.sh [START_LEN]      START_LEN 默认 8
# 环境：GIT_SHA_BASE  可选，改用给定的完整 SHA（CI 场景）
# ===========================================================================
set -euo pipefail

start_len="${1:-8}"
if ! [[ "${start_len}" =~ ^[1-9][0-9]*$ ]]; then
  start_len=8
fi

if [[ -n "${GIT_SHA_BASE:-}" ]]; then
  full_sha="${GIT_SHA_BASE}"
else
  script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
  repo_dir="$(git -C "${script_dir}" rev-parse --show-toplevel 2>/dev/null || echo "${script_dir}")"
  full_sha="$(git -C "${repo_dir}" rev-parse HEAD 2>/dev/null || true)"
fi

if [[ -z "${full_sha}" ]]; then
  echo "unknown"
  exit 0
fi

full_len="${#full_sha}"
n="${start_len}"
if (( n > full_len )); then
  n="${full_len}"
fi

sha="${full_sha:0:${n}}"
while [[ "${sha}" != *[a-f]* && "${n}" -lt "${full_len}" ]]; do
  n=$((n + 1))
  sha="${full_sha:0:${n}}"
done

echo "${sha}"
