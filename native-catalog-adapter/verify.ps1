param(
 [Parameter(Mandatory=$true)][string]$NativeSource,
 [Parameter(Mandatory=$true)][string]$OutputDirectory,
 [string]$GoExecutable='go',
 [switch]$Apply,
 [switch]$BackendFixture
)
$ErrorActionPreference='Stop'
$source=(Resolve-Path -LiteralPath $NativeSource).Path
$output=[IO.Path]::GetFullPath($OutputDirectory)
if($output -eq $source -or $output.StartsWith($source+[IO.Path]::DirectorySeparatorChar,[StringComparison]::OrdinalIgnoreCase)){throw 'Use an output directory outside the native source tree.'}
New-Item -ItemType Directory -Path $output -Force | Out-Null
$manifest=Get-Content -LiteralPath (Join-Path $PSScriptRoot 'manifest.json') -Raw | ConvertFrom-Json
$patch=Join-Path $PSScriptRoot $manifest.patch.file
if((Get-FileHash -Algorithm SHA256 -LiteralPath $patch).Hash.ToLowerInvariant() -ne $manifest.patch.sha256){throw 'Patch integrity mismatch.'}
$head=(& git -C $source rev-parse HEAD);if($LASTEXITCODE -ne 0){throw 'Native source must be a fresh Git checkout or verified snapshot repository.'}
$tree=(& git -C $source rev-parse 'HEAD^{tree}');if($LASTEXITCODE -ne 0){throw 'Native tree lookup failed.'}
if($head -ne $manifest.native_revision -and $tree -ne $manifest.local_source_snapshot_tree){throw 'Native source does not match the pinned revision/snapshot tree.'}
$dirty=(& git -C $source status --porcelain --untracked-files=normal);if($LASTEXITCODE -ne 0 -or $dirty){throw 'Use a clean source tree; compose other adapter hooks separately.'}
& git -C $source apply --check -- $patch
if($LASTEXITCODE -ne 0){throw 'Patch preflight failed.'}
if(-not $Apply){Write-Output 'Patch preflight passed. Supply -Apply to run full candidate verification in this disposable source tree.';return}
& git -C $source apply -- $patch
if($LASTEXITCODE -ne 0){throw 'Patch apply failed.'}
$index=Join-Path $source 'web/dist/index.html'
$fixtureUsed=$false
if(-not (Test-Path -LiteralPath $index)){
 if(-not $BackendFixture){throw 'Build the upstream frontend, or supply -BackendFixture for a backend-only test embed.'}
 New-Item -ItemType Directory -Path (Split-Path -Parent $index) -Force | Out-Null
 '<!doctype html><title>Backend compilation fixture only</title><p>Not a production frontend build.</p>' | Set-Content -LiteralPath $index -Encoding utf8
 $fixtureUsed=$true
}
$env:GOTOOLCHAIN='local';$env:GOWORK='off'
$results=[Collections.Generic.List[object]]::new()
function Invoke-VerifiedGo([string]$Name,[string[]]$Arguments){
 & $GoExecutable @Arguments 2>&1 | Tee-Object -FilePath (Join-Path $output ($Name+'.log'))
 $exitCode=$LASTEXITCODE
 $results.Add(@{name=$Name;command=@('go')+$Arguments;exit_code=$exitCode})
 if($exitCode -ne 0){throw ('Verification failed: '+$Name)}
}
Push-Location $source
try {
 Invoke-VerifiedGo 'catalog-middleware-router-tests' @('test','./internal/momiaocatalog','./middleware','./router','-count=1')
 Invoke-VerifiedGo 'native-session-group-tests' @('test','./service','-run','Test.*(Session|AutoGroup|UserUsable|UserSelectable)','-count=1')
 Invoke-VerifiedGo 'vet' @('vet','./internal/momiaocatalog','./middleware','./router','./service')
 Invoke-VerifiedGo 'backend-build' @('build','-o',(Join-Path $output 'new-api-catalog-backend-test'),'.')
} finally {
 Pop-Location
 @{native_revision=$manifest.native_revision;source_head=$head;source_tree=$tree;patch_sha256=$manifest.patch.sha256;backend_fixture_used=$fixtureUsed;postgres_requested=[bool]$env:MOMIAO_CATALOG_TEST_DSN;commands=$results;observed_at=[DateTime]::UtcNow.ToString('o')} | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath (Join-Path $output 'verification-result.json') -Encoding utf8
}
