#!/bin/sh
# RF-Swift Enhanced Installer Script
# Usage: curl -fsSL "https://get.rfswift.io/" | sh
# or: wget -qO- "https://get.rfswift.io/" | sh

set -e

# Configuration
GITHUB_REPO="PentHertz/RF-Swift"
# Fallback for the dev channel when GitHub cannot be asked for the newest
# prerelease tag (see get_latest_release); keep it at the current prerelease.
DEV_VERSION="4.0.1-dev"
RELEASE_CHANNEL="${RFSWIFT_CHANNEL:-stable}"
INSTALL_COMPONENTS="${RFSWIFT_INSTALL:-cli}"
WORKBENCH_FORMAT="${RFSWIFT_WORKBENCH_FORMAT:-native}"
# native = distro package (deb/rpm/pacman, with man pages, completions and
# clean uninstall) on Linux, Homebrew cask on macOS; tarball = the classic
# archive install to a directory. Empty = offer the choice (native first).
PKG_FORMAT="${RFSWIFT_PKG_FORMAT:-}"
# udev rules for RF hardware (Linux): 1 = install without asking, 0 = skip,
# empty = ask. Installed by the freshly installed CLI (rfswift host udev).
UDEV_RULES="${RFSWIFT_UDEV:-}"
# Container engine to set up when none is installed: docker, podman, both or
# skip (macOS also: lima); empty = ask. Nix engine: 1 = install, 0 = skip,
# empty = ask. Both make curl|sh and automated installs deterministic.
ENGINE_PREF="${RFSWIFT_ENGINE:-}"
NIX_PREF="${RFSWIFT_NIX:-}"
# Tarball flow: install directory (absolute path, e.g. /usr/local/bin or
# ~/.rfswift/bin); empty = ask.
INSTALL_DIR_PREF="${RFSWIFT_INSTALL_DIR:-}"
NATIVE_INSTALLED=false
FOUND_VERSION=false

# Color codes for better readability
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
MAGENTA='\033[0;35m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Function to output colored text
color_echo() {
  local color=$1
  local text=$2
  case $color in
    "red") printf "${RED}%s${NC}\n" "${text}" ;;
    "green") printf "${GREEN}%s${NC}\n" "${text}" ;;
    "yellow") printf "${YELLOW}%s${NC}\n" "${text}" ;;
    "blue") printf "${BLUE}%s${NC}\n" "${text}" ;;
    "magenta") printf "${MAGENTA}%s${NC}\n" "${text}" ;;
    "cyan") printf "${CYAN}%s${NC}\n" "${text}" ;;
    *) printf "%s\n" "${text}" ;;
  esac
}

# configure_xquartz_tcp enables XQuartz "Allow connections from network clients"
# (nolisten_tcp=false) so the container can reach the host X server over TCP
# (DISPLAY=<host-ip>:0). Without it XQuartz binds no TCP port and GUI tools fail
# with "could not connect to display". macOS-only; safe to call repeatedly.
configure_xquartz_tcp() {
    [ "$(uname)" = "Darwin" ] || return 0
    if [ "$(defaults read org.xquartz.X11 nolisten_tcp 2>/dev/null || echo unset)" = "0" ]; then
        return 0
    fi
    color_echo "cyan" "🔧 Enabling XQuartz 'Allow connections from network clients'... 🔧"
    defaults write org.xquartz.X11 nolisten_tcp -bool false
    if pgrep -qx Xquartz 2>/dev/null; then
        osascript -e 'quit app "XQuartz"' >/dev/null 2>&1 || true
        sleep 1
        open -a XQuartz >/dev/null 2>&1 || true
    fi
}

# Enhanced xhost check with Arch Linux support
check_xhost() {
    if ! command -v xhost >/dev/null 2>&1; then
        # On macOS, xhost may be installed but not in PATH
        if [ "$(uname)" = "Darwin" ] && [ -x /opt/X11/bin/xhost ]; then
            color_echo "yellow" "⚠️ xhost found at /opt/X11/bin/xhost but not in PATH. Adding it."
            export PATH="/opt/X11/bin:$PATH"
            color_echo "green" "✅ xhost is now available. ✅"
            configure_xquartz_tcp
            return
        fi

        color_echo "red" "❌ xhost is not installed on this system. ❌"

        if [ "$(uname)" = "Darwin" ]; then
            color_echo "cyan" "🍎 macOS detected. Installing XQuartz via Homebrew... 📦"
            if ! command -v brew >/dev/null 2>&1; then
                color_echo "red" "❌ Homebrew is not installed. Please install it first: https://brew.sh ❌"
                exit 1
            fi
            brew install --cask xquartz
            export PATH="/opt/X11/bin:$PATH"
            if [ -x /opt/X11/bin/xhost ]; then
                color_echo "green" "✅ XQuartz installed successfully. ✅"
                configure_xquartz_tcp
                color_echo "yellow" "⚠️ Log out and back in once for XQuartz to register the display and apply the setting."
            else
                color_echo "red" "❌ XQuartz installed but xhost not found. Please reboot and try again. ❌"
                exit 1
            fi
        else
            local distro=$(detect_distro)
            case "$distro" in
                "arch")
                    color_echo "cyan" "🏛️ Installing xorg-xhost using pacman on Arch Linux... 📦"
                    sudo pacman -Sy --noconfirm
                    sudo pacman -S --noconfirm --needed xorg-xhost
                    ;;
                "fedora")
                    # Fedora split xorg-x11-server-utils into per-tool packages
                    # (xhost, xrandr, ...); the old name no longer resolves.
                    color_echo "yellow" "📦 Installing xhost using dnf... 📦"
                    sudo dnf install -y xhost
                    ;;
                "rhel"|"centos")
                    if command -v dnf >/dev/null 2>&1; then
                        color_echo "yellow" "📦 Installing xorg-x11-server-utils using dnf... 📦"
                        sudo dnf install -y xorg-x11-server-utils
                    else
                        color_echo "yellow" "📦 Installing xorg-x11-utils using yum... 📦"
                        sudo yum install -y xorg-x11-utils
                    fi
                    ;;
                "debian"|"ubuntu")
                    color_echo "yellow" "📦 Installing x11-xserver-utils using apt... 📦"
                    sudo apt update
                    sudo apt install -y x11-xserver-utils
                    ;;
                "opensuse")
                    color_echo "yellow" "📦 Installing xorg-x11-server using zypper... 📦"
                    sudo zypper install -y xorg-x11-server
                    ;;
                *)
                    color_echo "red" "❌ Unsupported package manager. Please install xhost manually. ❌"
                    exit 1
                    ;;
            esac
            color_echo "green" "✅ xhost installed successfully. ✅"
        fi
    else
        color_echo "green" "✅ xhost is already installed. Moving on. ✅"
        # xhost present, but on macOS the network-clients toggle may still be off
        # from a hand-installed XQuartz — ensure it so GUI forwarding works.
        configure_xquartz_tcp
    fi
}


# Enhanced Arch Linux detection function
is_arch_linux() {
  # Primary check: /etc/arch-release file
  if [ -f /etc/arch-release ]; then
    return 0
  fi
  
  # Secondary check: /etc/os-release contains Arch
  if [ -f /etc/os-release ] && grep -qi "^ID=arch" /etc/os-release; then
    return 0
  fi
  
  # Tertiary check: pacman command exists and /etc/pacman.conf exists
  if command_exists pacman && [ -f /etc/pacman.conf ]; then
    return 0
  fi
  
  return 1
}

# Enhanced Steam Deck detection
is_steam_deck() {
  # Check for Steam Deck specific indicators
  if [ -f /etc/steamos-release ]; then
    return 0
  fi
  
  # Check for Steam Deck hardware identifiers
  if [ -f /sys/devices/virtual/dmi/id/product_name ] && grep -q "Steam Deck" /sys/devices/virtual/dmi/id/product_name 2>/dev/null; then
    return 0
  fi
  
  # Check for deck user
  if [ "$(whoami)" = "deck" ] || [ "$USER" = "deck" ]; then
    return 0
  fi
  
  # Check for Steam Deck specific mount points
  if [ -d /home/deck ] && [ -f /usr/bin/steamos-readonly ]; then
    return 0
  fi
  
  return 1
}

# Enhanced Linux distribution detection
detect_distro() {
  # Enhanced Arch Linux detection first
  if is_arch_linux; then
    echo "arch"
    return 0
  fi
  
  # Check for other distributions
  if [ -f /etc/fedora-release ]; then
    echo "fedora"
  elif [ -f /etc/redhat-release ]; then
    if grep -q "CentOS" /etc/redhat-release; then
      echo "centos"
    else
      echo "rhel"
    fi
  elif [ -f /etc/debian_version ]; then
    if grep -q "Ubuntu" /etc/os-release 2>/dev/null; then
      echo "ubuntu"
    else
      echo "debian"
    fi
  elif [ -f /etc/gentoo-release ]; then
    echo "gentoo"
  elif [ -f /etc/alpine-release ]; then
    echo "alpine"
  elif [ -f /etc/opensuse-release ] || [ -f /etc/SUSE-brand ]; then
    echo "opensuse"
  else
    echo "unknown"
  fi
}

# Enhanced package manager detection
get_package_manager() {
  # Prioritize Arch Linux package manager
  if is_arch_linux && command_exists pacman; then
    echo "pacman"
    return 0
  fi
  
  # Check for other package managers
  if command_exists dnf; then
    echo "dnf"
  elif command_exists yum; then
    echo "yum"
  elif command_exists apt; then
    echo "apt"
  elif command_exists zypper; then
    echo "zypper"
  elif command_exists apk; then
    echo "apk"
  elif command_exists emerge; then
    echo "emerge"
  else
    echo "unknown"
  fi
}

# Check if PipeWire is running
is_pipewire_running() {
  if command_exists pgrep; then
    pgrep -x pipewire >/dev/null 2>&1 && return 0
  fi
  
  # Check for PipeWire socket
  USER_ID=$(id -u 2>/dev/null || echo "1000")
  if [ -S "/run/user/${USER_ID}/pipewire-0" ]; then
    return 0
  fi
  
  return 1
}

# Check if PulseAudio is running
is_pulseaudio_running() {
  if command_exists pulseaudio; then
    pulseaudio --check >/dev/null 2>&1
  else
    return 1
  fi
}

# Detect current audio system
detect_audio_system() {
  if is_pipewire_running; then
    echo "pipewire"
  elif is_pulseaudio_running; then
    echo "pulseaudio"
  else
    echo "none"
  fi
}

# Ensure pactl (pulseaudio-utils) is installed - required by
# 'rfswift host audio enable' to manage the TCP module. Must run even when an
# audio server is already up: modern Ubuntu ships PipeWire by default, but
# pulseaudio-utils is not always installed with it.
ensure_pactl() {
  local distro="$1"

  if command_exists pactl; then
    color_echo "green" "✅ pactl available"
    return 0
  fi

  color_echo "yellow" "⚠️ pactl not found - installing pulseaudio-utils..."
  if ! have_sudo_access; then
    color_echo "red" "❌ sudo access required to install pulseaudio-utils - audio TCP module may not work"
    return 1
  fi

  case "$distro" in
    "arch")
      sudo pacman -S --noconfirm --needed libpulse
      ;;
    "fedora"|"rhel"|"centos")
      if command_exists dnf; then
        sudo dnf install -y pulseaudio-utils
      else
        sudo yum install -y pulseaudio-utils
      fi
      ;;
    "debian"|"ubuntu")
      sudo apt update
      sudo apt install -y pulseaudio-utils
      ;;
    "opensuse")
      sudo zypper install -y pulseaudio-utils
      ;;
    *)
      color_echo "red" "❌ Unsupported distribution - please install pulseaudio-utils (pactl) manually"
      return 1
      ;;
  esac

  if command_exists pactl; then
    color_echo "green" "✅ pactl installed"
    return 0
  fi

  color_echo "red" "❌ pactl still not found - audio TCP module may not work"
  return 1
}

# Check if we should prefer PipeWire for this distribution
should_prefer_pipewire() {
  local distro="$1"
  
  case "$distro" in
    "arch")
      # Arch Linux: PipeWire is modern and well-supported
      return 0
      ;;
    "fedora")
      # PipeWire is default since Fedora 34
      return 0
      ;;
    "ubuntu"|"debian")
      # Available in modern versions
      return 0
      ;;
    "opensuse")
      # OpenSUSE has good PipeWire support
      return 0
      ;;
    "rhel"|"centos")
      # Check if dnf is available (RHEL 8+)
      command_exists dnf
      ;;
    *)
      return 1
      ;;
  esac
}

# Enhanced PipeWire installation with Arch Linux optimization
install_pipewire() {
  local distro="$1"
  
  color_echo "blue" "🔊 Installing PipeWire audio system..."
  
  case "$distro" in
    "arch")
      if have_sudo_access; then
        color_echo "blue" "📦 Using pacman to install PipeWire on Arch Linux..."
        # Update package database first
        sudo pacman -Sy --noconfirm
        # Install PipeWire and related packages
        sudo pacman -S --noconfirm --needed pipewire pipewire-pulse pipewire-alsa pipewire-jack wireplumber libpulse
        # Optional: install additional tools
        sudo pacman -S --noconfirm --needed pipewire-audio || true
      else
        color_echo "red" "sudo access required for package installation"
        return 1
      fi
      ;;
    "fedora")
      if have_sudo_access; then
        sudo dnf install -y pipewire pipewire-pulseaudio pipewire-alsa pipewire-jack-audio-connection-kit pulseaudio-utils wireplumber
      else
        color_echo "red" "sudo access required for package installation"
        return 1
      fi
      ;;
    "rhel"|"centos")
      if command_exists dnf; then
        if have_sudo_access; then
          sudo dnf install -y pipewire pipewire-pulseaudio pipewire-alsa wireplumber pulseaudio-utils
        else
          color_echo "red" "sudo access required for package installation"
          return 1
        fi
      else
        color_echo "yellow" "ℹ️ PipeWire not available on RHEL/CentOS 7, installing PulseAudio instead"
        install_pulseaudio "$distro"
        return $?
      fi
      ;;
    "debian"|"ubuntu")
      if have_sudo_access; then
        sudo apt update
        sudo apt install -y pipewire pipewire-pulse pipewire-alsa wireplumber pulseaudio-utils
      else
        color_echo "red" "sudo access required for package installation"
        return 1
      fi
      ;;
    "opensuse")
      if have_sudo_access; then
        sudo zypper install -y pipewire pipewire-pulseaudio pipewire-alsa wireplumber pulseaudio-utils
      else
        color_echo "red" "sudo access required for package installation"
        return 1
      fi
      ;;
    *)
      color_echo "red" "❌ Unsupported distribution for PipeWire installation"
      return 1
      ;;
  esac
  
  # Enable PipeWire services
  color_echo "blue" "🔧 Enabling PipeWire services..."
  if command_exists systemctl; then
    systemctl --user enable pipewire.service pipewire-pulse.service 2>/dev/null || true
    systemctl --user enable wireplumber.service 2>/dev/null || true
  fi
  
  return 0
}

# Enhanced PulseAudio installation with Arch Linux optimization
install_pulseaudio() {
  local distro="$1"
  
  color_echo "blue" "🔊 Installing PulseAudio audio system..."
  
  case "$distro" in
    "arch")
      if have_sudo_access; then
        color_echo "blue" "📦 Using pacman to install PulseAudio on Arch Linux..."
        # Update package database first
        sudo pacman -Sy --noconfirm
        # Install PulseAudio and related packages
        sudo pacman -S --noconfirm --needed pulseaudio pulseaudio-alsa alsa-utils
        # Optional: install additional tools
        sudo pacman -S --noconfirm --needed pulseaudio-bluetooth pavucontrol || true
      else
        color_echo "red" "sudo access required for package installation"
        return 1
      fi
      ;;
    "fedora")
      if have_sudo_access; then
        sudo dnf install -y pulseaudio pulseaudio-utils alsa-utils
      else
        color_echo "red" "sudo access required for package installation"
        return 1
      fi
      ;;
    "rhel"|"centos")
      if have_sudo_access; then
        if command_exists dnf; then
          sudo dnf install -y pulseaudio pulseaudio-utils alsa-utils
        else
          sudo yum install -y epel-release
          sudo yum install -y pulseaudio pulseaudio-utils alsa-utils
        fi
      else
        color_echo "red" "sudo access required for package installation"
        return 1
      fi
      ;;
    "debian"|"ubuntu")
      if have_sudo_access; then
        sudo apt update
        sudo apt install -y pulseaudio pulseaudio-utils alsa-utils
      else
        color_echo "red" "sudo access required for package installation"
        return 1
      fi
      ;;
    "opensuse")
      if have_sudo_access; then
        sudo zypper install -y pulseaudio pulseaudio-utils alsa-utils
      else
        color_echo "red" "sudo access required for package installation"
        return 1
      fi
      ;;
    *)
      color_echo "red" "❌ Unsupported distribution for PulseAudio installation"
      return 1
      ;;
  esac
  
  return 0
}

# Start PipeWire
start_pipewire() {
  color_echo "blue" "🎵 Starting PipeWire..."
  
  # Try systemd user services first
  if command_exists systemctl; then
    if systemctl --user start pipewire.service pipewire-pulse.service 2>/dev/null; then
      systemctl --user start wireplumber.service 2>/dev/null || true
      color_echo "green" "🎧 PipeWire started via systemd services"
      return 0
    fi
  fi
  
  # Fallback to direct execution
  if command_exists pipewire && command_exists pipewire-pulse; then
    pipewire >/dev/null 2>&1 &
    pipewire-pulse >/dev/null 2>&1 &
    if command_exists wireplumber; then
      wireplumber >/dev/null 2>&1 &
    fi
    sleep 2
    color_echo "green" "🎧 PipeWire started directly"
    return 0
  fi
  
  color_echo "yellow" "⚠️ Could not start PipeWire"
  return 1
}

# Start PulseAudio
start_pulseaudio() {
  color_echo "blue" "🎵 Starting PulseAudio..."
  
  if command_exists pulseaudio; then
    if ! pulseaudio --check >/dev/null 2>&1; then
      pulseaudio --start >/dev/null 2>&1
    fi
    color_echo "green" "🎧 PulseAudio is running"
    return 0
  fi
  
  color_echo "yellow" "⚠️ Could not start PulseAudio"
  return 1
}

# Enhanced audio system check with better Arch Linux support
check_audio_system() {
  color_echo "blue" "🔍 Checking audio system..."
  
  # macOS: install PulseAudio via Homebrew for container audio support
  case "$(uname -s)" in
    Darwin*)
      color_echo "yellow" "🍎 macOS detected. Setting up PulseAudio for container audio..."
      if ! command_exists brew; then
        color_echo "red" "❌ Homebrew is not installed. Please install Homebrew first."
        color_echo "yellow" "Run: /bin/bash -c \"\$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)\""
        return 1
      fi

      if ! command_exists pulseaudio; then
        color_echo "blue" "📦 Installing PulseAudio via Homebrew..."
        brew install pulseaudio
      fi
      color_echo "green" "✅ PulseAudio installed on macOS"

      # Verify pactl is available (comes with Homebrew pulseaudio)
      if ! command_exists pactl; then
        color_echo "red" "❌ pactl not found - required for audio TCP module management"
        color_echo "yellow" "Try reinstalling: brew reinstall pulseaudio"
        return 1
      fi
      color_echo "green" "✅ pactl available"

      color_echo "cyan" "ℹ️ Audio chain: Container -> Lima VM -> port 34567 -> macOS PulseAudio -> CoreAudio"
      color_echo "cyan" "ℹ️ Enable audio with: rfswift host audio enable"
      return 0
      ;;
  esac
  
  # Detect Linux distribution and current audio system
  local distro=$(detect_distro)
  local current_audio=$(detect_audio_system)
  
  color_echo "blue" "🐧 Detected distribution: $distro"
  
  # Special message for Arch Linux
  if [ "$distro" = "arch" ]; then
    color_echo "cyan" "🏛️ Arch Linux detected - using optimized package management with pacman"
  fi
  
  # Check current audio system status. Even when a server is already running,
  # pactl may still be missing (e.g. default PipeWire on Ubuntu), so verify it
  # before returning.
  case "$current_audio" in
    "pipewire")
      color_echo "green" "✅ PipeWire is already running"
      ensure_pactl "$distro" || true
      return 0
      ;;
    "pulseaudio")
      color_echo "green" "✅ PulseAudio is already running"
      ensure_pactl "$distro" || true
      return 0
      ;;
    "none")
      color_echo "yellow" "⚠️ No audio system detected"
      ;;
  esac
  
  # Ask user if they want to install audio system
  if ! prompt_yes_no "Would you like to install an audio system for RF-Swift?" "y"; then
    color_echo "yellow" "⚠️ Audio system installation skipped"
    return 0
  fi
  
  # Determine which audio system to install
  if should_prefer_pipewire "$distro"; then
    color_echo "blue" "🎯 PipeWire is recommended for $distro"
    
    # Check if PipeWire is available
    if command_exists pipewire || command_exists pw-cli; then
      color_echo "green" "✅ PipeWire is already installed"
      start_pipewire
    else
      color_echo "blue" "📦 Installing PipeWire..."
      if install_pipewire "$distro"; then
        color_echo "green" "✅ PipeWire installed successfully"
        start_pipewire
      else
        color_echo "red" "❌ Failed to install PipeWire, falling back to PulseAudio"
        if install_pulseaudio "$distro"; then
          start_pulseaudio
        fi
      fi
    fi
  else
    color_echo "blue" "🎯 PulseAudio is recommended for $distro"
    
    # Check if PulseAudio is available
    if command_exists pulseaudio; then
      color_echo "green" "✅ PulseAudio is already installed"
      start_pulseaudio
    else
      color_echo "blue" "📦 Installing PulseAudio..."
      if install_pulseaudio "$distro"; then
        color_echo "green" "✅ PulseAudio installed successfully"
        start_pulseaudio
      else
        color_echo "red" "❌ Failed to install PulseAudio"
        return 1
      fi
    fi
  fi

  # Verify pactl is available (required for TCP module management)
  ensure_pactl "$distro" || true

  return 0
}

# Display audio system status
show_audio_status() {
  color_echo "blue" "🎵 Audio System Status"
  echo "=================================="
  
  case "$(uname -s)" in
    Darwin*)
      color_echo "yellow" "🍎 macOS: Audio via PulseAudio -> CoreAudio"
      if command_exists pulseaudio; then
        color_echo "green" "✅ PulseAudio installed"
      else
        color_echo "red" "❌ PulseAudio not installed - run: brew install pulseaudio"
      fi
      if command_exists pactl; then
        color_echo "green" "✅ pactl available"
      else
        color_echo "red" "❌ pactl not found"
      fi
      if pgrep -x pulseaudio >/dev/null 2>&1; then
        color_echo "green" "✅ PulseAudio is running"
      else
        color_echo "yellow" "⚠️ PulseAudio is not running - enable with: rfswift host audio enable"
      fi
      echo "=================================="
      return 0
      ;;
  esac
  
  local current_audio=$(detect_audio_system)
  case "$current_audio" in
    "pipewire")
      color_echo "green" "✅ PipeWire is running"
      if command_exists pactl; then
        color_echo "blue" "ℹ️ PulseAudio info:"
        pactl info 2>/dev/null | grep -E "(Server|Version)" || echo "Unable to get detailed info"
      fi
      ;;
    "none")
      color_echo "red" "❌ No audio system detected"
      ;;
  esac
  echo "=================================="
}

# Fun welcome message
fun_welcome() {
  color_echo "cyan" "🎉 WELCOME TO THE RF-Swift Enhanced Installer! 🚀"
  color_echo "yellow" "Prepare yourself for an epic journey in the world of radio frequencies! 📡"
  
  # Show system information
  local distro=$(detect_distro)
  local pkg_mgr=$(get_package_manager)
  
  color_echo "blue" "🖥️ System Information:"
  color_echo "blue" "   OS: $(uname -s)"
  color_echo "blue" "   Architecture: $(uname -m)"
  color_echo "blue" "   Distribution: $distro"
  color_echo "blue" "   Package Manager: $pkg_mgr"
  
  if is_steam_deck; then
    color_echo "magenta" "🎮 Steam Deck detected!"
  fi
}

# Fun thank you message after installation
thank_you_message() {
  color_echo "green" "🌟 You did it! RF-Swift is now ready for action! 🎉"
  color_echo "magenta" "Thank you for installing. You've just taken the first step towards RF mastery! 🔧"
}

# Function to check if a command exists
command_exists() {
  command -v "$1" >/dev/null 2>&1
}

# Function to check if we have sudo privileges
have_sudo_access() {
  if command_exists sudo; then
    sudo -v >/dev/null 2>&1
    return $?
  fi
  return 1
}

# Fetch a URL to stdout with whatever the host has (Debian desktops ship wget
# but not curl). Fails silently; callers decide what a miss means.
http_get() {
  if command_exists curl; then
    curl -fsSL --retry 2 -H "User-Agent: RF-Swift-Installer" "$1" 2>/dev/null
  elif command_exists wget; then
    wget -qO- --header="User-Agent: RF-Swift-Installer" "$1" 2>/dev/null
  else
    return 1
  fi
}

# Debian sets a root password at install time and leaves the first user out of
# the sudo group, so the natural way to run this installer there is a root
# shell (`su -`). Every privileged step calls `sudo cmd`; as root without the
# sudo package, run cmd directly.
provide_sudo_shim() {
  if [ "$(id -u 2>/dev/null)" = "0" ] && ! command_exists sudo; then
    sudo() {
      [ "$1" = "-v" ] && return 0
      "$@"
    }
  fi
}

# True when the account can get root neither directly nor through sudo
# (checked without a password prompt). Used for an early, one-time notice.
lacks_root_access() {
  [ "$(id -u 2>/dev/null)" = "0" ] && return 1
  command_exists sudo || return 0
  sudo -n -v >/dev/null 2>&1 && return 1
  id -nG 2>/dev/null | tr ' ' '\n' | grep -qxE 'sudo|wheel|admin' && return 1
  return 0
}

warn_without_root_access() {
  lacks_root_access || return 0
  me=$(id -un 2>/dev/null)
  grant="usermod -aG sudo ${me}"
  command_exists sudo || grant="apt-get install -y sudo && ${grant}"
  color_echo "yellow" "⚠️  This account cannot use sudo: packages, container engines, Nix, udev rules and a system-wide install are out of reach; only a user-local tarball install (~/.rfswift/bin) can proceed."
  color_echo "cyan" "   Debian leaves the first user out of the sudo group when a root password is set. Either run this installer from a root shell:"
  color_echo "cyan" "     su -                      (then re-run the installer; it sets things up for ${me})"
  color_echo "cyan" "   or grant sudo once and log out and back in:"
  color_echo "cyan" "     su - -c '${grant}'"
}

# Is a directory already on PATH (so a shell alias would be redundant)?
dir_on_path() {
  case ":$PATH:" in *":$1:"*) return 0 ;; esac
  return 1
}

# The user things are set up for: the sudo caller, or the desktop user behind
# a `su -` root shell (logname), else the current account.
get_real_user() {
  if [ -n "${SUDO_USER:-}" ]; then
    echo "$SUDO_USER"
    return 0
  fi
  if [ "$(id -u 2>/dev/null)" = "0" ]; then
    login_user=$(logname 2>/dev/null || true)
    if [ -n "$login_user" ] && [ "$login_user" != "root" ]; then
      echo "$login_user"
      return 0
    fi
  fi
  whoami
}

# Function to prompt user for yes/no with terminal redirection solution
prompt_yes_no() {
  local prompt="$1"
  local default="$2"  # Optional default (y/n)
  local response
  
  # Try to use /dev/tty for interactive input even in pipe scenarios
  if [ -t 0 ]; then
    tty_device="/dev/stdin"
  elif [ -e "/dev/tty" ] && ( : < /dev/tty ) 2>/dev/null; then
    tty_device="/dev/tty"
  else
    # No interactive terminal available, use defaults
    if [ "$default" = "n" ]; then
      echo "${YELLOW}${prompt} (y/n): Defaulting to no (no terminal available)${NC}"
      return 1
    else
      echo "${YELLOW}${prompt} (y/n): Defaulting to yes (no terminal available)${NC}"
      return 0
    fi
  fi
  
  # Try to read from the terminal
  while true; do
    printf "${YELLOW}%s (y/n): ${NC}" "${prompt}"
    if read -r response 2>/dev/null < "$tty_device"; then
      case "$response" in
        [Yy]* ) return 0 ;;
        [Nn]* ) return 1 ;;
        * ) echo "Please answer yes (y) or no (n)." ;;
      esac
    else
      # Failed to read from terminal, use default
      if [ "$default" = "n" ]; then
        echo "${YELLOW}${prompt} (y/n): Defaulting to no (couldn't read from terminal)${NC}"
        return 1
      else
        echo "${YELLOW}${prompt} (y/n): Defaulting to yes (couldn't read from terminal)${NC}"
        return 0
      fi
    fi
  done
}

# Function to prompt user for a numbered choice
prompt_choice() {
  local prompt="$1"
  shift
  local options="$@"
  local response
  local num=1

  if [ -t 0 ]; then
    tty_device="/dev/stdin"
  elif [ -e "/dev/tty" ] && ( : < /dev/tty ) 2>/dev/null; then
    tty_device="/dev/tty"
  else
    printf "${YELLOW}%s: Defaulting to option 1 (no terminal available)${NC}\n" "${prompt}" >&2
    echo "1"
    return 0
  fi

  printf "${YELLOW}%s${NC}\n" "${prompt}" >&2
  for opt in $options; do
    printf "  ${CYAN}%d)${NC} %s\n" "$num" "$opt" >&2
    num=$((num + 1))
  done
  num=$((num - 1))

  while true; do
    printf "${YELLOW}Enter your choice [1-%d]: ${NC}" "$num" >&2
    if read -r response 2>/dev/null < "$tty_device"; then
      case "$response" in
        [1-9]|[1-9][0-9])
          if [ "$response" -ge 1 ] && [ "$response" -le "$num" ] 2>/dev/null; then
            echo "$response"
            return 0
          fi
          ;;
      esac
      echo "Please enter a number between 1 and $num." >&2
    else
      printf "${YELLOW}Defaulting to option 1 (couldn't read from terminal)${NC}\n" >&2
      echo "1"
      return 0
    fi
  done
}

# Function to create an alias for RF-Swift in the user's shell configuration
# After a native package install (binary in /usr/bin), copies left by earlier
# tarball installs shadow it: /usr/local/bin comes before /usr/bin in PATH, and
# the `alias rfswift=~/.rfswift/bin/rfswift` this script used to write wins
# over both. The result is a freshly installed package and a stale binary that
# still answers `rfswift`. Offer to remove the stale copies and the alias, and
# say so when `rfswift` still does not resolve to the packaged binary.
cleanup_legacy_installs() {
  local pkg_bin="/usr/bin/rfswift" wb_bin="/usr/bin/rfswift-workbench" stale="" f
  local user_home
  user_home="$(eval echo "~$(get_real_user)")"
  for f in /usr/local/bin/rfswift /usr/local/bin/rfswift-workbench \
           "${user_home}/.rfswift/bin/rfswift" "${user_home}/.rfswift/bin/rfswift-workbench" \
           "${HOME}/.rfswift/bin/rfswift" "${HOME}/.rfswift/bin/rfswift-workbench"; do
    [ -f "$f" ] || continue
    case " $stale " in *" $f "*) continue ;; esac
    if [ "$f" -ef "$pkg_bin" ] || [ "$f" -ef "$wb_bin" ]; then continue; fi
    stale="$stale $f"
  done
  stale="${stale# }"
  if [ -n "$stale" ]; then
    color_echo "yellow" "⚠️  Older RF-Swift copies from a previous tarball install were found:"
    for f in $stale; do color_echo "yellow" "   - $f"; done
    color_echo "yellow" "   They shadow the package's /usr/bin/rfswift (PATH order), so 'rfswift' would keep running the old version."
    if prompt_yes_no "Remove these old copies?" "y"; then
      for f in $stale; do
        case "$f" in
          /usr/local/bin/*) sudo rm -f "$f" && color_echo "green" "✅ Removed $f" || color_echo "red" "❌ Could not remove $f" ;;
          *) rm -f "$f" && color_echo "green" "✅ Removed $f" || color_echo "red" "❌ Could not remove $f" ;;
        esac
      done
    else
      color_echo "yellow" "   Kept. Run /usr/bin/rfswift explicitly, or remove them later."
    fi
  fi

  # The alias only ever pointed at a tarball location; the package is on PATH.
  local rc
  for rc in "${user_home}/.bashrc" "${user_home}/.bash_profile" "${user_home}/.zshrc" "${user_home}/.config/fish/config.fish"; do
    [ -f "$rc" ] || continue
    grep -q -E "^alias rfswift[ =]" "$rc" 2>/dev/null || continue
    color_echo "yellow" "⚠️  $rc still defines an 'rfswift' alias pointing at the old install; it would hide /usr/bin/rfswift."
    if prompt_yes_no "Remove the alias from $rc?" "y"; then
      sed -i.bak -E '/^alias rfswift[ =]/d' "$rc" 2>/dev/null || sed -i '' -E '/^alias rfswift[ =]/d' "$rc" 2>/dev/null
      color_echo "green" "✅ Alias removed from $rc (backup: $rc.bak). Open a new shell, or run: unalias rfswift"
    fi
  done

  local resolved
  resolved="$(command -v rfswift 2>/dev/null || true)"
  if [ -n "$resolved" ] && [ ! "$resolved" -ef "$pkg_bin" ]; then
    color_echo "yellow" "⚠️  'rfswift' currently resolves to $resolved, not to the package's $pkg_bin. Check your PATH and shell aliases (hash -r / a new shell may be enough)."
  fi
}

create_alias() {
  local bin_path="$1"
  color_echo "blue" "🔗 Setting up an alias for RF-Swift..."
  
  # Get the real user even when run with sudo
  REAL_USER=$(get_real_user)
  case "$REAL_USER" in
    ""|*[!A-Za-z0-9._-]*)
      color_echo "red" "Invalid local user name; refusing to edit a shell profile."
      return 1
      ;;
  esac
  USER_HOME=$(getent passwd "$REAL_USER" 2>/dev/null | cut -d: -f6)
  if [ -z "$USER_HOME" ] && command_exists dscl; then
    USER_HOME=$(dscl . -read "/Users/$REAL_USER" NFSHomeDirectory 2>/dev/null | awk '{print $2}')
  fi
  if [ -z "$USER_HOME" ]; then
    if [ "$REAL_USER" = "$(id -un 2>/dev/null)" ]; then
      USER_HOME=$HOME
    else
      color_echo "red" "Could not determine the home directory for $REAL_USER."
      return 1
    fi
  fi
  
  # Determine shell from the user's default shell
  USER_SHELL=$(getent passwd "${REAL_USER}" 2>/dev/null | cut -d: -f7 | xargs basename 2>/dev/null)
  if [ -z "${USER_SHELL}" ]; then
    USER_SHELL=$(basename "${SHELL}")
  fi
  
  SHELL_RC=""
  ALIAS_LINE="alias rfswift='${bin_path}/rfswift'"
  
  # Determine the correct shell configuration file
  case "${USER_SHELL}" in
    bash)
      # macOS terminals open login shells and read .bash_profile; Linux
      # terminals open interactive non-login shells and read .bashrc (an Arch
      # or Fedora skeleton .bash_profile would otherwise swallow the alias).
      if [ "$(uname -s)" = "Darwin" ] && [ -f "${USER_HOME}/.bash_profile" ]; then
        SHELL_RC="${USER_HOME}/.bash_profile"
      else
        SHELL_RC="${USER_HOME}/.bashrc"
      fi
      ;;
    zsh)
      SHELL_RC="${USER_HOME}/.zshrc"
      ;;
    fish)
      SHELL_RC="${USER_HOME}/.config/fish/config.fish"
      ALIAS_LINE="alias rfswift '${bin_path}/rfswift'"  # fish has different syntax
      ;;
    *)
      color_echo "yellow" "⚠️ Unsupported shell ${USER_SHELL}. Please manually add an alias for rfswift."
      return 1
      ;;
  esac
  
  # Create the configuration file if it doesn't exist
  if [ ! -f "${SHELL_RC}" ]; then
    if [ "${USER_SHELL}" = "fish" ]; then
      # For fish, ensure config directory exists
      mkdir -p "$(dirname "${SHELL_RC}")"
    fi
    touch "${SHELL_RC}"
    if [ $? -ne 0 ]; then
      color_echo "yellow" "⚠️ Unable to create ${SHELL_RC}. Please manually add the alias."
      return 1
    fi
  fi
  
  # Check if alias already exists
  if grep -q "alias rfswift" "${SHELL_RC}" 2>/dev/null; then
    color_echo "yellow" "An existing rfswift alias was found in ${SHELL_RC}"
    if prompt_yes_no "Do you want to replace the existing alias?" "y"; then
      # Remove the existing alias line(s)
      if [ "${USER_SHELL}" = "fish" ]; then
        sed -i.bak '/alias rfswift /d' "${SHELL_RC}" 2>/dev/null || sed -i '' '/alias rfswift /d' "${SHELL_RC}" 2>/dev/null
      else
        sed -i.bak '/alias rfswift=/d' "${SHELL_RC}" 2>/dev/null || sed -i '' '/alias rfswift=/d' "${SHELL_RC}" 2>/dev/null
      fi
      
      # Add the new alias
      if echo "${ALIAS_LINE}" >> "${SHELL_RC}"; then
        color_echo "green" "✅ Updated RF-Swift alias in ${SHELL_RC}"
        color_echo "yellow" "⚡ To use the alias immediately, run: source ${SHELL_RC}"
        return 0
      else
        color_echo "yellow" "⚠️ Failed to update alias in ${SHELL_RC}. Please manually update the alias."
        color_echo "blue" "💡 Run this command to add it manually: echo '${ALIAS_LINE}' >> ${SHELL_RC}"
        return 1
      fi
    else
      color_echo "blue" "Keeping existing alias."
      return 0
    fi
  fi
  
  # Add the alias if it doesn't exist
  if echo "${ALIAS_LINE}" >> "${SHELL_RC}"; then
    color_echo "green" "✅ Added RF-Swift alias to ${SHELL_RC}"
    color_echo "yellow" "⚡ To use the alias immediately, run: source ${SHELL_RC}"
    return 0
  else
    color_echo "yellow" "⚠️ Failed to add alias to ${SHELL_RC}. Please manually add the alias."
    color_echo "blue" "💡 Run this command to add it manually: echo '${ALIAS_LINE}' >> ${SHELL_RC}"
    return 1
  fi
}

# ═══════════════════════════════════════════════════════════════════════════════
# Container Engine Selection: Docker or Podman
# ═══════════════════════════════════════════════════════════════════════════════

# The "also install the other engine?" question when one is present already:
# RFSWIFT_ENGINE answers it (podman or both -> Podman yes, docker or both ->
# Docker yes, anything else -> no); without the knob, ask, default no.
want_second_engine() {
  case "$ENGINE_PREF" in
    "") prompt_yes_no "$2" "n" ;;
    "$1"|both) color_echo "cyan" "   RFSWIFT_ENGINE=${ENGINE_PREF}"; return 0 ;;
    *) return 1 ;;
  esac
}

engine_setup_incomplete() {
  color_echo "yellow" "⚠️  $1 setup did not complete; RF Swift itself is still installed. Set it up later with: rfswift host setup"
}

docker_service_not_started() {
  color_echo "yellow" "⚠️  Docker is installed but its service could not be started (no systemd, e.g. a container or WSL without systemd)."
  color_echo "cyan" "   Start it by hand before using RF Swift: sudo systemctl enable --now docker"
}

# Check which container engines are already installed
detect_container_engines() {
  HAS_DOCKER=false
  HAS_PODMAN=false

  # Check Podman first (may provide a 'docker' shim via podman-docker)
  if command_exists podman || [ -x /usr/bin/podman ] || [ -x /usr/local/bin/podman ]; then
    HAS_PODMAN=true
  fi

  # Check Docker - must have a running daemon to count
  # Skip 'docker info' if the binary is actually podman-docker shim
  if command_exists docker; then
    # Detect podman-docker shim: 'docker --version' contains "podman"
    docker_version_output=$(docker --version 2>/dev/null || true)
    if echo "$docker_version_output" | grep -qi "podman"; then
      # This is podman-docker, not real Docker
      HAS_PODMAN=true
    elif docker info >/dev/null 2>&1; then
      HAS_DOCKER=true
    else
      # Docker binary exists but daemon not running
      HAS_DOCKER=true
      DOCKER_DAEMON_DOWN=true
    fi
  fi
}

# Offer Lima for USB passthrough on macOS (called even when Docker/Podman is present)
offer_lima_for_usb_get_rfswift() {
  echo ""
  color_echo "cyan" "🦙 USB passthrough on macOS"
  color_echo "cyan" "   Docker Desktop and Podman on macOS cannot forward USB devices (SDR dongles,"
  color_echo "cyan" "   HackRF, RTL-SDR, etc.) into containers. Lima runs a QEMU VM with its own"
  color_echo "cyan" "   Docker that supports USB hot-plug for your RF hardware."
  echo ""
  color_echo "cyan" "   Workflow when you need USB:"
  color_echo "cyan" "     rfswift macusb attach --vid 0x1d50 --pid 0x604b  # forward device to VM"
  color_echo "cyan" "     rfswift --engine lima run -i <image>              # run via Lima's Docker"
  color_echo "cyan" "     rfswift macusb detach --vid 0x1d50 --pid 0x604b  # unplug when done"
  echo ""

  if command_exists limactl; then
    color_echo "green" "   Lima is already installed."
    # Offer to update the Lima template if a bundled one is available
    local bundled_template=""
    local script_dir
    script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" 2>/dev/null && pwd)
    for candidate in \
        "${script_dir}/lima/rfswift.yaml" \
        "$(pwd)/lima/rfswift.yaml"; do
      if [ -f "$candidate" ]; then
        bundled_template="$(cd "$(dirname "$candidate")" && pwd)/$(basename "$candidate")"
        break
      fi
    done
    if [ -n "$bundled_template" ]; then
      # Find existing user template or default location
      local user_template=""
      for candidate in \
          "$HOME/.config/rfswift/lima.yaml" \
          "$HOME/.rfswift/lima.yaml"; do
        if [ -f "$candidate" ]; then
          user_template="$candidate"
          break
        fi
      done
      [ -z "$user_template" ] && user_template="$HOME/.config/rfswift/lima.yaml"

      local needs_update=false
      if [ ! -f "$user_template" ]; then
        needs_update=true
      elif ! diff -q "$bundled_template" "$user_template" >/dev/null 2>&1; then
        needs_update=true
      fi

      if $needs_update; then
        echo ""
        color_echo "yellow" "   A newer Lima template is available (kernel modules, Bluetooth, udev rules)."
        if prompt_yes_no "   Would you like to update your Lima template?" "y"; then
          mkdir -p "$(dirname "$user_template")"
          cp "$bundled_template" "$user_template"
          color_echo "green" "   Lima template updated at ${user_template}"
          color_echo "cyan" "   Apply with: rfswift engine lima reconfig"
          color_echo "cyan" "   Or full rebuild: rfswift engine lima reset"
        fi
      fi
    fi

    # Seed the opt-in GPU (krunkit) template if bundled and not already present,
    # so `rfswift --gpu` works out of the box.
    if [ ! -f "$HOME/.config/rfswift/lima-gpu.yaml" ]; then
      for gpu_src in "${script_dir}/lima/rfswift-gpu.yaml" "$(pwd)/lima/rfswift-gpu.yaml"; do
        if [ -f "$gpu_src" ]; then
          mkdir -p "$HOME/.config/rfswift"
          cp "$gpu_src" "$HOME/.config/rfswift/lima-gpu.yaml"
          color_echo "green" "   Added GPU Lima template at ~/.config/rfswift/lima-gpu.yaml"
          break
        fi
      done
    fi

    if ! limactl list --json 2>/dev/null | grep -q '"name":"rfswift"'; then
      color_echo "yellow" "   No rfswift Lima instance yet."
      color_echo "cyan" "   Create with: limactl create --name rfswift lima/rfswift.yaml"
      color_echo "cyan" "   Or let RF Swift auto-create it on first 'rfswift --engine lima run'."
    else
      color_echo "green" "   Lima instance 'rfswift' exists. USB passthrough available."
    fi
  else
    if prompt_yes_no "   Would you like to install Lima for USB passthrough?" "n"; then
      install_lima
    fi
  fi
}

# Main container engine check - replaces the old check_docker()
check_container_engine() {
  color_echo "blue" "🔍 Checking for container engines..."

  detect_container_engines

  # ── Both already installed ──────────────────────────────────────────────
  if [ "$HAS_DOCKER" = true ] && [ "$HAS_PODMAN" = true ]; then
    color_echo "green" "✅ Both Docker and Podman are installed."
    color_echo "cyan" "ℹ️  RF-Swift auto-detects the engine at runtime."
    color_echo "cyan" "   Use 'rfswift --engine docker' or 'rfswift --engine podman' to override."
    # On macOS, offer Lima for USB passthrough even with Docker/Podman present
    if [ "$(uname -s)" = "Darwin" ]; then
      offer_lima_for_usb_get_rfswift
    fi
    return 0
  fi

  # ── Only Docker installed ──────────────────────────────────────────────
  if [ "$HAS_DOCKER" = true ]; then
    color_echo "green" "✅ Docker is already installed."
    if want_second_engine podman "Would you also like to install Podman (rootless containers)?"; then
      install_podman || engine_setup_incomplete "Podman"
    fi
    # On macOS, offer Lima for USB passthrough
    if [ "$(uname -s)" = "Darwin" ]; then
      offer_lima_for_usb_get_rfswift
    fi
    return 0
  fi

  # ── Only Podman installed ──────────────────────────────────────────────
  if [ "$HAS_PODMAN" = true ]; then
    color_echo "green" "✅ Podman is already installed."
    if want_second_engine docker "Would you also like to install Docker?"; then
      install_docker || engine_setup_incomplete "Docker"
    fi
    # On macOS, offer Lima for USB passthrough
    if [ "$(uname -s)" = "Darwin" ]; then
      offer_lima_for_usb_get_rfswift
    fi
    return 0
  fi

  # ── Neither installed ──────────────────────────────────────────────────
  color_echo "yellow" "⚠️  No container engine found."
  color_echo "blue" "ℹ️  Docker or Podman is required only for containers; the Nix engine needs neither."
  echo ""
  color_echo "cyan" "📝 Which container engine would you like to install?"
  echo ""
  color_echo "cyan" "   🐳 Docker  - Industry standard, requires daemon (root)"
  color_echo "cyan" "              Best compatibility, large ecosystem"
  echo ""
  color_echo "cyan" "   🦭 Podman  - Daemonless, rootless by default"
  color_echo "cyan" "              Drop-in Docker replacement, no root needed"
  echo ""

  # macOS: also offer Lima
  if [ "$(uname -s)" = "Darwin" ]; then
    color_echo "cyan" "   🦙 Lima    - Lightweight VM with Docker inside (QEMU)"
    color_echo "cyan" "              Enables USB device passthrough for SDR hardware"
    echo ""
  fi

  # Check if this is a Steam Deck - special case
  if [ "$(uname -s)" = "Linux" ] && is_steam_deck; then
    color_echo "magenta" "🎮 Steam Deck detected! Docker with Steam Deck optimizations is recommended."
    if prompt_yes_no "Install Docker with Steam Deck optimizations?" "y"; then
      install_docker_steamdeck
      return $?
    fi
  fi

  local choices="Docker Podman Both"
  if [ "$(uname -s)" = "Darwin" ]; then
    choices="Docker Podman Both Lima Skip"
  else
    choices="Docker Podman Both Skip"
  fi

  if [ -n "$ENGINE_PREF" ]; then
    case "$ENGINE_PREF" in
      docker) CHOICE=1 ;;
      podman) CHOICE=2 ;;
      both)   CHOICE=3 ;;
      lima)   CHOICE=4 ;;
      *) if [ "$(uname -s)" = "Darwin" ]; then CHOICE=5; else CHOICE=4; fi ;;
    esac
    color_echo "cyan" "   RFSWIFT_ENGINE=${ENGINE_PREF}"
  else
    CHOICE=$(prompt_choice "Select a container engine to install:" $choices)
  fi

  # An engine that fails to install (or to start: no systemd in a container or
  # on WSL, an unsupported distribution) must not take the RF Swift install
  # down with it, which a plain call would do through `set -e`.
  case "$CHOICE" in
    1)
      install_docker || engine_setup_incomplete "Docker"
      ;;
    2)
      install_podman || engine_setup_incomplete "Podman"
      ;;
    3)
      install_docker || engine_setup_incomplete "Docker"
      install_podman || engine_setup_incomplete "Podman"
      ;;
    4)
      if [ "$(uname -s)" = "Darwin" ]; then
        # Lima option on macOS
        color_echo "blue" "🦙 Installing Lima..."
        install_lima
      else
        color_echo "yellow" "⚠️  Container engine installation skipped."
        color_echo "cyan" "   You can select the Nix engine in the next step, or install Docker/Podman later."
        return 0
      fi
      ;;
    5)
      color_echo "yellow" "⚠️  Container engine installation skipped."
      color_echo "cyan" "   You can select the Nix engine in the next step, or install Docker/Podman later."
      return 0
      ;;
  esac

  # ── Both already installed ──────────────────────────────────────────────
  if [ "$HAS_DOCKER" = true ] && [ "$HAS_PODMAN" = true ]; then
    color_echo "green" "✅ Both Docker and Podman are installed."
    if [ "$DOCKER_DAEMON_DOWN" = true ]; then
      color_echo "yellow" "⚠️  Docker daemon is not running. Start it with: sudo systemctl start docker"
    fi
    color_echo "cyan" "ℹ️  RF-Swift auto-detects the engine at runtime."
    return 0
  fi

  # ── Only Docker installed ──────────────────────────────────────────────
  if [ "$HAS_DOCKER" = true ]; then
    color_echo "green" "✅ Docker is already installed."
    if [ "$DOCKER_DAEMON_DOWN" = true ]; then
      color_echo "yellow" "⚠️  Docker daemon is not running. Start it with: sudo systemctl start docker"
    fi
    color_echo "cyan" "ℹ️  RF-Swift auto-detects the engine at runtime."
    return 0
  fi
}

# ═══════════════════════════════════════════════════════════════════════════════
# Podman Installation
# ═══════════════════════════════════════════════════════════════════════════════

# Install QEMU + official Lima. USB passthrough works on stock Lima because the
# rfswift VM template sets video.display, which makes Lima add a qemu-xhci
# controller - the old PentHertz fork (and its `usb: true` field) is no longer
# needed.
# Detect and offer to remove the old PentHertz Lima fork (installed under
# /usr/local by earlier RF-Swift releases). It shadows the official Homebrew Lima
# in PATH, so leaving it in place blocks the migration to official Lima.
maybe_remove_lima_fork() {
  [ "$(uname -s)" = "Darwin" ] || return 0
  [ -x /usr/local/bin/limactl ] || return 0

  # On Intel Macs Homebrew's prefix is /usr/local, so a brew-managed lima also
  # lives at /usr/local/bin/limactl - don't treat that as the fork.
  if command_exists brew && [ "$(brew --prefix 2>/dev/null)" = "/usr/local" ] \
      && brew list lima >/dev/null 2>&1; then
    return 0
  fi

  color_echo "yellow" "⚠️  A non-Homebrew Lima (the old PentHertz fork) is installed at /usr/local/bin/limactl."
  color_echo "yellow" "   RF-Swift now uses official Lima - the fork is no longer needed and can shadow it."
  if prompt_yes_no "Remove the old Lima fork from /usr/local?" "y"; then
    sudo rm -f /usr/local/bin/limactl /usr/local/bin/lima
    sudo rm -rf /usr/local/share/lima /usr/local/share/doc/lima
    hash -r 2>/dev/null || true
    color_echo "green" "✅ Removed the old Lima fork; official Lima will be installed via Homebrew."
  else
    color_echo "yellow" "   Keeping the fork - note it may prevent the official Lima from being used."
  fi
}

install_lima() {
  if [ "$(uname -s)" != "Darwin" ]; then
    color_echo "yellow" "Lima is only needed on macOS for USB passthrough."
    return 0
  fi

  if ! command_exists brew; then
    color_echo "red" "Homebrew is required to install QEMU and Lima."
    color_echo "yellow" "Install Homebrew: https://brew.sh/"
    return 1
  fi

  # Remove the old PentHertz Lima fork first so the official one can take over.
  maybe_remove_lima_fork

  # QEMU backend is required - USB passthrough does not work with the vz backend.
  if ! command_exists qemu-img; then
    color_echo "blue" "Installing QEMU via Homebrew..."
    brew install qemu
  fi

  if ! command_exists limactl; then
    color_echo "blue" "Installing Lima via Homebrew..."
    brew install lima
  fi

  if ! command_exists limactl; then
    color_echo "red" "Lima installation failed - limactl not found in PATH."
    color_echo "yellow" "Ensure Homebrew's bin directory is in your PATH."
    return 1
  fi

  color_echo "green" "✅ Official Lima and QEMU installed."
  limactl --version
  color_echo "cyan" "   Use 'rfswift --engine lima' when you need USB devices."
}

install_podman() {
  color_echo "blue" "🦭 Installing Podman..."

  case "$(uname -s)" in
    Darwin*)
      install_podman_macos
      ;;
    Linux*)
      install_podman_linux
      ;;
    *)
      color_echo "red" "🚨 Unsupported OS: $(uname -s)"
      return 1
      ;;
  esac
}

install_podman_macos() {
  if command_exists brew; then
    color_echo "blue" "🍏 Installing Podman via Homebrew..."
    brew install podman

    color_echo "blue" "🚀 Initialising Podman machine..."
    podman machine init 2>/dev/null || true
    podman machine start 2>/dev/null || true

    if podman info >/dev/null 2>&1; then
      color_echo "green" "🎉 Podman is up and running on macOS!"
    else
      color_echo "yellow" "⚠️  Podman installed. Run 'podman machine start' to start the VM."
    fi
  else
    color_echo "red" "🚨 Homebrew is not installed! Please install Homebrew first:"
    color_echo "yellow" '/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"'
    return 1
  fi
}

install_podman_linux() {
  if ! have_sudo_access; then
    color_echo "red" "🚨 Podman installation requires sudo privileges."
    return 1
  fi

  local distro=$(detect_distro)

  case "$distro" in
    "arch")
      color_echo "cyan" "🏛️ Installing Podman using pacman..."
      sudo pacman -Sy --noconfirm
      sudo pacman -S --noconfirm --needed podman podman-compose slirp4netns fuse-overlayfs crun
      ;;
    "fedora")
      color_echo "blue" "📦 Installing Podman using dnf..."
      sudo dnf install -y podman podman-compose slirp4netns fuse-overlayfs
      ;;
    "rhel"|"centos")
      color_echo "blue" "📦 Installing Podman..."
      if command_exists dnf; then
        sudo dnf install -y podman podman-compose slirp4netns fuse-overlayfs
      else
        sudo yum install -y podman slirp4netns fuse-overlayfs
      fi
      ;;
    "debian"|"ubuntu")
      color_echo "blue" "📦 Installing Podman using apt..."
      sudo apt update
      sudo apt install -y podman podman-compose slirp4netns fuse-overlayfs uidmap
      ;;
    "opensuse")
      color_echo "blue" "📦 Installing Podman using zypper..."
      sudo zypper install -y podman podman-compose slirp4netns fuse-overlayfs
      ;;
    "alpine")
      color_echo "blue" "📦 Installing Podman using apk..."
      sudo apk add podman podman-compose fuse-overlayfs slirp4netns
      ;;
    *)
      color_echo "red" "❌ Unsupported distribution: $distro"
      color_echo "yellow" "Please install Podman manually: https://podman.io/docs/installation"
      return 1
      ;;
  esac

  # ── Configure rootless Podman ──────────────────────────────────────────
  configure_podman_rootless

  color_echo "green" "🎉 Podman installed successfully!"
  color_echo "cyan" "💡 Tip: Podman is a drop-in replacement for Docker."
  color_echo "cyan" "   RF-Swift will auto-detect Podman at runtime."
  return 0
}

# Configure rootless Podman (subuid/subgid, lingering, etc.)
configure_podman_rootless() {
  local current_user=$(get_real_user)

  color_echo "blue" "🔧 Configuring rootless Podman for '$current_user'..."

  # ── Ensure subuid/subgid ranges ──
  if [ -f /etc/subuid ]; then
    if ! grep -q "^${current_user}:" /etc/subuid 2>/dev/null; then
      color_echo "blue" "   Adding subordinate UID range..."
      sudo usermod --add-subuids 100000-165535 "$current_user" 2>/dev/null || true
    fi
  fi

  if [ -f /etc/subgid ]; then
    if ! grep -q "^${current_user}:" /etc/subgid 2>/dev/null; then
      color_echo "blue" "   Adding subordinate GID range..."
      sudo usermod --add-subgids 100000-165535 "$current_user" 2>/dev/null || true
    fi
  fi

  # ── Enable lingering so rootless containers survive logout ──
  if command_exists loginctl; then
    color_echo "blue" "   Enabling login lingering..."
    sudo loginctl enable-linger "$current_user" 2>/dev/null || true
  fi

  # ── Enable Podman socket for compatibility with Docker-expecting tools ──
  if command_exists systemctl; then
    color_echo "blue" "   Enabling Podman socket..."
    systemctl --user enable podman.socket 2>/dev/null || true
    systemctl --user start podman.socket 2>/dev/null || true
  fi

  color_echo "green" "   ✅ Rootless Podman configured"
}

# ═══════════════════════════════════════════════════════════════════════════════
# Docker Installation (kept from original, with minor refactoring)
# ═══════════════════════════════════════════════════════════════════════════════

# Enhanced Steam Deck Docker installation with Arch Linux optimization
install_docker_steamdeck() {
  color_echo "yellow" "🎮 Installing Docker on Steam Deck using Arch Linux methods..."
  
  if ! have_sudo_access; then
    color_echo "red" "🚨 Steam Deck Docker installation requires sudo privileges."
    return 1
  fi
  
  # Installation steps for Docker on Steam Deck (Arch Linux based)
  color_echo "blue" "🎮 Disabling read-only mode on Steam Deck"
  sudo steamos-readonly disable

  color_echo "blue" "🔑 Initializing pacman keyring"
  sudo pacman-key --init
  sudo pacman-key --populate archlinux
  sudo pacman-key --populate holo

  color_echo "blue" "🐳 Installing Docker using pacman"
  sudo pacman -Syu --noconfirm docker docker-compose

  # Install Docker Compose for Steam Deck
  install_docker_compose_steamdeck

  # Add user to docker group
  add_user_to_docker_group

  # Start Docker service
  if command_exists systemctl; then
    color_echo "blue" "🚀 Starting Docker service..."
    sudo systemctl start docker
    sudo systemctl enable docker
  fi

  color_echo "green" "🎉 Docker installed successfully on Steam Deck using Arch Linux methods!"
  return 0
}

# Install Docker Compose for Steam Deck
install_docker_compose_steamdeck() {
  color_echo "blue" "🧩 Installing Docker Compose v2 plugin for Steam Deck"
  
  DOCKER_CONFIG=${DOCKER_CONFIG:-$HOME/.docker}
  mkdir -p "$DOCKER_CONFIG/cli-plugins"
  
  # Download Docker Compose for x86_64 (Steam Deck architecture)
  color_echo "blue" "📥 Downloading Docker Compose..."
  curl -SL https://github.com/docker/compose/releases/download/v2.36.0/docker-compose-linux-x86_64 -o "$DOCKER_CONFIG/cli-plugins/docker-compose"
  chmod +x "$DOCKER_CONFIG/cli-plugins/docker-compose"

  color_echo "green" "✅ Docker Compose v2 installed successfully for Steam Deck"
}

# Add current user to the docker group
add_user_to_docker_group() {
  if command_exists sudo && command_exists groups; then
    current_user=$(get_real_user)
    if ! groups "$current_user" 2>/dev/null | grep -qw docker; then
      color_echo "blue" "🔧 Adding '$current_user' to Docker group..."
      sudo usermod -aG docker "$current_user"
    fi
    grant_docker_session_access "$current_user"
  fi
}

# Group membership only counts from the next login. An ACL on the daemon's
# socket makes Docker usable in THIS session right away; it lasts until the
# daemon recreates the socket, by which time the group is in effect. Same as
# `rfswift host docker-access`. Silent when the socket is already writable.
grant_docker_session_access() {
  user="$1"
  if [ ! -S /var/run/docker.sock ]; then
    color_echo "yellow" "⚡ Docker socket not there yet; the docker group takes effect at your next login (or 'newgrp docker')."
    return 0
  fi
  [ -w /var/run/docker.sock ] && return 0
  if command_exists setfacl && sudo setfacl -m "u:${user}:rw" /var/run/docker.sock 2>/dev/null && [ -w /var/run/docker.sock ]; then
    color_echo "green" "✅ Docker is usable right away in this session; the docker group makes it permanent from your next login."
  else
    color_echo "yellow" "⚡ Log out and back in (or run 'newgrp docker') for the docker group to take effect."
  fi
}

# Locate the rfswift just installed: the packaged one on PATH, else the
# tarball copy in INSTALL_DIR, else whatever PATH has.
rfswift_binary_path() {
  if [ "$NATIVE_INSTALLED" = true ] && command_exists rfswift; then
    command -v rfswift
    return 0
  fi
  if [ -n "${INSTALL_DIR:-}" ] && [ -x "${INSTALL_DIR}/rfswift" ]; then
    echo "${INSTALL_DIR}/rfswift"
    return 0
  fi
  if command_exists rfswift; then
    command -v rfswift
  fi
}

# Offer RF Swift's udev rules (Linux, CLI installed). Rootless Podman and Nix
# environments run tools as the user and cannot open the root-owned USB nodes
# without them; Docker does not need them. The rules are embedded in the
# rfswift binary (and shipped by the packages under /usr/share/rfswift/udev/),
# so the freshly installed CLI does the work in one sudo call. The CLI reads
# its confirmation/sudo prompt from the terminal, so hand it /dev/tty when the
# script itself runs from a pipe (curl | sh). RFSWIFT_UDEV=1|0 answers
# non-interactively.
offer_udev_rules() {
  [ "$(uname -s)" = "Linux" ] || return 0
  case "$INSTALL_COMPONENTS" in cli|both) ;; *) return 0 ;; esac
  rfswift_bin=$(rfswift_binary_path)
  if [ -z "$rfswift_bin" ]; then
    color_echo "yellow" "⚠️  rfswift not found on PATH yet; run 'rfswift host udev' later for SDR/RF hardware access without root."
    return 0
  fi
  echo ""
  color_echo "blue" "🔌 udev rules for RF hardware (HackRF, RTL-SDR, bladeRF, USRP, Proxmark, ...)"
  color_echo "cyan" "   Rootless Podman and Nix environments run tools as your user and need them; Docker does not."
  color_echo "cyan" "   They grant group plugdev + the logged-in user (seat ACL), never world-writable device nodes."
  install_rules=false
  case "$UDEV_RULES" in
    0|no|false)
      color_echo "yellow" "   Skipped (RFSWIFT_UDEV=0). Later: rfswift host udev"
      return 0
      ;;
    1|yes|true)
      install_rules=true
      ;;
    *)
      if prompt_yes_no "Install RF Swift's udev rules now? (one sudo prompt)" "y"; then
        install_rules=true
      fi
      ;;
  esac
  if [ "$install_rules" != true ]; then
    color_echo "yellow" "   Skipped. Later: rfswift host udev"
    return 0
  fi
  # Checked in an `if` so a failure (no udev daemon, WSL, a container) reaches
  # the fallback message instead of ending the script through `set -e`.
  udev_ok=false
  if [ ! -t 0 ] && [ -c /dev/tty ] && ( : < /dev/tty ) 2>/dev/null; then
    if "$rfswift_bin" host udev --yes < /dev/tty; then udev_ok=true; fi
  else
    if "$rfswift_bin" host udev --yes; then udev_ok=true; fi
  fi
  if [ "$udev_ok" = true ]; then
    color_echo "green" "✅ udev rules installed (re-plug a device that was already connected)."
  else
    color_echo "yellow" "⚠️  Could not install the udev rules. Later: rfswift host udev"
  fi
}

# Enhanced Docker installation with Arch Linux support
install_docker() {
  color_echo "blue" "🐳 Installing Docker..."

  case "$(uname -s)" in
    Darwin*)
      if command_exists brew; then
        color_echo "blue" "🍏 Installing Docker via Homebrew..."
        brew install --cask docker
        
        color_echo "blue" "🚀 Launching Docker Desktop now... Hold tight!"
        open -a Docker
        
        color_echo "yellow" "⏳ Give it a moment, Docker is warming up!"
        i=1
        while [ $i -le 30 ]; do
          if command_exists docker && docker info >/dev/null 2>&1; then
            color_echo "green" "✅ Docker is up and running!"
            return 0
          fi
          sleep 2
          i=$((i + 1))
        done
        
        color_echo "yellow" "Docker is installed but still starting. Please open Docker manually if needed."
      else
        color_echo "red" "🚨 Homebrew is not installed! Please install Homebrew first:"
        color_echo "yellow" '/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"'
        color_echo "yellow" "Then, run the script again!"
        return 1
      fi
      ;;
      
    Linux*)
      color_echo "blue" "🐧 Installing Docker on your Linux machine..."
      
      # Enhanced Arch Linux Docker installation
      if is_arch_linux; then
        color_echo "cyan" "🏛️ Arch Linux detected - using pacman for Docker installation"
        
        if ! have_sudo_access; then
          color_echo "red" "🚨 Unable to obtain sudo privileges. Docker installation requires sudo."
          return 1
        fi
        
        color_echo "blue" "📦 Installing Docker using pacman..."
        sudo pacman -Sy --noconfirm
        sudo pacman -S --noconfirm --needed docker docker-compose || return 1
        
        # Enable and start Docker service
        if command_exists systemctl; then
          color_echo "blue" "🚀 Enabling and starting Docker service..."
          sudo systemctl enable --now docker || docker_service_not_started
        fi
        
        add_user_to_docker_group
        
        color_echo "green" "🎉 Docker installed successfully using pacman!"
        return 0
      else
        # Standard Docker installation for other distributions
        color_echo "yellow" "⚠️ This will require sudo privileges to install Docker."
        
        if ! have_sudo_access; then
          color_echo "red" "🚨 Unable to obtain sudo privileges. Docker installation requires sudo."
          return 1
        fi
        
        color_echo "blue" "Using sudo to install Docker..."
        
        # get.docker.com refuses Kali ("Unsupported distribution 'kali'");
        # Kali packages Docker itself, so install it from the Kali repository.
        if grep -qs '^ID=kali' /etc/os-release; then
          color_echo "cyan" "🐉 Kali Linux detected - installing docker.io from the Kali repository..."
          sudo apt-get update && sudo apt-get install -y docker.io || return 1
        elif command_exists curl; then
          curl -fsSL "https://get.docker.com/" | sudo sh || return 1
        elif command_exists wget; then
          wget -qO- "https://get.docker.com/" | sudo sh || return 1
        else
          color_echo "red" "🚨 Missing curl/wget. Please install one of them."
          return 1
        fi

        if command_exists systemctl; then
          color_echo "blue" "🚀 Starting Docker service..."
          sudo systemctl enable --now docker || docker_service_not_started
        fi
        add_user_to_docker_group

        color_echo "green" "🎉 Docker is now installed and running!"
      fi
      ;;
      
    *)
      color_echo "red" "🚨 Unsupported OS detected: $(uname -s). Docker can't be installed automatically here."
      return 1
      ;;
  esac
}

# ═══════════════════════════════════════════════════════════════════════════════
# Release download, system detection, and binary installation
# ═══════════════════════════════════════════════════════════════════════════════

# Function to get the latest release information
choose_release_and_components() {
  case "$RELEASE_CHANNEL" in
    stable|dev) ;;
    *) color_echo "red" "Invalid RFSWIFT_CHANNEL '$RELEASE_CHANNEL' (use stable or dev)"; exit 1 ;;
  esac
  case "$INSTALL_COMPONENTS" in
    cli|workbench|both) ;;
    *) color_echo "red" "Invalid RFSWIFT_INSTALL '$INSTALL_COMPONENTS' (use cli, workbench, or both)"; exit 1 ;;
  esac
  case "$PKG_FORMAT" in
    ""|native|tarball) ;;
    *) color_echo "red" "Invalid RFSWIFT_PKG_FORMAT '$PKG_FORMAT' (use native or tarball)"; exit 1 ;;
  esac
  case "$ENGINE_PREF" in
    ""|docker|podman|both|lima|skip|none) ;;
    *) color_echo "red" "Invalid RFSWIFT_ENGINE '$ENGINE_PREF' (use docker, podman, both, lima or skip)"; exit 1 ;;
  esac
  case "$NIX_PREF" in
    ""|1|0|yes|no|true|false) ;;
    *) color_echo "red" "Invalid RFSWIFT_NIX '$NIX_PREF' (use 1 or 0)"; exit 1 ;;
  esac
  case "$INSTALL_DIR_PREF" in
    ""|/*) ;;
    *) color_echo "red" "Invalid RFSWIFT_INSTALL_DIR '$INSTALL_DIR_PREF' (use an absolute path)"; exit 1 ;;
  esac

  # Environment variables make curl|sh and automated installs deterministic.
  # Interactive installs get a concise choice; no-TTY installs retain the
  # backwards-compatible stable CLI default.
  if [ -z "${RFSWIFT_CHANNEL+x}" ]; then
    channel_choice=$(prompt_choice "Select RF Swift release channel" "Stable" "Development-prerelease")
    [ "$channel_choice" = "2" ] && RELEASE_CHANNEL="dev" || RELEASE_CHANNEL="stable"
  fi
  if [ -z "${RFSWIFT_INSTALL+x}" ]; then
    component_choice=$(prompt_choice "What would you like to install?" "CLI" "Workbench-GUI" "CLI-and-Workbench")
    case "$component_choice" in
      2) INSTALL_COMPONENTS="workbench" ;;
      3) INSTALL_COMPONENTS="both" ;;
      *) INSTALL_COMPONENTS="cli" ;;
    esac
  fi
  color_echo "green" "📦 Channel: ${RELEASE_CHANNEL}; components: ${INSTALL_COMPONENTS}"
}

# A release version may only contain [0-9A-Za-z.-]: enough for semver plus
# prerelease tags, and safe to embed in URLs, filenames and package names.
validate_version_string() {
  case "$1" in
    ""|*[!0-9A-Za-z.-]*) return 1 ;;
    *) return 0 ;;
  esac
}

get_latest_release() {
  color_echo "blue" "🔍 Detecting the latest RF-Swift release..."

  if [ "$RELEASE_CHANNEL" = "dev" ]; then
    VERSION="$DEV_VERSION"
    # The prerelease tag moves (v4.0.0-dev, v4.0.1-dev, ...) and a stale
    # DEV_VERSION sent every dev-channel install to a 404. Ask GitHub for the
    # newest prerelease; DEV_VERSION only covers an unreachable API.
    latest_prerelease=$(http_get "https://api.github.com/repos/${GITHUB_REPO}/releases?per_page=20" \
      | tr -d '\n' | sed 's/},{/}\
{/g' | grep '"prerelease": *true' | grep -o '"tag_name": *"[^"]*"' | head -1 \
      | sed 's/.*: *"v\{0,1\}\([^"]*\)".*/\1/' || true)
    if [ -n "$latest_prerelease" ] && validate_version_string "$latest_prerelease"; then
      VERSION="$latest_prerelease"
    else
      color_echo "yellow" "⚠️ Could not query GitHub for the newest prerelease; assuming v${DEV_VERSION}."
    fi
    FOUND_VERSION=true
    RELEASE_URL="https://github.com/${GITHUB_REPO}/releases/tag/v${VERSION}"
    DOWNLOAD_BASE_URL="https://github.com/${GITHUB_REPO}/releases/download/v${VERSION}"
    color_echo "yellow" "🧪 Using development prerelease: v${VERSION}"
    return 0
  fi

  # Default version as fallback
  DEFAULT_VERSION="3.0.1"
  VERSION="${DEFAULT_VERSION}"  # Initialize with default
  
  # First try: Use GitHub API with a proper User-Agent to avoid rate limiting issues
  if command_exists curl; then
    # First method: direct API call with proper headers to avoid throttling
    LATEST_INFO=$(curl -s -H "User-Agent: RF-Swift-Installer" "https://api.github.com/repos/${GITHUB_REPO}/releases/latest")
    
    # Check if we got a proper response
    if echo "${LATEST_INFO}" | grep -q "tag_name"; then
      # Extract version, handle both with and without "v" prefix
      DETECTED_VERSION=$(echo "${LATEST_INFO}" | grep -o '"tag_name": *"[^"]*"' | sed 's/.*: *"v\{0,1\}\([^"]*\)".*/\1/')
      
      if [ -n "${DETECTED_VERSION}" ]; then
        VERSION="${DETECTED_VERSION}"
        FOUND_VERSION=true
        color_echo "green" "✅ Successfully retrieved latest version using GitHub API"
      fi
    else
      color_echo "yellow" "GitHub API query didn't return expected results. Trying alternative method..."
    fi
  fi

  # Second try: Parse the releases page directly if API method failed
  if [ "${FOUND_VERSION}" = false ] && command_exists curl; then
    color_echo "blue" "Trying direct HTML parsing method..."
    
    RELEASES_PAGE=$(curl -s -L -H "User-Agent: RF-Swift-Installer" "https://github.com/${GITHUB_REPO}/releases/latest")
    
    # Look for version in the page title and URL
    DETECTED_VERSION=$(echo "${RELEASES_PAGE}" | grep -o "${GITHUB_REPO}/releases/tag/v[0-9][0-9.a-z-]*" | head -1 | sed 's/.*tag\/v//')
    
    if [ -n "${DETECTED_VERSION}" ]; then
      VERSION="${DETECTED_VERSION}"
      color_echo "green" "✅ Retrieved version ${VERSION} by parsing GitHub releases page"
    else
      # One last attempt - look for version in the title
      DETECTED_VERSION=$(echo "${RELEASES_PAGE}" | grep -o '<title>Release v[0-9][0-9.a-z-]*' | head -1 | sed 's/.*Release v//')
      
      if [ -n "${DETECTED_VERSION}" ]; then
        VERSION="${DETECTED_VERSION}"
        FOUND_VERSION=true
        color_echo "green" "✅ Retrieved version ${VERSION} from page title"
      else
        color_echo "yellow" "⚠️ Using default version ${DEFAULT_VERSION} as a fallback"
      fi
    fi
  fi

  if [ "${FOUND_VERSION}" = false ]; then
    VERSION="${DEFAULT_VERSION}"  # Initialize with default
  fi

  # The version came off the network (GitHub API or HTML scraping) and flows
  # into URLs and filesystem paths - constrain it to a safe charset before
  # using it anywhere.
  if ! validate_version_string "$VERSION"; then
    color_echo "red" "🚨 Refusing suspicious version string: '${VERSION}'"
    exit 1
  fi

  # Set URLs based on the version
  RELEASE_URL="https://github.com/${GITHUB_REPO}/releases/tag/v${VERSION}"
  DOWNLOAD_BASE_URL="https://github.com/${GITHUB_REPO}/releases/download/v${VERSION}"

  color_echo "green" "📦 Using version: ${VERSION}"
}

# Function to detect OS and architecture
detect_system() {
  case "$(uname -s)" in
    Linux*) OS="Linux" ;;
    Darwin*) OS="Darwin" ;;
    *) color_echo "red" "Unsupported OS: $(uname -s)"; exit 1 ;;
  esac

  case "$(uname -m)" in
    x86_64) ARCH="x86_64" ;;
    arm64|aarch64) ARCH="arm64" ;;
    riscv64) ARCH="riscv64" ;;
    *) color_echo "red" "Unsupported architecture: $(uname -m)"; exit 1 ;;
  esac

  # Set the download filename
  FILENAME="rfswift_${OS}_${ARCH}.tar.gz"
  DOWNLOAD_URL="${DOWNLOAD_BASE_URL}/${FILENAME}"

  case "${OS}_${ARCH}" in
    Linux_x86_64) WORKBENCH_FILENAME="rfswift-workbench_Linux_x86_64.tar.gz" ;;
    Linux_arm64) WORKBENCH_FILENAME="rfswift-workbench_Linux_arm64.tar.gz" ;;
    Darwin_x86_64|Darwin_arm64) WORKBENCH_FILENAME="rfswift-workbench_Darwin_universal.zip" ;;
    *) WORKBENCH_FILENAME="" ;;
  esac

  if { [ "$INSTALL_COMPONENTS" = "workbench" ] || [ "$INSTALL_COMPONENTS" = "both" ]; } && [ -z "$WORKBENCH_FILENAME" ]; then
    color_echo "red" "RF Swift Workbench is not currently published for ${OS} ${ARCH}."
    color_echo "yellow" "The CLI remains available; Workbench release targets are Linux x86_64/arm64 and universal macOS."
    exit 1
  fi

  color_echo "blue" "🏠 Detected system: ${OS} ${ARCH}"
}

# Ask which Linux Workbench artifact to use. Only relevant for the tarball
# flow - the native-package flow ships the Workbench as a distro package - so
# it is asked right before downloading, not during system detection.
choose_workbench_format() {
  [ "$OS" = "Linux" ] || return 0
  { [ "$INSTALL_COMPONENTS" = "workbench" ] || [ "$INSTALL_COMPONENTS" = "both" ]; } || return 0
  case "$WORKBENCH_FORMAT" in native|appimage) ;; *) color_echo "red" "RFSWIFT_WORKBENCH_FORMAT must be native or appimage"; exit 1 ;; esac
  if [ -z "${RFSWIFT_WORKBENCH_FORMAT+x}" ]; then
    format_choice=$(prompt_choice "Choose the Linux Workbench package" "AppImage-portable" "Native-smaller")
    [ "$format_choice" = "2" ] && WORKBENCH_FORMAT="native" || WORKBENCH_FORMAT="appimage"
  fi
  if [ "$WORKBENCH_FORMAT" = "appimage" ]; then
    case "$ARCH" in
      x86_64) WORKBENCH_FILENAME="rfswift-workbench_Linux_x86_64.AppImage" ;;
      arm64)  WORKBENCH_FILENAME="rfswift-workbench_Linux_arm64.AppImage" ;;
    esac
  fi
  color_echo "blue" "🖥️  Will download: ${WORKBENCH_FILENAME}"
}

# ═══════════════════════════════════════════════════════════════════════════════
# Native packages: deb/rpm/pacman on Linux, Homebrew cask on macOS
# ═══════════════════════════════════════════════════════════════════════════════

# Quick existence probe so a release without native packages (or a wrong name)
# falls back to the tarball flow instead of aborting mid-install.
release_asset_exists() {
  if command_exists curl; then
    curl -fsIL -H "User-Agent: RF-Swift-Installer" "${DOWNLOAD_BASE_URL}/$1" >/dev/null 2>&1
  else
    wget -q --spider "${DOWNLOAD_BASE_URL}/$1" >/dev/null 2>&1
  fi
}

# Install one downloaded package file with the distro package manager (which
# also resolves the Workbench's GTK/WebKit dependencies).
install_pkg_file() {
  case "$NATIVE_PM" in
    apt)    sudo apt-get install -y "$1" ;;
    dnf)    sudo dnf install -y "$1" ;;
    yum)    sudo yum install -y "$1" ;;
    zypper) sudo zypper --non-interactive install --allow-unsigned-rpm "$1" ;;
    pacman) sudo pacman -U --noconfirm "$1" ;;
    *) return 1 ;;
  esac
}

# Download one native package with checksum verification. Returns non-zero so
# the caller can fall back to tarballs; a checksum MISMATCH still hard-aborts
# inside download_and_verify (fail closed, never fall back on tampering).
native_fetch_one() {
  download_and_verify "$1" "$2" || return 1
  [ -f "${TMP_DIR}/$1" ] || return 1
}

# Same opt-in, fail-closed attestation gate as the tarball flow: skipping is a
# user choice, a failed verification is fatal. Packages install as root, so
# they get the identical provenance treatment the tarballs get.
native_verify_attestations() {
  if ! command_exists gh; then
    color_echo "yellow" "ℹ️  Install the GitHub CLI (gh) to cryptographically verify build attestations."
    return 0
  fi
  prompt_yes_no "Verify GitHub build attestations for downloaded packages (recommended)?" "y" || return 0
  color_echo "blue" "🔏 Verifying build provenance with 'gh attestation verify'..."
  for pkg in "${TMP_DIR}"/rfswift*; do
    [ -f "$pkg" ] || continue
    gh attestation verify "$pkg" --repo "$GITHUB_REPO" || {
      color_echo "red" "🚨 Attestation verification failed for $(basename "$pkg")."
      rm -rf "$TMP_DIR"
      exit 1
    }
  done
  color_echo "green" "✅ Build provenance verified for ${GITHUB_REPO}."
}

# Prefer native packages (man pages, shell completions, dependency handling,
# clean uninstall) over the tarball when a supported package manager and sudo
# are available. Any failure returns non-zero and the classic tarball flow
# takes over. Stable channel only: prerelease package filenames mangle the
# version (4.0.0-dev -> 4.0.0~dev) and are not worth predicting here.
try_native_package_install() {
  [ "$RELEASE_CHANNEL" = "stable" ] || return 1
  case "$OS" in
    Darwin) try_native_install_macos ;;
    Linux)  try_native_install_linux ;;
    *) return 1 ;;
  esac
}

try_native_install_macos() {
  command_exists brew || return 1
  if [ -z "$PKG_FORMAT" ]; then
    color_echo "cyan" "🍺 Homebrew can install RF Swift from the signed, notarized release (CLI + Workbench, auto-updates with brew upgrade)."
    choice=$(prompt_choice "How would you like to install RF Swift?" "Homebrew-cask-recommended" "Direct-download")
    [ "$choice" = "1" ] && PKG_FORMAT="native" || PKG_FORMAT="tarball"
  fi
  [ "$PKG_FORMAT" = "native" ] || return 1
  color_echo "blue" "🍺 Installing the rfswift cask..."
  if brew install --cask penthertz/rfswift/rfswift; then
    NATIVE_INSTALLED=true
    INSTALL_DIR="$(brew --prefix)/bin"
    color_echo "green" "✅ Installed via Homebrew. Upgrade later with: brew upgrade --cask rfswift"
    return 0
  fi
  color_echo "yellow" "⚠️ Homebrew install failed (tap unreachable?). Falling back to direct download."
  return 1
}

try_native_install_linux() {
  [ "$PKG_FORMAT" = "tarball" ] && return 1
  NATIVE_PM=$(get_package_manager)
  case "$NATIVE_PM" in apt|dnf|yum|zypper|pacman) ;; *) return 1 ;; esac

  # Map this system onto each ecosystem's package naming.
  case "$NATIVE_PM" in
    apt)
      case "$ARCH" in x86_64) pkg_arch="amd64" ;; arm64) pkg_arch="arm64" ;; riscv64) pkg_arch="riscv64" ;; *) return 1 ;; esac
      cli_pkg="rfswift_${VERSION}_${pkg_arch}.deb"
      wb_pkg="rfswift-workbench_${VERSION}_${pkg_arch}.deb"
      ;;
    dnf|yum|zypper)
      case "$ARCH" in x86_64) pkg_arch="x86_64" ;; arm64) pkg_arch="aarch64" ;; riscv64) pkg_arch="riscv64" ;; *) return 1 ;; esac
      cli_pkg="rfswift-${VERSION}-1.${pkg_arch}.rpm"
      wb_pkg="rfswift-workbench-${VERSION}-1.${pkg_arch}.rpm"
      ;;
    pacman)
      case "$ARCH" in x86_64) pkg_arch="x86_64" ;; arm64) pkg_arch="aarch64" ;; *) return 1 ;; esac
      cli_pkg="rfswift-${VERSION}-1-${pkg_arch}.pkg.tar.zst"
      wb_pkg="rfswift-workbench-${VERSION}-1-${pkg_arch}.pkg.tar.zst"
      ;;
  esac

  # The Workbench package declares Debian/Fedora/Arch dependency names and
  # exists for amd64/arm64 only; anywhere else its tarball/AppImage flow
  # handles dependencies (see ensure_workbench_runtime). All-or-nothing per
  # run so the fallback never half-duplicates an install.
  if [ "$INSTALL_COMPONENTS" = "workbench" ] || [ "$INSTALL_COMPONENTS" = "both" ]; then
    { [ "$NATIVE_PM" = "zypper" ] || [ "$ARCH" = "riscv64" ]; } && return 1
  fi

  if ! have_sudo_access; then
    color_echo "yellow" "⚠️ Native packages need sudo; using the tarball install instead."
    return 1
  fi

  # Packages install as root, so checksum verification is not optional on this
  # path: without a SHA-256 tool, the tarball flow (which at least warns) is
  # the only acceptable fallback.
  if ! command_exists sha256sum && ! command_exists shasum; then
    color_echo "yellow" "⚠️ No SHA-256 tool found to verify packages; using the tarball install."
    return 1
  fi

  # Probe every requested package upfront (older releases predate them) so a
  # missing one can never leave a half-native install behind.
  if [ "$INSTALL_COMPONENTS" = "cli" ] || [ "$INSTALL_COMPONENTS" = "both" ]; then
    release_asset_exists "$cli_pkg" || return 1
  fi
  if [ "$INSTALL_COMPONENTS" = "workbench" ] || [ "$INSTALL_COMPONENTS" = "both" ]; then
    release_asset_exists "$wb_pkg" || return 1
  fi

  if [ -z "$PKG_FORMAT" ]; then
    color_echo "cyan" "📦 A native package is available for your system (man pages, shell completions, clean uninstall)."
    choice=$(prompt_choice "How would you like to install RF Swift?" "Native-package-recommended" "Tarball-to-a-directory")
    [ "$choice" = "1" ] && PKG_FORMAT="native" || PKG_FORMAT="tarball"
  fi
  [ "$PKG_FORMAT" = "native" ] || return 1

  # Fetch and verify everything first, then attest, then install - so nothing
  # is installed before every download has passed verification.
  TMP_DIR=$(mktemp -d)
  if [ "$INSTALL_COMPONENTS" = "cli" ] || [ "$INSTALL_COMPONENTS" = "both" ]; then
    if ! native_fetch_one "$cli_pkg" "RF-Swift_${VERSION}_checksums.txt"; then
      rm -rf "$TMP_DIR"
      color_echo "yellow" "⚠️ Native package download failed; falling back to the tarball flow."
      return 1
    fi
  fi
  if [ "$INSTALL_COMPONENTS" = "workbench" ] || [ "$INSTALL_COMPONENTS" = "both" ]; then
    if ! native_fetch_one "$wb_pkg" "RF-Swift_${VERSION}_workbench_checksums.txt"; then
      rm -rf "$TMP_DIR"
      color_echo "yellow" "⚠️ Workbench package download failed; falling back to the tarball flow."
      return 1
    fi
  fi
  native_verify_attestations
  if [ "$INSTALL_COMPONENTS" = "cli" ] || [ "$INSTALL_COMPONENTS" = "both" ]; then
    if ! install_pkg_file "${TMP_DIR}/${cli_pkg}"; then
      rm -rf "$TMP_DIR"
      color_echo "yellow" "⚠️ Native package install failed; falling back to the tarball flow."
      return 1
    fi
  fi
  if [ "$INSTALL_COMPONENTS" = "workbench" ] || [ "$INSTALL_COMPONENTS" = "both" ]; then
    if ! install_pkg_file "${TMP_DIR}/${wb_pkg}"; then
      rm -rf "$TMP_DIR"
      color_echo "yellow" "⚠️ Workbench package install failed; falling back to the tarball flow."
      return 1
    fi
  fi
  rm -rf "$TMP_DIR"
  NATIVE_INSTALLED=true
  INSTALL_DIR="/usr/bin"
  color_echo "green" "🎉 RF Swift installed via ${NATIVE_PM} packages - man pages and completions included."
  return 0
}

# Download one release asset into TMP_DIR and verify it against the release's
# checksums file. Used by both the tarball flow and the native-package flow.
download_and_verify() {
    asset_name="$1"
    checksums_name="$2"
    asset_url="${DOWNLOAD_BASE_URL}/${asset_name}"
    asset_path="${TMP_DIR}/${asset_name}"
    checksums_url="${DOWNLOAD_BASE_URL}/${checksums_name}"
    checksums_path="${TMP_DIR}/${checksums_name}"
    color_echo "blue" "🔽 Downloading ${asset_name}..."
    if command_exists curl; then
      curl -fL --retry 3 --progress-bar -o "$asset_path" "$asset_url"
    elif command_exists wget; then
      wget -q --show-progress -O "$asset_path" "$asset_url"
    else
      color_echo "red" "🚨 Missing curl or wget."
      exit 1
    fi

    calculated=""
    command_exists shasum && calculated=$(shasum -a 256 "$asset_path" | awk '{print $1}')
    [ -z "$calculated" ] && command_exists sha256sum && calculated=$(sha256sum "$asset_path" | awk '{print $1}')
    if [ -z "$calculated" ]; then
      color_echo "yellow" "⚠️ No SHA-256 utility is available; ${asset_name} cannot be verified."
      return 0
    fi
    if [ ! -f "$checksums_path" ]; then
      if command_exists curl; then
        curl -fsSL --retry 3 -o "$checksums_path" "$checksums_url"
      else
        wget -qO "$checksums_path" "$checksums_url"
      fi
    fi
    expected=$(grep -E "[[:space:]][*]?${asset_name}\$" "$checksums_path" | awk '{print $1}' | head -1)
    if [ -z "$expected" ] || [ "$expected" != "$calculated" ]; then
      color_echo "red" "🚨 Checksum verification failed for ${asset_name}."
      rm -rf "$TMP_DIR"
      exit 1
    fi
    color_echo "green" "✅ Verified ${asset_name}: ${calculated}"
}

# Download the files and display checksum information
download_files() {
  color_echo "blue" "🌟 Preparing to download RF-Swift..."
  TMP_DIR=$(mktemp -d)

  if [ "$INSTALL_COMPONENTS" = "cli" ] || [ "$INSTALL_COMPONENTS" = "both" ]; then
    download_and_verify "$FILENAME" "RF-Swift_${VERSION}_checksums.txt"
  fi
  if [ "$INSTALL_COMPONENTS" = "workbench" ] || [ "$INSTALL_COMPONENTS" = "both" ]; then
    download_and_verify "$WORKBENCH_FILENAME" "RF-Swift_${VERSION}_workbench_checksums.txt"
  fi
  
  # GitHub release page for manual verification
  RELEASE_PAGE_URL="https://github.com/${GITHUB_REPO}/releases/tag/v${VERSION}"
  color_echo "yellow" "If needed, verify the checksum by visiting the GitHub release page: ${RELEASE_PAGE_URL}"

  # Optional: verify GitHub build provenance attestation (Sigstore-backed).
  # Proves the binary was built by the official RF-Swift release workflow and not
  # swapped afterwards - same mechanism as LUKSbox.
  if command_exists gh; then
    if prompt_yes_no "Verify GitHub build attestations for downloaded artifacts (recommended)?" "y"; then
      color_echo "blue" "🔏 Verifying build provenance with 'gh attestation verify'..."
      for downloaded in "${TMP_DIR}"/rfswift*.tar.gz "${TMP_DIR}"/rfswift*.zip "${TMP_DIR}"/rfswift*.AppImage; do
        [ -f "$downloaded" ] || continue
        gh attestation verify "$downloaded" --repo "$GITHUB_REPO" || {
          color_echo "red" "🚨 Attestation verification failed for $(basename "$downloaded")."
          rm -rf "$TMP_DIR"
          exit 1
        }
      done
      color_echo "green" "✅ Build provenance verified for ${GITHUB_REPO}."
    fi
  else
    color_echo "yellow" "ℹ️  Install the GitHub CLI (gh) to cryptographically verify build attestations:"
    color_echo "yellow" "   https://cli.github.com"
  fi
  
  # Ask to continue
  if ! prompt_yes_no "Continue with installation?" "y"; then
    color_echo "red" "🚨 Installation aborted by user."
    rm -rf "${TMP_DIR}"
    exit 1
  fi
  
  # If we got here, continue with installation
  return 0
}

# Choose installation directory
choose_install_dir() {
  # Preserve the location selected by an earlier tarball installation. This is
  # especially important when CLI and Workbench were installed separately:
  # an upgrade must replace each binary in place instead of creating a second,
  # shadowed copy in the newly selected default directory.
  CLI_INSTALL_DIR=""
  WORKBENCH_INSTALL_DIR=""
  existing_cli=$(installed_binary_dir rfswift)
  existing_workbench=$(installed_binary_dir rfswift-workbench)

  color_echo "blue" "🏠 Choose where to install RF-Swift..."
  [ -n "$existing_cli" ] && color_echo "green" "✅ Existing CLI detected in ${existing_cli}; it will be replaced there."
  [ -n "$existing_workbench" ] && color_echo "green" "✅ Existing Workbench detected in ${existing_workbench}; it will be replaced there."

  if [ -n "$existing_cli" ]; then CLI_INSTALL_DIR="$existing_cli"; fi
  if [ -n "$existing_workbench" ]; then WORKBENCH_INSTALL_DIR="$existing_workbench"; fi

  # If every requested component already has a destination, no new location
  # needs to be selected. Keep INSTALL_DIR for the alias/post-install helpers.
  if { [ "$INSTALL_COMPONENTS" = "cli" ] && [ -n "$CLI_INSTALL_DIR" ]; } ||
     { [ "$INSTALL_COMPONENTS" = "workbench" ] && [ -n "$WORKBENCH_INSTALL_DIR" ]; } ||
     { [ "$INSTALL_COMPONENTS" = "both" ] && [ -n "$CLI_INSTALL_DIR" ] && [ -n "$WORKBENCH_INSTALL_DIR" ]; }; then
    INSTALL_DIR="${CLI_INSTALL_DIR:-$WORKBENCH_INSTALL_DIR}"
    return 0
  fi

  color_echo "cyan" "You have two options:"
  color_echo "cyan" "1. System-wide installation (/usr/local/bin) - requires sudo"
  color_echo "cyan" "2. User-local installation (~/.rfswift/bin) - doesn't require sudo"
  
  if [ -n "$INSTALL_DIR_PREF" ]; then
    INSTALL_DIR="$INSTALL_DIR_PREF"
    color_echo "cyan" "   RFSWIFT_INSTALL_DIR=${INSTALL_DIR}"
  elif prompt_yes_no "Install system-wide (requires sudo)?" "n"; then
    INSTALL_DIR="/usr/local/bin"
    if ! have_sudo_access; then
      color_echo "red" "🚨 System-wide installation requires sudo. You don't seem to have sudo access."
      color_echo "yellow" "Falling back to user-local installation."
      INSTALL_DIR="$HOME/.rfswift/bin"
    fi
  else
    INSTALL_DIR="$HOME/.rfswift/bin"
  fi

  [ -n "$CLI_INSTALL_DIR" ] || CLI_INSTALL_DIR="$INSTALL_DIR"
  [ -n "$WORKBENCH_INSTALL_DIR" ] || WORKBENCH_INSTALL_DIR="$INSTALL_DIR"
  
  color_echo "green" "👍 Will install RF-Swift to: ${INSTALL_DIR}"
  return 0
}

# Print the directory of an installed executable. Only absolute command paths
# are accepted (aliases/functions are ignored), and symlinks are resolved where
# the platform provides readlink -f so the actual installation is replaced.
installed_binary_dir() {
  binary_name="$1"
  binary_path=$(command -v "$binary_name" 2>/dev/null || true)
  case "$binary_path" in /*) ;; *) return 0 ;; esac
  if command_exists readlink; then
    resolved_path=$(readlink -f "$binary_path" 2>/dev/null || true)
    [ -n "$resolved_path" ] && binary_path="$resolved_path"
  fi
  dirname "$binary_path"
}

validate_tar_archive() {
  archive_path="$1"
  if tar -tzf "$archive_path" | awk '
    /^\// { bad=1 }
    { n=split($0,p,"/"); for(i=1;i<=n;i++) if(p[i]=="..") bad=1 }
    END { exit bad ? 1 : 0 }
  '; then :; else
    color_echo "red" "🚨 Unsafe path found in $(basename "$archive_path")."
    exit 1
  fi
  if tar -tvzf "$archive_path" | awk 'substr($0,1,1) ~ /[lh]/ { bad=1 } END { exit bad ? 1 : 0 }'; then :; else
    color_echo "red" "🚨 Links are not allowed in installer archives."
    exit 1
  fi
}

validate_zip_archive() {
  archive_path="$1"
  command_exists unzip || { color_echo "red" "unzip is required to validate the Workbench archive."; exit 1; }
  if unzip -Z1 "$archive_path" | awk '
    /^\// || /^[A-Za-z]:[\\\/]/ { bad=1 }
    { gsub(/\\/,"/"); n=split($0,p,"/"); for(i=1;i<=n;i++) if(p[i]=="..") bad=1 }
    END { exit bad ? 1 : 0 }
  '; then :; else
    color_echo "red" "🚨 Unsafe path found in $(basename "$archive_path")."
    exit 1
  fi
  if command_exists zipinfo && zipinfo -l "$archive_path" | awk '$1 ~ /^l/ { bad=1 } END { exit bad ? 1 : 0 }'; then :; else
    color_echo "red" "🚨 Symbolic links are not allowed in installer ZIP archives."
    exit 1
  fi
}

# Install the binary
install_binary() {
  [ "$INSTALL_COMPONENTS" = "workbench" ] && return 0
  cli_dir="${CLI_INSTALL_DIR:-$INSTALL_DIR}"
  color_echo "blue" "🔧 Installing RF-Swift..."
  
  # Create installation directory if needed
  if [ ! -w "$cli_dir" ] && [ -e "$cli_dir" ]; then
    if ! have_sudo_access; then
      color_echo "red" "🚨 System-wide installation requires sudo. Please run with sudo or choose user-local installation."
      exit 1
    fi
    sudo mkdir -p "$cli_dir"
  else
    mkdir -p "$cli_dir"
  fi
  
  color_echo "blue" "📦 Extracting archive..."
  validate_tar_archive "${TMP_DIR}/${FILENAME}"
  tar -xzf "${TMP_DIR}/${FILENAME}" -C "${TMP_DIR}"
  
  RFSWIFT_BIN=$(find "${TMP_DIR}" -name "rfswift" -type f)
  if [ -z "${RFSWIFT_BIN}" ]; then
    color_echo "red" "🚨 Couldn't find the binary in the archive."
    exit 1
  fi

  color_echo "blue" "🚀 Moving RF-Swift to ${cli_dir}..."
  if [ ! -w "$cli_dir" ]; then
    sudo cp "${RFSWIFT_BIN}" "${cli_dir}/rfswift"
    sudo chmod +x "${cli_dir}/rfswift"
  else
    cp "${RFSWIFT_BIN}" "${cli_dir}/rfswift"
    chmod +x "${cli_dir}/rfswift"
  fi
  
  color_echo "green" "🎉 RF-Swift has been installed successfully to ${cli_dir}/rfswift!"
}

ensure_workbench_runtime() {
  [ "$OS" = "Linux" ] || return 0
  if command_exists ldconfig && ldconfig -p 2>/dev/null | grep -q 'libwebkit2gtk-4.1'; then
    return 0
  fi
  color_echo "yellow" "RF Swift Workbench requires GTK3 and WebKit2GTK 4.1 on Linux."
  if ! prompt_yes_no "Install the Workbench runtime dependencies now?" "y"; then
    color_echo "yellow" "Install GTK3 and WebKit2GTK 4.1 before launching rfswift-workbench."
    return 0
  fi
  distro=$(detect_distro)
  case "$distro" in
    debian|ubuntu) sudo apt-get update && sudo apt-get install -y libgtk-3-0 libwebkit2gtk-4.1-0 ;;
    fedora) sudo dnf install -y gtk3 webkit2gtk4.1 ;;
    arch) sudo pacman -S --needed --noconfirm gtk3 webkit2gtk-4.1 ;;
    opensuse) sudo zypper install -y libgtk-3-0 libwebkit2gtk-4_1-0 ;;
    *) color_echo "yellow" "Unknown distro: install the GTK3 and WebKit2GTK 4.1 runtime packages manually." ;;
  esac
}

install_workbench() {
  [ "$INSTALL_COMPONENTS" = "cli" ] && return 0
  color_echo "blue" "🖥️  Installing RF Swift Workbench..."
  workbench_dir="${WORKBENCH_INSTALL_DIR:-$INSTALL_DIR}"
  case "$OS" in
    Linux)
      if [ "$WORKBENCH_FORMAT" = "appimage" ]; then
        if [ ! -w "$workbench_dir" ]; then
          sudo cp "${TMP_DIR}/${WORKBENCH_FILENAME}" "$workbench_dir/rfswift-workbench"
          sudo chmod 0755 "$workbench_dir/rfswift-workbench"
        else
          cp "${TMP_DIR}/${WORKBENCH_FILENAME}" "$workbench_dir/rfswift-workbench"
          chmod 0755 "$workbench_dir/rfswift-workbench"
        fi
        color_echo "green" "✅ Portable AppImage installed as ${workbench_dir}/rfswift-workbench"
        return 0
      fi
      ensure_workbench_runtime
      workbench_unpack="${TMP_DIR}/workbench"
      mkdir -p "$workbench_unpack"
      validate_tar_archive "${TMP_DIR}/${WORKBENCH_FILENAME}"
      tar -xzf "${TMP_DIR}/${WORKBENCH_FILENAME}" -C "$workbench_unpack"
      workbench_bin=$(find "$workbench_unpack" -type f -name rfswift-workbench | head -1)
      [ -n "$workbench_bin" ] || { color_echo "red" "Workbench binary is missing from its archive."; exit 1; }
      if [ ! -w "$workbench_dir" ]; then
        sudo cp "$workbench_bin" "$workbench_dir/rfswift-workbench"
        sudo chmod 0755 "$workbench_dir/rfswift-workbench"
      else
        cp "$workbench_bin" "$workbench_dir/rfswift-workbench"
        chmod 0755 "$workbench_dir/rfswift-workbench"
      fi
      color_echo "green" "✅ Workbench installed as ${workbench_dir}/rfswift-workbench"
      ;;
    Darwin)
      validate_zip_archive "${TMP_DIR}/${WORKBENCH_FILENAME}"
      app_root="$HOME/Applications"
      if [ -d "/Applications/rfswift-workbench.app" ]; then
        app_root="/Applications"
        color_echo "green" "✅ Existing Workbench detected in /Applications; it will be replaced there."
      elif [ -d "$HOME/Applications/rfswift-workbench.app" ]; then
        color_echo "green" "✅ Existing Workbench detected in $HOME/Applications; it will be replaced there."
      else
        prompt_yes_no "Install Workbench system-wide in /Applications?" "n" && app_root="/Applications"
      fi
      mkdir -p "$app_root"
      if [ "$app_root" = "/Applications" ]; then
        sudo ditto -x -k "${TMP_DIR}/${WORKBENCH_FILENAME}" "$app_root"
      else
        ditto -x -k "${TMP_DIR}/${WORKBENCH_FILENAME}" "$app_root"
      fi
      color_echo "green" "✅ Workbench installed in ${app_root}/rfswift-workbench.app"
      ;;
  esac
}

# ═══════════════════════════════════════════════════════════════════════════════
# Logo, fonts, asciinema, and miscellaneous
# ═══════════════════════════════════════════════════════════════════════════════

# Enhanced rainbow logo with Arch Linux easter egg
display_rainbow_logo_animated() {
    # Define color variables (sh doesn't support arrays)
    RED='\033[1;31m'
    ORANGE='\033[1;33m'
    GREEN='\033[1;32m'
    CYAN='\033[1;36m'
    BLUE='\033[1;34m'
    PURPLE='\033[1;35m'
    NC='\033[0m' # No Color
    
    # Clear the screen for better presentation (clear is missing on minimal
    # installs; the script must not stop there)
    if command_exists clear; then clear 2>/dev/null || true; fi
    
    # Store the logo lines in variables (sh doesn't support arrays)
    LINE1="   888~-_   888~~        ,d88~~\\                ,e,   88~\\   d8   "
    LINE2="   888   \\  888___       8888    Y88b    e    /  \"  *888*_ *d88*_ "
    LINE3="   888    | 888          'Y88b    Y88b  d8b  /  888  888    888   "
    LINE4="   888   /  888           'Y88b,   Y888/Y88b/   888  888    888   "
    LINE5="   888_-~   888             8888    Y8/  Y8/    888  888    888   "
    LINE6="   888 ~-_  888          \\__88P'     Y    Y     888  888    \"88_/"
    
    # First display with rainbow colors
    printf "%b%s%b\n" "$RED" "$LINE1" "$NC"
    sleep 0.1
    printf "%b%s%b\n" "$ORANGE" "$LINE2" "$NC"
    sleep 0.1
    printf "%b%s%b\n" "$GREEN" "$LINE3" "$NC"
    sleep 0.1
    printf "%b%s%b\n" "$CYAN" "$LINE4" "$NC"
    sleep 0.1
    printf "%b%s%b\n" "$BLUE" "$LINE5" "$NC"
    sleep 0.1
    printf "%b%s%b\n" "$PURPLE" "$LINE6" "$NC"
    sleep 0.5
    
    # Check if we're in an interactive terminal
    if [ -t 1 ]; then
        # Animation cycle 1
        # Move cursor up 6 lines
        printf "\033[6A"
        printf "%b%s%b\n" "$ORANGE" "$LINE1" "$NC"
        printf "%b%s%b\n" "$GREEN" "$LINE2" "$NC"
        printf "%b%s%b\n" "$CYAN" "$LINE3" "$NC"
        printf "%b%s%b\n" "$BLUE" "$LINE4" "$NC"
        printf "%b%s%b\n" "$PURPLE" "$LINE5" "$NC"
        printf "%b%s%b\n" "$RED" "$LINE6" "$NC"
        sleep 0.3
        
        # Animation cycle 2
        printf "\033[6A"
        printf "%b%s%b\n" "$GREEN" "$LINE1" "$NC"
        printf "%b%s%b\n" "$CYAN" "$LINE2" "$NC"
        printf "%b%s%b\n" "$BLUE" "$LINE3" "$NC"
        printf "%b%s%b\n" "$PURPLE" "$LINE4" "$NC"
        printf "%b%s%b\n" "$RED" "$LINE5" "$NC"
        printf "%b%s%b\n" "$ORANGE" "$LINE6" "$NC"
        sleep 0.3
        
        # Animation cycle 3
        printf "\033[6A"
        printf "%b%s%b\n" "$CYAN" "$LINE1" "$NC"
        printf "%b%s%b\n" "$BLUE" "$LINE2" "$NC"
        printf "%b%s%b\n" "$PURPLE" "$LINE3" "$NC"
        printf "%b%s%b\n" "$RED" "$LINE4" "$NC"
        printf "%b%s%b\n" "$ORANGE" "$LINE5" "$NC"
        printf "%b%s%b\n" "$GREEN" "$LINE6" "$NC"
        sleep 0.3
    fi
    
    # Add a tagline with Arch Linux easter egg
    printf "\n%b🔥 RF Swift by @Penthertz - Radio Frequency Swiss Army Knife 🔥%b\n" "$PURPLE" "$NC"
    
    printf "\n"
    
    # Add a slight delay before continuing
    sleep 0.5
}

# Enhanced system verification
verify_system_requirements() {
  color_echo "blue" "🔍 Verifying system requirements..."
  
  local issues=0
  
  # Check for required tools
  if ! command_exists curl && ! command_exists wget; then
    color_echo "red" "❌ Neither curl nor wget is available. Please install one of them."
    issues=$((issues + 1))
  fi
  
  # Check for tar
  if ! command_exists tar; then
    color_echo "red" "❌ tar is not available. Please install tar."
    issues=$((issues + 1))
  fi
  
  # Check for basic shell tools
  if ! command_exists grep || ! command_exists sed; then
    color_echo "red" "❌ Basic shell tools (grep, sed) are missing."
    issues=$((issues + 1))
  fi
  
  # Arch Linux specific checks
  if is_arch_linux; then
    if ! command_exists pacman; then
      color_echo "red" "❌ pacman is not available on this Arch Linux system."
      issues=$((issues + 1))
    else
      color_echo "green" "✅ pacman package manager detected"
    fi
  fi
  
  if [ $issues -gt 0 ]; then
    color_echo "red" "🚨 System requirements check failed. Please install the missing tools."
    return 1
  fi
  
  color_echo "green" "✅ All system requirements satisfied"
  return 0
}

install_powerline_fonts() {
  local distro="$1"
  
  color_echo "blue" "🔤 Installing Powerline fonts for better terminal experience..."
  
  case "$(uname -s)" in
    Darwin*)
      color_echo "blue" "🍎 Installing fonts on macOS..."
      
      if command_exists brew; then
        color_echo "blue" "📦 Using Homebrew to install fonts..."
        
        # Tap the font cask if not already tapped
        brew tap homebrew/cask-fonts 2>/dev/null || true
        
        # Install various powerline and nerd fonts
        color_echo "blue" "Installing Powerline fonts..."
        brew install --cask font-powerline-symbols 2>/dev/null || true
        
        color_echo "blue" "Installing Nerd Fonts (recommended for Oh My Zsh)..."
        brew install --cask font-fira-code-nerd-font 2>/dev/null || true
        brew install --cask font-hack-nerd-font 2>/dev/null || true
        brew install --cask font-meslo-lg-nerd-font 2>/dev/null || true
        brew install --cask font-source-code-pro-nerd-font 2>/dev/null || true
        
        color_echo "green" "✅ Fonts installed via Homebrew"
      else
        color_echo "yellow" "⚠️ Homebrew not found. Installing fonts manually..."
        
        # Create fonts directory
        FONTS_DIR="$HOME/Library/Fonts"
        mkdir -p "$FONTS_DIR"
        
        # Download and install Powerline symbols
        color_echo "blue" "📥 Downloading Powerline symbols..."
        if command_exists curl; then
          curl -fLo "$FONTS_DIR/PowerlineSymbols.otf" \
            https://github.com/powerline/powerline/raw/develop/font/PowerlineSymbols.otf
        elif command_exists wget; then
          wget -O "$FONTS_DIR/PowerlineSymbols.otf" \
            https://github.com/powerline/powerline/raw/develop/font/PowerlineSymbols.otf
        fi
        
        # Download a popular Nerd Font
        color_echo "blue" "📥 Downloading Fira Code Nerd Font..."
        TEMP_DIR=$(mktemp -d)
        if command_exists curl; then
          curl -fLo "$TEMP_DIR/FiraCode.zip" \
            https://github.com/ryanoasis/nerd-fonts/releases/download/v3.1.1/FiraCode.zip
        elif command_exists wget; then
          wget -O "$TEMP_DIR/FiraCode.zip" \
            https://github.com/ryanoasis/nerd-fonts/releases/download/v3.1.1/FiraCode.zip
        fi
        
        if [ -f "$TEMP_DIR/FiraCode.zip" ]; then
          cd "$TEMP_DIR"
          unzip -q FiraCode.zip
          cp *.ttf *.otf "$FONTS_DIR/" 2>/dev/null || true
          rm -rf "$TEMP_DIR"
          color_echo "green" "✅ Fonts installed manually"
        fi
      fi
      ;;
      
    Linux*)
      color_echo "blue" "🐧 Installing fonts on Linux..."
      
      # Create user fonts directory
      FONTS_DIR="$HOME/.local/share/fonts"
      mkdir -p "$FONTS_DIR"
      
      case "$distro" in
        "arch")
          if have_sudo_access; then
            color_echo "blue" "📦 Using pacman to install fonts on Arch Linux..."
            sudo pacman -Sy --noconfirm
            sudo pacman -S --noconfirm --needed \
              powerline-fonts \
              ttf-fira-code \
              ttf-hack \
              ttf-meslo-nerd \
              ttf-sourcecodepro-nerd \
              noto-fonts \
              noto-fonts-emoji 2>/dev/null || true
            
            # Also try AUR fonts if yay is available
            if command_exists yay; then
              color_echo "blue" "Installing additional fonts from AUR..."
              yay -S --noconfirm nerd-fonts-complete 2>/dev/null || true
            fi
          else
            color_echo "yellow" "⚠️ No sudo access, installing fonts manually..."
            install_fonts_manually_linux
          fi
          ;;
          
        "ubuntu"|"debian")
          if have_sudo_access; then
            color_echo "blue" "📦 Using apt to install fonts..."
            sudo apt update
            sudo apt install -y \
              fonts-powerline \
              fonts-firacode \
              fonts-hack \
              fonts-noto \
              fonts-noto-color-emoji 2>/dev/null || true
            
            # Install additional Nerd Fonts manually
            install_nerd_fonts_linux
          else
            color_echo "yellow" "⚠️ No sudo access, installing fonts manually..."
            install_fonts_manually_linux
          fi
          ;;
          
        "fedora")
          if have_sudo_access; then
            color_echo "blue" "📦 Using dnf to install fonts..."
            # dnf5 rejects the whole transaction when one name is unknown, so
            # every name here must exist in the Fedora repos (Hack is packaged
            # as source-foundry-hack-fonts; there is no google-noto-fonts).
            sudo dnf install -y \
              powerline-fonts \
              fira-code-fonts \
              source-foundry-hack-fonts \
              google-noto-sans-fonts \
              google-noto-sans-mono-fonts \
              google-noto-color-emoji-fonts 2>/dev/null || true
            
            # Install additional Nerd Fonts manually
            install_nerd_fonts_linux
          else
            color_echo "yellow" "⚠️ No sudo access, installing fonts manually..."
            install_fonts_manually_linux
          fi
          ;;
          
        "rhel"|"centos")
          if have_sudo_access; then
            color_echo "blue" "📦 Installing fonts on RHEL/CentOS..."
            if command_exists dnf; then
              sudo dnf install -y powerline-fonts google-noto-fonts 2>/dev/null || true
            else
              sudo yum install -y epel-release
              sudo yum install -y powerline-fonts google-noto-fonts 2>/dev/null || true
            fi
            
            install_nerd_fonts_linux
          else
            color_echo "yellow" "⚠️ No sudo access, installing fonts manually..."
            install_fonts_manually_linux
          fi
          ;;
          
        "opensuse")
          if have_sudo_access; then
            color_echo "blue" "📦 Using zypper to install fonts..."
            sudo zypper install -y \
              powerline-fonts \
              fira-code-fonts \
              hack-fonts \
              google-noto-fonts 2>/dev/null || true
            
            install_nerd_fonts_linux
          else
            color_echo "yellow" "⚠️ No sudo access, installing fonts manually..."
            install_fonts_manually_linux
          fi
          ;;
          
        *)
          color_echo "yellow" "⚠️ Unknown distribution, installing fonts manually..."
          install_fonts_manually_linux
          ;;
      esac
      
      # Refresh font cache
      if command_exists fc-cache; then
        color_echo "blue" "🔄 Refreshing font cache..."
        fc-cache -fv >/dev/null 2>&1
        color_echo "green" "✅ Font cache refreshed"
      fi
      ;;
      
    *)
      color_echo "red" "❌ Unsupported operating system for font installation"
      return 1
      ;;
  esac
  
  return 0
}

test_font_installation() {
  color_echo "blue" "🧪 Testing font installation..."
  
  color_echo "blue" "Font test symbols:"
  echo "Powerline symbols: "
  echo "Branch symbol: "
  echo "Lock symbol: "
  echo "Lightning: ⚡"
  echo "Gear: ⚙"
  echo "Arrow: ➜"
  
  color_echo "yellow" "If you see boxes or question marks instead of symbols,"
  color_echo "yellow" "restart your terminal and ensure it's using a Nerd Font."
}

show_font_configuration_help() {
  color_echo "cyan" "📝 Terminal Font Configuration Help:"
  echo "=================================="
  
  case "$(uname -s)" in
    Darwin*)
      color_echo "blue" "🍎 macOS Terminal Configuration:"
      color_echo "cyan" "- Terminal.app: Preferences -> Profiles -> Text -> Font"
      color_echo "cyan" "- iTerm2: Preferences -> Profiles -> Text -> Font"
      color_echo "cyan" "- Recommended fonts: 'Fira Code Nerd Font', 'Hack Nerd Font'"
      ;;
    Linux*)
      color_echo "blue" "🐧 Linux Terminal Configuration:"
      color_echo "cyan" "- GNOME Terminal: Preferences -> Profiles -> Text -> Custom font"
      color_echo "cyan" "- Konsole: Settings -> Edit Current Profile -> Appearance -> Font"
      color_echo "cyan" "- Alacritty: Edit ~/.config/alacritty/alacritty.yml"
      color_echo "cyan" "- Terminator: Right-click -> Preferences -> Profiles -> Font"
      color_echo "cyan" "- VS Code: Settings -> Terminal -> Font Family"
      ;;
  esac
  
  echo "=================================="
}

check_agnoster_dependencies() {
  color_echo "blue" "🔍 Checking agnoster theme dependencies..."
  
  local issues=0
  local distro=$(detect_distro)
  
  # Check for fonts
  color_echo "blue" "Checking for Powerline fonts..."
  
  case "$(uname -s)" in
    Darwin*)
      # Check if fonts exist in macOS
      if [ ! -f "$HOME/Library/Fonts/PowerlineSymbols.otf" ] && ! ls "$HOME/Library/Fonts"/*Nerd* >/dev/null 2>&1; then
        color_echo "yellow" "⚠️ Powerline/Nerd fonts not found in user fonts directory"
        issues=$((issues + 1))
      fi
      ;;
    Linux*)
      # Check if fonts exist in Linux
      if [ ! -f "$HOME/.local/share/fonts/PowerlineSymbols.otf" ] && ! ls "$HOME/.local/share/fonts"/*Nerd* >/dev/null 2>&1; then
        # Also check system fonts
        if ! fc-list | grep -i powerline >/dev/null 2>&1 && ! fc-list | grep -i nerd >/dev/null 2>&1; then
          color_echo "yellow" "⚠️ Powerline/Nerd fonts not found"
          issues=$((issues + 1))
        fi
      fi
      ;;
  esac
  
  # Check terminal capabilities
  if [ -z "$TERM" ] || ! echo "$TERM" | grep -q "256color"; then
    color_echo "yellow" "⚠️ Terminal may not support 256 colors (TERM=$TERM)"
    color_echo "cyan" "💡 Try setting: export TERM=xterm-256color"
  fi
  
  # Check for Git (agnoster shows git status)
  if ! command_exists git; then
    color_echo "yellow" "⚠️ Git not found (agnoster theme shows git information)"
    issues=$((issues + 1))
  fi
  
  if [ $issues -gt 0 ]; then
    color_echo "yellow" "⚠️ Found $issues potential issues with agnoster dependencies"
    
    if prompt_yes_no "Would you like to install missing fonts?" "y"; then
      install_powerline_fonts "$distro"
      test_font_installation
      show_font_configuration_help
    fi
  else
    color_echo "green" "✅ All agnoster dependencies appear to be satisfied"
  fi
}

install_nerd_fonts_linux() {
  color_echo "blue" "📥 Installing Nerd Fonts manually..."
  if ! command_exists unzip; then
    color_echo "yellow" "⚠️  'unzip' is not installed; skipping the Nerd Fonts download (install unzip and re-run, or use your distribution's nerd-font packages)."
    return 0
  fi
  
  FONTS_DIR="$HOME/.local/share/fonts"
  mkdir -p "$FONTS_DIR"
  
  # Download popular Nerd Fonts
  TEMP_DIR=$(mktemp -d)
  
  # Fira Code Nerd Font
  if command_exists curl; then
    curl -fLo "$TEMP_DIR/FiraCode.zip" \
      https://github.com/ryanoasis/nerd-fonts/releases/download/v3.1.1/FiraCode.zip
  elif command_exists wget; then
    wget -O "$TEMP_DIR/FiraCode.zip" \
      https://github.com/ryanoasis/nerd-fonts/releases/download/v3.1.1/FiraCode.zip
  fi
  
  # Hack Nerd Font
  if command_exists curl; then
    curl -fLo "$TEMP_DIR/Hack.zip" \
      https://github.com/ryanoasis/nerd-fonts/releases/download/v3.1.1/Hack.zip
  elif command_exists wget; then
    wget -O "$TEMP_DIR/Hack.zip" \
      https://github.com/ryanoasis/nerd-fonts/releases/download/v3.1.1/Hack.zip
  fi
  
  # Extract and install fonts
  cd "$TEMP_DIR"
  for font_zip in *.zip; do
    if [ -f "$font_zip" ]; then
      color_echo "blue" "Extracting $font_zip..."
      unzip -o -q "$font_zip" -d "${font_zip%.zip}"  # Extract to subdirectory
      find "${font_zip%.zip}" -type f \( -name "*.ttf" -o -name "*.otf" \) -exec cp {} "$FONTS_DIR/" \;
    fi
  done
  
  cd - >/dev/null
  rm -rf "$TEMP_DIR"
  color_echo "green" "✅ Nerd Fonts installed manually"
}

install_fonts_manually_linux() {
  color_echo "blue" "📥 Installing fonts manually (no package manager)..."
  
  FONTS_DIR="$HOME/.local/share/fonts"
  mkdir -p "$FONTS_DIR"
  
  # Install Powerline symbols
  color_echo "blue" "Installing Powerline symbols..."
  if command_exists curl; then
    curl -fLo "$FONTS_DIR/PowerlineSymbols.otf" \
      https://github.com/powerline/powerline/raw/develop/font/PowerlineSymbols.otf
  elif command_exists wget; then
    wget -O "$FONTS_DIR/PowerlineSymbols.otf" \
      https://github.com/powerline/powerline/raw/develop/font/PowerlineSymbols.otf
  fi
  
  # Install Nerd Fonts
  install_nerd_fonts_linux
}

# Check and install asciinema for terminal recording
check_asciinema() {
    if command -v asciinema >/dev/null 2>&1; then
        color_echo "green" "✅ asciinema is already installed. Moving on. ✅"
        return 0
    fi
    
    color_echo "yellow" "⚠️ asciinema is not installed on this system."
    color_echo "blue" "ℹ️ asciinema allows you to record and share terminal sessions."
    
    if ! prompt_yes_no "Would you like to install asciinema?" "n"; then
        color_echo "yellow" "⚠️ asciinema installation skipped."
        return 0
    fi
    
    color_echo "blue" "📦 Installing asciinema..."
    
    local distro=$(detect_distro)
    case "$(uname -s)" in
        Darwin*)
            if command -v brew >/dev/null 2>&1; then
                color_echo "blue" "🍎 Installing asciinema via Homebrew..."
                brew install asciinema
            else
                color_echo "yellow" "⚠️ Homebrew not found. Installing via pip..."
                if command -v pip3 >/dev/null 2>&1; then
                    pip3 install asciinema
                elif command -v pip >/dev/null 2>&1; then
                    pip install asciinema
                else
                    color_echo "red" "❌ Neither Homebrew nor pip found. Please install asciinema manually."
                    return 1
                fi
            fi
            ;;
        Linux*)
            case "$distro" in
                "arch")
                    if have_sudo_access; then
                        color_echo "cyan" "🏛️ Installing asciinema using pacman on Arch Linux... 📦"
                        sudo pacman -Sy --noconfirm
                        sudo pacman -S --noconfirm --needed asciinema
                    else
                        color_echo "red" "❌ sudo access required for package installation"
                        return 1
                    fi
                    ;;
                "fedora")
                    if have_sudo_access; then
                        color_echo "yellow" "📦 Installing asciinema using dnf... 📦"
                        sudo dnf install -y asciinema
                    else
                        color_echo "red" "❌ sudo access required for package installation"
                        return 1
                    fi
                    ;;
                "rhel"|"centos")
                    if have_sudo_access; then
                        if command -v dnf >/dev/null 2>&1; then
                            color_echo "yellow" "📦 Installing asciinema using dnf... 📦"
                            sudo dnf install -y asciinema
                        else
                            color_echo "yellow" "📦 Installing asciinema using pip... 📦"
                            sudo yum install -y python3-pip
                            pip3 install asciinema
                        fi
                    else
                        color_echo "red" "❌ sudo access required for package installation"
                        return 1
                    fi
                    ;;
                "debian"|"ubuntu")
                    if have_sudo_access; then
                        color_echo "yellow" "📦 Installing asciinema using apt... 📦"
                        sudo apt update
                        sudo apt install -y asciinema
                    else
                        color_echo "red" "❌ sudo access required for package installation"
                        return 1
                    fi
                    ;;
                "opensuse")
                    if have_sudo_access; then
                        color_echo "yellow" "📦 Installing asciinema using zypper... 📦"
                        sudo zypper install -y asciinema
                    else
                        color_echo "red" "❌ sudo access required for package installation"
                        return 1
                    fi
                    ;;
                *)
                    color_echo "yellow" "⚠️ Unknown distribution. Trying pip installation..."
                    if command -v pip3 >/dev/null 2>&1; then
                        pip3 install --user asciinema
                    elif command -v pip >/dev/null 2>&1; then
                        pip install --user asciinema
                    else
                        color_echo "red" "❌ Unsupported package manager and pip not found. Please install asciinema manually."
                        return 1
                    fi
                    ;;
            esac
            ;;
        *)
            color_echo "red" "❌ Unsupported operating system for asciinema installation"
            return 1
            ;;
    esac
    
    # Verify installation
    if command -v asciinema >/dev/null 2>&1; then
        color_echo "green" "✅ asciinema installed successfully. ✅"
        color_echo "cyan" "💡 Tip: Run 'asciinema rec' to start recording your terminal session."
        return 0
    else
        color_echo "yellow" "⚠️ asciinema may have been installed but is not in PATH."
        color_echo "cyan" "💡 Try restarting your terminal or check ~/.local/bin/"
        return 0
    fi
}

# ═══════════════════════════════════════════════════════════════════════════════
# Main
# ═══════════════════════════════════════════════════════════════════════════════

# Debian's `su` (without `-`) and ordinary user shells have no sbin directory
# on PATH, and the Determinate installer looks `groupadd`/`addgroup` up on the
# caller's PATH before it escalates. It then stops with "Could not find a
# supported command to create groups" although /usr/sbin/groupadd is right
# there. Put the sbin directories on PATH for the installer; sudo's secure_path
# runs the same binaries for the privileged steps anyway.
ensure_sbin_on_path() {
  local dir
  for dir in /usr/local/sbin /usr/sbin /sbin; do
    [ -d "$dir" ] || continue
    case ":$PATH:" in
      *":$dir:"*) ;;
      *) PATH="$PATH:$dir" ;;
    esac
  done
  export PATH
}

# Run a command as root: directly when already root, through sudo otherwise.
run_as_root() {
  if [ "$(id -u 2>/dev/null)" = "0" ]; then
    "$@"
  else
    sudo "$@"
  fi
}

# The official installer leaves flakes and nix-command off; RF Swift's engine
# needs both, so enable them system-wide and restart the daemon.
nix_enable_flakes() {
  local conf="/etc/nix/nix.conf"
  if grep -qs 'experimental-features.*flakes' "$conf"; then
    return 0
  fi
  if [ "$(id -u 2>/dev/null)" != "0" ] && ! have_sudo_access; then
    color_echo "yellow" "   Add 'experimental-features = nix-command flakes' to $conf to finish enabling flakes."
    return 0
  fi
  printf 'experimental-features = nix-command flakes\n' | run_as_root tee -a "$conf" >/dev/null
  if command_exists systemctl; then
    run_as_root systemctl restart nix-daemon >/dev/null 2>&1 || true
  fi
}

# The installers are fetched with curl or, on hosts without it (a stock Debian
# desktop ships wget only), with wget.
fetch_installer() {
  if command_exists curl; then
    curl --proto '=https' --tlsv1.2 -sSf -L "$1"
  elif command_exists wget; then
    wget -qO- --https-only "$1"
  else
    color_echo "red" "🚨 Neither curl nor wget is available to download the Nix installer."
    return 1
  fi
}

nix_installer_determinate() {
  color_echo "blue" "🚀 Running the Determinate Systems Nix installer (enables flakes + nix-command)..."
  fetch_installer https://install.determinate.systems/nix | sh -s -- install --no-confirm
}

nix_installer_official() {
  color_echo "yellow" "⚠️  The Determinate installer did not complete; trying the official NixOS multi-user installer..."
  if ! command_exists xz; then
    color_echo "yellow" "   It unpacks with 'xz' (Debian/Ubuntu: sudo apt-get install xz-utils; Fedora: sudo dnf install xz)."
  fi
  fetch_installer https://nixos.org/nix/install | sh -s -- --daemon --yes || return 1
  nix_enable_flakes
}

install_nix() {
  color_echo "blue" "❄️  Installing Nix (native engine)..."
  if command_exists nix; then
    color_echo "green" "✅ Nix is already installed."
    return 0
  fi
  if [ "$(uname)" = "Linux" ]; then
    ensure_sbin_on_path
    if ! command_exists groupadd && ! command_exists addgroup; then
      color_echo "red" "🚨 Neither 'groupadd' nor 'addgroup' is available; the Nix installer needs one to create the nixbld build group."
      color_echo "cyan" "   Install it first (Debian/Ubuntu: 'sudo apt-get install passwd'; Alpine: 'apk add shadow'), then re-run."
      return 1
    fi
  fi
  if nix_installer_determinate || nix_installer_official; then
    color_echo "green" "✅ Nix installed."
    color_echo "cyan" "   Open a new shell (or 'source /nix/var/nix/profiles/default/etc/profile.d/nix-daemon.sh') so 'nix' is on PATH,"
    color_echo "cyan" "   then:  rfswift --engine nix run <environment>"
    color_echo "cyan" "   Tip: set 'engine = nix' under [general] in ~/.config/rfswift/config.ini to make Nix the default."
  else
    color_echo "red" "🚨 Nix installation failed. Install it manually from https://nixos.org/download and re-run."
    return 1
  fi
}

# Bubblewrap powers the Nix engine's `--isolate` jail (Linux user namespaces):
# it hides $HOME and the host filesystem while keeping USB/serial devices, the
# display and the network. Optional - without it, `--isolate` builds bwrap from
# nixpkgs on first use - but installing it makes the jail work out of the box.
check_bubblewrap() {
  # Linux only: bubblewrap relies on Linux namespaces (macOS is unsupported).
  if [ "$(uname)" != "Linux" ]; then
    return 0
  fi
  if command_exists bwrap; then
    color_echo "green" "✅ bubblewrap present - 'rfswift run --engine nix --isolate' works out of the box."
    return 0
  fi
  color_echo "cyan" "   bubblewrap enables the Nix engine's --isolate jail: hides \$HOME and the host filesystem while keeping USB devices, the display and the network."
  choice=$(prompt_choice "Install bubblewrap for the Nix engine's --isolate jail?" "Yes" "No")
  if [ "$choice" != "1" ]; then
    color_echo "yellow" "   Skipped. --isolate will build bubblewrap from nixpkgs on first use, or install it later (e.g. 'sudo apt install bubblewrap')."
    return 0
  fi
  pm=$(get_package_manager)
  case "$pm" in
    apt)    sudo apt update && sudo apt install -y bubblewrap ;;
    dnf)    sudo dnf install -y bubblewrap ;;
    yum)    sudo yum install -y bubblewrap ;;
    pacman) sudo pacman -Sy --noconfirm --needed bubblewrap ;;
    zypper) sudo zypper install -y bubblewrap ;;
    apk)    sudo apk add bubblewrap ;;
    emerge) sudo emerge --ask=n sys-apps/bubblewrap ;;
    *)      color_echo "yellow" "   Unknown package manager; install 'bubblewrap' manually to use --isolate." ; return 0 ;;
  esac
  if command_exists bwrap; then
    color_echo "green" "✅ bubblewrap installed."
  else
    color_echo "yellow" "   bubblewrap install did not complete; --isolate will fall back to building it from nixpkgs."
  fi
  check_userns
}

# bubblewrap needs unprivileged user namespaces unless it is setuid-root.
# Ubuntu 24.04+/Debian restrict them by default (AppArmor), which makes
# --isolate fail with a uid-map permission error. Detect it and offer to relax.
check_userns() {
  command_exists bwrap || return 0
  if bwrap --ro-bind / / --proc /proc -- true >/dev/null 2>&1; then
    color_echo "green" "✅ bubblewrap sandbox works - 'rfswift run --engine nix --isolate' is ready."
    return 0
  fi
  if [ -u "$(command -v bwrap)" ]; then
    color_echo "yellow" "   bubblewrap is setuid but the sandbox test failed; check your AppArmor/seccomp policy."
    return 0
  fi
  color_echo "yellow" "   bubblewrap cannot create a sandbox yet: unprivileged user namespaces look restricted (default on Ubuntu 24.04+/Debian)."
  choice=$(prompt_choice "Enable unprivileged user namespaces for --isolate (sysctl, persisted)?" "Yes" "No")
  if [ "$choice" != "1" ]; then
    color_echo "yellow" "   Skipped. --isolate needs unprivileged user namespaces enabled or a setuid bubblewrap."
    return 0
  fi
  sudo sysctl -w kernel.apparmor_restrict_unprivileged_userns=0 >/dev/null 2>&1 || true
  sudo sysctl -w kernel.unprivileged_userns_clone=1 >/dev/null 2>&1 || true
  printf 'kernel.apparmor_restrict_unprivileged_userns=0\nkernel.unprivileged_userns_clone=1\n' \
    | sudo tee /etc/sysctl.d/99-rfswift-userns.conf >/dev/null 2>&1 || true
  if bwrap --ro-bind / / --proc /proc -- true >/dev/null 2>&1; then
    color_echo "green" "✅ Unprivileged user namespaces enabled - --isolate is ready."
  else
    color_echo "yellow" "   Still restricted; a reboot or an AppArmor policy change may be required."
  fi
}

# RF Swift can run tool environments natively via Nix (rfswift --engine nix),
# with no container. Offer to install it alongside (or instead of) a container
# engine.
check_nix_engine() {
  color_echo "blue" "❄️  Native engine (Nix)"
  if command_exists nix; then
    color_echo "green" "✅ Nix is available - RF Swift can run tools natively with 'rfswift --engine nix' (no container)."
    check_bubblewrap
    return 0
  fi
  color_echo "cyan" "   RF Swift can also run its tool environments natively via Nix - no container needed."
  case "$NIX_PREF" in
    1|yes|true) choice=1; color_echo "cyan" "   RFSWIFT_NIX=${NIX_PREF}" ;;
    0|no|false) choice=2; color_echo "cyan" "   RFSWIFT_NIX=${NIX_PREF}" ;;
    *) choice=$(prompt_choice "Install Nix for the native engine?" "Yes" "No") ;;
  esac
  if [ "$choice" = "1" ]; then
    # A Nix installer that gives up must not end the RF Swift install.
    if install_nix; then
      check_bubblewrap
    else
      color_echo "yellow" "⚠️  Nix setup did not complete; RF Swift itself is still installed. Retry later from https://nixos.org/download"
    fi
  else
    color_echo "yellow" "   Skipped. Install Nix later from https://nixos.org/download to use '--engine nix'."
  fi
}

main() {
  # A root shell reached with plain `su` keeps the user's PATH, without the
  # sbin directories (Debian's su no longer adds them). dpkg, ldconfig and the
  # Nix installer then report tools as missing that are installed; see
  # ensure_sbin_on_path. Normalise once, for the whole run.
  ensure_sbin_on_path
  provide_sudo_shim

  display_rainbow_logo_animated

  fun_welcome
  choose_release_and_components
  
  # Verify system requirements first
  if ! verify_system_requirements; then
    color_echo "red" "🚨 Cannot proceed due to missing system requirements."
    exit 1
  fi
  warn_without_root_access
  
  # Show Steam Deck detection status
  if is_steam_deck; then
    color_echo "magenta" "🎮 Steam Deck detected! Special optimizations will be applied."
  fi
  
  # Host extras: a container engine, the Nix engine and audio. Each is
  # optional, so a failure there is reported and the RF Swift install goes on
  # (a plain call would end the script through `set -e` before anything of
  # RF Swift itself is installed).
  check_container_engine || color_echo "yellow" "⚠️  Container engine setup did not complete; continuing with the RF Swift install."

  # Offer the native Nix engine (rfswift --engine nix)
  check_nix_engine || color_echo "yellow" "⚠️  Nix setup did not complete; continuing with the RF Swift install."

  # Check and install audio system
  check_audio_system || color_echo "yellow" "⚠️  Audio setup did not complete; continuing with the RF Swift install."

  # Check X11 display forwarding (installs XQuartz on macOS). This is a host
  # dependency like the container engine and audio above, so it runs here —
  # before the binary download — so a failed/skipped download or a dev channel
  # without a published asset never leaves GUI tools (gqrx, ...) without a
  # display to connect to.
  check_xhost

  # Get latest release info
  get_latest_release

  # Detect system architecture
  detect_system

  # Prefer native packages (deb/rpm/pacman, or the Homebrew cask on macOS);
  # any miss falls back to the classic tarball flow below.
  if try_native_package_install; then
    # Linux packages land in /usr/bin; make sure nothing older shadows them.
    case "$(uname -s)" in Linux*) cleanup_legacy_installs ;; esac
  else
    choose_workbench_format

    # Download files
    download_files

    # Choose installation directory
    choose_install_dir

    # Install binary
    install_binary
    install_workbench
    rm -rf "${TMP_DIR}"
  fi

  # udev rules for RF hardware (asks; RFSWIFT_UDEV=1|0 to answer up front)
  offer_udev_rules || true

  # Fonts and asciinema are cosmetic extras; never let them end the install.
  check_agnoster_dependencies || color_echo "yellow" "⚠️  Font setup did not complete."
  check_asciinema || color_echo "yellow" "⚠️  asciinema setup did not complete."

  # Set up alias if requested. Native packages land on PATH with completions
  # already installed, and a directory that is already on PATH needs none, so
  # the alias only matters for tarball installs into a private directory.
  if [ "$NATIVE_INSTALLED" != true ] && ! dir_on_path "$INSTALL_DIR" && prompt_yes_no "Would you like to set up an alias for RF-Swift?" "y"; then
    create_alias "$INSTALL_DIR" || true
  fi
  
  # Show audio system status
  show_audio_status
  
  thank_you_message
  
  # Final instructions should only advertise components actually requested.
  if [ "$INSTALL_COMPONENTS" = "cli" ] || [ "$INSTALL_COMPONENTS" = "both" ]; then
    if [ "$NATIVE_INSTALLED" != true ] && ! dir_on_path "$INSTALL_DIR"; then
      color_echo "cyan" "🚀 To use RF-Swift, you can:"
      color_echo "cyan" "   - Run it directly: ${INSTALL_DIR}/rfswift"
      color_echo "cyan" "   - Add ${INSTALL_DIR} to your PATH"
      if is_arch_linux; then
        color_echo "cyan" "   - Or use the alias if you set it up: rfswift"
      fi
    else
      color_echo "cyan" "🚀 You can now run RF-Swift by simply typing: rfswift"
    fi
  fi
  if [ "$INSTALL_COMPONENTS" = "workbench" ] || [ "$INSTALL_COMPONENTS" = "both" ]; then
    case "$(uname -s)" in
      Darwin*) color_echo "cyan" "🖥️  Open RF Swift Workbench from Applications." ;;
      *) color_echo "cyan" "🖥️  Start the GUI with: ${INSTALL_DIR}/rfswift-workbench" ;;
    esac
  fi
  
  # Show container engine status
  detect_container_engines
  if [ "$HAS_DOCKER" = true ] && [ "$HAS_PODMAN" = true ]; then
    color_echo "cyan" "🐳🦭 Both Docker and Podman available - RF-Swift will auto-detect at runtime."
  elif [ "$HAS_DOCKER" = true ]; then
    color_echo "cyan" "🐳 Container engine: Docker"
  elif [ "$HAS_PODMAN" = true ]; then
    color_echo "cyan" "🦭 Container engine: Podman (rootless)"
  else
    color_echo "yellow" "⚠️  No container engine installed - please install Docker or Podman before using RF-Swift."
  fi

  case "$(uname -s)" in
    Darwin*)
      color_echo "cyan" "🎵 For container audio on macOS, see: brew install pulseaudio"
      # Offer Lima installation for USB passthrough on macOS
      echo ""
      color_echo "cyan" "🦙 Lima VM enables USB device passthrough for SDR hardware on macOS."
      if command_exists limactl; then
        color_echo "green" "   Lima is already installed."
        if limactl list --json 2>/dev/null | grep -q '"name":"rfswift"'; then
          color_echo "green" "   rfswift Lima instance exists."
          color_echo "yellow" "   Tip: update your Lima template if upgrading (new modules, Bluetooth, udev rules):"
          color_echo "cyan" "   rfswift engine lima reconfig"
        else
          color_echo "yellow" "   No rfswift Lima instance yet. Create one with:"
          color_echo "cyan" "   limactl create --name rfswift lima/rfswift.yaml && limactl start rfswift"
          color_echo "cyan" "   Or let RF Swift auto-create it on first 'rfswift run' when no Docker/Podman is found."
        fi
      else
        color_echo "yellow" "   Lima is not installed. Run the installer, or: brew install qemu lima"
        color_echo "cyan" "   After installing, RF Swift can auto-manage a QEMU VM with USB passthrough."
        color_echo "cyan" "   USB commands: rfswift macusb list | attach | detach | status"
      fi
      ;;
    *)
      color_echo "magenta" "🎵 Audio system is configured and ready for RF-Swift containers!"
      ;;
  esac
  
  # Arch Linux specific final message
  if is_arch_linux; then
    color_echo "cyan" "🏛️ Arch Linux optimized installation complete!"
    color_echo "cyan" "💡 All packages were installed using pacman for optimal integration"
  fi
  
  # Steam Deck specific final message
  if is_steam_deck; then
    echo -e "${YELLOW}[+] 🔒 Re-enabling read-only mode on Steam Deck 🔒${NC}"
    sudo steamos-readonly enable
    color_echo "magenta" "🎮 Steam Deck setup complete! RF-Swift is optimized for your device."
    color_echo "cyan" "💡 Tip: You may need to reboot or log out/in for Docker group changes to take effect."
  fi

  # Suggest profile initialization/update when the CLI was installed.
  if [ "$INSTALL_COMPONENTS" = "cli" ] || [ "$INSTALL_COMPONENTS" = "both" ]; then
    echo ""
    color_echo "cyan" "📋 Default profiles provide quick-start presets for common RF tasks."
    color_echo "cyan" "   Initialize or update them with: rfswift profile init --force"
  fi

  color_echo "cyan" "📡 Happy RF hacking! 🚀"
}

# Run normally, while allowing the installer test suite to source the validated
# helper functions without performing network or system changes.
if [ "${RFSWIFT_INSTALLER_LIB_ONLY:-0}" != "1" ]; then
  main
fi
