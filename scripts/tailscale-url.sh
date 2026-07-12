#!/usr/bin/env bash
set -euo pipefail

if ! command -v tailscale >/dev/null 2>&1; then
  echo "tailscale CLI not found. Run: brew install tailscale" >&2
  exit 1
fi

if ! tailscale status >/dev/null 2>&1; then
  echo "Tailscale is not connected. Open the Tailscale app and sign in." >&2
  exit 1
fi

IP="$(tailscale ip -4 2>/dev/null | head -1 || true)"
if [[ -z "$IP" ]]; then
  echo "No Tailscale IPv4 address found." >&2
  exit 1
fi

PORT="${DENGOVOD_PORT:-5173}"
echo "http://$IP:$PORT"
