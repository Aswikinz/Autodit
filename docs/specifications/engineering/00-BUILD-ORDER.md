# 00 — Build Order

Build in this order. Each milestone is a working vertical slice, not a horizontal layer. Do not start a milestone before its predecessor is demonstrably complete, and do not build ahead into a later milestone because it seems convenient.

The reason for this sequencing: the value of every later milestone depends on exception identity and tie-out being correct. Building the rule editor before the exception lifecycle produces a beautiful authoring experience over findings nobody trusts.

---

## M1 — The spine

**Outcome:** one file source produces one reconciled exception, visible in a queue, dismissible, and still dismissed tomorrow.

| Deliverable | Acceptance |
|---|---|
| Canonical model schema | All seven entities migrated, with constraints and indexes. See `01-DOMAIN-MODEL.md`. |
| Single-binary skeleton | `audit api`, `audit worker`, `audit migrate` subcommands; config, structured logging, health, graceful shutdown |
| File/CSV source connector | Behind the `SourceConnector` interface. Watermark state persisted. |
| Tie-out gate | Run halts on count or amount mismatch; failure surfaces with both figures |
| Parquet snapshot | Written to object storage, referenced by run |
| dbt project | Raw → canonical for the three entities the first tests need |
| Three audit tests | One duplicate-payment (set-based), one weekend-posting (row-level), one approval-limit breach (row-level) |
| ZEN integration | JDM model loaded from Postgres, evaluated in-process, trace persisted |
| Exception identity | Deterministic hash; unit tested against fixed fixtures |
| Run reconciliation | new / still-open / resolved-in-source classification, tested across three consecutive runs |
| Exception queue | Server-side filter, sort, pagination; disposition with suppression expiry |
| Evidence chain | Non-nullable columns populated on every exception |

**Exit test:** run the pipeline three times against a dataset where one exception is resolved in source between run 2 and run 3. Dismissed items must not reappear; the resolved item must be classified correctly; every exception must resolve to its snapshot.

Do not proceed until this passes.

---

## M2 — Authoring

**Outcome:** an auditor changes a threshold and a decision table without engineering involvement.

| Deliverable | Acceptance |
|---|---|
| JDM Editor embedded | Rendered inside the app shell, single origin, our design system |
| Guided builder | Default surface: entity → conditions → thresholds, generating JDM beneath. Raw canvas is a privileged mode. |
| Rule catalog | Enable/disable, last run, exception rate, precision per rule |
| Tenant parameters | Materiality, approval limits, fiscal calendar, account groupings, SoD matrix — supplied as decision inputs |
| Rule versioning | Immutable versions, content-hashed, releases as bundles |
| Simulation | Fixture set evaluated against a draft, trace displayed, release blocked on failure |
| Decision table diff | Row-level, keyed on stable row id. Not a JSON diff. |
| Concurrency control | Optimistic locking or explicit checkout. Not live collaborative editing. |

**Exit test:** a non-engineer changes a tolerance, simulates, releases, and sees the exception count change on the next run — with the prior findings still bound to the prior rule version.

---

## M3 — Operability

**Outcome:** the stack can be installed at a client and supported without shell access.

| Deliverable | Acceptance |
|---|---|
| Compose stack | Correct start ordering, real health probes, graceful shutdown with extended grace period on workers |
| Quadlet overlay | Generated from the same templates |
| Zitadel integration | OIDC to the app, upstream federation configured and tested |
| RBAC | Roles enforced server-side on every endpoint; tenant scoping on every query |
| Telemetry | OTel → collector → OpenObserve; redaction processors configured |
| Run monitor screen | Freshness per source, landed/errored/passed-with-zero-findings |
| Deadman alerting | Alert on expected-run-not-completed, not only on failure |
| Backup/restore runbook | Verified by an actual restore test in CI |
| Install/upgrade/verify/support-bundle scripts | Idempotent; preflight mode with no side effects |
| Supply chain artefacts | Signed images, SBOM, scan report in every release |

**Exit test:** a clean VM goes from tarball to working login in under thirty minutes, using only the runbook, with no network egress.

---

## M4 — Onboarding

**Outcome:** an implementation consultant connects a new client's source without engineering.

| Deliverable | Acceptance |
|---|---|
| Source & mapping console | Connection test, schema discovery, column picker, mapping with name-similarity pre-fill |
| Profiling | Nulls, distinct counts, date ranges on a sample |
| Transform preview | Canonical rows shown before commit |
| Database connector | Generic SQL source with declarative query spec |
| REST templates | SAP OData, NetSuite, Xero — credentials and base endpoint only |
| YAML escape hatch | Available to internal staff so an unusual source never blocks a deal |
| Extraction contract docs | Published spec per canonical entity, with control totals |

**Exit test:** a consultant connects a previously unseen Postgres source and reaches first exceptions without opening an editor.

---

## M5 — Scale and assurance

**Outcome:** ready for a large client and for an audit committee.

| Deliverable | Acceptance |
|---|---|
| DuckDB analytics path | Set-based tests execute over Parquet; population definition shared with the Postgres path |
| Partitioning | Exception table partitioned by period and tenant |
| Per-rule exception ceiling | A rule exceeding its threshold halts and flags rather than flooding the queue |
| Performance benchmark suite | Synthetic 5M-line dataset; targets from blueprint §11.4 as regression thresholds |
| Blocking keys enforced | All pairwise tests; enforced by a test, not by convention |
| Assurance dashboard | Coverage by process, exception rate trend, aging, disposition mix |
| SIEM export | Syslog/OTLP forwarding, documented log schema |
| Public API + webhooks | Exception lifecycle events for downstream GRC/ticketing |
| Multi-arch images | amd64 and arm64 built and tested in CI |

**Exit test:** the benchmark suite runs within the published targets and fails CI if a change regresses them.

---

## Cross-cutting, from day one

These are not a milestone. They are built into M1 and maintained thereafter.

- Coverage gates active from the first PR (`04-TESTING.md`)
- ADRs for every architecturally significant decision (`06-DOCUMENTATION.md`)
- Log-content lint active from the first PR (`07-SECURITY.md`)
- Conventional commits and PR template from the first PR (`02-REPOSITORY.md`)
- Dependency licence check in CI from the first dependency
