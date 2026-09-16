# Implementation and verification

Source: both supplied archives, blueprint revision 1.1.

## Delivered: 0.1.0 evaluation release

Verified on 2026-09-15, with dependency hardening and revalidation on 2026-09-16,
using Podman 5.8.3, podman-compose 1.6.0,
PostgreSQL 17, Go 1.27.1 and Node 24 on linux/amd64 through Windows Podman
Machine. Clean deployment was verified with both rootful and rootless Podman
connections; application containers run as UID 10001. A separate physical Linux
server and its client-specific identity/storage configuration still require
acceptance testing before production use.

| Blueprint area | Implemented behavior |
| --- | --- |
| M1 audit spine | Complete-population CSV/JSON import, exact decimal controls, explicit periods, canonical data, immutable Parquet, three tests, queue and replay |
| M2 rule governance | Bounded ZEN graphs, guided severity/routing, immutable versions, true/false/null fixtures, simulation, release and stable-row diff |
| M3 operations | OIDC/PKCE and roles, RLS, isolated runtime DB role, persistent worker jobs, freshness indicators, offline images, backup/restore tools |
| M4 onboarding | Source configuration, column mapping, profiling, manual import and automatic atomic-file inbox with durable deduplication |
| M5 assurance | Run trends, exception mix, successful-run resolution and assurance dashboard |

The shipped rules are AP-01 duplicate payments, JE-01 weekend postings and
AP-02 approval-limit breaches. Candidate joins are engineering-owned SQL;
financial boundary checks use exact decimals before ZEN evaluates the enriched
row. This is documented in [ADR 0002](adr/0002-exact-qualification-and-local-snapshots.md).

## Workspace extension (2026-09-16)

- Full-screen decision viewing and guided business fields for the shipped audit rules.
- Local administrator bootstrap, required initial password change, named accounts,
  role descriptions, account disabling, session revocation and last-admin protection.
- Administrator-managed reporting currency, explicit transaction-currency thresholds,
  timezone, display locale, fiscal year and non-working days.
- Database discovery and sample previews, Excel worksheet selection, CSV and JSON previews.
- Versioned review workflows, assignments, send-back, separate approval, closure,
  reopening and durable case history. Legacy dispositions cannot bypass managed cases.
- PostgreSQL integration tests cover account lifecycle, permissions, workspace policy,
  worksheet parsing, connector previews and workflow enforcement. The fresh-install
  Chromium test also passes account creation, first-login password change, currency
  setup, workflow publication, PostgreSQL and Excel previews, a full-width graph,
  rejection of self-approval and closure by a separate approver.
- PostgreSQL connections were exercised against a live server. MySQL and SQL Server
  driver configuration is tested; live-server acceptance for those engines remains
  an environment-specific validation step.

## Measured verification

The data workspace adds a single load, preview, configure and test flow for
CSV, Excel, flat JSON and bounded database tables. Selected columns feed a
full-screen graph with Request, Response, Decision table, Expression, Function
and Switch nodes. Analyses retain their dataset and graph in immutable versions;
results can be inspected and exported. App help boxes have been removed.

Graph execution runs in a disposable process with time and memory limits.
Syntax preflight rejects malformed table conditions before they can appear as
successful non-matches. The Function editor and its workers are bundled locally.

- Go race-enabled unit and real PostgreSQL integration tests pass. The golden
  population produces exactly four expected findings. Negative tests cover
  failed reconciliation, RLS, immutable evidence, conflicting edits, replay,
  suppression expiry/source change, disabled-rule scope and fiscal-period drift.
- Backend coverage after the data workspace: **86.3% overall**; domain 99.1%, exceptions 100%, rules 93.9%,
  analytics 87.2%, ingestion 86.4%, API 95.1%, storage 79.7%, platform 85.7%.
  All configured coverage floors pass.
- OIDC tests use a TLS identity provider and signed RSA ID tokens, exercise
  PKCE exchange, and reject tenant mismatch and reused login state.
- TypeScript production build, Go vet and repository checks pass. Nine
  frontend contract and editor integration tests pass. The npm dependency audit reports zero known
  vulnerabilities at verification time.
- Nine Chromium acceptance tests pass in a fresh local-account deployment,
  including the eight new analysis flows and the existing administration and
  review workflow. These exercise CSV, Excel, JSON, full PostgreSQL table loading,
  column changes, saved analyses, export, permissions, actual WebAssembly readiness
  and an immediate Function edit followed by close and test. The separate demo
  workflow uses its own temporary credentials and database.
- Chromium completes the real deployed workflow: import, wait for completion,
  queue search, replay, dismiss, reimport, verify dismissal survives, simulate
  a rule and open the decision graph and assurance pages. No uncaught browser
  errors or external asset requests occur. Screenshots were visually reviewed.
- A fresh isolated deployment loaded only the offline bundle's OCI images
  with pulls disabled, initialized an empty PostgreSQL volume and fresh native
  secrets, produced all four golden findings and replayed evidence successfully.
  Atomic inbox publication and deduplication across worker restart also pass.
  The final patched images passed on the rootless connection on 2026-09-16.
  This validates a clean namespace on the current machine, not a second host.
- A restore drill restored **24 observations and six immutable snapshots**
  into an isolated database/volume and verified counts and every snapshot hash.
  It did not overwrite the running database.

Reproduce with `make check test-web`, `make test-int`,
`python scripts/coverage.py`, `make test-e2e`,
`python scripts/deployment-smoke.py`, and `python scripts/restore-drill.py`.
On this Windows workstation, the check target's Go commands ran inside the
Linux development container and its Python/npm commands ran on the host.

## Remaining blueprint work and capacity limits

- Database connection tests and table previews support PostgreSQL, MySQL and SQL Server.
  Preview credentials are request scoped. Scheduled database/REST extraction and saved
  connector credentials remain outstanding; automatic ingestion consumes complete JSON files in the inbox.
- dbt and DuckDB are not included. PostgreSQL performs candidate SQL and typed
  transforms. S3 is not included; use the persistent filesystem/NAS option.
- Invoice, vendor, employee and access-grant tables exist, but their ingestion
  and additional ITGC tests are not implemented.
- The Analyze data workspace supports custom graphs against imported rows. The
  reconciled audit pipeline still uses engineering-owned population joins and
  three shipped audit tests. Custom analysis matches do not create managed cases.
  See [Product capabilities](product-capabilities.md) for the broader feature comparison.
- Native Quadlet, Helm/Kubernetes, bundled identity/observability services,
  SIEM/webhooks and external notifications remain outstanding.
- The input limit is 100,000 records per population with a bounded exception
  ceiling. Five-million-row throughput, latency SLOs and concurrent load have
  not been measured; blueprint figures remain targets, not capacity claims.
- Multiarchitecture images, signed release provenance, a cross-version upgrade
  path, high availability and external security review remain outstanding.
  The generated archive is an unsigned development bundle with SHA-256 checksums.

Original documents are preserved under `docs/specifications`. Product requirements
were used as the design source. Embedded instructions to agents, sample approval
processes and team assignments were treated as reference material, separately
from the owner's request.
