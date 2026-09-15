# Install Autodit

## Evaluation deployment

Run the platform as a rootless Linux service account, or on Windows using Podman
Machine. Windows is a development/evaluation target; use Linux for a server.
Podman 4.4+ and a compatible Compose provider are required. The verified
workstation uses Podman 5.8.3, podman-compose 1.6.0 and linux/amd64 containers
through a rootful Podman Machine connection. A separate rootless Linux host
acceptance test remains required before a production deployment.

On Windows, `scripts/install.ps1` locates the standard Podman installation even
when a terminal has not picked up the new PATH. Install Python 3.10+ for the
Compose provider. On Linux, use your distribution's Podman/Compose packages.

From the project root:

```sh
sh scripts/install.sh --preflight
sh scripts/install.sh
```

Or from PowerShell:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\install.ps1 -Preflight
powershell -ExecutionPolicy Bypass -File .\scripts\install.ps1
```

The first online build downloads pinned application dependencies and Debian-based
images. Subsequent deployments can use an offline image bundle. `--no-build` /
`-NoBuild` uses already built images. Do not use it on a host without the images.

Installation creates four random secret files, initializes PostgreSQL's separate
runtime role, migrates the schema, bootstraps the tenant/rulepack, then starts the
API, worker and proxy. It verifies the real database-backed health endpoint.
Host files stay private; the installer creates native `autodit_*` Podman secrets
with read access inside only the services that need them. Use one installation
per Podman connection. Keep secret files and the existing secret store together
when reinstalling; deleting local secrets does not reset an existing database.

Open `http://localhost:8088`. Use `secrets/demo_token` to sign in. Demo mode grants
combined evaluation roles to one local operator; it is not a production identity
configuration. The token is never embedded in the web bundle or committed.

## Rootless Linux prerequisites

Configure subordinate UID/GID ranges for the service account, and enable user
lingering if running at boot. Place container storage and data volumes on a disk
with capacity for retained snapshots and backups. The default high port avoids
privileged-port changes. Use your host's disk monitoring and backup alerts.

Install the generated `deploy/systemd/autodit.service` under
`~/.config/systemd/user/`, with the checkout at `~/autodit`, then enable it:

```sh
systemctl --user daemon-reload
systemctl --user enable --now autodit.service
```

This unit wraps the same Compose topology. Native Quadlet and Helm are not
provided by this release.

## Production identity and TLS

The application supports an existing standards-compliant OIDC provider,
including a client-managed Zitadel deployment. Configure an OIDC application
with callback `https://YOUR-AUDIT-ORIGIN/auth/callback` and authorization code flow.
The ID token must contain:

```json
{
  "autodit_tenant": "11111111-1111-4111-8111-111111111111",
  "autodit_roles": ["auditor"]
}
```

Set `AUTODIT_AUTH_MODE=oidc`, `AUTODIT_PUBLIC_URL=https://YOUR-AUDIT-ORIGIN`,
`AUTODIT_OIDC_ISSUER`, and `AUTODIT_OIDC_CLIENT_ID` in the untracked `.env`.
Replace the contents of `secrets/oidc_secret` with the OIDC client's secret.
Before recreating the API, replace the corresponding native secret on Podman 5:

```sh
podman secret create --replace autodit_oidc_secret secrets/oidc_secret
```

Demo token authentication is then unavailable. Role assignment belongs to the
provider; browser-supplied role/tenant headers are never trusted.

Terminate TLS at the client's existing reverse proxy and forward to Autodit's
loopback port. This keeps certificate custody in the client's established
process. If the ingress is on another host, change the bind address deliberately
and restrict it to that host in the firewall. Use HTTPS for every user session.

OIDC discovery/token/JWKS requests originate from the API. The default internal
network intentionally has no outbound route. Provide an explicit network path
to the approved internal identity provider and enforce destination restrictions
with the client's firewall. Never disable tenant or role checks to solve a
networking failure.

## Offline setup

Build `python scripts/bundle.py` on the connected build host. Verify the tarball
checksum independently after transfer. Extract into an empty deployment
directory, then run the platform-specific installer with `--offline` / `-Offline`.
The installer validates file checksums before loading images. Runtime images
and dependency metadata are included; Podman, its VM and the Compose provider
are host prerequisites and must be provisioned separately.

Development bundles use checksums, not a trusted release signature. Do not
describe these bundles as signed/certified production releases.
