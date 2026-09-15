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
| auditor | Read/dispose exceptions, read runs/rules/assurance, simulate |
| audit_manager | Auditor rights plus rule/parameter release and source freshness |
| rule_engineer | Rule authoring, simulation and release; raw bounded graph editor |
| implementer | Source configuration, profile/import populations, read run metadata |
| admin | Run/source metadata; no exception disposition privilege |

Roles combine only when deliberately assigned by the identity provider. Demo mode
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
