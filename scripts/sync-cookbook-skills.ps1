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
  [switch]$Check
)
$ErrorActionPreference = 'Stop'
$cookbookRoot = (Resolve-Path $Cookbook).Path
$bundled = Join-Path $PSScriptRoot '..\internal\skills\bundled'
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
$stage = Join-Path ([IO.Path]::GetTempPath()) ("tau-bundled-" + [guid]::NewGuid())
New-Item -ItemType Directory $stage, "$stage\skills" | Out-Null
Copy-Item $manifestPath $stage
foreach ($name in $names) {
  $source = Join-Path $cookbookRoot "skills\$name"
  if (-not (Test-Path (Join-Path $source 'SKILL.md'))) { throw "Skill $name has no SKILL.md" }
  Copy-Item -Recurse $source "$stage\skills\$name"
}
# The snapshot is embedded with go:embed, so store text files with LF endings
# even when the cookbook checkout uses CRLF.
Get-ChildItem -Recurse -File $stage | ForEach-Object {
  $bytes = [IO.File]::ReadAllBytes($_.FullName)
  if ($bytes -notcontains 0) {
    $text = [Text.Encoding]::UTF8.GetString($bytes)
    if ($text.Contains("`r`n")) { [IO.File]::WriteAllText($_.FullName, $text.Replace("`r`n", "`n")) }
  }
}
$source = "repo: https://github.com/mgerdov-igt/ai-cookbook`ncommit: $commit`n"
if ($dirty) { $source += "note: synced from a checkout with uncommitted changes`n" }
[IO.File]::WriteAllText((Join-Path $stage 'SOURCE'), $source)
function Get-Digest($root) {
  Get-ChildItem -Recurse -File $root | Sort-Object FullName | ForEach-Object {
    $rel = $_.FullName.Substring((Resolve-Path $root).Path.Length)
    $text = [IO.File]::ReadAllText($_.FullName) -replace "`r`n", "`n"
    "$rel $((Get-FileHash -InputStream ([IO.MemoryStream]::new([Text.Encoding]::UTF8.GetBytes($text)))).Hash)"
  }
}
if ($Check) {
  $drift = (Get-Digest $stage | Where-Object { $_ -notmatch 'SOURCE' }) -join "`n"
  $have = if (Test-Path $bundled) { (Get-Digest $bundled | Where-Object { $_ -notmatch 'SOURCE' }) -join "`n" } else { '' }
  Remove-Item -Recurse -Force $stage
  if ($drift -ne $have) { Write-Host 'Bundled skills differ from the cookbook.'; exit 1 }
  Write-Host 'Bundled skills match the cookbook.'; exit 0
}
if (Test-Path $bundled) { Remove-Item -Recurse -Force $bundled }
Move-Item $stage $bundled
Write-Host "Bundled $($names.Count) skills at $commit into internal\skills\bundled"
