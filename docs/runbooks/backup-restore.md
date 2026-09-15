# Backup and restore

## Scope and impact

The PostgreSQL database and Parquet volume together form the audit record.
Backing up one without the other leaves unreplayable observations. Backups
contain financial data and must be encrypted and access controlled by the client.
Secrets are backed up separately and must never be included in a support bundle.

## Create a consistent backup

```sh
sh scripts/backup.sh
```

Windows:

```powershell
.\scripts\backup.ps1
```

These stop API/worker writes, dump PostgreSQL, export the snapshot volume, then
restart the writers even if the backup fails. Existing browser sessions may
require a new sign-in. Copy the resulting `dist/backups` artifacts and checksums
to protected storage outside this machine. Also preserve `secrets/`, `.env`,
the exact application images/bundle and any external OIDC configuration.

## Verify recovery without touching the live database

```sh
python scripts/restore-drill.py
```

The drill restores into a newly named database and newly named snapshot volume,
compares observation counts, and verifies every restored snapshot SHA-256. It
never restores over the live database. Temporary drill resources are removed
and services restarted. The Python verification container is a development
tool dependency; pre-pull it for an offline drill.

## Recover on a replacement host

1. Verify the backup checksums and the application bundle provenance.
2. Install the matching version on an empty host. Preserve the original tenant ID.
3. Stop API and worker. Confirm the target database and volume are the **new,
   empty deployment** before restoring anything. Never overwrite a live audit store.
4. Restore the PostgreSQL dump using `pg_restore --exit-on-error`, as the schema
   owner, into a fresh database. The destination's `autodit_app` role must exist.
5. Import the matching snapshot archive into a fresh persistent volume. Configure
   the restored API/worker to use this database and volume together.
6. Restore protected secrets, or rotate runtime database credentials consistently.
   Retain the original evidence/rulepack engine images for replay.
7. Start services; verify health, completed-run counts, a dismissed disposition,
   and an original observation's **Verify replay** action.
8. Keep the old host/backup intact until recovery is accepted and retention policy
   permits decommissioning. No automated evidence deletion is supplied.

The current verified recovery test checks counts and snapshot checksums. It is
not a contractual RTO measurement or a substitute for an off-host disaster drill.
