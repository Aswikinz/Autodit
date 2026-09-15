# 06 — Documentation

## 1. Four audiences, four kinds of document

Conflating these produces documentation nobody reads. Keep them separate.

| Audience | Document type | Lives in | Built by |
|---|---|---|---|
| Engineers and agents building the system | Standards, domain model, ADRs | `docs/`, `docs/adr/` | Read as markdown in the repo |
| Client operations staff installing and running it | Runbooks, install guide, network requirements | `docs/runbooks/` | Shipped in the release bundle |
| Auditors and client users | User guide, test catalogue | `docs/user/` | Docs site |
| Client security and procurement | Security posture, SBOM, questionnaire responses | `docs/security/` | Shipped in the release bundle |

**Documentation ships in the same PR as the change.** A PR that changes behaviour and not the docs is incomplete, and CI cannot detect it — this is a review responsibility.

---

## 2. Tooling

- **MkDocs Material.** Markdown in the repo, versioned with the code, buildable offline. Not a wiki — a wiki drifts from the code within a quarter and nobody notices.
- `mkdocs.yml` at the root; `make docs` serves locally.
- API reference generated from `internal/api/openapi.yaml`, never hand-written.
- Go package docs via standard doc comments; `pkgsite` locally.
- Diagrams as committed `.drawio` files exported to SVG at build time, so they version and diff.

```
make docs          # serve locally at :8000
make docs-build    # static site into site/
make docs-offline  # bundle for the release tarball
```

The docs site must build **without network access**. It is shipped to air-gapped clients; a docs page that loads a font from a CDN renders broken at the exact moment somebody needs it.

---

## 3. Architecture Decision Records

### 3.1 When an ADR is required

Write one when a decision is expensive to reverse or when a future reader would otherwise ask "why on earth is it done this way".

Required for: choosing or replacing a dependency, changing the canonical model, changing exception identity or the evidence chain, changing the rule/SQL boundary, deployment or packaging changes, anything with a security or licence implication.

Not required for: naming, formatting, ordinary refactors, adding a test.

### 3.2 Format

`docs/adr/NNNN-short-title.md`, sequential, never renumbered.

```markdown
# ADR-0012: Blocking keys mandatory on pairwise candidate models

- **Status:** Accepted
- **Date:** 2026-03-14
- **Deciders:** @platform-team, @audit-domain
- **Supersedes:** —
- **Superseded by:** —

## Context

Duplicate-payment detection compares every payment against every other
payment in the period. At 1.2M payments this is an unblocked cross join and
took 40 minutes, dominating total run time. Client volumes go higher.

## Decision

Every pairwise candidate model must declare a blocking key. For AP-01 the
key is (vendor_id, amount_band, 30-day window). A test enforces that every
model matching `cand_*` containing a self-join declares one.

## Consequences

**Positive.** Run time for AP-01 drops from ~40 min to ~90 s at 1.2M rows.
The constraint is enforced mechanically rather than by review discipline.

**Negative.** Blocking can miss genuine duplicates that fall outside the
key — a duplicate paid to a differently-spelled vendor record 45 days apart
is not caught. This is a deliberate recall/runtime trade, and it must be
documented in the test catalogue so auditors know the limitation.

**Follow-up.** Fuzzy vendor matching as a separate test rather than by
widening this key.

## Alternatives considered

- **Unblocked scan with more hardware.** Rejected: does not scale, and the
  single-VM deployment target makes hardware the client's problem.
- **Approximate nearest-neighbour index.** Rejected as premature; revisit if
  recall complaints arise.
```

### 3.3 Never edit an accepted ADR

Supersede it with a new one and cross-link both. The record of what was believed at the time is the point.

`make adr TITLE="Blocking keys on pairwise models"` scaffolds the file with the next number.

---

## 4. Runbooks

Operational documents for people who did not build the system, possibly at 3am, possibly with no vendor access.

`docs/runbooks/`:

```
install.md                  # clean VM to working login
upgrade.md                  # version to version, with rollback
backup-restore.md           # including a verified restore drill
source-onboarding.md        # connecting a new client source
run-failed.md               # a scheduled run did not complete
tieout-failed.md            # the run halted on reconciliation
rule-flooding.md            # a rule hit its exception ceiling
performance-degraded.md
disk-pressure.md            # the one that takes Postgres down
support-bundle.md           # collecting diagnostics on an air-gapped host
```

### 4.1 Runbook structure

Every runbook: symptom, impact, diagnosis steps with exact commands, resolution, escalation, and what to capture before making changes. Copy-pasteable commands, no placeholders that require guessing.

```markdown
# Run failed to complete

## Symptom
Deadman alert: expected run for tenant X did not complete within its window.

## Impact
Controls were not tested for the period. This is an assurance gap and must
be recorded — a control that did not run is indistinguishable from a control
that passed, which is precisely why the deadman alert exists.

## Before changing anything
    ./support-bundle.sh --run-id <run_id>

## Diagnosis
1. Check the run row: ...
2. Check which stage: ...
```

That "Impact" section matters. Without it an operator treats a missed run as a minor outage rather than an assurance gap.

### 4.2 Runbooks are tested

The backup/restore runbook runs in CI as an actual restore drill (`04-TESTING.md`). The install runbook is exercised by `test-install-clean` in the release workflow. A runbook nobody has executed is fiction.

---

## 5. The test catalogue

The most client-visible document in the product. One page per shipped audit test.

```markdown
# AP-01 — Duplicate Payments

**Area:** Accounts Payable · **Since:** v1.0 · **Last changed:** v1.4.0

## Objective
Identify payments that may have settled the same liability more than once.

## Population
All payments in the period, excluding reversals and intercompany transfers.

## Logic
Payments are compared within blocks of (vendor, amount band, 30-day window).
A candidate pair is raised where amount and vendor match and the payment
dates differ by less than the configured window.

## Parameters
| Name | Default | Meaning |
|---|---|---|
| `window_days` | 30 | Maximum gap between paired payments |
| `amount_tolerance` | 0.00 | Permitted difference between amounts |
| `min_amount` | 1000.00 | Pairs below this are not raised |

## Known limitations
Blocking means duplicates paid to differently-spelled vendor master records,
or more than `window_days` apart, are not detected. See ADR-0012.

## Provenance
Implements the duplicate-payment test as described in [standard reference].

## Change history
| Version | Change | Effect on findings |
|---|---|---|
| 1.4.0 | Added `min_amount` parameter | Fewer low-value findings |
```

**"Known limitations" and "Effect on findings" are mandatory.** An auditor relying on this test needs to know what it does not catch, and a client upgrading needs to know why their exception count moved. Omitting either is how a product loses credibility with an audit committee.

---

## 6. Code documentation

- Every exported Go symbol has a doc comment starting with its name. Enforced by `revive`.
- Package-level `doc.go` for anything non-obvious, stating the package's responsibility and its boundaries.
- Domain rules get comments explaining *why*, per `03-CODING-STANDARDS.md` §6.
- dbt models: `schema.yml` descriptions on every model and every column. These surface in the generated dbt docs, which is how a data-literate client user understands the canonical model.

---

## 7. README

The root `README.md` is for orientation only, and stays short:

- What this is, in three sentences
- Quick start for a developer (`make dev`)
- Where to go next: `AGENTS.md` for contributors, `docs/` for everything else
- Licence and support

Resist the urge to grow it. Everything substantial belongs in `docs/` where it is versioned, navigable and searchable.

---

## 8. Keeping documentation honest

- Docs live beside the code and version with it. A release tag is a documentation snapshot.
- `make docs-build` runs in CI; broken internal links fail the build.
- Every runbook carries a `last-verified` date. Anything older than six months is flagged in the quarterly review.
- When a shipped test changes, three things update in the same PR: the golden expectation, the test catalogue entry, and the changelog's audit-relevant section. Missing any one of the three is an incomplete change.
