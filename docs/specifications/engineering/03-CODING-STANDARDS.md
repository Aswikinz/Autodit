# 03 — Coding Standards

Style is largely delegated to tools. This file covers what tools cannot enforce, and the conventions specific to this domain.

---

## 1. Go

### 1.1 Tooling

`gofumpt` for formatting, `golangci-lint` for everything else. Both run in `make lint` and in CI.

`.golangci.yml`:

```yaml
run:
  timeout: 5m
  tests: true

linters:
  enable:
    - errcheck          # unchecked errors
    - govet
    - staticcheck
    - revive
    - gosec
    - bodyclose
    - rowserrcheck      # sql.Rows.Err() not checked
    - sqlclosecheck
    - contextcheck      # context not propagated
    - noctx             # http request without context
    - errorlint         # %w vs %v, errors.As/Is
    - nilerr
    - dupl
    - gocritic
    - misspell
    - unconvert
    - unparam
    - wastedassign
    - prealloc
    - exhaustive        # switch over enums
    - forbidigo
    - depguard

linters-settings:
  errcheck:
    check-type-assertions: true
    check-blank: true
  exhaustive:
    default-signifies-exhaustive: false
  forbidigo:
    forbid:
      - p: '^fmt\.Print.*'
        msg: "use the structured logger, not fmt.Print"
      - p: '^panic$'
        msg: "return an error; panic only in main() during startup"
      - p: '^time\.Now$'
        msg: "inject a Clock — time.Now makes tests non-deterministic"
  depguard:
    rules:
      main:
        deny:
          - pkg: "math/rand"
            desc: "use crypto/rand for anything security-relevant; for tests inject a seed"
  gosec:
    excludes:
      - G404   # handled by depguard above
  revive:
    rules:
      - name: exported
        arguments: [checkPrivateReceivers]

issues:
  exclude-rules:
    - path: _test\.go
      linters: [dupl, gosec, prealloc]
```

### 1.2 Errors

- Wrap with context at every layer boundary: `fmt.Errorf("reconcile exceptions for run %s: %w", runID, err)`.
- Sentinel errors in the package that owns the concept: `var ErrTieOutFailed = errors.New("tie-out failed")`.
- Domain errors carry structured fields, not formatted strings, so the API layer can map them without parsing:

```go
type TieOutError struct {
    Entity   string
    Expected decimal.Decimal
    Actual   decimal.Decimal
}

func (e *TieOutError) Error() string {
    return fmt.Sprintf("tie-out failed for %s", e.Entity)  // no amounts in the message
}
```

Note what that error message does **not** contain. See §1.6.

- `panic` only in `main()` during startup, never in library code, never in a handler.

### 1.3 Context

Every function that does I/O takes `ctx context.Context` first. Never store a context in a struct. Never pass `context.Background()` below `main()` or a job entry point.

Long-running work checks `ctx.Done()` between units so graceful shutdown actually works — a worker that ignores cancellation defeats the extended stop grace period.

### 1.4 Interfaces at the consumer

Define interfaces where they are used, not where they are implemented. The exception is `SourceConnector`, which is a deliberate architectural seam and lives in `internal/ingest`:

```go
// SourceConnector is the seam that keeps Sling, dlt and future
// implementations substitutable. See ADR-0007.
type SourceConnector interface {
    Discover(ctx context.Context, cfg SourceConfig) (Schema, error)
    Extract(ctx context.Context, cfg SourceConfig, wm Watermark) (ExtractResult, error)
    TestConnection(ctx context.Context, cfg SourceConfig) error
}
```

Keep interfaces small. An interface with eight methods is a struct.

### 1.5 Determinism

Injected, never called directly:

- `Clock` for time. `time.Now()` is forbidden by lint.
- `IDGen` for UUIDs.
- `Rand` where randomness is needed.

This is not fastidiousness. Exception identity, snapshot boundaries and period resolution all depend on time, and non-deterministic tests around them hide real bugs.

### 1.6 Logging

`log/slog`, structured, no `fmt.Print`.

```go
// Correct
log.InfoContext(ctx, "exception created",
    "rule_id", ruleID,
    "exception_key", key,
    "entity_type", "payment",
    "entity_id", entityID,
    "run_id", runID,
)

// PROHIBITED — see AGENTS.md §3.1
log.InfoContext(ctx, "exception created",
    "amount", payment.Amount,          // financial value
    "vendor_name", vendor.Name,        // personal/commercial data
    "bank_account", vendor.IBAN,       // never
)
```

Levels:

| Level | Use |
|---|---|
| `Error` | Something failed that needs human attention. Always with an error value. |
| `Warn` | Degraded but proceeding. Tie-out tolerance used, connector retried, rule hit its ceiling. |
| `Info` | Lifecycle events: run started, stage completed, exceptions created, service started/stopped. |
| `Debug` | Development only. Never enabled by default in a client deployment. |

Every log line inside a run carries `run_id`, `tenant_id` and `stage`.

### 1.7 Concurrency

- `errgroup.WithContext` for parallel work; never bare goroutines in request or job paths.
- Every goroutine has a clear owner and a defined exit. No fire-and-forget.
- Bound parallelism explicitly — per-tenant worker concurrency limits exist for a reason (noisy neighbours, blueprint §11.3).
- The race detector runs in CI. A flaky test is a bug to fix, never to retry.

### 1.8 Money

```go
// Correct — shopspring/decimal or equivalent
type Money struct {
    Amount   decimal.Decimal
    Currency string
}

// PROHIBITED
amount float64
```

There is a lint rule for `float64` in `internal/domain`. Do not work around it.

### 1.9 SQL in Go

- `sqlc` for generated, type-safe queries from hand-written SQL. Not an ORM.
- Named parameters. Never string concatenation, never `fmt.Sprintf` into SQL.
- Enumerate columns. `SELECT *` against canonical tables is prohibited — schema drift silently changes test behaviour.
- Every query against a canonical or exception table includes `tenant_id`. The storage layer enforces this; a query path that can omit it is a defect, not a shortcut.

---

## 2. TypeScript and React

### 2.1 Tooling

ESLint (typescript-eslint strict, react-hooks, jsx-a11y), Prettier, `tsc --noEmit` in CI.

```json
{
  "compilerOptions": {
    "strict": true,
    "noUncheckedIndexedAccess": true,
    "noImplicitOverride": true,
    "exactOptionalPropertyTypes": true,
    "verbatimModuleSyntax": true
  }
}
```

`any` is forbidden. `unknown` plus narrowing, or a real type. `@ts-expect-error` requires a comment explaining why and a linked issue.

### 2.2 Structure

- By feature (`features/queue/`), not by type.
- Server state via TanStack Query. Client state via `useState`/`useReducer`. **No global state library** — almost everything in this app is server state, and Redux-shaped solutions here create cache-invalidation bugs that present as stale exception counts.
- The API client is generated from `openapi.yaml`. Never hand-write a fetch call to our own API.

### 2.3 The queue is the performance-critical surface

- TanStack Table with virtualisation. Never render hundreds of thousands of rows.
- Filtering, sorting and pagination are **server-side**. Client-side filtering of an exception queue is a correctness problem as well as a performance one — the user believes they are seeing all matches.
- Debounce filter input; cancel in-flight requests on change.

### 2.4 Formatting money and dates

One shared formatter, tenant-locale aware, currency-aware. Never `toFixed(2)`. Never assume the browser's timezone — accounting dates are dates, not instants, and rendering a posting date shifted by a timezone produces weekend-posting findings that are simply wrong.

### 2.5 Accessibility

Keyboard navigation through the queue is a working requirement, not a nice-to-have — auditors process hundreds of items a day and will not reach for a mouse. `jsx-a11y` errors fail the build.

---

## 3. SQL and dbt

### 3.1 Style

`sqlfluff` with the dbt templater, in CI. Lowercase keywords, trailing commas, CTEs over subqueries, one column per line.

### 3.2 Model layering

```
staging/     stg_<source>__<entity>     1:1 with source, renamed, typed, no logic
canonical/   <entity>                   the seven entities, tenant-agnostic
analytics/   cand_<test_id>             candidate generation per test
```

No model reads from another layer's internals. Staging never joins across sources. Canonical never contains test-specific logic.

### 3.3 Required tests on every canonical model

```yaml
models:
  - name: payment
    columns:
      - name: payment_key
        tests: [unique, not_null]
      - name: tenant_id
        tests: [not_null]
      - name: snapshot_id
        tests: [not_null]
      - name: amount
        tests:
          - not_null
          - dbt_utils.accepted_range: {min_value: 0, inclusive: false}
      - name: vendor_id
        tests:
          - relationships: {to: ref('vendor'), field: vendor_id}
```

Plus, on every canonical model, a **row-count reconciliation test** against staging. This is the single most valuable test in the project: it catches the fan-out join that silently triples the population, which is the failure mode that produces fabricated findings with no error raised anywhere.

### 3.4 Candidate models

Every `cand_*` model must:

- Emit the columns required by the evidence chain
- Emit every feature the decision model consumes — the engine performs no lookups
- Use a blocking key on any pairwise comparison, with a comment naming the key
- Carry a header comment stating the test id, the audit objective, and the population definition in prose

```sql
-- AP-01 Duplicate Payments
-- Objective: identify payments that may have been made twice for the same liability.
-- Population: all payments in the period, excluding reversals and intercompany.
-- Blocking key: (vendor_id, amount_band, 30-day window) — see ADR-0012 for why
--               an unblocked pairwise scan is not viable at client volumes.
```

---

## 4. JDM rule authoring

- One decision model per test. File name matches the test id: `rulepack/ap/AP-01.jdm.json`.
- Every model has a fixture set in `rulepack/fixtures/AP-01/` with at least: one clear pass, one clear fail, one boundary case per threshold, and one null-handling case.
- Tenant-varying values are **inputs**, never literals in the table. A hard-coded materiality threshold is a defect.
- Decision tables should be readable by an auditor. If a table needs a comment to explain what it does, the logic probably belongs upstream in SQL.

---

## 5. Naming

| Thing | Convention | Example |
|---|---|---|
| Audit test id | `<AREA>-<NN>` | `AP-01`, `JE-07`, `ITGC-03` |
| dbt candidate model | `cand_<test_id_lower>` | `cand_ap_01` |
| JDM file | `<TEST_ID>.jdm.json` | `AP-01.jdm.json` |
| Go package | singular, no underscores | `exceptions`, not `exception_mgmt` |
| DB table | singular | `payment`, not `payments` |
| DB column | `snake_case` | `posting_date` |
| Migration | `NNNN_verb_noun.sql` | `0042_add_exception_content_hash.sql` |
| React component | `PascalCase` | `ExceptionQueue.tsx` |
| Feature flag | `snake_case` | `duckdb_analytics_path` |

---

## 6. Comments

Comment *why*, not *what*. The code says what.

Worth commenting: non-obvious domain rules, decisions with a rejected alternative, performance constraints that forced a shape, anything that will look wrong to a reader without audit context.

```go
// Sort the pair before hashing so a duplicate-payment exception yields the
// same key regardless of which payment the scan encountered first. Without
// this, re-running after a re-ingest resurrects every dismissed pair.
ids := []string{a.ID, b.ID}
sort.Strings(ids)
```

Not worth commenting: what a well-named function does.

---

## 7. Dead code and debt

Delete rather than comment out; git holds the history. Debt is tracked as an issue labelled `type:debt` and linked from a `// TODO(#412):` comment. A `TODO` without an issue number fails lint.
