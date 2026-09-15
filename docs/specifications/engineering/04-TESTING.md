# 04 — Testing and Coverage

## 1. What correctness means here

Most software fails loudly. This one fails quietly: a wrong join produces plausible-looking exceptions, a missing tie-out produces confident findings over half a population, an unstable identity hash produces a queue that resets itself. None of these throw an error.

So the test strategy is weighted toward **data correctness**, not toward code coverage percentage. Coverage is a floor that stops obvious rot; golden datasets are what actually catch the failures that matter.

---

## 2. The pyramid, adapted

| Layer | Proportion | Runtime | What it covers |
|---|---|---|---|
| Unit | ~60% | < 30s total | Pure logic: identity hashing, reconciliation state machine, period resolution, money arithmetic, decision input construction |
| Golden dataset | ~15% | < 3 min | Whole-pipeline correctness against fixed inputs with known expected exceptions |
| Integration | ~20% | < 8 min | Real Postgres, real MinIO, real ZEN, real dbt via testcontainers |
| Contract | ~3% | < 1 min | API responses against `openapi.yaml`; connector implementations against `SourceConnector` |
| End-to-end | ~2% | < 5 min | Playwright: log in, filter queue, dismiss an exception, confirm it stays dismissed |

Benchmarks run separately (§8), not in the standard suite.

---

## 3. Golden datasets — the core of the strategy

A golden dataset is a fixed, committed input population plus the exact set of exceptions it must produce.

```
test/fixtures/golden/
├── ap-duplicates-basic/
│   ├── input/               # CSV or Parquet, small enough to read
│   ├── params.json          # tenant parameters
│   ├── control-totals.json  # for tie-out
│   └── expected.json        # exception keys, severities, and why each is expected
├── je-weekend-postings/
├── period-close-adjustment/     # late data arriving after a period was tested
├── reconciliation-three-runs/   # dismissal persistence across runs
└── tieout-failure/              # must halt, must not produce exceptions
```

Rules:

- `expected.json` lists **exception keys**, not counts. A count assertion passes when two bugs cancel out.
- Every expected exception carries a one-line prose justification. If nobody can write the justification, the test's logic is not understood well enough to ship.
- Fixtures are small — hundreds of rows, not millions. They must be readable by a human during review.
- Every new shipped audit test requires a golden dataset before it can merge. No exceptions to this.
- When a shipped test's behaviour intentionally changes, the golden expectation changes in the same PR, and the diff on `expected.json` **is** the change review. That diff is what a domain reviewer reads.

### 3.1 The four golden datasets that must exist from M1

These encode the failure modes described in `01-DOMAIN-MODEL.md`:

1. **Reconciliation across three runs** — dismissed items stay dismissed; resolved-in-source closes correctly; reopened-on-change works.
2. **Tie-out failure** — the run halts, zero exceptions are written, the delta is reported.
3. **Fan-out join** — a source with a one-to-many relationship that, if joined wrongly, triples the population. Row-count reconciliation must catch it.
4. **Late data** — a back-dated entry arriving after the period was tested produces a new snapshot without mutating the old one, and old findings remain bound to the old snapshot.

---

## 4. Unit testing conventions

- Table-driven, `t.Parallel()`, `t.Run` per case.
- Named cases describing behaviour: `"suppressed exception reopens when consumed fields change"`.
- No mocks for things you own. Use real implementations with in-memory or containerised backing. Mock only at genuine external boundaries.
- `testify/require` for assertions that must stop the test, `assert` for those that should not.
- Deterministic: inject `Clock`, `IDGen`, `Rand`. A test that depends on wall-clock time will eventually fail at midnight UTC on a Sunday, which is also exactly when weekend-posting logic is most interesting.

### 4.1 Property-based tests where they pay

Use `testing/quick` or `gopter` for:

- Exception identity: same inputs always produce the same key; different inputs never collide within a run.
- Reconciliation state machine: no input sequence reaches an invalid state, and no sequence loses a disposition.
- Money arithmetic: no precision loss across currency conversion round-trips.

These three have produced real bugs in systems of this shape. Prioritise them over broad property testing elsewhere.

---

## 5. Integration testing

`testcontainers-go`, real dependencies, no shared state between tests.

```go
func TestReconciliation(t *testing.T) {
    ctx := context.Background()
    pg := testutil.StartPostgres(t, ctx)   // fresh database per test
    s3 := testutil.StartMinIO(t, ctx)
    // ...
}
```

- One container set per package, one fresh **database** per test. Sharing a database across tests creates order dependence that presents as flakiness.
- Migrations run as part of setup, from the real migration files. This means every test also tests the migrations.
- dbt runs for real against the test Postgres for canonical-model tests. Mocked dbt output tests nothing useful.

---

## 6. Coverage policy

### 6.1 Thresholds

| Package | Minimum | Rationale |
|---|---|---|
| `internal/domain` | **95%** | Identity, evidence chain, money, periods. Errors here are silent and severe. |
| `internal/exceptions` | **90%** | Reconciliation and workflow. |
| `internal/rules` | **85%** | Engine wrapper, versioning, simulation. |
| `internal/analytics` | **85%** | Test execution. |
| `internal/ingest` | 80% | Connector adapters; some paths need real sources. |
| `internal/api` | 80% | Handlers and RBAC. |
| `internal/storage` | 75% | Covered substantially by integration tests. |
| `internal/platform` | 70% | Wiring. |
| `web/src` | 70% | Weighted to queue and exception features. |
| **Project total** | **82%** | |

### 6.2 Ratchet, never relax

`codecov.yml`:

```yaml
coverage:
  precision: 2
  round: down
  status:
    project:
      default:
        target: 82%
        threshold: 0%          # coverage may not fall, at all
        if_ci_failed: error
      domain:
        paths: ["internal/domain/"]
        target: 95%
        threshold: 0%
      exceptions:
        paths: ["internal/exceptions/"]
        target: 90%
        threshold: 0%
    patch:
      default:
        target: 90%            # new code is held to a higher bar
        threshold: 0%

comment:
  layout: "reach, diff, flags, files"
  behavior: default
  require_changes: false

flags:
  go:
    paths: ["internal/", "cmd/"]
  web:
    paths: ["web/src/"]

ignore:
  - "**/*_generated.go"
  - "**/mock_*.go"
  - "web/src/api/**"
  - "test/**"
```

Thresholds move **up** as coverage improves, never down. Lowering a threshold to make CI pass is prohibited (`AGENTS.md` §3.9). If a PR cannot meet the patch target, the answer is more tests or a smaller PR.

### 6.3 What coverage does not tell you

A package at 95% coverage can still produce wrong audit findings. Coverage measures lines executed, not behaviour verified. The golden datasets are the real correctness gate; treat the coverage number as a hygiene signal.

Specifically, do **not**:

- Write tests that execute code without asserting on outcomes to lift a number
- Exclude files from coverage because they are hard to test — make them easier to test
- Treat 100% as a goal anywhere; the last few percent is usually error paths that should be covered by integration tests instead

---

## 7. PR checklist

CI enforces items 1–6. Items 7–10 are the ones that matter most and cannot be automated.

1. `make check` passes
2. Coverage thresholds met, patch coverage ≥ 90%
3. No new lint findings
4. No financial values or personal data in logs (`make lint-logs`)
5. Generated artefacts regenerated and committed
6. Migrations are forward-only and tested
7. **A golden dataset exists for any new or changed audit test**
8. **The `expected.json` diff has been read by a domain reviewer**
9. **Exception identity is unchanged, or the change is deliberate and signed off**
10. **Tie-out behaviour is unchanged, or the change is deliberate and signed off**

---

## 8. Performance testing

Separate workflow, nightly on `main`, and on any PR labelled `perf`.

- Synthetic generator in `test/bench/` produces a 5M-line population with realistic vendor and amount distributions. Realistic distribution matters: uniform random data makes duplicate detection and Benford tests behave nothing like production.
- Go benchmarks for hot paths: identity hashing, decision evaluation, reconciliation.
- Pipeline benchmarks per stage, compared against the targets in the blueprint (§11.4). Those targets are regression thresholds, not aspirations.
- A **blocking-key test** asserts that every pairwise candidate model declares one. Enforced by a test, not by convention — this is the difference between a 90-second scan and a 40-minute one.
- Results tracked over time; a regression beyond 20% fails the workflow.

---

## 9. Flaky tests

A flaky test is a bug, either in the test or in the code. The response is to fix it or delete it within one working day — never to add a retry. Retries on a system whose core risk is silent incorrectness are actively harmful: they hide exactly the race conditions that corrupt reconciliation state.

CI runs with `-race` and `-count=1` (no caching of results).
