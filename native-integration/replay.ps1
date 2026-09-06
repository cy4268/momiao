param(
 [Parameter(Mandatory=$true)][string]$NativeSource,
 [Parameter(Mandatory=$true)][string]$OutputDirectory,
 [switch]$Apply
)
$ErrorActionPreference='Stop'
$source=(Resolve-Path -LiteralPath $NativeSource).Path
$output=[IO.Path]::GetFullPath($OutputDirectory)
function Invoke-SourceGit([string[]]$Arguments){
 $result=& git -C $source @Arguments
 if($LASTEXITCODE -ne 0){throw ('Git failed: '+($Arguments -join ' '))}
 return $result
}
$gitRoot=[IO.Path]::GetFullPath((Invoke-SourceGit @('rev-parse','--show-toplevel')))
if($source.TrimEnd('/','\') -ne $gitRoot.TrimEnd('/','\')){throw 'NativeSource must be the exact source root, not a parent repository or subdirectory.'}
if($output -eq $source -or $output.StartsWith($source+[IO.Path]::DirectorySeparatorChar,[StringComparison]::OrdinalIgnoreCase)){throw 'Use an output directory outside the source root.'}
if(Invoke-SourceGit @('status','--porcelain','--untracked-files=all')){throw 'Use a clean source checkout; this entry never resets existing work.'}
$manifest=Get-Content -LiteralPath (Join-Path $PSScriptRoot 'manifest.json') -Raw | ConvertFrom-Json
$head=Invoke-SourceGit @('rev-parse','HEAD')
$baseTree=Invoke-SourceGit @('rev-parse','HEAD^{tree}')
if($head -ne $manifest.native_revision -and $baseTree -ne $manifest.source_snapshot_tree){throw 'Source revision/tree differs from the pinned native source.'}
$steps=@(foreach($patch in $manifest.patches){
 $path=[IO.Path]::GetFullPath((Join-Path $PSScriptRoot $patch.path))
 if((Get-FileHash -Algorithm SHA256 -LiteralPath $path).Hash.ToLowerInvariant() -ne $patch.sha256){throw ('Patch integrity mismatch: '+$patch.path)}
 $args=@()
 if($patch.exclude){$args+=('--exclude='+$patch.exclude)}
 [pscustomobject]@{path=$path;arguments=$args}
})
New-Item -ItemType Directory -Path $output -Force | Out-Null
$temporaryIndex=Join-Path $output ('replay-'+[Guid]::NewGuid().ToString('N')+'.index')
$previousIndex=$env:GIT_INDEX_FILE
try {
 $env:GIT_INDEX_FILE=$temporaryIndex
 Invoke-SourceGit @('read-tree','HEAD') | Out-Null
 foreach($step in $steps){Invoke-SourceGit (@('apply','--cached','--whitespace=error-all')+$step.arguments+@('--',$step.path)) | Out-Null}
 $tree=Invoke-SourceGit @('write-tree')
 if($tree -ne $manifest.integrated_tree){throw 'Composed tree differs from the reviewed integrated tree.'}
 Invoke-SourceGit @('diff','--cached','--check') | Out-Null
} finally {
 $env:GIT_INDEX_FILE=$previousIndex
 if(Test-Path -LiteralPath $temporaryIndex){Remove-Item -LiteralPath $temporaryIndex}
}
if($Apply){
 foreach($step in $steps){Invoke-SourceGit (@('apply','--index','--whitespace=error-all')+$step.arguments+@('--',$step.path)) | Out-Null}
 if((Invoke-SourceGit @('write-tree')) -ne $tree){throw 'Applied index differs from preflight tree; preserve this checkout for diagnosis.'}
 if(Invoke-SourceGit @('diff','--name-only')){throw 'Applied worktree differs from its index.'}
 Invoke-SourceGit @('diff','--cached','--check') | Out-Null
}
[ordered]@{native_revision=$manifest.native_revision;source_head=$head;source_tree=$baseTree;integrated_tree=$tree;applied=[bool]$Apply;patches=$manifest.patches;observed_at=[DateTime]::UtcNow.ToString('o')} |
 ConvertTo-Json -Depth 6 | Set-Content -LiteralPath (Join-Path $output 'replay-result.json') -Encoding utf8
Write-Output ('Verified integration tree '+$tree+'; applied='+[bool]$Apply)
