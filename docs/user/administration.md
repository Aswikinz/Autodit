# Workspace administration

New installations use local accounts. Sign in as `admin` with the generated password in `secrets/admin_password`, then choose a new password. The installer never replaces an existing admin password. Keep the initial secret private. HTTPS is required for access beyond localhost.

Open **Administration** to create accounts and assign roles. Each checkbox explains the work it permits. Administrator access manages accounts and workspace settings. Add operational roles separately when the same person also connects sources or performs audits. Changed accounts must sign in again. The last enabled administrator cannot be removed.

Set the workspace name, reporting currency, locale, timezone and fiscal start month. Define a materiality threshold for every transaction currency. New local workspaces have no currency policy, so scoring stops until the policy is configured. Currency settings never imply conversion or exchange rates. Existing immutable evidence keeps its original policy.

The rule editor opens across the full browser window. Use its Help button for authoring and release instructions.

Passwords use salted PBKDF2-HMAC-SHA256 with 600,000 iterations, following the [OWASP password storage guidance](https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html). Password resets require a change on next login. Account revisions invalidate existing sessions after access changes.
