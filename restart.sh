#!/usr/bin/env bash
set -euo pipefail

# Always run from the project directory, including when called from elsewhere.
cd "$(dirname "${BASH_SOURCE[0]}")"

no_pull=false
service=
while (( $# )); do
  case "$1" in
    --no-pull) no_pull=true; shift ;;
    --service)
      [[ $# -ge 2 && ( "$2" == ui || "$2" == api ) ]] || {
        echo '用法：./restart.sh [--no-pull] [--service ui|api]' >&2; exit 2;
      }
      service=$2; shift 2 ;;
    -h|--help)
      echo '用法：./restart.sh [--no-pull] [--service ui|api]'
      echo '自动保留本目录的生产叠加配置；单服务更新不会重建其依赖。'
      exit 0 ;;
    *) echo "未知参数：$1" >&2; exit 2 ;;
  esac
done

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
"${compose[@]}" config --quiet
if [[ -n "$service" ]]; then
  "${compose[@]}" build "$service"
  "${compose[@]}" up -d --no-deps "$service"
else
  "${compose[@]}" up -d --build
fi

printf '\n[3/3] 当前容器状态：\n'
"${compose[@]}" ps
