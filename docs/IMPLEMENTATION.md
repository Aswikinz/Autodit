# Implementation and verification

Source: both supplied archives, blueprint revision 1.1.

## Delivered: 0.1.0 evaluation release

Verified on 2026-09-15, with dependency hardening and revalidation on 2026-09-16,
using Podman 5.8.3, podman-compose 1.6.0,
PostgreSQL 17, Go 1.27.1 and Node 24 on linux/amd64 through Windows Podman
Machine. The tested machine connection is rootful; application containers run
as UID 10001. The supplied Linux configuration supports rootless deployment,
but a separate rootless server acceptance run is still outstanding.

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

## Measured verification

- Go race-enabled unit and real PostgreSQL integration tests pass. The golden
  population produces exactly four expected findings. Negative tests cover
  failed reconciliation, RLS, immutable evidence, conflicting edits, replay,
  suppression expiry/source change, disabled-rule scope and fiscal-period drift.
- Backend coverage: **85.2% overall**; domain 99.1%, exceptions 100%, rules 93.9%,
  analytics 87.2%, ingestion 89.4%, API 90.7%, storage 78.3%, platform 88.6%.
  All configured coverage floors pass.
- OIDC tests use a TLS identity provider and signed RSA ID tokens, exercise
  PKCE exchange, and reject tenant mismatch and reused login state.
- TypeScript production build, Go vet and repository checks pass. Three
  frontend contract tests pass. The npm dependency audit reports zero known
  vulnerabilities at verification time.
- Chromium completes the real deployed workflow: import, wait for completion,
  queue search, replay, dismiss, reimport, verify dismissal survives, simulate
  a rule and open the decision graph and assurance pages. No uncaught browser
  errors or external asset requests occur. Screenshots were visually reviewed.
- A fresh isolated deployment loaded only the offline bundle's OCI images
  with pulls disabled, initialized an empty PostgreSQL volume and fresh native
  secrets, produced all four golden findings and replayed evidence successfully.
  Atomic inbox publication and deduplication across worker restart also pass.
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

- Database/REST extractors and their credential onboarding are not implemented.
  Automatic ingestion currently consumes complete JSON files in the inbox.
- dbt and DuckDB are not included. PostgreSQL performs candidate SQL and typed
  transforms. S3 is not included; use the persistent filesystem/NAS option.
- Invoice, vendor, employee and access-grant tables exist, but their ingestion
  and additional ITGC tests are not implemented.
- No arbitrary new-test authoring: changing populations, joins or graph topology
  requires engineering work and fixtures. The existing graph editor is bounded
  by server-side validation.
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
