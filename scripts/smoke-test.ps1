<#
.SYNOPSIS
End-to-end smoke test: launches a built TauShell in an isolated profile and
checks that it starts, renders Home and has its whole bridge bound.
.PARAMETER Zip
A release ZIP (TauShell-<version>-windows-x64.zip) to extract and test.
.PARAMETER Exe
Alternatively, a taushell.exe (with its sibling files) to test in place.
.PARAMETER Version
Expected app version; omit to skip that check.
#>
param(
    [string]$Zip,
    [string]$Exe,
    [string]$Version
)
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$work = Join-Path ([IO.Path]::GetTempPath()) ("taushell-smoke-" + [Guid]::NewGuid().ToString('N').Substring(0, 8))
$profileDir = Join-Path $work 'profile'
$tempDir = Join-Path $work 'temp'
New-Item -ItemType Directory -Force -Path $profileDir, $tempDir | Out-Null

if ($Zip) {
    $Exe = Join-Path $work 'app\taushell.exe'
    Expand-Archive -Path $Zip -DestinationPath (Join-Path $work 'app')
}
if (-not $Exe -or -not (Test-Path $Exe)) { throw 'Pass -Zip or -Exe pointing at a built TauShell' }

# Isolated user profile and temp dir so the run never touches real sessions or logs.
$saved = @{}
foreach ($name in 'USERPROFILE', 'TEMP', 'TMP', 'APPDATA') {
    $saved[$name] = [Environment]::GetEnvironmentVariable($name)
}
$env:USERPROFILE = $profileDir
$env:TEMP = $tempDir
$env:TMP = $tempDir
# Wails derives the WebView2 data folder from %AppData%\<exe name>, so isolate it.
$env:APPDATA = Join-Path $work 'appdata'
New-Item -ItemType Directory -Force $env:APPDATA | Out-Null

try {
    $nodeArgs = @("$root\e2e\smoke.mjs", '--exe', $Exe, '--log', (Join-Path $tempDir 'TauShell\taushell.log'))
    if ($Version) { $nodeArgs += @('--version', $Version) }
    & node @nodeArgs
    $code = $LASTEXITCODE
    if ($code -ne 0) {
        $log = Join-Path $tempDir 'TauShell\taushell.log'
        if (Test-Path $log) { Write-Host '--- diagnostics log (tail) ---'; Get-Content $log -Tail 40 }
    }
    exit $code
}
finally {
    foreach ($name in $saved.Keys) { [Environment]::SetEnvironmentVariable($name, $saved[$name]) }
    Start-Sleep -Seconds 1
    Remove-Item -Recurse -Force $work -ErrorAction SilentlyContinue
}
