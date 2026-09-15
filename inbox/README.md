# Scheduled extracts

Configure a source in Autodit, then publish complete population JSON files here.
Write each file as `name.partial` and atomically rename it to `name.json` only
after the source's completion signal and independent control report are ready.
The worker detects complete files automatically. A durable receipt prevents
duplicate runs for the same filename and content across restarts.

On Linux, grant the mapped container UID 10001 read access to published files
and traversal access to this directory using the service account's filesystem
ACLs. Verify access with `podman exec autodit_worker_1 ls /data/inbox`; files
with owner-only permissions are not readable by the non-root worker. On
SELinux hosts, label the inbox and initialization-script binds for containers
according to the host policy before starting the stack.

Files are never removed by Autodit. Manage incoming-file retention separately
after verifying successful ingestion and the retained evidence snapshot.
