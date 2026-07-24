#!/usr/bin/env bash
set -Eeuo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
xdd_bin="${XDD_BIN:-${repo_root}/bin/xdd}"
test_dir="$(mktemp -d)"
test_env="${test_dir}/local.env"
trap 'rm -rf "${test_dir}"' EXIT

fail() {
  printf 'FAIL: %s\n' "$*" >&2
  exit 1
}

if [[ ! -x "${xdd_bin}" ]]; then
  fail "compiled xdd binary is not executable: ${xdd_bin}"
fi

write_env() {
  local postgres_port="${1:-55432}"
  local admin_password="${2-compose-test-admin-password}"

  {
    printf 'XDN_LOCAL_POSTGRES_PORT=%s\n' "${postgres_port}"
    printf 'XDN_LOCAL_POSTGRES_ADMIN_PASSWORD=%s\n' "${admin_password}"
    printf 'XDN_GATEWAY_MIGRATOR_PASSWORD=compose-test-migrator-password\n'
    printf 'XDN_GATEWAY_RUNTIME_PASSWORD=compose-test-runtime-password\n'
    printf 'XDN_LOCAL_SEAWEEDFS_PORT=58333\n'
    printf 'XDN_LOCAL_SEAWEEDFS_MASTER_PORT=59333\n'
    printf 'XDN_LOCAL_TEMPORAL_PORT=57233\n'
    printf 'XDN_LOCAL_TEMPORAL_UI_PORT=58233\n'
    printf 'XDN_LOCAL_DBGATE_PORT=53000\n'
    printf 'XDN_LOCAL_GATEWAY_PORT=53001\n'
    printf 'XDN_GATEWAY_AUTH_SECRET=compose-test-gateway-auth-secret-32-bytes\n'
  } >"${test_env}"
}

xdd() {
  XDD_LOCAL_ENV_FILE="${test_env}" \
    XDD_LOCAL_PROJECT_NAME="xdenovo-platform-local-smoke" \
    "${xdd_bin}" "$@"
}

assert_lines() {
  local actual="$1"
  local expected="$2"
  local description="$3"

  if [[ "$(printf '%s\n' "${actual}" | sort)" != "$(printf '%s\n' "${expected}" | sort)" ]]; then
    fail "${description}: expected [${expected//$'\n'/, }], got [${actual//$'\n'/, }]"
  fi
}

xdd local dev init >/dev/null
[[ -f "${test_env}" ]] ||
  fail "Local init did not create the configured environment file"
if xdd local dev init >/dev/null 2>&1; then
  fail "Local init must refuse to overwrite an existing environment file"
fi

write_env
xdd local dev check >/dev/null
xdd local full check >/dev/null

dev_services="$(xdd local dev -- config --services)"
assert_lines \
  "${dev_services}" \
  $'dbgate\npostgres\nseaweedfs\ntemporal\ntemporal-ui' \
  "dev profile services"

full_services="$(xdd local full -- config --services)"
assert_lines \
  "${full_services}" \
  $'bindcraft-mcp\ndbgate\ngateway\ngraphpep-mcp\npepmimic-mcp\npostgres\nseaweedfs\ntemporal\ntemporal-ui' \
  "full profile services"
[[ "${full_services}" != *"website"* ]] ||
  fail "full profile must not include Website"

images="$(xdd local dev -- config --images)"
for image in \
  "chrislusf/seaweedfs:4.40" \
  "dbgate/dbgate:7.2.3-alpine" \
  "postgres:18.4" \
  "temporalio/auto-setup:1.29.7" \
  "temporalio/ui:2.49.1"; do
  [[ "${images}" == *"${image}"* ]] ||
    fail "dev profile must use ${image}"
done

volumes="$(xdd local dev -- config --volumes)"
assert_lines \
  "${volumes}" \
  $'dbgate_data\npostgres_data\nseaweedfs_data' \
  "Local named volumes"

rendered="$(xdd local full -- config)"
[[ "${rendered}" == *"host_ip: 127.0.0.1"* ]] ||
  fail "all Local host publishing must bind to 127.0.0.1"
[[ "${rendered}" == *"COMPUTE_BACKEND: fake"* ]] ||
  fail "full-profile applications must receive the fake compute placeholder"
[[ "${rendered}" == *"condition: service_healthy"* ]] ||
  fail "full-profile dependencies must wait for healthy Local infrastructure"
[[ "${rendered}" == *"context: "*"/gateway"* ]] ||
  fail "Gateway must build from its sibling checkout"
[[ "${rendered}" == *"context: "*"/pepmimic-mcp"* ]] ||
  fail "Compute MCP Services must build from sibling checkouts"

write_env 55432 ""
if invalid_output="$(xdd local dev check 2>&1)"; then
  fail "blank bootstrap administrator credentials must be rejected"
fi
[[ "${invalid_output}" != *"compose-test-migrator-password"* ]] ||
  fail "configuration failures must not print other credentials"
[[ "${invalid_output}" != *"compose-test-runtime-password"* ]] ||
  fail "configuration failures must not print other credentials"

write_env not-a-port
if xdd local dev check >/dev/null 2>&1; then
  fail "a malformed Local PostgreSQL host port must be rejected"
fi

printf 'Local Compose profile checks passed.\n'
