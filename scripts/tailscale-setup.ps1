$ErrorActionPreference = 'Stop'

Write-Host '==> Tailscale setup for Денговод'
Write-Host ''

$cli = Get-Command tailscale -ErrorAction SilentlyContinue
if (-not $cli) {
    Write-Host 'Tailscale CLI not found.'
    Write-Host ''
    Write-Host 'Install from: https://tailscale.com/download/windows'
    exit 1
}

$null = & tailscale status 2>&1
if ($LASTEXITCODE -ne 0) {
    Write-Host 'Tailscale is installed but not connected.'
    Write-Host 'Open the Tailscale app and sign in.'
    Write-Host ''
    Write-Host 'Or run in terminal:'
    Write-Host '  tailscale up'
    exit 1
}

$ip = (& tailscale ip -4 2>$null | Select-Object -First 1).Trim()
if (-not $ip) {
    Write-Host 'Tailscale is running, but no IPv4 address was found.'
    Write-Host 'Check the Tailscale app and try again.'
    exit 1
}

Write-Host 'Tailscale is ready.'
Write-Host ''
Write-Host "Tailscale IP: $ip"
Write-Host "App URL (after make dev-remote): http://${ip}:5173"
Write-Host ''
Write-Host 'Next steps:'
Write-Host '  1. Install Tailscale on your phone and sign in to the same account'
Write-Host '  2. On this PC: make dev-remote'
Write-Host "  3. On phone browser: http://${ip}:5173"
