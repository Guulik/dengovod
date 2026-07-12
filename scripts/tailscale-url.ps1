$ErrorActionPreference = 'Stop'

if (-not (Get-Command tailscale -ErrorAction SilentlyContinue)) {
    Write-Error 'tailscale CLI not found. Install from: https://tailscale.com/download/windows'
    exit 1
}

$null = & tailscale status 2>&1
if ($LASTEXITCODE -ne 0) {
    Write-Error 'Tailscale is not connected. Open the Tailscale app and sign in.'
    exit 1
}

$ip = (& tailscale ip -4 2>$null | Select-Object -First 1).Trim()
if (-not $ip) {
    Write-Error 'No Tailscale IPv4 address found.'
    exit 1
}

$port = if ($env:DENGOVOD_PORT) { $env:DENGOVOD_PORT } else { '5173' }
Write-Output "http://${ip}:${port}"
