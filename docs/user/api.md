# Workspace and API guide

## Review workflow

Open or reopened exceptions enter `in_review` with a reason. From review, an
auditor can accept a finding, dismiss it, or suppress it until a required future
time. Comments are append-only. `resolved_in_source` is system-controlled and
occurs only after a successful run of that same source, period and enabled rule.
Disabled or absent entity populations cannot resolve untested exceptions.

The queue filters, sorts and paginates on the server. Detail includes all
observations and an activity trail. **Verify replay** checks the retained Parquet
checksum, original model version and original decision result.

## Roles

| Role | Permissions |
|---|---|
| auditor | Read/dispose exceptions, read runs/rules/assurance, preview data and test analyses |
| audit_manager | Auditor rights plus saved analyses, rule/parameter release and source freshness |
| rule_engineer | Load data, save analyses and graphs, simulate and release audit rules |
| implementer | Load data, save analyses, configure sources, import audit populations and read runs |
| admin | Manage accounts, settings and review workflows; assign operational roles separately |

Roles combine only when deliberately assigned by an administrator or identity provider. Demo mode
combines manager/implementer/engineer for local evaluation. The role guard is
server-side on every endpoint; hidden buttons are not the security boundary.

## API contract

`internal/api/openapi.json` lists the endpoint surface. Generated TypeScript
path types live in `web/src/api/schema.ts`. Cookie sessions are opaque and
HTTP-only. Mutations require the exact configured `Origin` and the
`X-CSRF-Token` supplied by `/api/session`. Tenant scope comes from the verified
identity, never from request body/header overrides.

Principal endpoints: `/api/exceptions`, `/api/exceptions/{key}/disposition`,
`/api/exceptions/{key}/comments`, `/api/observations/{id}/replay`, `/api/runs`,
`/api/imports`, `/api/sources`, `/api/sources/profile`, `/api/rules`,
`/api/rules/simulate`, `/api/parameters`, `/api/assurance`.

Queue parameters: `state`, `rule`, `severity`, `search`, `sort` (severity, oldest,
newest), `page` and `size` (1–100). Mutating dispositions/rules/parameters require
the current `revision`; stale requests return HTTP 409. Monetary values are
decimal strings. HTTP 202 means queued, not completed. Always inspect run status.

The JSON schemas currently describe paths and broad object shapes; the detailed
population contract is in `extraction-contract.md`. Service-account bearer auth,
webhooks and external GRC adapters are not supplied in this release.

## Custom analyses

`POST /api/analyses/preview` accepts `{format,name,data,sheet}`. Formats are
`csv`, `json` (a flat array of objects), or `xlsx` (base64 workbook bytes).
An Excel request without a sheet returns `{sheets:[...]}`. With a sheet selected,
the result is `{name,origin,columns:[{name,type}],rows:[{...}],row_count}`.
Raw cell text is retained. Types are `string`, `number` or `boolean` and are
applied when testing; invalid values appear as row errors.

`POST /api/analyses/connection` uses the database connection contract to test
access and discover tables. A preview request with
`{format:"database",name,connection,table:{schema,name}}` loads the selected
table. It rejects tables exceeding the dataset limits instead of silently
running a sample. Credentials are request-scoped and are never saved in analyses.

`POST /api/analyses/test` accepts `{dataset,selected_columns,model}` and returns
`{total,flagged,errors,results,duration_ms}`. Every result contains a 1-based
`row`, the typed `input`, and `output` or `error`. Retained `trace` values can be
shown in the graph. Inputs within expressions and functions are nested under
`data`, for example `data["Invoice amount"]`. A true boolean `flag` in an
output marks a row as flagged. Tests run in a disposable process with bounded
time, memory and result size; a failed overall execution does not return a
misleading partial success.

`POST /api/analyses` saves `{id?,name,revision,dataset,selected_columns,model,origin?}`.
Use revision 0 for a new analysis. The response includes its ID, new revision,
save time and author. Each version is immutable; stale saves return 409.
`GET /api/analyses?page=1` lists 100 latest analyses per page.
`GET /api/analyses/{id}` returns the latest version; `?revision=1` reads history.
Tenant isolation and CSRF checks apply to all these endpoints. Custom analysis
tests do not create reconciled audit runs or managed review cases.
