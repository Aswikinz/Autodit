# A run failed or did not arrive

## Impact

A failed or missing run is an assurance gap. It is not a clean control result.
An empty exception queue never proves that an expected population was tested.

## Diagnose

Open Run monitor. Check the source's freshness, run status and last stage.
For `tieout_failed`, open the run and compare the independent report with the
actual extract count/amount/debit/credit figures. No findings or source-resolution
changes are committed from a halted run.

For `ceiling_exceeded`, investigate a mapping/duplicate fan-out or a policy that
generates excessive candidates. The whole run remains uncommitted. Fix the
population or review thresholds; do not bypass the ceiling.

For `failed`, check the source contract, per-currency policy, worker connectivity,
database permissions, snapshot volume capacity and shipped rulepack. The stage
history narrows the problem without exposing source values in operational logs.

```sh
podman compose --env-file .env -f deploy/compose/compose.json ps
podman compose --env-file .env -f deploy/compose/compose.json logs --tail=100 api worker
curl --max-time 3 http://localhost:8088/healthz
```

For a missing file-drop run, verify the source was configured, the completed
file ends in `.json`, and it was atomically renamed after the source completion
signal. Invalid `.json` files remain in place and are reported without blocking
valid files. The worker does not read `.partial` files.

## Resolve

Correct the extract or configuration and submit a new run. Historical inputs,
errors, snapshots and dispositions remain intact. A worker interrupted before
commit safely reclaims its running job when the PostgreSQL lock is released.

Collect `sh scripts/support-bundle.sh` for diagnostics. The bundle includes only
service status, versions and health, not raw logs, credentials or audit data.
