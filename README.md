# XDenovo Platform Deployment

This repository composes the XDenovo Platform's Local and Production
deployments. The first executable slice provides Local PostgreSQL for the
Gateway's persistent authentication data. It follows the canonical
[Platform architecture and database-role contract](https://github.com/XDenovo/platform/blob/main/docs/architecture.md)
and the approved
[Platform technology stack](https://github.com/XDenovo/platform/blob/main/docs/techstack.md).

## Local PostgreSQL

The Local Compose project is named `xdenovo-platform-local`. It contains only
PostgreSQL, uses the `postgres:18.4` image baseline shared with Gateway
Testcontainers integration tests, publishes PostgreSQL to `127.0.0.1`, and
persists data in the `postgres_data` named volume.

PostgreSQL 18 official images store versioned data below
`/var/lib/postgresql`, so the named volume is mounted at that path. The
container enables `no-new-privileges`. Its filesystem remains writable and its
image-default startup capabilities remain available because the official
entrypoint must initialize PostgreSQL and set ownership on a new named volume.
The service is loopback-only and contains no application process.

### Prerequisites and Local configuration

Install Docker with Docker Compose and make sure its engine is running. Create
the ignored Local environment file from the repository root:

```bash
./scripts/init-local-env.sh config/local.env
```

The command writes mode-`0600` random credentials without printing them and
refuses to overwrite an existing file. `config/local.env.example` documents the
required non-secret shape; do not run Local PostgreSQL with its placeholders.
Real `*.env` files are ignored by Git.

The generated file contains an administrator credential used only by the
deployment bootstrap plus separate `gateway_migrator` and `gateway_runtime`
credentials. Give Gateway processes only the credential for their identity:

```text
postgresql://gateway_migrator:<migrator-password>@127.0.0.1:5432/platform
postgresql://gateway_runtime:<runtime-password>@127.0.0.1:5432/platform
```

Do not copy `XDN_LOCAL_POSTGRES_ADMIN_PASSWORD` into the Gateway repository or
any Gateway process. Generated passwords are hexadecimal, so they can be used
in these Local URLs without additional URL encoding. If the configured host
port changes, update the URLs accordingly.

### Validate, start, and bootstrap

Validate interpolation and the rendered Compose model without printing its
potentially secret-bearing output:

```bash
./scripts/local-postgres.sh --env-file config/local.env validate
```

Start PostgreSQL and wait for its health check to pass:

```bash
./scripts/local-postgres.sh --env-file config/local.env start
```

Inspect container and health status:

```bash
./scripts/local-postgres.sh --env-file config/local.env status
```

Create or converge the `platform` database and the two Gateway login roles:

```bash
./scripts/local-postgres.sh --env-file config/local.env bootstrap
```

Bootstrap is idempotent. It repairs elevated role attributes and unexpected
role memberships, restores bootstrap ownership of the `platform` database,
and grants:

- `gateway_migrator`: `CONNECT` and `CREATE` on the `platform` database;
- `gateway_runtime`: `CONNECT` on the `platform` database.

The bootstrap does not create `auth` or `gateway` schemas, application tables,
Drizzle migration state, or domain data. Gateway owns those schemas,
migrations, and schema-level runtime grants.

### Non-destructive stop and restart

Stop and remove the Local container and network while preserving PostgreSQL
data:

```bash
./scripts/local-postgres.sh --env-file config/local.env stop
```

Run the `start` command again to recreate the container with the same named
volume. Re-running `bootstrap` after a restart is safe.

### Destructive reset

The following command permanently deletes the Local PostgreSQL named volume.
It is separate from the normal lifecycle and requires the explicit destructive
confirmation flag:

```bash
./scripts/local-postgres.sh --env-file config/local.env reset --confirm-destroy-data
```

The script displays the exact Local Compose project before deleting its
containers and volume. This reset is not part of normal stop or restart.

## Validation

The repository's executable validation commands are:

```bash
bash -n scripts/*.sh tests/*.sh
tests/local-compose-config.sh
tests/local-postgres-smoke.sh
git diff --check
```

The Compose test checks the single-service model, exact PostgreSQL image,
loopback binding, persistent named volume, health check, and fail-closed
configuration. The smoke test uses the separate stable project
`xdenovo-platform-local-smoke` with a Docker-selected ephemeral loopback port.
It starts PostgreSQL, waits for readiness, bootstraps twice, verifies the
database and least-privilege role contract, repairs deliberately elevated
attributes, proves that no Gateway schema or application table was created,
and recreates the container to verify persistence. Its trap deletes only the
smoke project's generated volume.

GitHub Actions runs both tests for pull requests and changes to `main`.
