$ErrorActionPreference='Stop'
$root=Split-Path $PSScriptRoot -Parent
$manifest=Join-Path $root 'checksums.txt'
if(-not(Test-Path -LiteralPath $manifest)){throw 'Bundle checksum manifest missing.'}
foreach($line in Get-Content -LiteralPath $manifest){
  $parts=$line -split '  ',2
  if($parts.Count -ne 2){throw 'Malformed checksum manifest.'}
  $target=[IO.Path]::GetFullPath((Join-Path $root $parts[1]))
  if(-not $target.StartsWith($root+[IO.Path]::DirectorySeparatorChar,[StringComparison]::OrdinalIgnoreCase)){throw 'Checksum path escapes bundle.'}
  if((Get-FileHash -LiteralPath $target -Algorithm SHA256).Hash.ToLower() -ne $parts[0]){throw "Checksum mismatch: $($parts[1])"}
}
Write-Host 'Bundle checksums verified. Development bundle; provenance must be verified separately.'
