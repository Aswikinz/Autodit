param([switch]$Preflight,[switch]$Offline,[switch]$NoBuild)
$ErrorActionPreference='Stop'
$root=Split-Path $PSScriptRoot -Parent
Set-Location -LiteralPath $root
if (-not $NoBuild -and -not (Test-Path -LiteralPath 'Containerfile') -and (Test-Path -LiteralPath 'checksums.txt')) { $Offline=$true }
$podman=(Get-Command podman -ErrorAction SilentlyContinue).Source
if (-not $podman) { $podman='C:\Program Files\RedHat\Podman\podman.exe' }
if (-not (Test-Path -LiteralPath $podman)) { throw 'Install Podman and start Podman Machine first.' }
function Run-Podman { & $podman @args; if ($LASTEXITCODE -ne 0) { throw 'Podman command failed.' } }
Run-Podman info --format '{{.Host.Arch}}'
$pythonScripts = Join-Path (Split-Path (Get-Command python).Source -Parent) 'Scripts'
$env:PATH="$pythonScripts;$env:PATH"
$env:PODMAN_COMPOSE_PROVIDER='podman-compose'
Run-Podman compose version
if ($Preflight) { Write-Host 'Preflight passed: Podman engine and compose provider available. No installation changes made.'; exit 0 }
if (Test-Path -LiteralPath '.env') {
  $versionLine=Get-Content .env | Where-Object { $_ -like 'AUTODIT_VERSION=*' }
  if ($versionLine -ne 'AUTODIT_VERSION=0.1.0') { throw 'Existing installation version differs. Use the upgrade runbook.' }
}
$secretDir=Join-Path $root 'secrets'
New-Item -ItemType Directory -Force -Path $secretDir | Out-Null
New-Item -ItemType Directory -Force -Path (Join-Path $root 'inbox') | Out-Null
foreach ($name in @('db_password','app_password','demo_token','oidc_secret')) {
  $file=Join-Path $secretDir $name
  if (-not (Test-Path -LiteralPath $file)) {
    $bytes=New-Object byte[] 36
    [Security.Cryptography.RandomNumberGenerator]::Create().GetBytes($bytes)
    [IO.File]::WriteAllText($file,[Convert]::ToBase64String($bytes))
  }
}
# Restrict Windows secrets to the current account. No secret enters Git or a build context.
$account=[Security.Principal.WindowsIdentity]::GetCurrent().Name
& icacls.exe $secretDir /inheritance:r /grant:r "${account}:(OI)(CI)F" | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'Could not restrict secret directory permissions.' }
foreach ($name in @('db_password','app_password','demo_token','oidc_secret')) {
  & $podman secret exists "autodit_$name"
  if ($LASTEXITCODE -ne 0) { Run-Podman secret create "autodit_$name" (Join-Path $secretDir $name) }
}
if (-not (Test-Path -LiteralPath '.env')) { Copy-Item -LiteralPath '.env.example' -Destination '.env' }
$settings=@{}
Get-Content .env | ForEach-Object { if ($_ -match '^([A-Z_]+)=(.*)$') { $settings[$matches[1]]=$matches[2] } }
$port=if($env:AUTODIT_PORT){$env:AUTODIT_PORT}elseif($settings['AUTODIT_PORT']){$settings['AUTODIT_PORT']}else{'8088'}
if($port -notmatch '^\d{1,5}$' -or [int]$port -lt 1 -or [int]$port -gt 65535){throw 'AUTODIT_PORT must be a valid TCP port.'}
$publicURL=if($env:AUTODIT_PUBLIC_URL){$env:AUTODIT_PUBLIC_URL}elseif($settings['AUTODIT_PUBLIC_URL']){$settings['AUTODIT_PUBLIC_URL']}else{"http://localhost:$port"}
if ($Offline) { & (Join-Path $PSScriptRoot 'verify.ps1'); Get-ChildItem -LiteralPath 'dist\images' -Filter '*.tar' | ForEach-Object { Run-Podman load -i $_.FullName } }
elseif (-not $NoBuild) { Run-Podman build -t localhost/autodit:0.1.0 -f Containerfile .; Run-Podman build -t localhost/autodit-proxy:0.1.0 -f Containerfile.proxy . }
$composeArgs=@('compose','--env-file','.env','-f','deploy/compose/compose.json')
Run-Podman @composeArgs up -d postgres
Run-Podman @composeArgs --profile setup run --rm migrate
Run-Podman @composeArgs --profile setup run --rm bootstrap
Run-Podman @composeArgs up -d --force-recreate api worker proxy
$ready=$false
for($attempt=0;$attempt -lt 60;$attempt++) { try { $r=Invoke-RestMethod "http://localhost:$port/healthz" -TimeoutSec 3; if($r.status -eq 'ok') {$ready=$true;break} } catch {}; Start-Sleep -Seconds 2 }
if(-not $ready){throw 'Health verification timed out. Check podman compose logs.'}
Write-Host "Autodit is ready at $publicURL"
Write-Host "Access token file: $secretDir\demo_token"
