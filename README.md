# Autodit

A continuous audit workspace for complete-population testing, exception review
and replayable evidence. Runs locally with **Podman**, with a rootless Linux deployment configuration. No cloud account,
commercial runtime licence or vendor connection is required.

## Start on Windows

Install Podman, start a Podman Machine, and install the Compose provider:

```powershell
python -m pip install podman-compose==1.6.0
powershell -ExecutionPolicy Bypass -File .\scripts\install.ps1
```

Open **http://localhost:8088**. Sign in as **admin** with the locally generated password in
`secrets/admin_password`. Choose a new password when prompted. The installer preserves
existing credentials and data when rerun. All credentials are ignored by Git.

## Start on Linux

Prerequisites: Podman 4.4+, a Compose provider, curl and a rootless service account.
Allow approximately 4 GB RAM for evaluation; production sizing requires a
representative population benchmark.

```sh
sh scripts/install.sh --preflight
sh scripts/install.sh
```

Only the proxy publishes a port, bound to loopback by default. The API, worker
and database share an internal network. The API also has an outbound network for source connection tests. Persistent named volumes hold PostgreSQL
and Parquet snapshots. The services restart with the container runtime.

## First audit

1. In **Administration**, configure workspace settings and a materiality threshold for each transaction currency. The fictional example uses USD, which must be configured explicitly before running it.
2. Create named accounts with the required roles. Publish a review and approval workflow.
3. Sign in with an account that has the Implementer role. Open **Sources & mapping**, preview a database or file, then import a complete population and independent controls.
4. Open **Run monitor** and wait for `Completed`. A failed tie-out halts the run.
5. Auditors inspect findings in **Exception queue** and verify their evidence replay. Complete the steps in **Review cases** to reach approval and closure.

The [administration guide](docs/user/administration.md) explains account setup and permissions. The [workflow and sources guide](docs/user/workflows-and-sources.md) covers reviews, database previews and file imports. Every page includes an in-app Help button.

The example contains fictional data and an explicitly synthetic control report.
Real extracts require independently produced controls; never derive the control
totals from the same uploaded file.

## Implemented

- CSV, Excel worksheet and population JSON import, record previews and mapping.
- PostgreSQL, MySQL and SQL Server connection tests and table previews.
- Local administrator login, named accounts, readable roles and workspace settings.
- Configurable review, remediation and approval steps with assignment and case history.
- Automatic complete-file inbox.
- Explicit fiscal periods, exact decimal amounts and per-currency materiality.
- Count, amount, debit and credit reconciliation before any test runs.
- Blocked duplicate-payment, weekend-posting and approval-limit tests.
- Full-screen ZEN decision graphs, guided priority/routing edits, simulation and version release.
- Server-side queue search, filtering, sorting and pagination; investigation notes.
- Stable identities, suppression expiry/change handling and source resolution.
- Immutable Parquet snapshots, append-only observations/events and replay checks.
- PostgreSQL RLS, separate runtime credentials, OIDC/PKCE and server-side RBAC.
- Run history, freshness/deadman indicators, assurance reporting and recovery tools.
- Linux/Windows installers, an offline bundle builder, backup and restore drill.

This is the **0.1.0 evaluation release**, not the entire five-milestone blueprint.
See [implementation status](docs/IMPLEMENTATION.md) for exact scope, measured
checks and remaining enterprise integrations. Use HTTPS ingress for shared-network access. Federated OIDC remains available.

## Move to a new system

On a connected build system with the three runtime images already built:

```sh
python scripts/bundle.py
```

Transfer `dist/autodit-0.1.0-offline.tar.gz` and its checksum to the new host,
extract it, and run `scripts/install.sh --offline` (Linux) or
`scripts/install.ps1 -Offline` (Windows). The destination needs Podman and a
Compose provider already installed. The archive contains images, installers,
configuration and runbooks, **never your secrets or client data**.

To migrate existing audit history, also transfer a verified database/snapshot
backup and secrets through your protected operational process. See
[backup and restore](docs/runbooks/backup-restore.md).

## Development and checks

The backend requires Go 1.26+ and a C toolchain for ZEN. Linux containers are the
tested build environment. Frontend dependencies are pinned in `web/package-lock.json`.

```sh
make check       # Go race tests, vet, repository checks and TypeScript production build
make test-int    # disposable real PostgreSQL, RLS, API, ZEN and Parquet tests
make test-web    # frontend contract tests
python scripts/deployment-smoke.py --local --browser --use-local-images # isolated account and workflow acceptance test
make images     # application and Caddy proxy images
python scripts/deployment-smoke.py # clean namespace using the offline bundle
```

All changes in this working session are local commits. No push or remote release
is performed by the installation scripts.

## Documentation

- [Install and production configuration](docs/runbooks/install.md)
- [Extraction contract](docs/user/extraction-contract.md)
- [Audit test catalogue](docs/user/test-catalogue.md)
- [User and API guide](docs/user/api.md)
- [Backup and restore](docs/runbooks/backup-restore.md)
- [Failure diagnosis](docs/runbooks/run-failed.md)
- [Security and dependencies](docs/security/posture.md)
- [Original supplied specifications](docs/specifications/)

Licensed under Apache-2.0. See [third-party notices](THIRD_PARTY_NOTICES.md).
