#!/usr/bin/env bash
# ============================================================
# 离线部署包生成脚本（位于 deploy/gocron/，模板 = 本脚本所在目录）
#
# 产物：dist/gocron-<环境>-<tag>-<日期>.tar.gz
#   包内为扁平部署文件（无顶层目录），解压即覆盖目标部署目录：
#   gocron-<tag>.tar / Makefile / docker-compose.yml / .env
#   conf.d/config.*.yaml（local|test|prod 三套全带）
#   start.sh / stop.sh / README.md
#   数据默认 /data/gocron/data、日志默认 /data/gocron/logs（与部署目录分离）
#
# 目标机（Jenkins 流程：下载 -> 解压到 /var/www/gocron -> 执行 start.sh）：
#   mkdir -p /var/www/gocron && tar -xzf gocron-*.tar.gz -C /var/www/gocron
#   cd /var/www/gocron && ./start.sh
#   解压只覆盖部署文件，数据库 /data/gocron/data 与日志 /data/gocron/logs 保留
#
# 用法：
#   ./deploy/gocron/package.sh [tag] [env]
#   ENV=test IMAGE_TAG=v1.0.0 ./deploy/gocron/package.sh
#   SKIP_BUILD=1  ...  # 跳过构建，复用本地已有镜像
#
# 镜像策略：
#   默认         -> 重新构建应用镜像（--pull=false，只用本地基础镜像，不联网）
#   SKIP_BUILD=1 -> 跳过构建：优先复用 gocron:<tag>；若只有旧名
#                   scheduler-platform:<tag>，自动 docker tag 成新名复用
#
# 环境可选：local（本地）/ test（测试）/ prod（线上，默认）
# ============================================================
set -euo pipefail

# ============================================================
# 模块一：日志
# ============================================================
if [ -t 1 ] && [ "${NO_COLOR:-0}" != "1" ]; then
  C_OFF=$'\033[0m'; C_CYAN=$'\033[36m'; C_GREEN=$'\033[32m'
  C_YELLOW=$'\033[33m'; C_RED=$'\033[31m'
else
  C_OFF=""; C_CYAN=""; C_GREEN=""; C_YELLOW=""; C_RED=""
fi

STEP_NO=0
TOTAL_STEPS=6

log_raw()  { printf '%b\n' "$*"; }
log_info() { printf '%b\n' "${C_CYAN}==>${C_OFF} $*"; }
log_step() { STEP_NO=$((STEP_NO + 1)); printf '%b\n' "${C_CYAN}==>${C_OFF} ${STEP_NO}/${TOTAL_STEPS} $*"; }
log_sub()  { printf '%b\n' "    $*"; }
log_ok()   { printf '%b\n' "    ${C_GREEN}OK${C_OFF} $*"; }
log_warn() { printf '%b\n' "    ${C_YELLOW}WARN${C_OFF} $*" >&2; }
die()      { printf '%b\n' "${C_RED}错误：${C_OFF}$*" >&2; exit 1; }

trap 'die "第 ${LINENO} 行执行失败，打包已中止"' ERR

# ============================================================
# 模块二：通用工具
# ============================================================
require_cmd()  { command -v "$1" >/dev/null 2>&1 || die "未找到命令：$1（$2）"; }
require_file() { [ -f "$1" ] || die "$2（缺少文件：$1）"; }
require_dir()  { [ -d "$1" ] || die "$2（缺少目录：$1）"; }

# env_get <file> <key>
env_get() { sed -n "s/^$2=//p" "$1" 2>/dev/null | head -1; }

# env_set <file> <key> <value>（存在替换、不存在追加）
env_set() {
  local file=$1 key=$2 value=$3
  if grep -qE "^${key}=" "$file"; then
    sed -i -E "s|^${key}=.*|${key}=${value}|" "$file"
  else
    printf '%s=%s\n' "$key" "$value" >> "$file"
  fi
}

# run <cmd...>：打印并执行，便于排错
run() { log_sub "$ $*"; "$@"; }

# ============================================================
# 模块三：变量解析
# ============================================================
resolve_vars() {
  # 本脚本位于 deploy/gocron/，仓库根为上两级
  SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
  ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
  cd "$ROOT"

  IMAGE_NAME="${IMAGE_NAME:-gocron}"
  # 改名前的旧镜像名：本地有时直接复用，避免无网环境去远端拉取
  LEGACY_IMAGE_NAME="${LEGACY_IMAGE_NAME:-scheduler-platform}"

  IMAGE_TAG="${1:-${IMAGE_TAG:-}}"
  [ -n "$IMAGE_TAG" ] || IMAGE_TAG="$(env_get "${SCRIPT_DIR}/.env" SCHEDULER_IMAGE_TAG)"
  IMAGE_TAG="${IMAGE_TAG:-latest}"

  case "${ENV:-${2:-prod}}" in
    prod|production|线上) ENV_NAME="prod";  APP_ENV="production" ;;
    test|testing|测试)    ENV_NAME="test";  APP_ENV="test" ;;
    local|dev|本地)       ENV_NAME="local"; APP_ENV="local" ;;
    *) die "未知环境 '${ENV:-${2:-}}'（可选：local / test / prod）" ;;
  esac

  TEMPLATE_DIR="$SCRIPT_DIR"
  ENV_CONFIG_SRC="server/conf.d/config.${ENV_NAME}.yaml"
  ENV_CONFIG_FILE="conf.d/config.${ENV_NAME}.yaml"
  IMPORT_SCRIPT_SRC="server/scripts/import.${ENV_NAME}.yaml"

  DATE="$(date +%Y%m%d)"
  OUT_ROOT="${OUT_ROOT:-dist}"
  PKG_NAME="gocron"                                          # 解压后目录名（固定）
  BUILD_DIR_NAME="gocron-${ENV_NAME}-${IMAGE_TAG}-${DATE}"   # dist 下构建目录
  PKG_DIR="${OUT_ROOT}/${BUILD_DIR_NAME}"
  TARBALL_NAME="gocron-${ENV_NAME}-${IMAGE_TAG}-${DATE}.tar.gz"
  IMAGE_TAR="${IMAGE_NAME}-${IMAGE_TAG}.tar"
}

precheck() {
  require_cmd docker "打包需要 Docker"
  require_dir "$TEMPLATE_DIR" "部署模板目录"
  require_file "${TEMPLATE_DIR}/.env" "部署环境变量模板"
  require_file "$ENV_CONFIG_SRC" "目标环境配置文件"
  require_file "$IMPORT_SCRIPT_SRC" "目标环境导入脚本"
}

# render <file>：替换模板占位符
render() {
  local file=$1
  sed -i \
    -e "s/@@IMAGE_NAME@@/${IMAGE_NAME}/g" \
    -e "s/@@IMAGE_TAG@@/${IMAGE_TAG}/g" \
    -e "s/@@IMAGE_TAR@@/${IMAGE_TAR}/g" \
    -e "s/@@PKG_NAME@@/${PKG_NAME}/g" \
    -e "s/@@TARBALL@@/${TARBALL_NAME}/g" \
    -e "s/@@ENV_NAME@@/${ENV_NAME}/g" \
    -e "s|@@ENV_CONFIG_FILE@@|${ENV_CONFIG_FILE}|g" \
    "$file"
}

# ============================================================
# 模块四：镜像
#   默认：重新构建应用镜像，但 --pull=false，只使用本地已有基础镜像（不联网）
#   SKIP_BUILD=1：不构建，复用本地已有成品镜像
# ============================================================
image_exists() { docker image inspect "$1" >/dev/null 2>&1; }

build_image() {
  log_sub "重新构建镜像（--pull=false，不拉取远程基础镜像）"
  run docker build --pull=false -f Dockerfile \
    -t "${IMAGE_NAME}:${IMAGE_TAG}" \
    -t "${IMAGE_NAME}:latest" .
  log_ok "镜像已重建：${IMAGE_NAME}:${IMAGE_TAG}"
}

# 复用本地已有成品镜像（SKIP_BUILD=1 时使用）
reuse_local_image() {
  if image_exists "${IMAGE_NAME}:${IMAGE_TAG}"; then
    log_ok "复用本地镜像 ${IMAGE_NAME}:${IMAGE_TAG}（跳过构建）"
  elif image_exists "${LEGACY_IMAGE_NAME}:${IMAGE_TAG}"; then
    log_sub "复用旧名镜像 ${LEGACY_IMAGE_NAME}:${IMAGE_TAG}"
    run docker tag "${LEGACY_IMAGE_NAME}:${IMAGE_TAG}" "${IMAGE_NAME}:${IMAGE_TAG}"
    run docker tag "${IMAGE_NAME}:${IMAGE_TAG}" "${IMAGE_NAME}:latest"
    log_ok "已打标签 ${IMAGE_NAME}:${IMAGE_TAG}"
  else
    die "本地无 ${IMAGE_NAME}:${IMAGE_TAG}（也无旧名 ${LEGACY_IMAGE_NAME}:${IMAGE_TAG}），且指定了 SKIP_BUILD=1\n     请先离线导入镜像，或去掉 SKIP_BUILD=1 重新构建"
  fi
}

prepare_image() {
  log_step "准备镜像 ${IMAGE_NAME}:${IMAGE_TAG}（环境：${ENV_NAME}）"
  if [ "${SKIP_BUILD:-0}" = "1" ]; then
    reuse_local_image
  else
    if ! image_exists "${IMAGE_NAME}:${IMAGE_TAG}" && image_exists "${LEGACY_IMAGE_NAME}:${IMAGE_TAG}"; then
      log_sub "提示：本地有旧名镜像 ${LEGACY_IMAGE_NAME}:${IMAGE_TAG}，不想重建可加 SKIP_BUILD=1"
    fi
    build_image
  fi
}

export_image() {
  log_step "导出镜像 -> ${IMAGE_TAR}"
  rm -rf "$PKG_DIR"
  mkdir -p "$PKG_DIR"
  run docker save -o "${PKG_DIR}/${IMAGE_TAR}" "${IMAGE_NAME}:${IMAGE_TAG}"
}

# ============================================================
# 模块五：产物组装
# ============================================================
stage_files() {
  cp -a "${TEMPLATE_DIR}/." "${PKG_DIR}/"
  rm -f "${PKG_DIR}/package.sh"                 # 打包脚本不进部署包
  mkdir -p "${PKG_DIR}/conf.d" "${PKG_DIR}/scripts"
  cp server/conf.d/config-docker.yaml "${PKG_DIR}/conf.d/config.yaml"
  cp server/conf.d/config.*.yaml "${PKG_DIR}/conf.d/"
  cp server/scripts/import.*.yaml "${PKG_DIR}/scripts/"
  log_sub "三套环境配置已随包分发"
}

stage_env() {
  cp "${TEMPLATE_DIR}/.env" "${PKG_DIR}/.env"
  env_set "${PKG_DIR}/.env" SCHEDULER_IMAGE_TAG "$IMAGE_TAG"
  env_set "${PKG_DIR}/.env" SCHED_APP_ENV "$APP_ENV"
  env_set "${PKG_DIR}/.env" SCHED_CONFIG_FILE "$ENV_CONFIG_FILE"
  log_sub "环境已固定：${ENV_NAME} -> ${ENV_CONFIG_FILE}"
}

assemble_package() {
  log_step "组装部署文件（模板：${TEMPLATE_DIR}）"
  stage_files
  stage_env
  log_ok "部署文件就绪"
}

# ============================================================
# 模块六：生成目标机脚本
# ============================================================
write_start_script() {
  cat > "${PKG_DIR}/start.sh" <<'EOF'
#!/usr/bin/env bash
# 离线启动：校验配置 -> 准备目录 -> 加载镜像 -> 启动容器
set -euo pipefail
cd "$(dirname "$0")"

IMAGE_NAME="@@IMAGE_NAME@@"
IMAGE_TAG="@@IMAGE_TAG@@"
IMAGE_TAR="@@IMAGE_TAR@@"
ENV_NAME="@@ENV_NAME@@"
ENV_CONFIG_FILE="@@ENV_CONFIG_FILE@@"

info()     { printf '==> %s\n' "$*"; }
sub()      { printf '    %s\n' "$*"; }
die()      { printf '错误：%s\n' "$*" >&2; exit 1; }
env_get()  { sed -n "s/^$2=//p" "$1" 2>/dev/null | head -1; }

detect_compose() {
  if docker compose version >/dev/null 2>&1; then
    COMPOSE="docker compose"
  else
    COMPOSE="docker-compose"
  fi
}

ensure_dirs() {
  mkdir -p conf.d
  HOST_DATA_DIR="$(env_get .env HOST_DATA_DIR)"; HOST_DATA_DIR="${HOST_DATA_DIR:-/data/gocron/data}"
  HOST_LOG_DIR="$(env_get .env HOST_LOG_DIR)";   HOST_LOG_DIR="${HOST_LOG_DIR:-/data/gocron/logs}"
  mkdir -p "${HOST_DATA_DIR}" "${HOST_LOG_DIR}" \
    || die "无法创建数据/日志目录（${HOST_DATA_DIR} / ${HOST_LOG_DIR}），请用 root 或具备 /data/gocron 写权限的用户执行"
  sub "数据：${HOST_DATA_DIR}   日志：${HOST_LOG_DIR}"
}

ensure_config() {
  CONFIG_FILE="$(env_get .env SCHED_CONFIG_FILE)"
  CONFIG_FILE="${CONFIG_FILE:-${ENV_CONFIG_FILE}}"
  [ -f "$CONFIG_FILE" ] \
    || die "配置文件不存在：${CONFIG_FILE}\n可用：conf.d/config.local.yaml / conf.d/config.test.yaml / conf.d/config.prod.yaml"
  sub "配置：./${CONFIG_FILE}"
}

ensure_image() {
  # 每次都加载随包镜像：同名标签（如 latest）覆盖时，仅当本地已存在就跳过会导致升级不生效
  if [ -f "${IMAGE_TAR}" ]; then
    info "加载随包镜像 ${IMAGE_TAR}（覆盖同名标签，确保升级生效）..."
    docker load -i "${IMAGE_TAR}"
  elif docker image inspect "${IMAGE_NAME}:${IMAGE_TAG}" >/dev/null 2>&1; then
    sub "未附带镜像 tar，使用本地已有镜像：${IMAGE_NAME}:${IMAGE_TAG}"
  else
    die "镜像 tar 不存在（${IMAGE_TAR}）且本地无 ${IMAGE_NAME}:${IMAGE_TAG}"
  fi
}

# 等待新容器健康检查通过（平滑升级的完成判据）
wait_healthy() {
  local cid max=60 i
  info "等待服务就绪（最多约 $((max * 2))s）..."
  for i in $(seq 1 $max); do
    cid="$(${COMPOSE} ps -q scheduler 2>/dev/null || true)"
    if [ -n "$cid" ] && [ "$(docker inspect -f '{{.State.Health.Status}}' "$cid" 2>/dev/null)" = "healthy" ]; then
      sub "容器健康：已就绪"
      return 0
    fi
    sleep 2
  done
  info "等待超时，请查看日志：${COMPOSE} logs --tail=100 scheduler"
  return 1
}

compose_up() {
  info "启动 / 升级服务（环境：${ENV_NAME}）..."
  # 镜像变化时会自动停旧容器、起新容器（单实例有秒级切换中断）
  ${COMPOSE} up -d
  ${COMPOSE} ps
  wait_healthy || true
}

print_summary() {
  PORT="$(env_get .env SCHEDULER_HTTP_PORT)"
  info "部署完成，访问 http://localhost:${PORT:-18080}"
  sub "配置 ./${CONFIG_FILE}   数据 ${HOST_DATA_DIR}   日志 ${HOST_LOG_DIR}"
  sub "默认账号 admin / admin123（请尽快修改 .env 中的 SCHED_SECURITY_JWTSECRET）"
}

main() {
  detect_compose
  # 适配旧版 docker daemon：读取 daemon 支持的最高 API 版本并导出，
  # 避免 client 协商版本（如 1.53）高于 daemon 上限（如 1.39）时报错。
  # 若该值缺失则不导出，交给 docker 默认协商。
  if DAV="$(docker version -f '{{.Server.APIVersion}}' 2>/dev/null)" && [ -n "$DAV" ]; then
    export DOCKER_API_VERSION="$DAV"
  fi
  ensure_dirs
  ensure_config
  ensure_image
  compose_up
  print_summary
}

main "$@"
EOF
}

write_stop_script() {
  cat > "${PKG_DIR}/stop.sh" <<'EOF'
#!/usr/bin/env bash
# 停止并移除容器（数据保留）
set -euo pipefail
cd "$(dirname "$0")"

info() { printf '==> %s\n' "$*"; }

detect_compose() {
  if docker compose version >/dev/null 2>&1; then
    COMPOSE="docker compose"
  else
    COMPOSE="docker-compose"
  fi
}

main() {
  detect_compose
  # 同 start.sh：适配旧版 docker daemon 的 API 版本上限
  if DAV="$(docker version -f '{{.Server.APIVersion}}' 2>/dev/null)" && [ -n "$DAV" ]; then
    export DOCKER_API_VERSION="$DAV"
  fi
  ${COMPOSE} down
  info "已停止；如需彻底清理数据卷： ${COMPOSE} down -v"
}

main "$@"
EOF
}

generate_scripts() {
  log_step "生成启动 / 停止脚本"
  write_start_script
  write_stop_script
  render "${PKG_DIR}/start.sh"
  render "${PKG_DIR}/README.md"
  chmod +x "${PKG_DIR}/start.sh" "${PKG_DIR}/stop.sh"
  log_ok "start.sh / stop.sh / README.md 已渲染"
}

# ============================================================
# 模块七：校验与打包
# ============================================================
verify_artifacts() {
  log_step "校验产物完整性"
  local missing=0 f
  for f in "${IMAGE_TAR}" Makefile docker-compose.yml .env \
           start.sh stop.sh README.md "${ENV_CONFIG_FILE}"; do
    if [ ! -e "${PKG_DIR}/${f}" ]; then
      log_warn "缺失：${f}"
      missing=1
    fi
  done
  [ "$missing" = "0" ] || die "产物不完整，已中止打包"
  log_ok "产物完整"
}

make_tarball() {
  log_step "打包（包内扁平，解压即覆盖目标部署目录）"
  TARBALL="${OUT_ROOT}/${TARBALL_NAME}"
  run tar -czf "$TARBALL" -C "$PKG_DIR" .
  log_ok "$(du -h "$TARBALL" | cut -f1)"
}

summary() {
  log_info "打包完成"
  log_raw ""
  log_raw "  构建目录：$(pwd)/${PKG_DIR}"
  log_raw "  离线包  ：$(pwd)/${TARBALL}"
  log_raw "  目标环境：${ENV_NAME}（配置 ${ENV_CONFIG_FILE}）"
  log_raw "  数据目录：$(env_get "${PKG_DIR}/.env" HOST_DATA_DIR)"
  log_raw "  日志目录：$(env_get "${PKG_DIR}/.env" HOST_LOG_DIR)"
  log_raw ""
  log_raw "  目标机：mkdir -p /var/www/gocron && tar -xzf ${TARBALL_NAME} -C /var/www/gocron && cd /var/www/gocron && ./start.sh"
}

# ============================================================
# 主流程
# ============================================================
main() {
  resolve_vars "$@"
  precheck
  prepare_image
  export_image
  assemble_package
  generate_scripts
  verify_artifacts
  make_tarball
  summary
}

main "$@"
