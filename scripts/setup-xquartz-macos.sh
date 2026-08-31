#!/usr/bin/env bash
# This code is part of RF Swift by @Penthertz
# Author(s): Sébastien Dudek (@FlUxIuS)
#
# One-command XQuartz setup for X11 GUI forwarding on macOS.
#
# RF Swift forwards a container's X11 display to the host's X server over TCP
# (DISPLAY=<host-ip>:0). On macOS that server is XQuartz, and three things must
# be true for GUI tools (gqrx, sdrpp, ...) to open a window:
#   1. XQuartz is installed.
#   2. "Allow connections from network clients" is enabled (nolisten_tcp=false),
#      so the container can reach the X server over TCP.
#   3. The host has authorised the client (xhost) — RF Swift does this per run.
#
# Steps 1-2 are one-time and easy to forget; this script does them (and refreshes
# xhost) so `rfswift run` / the Workbench "just works". Safe to re-run.

set -euo pipefail

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[0;33m'; CYAN='\033[0;36m'; NC='\033[0m'
say() { printf "%b%s%b\n" "$1" "$2" "$NC"; }

if [[ "$(uname -s)" != "Darwin" ]]; then
    say "$RED" "❌ This script is macOS-only. On Linux, install your distro's xhost (x11-xserver-utils / xorg-xhost)."
    exit 1
fi

# --- 1. Install XQuartz (idempotent) -----------------------------------------
xquartz_installed() { [[ -x /opt/X11/bin/xhost || -d /Applications/Utilities/XQuartz.app ]]; }

if xquartz_installed; then
    say "$GREEN" "✅ XQuartz is already installed."
    fresh_install=0
else
    say "$CYAN" "📦 Installing XQuartz via Homebrew..."
    if ! command -v brew >/dev/null 2>&1; then
        say "$RED" "❌ Homebrew is not installed. Install it first: https://brew.sh"
        exit 1
    fi
    # The XQuartz .pkg needs administrator rights; Homebrew will prompt for it.
    brew install --cask xquartz
    fresh_install=1
    if xquartz_installed; then
        say "$GREEN" "✅ XQuartz installed."
    else
        say "$RED" "❌ XQuartz install did not produce /opt/X11. Reboot and re-run this script."
        exit 1
    fi
fi

# --- 2. Allow connections from network clients (TCP listening) ----------------
# This is the "XQuartz ▸ Settings ▸ Security ▸ Allow connections from network
# clients" checkbox. Without it XQuartz binds no TCP port and the container's
# DISPLAY=<host-ip>:0 has nothing to connect to.
current="$(defaults read org.xquartz.X11 nolisten_tcp 2>/dev/null || echo "unset")"
if [[ "$current" == "0" ]]; then
    say "$GREEN" "✅ 'Allow connections from network clients' already enabled."
    tcp_changed=0
else
    say "$CYAN" "🔧 Enabling 'Allow connections from network clients'..."
    defaults write org.xquartz.X11 nolisten_tcp -bool false
    tcp_changed=1
fi

# --- 3. Apply the setting: (re)start XQuartz ---------------------------------
# A running XQuartz only re-reads nolisten_tcp on start, so restart it when the
# setting changed. On a brand-new install the display socket is not wired up
# until the next login, so we tell the user to log out/in instead of starting.
if [[ "$fresh_install" == "1" ]]; then
    say "$YELLOW" "⚠️  Fresh install: log out and back in once so XQuartz registers the :0 display, then re-run RF Swift."
elif [[ "$tcp_changed" == "1" ]] && pgrep -qx Xquartz 2>/dev/null; then
    say "$CYAN" "🔄 Restarting XQuartz to apply the setting..."
    osascript -e 'quit app "XQuartz"' >/dev/null 2>&1 || true
    sleep 1
    open -a XQuartz >/dev/null 2>&1 || true
    sleep 2
elif ! pgrep -qx Xquartz 2>/dev/null; then
    open -a XQuartz >/dev/null 2>&1 || true
    sleep 2
fi

# --- 4. Refresh the xhost ACL for this host (best-effort) --------------------
# RF Swift also runs this on every `run`; doing it here lets a GUI tool started
# immediately after setup connect without waiting for the next `run`.
ip="$(ipconfig getifaddr en0 2>/dev/null || true)"
if [[ -n "$ip" ]] && [[ -n "${DISPLAY:-}" ]] && command -v /opt/X11/bin/xhost >/dev/null 2>&1; then
    /opt/X11/bin/xhost "+${ip}" >/dev/null 2>&1 && say "$GREEN" "✅ Authorised ${ip} via xhost." || true
fi

say "$GREEN" "🎉 XQuartz setup complete."
if [[ "$fresh_install" == "1" ]]; then
    say "$YELLOW" "   Next: log out and back in, then run your RF Swift GUI tool again."
else
    say "$CYAN" "   You can now run GUI tools (e.g. gqrx) from an RF Swift container."
fi
