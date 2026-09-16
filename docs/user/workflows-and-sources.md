# Review cases

An administrator publishes the workflow under **Administration > Review and approval workflow**. Each step has a name, responsible role and optional requirement for a different person from the previous step. Publish after creating users with the required roles.

New findings enter the published workflow automatically. Publishing also enrolls active findings that do not already have a case. Existing cases retain their original workflow version.

In **Review cases**, open a case, inspect its finding evidence and record a note. Complete the current step or send it back for corrections. Managers can assign a step to an enabled account with the correct role. The last step closes the case as a confirmed issue or a dismissed finding. A manager can reopen a case with a reason. Changed evidence can also reopen a closed case.

Case actions record the actor, note, timestamp and resulting step. Updates check the current revision, so concurrent reviewers cannot overwrite each other. Findings managed through a case cannot bypass their workflow through the exception disposition endpoint.

# Source previews

**Sources & mapping** can test PostgreSQL, MySQL and SQL Server connections, list tables and views, and show up to 50 sample rows. Use a read-only database account. Credentials are held only for the current form and request; scheduled database extraction is not configured by this preview.

CSV files and population JSON files show sample records before import. Excel `.xlsx` files support worksheet selection and use the same column mapping and independent control report as CSV. Headers must be unique. Dates must use `YYYY-MM-DD`. Formula cells use saved workbook values; Autodit does not calculate formulas or execute macros. The workbook limit is 16 MB compressed and 64 MB expanded, with 100,000 records and 200 columns per worksheet.

For other source systems, export the same CSV or population JSON contract. The inbox connector accepts complete population JSON files after an atomic rename. Preview samples never substitute for independent control totals.
