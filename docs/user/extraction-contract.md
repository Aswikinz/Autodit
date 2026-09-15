# Extraction contract

The initial connector accepts a complete replacement population for one source
and explicit fiscal period. It does **not** treat a delta as a complete period.
Incremental upstream extracts must be merged to a full population by the client
before publication. Imports are capped at 100,000 records and 32 MB of CSV/file
data in this evaluation release.

## Population envelope

Use `test/fixtures/population.json` as a synthetic format example. Required fields:

| Field | Meaning |
|---|---|
| `source_id` | Stable, configured source ID; never reuse it for another source |
| `period.id` | Tenant's fiscal period identifier |
| `period.start`, `period.end` | Inclusive ISO accounting dates |
| `control_reference` | Reference to an independently produced control report |
| `controls` | One entry for every entity/currency in scope, including explicit zero populations |
| `records` | Full canonical rows for that source/period |

Period definitions cannot be silently changed after first use. Natural keys
allow ASCII letters, digits and `_.:@/-`; control delimiters and pipe characters
are rejected. Grain is one payment, one journal **line**, or one approval event.

## Row columns

CSV headers map to these exact canonical fields. The source console can map
differently named source headers; it never constructs joins.

| Column | Type and interpretation |
|---|---|
| `entity` | `payment`, `journal_entry`, or `approval` |
| `id` | Stable source natural key, unique within entity and snapshot |
| `date` | ISO accounting date, inside the explicit period |
| `amount` | Signed decimal string; payments must be positive |
| `currency` | Three uppercase letters; transaction currency |
| `reporting_amount`, `reporting_currency` | Explicit reporting-currency value and code |
| `exchange_rate`, `rate_date` | Positive decimal rate and ISO rate date |
| `vendor_id` | Required on payments; stable vendor identity |
| `debit`, `credit` | Nonnegative journal values; exactly one positive per line |
| `limit` | Nonnegative limit for approval events |
| `reversal`, `intercompany` | Explicit booleans; excluded from duplicate-payment candidates |

Use decimal strings with at most 16 integer and 4 fractional digits. Exponents,
NaN, floats, missing required values and silent rounding are rejected. Enter
`"0"` for nonapplicable amount/debit/credit/limit columns; use `""` for a
nonapplicable vendor. Reporting values/rates are retained but these three tests
use **transaction-currency** amounts. Configure materiality for each currency
before running it; USD uses the primary policy field.

## Independent controls

Every control contains `entity`, `currency`, `count`, `amount`, `debit`, `credit`.
Counts and transaction-currency sums must match exactly. Journal debit and
credit sums must also balance. No rounding tolerance or guessed fiscal month is
applied. A count derived from the uploaded CSV is not an independent control.

## Scheduled file publication

Write complete population JSON under `inbox/` as `extract.partial`. After the
source's completion signal and independent controls are ready, atomically rename
it to `extract.json`. Autodit queues it automatically and stores a durable
filename/content receipt. Republishing unchanged bytes under the same name
does not queue another run. Use a new dated filename for an intentional retest.
The original file is retained. Invalid files do not prevent other valid files
from being processed.

Additional canonical entity schemas are migrated, but invoice, vendor, employee
and access-grant ingestion/tests are not exposed in this release.
