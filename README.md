# XDenovo Platform Deployment

This repository composes the XDenovo Platform's two environments: Local and
Production. Local has two Docker Compose profiles:

- `dev` runs PostgreSQL, SeaweedFS, Temporal, and DbGate as real background
  dependencies for service development.
- `full` runs everything in `dev` plus Gateway and the three Compute MCP
  Services from sibling source checkouts. It is a staging-like Local profile,
  not a third environment.

Production remains outside the Local CLI and continues to use its explicit
Docker Compose and systemd topology. The canonical
[Platform architecture](https://github.com/XDenovo/platform/blob/main/docs/architecture.md)
and
[Platform technology stack](https://github.com/XDenovo/platform/blob/main/docs/techstack.md)
own the platform-wide boundaries and technology decisions.

## Install `xdd`

Install Go and Docker with Docker Compose, then build the Local-only Cobra CLI
from this repository:

```bash
go install ./cmd/xdd
xdd --help
```

`go install` writes `xdd` to `GOBIN`, or to the Go workspace's `bin`
directory when `GOBIN` is unset. This repository intentionally does not
publish cross-platform CLI binaries.

Run `xdd` from the `platform-deploy` repository root. Every invocation uses
the stable `xdenovo-platform-local` Compose project, `compose.local.yaml`, and
`config/local.env`.

## Configure Local

Create the ignored Local environment file with independent random secrets:

```bash
xdd local dev init
```

The command writes `config/local.env` with mode `0600`, never prints the
generated secrets, and refuses to overwrite an existing file.
`config/local.env.example` documents the non-secret shape for manual review.
Do not give the PostgreSQL administrator credential to Gateway processes.

The default loopback endpoints are:

| Service | Endpoint |
|---|---|
| PostgreSQL | `127.0.0.1:5432` |
| SeaweedFS S3 | `http://127.0.0.1:8333` |
| Temporal | `127.0.0.1:7233` |
| DbGate | `http://127.0.0.1:3000` |
| Gateway (`full` only) | `http://127.0.0.1:3001` |

All published ports bind to `127.0.0.1`. Real environment files remain out of
Git.

## Use the `dev` profile

Validate interpolation and the rendered Compose model without changing
containers:

```bash
xdd local dev check
```

Start all development dependencies and wait for their health checks:

```bash
xdd local dev up
```

Converge the deployment-owned PostgreSQL database, roles, and grants:

```bash
xdd local dev bootstrap
```

Bootstrap is idempotent. It owns only cluster/database-level PostgreSQL
resources. Gateway continues to own its schemas, tables, and migrations.
Temporal's Local `auto-setup` container provisions its own databases and
schema. SeaweedFS bucket and policy convergence will be added when a consuming
service defines that contract.

Inspect or operate the profile through Docker Compose without adding commands
to `xdd`:

```bash
xdd local dev -- ps
xdd local dev -- logs --follow temporal
xdd local dev -- exec postgres psql --username xdenovo_bootstrap --dbname postgres
```

The arguments after `--` pass straight through to `docker compose` after
`xdd` injects the project, env file, Compose file, and resolved profile flags.
Pass-through cannot replace those global options or delete volumes; use the
confirmed `reset` action for volume deletion.

Stop containers and the project network while preserving named volumes:

```bash
xdd local dev down
```

Delete the exact Local named volumes only with explicit confirmation:

```bash
xdd local dev reset --confirm-destroy-data
```

Before deletion, `xdd` resolves and displays the existing PostgreSQL,
SeaweedFS, and DbGate volume names from their Compose labels. It then stops the
profile without implicit volume deletion and removes only those displayed
volumes.

## Use the `full` profile

Place these repositories beside `platform-deploy`, each with a root
`Dockerfile`:

```text
Platform/
├── platform-deploy/
├── gateway/
├── pepmimic-mcp/
├── graphpep-mcp/
└── bindcraft-mcp/
```

Then use the same action surface:

```bash
xdd local full check
xdd local full up
xdd local full bootstrap
xdd local full -- logs --follow gateway
xdd local full down
```

`full` activates both the `dev` and `full` Compose profiles. Its application
services build from the sibling checkouts and receive
`COMPUTE_BACKEND=fake` as the temporary fake-compute contract.
`xdd local full up` fails before Docker runs if a required checkout or root Dockerfile is
missing. The exact fake-backend variable is still owned by the application
repositories and must be updated when their cross-repository contract lands.

Website is deliberately absent from `full`; run Website with its own
`pnpm dev` command and point it at the loopback Gateway endpoint.

## Security scope

The Local infrastructure images use `no-new-privileges`. Writable filesystems
and image-default users/capabilities remain where the upstream entrypoints
must initialize named volumes, generate Temporal configuration/schema, or
update DbGate's container-local host mapping. These are loopback-only Local
exceptions, not Production baselines.

## Validation

Run the repository's executable checks from its root:

```bash
test -z "$(gofmt -l cmd internal)"
go vet ./...
go test ./...
go install ./cmd/xdd

test_xdd="$(mktemp)"
go build -o "${test_xdd}" ./cmd/xdd
bash -n tests/*.sh
XDD_BIN="${test_xdd}" tests/local-compose.sh
XDD_BIN="${test_xdd}" tests/local-smoke.sh
rm -f "${test_xdd}"

git diff --check
```

The smoke test uses the separate
`xdenovo-platform-local-smoke` project and ephemeral loopback ports. Its trap
removes only that project's containers, network, and three generated named
volumes.
