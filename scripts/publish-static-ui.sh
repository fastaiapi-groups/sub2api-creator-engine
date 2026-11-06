#!/usr/bin/env bash
set -euo pipefail

# Copy content-addressed assets first, then replace only index.html atomically.
# Keep old assets so open browser sessions can finish without being reloaded.
container=${1:?UI container ID is required}
docker_bin=${DOCKER_BIN:-}
if [[ -z "$docker_bin" ]]; then
  if command -v docker >/dev/null 2>&1; then docker_bin=$(command -v docker)
  else docker_bin=/xp/server/docker/docker; fi
fi
image=$("$docker_bin" inspect "$container" --format '{{.Config.Image}}')
staging=$(mktemp -d)
temporary=
cleanup() {
  if [[ -n "$temporary" ]]; then "$docker_bin" rm "$temporary" >/dev/null; fi
  rm -r -- "$staging"
}
trap cleanup EXIT
temporary=$("$docker_bin" create "$image")
"$docker_bin" cp "$temporary:/usr/share/nginx/html/." "$staging/"
[[ -s "$staging/index.html" ]] || { echo '新前端缺少 index.html，已保留原站点' >&2; exit 1; }
for entry in "$staging"/*; do
  [[ "${entry##*/}" == index.html ]] && continue
  "$docker_bin" cp "$entry" "$container:/usr/share/nginx/html/"
done
"$docker_bin" cp "$staging/index.html" "$container:/usr/share/nginx/html/.pending-index.html"
"$docker_bin" exec "$container" mv /usr/share/nginx/html/.pending-index.html /usr/share/nginx/html/index.html
printf '前端静态文件已原子更新，原容器和历史资源保持运行。\n'
