#!/usr/bin/env bash
set -euo pipefail
backend_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
test_prefix="sub-adds-recharge-test-$$"
cleanup() {
  docker rm -f "${test_prefix}-go" "${test_prefix}-pg" "${test_prefix}-redis" >/dev/null 2>&1 || true
  docker network rm "$test_prefix" >/dev/null 2>&1 || true
  docker volume rm "${test_prefix}-go-cache" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM
docker network create "$test_prefix" >/dev/null
docker run -d --name "${test_prefix}-pg" --network "$test_prefix" -e POSTGRES_PASSWORD=test -e POSTGRES_DB=recharge_test postgres:16-alpine >/dev/null
docker run -d --name "${test_prefix}-redis" --network "$test_prefix" redis:7-alpine >/dev/null
ready=false
for attempt in $(seq 1 30); do
  if docker exec "${test_prefix}-pg" pg_isready -U postgres >/dev/null 2>&1 && docker exec "${test_prefix}-redis" redis-cli ping >/dev/null 2>&1; then
    ready=true
    break
  fi
  sleep 1
done
if [ "$ready" != true ]; then
  echo 'Temporary test databases did not become ready.' >&2
  exit 1
fi
# These fixed URLs always target the containers above, never an existing .env.
docker run --rm --name "${test_prefix}-go" --network "$test_prefix" \
  -e "RECHARGE_TEST_DATABASE_URL=postgres://postgres:test@${test_prefix}-pg:5432/recharge_test?sslmode=disable" \
  -e "RECHARGE_TEST_REDIS_URL=redis://${test_prefix}-redis:6379/0" \
  -v "$backend_dir:/src" -v "${test_prefix}-go-cache:/go" -w /src golang:1.22-alpine \
  sh -c 'go test -count=1 -v ./... && go vet ./...'
