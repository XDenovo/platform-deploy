#!/usr/bin/env bash
set -Eeuo pipefail

usage() {
  cat >&2 <<'USAGE'
Usage: scripts/init-local-env.sh <environment-file> [host-port]

Create a mode-0600 Local environment file with independent random PostgreSQL
credentials. The file must not already exist. The default host port is 5432;
port 0 asks Docker to select an ephemeral loopback port for smoke validation.
USAGE
}

if (( $# < 1 || $# > 2 )); then
  usage
  exit 2
fi

env_file="$1"
host_port="${2:-5432}"
env_parent="$(dirname "${env_file}")"

if [[ ! "${host_port}" =~ ^[0-9]+$ ]] ||
  (( host_port < 0 || host_port > 65535 )); then
  printf 'ERROR: host-port must be an integer from 0 through 65535.\n' >&2
  exit 2
fi

if [[ -e "${env_file}" ]]; then
  printf 'ERROR: refusing to overwrite existing Local environment file: %s\n' \
    "${env_file}" >&2
  exit 1
fi

if [[ ! -d "${env_parent}" ]]; then
  printf 'ERROR: environment-file parent directory does not exist: %s\n' \
    "${env_parent}" >&2
  exit 1
fi

if ! command -v openssl >/dev/null 2>&1; then
  printf 'ERROR: openssl is required to generate Local credentials.\n' >&2
  exit 1
fi

umask 077
temporary_env="$(mktemp "${env_file}.tmp.XXXXXX")"
trap 'rm -f "${temporary_env}"' EXIT

admin_password="$(openssl rand -hex 32)"
migrator_password="$(openssl rand -hex 32)"
runtime_password="$(openssl rand -hex 32)"

{
  printf 'XDN_LOCAL_POSTGRES_PORT=%s\n' "${host_port}"
  printf 'XDN_LOCAL_POSTGRES_ADMIN_PASSWORD=%s\n' "${admin_password}"
  printf 'XDN_GATEWAY_MIGRATOR_PASSWORD=%s\n' "${migrator_password}"
  printf 'XDN_GATEWAY_RUNTIME_PASSWORD=%s\n' "${runtime_password}"
} >"${temporary_env}"

chmod 600 "${temporary_env}"
mv "${temporary_env}" "${env_file}"
trap - EXIT

printf 'Created Local environment file %s with mode 0600.\n' "${env_file}"
