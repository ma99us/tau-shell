<#
.SYNOPSIS
  Vendors the ai-cookbook skills listed in tau-shell.yaml into internal/skills/bundled.
.DESCRIPTION
  Copies tau-shell.yaml and each manifest-listed skills/<name>/ folder from a
  cookbook checkout, and writes SOURCE with the repository URL and commit.
  -Check changes nothing and exits 1 when the bundled snapshot differs.
#>
param(
  [string]$Cookbook = (Join-Path $PSScriptRoot '..\..\ai-cookbook'),
  [switch]$Check,
  [string]$Destination = (Join-Path $PSScriptRoot '..\internal\skills\bundled')
)
$ErrorActionPreference = 'Stop'
$cookbookRoot = (Resolve-Path $Cookbook).Path
$bundled = [IO.Path]::GetFullPath($Destination)
$bundleParent = Split-Path -Parent $bundled
$textExtensions = [Collections.Generic.HashSet[string]]::new([StringComparer]::OrdinalIgnoreCase)
foreach ($extension in @('.md', '.yaml', '.yml', '.json', '.txt', '.ps1', '.sh', '.py', '.js', '.ts', '.tsx', '.css', '.html', '.xml', '.toml', '.ini', '.cfg', '.csv', '.sql', '.go', '.bat', '.cmd', '.properties', '.gitattributes', '.gitignore')) {
  [void]$textExtensions.Add($extension)
}
$textNames = [Collections.Generic.HashSet[string]]::new([StringComparer]::OrdinalIgnoreCase)
foreach ($name in @('LICENSE', 'NOTICE', 'Makefile', 'Dockerfile', '.gitattributes', '.gitignore')) { [void]$textNames.Add($name) }
$strictUtf8 = [Text.UTF8Encoding]::new($false, $true)
$utf8 = [Text.UTF8Encoding]::new($false)

function Get-SnapshotBytes([string]$path, [switch]$NormalizeText) {
  $bytes = [IO.File]::ReadAllBytes($path)
  $extension = [IO.Path]::GetExtension($path)
  $name = [IO.Path]::GetFileName($path)
  if (-not $textExtensions.Contains($extension) -and -not $textNames.Contains($name)) { return ,$bytes }
  if ($bytes -contains 0) { throw "Classified text file contains NUL: $path" }
  try { $text = $strictUtf8.GetString($bytes) }
  catch { throw "Classified text file is not valid UTF-8: $path. $($_.Exception.Message)" }
  if ($NormalizeText) { $text = $text.Replace("`r`n", "`n") }
  return ,$utf8.GetBytes($text)
}

$manifestPath = Join-Path $cookbookRoot 'tau-shell.yaml'
if (-not (Test-Path $manifestPath)) { throw "No tau-shell.yaml in $cookbookRoot" }
$inSkills = $false; $names = @()
foreach ($line in Get-Content $manifestPath) {
  if ($line -match '^skills:') { $inSkills = $true; continue }
  if ($line -match '^\S' -and $inSkills) { $inSkills = $false }
  if ($inSkills -and $line -match '^\s*-\s+([a-z0-9-]+)\s*$') { $names += $Matches[1] }
}
if (-not $names) { throw 'tau-shell.yaml lists no skills' }
$commit = (git -C $cookbookRoot rev-parse HEAD).Trim()
$dirty = [bool](git -C $cookbookRoot status --porcelain)
$stageParent = [IO.Path]::GetTempPath()
if (-not $Check) {
  New-Item -ItemType Directory -Force $bundleParent | Out-Null
  $stageParent = $bundleParent
}
$stage = Join-Path $stageParent (".tau-bundled-stage-" + [guid]::NewGuid())
New-Item -ItemType Directory $stage, "$stage\skills" | Out-Null
try {
  Copy-Item $manifestPath $stage
  foreach ($name in $names) {
    $source = Join-Path $cookbookRoot "skills\$name"
    if (-not (Test-Path (Join-Path $source 'SKILL.md'))) { throw "Skill $name has no SKILL.md" }
    Copy-Item -Recurse $source "$stage\skills\$name"
  }
  # Normalize only allowlisted UTF-8 text files; all other content stays binary.
  Get-ChildItem -Recurse -File $stage | ForEach-Object {
    $normalized = Get-SnapshotBytes $_.FullName -NormalizeText
    [IO.File]::WriteAllBytes($_.FullName, $normalized)
  }
  $source = "repo: https://github.com/mgerdov-igt/ai-cookbook`ncommit: $commit`n"
  if ($dirty) { $source += "note: synced from a checkout with uncommitted changes`n" }
  [IO.File]::WriteAllText((Join-Path $stage 'SOURCE'), $source, $utf8)
  function Get-Digest($root) {
    $rootPath = (Resolve-Path $root).Path
    Get-ChildItem -Recurse -File $rootPath | Sort-Object FullName | ForEach-Object {
      $rel = $_.FullName.Substring($rootPath.Length)
      $bytes = Get-SnapshotBytes $_.FullName -NormalizeText
      $stream = [IO.MemoryStream]::new([byte[]]$bytes)
      try { $hash = (Get-FileHash -InputStream $stream -Algorithm SHA256).Hash }
      finally { $stream.Dispose() }
      "$rel $hash"
    }
  }
  if ($Check) {
    $drift = (Get-Digest $stage | Where-Object { $_ -notmatch '(^|[\\/])SOURCE$' }) -join "`n"
    $have = if (Test-Path $bundled) { (Get-Digest $bundled | Where-Object { $_ -notmatch '(^|[\\/])SOURCE$' }) -join "`n" } else { '' }
    if ($drift -ne $have) { throw 'Bundled skills differ from the cookbook.' }
    Write-Host 'Bundled skills match the cookbook.'
    return
  }
  $backup = $null
  if (Test-Path $bundled) {
    $backup = Join-Path $bundleParent (".tau-bundled-backup-" + [guid]::NewGuid())
    Move-Item -LiteralPath $bundled -Destination $backup
  }
  try {
    Move-Item -LiteralPath $stage -Destination $bundled
    $stage = $null
  } catch {
    $activationError = $_.Exception.Message
    if ($backup -and (Test-Path $backup) -and -not (Test-Path $bundled)) {
      try { Move-Item -LiteralPath $backup -Destination $bundled }
      catch { throw "Snapshot activation failed: $activationError. Prior bundle remains at '$backup'; rollback failed: $($_.Exception.Message)" }
    }
    throw
  }
  if ($backup -and (Test-Path $backup)) {
    try { Remove-Item -LiteralPath $backup -Recurse -Force }
    catch { throw "New snapshot is active at '$bundled', but prior bundle cleanup failed at '$backup': $($_.Exception.Message)" }
  }
  Write-Host "Bundled $($names.Count) skills at $commit into internal\skills\bundled"
} finally {
  if ($stage -and (Test-Path $stage)) { Remove-Item -Recurse -Force $stage }
}
