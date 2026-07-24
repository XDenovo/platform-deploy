#!/usr/bin/env bash
set -Eeuo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
compose_file="${repo_root}/compose.local.yaml"
test_env="$(mktemp)"
trap 'rm -f "${test_env}"' EXIT

fail() {
  printf 'FAIL: %s\n' "$*" >&2
  exit 1
}

write_env() {
  local port="${1:-55432}"
  local admin_password="${2-compose-test-admin-password}"

  {
    printf 'XDN_LOCAL_POSTGRES_PORT=%s\n' "${port}"
    printf 'XDN_LOCAL_POSTGRES_ADMIN_PASSWORD=%s\n' "${admin_password}"
    printf 'XDN_GATEWAY_MIGRATOR_PASSWORD=compose-test-migrator-password\n'
    printf 'XDN_GATEWAY_RUNTIME_PASSWORD=compose-test-runtime-password\n'
  } >"${test_env}"
}

write_env

compose=(
  docker compose
  --env-file "${test_env}"
  --file "${compose_file}"
)

rendered="$("${compose[@]}" config 2>&1)" ||
  fail "valid Local Compose configuration did not render"

services="$("${compose[@]}" config --services)"
[[ "${services}" == "postgres" ]] ||
  fail "Local Compose must contain only the postgres service"

images="$("${compose[@]}" config --images)"
[[ "${images}" == "postgres:18.4" ]] ||
  fail "Local PostgreSQL must use the postgres:18.4 image baseline"

volumes="$("${compose[@]}" config --volumes)"
[[ "${volumes}" == "postgres_data" ]] ||
  fail "Local PostgreSQL must use the postgres_data named volume"

[[ "${rendered}" == *"name: xdenovo-platform-local"* ]] ||
  fail "Local Compose must declare the stable xdenovo-platform-local project name"
[[ "${rendered}" == *"host_ip: 127.0.0.1"* ]] ||
  fail "PostgreSQL host publishing must bind to 127.0.0.1"
[[ "${rendered}" == *"target: 5432"* ]] ||
  fail "PostgreSQL must expose the container's standard port"
[[ "${rendered}" == *"healthcheck:"* ]] ||
  fail "PostgreSQL must define an executable health check"

write_env 55432 ""
if invalid_output="$("${compose[@]}" config --quiet 2>&1)"; then
  fail "blank bootstrap administrator credentials must be rejected"
fi
[[ "${invalid_output}" != *"compose-test-migrator-password"* ]] ||
  fail "configuration failures must not print other credentials"
[[ "${invalid_output}" != *"compose-test-runtime-password"* ]] ||
  fail "configuration failures must not print other credentials"

write_env not-a-port
if "${compose[@]}" config --quiet >/dev/null 2>&1; then
  fail "a malformed Local PostgreSQL host port must be rejected"
fi

write_env
if "${repo_root}/scripts/local-postgres.sh" \
  --env-file "${test_env}" \
  --project-name unrelated-project \
  validate >/dev/null 2>&1; then
  fail "the Local wrapper must reject project names outside its stable Local and smoke targets"
fi

printf 'Local Compose configuration checks passed.\n'
