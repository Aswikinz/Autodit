# 01 — Domain Model

This is the most important file in the repository. The rules here determine whether audit findings are correct and defensible. Read it fully before writing code that touches data.

---

## 1. The canonical audit model

Seven entities. Every test, every rule and every screen binds to these and never to a client's source schema. Onboarding a new organisation is a mapping exercise, not a redevelopment exercise.

| Entity | Grain | Notes |
|---|---|---|
| `journal_entry` | One line of a posting (not one document) | The richest source. Header attributes are denormalised onto the line. |
| `invoice` | One invoice header, with `invoice_line` child | AP and AR distinguished by a flag, not by separate tables |
| `payment` | One payment transaction | Links to invoice via `payment_application` (a payment may settle many invoices) |
| `vendor` | One vendor master record, versioned | Bank detail changes are first-class; see §4 |
| `employee` | One employee master record, versioned | Used for SoD, payroll and vendor-employee matching |
| `approval` | One approval event | Who approved what, at what limit, at what time |
| `access_grant` | One grant of a permission to a principal | ITGC and SoD tests |

Supporting dimensions: `account`, `cost_centre`, `period`, `currency`, `tenant`.

### 1.1 Required columns on every canonical entity

```
tenant_id          uuid        not null   -- row-level scoping, on every query
source_system_id   uuid        not null   -- which connected source produced this
source_record_id   text        not null   -- natural key in the source
snapshot_id        uuid        not null   -- which frozen population this came from
period_id          text        not null   -- fiscal period, not calendar
ingested_at        timestamptz not null
```

`(tenant_id, source_system_id, source_record_id, snapshot_id)` is unique.

### 1.2 Money and dates

- Money is `numeric(20,4)` plus a separate `currency_code`. **Never float.** Never a single scaled integer without the currency alongside it.
- Every amount has a transaction-currency value and a reporting-currency value, with the rate and rate date recorded. Tests must state which they use.
- Dates are `date` when they are accounting dates and `timestamptz` when they are event times. Posting date, document date and entry timestamp are three different things and tests routinely need to distinguish them.
- Fiscal periods are not calendar months. Never derive a period by truncating a date; always resolve through the tenant's fiscal calendar.

---

## 2. Exception identity

```go
// Stable across runs, across deployments, across restores.
func ExceptionKey(ruleID string, entityType string, entityID string, periodID string) string {
    h := sha256.Sum256([]byte(strings.Join([]string{
        ruleID, entityType, entityID, periodID,
    }, "\x1f")))
    return hex.EncodeToString(h[:])
}
```

Rules:

- **Never** include a timestamp, run id, row number, random value, or rule *version* in the key. Including the version means every rule edit resurrects every dismissed finding.
- `entityID` is the canonical entity's stable identity — `source_record_id` scoped to the source system — not a surrogate key that changes on re-ingestion.
- For exceptions spanning two records (duplicate payment pairs), sort the two ids lexically before hashing so the pair yields the same key regardless of ordering.

### 2.1 Reconciliation

On each run, for each rule in scope:

| Condition | Outcome |
|---|---|
| Key present in this run, absent in prior open set | `new` |
| Key present in both | `still_open` — do not touch the disposition |
| Key absent in this run, present in prior open set | `resolved_in_source` — auto-close with reason |
| Key present, disposition `suppressed`, suppression not expired | remains suppressed, not surfaced |
| Key present, disposition `suppressed`, suppression expired | returns to the queue as `reopened` |
| Key present, disposition `suppressed`, underlying record changed | returns to the queue as `reopened_changed` regardless of expiry |

"Underlying record changed" is detected by a content hash of the fields the rule consumed, stored with the exception.

### 2.2 Disposition states

```
open → in_review → { accepted | dismissed | suppressed }
suppressed → reopened   (on expiry or content change)
open → resolved_in_source  (system-driven only, never user-driven)
```

Dispositions are never deleted. State transitions are appended to `exception_event`, which is append-only.

---

## 3. Evidence chain

Every exception row carries, non-nullable:

```
rule_id             text
rule_version        text        -- content hash of the JDM model
parameter_set_hash  text        -- hash of the tenant parameters supplied
snapshot_id         uuid        -- the frozen Parquet population
engine_version      text        -- ZEN engine version string
input_hash          text        -- hash of the enriched row the engine evaluated
trace_ref           text        -- pointer to the stored ZEN trace
```

The test for this: given only an exception id, it must be possible to re-execute the exact evaluation and obtain the identical result, using only data still stored in the system, years later.

Do not store these as a JSON blob. They are queried, joined and reported on.

---

## 4. Slowly changing master data

Vendor and employee records change, and the changes are themselves audit evidence — a vendor bank-account change shortly before a large payment is one of the highest-value signals in the domain.

- `vendor` and `employee` are type-2: `valid_from`, `valid_to`, `is_current`.
- A test asking "what were the bank details at the time of payment" must resolve as-of the payment date, not use the current row.
- Where the source provides change documents (SAP `CDHDR`/`CDPOS`), ingest them as first-class `master_data_change` records rather than inferring changes by diffing snapshots. Inferred changes miss anything that changed and changed back between extracts.

---

## 5. Test taxonomy

Two mechanisms. Choosing wrongly is a correctness problem, not a style problem.

### 5.1 Set-based tests — SQL in dbt or DuckDB

Anything requiring aggregation, joins across records, window functions, or comparison between rows.

Examples: duplicate payments, Benford digit distribution, three-way match, round-number clustering, SoD conflict detection, gaps in document sequences, vendor-employee bank account matching.

Output: **candidate exceptions with computed features** — the variance, the day gap, the prior-occurrence count, the digit distribution deviation. Not final findings.

**Blocking keys are mandatory** on any pairwise comparison. Duplicate-payment detection without blocking on (vendor, amount band, date window) is quadratic and will become the slowest element of the run. This is enforced by a test, not by convention.

### 5.2 Row-level decisioning — ZEN decision models

Given one enriched candidate, decide: is this a finding, how severe, who owns it.

Examples: is the variance above tolerance, is the posting date outside working hours for this tenant's calendar, does the amount exceed this approver's limit, does the combination of flags warrant escalation.

The engine receives a flat input object. It performs no lookups and no aggregation.

### 5.3 The boundary, stated concretely

> "Find invoices whose total differs from the matched PO by more than tolerance."

- Matching invoice to PO, computing the variance: **SQL**.
- Deciding whether the variance breaches tolerance and at what severity: **ZEN**.

If you are tempted to pass a list into the engine, the work belongs upstream.

---

## 6. Tie-out and completeness

Before any test executes:

1. Count records ingested per entity for the period.
2. Sum the control amount per entity (for journal entries: debits and credits separately, and they must balance).
3. Compare against the independent control figure supplied with the extract — trial balance, control report, or source-provided totals.
4. On mismatch: halt the run, record both figures and the delta, alert. **Do not proceed.**

The tolerance for tie-out is zero for counts and exact for amounts in transaction currency. Rounding tolerance is permitted only in reporting currency and must be configured explicitly per tenant with a recorded justification.

Without demonstrable completeness, no finding from that run is defensible, because the population tested cannot be shown to be the whole population.

---

## 7. Period handling and late data

Back-dated entries, reversals and post-close adjustments will arrive after a period has already been tested. This is normal, not an error.

- Snapshots are immutable. A revised period produces a **new** snapshot, not a mutation of the old one.
- Exceptions reference the snapshot they were found in. A finding from snapshot A remains bound to A even after snapshot B supersedes it.
- Re-testing a period produces a new run; reconciliation compares against the prior run's open set as normal.
- The UI must be able to show which snapshot a finding came from and whether a newer snapshot exists for that period.

---

## 8. Multi-tenancy

- `tenant_id` on every table, every query, every index.
- Tenant scoping is enforced in one place — the storage layer — not repeated in handlers. A query path that can omit the tenant predicate is a defect.
- Postgres row-level security as a second line of defence, not as the primary mechanism.
- Rule packs are shared. Tenant differences are **parameters**, never forked decision models. See `AGENTS.md` §3.2 and the parameterise-don't-fork rule.

---

## 9. Vocabulary

Use these terms exactly; ambiguity here causes real bugs.

| Term | Means | Does not mean |
|---|---|---|
| **Test** | A shipped analytic in the library (e.g. AP-01 Duplicate Payments) | A unit test |
| **Rule** | A ZEN decision model attached to a test | The test itself |
| **Run** | One scheduled or ad-hoc execution of a test set for a tenant and period | A single test |
| **Candidate** | Output of the set-based stage, pre-decision | A finding |
| **Exception** | A candidate the decision model flagged | Any anomaly |
| **Finding** | An exception an auditor has accepted | An exception |
| **Disposition** | The auditor's decision on an exception | Its status in the pipeline |
| **Snapshot** | An immutable frozen population in Parquet | A database backup |
| **Population** | The complete set of records in scope for a test | A sample |
| **Precision** | accepted ÷ (accepted + dismissed) for a rule | Accuracy |
