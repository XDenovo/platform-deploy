# XDenovo Platform Deployment

## Repository Role

- This repository owns cross-service deployment composition, host integration, operational scripts, and runbooks for the XDenovo Platform.
- It supports exactly two environments: Local and Production.
- Application source code, scientific workloads, and business-schema migrations belong to their owning service repositories.
- Keep repository documentation understandable when this repository is checked out independently. Link platform-wide decisions through canonical GitHub URLs.

## Environment and Target Model

### Local

- Run the platform on one developer machine with real PostgreSQL, SeaweedFS, and Temporal services.
- Keep `dev` and `full` as Docker Compose profiles within Local, not as additional environments. `dev` provides background dependencies; `full` adds locally built application services.
- Exercise the real Gateway, MCP Service, Workflow, Worker, and Artifact paths with fake CPU compute backends.
- Bind host-published service ports to `127.0.0.1`.

### Production

- Use separate Docker Compose projects for the control-plane host and GPU-node host, supervised by systemd.
- Require every mutating operation to name the Production environment and the exact host target. A multi-host operation must identify each target.
- Reading status, validating configuration, or editing deployment files does not authorize applying changes, restarting services, or running migrations.
- Never enable fake compute backends.
- Keep the control-plane and GPU-node topology explicit rather than hiding it behind a complex Compose override chain.

## Ownership Boundaries

- Application repositories own their Dockerfiles, health endpoints, database migrations, and fake compute implementations.
- This repository selects immutable application image digests, supplies deployment configuration, and coordinates rollout order.
- It may document or invoke an application-owned migration, but must not duplicate or redefine migration code.
- PostgreSQL bootstrap may create cluster-level databases, roles, and grants only. Each application remains the source of truth for its schemas and domain data.
- SeaweedFS deployment configuration owns buckets, policies, lifecycle rules, and service access boundaries. Preserve per-service namespaces and policies rather than relying on path prefixes alone for isolation.
- Preserve separate Temporal namespaces and task queues for Compute MCP Services.
- Workers launch bounded Compute Job containers dynamically. Do not model those per-job containers as Compose services.

## Compose and Host Integration

- Keep Local and Production definitions explicit and assign stable project names to every Compose project.
- Pin every Production image by digest. Production definitions must not build application images, use `latest`, mount application source, or depend on mutable host checkouts.
- Use health checks and readiness-aware dependencies where startup order matters. A running container is not sufficient evidence that its service is ready.
- Use persistent named volumes for stateful services. Persist Caddy's `/data` and `/config`, and keep artifact storage independent of GPU hosts.
- Treat files on GPU hosts as disposable cache or job-local temporary data unless an approved design states otherwise.
- Caddy is the only component exposed on the public Production interface and the only component that binds ports `80` and `443`.
- Preserve streaming behavior for Gateway MCP routes and do not cache them.
- Keep PostgreSQL, SeaweedFS, Temporal, Gateway backends, Compute MCP Services, and workers on private networks without public host ports.
- Place external availability monitoring outside the control-plane and GPU-host failure domain. Internal probes may use only a trusted management network.
- Enable non-root users, `no-new-privileges`, dropped capabilities, and read-only filesystems when supported by the service. Document the reason and narrow scope of any exception.

## Scripts and Configuration

- Use the Go + Cobra `xdd` CLI for every Local lifecycle operation. Keep its command surface profile-explicit as `xdd local <dev|full> <action>`.
- Use Bash only for end-to-end tests and thin wrappers around Docker Compose and host tools.
- Start Bash scripts with `#!/usr/bin/env bash` and `set -Eeuo pipefail`.
- Make environment and host targets explicit inputs; do not infer a Production target from the current directory, hostname, or an unset variable.
- Prefer idempotent operations and provide a validation or preview step before a mutating Production action.
- Treat rendered Compose configuration, environment interpolation, and shell traces as potentially secret-bearing. Do not publish raw output or enable tracing around secret handling.
- Keep credentials, private keys, real environment files, backups, and generated deployment state out of Git.

## Production Safety

- Before changing Production, inspect the current state, state the intended target and effect, and verify that the user's authorization covers that operation.
- Define rollout health checks and a rollback path before applying a service or configuration change.
- Require separate explicit confirmation for restore, data reset, volume deletion, host-wide cleanup, and irreversible migration operations.
- Resolve and display exact resources before destructive operations. Do not use unresolved variables, broad globs, or host-wide prune commands as shortcuts.
- Back up affected persistent state before a risky change. Validate recoverability through a restore rehearsal; backup command success alone is insufficient.
- After a Production mutation, verify service health, external routing where applicable, and the expected image digest or configuration revision.

## Development and Validation

- Derive commands from committed manifests, scripts, and CI workflows. Do not invent commands for tooling that has not been configured.
- Format Go with `gofmt`, run `go vet ./...` and `go test ./...`, and verify installation with `go install ./cmd/xdd`.
- Build `xdd` before Bash end-to-end tests and pass its path through `XDD_BIN`; the tests must exercise Local Compose only through the compiled CLI.
- Validate Local configuration with `xdd local dev check` and `xdd local full check`. Use the separate smoke project only through the repository's committed smoke test.
- For documentation-only changes, review affected links and run `git diff --check`.
- Do not add placeholder CI that passes without validating an executable deployment artifact.
- `TODO: When Production Compose manifests land, document the exact configuration validation commands for the Production control-plane and Production GPU-node projects.`
- `TODO: When systemd units land, document the exact unit verification and safe installation checks.`
- `TODO: When backup, restore, and secret-delivery mechanisms are selected, document their rehearsal, rotation, redaction, and Production authorization checks.`

## Git and GitHub Workflow

- Record an approved cross-repository architecture or technology decision in the owning Platform document before implementation.
- Use a self-contained repository Issue as the implementation specification. Create a Platform Initiative only when the change affects a cross-repository contract, architecture boundary, or shared decision.
- Preserve unrelated work, stage only the intended paths, and report validation performed for this repository.
- Follow Conventional Commits. Pull requests report the implemented result and the checks actually run.

## Canonical References

| Topic | Source |
|---|---|
| Platform architecture and deployment topology | [XDenovo Platform architecture](https://github.com/XDenovo/platform/blob/main/docs/architecture.md) |
| Approved infrastructure and runtime choices | [XDenovo Platform technology stack](https://github.com/XDenovo/platform/blob/main/docs/techstack.md) |
| GitHub workflow and release conventions | [XDenovo Platform GitHub conventions](https://github.com/XDenovo/platform/blob/main/docs/github.md) |
| Organization Issue templates | [XDenovo organization Issue templates](https://github.com/XDenovo/.github/tree/main/.github/ISSUE_TEMPLATE) |
| Organization Pull Request template | [XDenovo organization Pull Request template](https://github.com/XDenovo/.github/blob/main/.github/PULL_REQUEST_TEMPLATE.md) |
