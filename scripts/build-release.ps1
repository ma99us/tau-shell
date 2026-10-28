<#
.SYNOPSIS
Builds a portable TauShell release ZIP (used by CI and for local dry runs).

.DESCRIPTION
Builds the frontend, generates the icon and version resources, builds
taushell.exe with the version stamped in, bundles the pinned ConPTY, and writes
dist\TauShell-<version>-windows-x64.zip plus dist\SHA256SUMS.txt.
Requires Node/npm, Go, and Python with Pillow.

-SkipPackage stops after staging the folder so the exe can be signed;
-PackageOnly then zips the already staged (and signed) folder.
#>
param(
    [Parameter(Mandatory = $true)][string]$Version,
    [string]$OutputDir = (Join-Path (Split-Path -Parent $PSScriptRoot) 'dist'),
    [switch]$SkipFrontend,
    [switch]$SkipPackage,
    [switch]$PackageOnly
)

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$Version = $Version.TrimStart('v')
if ($Version -notmatch '^\d+\.\d+\.\d+([-+][0-9A-Za-z.-]+)?$') {
    throw "Version '$Version' is not SemVer (x.y.z)"
}
$name = "TauShell-$Version-windows-x64"
$stage = Join-Path $OutputDir $name

function Invoke-Step([string]$Label, [scriptblock]$Action) {
    Write-Host "== $Label"
    & $Action
    if ($LASTEXITCODE -ne 0) { throw "$Label failed with exit code $LASTEXITCODE" }
}

Push-Location $root
try {
    if (-not $PackageOnly) {
        if (-not $SkipFrontend) {
            Push-Location frontend
            try {
                Invoke-Step 'npm ci' { npm ci }
                Invoke-Step 'frontend build' { npm run build }
                Invoke-Step 'token check' { npm run check:tokens }
            } finally { Pop-Location }
        }

        Invoke-Step 'icon and version resources' { python build\windows\gen_icon.py $Version }

        if (Test-Path $OutputDir) { Remove-Item -Recurse -Force $OutputDir }
        New-Item -ItemType Directory -Force -Path $stage | Out-Null

        $env:GOOS = 'windows'; $env:GOARCH = 'amd64'; $env:CGO_ENABLED = '0'
        Invoke-Step 'go build' {
            go build -trimpath -tags production -ldflags "-H=windowsgui -s -w -X main.version=$Version" `
                -o (Join-Path $stage 'taushell.exe') ./cmd/taushell
        }

        Write-Host '== bundled ConPTY'
        & (Join-Path $PSScriptRoot 'fetch-conpty.ps1') -Destination $stage

        foreach ($file in 'LICENSE', 'THIRD_PARTY_NOTICES.md', 'README.md') {
            Copy-Item -Force (Join-Path $root $file) $stage
        }
    }

    if ($SkipPackage) {
        Write-Host "Staged $stage (not packaged)"
        return
    }
    if (-not (Test-Path (Join-Path $stage 'taushell.exe'))) {
        throw "Nothing staged at $stage; run without -PackageOnly first"
    }
    $zip = Join-Path $OutputDir "$name.zip"
    if (Test-Path $zip) { Remove-Item -Force $zip }
    Compress-Archive -Path (Join-Path $stage '*') -DestinationPath $zip -CompressionLevel Optimal
    $hash = (Get-FileHash -Algorithm SHA256 $zip).Hash.ToLower()
    "$hash  $name.zip" | Set-Content -Encoding ascii (Join-Path $OutputDir 'SHA256SUMS.txt')
    Write-Host "Built $zip"
    Write-Host "SHA256 $hash"
} finally {
    Pop-Location
}
