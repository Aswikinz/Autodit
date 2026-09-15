# 02 — Repository, Branching and Review

## 1. Monorepo, and why

Go backend, React frontend, dbt project, rule pack and deployment templates live in one repository. The reason is release atomicity: a rule pack change that depends on a new canonical column must ship with the migration that adds it. Splitting these creates version-matrix problems at every client deployment.

---

## 2. Layout

```
.
├── AGENTS.md
├── CLAUDE.md -> AGENTS.md            # symlink
├── README.md
├── Makefile
├── go.mod
├── .golangci.yml
├── .editorconfig
├── codecov.yml
├── renovate.json
│
├── cmd/audit/                        # single binary, subcommands
│   ├── main.go
│   ├── api.go
│   ├── worker.go
│   └── migrate.go
│
├── internal/
│   ├── domain/                       # entities, exception identity, evidence chain
│   ├── ingest/
│   │   ├── connector.go              # SourceConnector interface — the seam
│   │   ├── sling/
│   │   ├── dlt/
│   │   └── file/
│   ├── transform/                    # dbt invocation, mapping config
│   ├── analytics/                    # DuckDB execution, test registry
│   ├── rules/                        # ZEN wrapper, JDM storage, versioning, simulation
│   ├── exceptions/                   # identity, reconciliation, workflow
│   ├── api/
│   │   ├── handler/
│   │   ├── middleware/
│   │   └── openapi.yaml              # source of truth for the API
│   ├── auth/
│   ├── storage/
│   │   ├── postgres/
│   │   │   └── migrations/           # numbered, forward-only
│   │   └── objectstore/
│   ├── telemetry/
│   └── platform/                     # config, logging, health, shutdown
│
├── web/
│   ├── src/
│   │   ├── features/                 # by feature, not by file type
│   │   │   ├── queue/
│   │   │   ├── exception/
│   │   │   ├── runs/
│   │   │   ├── rules/
│   │   │   ├── sources/
│   │   │   └── assurance/
│   │   ├── components/               # shared primitives only
│   │   ├── lib/
│   │   └── api/                      # generated from openapi.yaml — do not hand-edit
│   └── package.json
│
├── dbt/
│   ├── models/
│   │   ├── staging/                  # 1:1 with source, renamed and typed
│   │   ├── canonical/                # the seven entities
│   │   └── analytics/                # candidate generation
│   └── tests/
│
├── rulepack/
│   ├── ap/                           # AP-01.jdm.json, AP-02.jdm.json ...
│   ├── je/
│   ├── payroll/
│   ├── itgc/
│   └── fixtures/                     # simulation fixtures per rule
│
├── deploy/
│   ├── templates/                    # SINGLE SOURCE OF TRUTH
│   ├── compose/                      # generated — do not hand-edit
│   ├── quadlet/                      # generated
│   └── chart/                        # generated
│
├── scripts/
│   ├── install.sh
│   ├── upgrade.sh
│   ├── verify.sh
│   └── support-bundle.sh
│
├── test/
│   ├── fixtures/                     # golden datasets
│   ├── integration/
│   └── bench/                        # synthetic 5M-line generator
│
├── docs/
│   ├── adr/
│   ├── runbooks/
│   └── *.md
│
└── .github/
    ├── workflows/
    ├── CODEOWNERS
    ├── pull_request_template.md
    └── ISSUE_TEMPLATE/
```

### 2.1 Rules about layout

- **Nothing importable from outside lives outside `internal/`** until there is a deliberate decision to publish it, recorded in an ADR.
- **`deploy/compose`, `deploy/quadlet` and `deploy/chart` are generated.** Editing them directly is prohibited; the three will drift within two releases if hand-maintained. Edit `deploy/templates` and run `make deploy-gen`. CI fails if generated output differs from committed output.
- **`web/src/api` is generated** from `openapi.yaml`. Same rule.
- **Frontend is organised by feature, not by file type.** A `components/` directory containing every component in the app becomes unnavigable by M3.
- **Migrations are forward-only and numbered.** No down migrations; a mistake is corrected by a new migration. Down migrations against a database holding audit evidence are a liability, not a safety net.

---

## 3. Branching

Trunk-based. `main` is always releasable.

```
main
 └── feat/exception-reconciliation
 └── fix/tieout-rounding
 └── chore/bump-zen
```

- Branch from `main`, merge to `main` by squash.
- No long-lived develop or release branches. Release is a tag on `main`.
- Release branches (`release/1.4.x`) are created **only** when a hotfix is needed for a version a client is running and `main` has moved on. They are cherry-pick targets, not development branches.
- Branch names: `type/short-kebab-description`. Types match commit types below.

### 3.1 Feature flags over long branches

Anything that takes more than a few days goes behind a flag in `internal/platform/flags`, merged incrementally to `main`. Flags are removed once the feature is default-on for two releases; a lingering flag is technical debt and is tracked as such.

---

## 4. Commits

Conventional Commits, enforced in CI.

```
<type>(<scope>): <summary>

<body — why, not what>

<footer — refs, breaking changes>
```

Types: `feat`, `fix`, `perf`, `refactor`, `test`, `docs`, `build`, `ci`, `chore`, `revert`.

Scopes: `domain`, `ingest`, `transform`, `analytics`, `rules`, `exceptions`, `api`, `web`, `deploy`, `dbt`, `rulepack`, `docs`.

Examples:

```
feat(exceptions): reopen suppressed items when source content changes

Suppression previously survived a change to the underlying record, which
meant an auditor's dismissal of a $2k variance persisted after the invoice
was amended to $200k. Content hash of consumed fields now stored with the
exception and compared on reconciliation.

Refs #412
```

```
fix(analytics): add blocking key to duplicate payment pairwise scan

Full cross join at 1.2M payments took 40 minutes. Blocking on
(vendor_id, amount_band, 30-day window) reduces to 90 seconds with no
change to the result set — verified against the golden fixture.
```

### 4.1 Breaking changes

`BREAKING CHANGE:` in the footer. For this product, "breaking" includes:

- Any change to a shipped audit test's logic (findings change at every client)
- Any change to the exception identity function (dismissed items would resurrect)
- Any canonical model column removal or semantic change
- Any API contract change

The first two require explicit human sign-off; see `AGENTS.md` §9.

---

## 5. Pull requests

### 5.1 Size

One logical change. If a PR touches more than ~400 lines of non-generated, non-test code, it is probably two PRs. Large mechanical refactors are acceptable but must contain nothing else.

### 5.2 Template

`.github/pull_request_template.md`:

```markdown
## What and why

<!-- The problem, not the diff. -->

## Domain impact

- [ ] Does not change any shipped test's findings
- [ ] Does not change exception identity
- [ ] Does not change the evidence chain
- [ ] Tie-out behaviour unaffected

<!-- If any box is unchecked, explain and tag a domain reviewer. -->

## Verification

<!-- How you know it works. Golden fixtures used, runs executed. -->

## Checklist

- [ ] `make check` passes locally
- [ ] Tests added or updated; coverage thresholds met
- [ ] No financial values or personal data in logs, traces or metric labels
- [ ] Docs/runbooks updated in this PR
- [ ] ADR written and linked (if architecturally significant)
- [ ] New dependencies licence-checked and recorded
- [ ] Generated artefacts regenerated (`make deploy-gen`, `make api-gen`)
```

### 5.3 Review requirements

`CODEOWNERS`:

```
*                           @platform-team
/internal/domain/           @platform-team @audit-domain
/internal/exceptions/       @platform-team @audit-domain
/dbt/models/canonical/      @platform-team @audit-domain
/rulepack/                  @audit-domain
/internal/storage/postgres/migrations/  @platform-team @dba
/.github/                   @platform-team
/deploy/                    @platform-team @sre
/docs/adr/                  @platform-team
```

Two approvals for anything touching `domain`, `exceptions`, `rulepack`, or migrations. One approval elsewhere. Branch protection on `main` enforces this along with passing status checks and a linear history.

---

## 6. Versioning and releases

Semantic versioning on the whole repository. Application version, chart version and compose bundle version are **identical** — a client must be able to state exactly what they are running.

- Tag `v1.4.2` on `main` triggers the release workflow (`05-CICD.md` §5).
- `CHANGELOG.md` is generated from conventional commits, then hand-edited for the audit-relevant section: **which shipped tests changed behaviour in this release**. That section is what clients read.
- The rule pack carries its own version within the release, and every exception records the rule version it was produced by. These are related but not the same thing.

---

## 7. Issue hygiene

Labels: `milestone:M1..M5`, `area:<scope>`, `type:bug|feat|debt`, `domain-review-required`, `blocked`.

Any issue that would change a shipped test's findings carries `domain-review-required` and cannot be closed by a PR without a domain reviewer's approval.
