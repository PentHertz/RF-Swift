#!/bin/bash

# This code is part of RF Swift by @Penthertz
# Author(s): Sébastien Dudek (@FlUxIuS)

# Stop the script if any command fails
set -euo pipefail

GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
MAGENTA='\033[0;35m'
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

# Function to prompt user for yes/no with terminal redirection solution
prompt_yes_no() {
  local prompt="$1"
  local default="$2"  # Optional default (y/n)
  local response
  
  # Try to use /dev/tty for interactive input even in pipe scenarios
  if [ -t 0 ]; then
    tty_device="/dev/stdin"
  elif [ -e "/dev/tty" ]; then
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
    if read -r response < "$tty_device" 2>/dev/null; then
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

# Function to prompt user for a numbered choice (output goes to stderr, only result to stdout)
prompt_choice() {
  local prompt="$1"
  shift
  local options="$@"
  local response
  local num=1

  if [ -t 0 ]; then
    tty_device="/dev/stdin"
  elif [ -e "/dev/tty" ]; then
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
    if read -r response < "$tty_device" 2>/dev/null; then
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

# Function to check if a command exists
command_exists() {
  command -v "$1" >/dev/null 2>&1
}


# Enhanced package manager detection
get_package_manager() {
    # Prioritize Arch Linux package manager
    if is_arch_linux && command -v pacman &> /dev/null; then
        echo "pacman"
        return 0
    fi
    
    # Check for other package managers
    if command -v dnf &> /dev/null; then
        echo "dnf"
    elif command -v yum &> /dev/null; then
        echo "yum"
    elif command -v apt &> /dev/null; then
        echo "apt"
    elif command -v zypper &> /dev/null; then
        echo "zypper"
    elif command -v apk &> /dev/null; then
        echo "apk"
    elif command -v emerge &> /dev/null; then
        echo "emerge"
    else
        echo "unknown"
    fi
}

# Check if PipeWire is running
is_pipewire_running() {
    if command -v pgrep &> /dev/null; then
        pgrep -x pipewire &> /dev/null && return 0
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
    if command -v pulseaudio &> /dev/null; then
        pulseaudio --check &> /dev/null
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

# Install PipeWire packages with enhanced Arch Linux support
install_pipewire() {
    local distro="$1"
    local pkg_manager="$2"
    
    echo -e "${YELLOW}🔊 Installing PipeWire... 🔊${NC}"
    
    case "$distro" in
        "arch")
            echo -e "${CYAN}🏛️ Using pacman for PipeWire installation on Arch Linux${NC}"
            sudo pacman -Sy --noconfirm
            sudo pacman -S --noconfirm --needed pipewire pipewire-pulse pipewire-alsa pipewire-jack wireplumber
            sudo pacman -S --noconfirm --needed pipewire-audio pipewire-media-session || true
            ;;
        "fedora")
            sudo dnf install -y pipewire pipewire-pulseaudio pipewire-alsa pipewire-jack-audio-connection-kit wireplumber
            ;;
        "rhel"|"centos")
            if command -v dnf &> /dev/null; then
                sudo dnf install -y pipewire pipewire-pulseaudio pipewire-alsa wireplumber
            else
                echo -e "${YELLOW}ℹ️ PipeWire not available on RHEL/CentOS 7, installing PulseAudio instead ℹ️${NC}"
                sudo yum install -y epel-release
                sudo yum install -y pulseaudio pulseaudio-utils alsa-utils
                return
            fi
            ;;
        "debian"|"ubuntu")
            sudo apt update
            sudo apt install -y pipewire pipewire-pulse pipewire-alsa wireplumber
            ;;
        "opensuse")
            sudo zypper install -y pipewire pipewire-pulseaudio pipewire-alsa wireplumber
            ;;
        *)
            echo -e "${RED}❌ Unsupported distribution for PipeWire installation ❌${NC}"
            return 1
            ;;
    esac
    
    echo -e "${YELLOW}🔧 Enabling PipeWire services... 🔧${NC}"
    systemctl --user enable pipewire.service pipewire-pulse.service 2>/dev/null || true
    systemctl --user enable wireplumber.service 2>/dev/null || true
}

# Install PulseAudio packages with enhanced Arch Linux support
install_pulseaudio() {
    local distro="$1"
    local pkg_manager="$2"
    
    echo -e "${YELLOW}🔊 Installing PulseAudio... 🔊${NC}"
    
    case "$distro" in
        "arch")
            echo -e "${CYAN}🏛️ Using pacman for PulseAudio installation on Arch Linux${NC}"
            sudo pacman -Sy --noconfirm
            sudo pacman -S --noconfirm --needed pulseaudio pulseaudio-alsa alsa-utils
            sudo pacman -S --noconfirm --needed pulseaudio-bluetooth pavucontrol || true
            ;;
        "fedora")
            sudo dnf install -y pulseaudio pulseaudio-utils alsa-utils
            ;;
        "rhel"|"centos")
            if command -v dnf &> /dev/null; then
                sudo dnf install -y pulseaudio pulseaudio-utils alsa-utils
            else
                sudo yum install -y epel-release
                sudo yum install -y pulseaudio pulseaudio-utils alsa-utils
            fi
            ;;
        "debian"|"ubuntu")
            sudo apt update
            sudo apt install -y pulseaudio pulseaudio-utils alsa-utils
            ;;
        "opensuse")
            sudo zypper install -y pulseaudio pulseaudio-utils alsa-utils
            ;;
        *)
            echo -e "${RED}❌ Unsupported distribution for PulseAudio installation ❌${NC}"
            return 1
            ;;
    esac
}

# Start PipeWire
start_pipewire() {
    echo -e "${YELLOW}🎵 Starting PipeWire... 🎵${NC}"
    
    if systemctl --user start pipewire.service pipewire-pulse.service 2>/dev/null; then
        systemctl --user start wireplumber.service 2>/dev/null || true
        echo -e "${GREEN}🎧 PipeWire started via systemd services 🎧${NC}"
    else
        pipewire &
        pipewire-pulse &
        wireplumber &
        sleep 2
        echo -e "${GREEN}🎧 PipeWire started directly 🎧${NC}"
    fi
}

# Start PulseAudio
start_pulseaudio() {
    echo -e "${YELLOW}🎵 Starting PulseAudio... 🎵${NC}"
    pulseaudio --check &> /dev/null || pulseaudio --start
    echo -e "${GREEN}🎧 PulseAudio is running 🎧${NC}"
}

# Check if we should prefer PipeWire for this distribution
should_prefer_pipewire() {
    local distro="$1"
    
    case "$distro" in
        "arch")       return 0 ;;
        "fedora")     return 0 ;;
        "ubuntu"|"debian") return 0 ;;
        "opensuse")   return 0 ;;
        "rhel"|"centos")
            command -v dnf &> /dev/null
            ;;
        *)
            return 1
            ;;
    esac
}

# Enhanced audio system check with better Arch Linux support
check_audio_system() {
    if [[ "$OSTYPE" == "darwin"* ]]; then
        echo -e "${YELLOW}🍎 Detected macOS. Checking for Homebrew... 🍎${NC}"
        if ! command -v brew &> /dev/null; then
            echo -e "${RED}❌ Homebrew is not installed. Please install Homebrew first. ❌${NC}"
            echo -e "${YELLOW}ℹ️ You can install Homebrew by running: ℹ️${NC}"
            echo -e "${BLUE}🍺 /bin/bash -c \"\$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)\" 🍺${NC}"
            exit 1
        fi
        
        if ! command -v pulseaudio &> /dev/null; then
            echo -e "${YELLOW}🔊 Installing PulseAudio using Homebrew... 🔊${NC}"
            brew install pulseaudio
        fi
        echo -e "${GREEN}✅ PulseAudio installed on macOS ✅${NC}"
        echo -e "${YELLOW}🍎 macOS detected. Audio server management is handled by the system 🍎${NC}"
        return
    fi

    local distro=$(detect_distro)
    local pkg_manager=$(get_package_manager)
    local current_audio=$(detect_audio_system)
    
    echo -e "${BLUE}🐧 Detected distribution: $distro 🐧${NC}"
    echo -e "${BLUE}📦 Package manager: $pkg_manager 📦${NC}"
    
    case "$current_audio" in
        "pipewire")
            echo -e "${GREEN}✅ PipeWire is already running ✅${NC}"
            return
            ;;
        "pulseaudio")
            echo -e "${GREEN}✅ PulseAudio is already running ✅${NC}"
            return
            ;;
        "none")
            echo -e "${YELLOW}⚠️ No audio system detected ⚠️${NC}"
            ;;
    esac
    
    if should_prefer_pipewire "$distro"; then
        echo -e "${BLUE}🎯 PipeWire is recommended for $distro 🎯${NC}"
        
        if command -v pipewire &> /dev/null || command -v pw-cli &> /dev/null; then
            echo -e "${GREEN}✅ PipeWire is already installed ✅${NC}"
            start_pipewire
        else
            echo -e "${YELLOW}📦 Installing PipeWire... 📦${NC}"
            if install_pipewire "$distro" "$pkg_manager"; then
                echo -e "${GREEN}✅ PipeWire installed successfully ✅${NC}"
                start_pipewire
            else
                echo -e "${RED}❌ Failed to install PipeWire, falling back to PulseAudio ❌${NC}"
                install_pulseaudio "$distro" "$pkg_manager"
                start_pulseaudio
            fi
        fi
    else
        echo -e "${BLUE}🎯 PulseAudio is recommended for $distro 🎯${NC}"
        
        if command -v pulseaudio &> /dev/null; then
            echo -e "${GREEN}✅ PulseAudio is already installed ✅${NC}"
            start_pulseaudio
        else
            echo -e "${YELLOW}📦 Installing PulseAudio... 📦${NC}"
            if install_pulseaudio "$distro" "$pkg_manager"; then
                echo -e "${GREEN}✅ PulseAudio installed successfully ✅${NC}"
                start_pulseaudio
            else
                echo -e "${RED}❌ Failed to install PulseAudio ❌${NC}"
                exit 1
            fi
        fi
    fi
}

# Display audio system status
show_audio_status() {
    echo -e "${BLUE}🎵 Audio System Status 🎵${NC}"
    echo "=================================="
    
    local current_audio=$(detect_audio_system)
    case "$current_audio" in
        "pipewire")
            echo -e "${GREEN}✅ PipeWire is running${NC}"
            if command -v pw-cli &> /dev/null; then
                echo -e "${BLUE}ℹ️ PipeWire info:${NC}"
                pw-cli info 2>/dev/null | head -5 || echo "Unable to get detailed info"
            fi
            ;;
        "pulseaudio")
            echo -e "${GREEN}✅ PulseAudio is running${NC}"
            if command -v pactl &> /dev/null; then
                echo -e "${BLUE}ℹ️ PulseAudio info:${NC}"
                pactl info 2>/dev/null | grep -E "(Server|Version)" || echo "Unable to get detailed info"
            fi
            ;;
        "none")
            echo -e "${RED}❌ No audio system detected${NC}"
            ;;
    esac
    echo "=================================="
}

# Enhanced function with both PulseAudio and PipeWire support
check_pulseaudio() {
    echo -e "${BLUE}🔍 Checking audio system... 🔍${NC}"
    check_audio_system
}

check_agnoster_dependencies() {
  color_echo "blue" "🔍 Checking agnoster theme dependencies..."
  
  local issues=0
  local distro=$(detect_distro)
  
  color_echo "blue" "Checking for Powerline fonts..."
  
  case "$(uname -s)" in
    Darwin*)
      if [ ! -f "$HOME/Library/Fonts/PowerlineSymbols.otf" ] && ! ls "$HOME/Library/Fonts"/*Nerd* >/dev/null 2>&1; then
        color_echo "yellow" "⚠️ Powerline/Nerd fonts not found in user fonts directory"
        issues=$((issues + 1))
      fi
      ;;
    Linux*)
      if [ ! -f "$HOME/.local/share/fonts/PowerlineSymbols.otf" ] && ! ls "$HOME/.local/share/fonts"/*Nerd* >/dev/null 2>&1; then
        if ! fc-list | grep -i powerline >/dev/null 2>&1 && ! fc-list | grep -i nerd >/dev/null 2>&1; then
          color_echo "yellow" "⚠️ Powerline/Nerd fonts not found"
          issues=$((issues + 1))
        fi
      fi
      ;;
  esac
  
  if [ -z "$TERM" ] || ! echo "$TERM" | grep -q "256color"; then
    color_echo "yellow" "⚠️ Terminal may not support 256 colors (TERM=$TERM)"
    color_echo "cyan" "💡 Try setting: export TERM=xterm-256color"
  fi
  
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

# Enhanced xhost check with Arch Linux support
check_xhost() {
    if ! command -v xhost &> /dev/null; then
        echo -e "${RED}❌ xhost is not installed on this system. ❌${NC}"
        
        local distro=$(detect_distro)
        case "$distro" in
            "arch")
                echo -e "${CYAN}🏛️ Installing xorg-xhost using pacman on Arch Linux... 📦${NC}"
                sudo pacman -Sy --noconfirm
                sudo pacman -S --noconfirm --needed xorg-xhost
                ;;
            "fedora")
                echo -e "${YELLOW}📦 Installing xorg-x11-server-utils using dnf... 📦${NC}"
                sudo dnf install -y xorg-x11-server-utils
                ;;
            "rhel"|"centos")
                if command -v dnf &> /dev/null; then
                    echo -e "${YELLOW}📦 Installing xorg-x11-server-utils using dnf... 📦${NC}"
                    sudo dnf install -y xorg-x11-server-utils
                else
                    echo -e "${YELLOW}📦 Installing xorg-x11-utils using yum... 📦${NC}"
                    sudo yum install -y xorg-x11-utils
                fi
                ;;
            "debian"|"ubuntu")
                echo -e "${YELLOW}📦 Installing x11-xserver-utils using apt... 📦${NC}"
                sudo apt update
                sudo apt install -y x11-xserver-utils
                ;;
            "opensuse")
                echo -e "${YELLOW}📦 Installing xorg-x11-server using zypper... 📦${NC}"
                sudo zypper install -y xorg-x11-server
                ;;
            *)
                echo -e "${RED}❌ Unsupported package manager. Please install xhost manually. ❌${NC}"
                exit 1
                ;;
        esac
        echo -e "${GREEN}✅ xhost installed successfully. ✅${NC}"
    else
        echo -e "${GREEN}✅ xhost is already installed. Moving on. ✅${NC}"
    fi
}

# Enhanced curl check with Arch Linux support
check_curl() {
    if ! command -v curl &> /dev/null; then
        echo -e "${RED}❌ curl is not installed on this system. ❌${NC}"
        if [ "$(uname -s)" == "Darwin" ]; then
            echo -e "${YELLOW}🍎 Attempting to install curl on macOS using Homebrew... 🍎${NC}"
            if ! command -v brew &> /dev/null; then
                echo -e "${RED}❌ Homebrew is not installed. Please install Homebrew first. ❌${NC}"
                echo "Visit https://brew.sh/ for installation instructions."
                exit 1
            fi
            brew install curl
        elif [ "$(uname -s)" == "Linux" ]; then
            local distro=$(detect_distro)
            case "$distro" in
                "arch")
                    echo -e "${CYAN}🏛️ Installing cURL using pacman on Arch Linux... 🐧${NC}"
                    sudo pacman -Sy --noconfirm
                    sudo pacman -S --noconfirm --needed curl
                    ;;
                "fedora")
                    echo -e "${YELLOW}🐧 Installing cURL using dnf... 🐧${NC}"
                    sudo dnf install -y curl
                    ;;
                "rhel"|"centos")
                    if command -v dnf &> /dev/null; then
                        sudo dnf install -y curl
                    else
                        sudo yum install -y curl
                    fi
                    ;;
                "debian"|"ubuntu")
                    echo -e "${YELLOW}🐧 Installing cURL using apt... 🐧${NC}"
                    sudo apt update
                    sudo apt install -y curl
                    ;;
                "opensuse")
                    echo -e "${YELLOW}🐧 Installing cURL using zypper... 🐧${NC}"
                    sudo zypper install -y curl
                    ;;
                *)
                    echo -e "${RED}❌ Unable to detect package manager. Please install cURL manually. ❌${NC}"
                    exit 1
                    ;;
            esac
        else
            echo -e "${RED}❌ Unsupported operating system. Please install cURL manually. ❌${NC}"
            exit 1
        fi
        echo -e "${GREEN}✅ curl installed successfully. ✅${NC}"
    else
        echo -e "${GREEN}✅ curl is already installed. Moving on. ✅${NC}"
    fi
}

# ═══════════════════════════════════════════════════════════════════════════════
# Container Engine Detection & Selection (Docker / Podman)
# ═══════════════════════════════════════════════════════════════════════════════

# Global state set by detect_container_engines
HAS_DOCKER=false
HAS_PODMAN=false
DOCKER_DAEMON_DOWN=false

detect_container_engines() {
    HAS_DOCKER=false
    HAS_PODMAN=false
    DOCKER_DAEMON_DOWN=false

    # Check Podman first (may provide a 'docker' shim via podman-docker)
    if command_exists podman; then
        HAS_PODMAN=true
    fi

    # Check Docker — must distinguish real Docker from podman-docker shim
    if command_exists docker; then
        local docker_ver
        docker_ver=$(docker --version 2>/dev/null || true)
        if echo "$docker_ver" | grep -qi "podman"; then
            # podman-docker shim, not real Docker
            HAS_PODMAN=true
        elif docker info >/dev/null 2>&1; then
            HAS_DOCKER=true
        else
            # Docker binary exists but daemon is not running
            HAS_DOCKER=true
            DOCKER_DAEMON_DOWN=true
        fi
    fi
}

# Main container engine check — replaces the old check_docker()
check_container_engine() {
    echo -e "${BLUE}🔍 Checking for container engines... 🔍${NC}"

    detect_container_engines

    # ── Both already installed ────────────────────────────────────────────
    if [ "$HAS_DOCKER" = true ] && [ "$HAS_PODMAN" = true ]; then
        echo -e "${GREEN}✅ Both Docker and Podman are installed. ✅${NC}"
        if [ "$DOCKER_DAEMON_DOWN" = true ]; then
            echo -e "${YELLOW}⚠️  Docker daemon is not running. Start it with: sudo systemctl start docker ⚠️${NC}"
        fi
        echo -e "${CYAN}ℹ️  RF-Swift auto-detects the engine at runtime.${NC}"
        echo -e "${CYAN}   Use 'rfswift --engine docker' or 'rfswift --engine podman' to override.${NC}"
        return 0
    fi

    # ── Only Docker installed ─────────────────────────────────────────────
    if [ "$HAS_DOCKER" = true ]; then
        echo -e "${GREEN}✅ Docker is already installed. ✅${NC}"
        if [ "$DOCKER_DAEMON_DOWN" = true ]; then
            echo -e "${YELLOW}⚠️  Docker daemon is not running. Start it with: sudo systemctl start docker ⚠️${NC}"
        fi
        if prompt_yes_no "Would you also like to install Podman (rootless containers)?" "n"; then
            install_podman
        fi
        return 0
    fi

    # ── Only Podman installed ─────────────────────────────────────────────
    if [ "$HAS_PODMAN" = true ]; then
        echo -e "${GREEN}✅ Podman is already installed. ✅${NC}"
        if prompt_yes_no "Would you also like to install Docker?" "n"; then
            install_docker_standard
        fi
        return 0
    fi

    # ── Neither installed ─────────────────────────────────────────────────
    echo -e "${YELLOW}⚠️  No container engine found. ⚠️${NC}"
    echo -e "${BLUE}ℹ️  RF-Swift requires Docker or Podman to run containers.${NC}"
    echo ""
    echo -e "${CYAN}📝 Which container engine would you like to install?${NC}"
    echo ""
    echo -e "${CYAN}   🐳 Docker  — Industry standard, requires daemon (root)${NC}"
    echo -e "${CYAN}              Best compatibility, large ecosystem${NC}"
    echo ""
    echo -e "${CYAN}   🦭 Podman  — Daemonless, rootless by default${NC}"
    echo -e "${CYAN}              Drop-in Docker replacement, no root needed${NC}"
    echo ""

    # Steam Deck special case
    if [ "$(uname -s)" == "Linux" ] && is_steam_deck; then
        echo -e "${MAGENTA}🎮 Steam Deck detected! Docker with Steam Deck optimizations is recommended. 🎮${NC}"
        if prompt_yes_no "Install Docker with Steam Deck optimizations?" "y"; then
            install_docker_steamdeck
            return $?
        fi
    fi

    local CHOICE
    CHOICE=$(prompt_choice "Select a container engine to install:" "Docker" "Podman" "Both" "Skip")

    case "$CHOICE" in
        1)
            install_docker_standard
            install_buildx
            install_docker_compose
            ;;
        2)
            install_podman
            ;;
        3)
            install_docker_standard
            install_buildx
            install_docker_compose
            install_podman
            ;;
        4)
            echo -e "${YELLOW}⚠️  Container engine installation skipped. ⚠️${NC}"
            echo -e "${YELLOW}   You will need Docker or Podman before using RF-Swift.${NC}"
            return 1
            ;;
    esac
}

# Legacy wrapper — scripts calling check_docker() still work
check_docker() {
    check_container_engine
}

check_docker_user_only() {
    check_container_engine
}

# ═══════════════════════════════════════════════════════════════════════════════
# Podman Installation
# ═══════════════════════════════════════════════════════════════════════════════

install_podman() {
    echo -e "${BLUE}🦭 Installing Podman... 🦭${NC}"

    case "$(uname -s)" in
        Darwin*)
            install_podman_macos
            ;;
        Linux*)
            install_podman_linux
            ;;
        *)
            echo -e "${RED}🚨 Unsupported OS: $(uname -s) 🚨${NC}"
            return 1
            ;;
    esac
}

install_podman_macos() {
    if command_exists brew; then
        echo -e "${BLUE}🍏 Installing Podman via Homebrew... 🍏${NC}"
        brew install podman

        echo -e "${BLUE}🚀 Initialising Podman machine... 🚀${NC}"
        podman machine init 2>/dev/null || true
        podman machine start 2>/dev/null || true

        if podman info >/dev/null 2>&1; then
            echo -e "${GREEN}🎉 Podman is up and running on macOS! 🎉${NC}"
        else
            echo -e "${YELLOW}⚠️  Podman installed. Run 'podman machine start' to start the VM. ⚠️${NC}"
        fi
    else
        echo -e "${RED}🚨 Homebrew is not installed! Please install Homebrew first: 🚨${NC}"
        echo -e "${YELLOW}/bin/bash -c \"\$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)\"${NC}"
        return 1
    fi
}

install_podman_linux() {
    local distro=$(detect_distro)

    echo -e "${YELLOW}⚠️ This will require sudo privileges to install Podman. ⚠️${NC}"

    case "$distro" in
        "arch")
            echo -e "${CYAN}🏛️ Installing Podman using pacman... 📦${NC}"
            sudo pacman -Sy --noconfirm
            sudo pacman -S --noconfirm --needed podman podman-compose slirp4netns fuse-overlayfs crun
            ;;
        "fedora")
            echo -e "${BLUE}📦 Installing Podman using dnf... 📦${NC}"
            sudo dnf install -y podman podman-compose slirp4netns fuse-overlayfs
            ;;
        "rhel"|"centos")
            echo -e "${BLUE}📦 Installing Podman... 📦${NC}"
            if command -v dnf &> /dev/null; then
                sudo dnf install -y podman podman-compose slirp4netns fuse-overlayfs
            else
                sudo yum install -y podman slirp4netns fuse-overlayfs
            fi
            ;;
        "debian"|"ubuntu")
            echo -e "${BLUE}📦 Installing Podman using apt... 📦${NC}"
            sudo apt update
            sudo apt install -y podman podman-compose slirp4netns fuse-overlayfs uidmap
            ;;
        "opensuse")
            echo -e "${BLUE}📦 Installing Podman using zypper... 📦${NC}"
            sudo zypper install -y podman podman-compose slirp4netns fuse-overlayfs
            ;;
        "alpine")
            echo -e "${BLUE}📦 Installing Podman using apk... 📦${NC}"
            sudo apk add podman podman-compose fuse-overlayfs slirp4netns
            ;;
        *)
            echo -e "${RED}❌ Unsupported distribution: $distro ❌${NC}"
            echo -e "${YELLOW}Please install Podman manually: https://podman.io/docs/installation${NC}"
            return 1
            ;;
    esac

    # Configure rootless Podman
    configure_podman_rootless

    echo -e "${GREEN}🎉 Podman installed successfully! 🎉${NC}"
    echo -e "${CYAN}💡 Tip: Podman is a drop-in replacement for Docker.${NC}"
    echo -e "${CYAN}   RF-Swift will auto-detect Podman at runtime.${NC}"
    return 0
}

# Configure rootless Podman (subuid/subgid, lingering, etc.)
configure_podman_rootless() {
    local current_user
    current_user=$(whoami)

    echo -e "${BLUE}🔧 Configuring rootless Podman for '$current_user'... 🔧${NC}"

    # Ensure subuid/subgid ranges
    if [ -f /etc/subuid ]; then
        if ! grep -q "^${current_user}:" /etc/subuid 2>/dev/null; then
            echo -e "${BLUE}   Adding subordinate UID range...${NC}"
            sudo usermod --add-subuids 100000-165535 "$current_user" 2>/dev/null || true
        fi
    fi

    if [ -f /etc/subgid ]; then
        if ! grep -q "^${current_user}:" /etc/subgid 2>/dev/null; then
            echo -e "${BLUE}   Adding subordinate GID range...${NC}"
            sudo usermod --add-subgids 100000-165535 "$current_user" 2>/dev/null || true
        fi
    fi

    # Enable lingering so rootless containers survive logout
    if command_exists loginctl; then
        echo -e "${BLUE}   Enabling login lingering...${NC}"
        sudo loginctl enable-linger "$current_user" 2>/dev/null || true
    fi

    # Enable Podman socket for compatibility with Docker-expecting tools
    if command_exists systemctl; then
        echo -e "${BLUE}   Enabling Podman socket...${NC}"
        systemctl --user enable podman.socket 2>/dev/null || true
        systemctl --user start podman.socket 2>/dev/null || true
    fi

    echo -e "${GREEN}   ✅ Rootless Podman configured ✅${NC}"
}

# ═══════════════════════════════════════════════════════════════════════════════
# Docker Installation
# ═══════════════════════════════════════════════════════════════════════════════

# Add current user to the docker group
add_user_to_docker_group() {
    if command_exists sudo && command_exists groups; then
        current_user=$(whoami)
        if ! groups "$current_user" 2>/dev/null | grep -q docker; then
            echo -e "${BLUE}🔧 Adding '$current_user' to Docker group... 🔧${NC}"
            sudo usermod -aG docker "$current_user"
            echo -e "${YELLOW}⚡ You may need to log out and log back in for Docker group changes to take effect. ⚡${NC}"
        fi
    fi
}

# Enhanced Docker installation with Arch Linux support
install_docker_standard() {
    arch=$(uname -m)
    os=$(uname -s)

    echo -e "${BLUE}🐳 Installing Docker... 🐳${NC}"
    
    if [ "$os" == "Darwin" ]; then
        if ! command -v brew &> /dev/null; then
            echo -e "${RED}❌ Homebrew is not installed. Please install Homebrew first. ❌${NC}"
            echo "Visit https://brew.sh/ for installation instructions."
            exit 1
        fi
        echo -e "${YELLOW}🍎 Installing Docker using Homebrew... 🍎${NC}"
        brew install --cask docker
        echo -e "${GREEN}✅ Docker installed successfully on macOS. ✅${NC}"
        echo -e "${YELLOW}ℹ️ Please launch Docker from Applications to start the Docker daemon. ℹ️${NC}"
    elif [ "$os" == "Linux" ]; then
        echo -e "${YELLOW}🐧 Installing Docker on your Linux machine... 🐧${NC}"
        
        local distro=$(detect_distro)
        if [ "$distro" = "arch" ]; then
            echo -e "${CYAN}🏛️ Arch Linux detected - using pacman for Docker installation${NC}"
            
            sudo pacman -Sy --noconfirm
            sudo pacman -S --noconfirm --needed docker docker-compose
            
            if command -v systemctl &> /dev/null; then
                echo -e "${BLUE}🚀 Enabling and starting Docker service... 🚀${NC}"
                sudo systemctl enable docker
                sudo systemctl start docker
            fi
            
            add_user_to_docker_group
            
            echo -e "${GREEN}🎉 Docker installed successfully using pacman! 🎉${NC}"
            
            install_buildx
            install_docker_compose
            return 0
        else
            echo -e "${YELLOW}⚠️ This will require sudo privileges to install Docker. ⚠️${NC}"
            
            echo -e "${BLUE}Using Docker's official installation script... 🐧${NC}"
            
            if command -v curl &> /dev/null; then
                curl -fsSL "https://get.docker.com/" | sudo sh
            elif command -v wget &> /dev/null; then
                wget -qO- "https://get.docker.com/" | sudo sh
            else
                echo -e "${RED}❌ Missing curl/wget. Please install one of them. ❌${NC}"
                exit 1
            fi

            add_user_to_docker_group
            
            if command -v systemctl &> /dev/null; then
                echo -e "${BLUE}🚀 Starting Docker service... 🚀${NC}"
                sudo systemctl start docker
                sudo systemctl enable docker
            fi

            echo -e "${GREEN}🎉 Docker is now installed and running! 🎉${NC}"
            
            install_buildx
            install_docker_compose
        fi
    else
        echo -e "${RED}❌ Unsupported operating system: $os ❌${NC}"
        exit 1
    fi
}

# Enhanced Steam Deck Docker installation
install_docker_steamdeck() {
    echo -e "${MAGENTA}🎮 Installing Docker on Steam Deck using Arch Linux methods... 🎮${NC}"
    
    echo -e "${YELLOW}[+] 🎮 Disabling read-only mode on Steam Deck 🎮${NC}"
    sudo steamos-readonly disable

    echo -e "${YELLOW}[+] 🔑 Initializing pacman keyring 🔑${NC}"
    sudo pacman-key --init
    sudo pacman-key --populate archlinux
    sudo pacman-key --populate holo

    echo -e "${YELLOW}[+] 🐳 Installing Docker using pacman 🐳${NC}"
    sudo pacman -Syu --noconfirm docker docker-compose

    install_docker_compose_steamdeck

    add_user_to_docker_group
    
    if command -v systemctl &> /dev/null; then
        echo -e "${BLUE}🚀 Starting Docker service... 🚀${NC}"
        sudo systemctl start docker
        sudo systemctl enable docker
    fi

    echo -e "${GREEN}✅ Docker and Docker Compose installed successfully on Steam Deck using Arch methods! ✅${NC}"
}

install_docker_compose_steamdeck() {
    echo -e "${YELLOW}[+] 🧩 Installing Docker Compose v2 plugin for Steam Deck 🧩${NC}"
    DOCKER_CONFIG=${DOCKER_CONFIG:-$HOME/.docker}
    mkdir -p $DOCKER_CONFIG/cli-plugins
    
    curl -SL https://github.com/docker/compose/releases/download/v5.0.2/docker-compose-linux-x86_64 -o $DOCKER_CONFIG/cli-plugins/docker-compose
    chmod +x $DOCKER_CONFIG/cli-plugins/docker-compose

    echo -e "${GREEN}✅ Docker Compose v2 installed successfully for Steam Deck ✅${NC}"
}

install_buildx() {
    arch=$(uname -m)
    os=$(uname -s | tr '[:upper:]' '[:lower:]')
    version="v0.31.0"

    case "$arch" in
        x86_64|amd64)  arch="amd64";;
        arm64|aarch64) arch="arm64";;
        riscv64)       arch="riscv64";;
        *)
            printf "${RED}❌ Unsupported architecture: \"%s\" -> Unable to install Buildx ❌${NC}\n" "$arch" >&2; exit 2;;
    esac

    if ! sudo docker buildx version &> /dev/null; then
        echo -e "${YELLOW}[+] 🏗️ Installing Docker Buildx 🏗️${NC}"

        if [ "$os" = "linux" ]; then
            sudo docker run --privileged --rm tonistiigi/binfmt --install all
        fi

        mkdir -p ~/.docker/cli-plugins/

        buildx_url="https://github.com/docker/buildx/releases/download/${version}/buildx-${version}.${os}-${arch}"

        echo -e "${YELLOW}[+] 📥 Downloading Buildx from ${buildx_url} 📥${NC}"
        sudo curl -sSL "$buildx_url" -o "/usr/local/lib/docker/cli-plugins/docker-buildx"
        sudo chmod +x "/usr/local/lib/docker/cli-plugins/docker-buildx"

        echo -e "${GREEN}✅ Docker Buildx installed successfully. ✅${NC}"
    else
        echo -e "${GREEN}✅ Docker Buildx is already installed. Moving on. ✅${NC}"
    fi
}

install_docker_compose() {
    arch=$(uname -m)
    os=$(uname -s | tr '[:upper:]' '[:lower:]')
    version="v5.0.2"

    case "$arch" in
        x86_64|amd64)  arch="x86_64";;
        arm64|aarch64) arch="aarch64";;
        riscv64)       arch="riscv64";;
        *)
            printf "${RED}❌ Unsupported architecture: \"%s\" -> Unable to install Docker Compose ❌${NC}\n" "$arch" >&2; exit 2;;
    esac

    if ! sudo docker compose version &> /dev/null; then
        echo -e "${YELLOW}[+] 🧩 Installing Docker Compose v2 🧩${NC}"

        compose_url="https://github.com/docker/compose/releases/download/${version}/docker-compose-${os}-${arch}"

        DOCKER_CONFIG=${DOCKER_CONFIG:-$HOME/.docker}
        mkdir -p $DOCKER_CONFIG/cli-plugins

        echo -e "${YELLOW}[+] 📥 Downloading Docker Compose from ${compose_url} 📥${NC}"
        sudo curl -sSL "$compose_url" -o "/usr/local/lib/docker/cli-plugins/docker-compose"
        sudo chmod +x "/usr/local/lib/docker/cli-plugins/docker-compose"

        echo -e "${GREEN}✅ Docker Compose v2 installed successfully. ✅${NC}"
    else
        echo -e "${GREEN}✅ Docker Compose v2 is already installed. Moving on. ✅${NC}"
    fi
}

# ═══════════════════════════════════════════════════════════════════════════════
# Go, build, and image management
# ═══════════════════════════════════════════════════════════════════════════════

# Enhanced Go installation with Arch Linux support
install_go() {
    if command -v go &> /dev/null; then
        echo -e "${GREEN}✅ golang is already installed and in PATH. Moving on. ✅${NC}"
        return 0
    fi

    if [ -x "/usr/local/go/bin/go" ]; then
        echo -e "${GREEN}✅ golang is already installed in /usr/local/go/bin. Moving on. ✅${NC}"
        export PATH=$PATH:/usr/local/go/bin
        return 0
    fi

    local distro=$(detect_distro)
    if [ "$distro" = "arch" ]; then
        echo -e "${CYAN}🏛️ Arch Linux detected. Installing Go using pacman... 📦${NC}"
        sudo pacman -Sy --noconfirm
        sudo pacman -S --noconfirm --needed go
        echo -e "${GREEN}✅ Go installed successfully using pacman on Arch Linux. ✅${NC}"
        return 0
    fi

    [ -d thirdparty ] || mkdir thirdparty
    cd thirdparty
    arch=$(uname -m)
    os=$(uname -s | tr '[:upper:]' '[:lower:]')
    prog=""
    version="1.25.6"

    case "$arch" in
        x86_64|amd64)  arch="amd64";;
        i?86)          arch="386";;
        arm64|aarch64) arch="arm64";;
        riscv64)       arch="riscv64";;
        *)
            printf "${RED}❌ Unsupported architecture: \"%s\" -> Unable to install Go ❌${NC}\n" "$arch" >&2; exit 2;;
    esac

    case "$os" in
        linux|darwin)
            prog="go${version}.${os}-${arch}.tar.gz";;
        *)
            printf "${RED}❌ Unsupported OS: \"%s\" -> Unable to install Go ❌${NC}\n" "$os" >&2; exit 2;;
    esac

    echo -e "${YELLOW}[+] 📥 Downloading Go from https://go.dev/dl/${prog} 📥${NC}"
    wget "https://go.dev/dl/${prog}"
    sudo rm -rf /usr/local/go && sudo tar -C /usr/local -xzf $prog
    export PATH=$PATH:/usr/local/go/bin
    cd ..
    rm -rf thirdparty
    echo -e "${GREEN}✅ Go installed successfully. ✅${NC}"
}

building_rfswift() {
    cd go/rfswift/
    echo -e "${YELLOW}🔨 Building RF Swift Go Project... 🔨${NC}"
    go build .
    mv rfswift ../..
    cd ../..
    echo -e "${GREEN}✅ RF Swift Go Project built successfully. ✅${NC}"
}

build_docker_image() {
    echo -e "${YELLOW}🏗️ Select the architecture(s) to build for: 🏗️${NC}"
    echo "1) amd64 💻"
    echo "2) arm64/v8 📱"
    echo "3) riscv64 🔬"
    read -p "Choose an option (1, 2, or 3): " arch_option

    case "$arch_option" in
        1) PLATFORM="linux/amd64" ;;
        2) PLATFORM="linux/arm64/v8" ;;
        3) PLATFORM="linux/riscv64" ;;
        *)
            echo -e "${RED}❌ Invalid option. Exiting. ❌${NC}"
            exit 1
            ;;
    esac

    DEFAULT_IMAGE="myrfswift:latest"
    DEFAULT_DOCKERFILE="Dockerfile"

    read -p "Enter you ressources directory (where configs, and scripts are placed): " ressourcesdir
    read -p "Enter image tag value (default: $DEFAULT_IMAGE): " imagename
    read -p "Enter value for Dockerfile to use (default: $DEFAULT_DOCKERFILE): " dockerfile

    imagename=${imagename:-$DEFAULT_IMAGE}
    dockerfile=${dockerfile:-$DEFAULT_DOCKERFILE}

    echo -e "${YELLOW}[+] 🐳 Building the Docker container for $PLATFORM 🐳${NC}"
    sudo docker buildx build --platform $PLATFORM -t $imagename -f $dockerfile ressourcesdir
}

pull_docker_image() {
    sudo ./rfswift images remote
    read -p "Enter the image tag to pull (default: penthertz/rfswift:corebuild): " pull_image
    pull_image=${pull_image:-penthertz/rfswift:corebuild}

    echo -e "${YELLOW}[+] 📥 Pulling the Docker image 📥${NC}"
    sudo docker pull $pull_image
}

# ═══════════════════════════════════════════════════════════════════════════════
# Binary installation and alias management
# ═══════════════════════════════════════════════════════════════════════════════

install_binary_alias() {
    echo -e "${YELLOW}📦 Where would you like to install the rfswift binary? 📦${NC}"
    echo -e "1) /usr/local/bin (requires sudo privileges) 🔐"
    echo -e "2) $HOME/.rfswift/bin/ (user-only installation) 👤"
    read -p "Choose an option (1 or 2): " install_location

    if [ "$install_location" == "1" ]; then
        INSTALL_DIR="/usr/local/bin"
        BINARY_PATH="$INSTALL_DIR/rfswift"
        SUDO_CMD="sudo"
        echo -e "${YELLOW}[+] 💻 Installing to system location ($INSTALL_DIR) 🌐${NC}"
    else
        INSTALL_DIR="$HOME/.rfswift/bin"
        BINARY_PATH="$INSTALL_DIR/rfswift"
        SUDO_CMD=""
        echo -e "${YELLOW}[+] 🏠 Installing to user location ($INSTALL_DIR) 👤${NC}"
        mkdir -p "$INSTALL_DIR"
    fi

    SOURCE_BINARY=$(pwd)/rfswift
    if [ -f "$SOURCE_BINARY" ]; then
        echo -e "${YELLOW}[+] 📋 Copying binary to $INSTALL_DIR 📋${NC}"
        $SUDO_CMD cp "$SOURCE_BINARY" "$BINARY_PATH"
        $SUDO_CMD chmod +x "$BINARY_PATH"
    else
        echo -e "${RED}❌ Binary not found at $SOURCE_BINARY. Make sure the binary is built correctly. ❌${NC}"
        exit 1
    fi

    read -p "Do you want to create an alias for the binary? (yes/no): " create_alias
    if [ "$create_alias" == "yes" ]; then
        read -p "Enter the alias name for the binary (default: rfswift): " alias_name
        alias_name=${alias_name:-rfswift}
        
        if [ -n "${SUDO_USER-}" ]; then
            CURRENT_USER="$SUDO_USER"
            HOME_DIR=$(eval echo "~$SUDO_USER")
        else
            CURRENT_USER=$(whoami)
            HOME_DIR=$HOME
        fi
        
        SHELL_NAME=$(basename "$SHELL")
        
        case "$SHELL_NAME" in
            bash)
                if [[ "$OSTYPE" == "darwin"* ]]; then
                    ALIAS_FILE="$HOME_DIR/.bash_profile"
                else
                    ALIAS_FILE="$HOME_DIR/.bashrc"
                fi
                ;;
            zsh)  ALIAS_FILE="$HOME_DIR/.zshrc" ;;
            fish)
                ALIAS_FILE="$HOME_DIR/.config/fish/config.fish"
                mkdir -p "$(dirname "$ALIAS_FILE")"
                ;;
            *)    ALIAS_FILE="$HOME_DIR/.${SHELL_NAME}rc" ;;
        esac
        
        if [[ ! -f "$ALIAS_FILE" ]]; then
            echo -e "${YELLOW}[+] 📝 Alias file $ALIAS_FILE does not exist. Creating it... 🆕${NC}"
            touch "$ALIAS_FILE"
        fi
        
        ALIAS_EXISTS=false
        ALIAS_NEEDS_UPDATE=false
        if [ -f "$ALIAS_FILE" ]; then
            if [ "$SHELL_NAME" = "fish" ]; then
                EXISTING_ALIAS=$(grep "^alias $alias_name " "$ALIAS_FILE" 2>/dev/null || true)
            else
                EXISTING_ALIAS=$(grep "^alias $alias_name=" "$ALIAS_FILE" 2>/dev/null || true)
            fi
            
            if [ -n "$EXISTING_ALIAS" ]; then
                ALIAS_EXISTS=true
                if [ "$SHELL_NAME" = "fish" ]; then
                    EXISTING_PATH=$(echo "$EXISTING_ALIAS" | sed -E "s/^alias $alias_name '?([^']*)'?$/\1/")
                else
                    EXISTING_PATH=$(echo "$EXISTING_ALIAS" | sed -E "s/^alias $alias_name='?([^']*)'?$/\1/")
                fi
                
                if [ "$EXISTING_PATH" != "$BINARY_PATH" ]; then
                    ALIAS_NEEDS_UPDATE=true
                    echo -e "${YELLOW}[!] ⚠️ Alias '$alias_name' already exists but points to a different path: ⚠️${NC}"
                    echo -e "${YELLOW}    Current: $EXISTING_PATH${NC}"
                    echo -e "${YELLOW}    New: $BINARY_PATH${NC}"
                    read -p "Do you want to update the alias to the new path? (yes/no): " update_alias
                    if [ "$update_alias" == "yes" ]; then
                        if [ "$SHELL_NAME" = "fish" ]; then
                            sed -i.bak "/^alias $alias_name /d" "$ALIAS_FILE"
                            echo "alias $alias_name '$BINARY_PATH'" >> "$ALIAS_FILE"
                        else
                            sed -i.bak "/^alias $alias_name=/d" "$ALIAS_FILE"
                            echo "alias $alias_name='$BINARY_PATH'" >> "$ALIAS_FILE"
                        fi
                        echo -e "${GREEN}✅ Alias '$alias_name' updated successfully. ✅${NC}"
                    else
                        echo -e "${GREEN}👍 Keeping existing alias configuration. 👍${NC}"
                    fi
                else
                    echo -e "${GREEN}✅ Alias '$alias_name' already exists with the correct path. ✅${NC}"
                fi
            fi
        fi
        
        if [ "$ALIAS_EXISTS" = false ] && [ "$ALIAS_NEEDS_UPDATE" = false ]; then
            if [ "$SHELL_NAME" = "fish" ]; then
                echo "alias $alias_name '$BINARY_PATH'" >> "$ALIAS_FILE"
            else
                echo "alias $alias_name='$BINARY_PATH'" >> "$ALIAS_FILE"
            fi
            echo -e "${GREEN}✅ Alias '$alias_name' installed successfully! ✅${NC}"
        fi
        
        case "$SHELL_NAME" in
            "zsh")  echo -e "${YELLOW}🔄 Zsh configuration updated. Please restart your terminal or run 'exec zsh' to apply the changes. 🔄${NC}" ;;
            "bash") echo -e "${YELLOW}🔄 Bash configuration updated. Please run 'source $ALIAS_FILE' to apply the changes. 🔄${NC}" ;;
            "fish") echo -e "${YELLOW}🔄 Fish configuration updated. Please restart your terminal or run 'source $ALIAS_FILE' to apply the changes. 🔄${NC}" ;;
            *)      echo -e "${YELLOW}🔄 Please restart your terminal or source ${ALIAS_FILE} manually to apply the alias. 🔄${NC}" ;;
        esac
        
        if [ "$install_location" == "2" ]; then
            if [[ ":$PATH:" != *":$INSTALL_DIR:"* ]]; then
                echo -e "${YELLOW}[+] 🔀 Adding $INSTALL_DIR to your PATH 🔀${NC}"
                if [ "$SHELL_NAME" = "fish" ]; then
                    echo "set -gx PATH \$PATH $INSTALL_DIR" >> "$ALIAS_FILE"
                else
                    echo "export PATH=\$PATH:$INSTALL_DIR" >> "$ALIAS_FILE"
                fi
                echo -e "${GREEN}✅ PATH updated successfully. Please restart your terminal or source your shell config file. ✅${NC}"
            fi
        fi
    else
        echo -e "${GREEN}⏭️ Skipping alias creation. ⏭️${NC}"
        
        if [ "$install_location" == "2" ]; then
            SHELL_NAME=$(basename "$SHELL")
            case "$SHELL_NAME" in
                "bash")
                    if [[ "$OSTYPE" == "darwin"* ]]; then
                        [ -f "$HOME/.bash_profile" ] && RC_FILE="$HOME/.bash_profile" || RC_FILE="$HOME/.profile"
                    else
                        [ -f "$HOME/.bashrc" ] && RC_FILE="$HOME/.bashrc" || RC_FILE="$HOME/.profile"
                    fi
                    ;;
                "zsh")  RC_FILE="$HOME/.zshrc" ;;
                "fish")
                    RC_FILE="$HOME/.config/fish/config.fish"
                    mkdir -p "$(dirname "$RC_FILE")"
                    ;;
                *)      RC_FILE="$HOME/.profile" ;;
            esac
            
            if [[ ":$PATH:" != *":$INSTALL_DIR:"* ]]; then
                echo -e "${YELLOW}[+] 🔀 Would you like to add $INSTALL_DIR to your PATH? (yes/no) 🔀${NC}"
                read -p "Choose an option: " add_to_path
                if [ "$add_to_path" == "yes" ]; then
                    if [ "$SHELL_NAME" = "fish" ]; then
                        echo "set -gx PATH \$PATH $INSTALL_DIR" >> "$RC_FILE"
                    else
                        echo "export PATH=\$PATH:$INSTALL_DIR" >> "$RC_FILE"
                    fi
                    echo -e "${GREEN}✅ PATH updated in $RC_FILE. Please restart your terminal or run 'source $RC_FILE'. ✅${NC}"
                else
                    echo -e "${YELLOW}ℹ️ Note: You'll need to run $BINARY_PATH using its full path. ℹ️${NC}"
                fi
            fi
        fi
    fi
    
    echo -e "${GREEN}🎉 Installation complete! You can now use rfswift. 🎉${NC}"
    
    if is_steam_deck; then
        echo -e "${YELLOW}[+] 🔒 Re-enabling read-only mode on Steam Deck 🔒${NC}"
        sudo steamos-readonly enable
    fi
}

# ═══════════════════════════════════════════════════════════════════════════════
# Config file check
# ═══════════════════════════════════════════════════════════════════════════════

check_config_file() {
    if [[ "$OSTYPE" == "darwin"* ]]; then
        CONFIG_DIR="$HOME/Library/Application Support/rfswift"
    else
        CONFIG_DIR="$HOME/.config/rfswift"
    fi
    CONFIG_FILE="$CONFIG_DIR/config.ini"
    
    echo -e "${YELLOW}🔍 Checking configuration file at: $CONFIG_FILE 🔍${NC}"
    
    if [ ! -f "$CONFIG_FILE" ]; then
        echo -e "${YELLOW}📝 Config file not found at $CONFIG_FILE 📝${NC}"
        echo -e "${GREEN}✨ A new config file will be created on first run ;) ✨${NC}"
        return 0
    fi
    
    GENERAL_KEYS="imagename repotag"
    CONTAINER_KEYS="shell bindings network exposedports portbindings x11forward xdisplay extrahost extraenv devices privileged caps seccomp cgroups"
    AUDIO_KEYS="pulse_server"
    
    missing_fields=0
    current_section=""
    
    echo -e "${YELLOW}🔎 Scanning config file for keys... 🔎${NC}"
    
    while IFS= read -r line || [ -n "$line" ]; do
        line=$(echo "$line" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')
        
        if [[ -z "$line" || "$line" == \#* ]]; then
            continue
        fi
        
        if [[ "$line" =~ ^\[([a-zA-Z0-9_]+)\]$ ]]; then
            current_section="${BASH_REMATCH[1]}"
            echo -e "${YELLOW}📂 Found section: [$current_section] 📂${NC}"
            continue
        fi
        
        if [[ "$line" =~ ^([a-zA-Z0-9_]+)[[:space:]]*= ]]; then
            key="${BASH_REMATCH[1]}"
            echo -e "${GREEN}🔑 Found key: $key in section [$current_section] 🔑${NC}"
            
            if [[ "$current_section" == "general" ]]; then
                GENERAL_KEYS=$(echo "$GENERAL_KEYS" | sed -E "s/(^| )$key( |$)/ /g" | tr -s ' ' | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')
            elif [[ "$current_section" == "container" ]]; then
                CONTAINER_KEYS=$(echo "$CONTAINER_KEYS" | sed -E "s/(^| )$key( |$)/ /g" | tr -s ' ' | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')
            elif [[ "$current_section" == "audio" ]]; then
                AUDIO_KEYS=$(echo "$AUDIO_KEYS" | sed -E "s/(^| )$key( |$)/ /g" | tr -s ' ' | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')
            fi
        fi
    done < "$CONFIG_FILE"
    
    echo -e "${YELLOW}📋 Remaining required keys in [general]: ${GENERAL_KEYS} 📋${NC}"
    echo -e "${YELLOW}📋 Remaining required keys in [container]: ${CONTAINER_KEYS} 📋${NC}"
    echo -e "${YELLOW}📋 Remaining required keys in [audio]: ${AUDIO_KEYS} 📋${NC}"
    
    if [[ -n "$GENERAL_KEYS" ]]; then
        echo -e "${RED}❗ Missing keys in [general] section: ❗${NC}"
        for field in $GENERAL_KEYS; do
            echo -e "  - ${YELLOW}🔴 $field 🔴${NC}"
            missing_fields=$((missing_fields + 1))
        done
    fi
    
    if [[ -n "$CONTAINER_KEYS" ]]; then
        echo -e "${RED}❗ Missing keys in [container] section: ❗${NC}"
        for field in $CONTAINER_KEYS; do
            echo -e "  - ${YELLOW}🔴 $field 🔴${NC}"
            missing_fields=$((missing_fields + 1))
        done
    fi
    
    if [[ -n "$AUDIO_KEYS" ]]; then
        echo -e "${RED}❗ Missing keys in [audio] section: ❗${NC}"
        for field in $AUDIO_KEYS; do
            echo -e "  - ${YELLOW}🔴 $field 🔴${NC}"
            missing_fields=$((missing_fields + 1))
        done
    fi
    
    if [ $missing_fields -gt 0 ]; then
        echo -e "${RED}⚠️ WARNING: $missing_fields required keys are missing from your config file. ⚠️${NC}"
        echo -e "${YELLOW}💡 You should either: 💡${NC}"
        echo -e "  1. 📝 Add the missing keys to $CONFIG_FILE (values can be empty) 📝"
        echo -e "  2. 🔄 Rename or delete $CONFIG_FILE to generate a fresh config with defaults 🔄"
        return 1
    else
        echo -e "${GREEN}✅ Config file validation successful! All required keys present. ✅${NC}"
        return 0
    fi
}

# ═══════════════════════════════════════════════════════════════════════════════
# Logo, system info
# ═══════════════════════════════════════════════════════════════════════════════

display_rainbow_logo_animated() {
    colors=(
        '\033[1;31m'
        '\033[1;33m'
        '\033[1;32m'
        '\033[1;36m'
        '\033[1;34m'
        '\033[1;35m'
    )
    NC='\033[0m'
    
    logo=(
        "   888~-_   888~~        ,d88~~\\                ,e,   88~\\   d8   "
        "   888   \\  888___       8888    Y88b    e    /  \"  *888*_ *d88*_ "
        "   888    | 888          'Y88b    Y88b  d8b  /  888  888    888   "
        "   888   /  888           'Y88b,   Y888/Y88b/   888  888    888   "
        "   888_-~   888             8888    Y8/  Y8/    888  888    888   "
        "   888 ~-_  888          \\__88P'     Y    Y     888  888    \"88_/"
    )
    
    clear
    
    for i in {0..5}; do
        echo -e "${colors[$i]}${logo[$i]}${NC}"
        sleep 0.1
    done
    
    sleep 0.5
    
    if [ -t 1 ]; then
        for cycle in {1..3}; do
            for i in {1..6}; do
                echo -en "\033[1A"
            done
            
            for i in {0..5}; do
                color_index=$(( (i + cycle) % 6 ))
                echo -e "${colors[$color_index]}${logo[$i]}${NC}"
            done
            
            sleep 0.3
        done
    fi
    
    echo -e "\n${colors[5]}🔥 RF Swift by @Penthertz - Radio Frequency Swiss Army Knife 🔥${NC}"
    echo ""
    sleep 0.5
}

show_system_info() {
    echo -e "${BLUE}🖥️ System Information: 🖥️${NC}"
    echo -e "${BLUE}   OS: $(uname -s) 🖥️${NC}"
    echo -e "${BLUE}   Architecture: $(uname -m) 🏗️${NC}"
    
    local distro=$(detect_distro)
    local pkg_mgr=$(get_package_manager)
    
    echo -e "${BLUE}   Distribution: $distro 🐧${NC}"
    echo -e "${BLUE}   Package Manager: $pkg_mgr 📦${NC}"
    
    if is_steam_deck; then
        echo -e "${MAGENTA}   🎮 Steam Deck detected! 🎮${NC}"
    fi
    
    if is_arch_linux; then
        echo -e "${CYAN}   🏛️ Arch Linux system detected! 🏛️${NC}"
    fi
    
    # Show container engine status
    detect_container_engines
    if [ "$HAS_DOCKER" = true ] && [ "$HAS_PODMAN" = true ]; then
        echo -e "${BLUE}   Container engines: 🐳 Docker + 🦭 Podman${NC}"
    elif [ "$HAS_DOCKER" = true ]; then
        echo -e "${BLUE}   Container engine: 🐳 Docker${NC}"
    elif [ "$HAS_PODMAN" = true ]; then
        echo -e "${BLUE}   Container engine: 🦭 Podman${NC}"
    else
        echo -e "${YELLOW}   Container engine: ⚠️ None detected${NC}"
    fi
    
    echo ""
}

# Main execution section — if this script is run directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    display_rainbow_logo_animated
    echo -e "${BLUE}🎵 RF Swift Enhanced Installer with Arch Linux Support 🎵${NC}"
    echo ""
    show_system_info
    show_audio_status
fi