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
.PARAMETER Lifecycle
Also exercise fixture start/stop, auto-restore, one close confirmation, and clean process exit.
#>
param(
    [string]$Zip,
    [string]$Exe,
    [string]$Version,
    [switch]$Lifecycle
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

$fixtureBin = Join-Path $tempDir 'fixture-bin'
if ($Lifecycle) {
    New-Item -ItemType Directory -Force -Path $fixtureBin | Out-Null
    & go build -o (Join-Path $fixtureBin 'terminal-fixture.exe') ./cmd/terminal-fixture
    if ($LASTEXITCODE -ne 0) { throw 'Could not build the terminal fixture for lifecycle smoke tests' }
}

# Isolated user profile and temp dir so the run never touches real sessions or logs.
$saved = @{}
foreach ($name in 'USERPROFILE', 'TEMP', 'TMP', 'APPDATA', 'PATH') {
    $saved[$name] = [Environment]::GetEnvironmentVariable($name)
}
$env:USERPROFILE = $profileDir
$env:TEMP = $tempDir
$env:TMP = $tempDir
# Wails derives the WebView2 data folder from %AppData%\<exe name>, so isolate it.
$env:APPDATA = Join-Path $work 'appdata'
New-Item -ItemType Directory -Force $env:APPDATA | Out-Null
if ($Lifecycle) {
    $env:PATH = "$fixtureBin;$env:PATH"
    $tauRoot = Join-Path $profileDir '.tau_shell'
    $harnessDir = Join-Path $tauRoot 'harnesses'
    $sessionsDir = Join-Path $tauRoot 'sessions'
    $workingDir = Join-Path $work 'fixture-project'
    New-Item -ItemType Directory -Force -Path $harnessDir, $sessionsDir, $workingDir | Out-Null

    $manifestPath = Join-Path $harnessDir 'e2e-fixture.yaml'
    $manifest = @(
        'schema: 1',
        'id: e2e-fixture',
        'name: TauShell E2E Fixture',
        'short: E2E',
        'executable: terminal-fixture.exe',
        'order: 1000',
        'probes:',
        '  version_args: ["--version"]',
        '  help_args: ["--help"]',
        '  tested_from: "1.0.103"',
        '  tested_before: "1.0.104"',
        'capabilities:',
        '  - { name: Resume a fixture session, option: "--resume" }',
        'launch:',
        '  reserved_args: ["--resume"]',
        'session:',
        '  id: tau',
        '  resume_args: ["--resume", "{id}"]',
        'graceful_exit:',
        '  - { input: "q" }',
        'install:',
        '  vendor_url: https://example.invalid/taushell-e2e',
        'brand:',
        '  mark: monogram',
        '  text: E2',
        '  color: "#10a37f"'
    ) -join "`n"
    [IO.File]::WriteAllText($manifestPath, $manifest + "`n", [Text.UTF8Encoding]::new($false))
    $manifestHash = (Get-FileHash -LiteralPath $manifestPath -Algorithm SHA256).Hash.ToLowerInvariant()
    $trust = @{ 'e2e-fixture' = $manifestHash } | ConvertTo-Json -Compress
    [IO.File]::WriteAllText((Join-Path $tauRoot 'harness-trust.json'), $trust + "`n", [Text.UTF8Encoding]::new($false))

    $session = @(
        'id: e2e-fixture',
        'name: TauShell E2E Fixture',
        'tool: e2e-fixture',
        "working_dir: '$workingDir'",
        'environment:',
        "  TAUSHELL_FIXTURE_LOG: '$workingDir\fixture-events.log'",
        'auto_resume: true'
    ) -join "`n"
    [IO.File]::WriteAllText((Join-Path $sessionsDir 'e2e-fixture.yaml'), $session + "`n", [Text.UTF8Encoding]::new($false))
}

try {
    if ($Version) {
        $info = (Get-Item -LiteralPath $Exe).VersionInfo
        if ($info.ProductVersion -ne $Version) {
            throw "Windows ProductVersion '$($info.ProductVersion)' != '$Version'"
        }
        $parts = ($Version -split '[-+]', 2)[0].Split('.')
        $expectedFileVersion = if ($Version -eq 'dev') { '0.0.0.0' } else { "$($parts[0]).$($parts[1]).$($parts[2]).0" }
        if ($info.FileVersion -ne $expectedFileVersion) {
            throw "Windows FileVersion '$($info.FileVersion)' != '$expectedFileVersion'"
        }
        Write-Host "ok: Windows version resources match $Version"
    }

    $nodeArgs = @("$root\e2e\smoke.mjs", '--exe', $Exe, '--log', (Join-Path $tempDir 'TauShell\taushell.log'))
    if ($Version) { $nodeArgs += @('--version', $Version) }
    if ($Lifecycle) { $nodeArgs += @('--lifecycle', 'true') }
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
