# Security posture

Implemented: server-side role enforcement, tenant-scoped transaction handles,
PostgreSQL forced RLS, exact input validation, bounded uploads/models, OIDC code
flow with PKCE/state/nonce/issuer/audience verification, opaque HttpOnly sessions,
Origin/CSRF validation, restrictive response headers and immutable evidence guards.
Runtime services use non-root application users, read-only application filesystems
and dropped capabilities. PostgreSQL's official initialization entrypoint sets up
its volume before dropping to the database user.

Only Caddy publishes a host port. API/worker/database are on an internal network.
The proxy also has an ingress bridge so rootless published-port replies work.
No client data is sent to a vendor. The browser loads its code and JDM/WASM
assets locally; browser verification asserts no external requests during the
main workflow. Deployment secrets are generated locally and excluded from Git,
container build contexts and distribution bundles.

Operational application logs use static event names and error codes; source rows,
tokens and financial values are not interpolated. Support bundles omit raw logs,
environment and database content. Restrict access to database/proxy logs and
configure client logging policy before production use.

## Explicit evaluation-release limits

- One VM is a single point of failure; restore, not high availability, is provided.
- Existing OIDC and TLS ingress must be configured for real shared deployment.
- No bundled Zitadel/OpenObserve/OTel services, SIEM export, signed release pipeline
  or completed external penetration test is claimed.
- No field-level encryption of client data. Encrypt disks/backups using client controls.
- A database or host owner can bypass application permissions. Append-only guards
  do not make a machine administrator untrusted-but-powerless.
- Sessions are in memory; restarting the API signs users out.
- No client-source credentials are accepted or stored yet: ingestion is file-based.
- Dependency vulnerability scans are point-in-time checks, not ongoing certification.

Report security issues privately to the repository owner at devgru-azki@pm.me.

## Dependency verification

Run `make security` for the pinned Go vulnerability checker and npm audit.
The September 2026 hardening update fixes the application's text normalization,
compression and system dependencies, and the proxy's text, gRPC and
OpenTelemetry dependencies. The text fix addresses
[GO-2026-5970](https://pkg.go.dev/vuln/GO-2026-5970).
Scan release proxy binaries separately with
`go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 -mode=binary PATH_TO_CADDY`.
These checks do not replace OS package, native ZEN library or host security review.

On 2026-09-16, the application source scan reported no vulnerabilities and the
rebuilt Caddy binary scan reported no flagged vulnerable symbols. The proxy
scanner still lists advisories in dependencies whose vulnerable code is not
present on its detected call paths; this is not a claim that every transitive
package is advisory-free. npm audit reported zero known vulnerabilities.
