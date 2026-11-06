#!/usr/bin/env bash
set -euo pipefail

# Always run from the project directory, including when called from elsewhere.
cd "$(dirname "${BASH_SOURCE[0]}")"

no_pull=false
atomic_static=false
service=
while (( $# )); do
  case "$1" in
    --no-pull) no_pull=true; shift ;;
    --atomic-static) atomic_static=true; shift ;;
    --service)
      [[ $# -ge 2 && ( "$2" == ui || "$2" == api || "$2" == updates ) ]] || {
        echo '用法：./restart.sh [--no-pull] [--service ui|api|updates] [--atomic-static]' >&2; exit 2;
      }
      service=$2; shift 2 ;;
    -h|--help)
      echo '用法：./restart.sh [--no-pull] [--service ui|api|updates] [--atomic-static]'
      echo '自动保留本目录的生产叠加配置；单服务更新不会重建其依赖。'
      exit 0 ;;
    *) echo "未知参数：$1" >&2; exit 2 ;;
  esac
done
if "$atomic_static" && [[ "$service" != ui ]]; then
  echo '--atomic-static 只能与 --service ui 一起使用' >&2; exit 2
fi

if ! "$no_pull"; then
  printf '\n[1/3] 拉取最新代码…\n'
  # Stop on conflicts or divergent branches; do not restart an incomplete update.
  git pull --ff-only
fi

# Preserve the same overlays used by production, including package mounts and networks.
if [[ -z "${COMPOSE_FILE:-}" ]]; then
  COMPOSE_FILE=docker-compose.yml
  for overlay in compose.production.yml compose.client-updates.yml; do
    if [[ -f "$overlay" ]]; then COMPOSE_FILE+=":$overlay"; fi
  done
  export COMPOSE_FILE
fi

if [[ -n "${DOCKER_COMPOSE_BIN:-}" ]]; then
  compose=("$DOCKER_COMPOSE_BIN")
elif command -v docker >/dev/null 2>&1 && docker compose version >/dev/null 2>&1; then
  compose=(docker compose)
elif command -v docker-compose >/dev/null 2>&1; then
  compose=(docker-compose)
elif [[ -x /xp/server/docker/docker-compose ]]; then
  compose=(/xp/server/docker/docker-compose)
else
  printf '错误：请先安装 Docker Compose（docker compose 或 docker-compose）。\n' >&2
  exit 1
fi

printf '\n[2/3] 检查配置并重新构建、启动容器…\n'
if [[ "$service" == updates ]]; then
  unset COMPOSE_FILE
  [[ -f .env.manju-updates ]] || { echo '缺少独立更新服务配置 .env.manju-updates' >&2; exit 1; }
  "${compose[@]}" --env-file .env.manju-updates -f compose.manju-updates.yml config --quiet
  "${compose[@]}" --env-file .env.manju-updates -f compose.manju-updates.yml build updates
  "${compose[@]}" --env-file .env.manju-updates -f compose.manju-updates.yml up -d --no-deps updates
  "${compose[@]}" --env-file .env.manju-updates -f compose.manju-updates.yml ps updates
  exit 0
fi
"${compose[@]}" config --quiet
if [[ -n "$service" ]]; then
  "${compose[@]}" build "$service"
  if "$atomic_static"; then
    container=$("${compose[@]}" ps -q ui)
    [[ -n "$container" ]] || { echo '现有前端容器未运行，未执行原子发布' >&2; exit 1; }
    bash scripts/publish-static-ui.sh "$container"
  else
    "${compose[@]}" up -d --no-deps "$service"
  fi
else
  "${compose[@]}" up -d --build
fi

printf '\n[3/3] 当前容器状态：\n'
"${compose[@]}" ps
