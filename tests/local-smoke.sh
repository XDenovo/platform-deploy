#!/usr/bin/env bash
set -Eeuo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
xdd_bin="${XDD_BIN:-${repo_root}/bin/xdd}"
smoke_project="xdenovo-platform-local-smoke"
smoke_dir="$(mktemp -d)"
smoke_env="${smoke_dir}/local.env"

fail() {
  printf 'FAIL: %s\n' "$*" >&2
  exit 1
}

if [[ ! -x "${xdd_bin}" ]]; then
  fail "compiled xdd binary is not executable: ${xdd_bin}"
fi

xdd() {
  XDD_LOCAL_ENV_FILE="${smoke_env}" \
    XDD_LOCAL_PROJECT_NAME="${smoke_project}" \
    "${xdd_bin}" "$@"
}

cleanup() {
  if [[ -f "${smoke_env}" ]]; then
    xdd local dev reset --confirm-destroy-data >/dev/null 2>&1 || true
  fi
  rm -rf "${smoke_dir}"
}
trap cleanup EXIT

compose_exec_psql() {
  xdd local dev -- \
    exec --no-TTY postgres \
    psql \
    --username xdenovo_bootstrap \
    --dbname "${1}" \
    --tuples-only \
    --no-align \
    --set ON_ERROR_STOP=1 \
    --command "${2}"
}

assert_query() {
  local database="$1"
  local sql="$2"
  local expected="$3"
  local description="$4"
  local actual

  actual="$(compose_exec_psql "${database}" "${sql}")" ||
    fail "query failed while checking ${description}"
  [[ "${actual}" == "${expected}" ]] ||
    fail "${description}: expected '${expected}', got '${actual}'"
}

cat >"${smoke_env}" <<'ENV'
XDN_LOCAL_POSTGRES_PORT=0
XDN_LOCAL_POSTGRES_ADMIN_PASSWORD=smoke-test-admin-password
XDN_GATEWAY_MIGRATOR_PASSWORD=smoke-test-migrator-password
XDN_GATEWAY_RUNTIME_PASSWORD=smoke-test-runtime-password
XDN_LOCAL_SEAWEEDFS_PORT=0
XDN_LOCAL_SEAWEEDFS_MASTER_PORT=0
XDN_LOCAL_TEMPORAL_PORT=0
XDN_LOCAL_TEMPORAL_UI_PORT=0
XDN_LOCAL_DBGATE_PORT=0
XDN_LOCAL_GATEWAY_PORT=0
XDN_GATEWAY_AUTH_SECRET=smoke-test-gateway-auth-secret-32-bytes
ENV
chmod 600 "${smoke_env}"

xdd local dev check
xdd local dev up

running_services="$(xdd local dev -- ps --services --filter status=running)"
if [[ "$(printf '%s\n' "${running_services}" | sort)" != $'dbgate\npostgres\nseaweedfs\ntemporal\ntemporal-ui' ]]; then
  fail "dev profile did not leave every dependency running"
fi

temporal_ui_binding="$(xdd local dev -- port temporal-ui 8080)"
[[ "${temporal_ui_binding}" == 127.0.0.1:* ]] ||
  fail "Temporal UI must publish only on loopback"
seaweedfs_master_binding="$(xdd local dev -- port seaweedfs 9333)"
[[ "${seaweedfs_master_binding}" == 127.0.0.1:* ]] ||
  fail "SeaweedFS Master must publish only on loopback"

xdd local dev bootstrap
xdd local dev bootstrap

role_attributes_sql=$(
  cat <<'SQL'
SELECT concat_ws(
  '|',
  rolname,
  rolcanlogin,
  rolsuper,
  rolcreatedb,
  rolcreaterole,
  rolinherit,
  rolreplication,
  rolbypassrls
)
FROM pg_roles
WHERE rolname IN ('gateway_migrator', 'gateway_runtime')
ORDER BY rolname;
SQL
)
expected_role_attributes=$(
  cat <<'TEXT'
gateway_migrator|t|f|f|f|f|f|f
gateway_runtime|t|f|f|f|f|f|f
TEXT
)

assert_query \
  postgres \
  "${role_attributes_sql}" \
  "${expected_role_attributes}" \
  "Gateway role attributes"
assert_query \
  postgres \
  "SELECT count(*) FROM pg_auth_members memberships JOIN pg_roles members ON members.oid = memberships.member WHERE members.rolname IN ('gateway_migrator', 'gateway_runtime');" \
  "0" \
  "Gateway role memberships"
assert_query \
  postgres \
  "SELECT owner.rolname FROM pg_database databases JOIN pg_roles owner ON owner.oid = databases.datdba WHERE databases.datname = 'platform';" \
  "xdenovo_bootstrap" \
  "platform database ownership"
assert_query \
  postgres \
  "SELECT has_database_privilege('gateway_migrator', 'platform', 'CONNECT'), has_database_privilege('gateway_migrator', 'platform', 'CREATE'), has_database_privilege('gateway_runtime', 'platform', 'CONNECT'), has_database_privilege('gateway_runtime', 'platform', 'CREATE');" \
  "t|t|t|f" \
  "Gateway database privileges"
assert_query \
  platform \
  "SELECT count(*) FROM pg_namespace WHERE nspname IN ('auth', 'gateway');" \
  "0" \
  "absence of Gateway-owned schemas"
assert_query \
  platform \
  "SELECT count(*) FROM information_schema.tables WHERE table_schema NOT IN ('information_schema', 'pg_catalog');" \
  "0" \
  "absence of application tables"

compose_exec_psql \
  postgres \
  "ALTER ROLE gateway_runtime WITH SUPERUSER CREATEDB CREATEROLE INHERIT REPLICATION BYPASSRLS; GRANT xdenovo_bootstrap TO gateway_runtime; ALTER DATABASE platform OWNER TO gateway_runtime;" \
  >/dev/null
xdd local dev bootstrap

assert_query \
  postgres \
  "${role_attributes_sql}" \
  "${expected_role_attributes}" \
  "converged Gateway role attributes"
assert_query \
  postgres \
  "SELECT count(*) FROM pg_auth_members memberships JOIN pg_roles members ON members.oid = memberships.member WHERE members.rolname IN ('gateway_migrator', 'gateway_runtime');" \
  "0" \
  "converged Gateway role memberships"
assert_query \
  postgres \
  "SELECT owner.rolname FROM pg_database databases JOIN pg_roles owner ON owner.oid = databases.datdba WHERE databases.datname = 'platform';" \
  "xdenovo_bootstrap" \
  "converged platform database ownership"

xdd local dev down
xdd local dev up

assert_query \
  postgres \
  "SELECT count(*) FROM pg_database WHERE datname = 'platform';" \
  "1" \
  "platform database persistence after normal down and up"
assert_query \
  postgres \
  "${role_attributes_sql}" \
  "${expected_role_attributes}" \
  "Gateway role persistence after normal down and up"

reset_output="$(xdd local dev reset --confirm-destroy-data)"
for volume in postgres_data seaweedfs_data dbgate_data; do
  [[ "${reset_output}" == *"${smoke_project}_${volume}"* ]] ||
    fail "destructive reset must display the exact ${volume} named volume"
done

printf 'Local dev profile smoke checks passed.\n'
