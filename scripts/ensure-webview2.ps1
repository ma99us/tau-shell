# Installs the Evergreen WebView2 runtime when a CI runner lacks it; the smoke test needs it.
$ErrorActionPreference = 'Stop'
$keys = 'HKLM:\SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients', 'HKLM:\SOFTWARE\Microsoft\EdgeUpdate\Clients', 'HKCU:\SOFTWARE\Microsoft\EdgeUpdate\Clients'
$found = $keys | Where-Object { Test-Path $_ } | ForEach-Object {
    Get-ChildItem $_ | Where-Object { (Get-ItemProperty $_.PSPath).name -eq 'Microsoft Edge WebView2 Runtime' }
}
if ($found) { Write-Host 'WebView2 runtime present'; return }
Write-Host 'WebView2 runtime missing on this runner; installing the Evergreen runtime'
$setup = Join-Path ($env:RUNNER_TEMP ?? $env:TEMP) 'MicrosoftEdgeWebview2Setup.exe'
Invoke-WebRequest -Uri 'https://go.microsoft.com/fwlink/p/?LinkId=2124703' -OutFile $setup
$p = Start-Process -FilePath $setup -ArgumentList '/silent', '/install' -Wait -PassThru
if ($p.ExitCode -ne 0) { throw "WebView2 setup failed with exit code $($p.ExitCode)" }
