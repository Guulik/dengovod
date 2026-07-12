#!/usr/bin/env bash
set -euo pipefail

APP="/Applications/Tailscale.app"
CLI="$(command -v tailscale || true)"

echo "==> Tailscale setup for Денговод"
echo

if [[ ! -d "$APP" ]]; then
  echo "Tailscale app is not installed."
  echo
  echo "Install it (pick one):"
  echo "  1. App Store: search for \"Tailscale\""
  echo "  2. Homebrew:  brew install --cask tailscale"
  echo
  echo "Then open Tailscale from Applications and sign in."
  exit 1
fi

if [[ -z "$CLI" ]]; then
  echo "Tailscale CLI not found. Install the CLI:"
  echo "  brew install tailscale"
  exit 1
fi

if ! tailscale status >/dev/null 2>&1; then
  echo "Tailscale is installed but not connected."
  echo "Open the Tailscale app from Applications and sign in."
  echo
  echo "Or run in terminal:"
  echo "  tailscale up"
  exit 1
fi

IP="$(tailscale ip -4 2>/dev/null | head -1 || true)"
if [[ -z "$IP" ]]; then
  echo "Tailscale is running, but no IPv4 address was found."
  echo "Check the Tailscale app and try again."
  exit 1
fi

echo "Tailscale is ready."
echo
echo "Mac Tailscale IP: $IP"
echo "App URL (after make dev-remote): http://$IP:5173"
echo
echo "Next steps:"
echo "  1. Install Tailscale on your phone and sign in to the same account"
echo "  2. On Mac: make dev-remote"
echo "  3. On phone browser: http://$IP:5173"
