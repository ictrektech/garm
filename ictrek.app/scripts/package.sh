#!/usr/bin/env bash
# 生成 pull 模式 VOS 安装包：dist/garm_<version>_pull.tar，外层只含 app.tar.gz。
#
# 用法：package.sh [--version <version>]
# 环境：
#   PACKAGE_VERSION  版本覆盖，CI 从 tag 名传入
#   FEISHU_CONFIG_FILE  发布表只读凭据，默认优先 ~/.feishu.components.json
set -euo pipefail

APP_NAME="garm"
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SRC_DIR="${ROOT_DIR}/src"
DIST_DIR="${ROOT_DIR}/dist"
STAGE_DIR="${DIST_DIR}/staging"
PACKAGE_ROOT="${DIST_DIR}/package-root"
PACKAGE_VERSION="${PACKAGE_VERSION:-}"

die() { echo "package.sh: $*" >&2; exit 1; }

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version) PACKAGE_VERSION="${2:-}"; shift 2 ;;
    -h|--help) sed -n '2,10p' "$0"; exit 0 ;;
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

if [[ -z "${FEISHU_CONFIG_FILE:-}" && -r "${HOME}/.feishu.components.json" ]]; then
  export FEISHU_CONFIG_FILE="${HOME}/.feishu.components.json"
fi
read_image() {
  python3 "${ROOT_DIR}/scripts/feishu_sheet.py" --read --sheet "$1" --component garm \
    --value "$2_v${version//-/_}" --date "${date}"
}
amd_image="$(read_image AMD_with_cuda amd)"
arm_image="$(read_image ARM_without_cuda arm)"
for image in "${amd_image}" "${arm_image}"; do
  [[ "${image}" =~ ^[a-zA-Z0-9.-]+\.[a-zA-Z0-9.-]+/.+:[^[:space:]]+$ ]] \
    || die "expected a full registry image path"
done

for d in "${STAGE_DIR}" "${PACKAGE_ROOT}"; do
  [[ -d "${d}" ]] && find "${d}" -mindepth 1 -delete
done
mkdir -p "${STAGE_DIR}" "${PACKAGE_ROOT}"

printf 'GARM_AMD_IMAGE=%s\nGARM_ARM_IMAGE=%s\n' "${amd_image}" "${arm_image}" > "${STAGE_DIR}/.env"

files=(manifest.yml docker-compose.yml configs.yml routers.yml README.zh-CN.md README.en.md)
for f in "${files[@]}"; do
  [[ -f "${SRC_DIR}/${f}" ]] || die "missing source file: src/${f}"
  sed -e "s|__APP_VERSION__|${version}|g" \
    -e 's|${GARM_AMD_IMAGE}|'"${amd_image}"'|g' \
    -e 's|${GARM_ARM_IMAGE}|'"${arm_image}"'|g' \
    "${SRC_DIR}/${f}" > "${STAGE_DIR}/${f}"
done
cp "${SRC_DIR}/icon.png" "${STAGE_DIR}/icon.png"
cp -R "${SRC_DIR}/garm" "${STAGE_DIR}/garm"
chmod +x "${STAGE_DIR}/garm/entrypoint.sh"

# 校验渲染结果（garm/config.toml.tpl 的占位符由容器入口脚本在运行时替换）
if grep -qE '__[A-Z0-9_]+__' "${STAGE_DIR}"/*.yml "${STAGE_DIR}"/*.md "${STAGE_DIR}/.env"; then
  die "unrendered placeholder left in package"
fi
grep -qx "version: ${version}" "${STAGE_DIR}/manifest.yml" \
  || die "manifest version mismatch"
for image in "${amd_image}" "${arm_image}"; do
  grep -qFx "    image: ${image}" "${STAGE_DIR}/docker-compose.yml" \
    || die "compose image mismatch: expected ${image}"
done
if grep -qE '\$\{[A-Z_]+_IMAGE\}' "${STAGE_DIR}/docker-compose.yml"; then
  die "unresolved compose image"
fi
head -c 8 "${STAGE_DIR}/icon.png" | grep -q $'\x89PNG' || die "icon.png is not a PNG"
[[ -x "${STAGE_DIR}/garm/entrypoint.sh" ]] || die "entrypoint.sh is not executable"

tar czf "${PACKAGE_ROOT}/app.tar.gz" -C "${STAGE_DIR}" \
  .env manifest.yml docker-compose.yml configs.yml routers.yml icon.png \
  README.zh-CN.md README.en.md garm

package="${DIST_DIR}/${APP_NAME}_${version}_pull.tar"
tar cf "${package}" -C "${PACKAGE_ROOT}" app.tar.gz
[[ "$(tar tf "${package}" | wc -l | tr -d ' ')" == "1" ]] || die "outer tar must hold only app.tar.gz"

echo "package.sh: built ${package}"
printf 'package.sh: amd image %s\npackage.sh: arm image %s\n' "${amd_image}" "${arm_image}"
