#!/usr/bin/env bash
set -Eeuo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
compose_file="${repo_root}/compose.local.yaml"
default_project_name="xdenovo-platform-local"
smoke_project_name="xdenovo-platform-local-smoke"

usage() {
  cat >&2 <<'USAGE'
Usage:
  scripts/local-postgres.sh --env-file <path> [--project-name <name>] <command>

Commands:
  validate   Validate the rendered Local Compose configuration without printing it
  start      Start PostgreSQL and wait until its health check passes
  status     Show Local PostgreSQL container status
  bootstrap  Converge the platform database and Gateway login roles
  stop       Stop and remove containers while preserving the named data volume
  reset --confirm-destroy-data
             Destructively remove containers and the named data volume
USAGE
}

env_file=""
project_name=""

while (( $# > 0 )); do
  case "$1" in
    --env-file)
      [[ $# -ge 2 ]] || {
        usage
        exit 2
      }
      env_file="$2"
      shift 2
      ;;
    --project-name)
      [[ $# -ge 2 ]] || {
        usage
        exit 2
      }
      project_name="$2"
      shift 2
      ;;
    --help | -h)
      usage
      exit 0
      ;;
    *)
      break
      ;;
  esac
done

if [[ -z "${env_file}" || $# -lt 1 ]]; then
  usage
  exit 2
fi

if [[ ! -f "${env_file}" ]]; then
  printf 'ERROR: Local environment file does not exist: %s\n' "${env_file}" >&2
  exit 1
fi

if [[ -n "${project_name}" &&
  ! "${project_name}" =~ ^[a-z0-9][a-z0-9_-]*$ ]]; then
  printf 'ERROR: project name must contain only lowercase letters, numbers, underscores, and hyphens.\n' >&2
  exit 2
fi

if [[ -n "${project_name}" &&
  "${project_name}" != "${default_project_name}" &&
  "${project_name}" != "${smoke_project_name}" ]]; then
  printf 'ERROR: project name must be %s or %s.\n' \
    "${default_project_name}" "${smoke_project_name}" >&2
  exit 2
fi

effective_project_name="${project_name:-${default_project_name}}"
compose=(
  docker compose
  --env-file "${env_file}"
  --file "${compose_file}"
)
if [[ -n "${project_name}" ]]; then
  compose+=(--project-name "${project_name}")
fi

command_name="$1"
shift

case "${command_name}" in
  validate)
    (( $# == 0 )) || {
      usage
      exit 2
    }
    "${compose[@]}" config --quiet
    printf 'Local Compose configuration is valid.\n'
    ;;
  start)
    (( $# == 0 )) || {
      usage
      exit 2
    }
    "${compose[@]}" config --quiet
    "${compose[@]}" up --detach --wait postgres
    ;;
  status)
    (( $# == 0 )) || {
      usage
      exit 2
    }
    "${compose[@]}" config --quiet
    "${compose[@]}" ps postgres
    ;;
  bootstrap)
    (( $# == 0 )) || {
      usage
      exit 2
    }
    "${compose[@]}" config --quiet
    if ! "${compose[@]}" exec --no-TTY postgres \
      pg_isready --username xdenovo_bootstrap --dbname postgres \
      >/dev/null 2>&1; then
      printf 'ERROR: Local PostgreSQL is not ready. Run the start command and inspect status.\n' >&2
      exit 1
    fi
    "${compose[@]}" exec --no-TTY postgres \
      psql \
      --username xdenovo_bootstrap \
      --dbname postgres \
      --set ON_ERROR_STOP=1 \
      --file /opt/xdenovo/bootstrap-platform.sql
    printf 'Local PostgreSQL bootstrap is complete.\n'
    ;;
  stop)
    (( $# == 0 )) || {
      usage
      exit 2
    }
    "${compose[@]}" config --quiet
    "${compose[@]}" down --remove-orphans
    ;;
  reset)
    if [[ $# -ne 1 || "$1" != "--confirm-destroy-data" ]]; then
      printf 'ERROR: reset destroys Local PostgreSQL data and requires --confirm-destroy-data.\n' >&2
      exit 2
    fi
    "${compose[@]}" config --quiet
    volume_name="$(
      docker volume ls \
        --filter "label=com.docker.compose.project=${effective_project_name}" \
        --filter "label=com.docker.compose.volume=postgres_data" \
        --format '{{.Name}}'
    )"
    if [[ "${volume_name}" == *$'\n'* ]]; then
      printf 'ERROR: more than one PostgreSQL data volume is labeled for project %s; refusing reset.\n' \
        "${effective_project_name}" >&2
      exit 1
    fi
    volume_name="${volume_name:-${effective_project_name}_postgres_data}"
    printf 'Destructive reset target:\n'
    printf '  Compose project: %s\n' "${effective_project_name}"
    printf '  PostgreSQL data volume: %s\n' "${volume_name}"
    "${compose[@]}" down --volumes --remove-orphans
    ;;
  *)
    usage
    exit 2
    ;;
esac
