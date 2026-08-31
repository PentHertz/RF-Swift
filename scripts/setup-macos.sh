#!/usr/bin/env bash
# This code is part of RF Swift by @Penthertz
# Author(s): Sébastien Dudek (@FlUxIuS)
#
# Interactive macOS dependency setup - the macOS counterpart of the Windows
# installer's dependency bundle (windows/installer/Bundle.wxs): pick ONE
# engine (Docker Desktop, OrbStack, Podman, Lima, or native Nix) and
# optionally set up X11 (XQuartz) and audio (PulseAudio).
#
# Shipped inside the release .dmg as "RF Swift Setup.command" (double-click
# opens Terminal) and runnable from a repo checkout as scripts/setup-macos.sh.
# Detection-first and idempotent: components already present are skipped, and
# re-running is always safe. Homebrew drives every install except Nix, which
# uses the Determinate installer the Windows bundle and docs also use.
#
# Non-interactive use:
#   RFSWIFT_SETUP_ENGINE=docker|orbstack|podman|lima|nix|none \
#   RFSWIFT_SETUP_XQUARTZ=0|1 RFSWIFT_SETUP_AUDIO=0|1 scripts/setup-macos.sh
set -euo pipefail

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[0;33m'; CYAN='\033[0;36m'; NC='\033[0m'
say() { printf "%b%s%b\n" "$1" "$2" "$NC"; }

if [[ "$(uname -s)" != "Darwin" ]]; then
    say "$RED" "❌ This script is macOS-only. On Linux, use your distro package or scripts/get_rfswift.sh."
    exit 1
fi

# Piped runs (curl ... | bash) leave stdin on the pipe; take the prompts from
# the terminal instead so the menus still work.
if [[ ! -t 0 && -r /dev/tty ]]; then
    exec < /dev/tty
fi

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
have() { command -v "$1" >/dev/null 2>&1; }

# --- Homebrew (the one bootstrap everything else hangs off) ------------------
ensure_brew() {
    if have brew; then return 0; fi
    say "$YELLOW" "Homebrew is required for this step but is not installed."
    read -r -p "Install Homebrew now? [y/N] " answer
    if [[ "$answer" == [yY]* ]]; then
        /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
        # The installer does not update the current shell's PATH.
        [[ -x /opt/homebrew/bin/brew ]] && eval "$(/opt/homebrew/bin/brew shellenv)"
        [[ -x /usr/local/bin/brew ]] && eval "$(/usr/local/bin/brew shellenv)"
        have brew && return 0
    fi
    say "$RED" "❌ Skipping: Homebrew is needed for this component (https://brew.sh)."
    return 1
}

# --- Detection ---------------------------------------------------------------
has_docker()   { [[ -d /Applications/Docker.app ]] || have docker; }
has_orbstack() { [[ -d /Applications/OrbStack.app ]] || have orb; }
has_podman()   { have podman; }
has_lima()     { have limactl; }
has_nix()      { have nix || [[ -x /nix/var/nix/profiles/default/bin/nix ]]; }

say "$CYAN" "🔍 RF Swift macOS setup - current state:"
for probe in "Docker Desktop:has_docker" "OrbStack:has_orbstack" "Podman:has_podman" \
             "Lima:has_lima" "Nix:has_nix"; do
    name="${probe%%:*}"; fn="${probe##*:}"
    if "$fn"; then say "$GREEN" "  ✅ $name is installed"; else say "$YELLOW" "  ⬜ $name not found"; fi
done

# --- rfswift CLI itself (when run from the .dmg, the binary sits alongside) --
if ! have rfswift && [[ -x "$SCRIPT_DIR/rfswift" ]]; then
    read -r -p "Install the rfswift CLI to /usr/local/bin? (sudo password may be asked) [Y/n] " answer
    if [[ "${answer:-y}" != [nN]* ]]; then
        sudo mkdir -p /usr/local/bin
        sudo install -m 0755 "$SCRIPT_DIR/rfswift" /usr/local/bin/rfswift
        say "$GREEN" "✅ rfswift installed to /usr/local/bin"
    fi
fi

# --- Engine choice (mirrors the Windows bundle's ContainerEngine radio) ------
engine="${RFSWIFT_SETUP_ENGINE:-}"
if [[ -z "$engine" ]]; then
    say "$CYAN" ""
    say "$CYAN" "Choose an engine for RF Swift environments:"
    echo "  1) Docker Desktop  - easiest start; no USB passthrough (pick Lima for SDR hardware)"
    echo "  2) OrbStack        - lightweight Docker Desktop alternative; no USB passthrough"
    echo "  3) Podman          - daemonless, runs in a Podman machine VM; no USB passthrough"
    echo "  4) Lima            - RF Swift-managed QEMU VM WITH USB passthrough for SDR/RF gear"
    echo "  5) Nix             - native macOS environments, no VM and no daemon"
    echo "  6) Skip            - keep what is already installed"
    read -r -p "Selection [1-6, default 1]: " choice
    case "${choice:-1}" in
        1) engine=docker ;;
        2) engine=orbstack ;;
        3) engine=podman ;;
        4) engine=lima ;;
        5) engine=nix ;;
        *) engine=none ;;
    esac
fi

case "$engine" in
    docker)
        if has_docker; then
            say "$GREEN" "✅ Docker Desktop already installed."
        elif ensure_brew; then
            brew install --cask docker
            say "$GREEN" "✅ Docker Desktop installed. Launching it once to finish its own setup..."
            open -a Docker || true
        fi
        ;;
    orbstack)
        if has_orbstack; then
            say "$GREEN" "✅ OrbStack already installed."
        elif ensure_brew; then
            brew install --cask orbstack
            say "$GREEN" "✅ OrbStack installed."
        fi
        ;;
    podman)
        if ! has_podman; then
            ensure_brew && brew install podman
        fi
        if have podman; then
            podman machine inspect >/dev/null 2>&1 || podman machine init
            podman machine start 2>/dev/null || true
            say "$GREEN" "✅ Podman machine is set up."
        fi
        ;;
    lima)
        if has_lima && { have qemu-system-aarch64 || have qemu-system-x86_64; }; then
            say "$GREEN" "✅ Lima and QEMU already installed."
        elif ensure_brew; then
            # QEMU (not vz) is required for USB passthrough - see lima/rfswift.yaml.
            brew install qemu lima
            say "$GREEN" "✅ Lima + QEMU installed."
        fi
        say "$CYAN" "ℹ️ rfswift creates and starts its VM on first use: rfswift run --engine lima ..."
        say "$CYAN" "ℹ️ Attach SDR hardware with: rfswift macusb attach --vid 0x... --pid 0x..."
        ;;
    nix)
        if has_nix; then
            say "$GREEN" "✅ Nix already installed."
        else
            # Same installer the Windows bundle's Nix step and the docs use.
            curl --proto '=https' --tlsv1.2 -sSf -L https://install.determinate.systems/nix \
                | sh -s -- install --no-confirm
            say "$GREEN" "✅ Nix installed. Open a new terminal, then: rfswift run --engine nix ..."
        fi
        ;;
    *)
        say "$YELLOW" "Engine setup skipped."
        ;;
esac

# --- X11 GUI forwarding (XQuartz) --------------------------------------------
xq="${RFSWIFT_SETUP_XQUARTZ:-}"
if [[ -z "$xq" ]]; then
    read -r -p "Set up XQuartz for GUI tools (gqrx, sdrpp, ...)? [Y/n] " answer
    [[ "${answer:-y}" == [nN]* ]] && xq=0 || xq=1
fi
if [[ "$xq" == 1 ]]; then
    if [[ -x "$SCRIPT_DIR/setup-xquartz-macos.sh" ]]; then
        # Repo checkout: the dedicated script does the full job (install,
        # network-clients toggle, xhost refresh).
        "$SCRIPT_DIR/setup-xquartz-macos.sh" || true
    else
        if [[ -x /opt/X11/bin/xhost || -d /Applications/Utilities/XQuartz.app ]]; then
            say "$GREEN" "✅ XQuartz already installed."
        elif ensure_brew; then
            brew install --cask xquartz
        fi
        # Containers reach the X server over TCP; XQuartz blocks that by default.
        defaults write org.xquartz.X11 nolisten_tcp -bool false || true
        say "$YELLOW" "⚠️ Log out and back in (or reboot) before the first GUI use."
    fi
fi

# --- Audio (PulseAudio -> CoreAudio) -----------------------------------------
audio="${RFSWIFT_SETUP_AUDIO:-}"
if [[ -z "$audio" ]]; then
    read -r -p "Set up PulseAudio for container audio? [Y/n] " answer
    [[ "${answer:-y}" == [nN]* ]] && audio=0 || audio=1
fi
if [[ "$audio" == 1 ]]; then
    if have pactl; then
        say "$GREEN" "✅ PulseAudio already installed."
    elif ensure_brew; then
        brew install pulseaudio
        brew services start pulseaudio || true
        say "$GREEN" "✅ PulseAudio installed and started."
    fi
fi

say "$CYAN" ""
say "$CYAN" "🎉 Setup finished. Next steps:"
say "$CYAN" "   rfswift system doctor          # verify the setup"
say "$CYAN" "   rfswift run -i sdr_light -n lab"
say "$CYAN" "   Docs: https://rfswift.io/docs/getting-started/"
