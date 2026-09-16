# Product scope and next steps

Autodit targets a simple continuous-audit workflow: load data, choose columns,
configure a decision, inspect results and follow up on findings. The comparison
below is a scope reference, not a claim of feature parity or compatibility.

## Reference product

The supplied [Galvanize announcement](https://www.diligent.com/company/newsroom/introducing-galvanize-acl-rsam-rebrand-create-grc-category-defining-company)
is dated May 13, 2019. It describes the combined ACL and Rsam business and the
HighBond platform, including data integration, automation, internal audit,
third-party risk and security incident management. It does not specify individual
screens or a complete list of product requirements.

For the data-analysis experience, Diligent's current
[Analytics 19.1 guide](https://help.diligentoneplatform.com/helpdocs/analytics-191/en-us/Content/analytics/getting_started/what_is_acl_analytics.htm)
describes a short path: import a table, analyze it, then optionally export or
report the results. It also describes analysis of complete populations or
samples, preservation of source data and recorded analysis steps.

The [HighBond overview](https://www.diligent.com/products/highbond) adds configurable
GRC workflows and dashboards for communicating results. These are separate needs
from loading a spreadsheet and testing a rule.

## Autodit capabilities

| Area | Available behavior | Remaining work |
| --- | --- | --- |
| Data access | CSV, Excel worksheet and flat JSON rows; PostgreSQL, MySQL and SQL Server connection testing, table discovery, previews and bounded table loading for analysis | Saved database credentials, scheduled database extraction, REST/ERP connectors and connector-specific production acceptance |
| Custom analysis | Column selection and explicit text, number or boolean types; conditions over selected columns; GoRules decision graphs; row results and errors; saved analyses and CSV results | Shared analysis projects, version comparison, recurrence and promotion of custom results into governed findings |
| Audit analytics | Three complete-population tests: duplicate payments, weekend journals and approval-limit breaches; independent control-total reconciliation and exact decimal qualification | Joins, grouping, reconciliation between arbitrary tables, statistical sampling, aging and a broader prebuilt test library |
| Evidence | Immutable source snapshots, versioned audit decisions and policy, observation history and replay verification | Retention policies, external evidence attachments and signed release provenance |
| Review and remediation | Versioned review steps, named assignees, role permissions, separate approval, return for correction, closure and reopening | Due-date rules, escalations, notifications, questionnaires and external respondent portals |
| Reporting | Run history, exception dispositions, test coverage, trends and assurance overview | Custom dashboards, report templates, scheduled distribution and board reporting packs |
| Access and administration | Local accounts, first-login password change, account disabling, role descriptions, tenant isolation, optional OIDC, configurable currencies and display settings | MFA for local accounts, organization hierarchies and delegated administration |
| Operation | Podman deployment, offline installation bundle, durable background jobs, atomic-file inbox ingestion and backup/restore tools | Scheduled database jobs, high availability, multiarchitecture release images and measured large-volume capacity |
| Broader GRC | Review workflows for audit findings | Risk and control registers, control-to-framework mapping, audit plans/workpapers, vendor assessments, policy management and security incident management |

## Next priorities

1. Add saved connection definitions, credential storage and schedules, with clear
   ownership, completeness checks and failed-extraction recovery.
2. Add joins, grouped checks and repeatable extraction before claiming broad audit
   analytics coverage. Financial reconciliations must retain exact amounts and
   independent completeness checks.
3. Connect approved custom findings to governed review cases, with explicit owners,
   due dates and notification rules.
4. Extend the GRC model with risks, controls, frameworks, assessments and reporting
   only with corresponding permission, history and workflow tests.

## Start an analysis

1. Open **Analyze data** and choose a CSV, Excel or JSON file. For Excel, choose
   the worksheet and select **Preview worksheet**. JSON accepts an array of flat
   objects. The database option connects, lists tables and loads the selected
   table within the dataset limit.
2. Inspect the preview, select the columns to use and confirm each column's
   type. Select **Continue to rules**.
3. Add conditions by choosing a column, comparison and value. A row is flagged
   when all conditions match. **Open decision graph** opens the full-window
   editor for tables, expressions, functions and branches. Selected fields are
   available under `data`, for example `data["Invoice amount"]`.
4. Select **Test rules**, then filter the results to flagged rows or errors.
   **Inspect** opens a row's graph trace. **Export results** exports the current
   filter as CSV.
5. Name and save the analysis. Reopen it from **Saved analyses** to test or change
   its stored data and graph. Saving uses revision checks to prevent overwriting
   another person's changes. Auditors can test and inspect; implementers, rule
   engineers and audit managers can save.

## Verification boundaries

Automated checks cover the supported import and rule paths, tenant and role
boundaries, immutable audit evidence and case approvals. A browser test against a
fresh Podman deployment exercises the end-to-end user flow. These checks do not
establish compatibility with every external database, every Excel workbook or the
complete Diligent product family. MySQL and SQL Server still need live-server
acceptance in the intended environment.

The financial audit pipeline requires a complete extract and independent control
totals. An exploratory analysis of an uploaded file or a database sample is not a
statement that the full source population was audited.

## Analysis limits

An exploratory dataset can contain up to 10,000 rows and 200 columns, with a
16 MiB size limit. Text retains the supplied values; number columns use the
decision engine's ordinary numeric representation. Keep long identifiers and
values requiring exact decimal preservation as text. Use the reconciled audit
pipeline for the shipped financial tests and exact money boundaries.

Executable graphs support Request, Response, Decision table, Expression,
Function and Switch nodes. Graphs must have one Request, a reachable Response
for every path, no cycles and no disconnected nodes. The limit is 32 nodes and
64 connections. Referenced decisions and custom node loaders are not installed.
Evaluation has a 20-second wall-clock limit in a disposable process. Row-level
conversion or execution failures remain visible in the results.
