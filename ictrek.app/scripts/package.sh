#!/usr/bin/env bash
# 生成 pull 模式 VOS 安装包：dist/garm_<version>_pull.tar，外层只含 app.tar.gz。
#
# 用法：package.sh [--version <version>]
# 环境：
#   PACKAGE_VERSION  版本覆盖，CI 从 tag 名传入
#   IMAGE_REPO       镜像仓库前缀，默认 swr.cn-southwest-2.myhuaweicloud.com/ictrek/garm
#   IMAGE_REF_AMD    amd 镜像引用覆盖，优先级最高
#   IMAGE_REF_ARM    arm 镜像引用覆盖，优先级最高
set -euo pipefail

APP_NAME="garm"
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SRC_DIR="${ROOT_DIR}/src"
DIST_DIR="${ROOT_DIR}/dist"
STAGE_DIR="${DIST_DIR}/staging"
PACKAGE_ROOT="${DIST_DIR}/package-root"
IMAGE_REPO="${IMAGE_REPO:-swr.cn-southwest-2.myhuaweicloud.com/ictrek/garm}"
PACKAGE_VERSION="${PACKAGE_VERSION:-}"

die() { echo "package.sh: $*" >&2; exit 1; }

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version) PACKAGE_VERSION="${2:-}"; shift 2 ;;
    -h|--help) sed -n '2,9p' "$0"; exit 0 ;;
    *) die "unknown argument: $1" ;;
  esac
done

if [[ -n "${PACKAGE_VERSION}" ]]; then
  version="${PACKAGE_VERSION}"
  [[ "${version}" =~ ^[0-9]+\.[0-9]+\.[0-9]+-([0-9]{8})\.([0-9]+)\.([0-9A-Za-z]+)$ ]] \
    || die "invalid version: ${version}"
  date="${BASH_REMATCH[1]}"
  sha="${BASH_REMATCH[3]}"
else
  eval "$("${ROOT_DIR}/scripts/version.sh")"
  version="${VERSION}"
  date="${DATE}"
  sha="${SHA}"
fi

tag_suffix="v${version//-/_}"
amd_ref="${IMAGE_REF_AMD:-${IMAGE_REPO}:amd_${tag_suffix}}"
arm_ref="${IMAGE_REF_ARM:-${IMAGE_REPO}:arm_${tag_suffix}}"

for d in "${STAGE_DIR}" "${PACKAGE_ROOT}"; do
  [[ -d "${d}" ]] && find "${d}" -mindepth 1 -delete
done
mkdir -p "${STAGE_DIR}" "${PACKAGE_ROOT}"

files=(manifest.yml docker-compose.yml configs.yml routers.yml README.zh-CN.md README.en.md)
for f in "${files[@]}"; do
  [[ -f "${SRC_DIR}/${f}" ]] || die "missing source file: src/${f}"
  sed -e "s|__APP_VERSION__|${version}|g" \
    -e "s|\${GARM_AMD_IMAGE}|${amd_ref}|g" \
    -e "s|\${GARM_ARM_IMAGE}|${arm_ref}|g" \
    "${SRC_DIR}/${f}" > "${STAGE_DIR}/${f}"
done
cp "${SRC_DIR}/icon.png" "${STAGE_DIR}/icon.png"
cp -R "${SRC_DIR}/garm" "${STAGE_DIR}/garm"
chmod +x "${STAGE_DIR}/garm/entrypoint.sh"
{
  printf 'GARM_AMD_IMAGE=%s\n' "${amd_ref}"
  printf 'GARM_ARM_IMAGE=%s\n' "${arm_ref}"
} > "${STAGE_DIR}/.env"

# 校验渲染结果（garm/config.toml.tpl 的占位符由容器入口脚本在运行时替换）
if grep -qE '__[A-Z0-9_]+__' "${STAGE_DIR}"/*.yml "${STAGE_DIR}"/*.md "${STAGE_DIR}/.env"; then
  die "unrendered placeholder left in package"
fi
grep -qE '\$\{[A-Z0-9_]*_IMAGE\}' "${STAGE_DIR}/docker-compose.yml" \
  && die "unrendered image variable left in docker-compose.yml"
grep -qx "version: ${version}" "${STAGE_DIR}/manifest.yml" \
  || die "manifest version mismatch"
grep -qF "image: ${amd_ref}" "${STAGE_DIR}/docker-compose.yml" \
  || die "compose amd image mismatch: expected ${amd_ref}"
grep -qF "image: ${arm_ref}" "${STAGE_DIR}/docker-compose.yml" \
  || die "compose arm image mismatch: expected ${arm_ref}"
head -c 8 "${STAGE_DIR}/icon.png" | grep -q $'\x89PNG' || die "icon.png is not a PNG"
[[ -x "${STAGE_DIR}/garm/entrypoint.sh" ]] || die "entrypoint.sh is not executable"

tar czf "${PACKAGE_ROOT}/app.tar.gz" -C "${STAGE_DIR}" \
  .env manifest.yml docker-compose.yml configs.yml routers.yml icon.png \
  README.zh-CN.md README.en.md garm

package="${DIST_DIR}/${APP_NAME}_${version}_pull.tar"
tar cf "${package}" -C "${PACKAGE_ROOT}" app.tar.gz
[[ "$(tar tf "${package}" | wc -l | tr -d ' ')" == "1" ]] || die "outer tar must hold only app.tar.gz"

echo "package.sh: built ${package}"
echo "package.sh: image amd ${amd_ref}"
echo "package.sh: image arm ${arm_ref}"
