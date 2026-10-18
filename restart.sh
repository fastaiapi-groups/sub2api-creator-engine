#!/usr/bin/env bash
set -euo pipefail

# Always run from the project directory, including when called from elsewhere.
cd "$(dirname "${BASH_SOURCE[0]}")"

printf '\n[1/3] 拉取最新代码…\n'
# Stop on conflicts or divergent branches; do not restart an incomplete update.
git pull --ff-only

if command -v docker >/dev/null 2>&1 && docker compose version >/dev/null 2>&1; then
  compose=(docker compose)
elif command -v docker-compose >/dev/null 2>&1; then
  compose=(docker-compose)
else
  printf '错误：请先安装 Docker Compose（docker compose 或 docker-compose）。\n' >&2
  exit 1
fi

printf '\n[2/3] 检查配置并重新构建、启动容器…\n'
"${compose[@]}" config --quiet
# Recreate containers even if their images have not changed; retain data volumes.
"${compose[@]}" up -d --build --force-recreate

printf '\n[3/3] 当前容器状态：\n'
"${compose[@]}" ps
