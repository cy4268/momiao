param(
 [Parameter(Mandatory=$true)][string]$NativeBinary,
 [Parameter(Mandatory=$true)][string]$MigratorBinary,
 [Parameter(Mandatory=$true)][string]$OutputDirectory,
 [ValidatePattern('^momiao-native:[a-z0-9._-]+$')][string]$ImageTag='momiao-native:local-9520310d',
 [switch]$Build
)
$ErrorActionPreference='Stop'
$manifest=Get-Content -LiteralPath (Join-Path $PSScriptRoot 'manifest.json') -Raw|ConvertFrom-Json
$output=[IO.Path]::GetFullPath($OutputDirectory)
if(Test-Path -LiteralPath $output){throw 'Use a new empty output directory; existing artifacts are preserved.'}
$sources=@{ 'new-api'=$NativeBinary; 'momiao-admission-migrate'=$MigratorBinary; 'Dockerfile'=(Join-Path $PSScriptRoot 'Dockerfile') }
foreach($item in $manifest.context_files){
 $file=Get-Item -LiteralPath $sources[$item.path]
 if($file.PSIsContainer -or $file.LinkType -or (Get-FileHash -Algorithm SHA256 -LiteralPath $file.FullName).Hash.ToLowerInvariant() -ne $item.sha256){throw ('Container artifact integrity mismatch: '+$item.path)}
}
$context=Join-Path $output 'context'
New-Item -ItemType Directory -Path $context | Out-Null
foreach($name in $sources.Keys){
 $destination=Join-Path $context $name
 Copy-Item -LiteralPath $sources[$name] -Destination $destination
 (Get-Item -LiteralPath $destination).LastWriteTimeUtc=[DateTime]::new(1970,1,1,0,0,0,[DateTimeKind]::Utc)
}
foreach($item in $manifest.context_files){
 if((Get-FileHash -Algorithm SHA256 -LiteralPath (Join-Path $context $item.path)).Hash.ToLowerInvariant() -ne $item.sha256){throw 'Copied container artifact integrity mismatch.'}
}
$result=[ordered]@{base_image=$manifest.base_image;platform='linux/amd64';native_tree=$manifest.native_tree;files=$manifest.context_files;image_built=$false;image_id=$null;image_tag=$ImageTag}
$record=Join-Path $output 'context-manifest.json'
$result|ConvertTo-Json -Depth 6|Set-Content -LiteralPath $record -Encoding utf8
if($Build){
 & docker version --format '{{.Server.Version}}' 2>&1 | Tee-Object -FilePath (Join-Path $output 'runtime.log')
 if($LASTEXITCODE -ne 0){throw 'An already running Linux container engine is required; no daemon is started by this entry.'}
 & docker image inspect $manifest.base_image --format '{{.Id}}' 2>&1 | Tee-Object -FilePath (Join-Path $output 'base-image.log')
 if($LASTEXITCODE -ne 0){throw 'Load the exact approved base image in the build environment first; this entry never substitutes a tag.'}
 $iid=Join-Path $output 'image-id.txt'
 & docker build --pull=false --network=none --platform linux/amd64 --iidfile $iid --tag $ImageTag $context 2>&1 | Tee-Object -FilePath (Join-Path $output 'container-build.log')
 if($LASTEXITCODE -ne 0){throw 'Container build failed; retain the context and log for review.'}
 $id=[IO.File]::ReadAllText($iid).Trim()
 if($id -notmatch '^sha256:[0-9a-f]{64}$'){throw 'Container engine did not return an immutable image ID.'}
 $result.image_built=$true;$result.image_id=$id
 $result|ConvertTo-Json -Depth 6|Set-Content -LiteralPath $record -Encoding utf8
}
Write-Output ('Verified container context; image_built='+$result.image_built)
