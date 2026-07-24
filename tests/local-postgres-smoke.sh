#!/usr/bin/env bash
set -Eeuo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
local_postgres="${repo_root}/scripts/local-postgres.sh"
init_local_env="${repo_root}/scripts/init-local-env.sh"
smoke_project="xdenovo-platform-local-smoke"
smoke_dir="$(mktemp -d)"
smoke_env="${smoke_dir}/local.env"

cleanup() {
  if [[ -x "${local_postgres}" && -f "${smoke_env}" ]]; then
    "${local_postgres}" \
      --env-file "${smoke_env}" \
      --project-name "${smoke_project}" \
      reset --confirm-destroy-data >/dev/null 2>&1 || true
  fi
  rm -rf "${smoke_dir}"
}
trap cleanup EXIT

fail() {
  printf 'FAIL: %s\n' "$*" >&2
  exit 1
}

local_command() {
  "${local_postgres}" \
    --env-file "${smoke_env}" \
    --project-name "${smoke_project}" \
    "$@"
}

compose_exec_psql() {
  docker compose \
    --env-file "${smoke_env}" \
    --file "${repo_root}/compose.local.yaml" \
    --project-name "${smoke_project}" \
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

"${init_local_env}" "${smoke_env}" 0
local_command validate
local_command start
local_command bootstrap
local_command bootstrap

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
local_command bootstrap

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

local_command stop
local_command start

assert_query \
  postgres \
  "SELECT count(*) FROM pg_database WHERE datname = 'platform';" \
  "1" \
  "platform database persistence after a normal stop and start"
assert_query \
  postgres \
  "${role_attributes_sql}" \
  "${expected_role_attributes}" \
  "Gateway role persistence after a normal stop and start"

reset_output="$(local_command reset --confirm-destroy-data)"
[[ "${reset_output}" == *"${smoke_project}_postgres_data"* ]] ||
  fail "destructive reset must display the exact named data volume"
if docker volume inspect "${smoke_project}_postgres_data" >/dev/null 2>&1; then
  fail "destructive reset did not delete the smoke project's named data volume"
fi

printf 'Local PostgreSQL smoke checks passed.\n'
