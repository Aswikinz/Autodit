# Shipped audit tests

These are engineering-defined analytic checks, not representations of certification
against a particular audit standard. Client audit teams must approve the population
and policy inputs before relying on the results.

## AP-01 Duplicate payments

Population: all payments in the supplied complete source/period, excluding
reversals and intercompany transactions. SQL compares stable ordered pairs with
the same vendor ID, currency and exact transaction amount, at most 30 calendar
days apart (inclusive). Materiality includes its boundary. ZEN decides flag,
severity and reviewer role.

Default: high priority; auditor routing; USD materiality 1000.0000.

Limitations: no fuzzy vendor matching, no cross-source/cross-period pairing,
no currency conversion and no amount tolerance. Equal amounts are candidates,
not proof of duplicate settlement. The candidate ceiling halts dense blocks
before they flood the queue.

## JE-01 Weekend postings

Population: each journal line in the complete period. The test uses the accounting
posting date, never the browser timezone or entry timestamp. A line qualifies
when its date falls on a tenant-configured non-working day and debit plus credit
meets transaction-currency materiality.

Default: Saturday/Sunday; medium priority; auditor routing.

Limitations: no public-holiday calendar or after-hours timestamp test. A weekend
posting may be fully authorized. Journal control totals must balance first.

## AP-02 Approval limit breach

Population: each approval event in the complete period. A record qualifies when
its transaction amount strictly exceeds the limit attached to that event and
meets transaction-currency materiality. Equality with the limit is permitted.

Default: high priority; auditor routing.

Limitations: the source must supply the limit applicable at the event date and
in the same currency. No authority is inferred from current employee master data.

## Evidence and version changes

Each observation stores rule version, parameters, snapshot, engine, input hash,
result and trace. Changing priority/routing or tenant materiality creates a new
version used only by future runs. It never changes the first observation's evidence
or resurrects dismissed exceptions. Expired suppressions, or changes to consumed
source fields, reopen qualifying exceptions.

Initial release 0.1.0 introduces these three checks. No prior shipped test behavior
exists to migrate. New joins or population definitions require engineering code,
fixtures and explicit audit-domain review.
