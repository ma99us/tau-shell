<#
.SYNOPSIS
Fetches the pinned Microsoft.Windows.Console.ConPTY package and places the
x64 conpty.dll and OpenConsole.exe next to taushell.exe.

.DESCRIPTION
TauShell prefers this bundled ConPTY because it passes harness-requested mouse
modes and terminal query replies through; inbox Windows 10 ConPTY consumes
them. When the files are absent TauShell falls back to inbox ConPTY.
The package hash and Microsoft Authenticode signatures are verified before
any file is copied. Nothing is installed system-wide.
#>
param(
    [string]$Destination = (Split-Path -Parent $PSScriptRoot)
)

$ErrorActionPreference = 'Stop'

$version = '1.25.260930003'
$expectedSha256 = '02B07B349AF66D801159BDF9E440D4A1CE78BB951F37FC8609731665AFDAE7EE'
$url = "https://api.nuget.org/v3-flatcontainer/microsoft.windows.console.conpty/$version/microsoft.windows.console.conpty.$version.nupkg"

$work = Join-Path ([IO.Path]::GetTempPath()) "taushell-conpty-$version"
$package = Join-Path $work 'package.zip'
$extracted = Join-Path $work 'package'
New-Item -ItemType Directory -Force -Path $work | Out-Null

if (-not (Test-Path $package)) {
    Write-Host "Downloading Microsoft.Windows.Console.ConPTY $version"
    Invoke-WebRequest -Uri $url -OutFile $package -UseBasicParsing
}
$actual = (Get-FileHash -Algorithm SHA256 -Path $package).Hash
if ($actual -ne $expectedSha256) {
    Remove-Item -Force $package
    throw "ConPTY package hash mismatch: expected $expectedSha256, got $actual"
}

if (Test-Path $extracted) { Remove-Item -Recurse -Force $extracted }
Expand-Archive -Path $package -DestinationPath $extracted

$files = @(
    (Join-Path $extracted 'runtimes\win-x64\native\conpty.dll'),
    (Join-Path $extracted 'build\native\runtimes\x64\OpenConsole.exe')
)
foreach ($file in $files) {
    $signature = Get-AuthenticodeSignature -FilePath $file
    if ($signature.Status -ne 'Valid' -or $signature.SignerCertificate.Subject -notmatch '^CN=Microsoft Corporation,') {
        throw "Unexpected signature on $(Split-Path -Leaf $file): $($signature.Status) $($signature.SignerCertificate.Subject)"
    }
}

New-Item -ItemType Directory -Force -Path $Destination | Out-Null
foreach ($file in $files) {
    Copy-Item -Force -Path $file -Destination $Destination
}
Write-Host "Bundled ConPTY $version copied to $Destination"
