# ADR 0002 Exact qualification and persistent snapshot storage

Status: Accepted for the first deployable release

## Decisions

The Go decimal implementation computes monetary qualification and calendar
features before calling ZEN. ZEN consumes one enriched row and decides whether
to flag, severity and routing. Auditor-configurable materiality remains a
versioned tenant input. This prevents floating-point coercion from changing a
boundary decision. The guided editor exposes the supported decision table;
arbitrary function nodes and graph topologies are rejected server-side.

Use PostgreSQL for the initial blocked set-based analytics path. The blueprint
permits deferring DuckDB until measured volumes justify a second SQL dialect.
Canonical transforms are explicit typed inserts with a second count gate.
The initial three canonical entities are security-invoker views over an
immutable canonical row table; the other four have dedicated typed tables.

Store content-addressed Parquet on a named persistent volume or client NAS.
The blueprint explicitly retains the NAS option. Atomic no-overwrite commits
and SHA-256 verification preserve snapshots. The API and worker share this
volume. An S3 adapter and dbt-driven transformations remain roadmap items;
neither is silently represented as implemented.

## Consequences

The first deployment needs only PostgreSQL, API, worker and reverse proxy.
Migration and bootstrap are one-shot commands using the same application
image. The runtime database user is separate from the schema owner.

Snapshots, observations and events are append-only at the application layer
and protected by database permissions and triggers. A host or database owner
can still tamper with physical storage; backups and independent checksum
retention remain operational responsibilities.

The bounded graph editor offers severity and routing changes, not arbitrary
new tests. New populations and joins require engineering changes and fixtures.
