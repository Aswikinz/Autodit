# ADR 0001 Portable audit spine

Status: Accepted for implementation

## Decision

Use Go for the API and worker, PostgreSQL for transactional state, immutable
Parquet snapshots, engineering-owned SQL for candidate generation, ZEN for
single-row decisions, and React for the queue. Ship a rootless Podman Compose
installation with locally generated secrets and persistent named volumes.

Archive documents are retained as source specifications under
`docs/specifications`. Their product requirements inform implementation; their
instructions to agents, example teams, approvals, and sample CI configurations
are reference material and do not override the repository owner's request.

Build and test the audit spine before extending authoring and operations.
Track acceptance evidence and outstanding roadmap items in `docs/IMPLEMENTATION.md`.
Do not describe unmeasured capacity targets or unsigned development bundles as
production certification. Preserve historical evidence; no retention deletion
is enabled without a client policy.

## Correctness boundaries

Money is decimal with currency. Independent controls are mandatory. Fiscal
periods are explicit. Snapshot identities and exception identities are distinct.
Reconciliation only touches the tenant, source, period, and rules successfully
tested by the current run. Every observation retains its original evidence.

## Deployment

Linux is the server target. Windows development uses Podman Machine. Runtime
configuration must support an existing OIDC provider and S3-compatible store.
No remote publishing or pushes are part of this implementation task.
