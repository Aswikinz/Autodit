$ErrorActionPreference='Stop'
$root=Split-Path $PSScriptRoot -Parent
Set-Location -LiteralPath $root
$podman=(Get-Command podman -ErrorAction SilentlyContinue).Source
if(-not $podman){$podman='C:\Program Files\RedHat\Podman\podman.exe'}
$stamp=Get-Date -Format 'yyyyMMddTHHmmss'
$backup=Join-Path $root "dist\backups\$stamp"
New-Item -ItemType Directory -Force -Path $backup | Out-Null
$env:PODMAN_COMPOSE_PROVIDER='podman-compose'
& $podman compose --env-file .env -f deploy/compose/compose.json stop -t 120 api worker
if($LASTEXITCODE -ne 0){throw 'Could not quiesce writers.'}
try {
  & $podman exec autodit_postgres_1 pg_dump -U autodit_owner -d autodit -Fc -f /tmp/autodit-backup.dump
  if($LASTEXITCODE -ne 0){throw 'Database backup failed.'}
  & $podman cp autodit_postgres_1:/tmp/autodit-backup.dump (Join-Path $backup 'database.dump')
  if($LASTEXITCODE -ne 0){throw 'Backup copy failed.'}
  & $podman volume export autodit_snapshots --output (Join-Path $backup 'snapshots.tar')
  if($LASTEXITCODE -ne 0){throw 'Snapshot backup failed.'}
  Get-ChildItem -LiteralPath $backup -File | Get-FileHash -Algorithm SHA256 | Select-Object Hash,Path | ConvertTo-Json | Set-Content -Encoding UTF8 (Join-Path $backup 'checksums.json')
} finally { & $podman compose --env-file .env -f deploy/compose/compose.json start api worker }
Write-Host "Backup saved to $backup. Protect it as financial data and copy secrets separately."
