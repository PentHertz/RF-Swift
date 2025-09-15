#!/bin/sh
# RF-Swift Enhanced Installer Script
# Usage: curl -fsSL "https://get.rfswift.io/" | sh
# or: wget -qO- "https://get.rfswift.io/" | sh

set -e

# Configuration
GITHUB_REPO="PentHertz/RF-Swift"

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
        sudo pacman -S --noconfirm --needed pipewire-audio pipewire-media-session || true
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
        return install_pulseaudio "$distro"
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
  
  # Skip audio setup on macOS
  case "$(uname -s)" in
    Darwin*)
      color_echo "yellow" "🍎 macOS detected. Audio system management is handled by the system"
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
  
  # Check current audio system status
  case "$current_audio" in
    "pipewire")
      color_echo "green" "✅ PipeWire is already running"
      return 0
      ;;
    "pulseaudio")
      color_echo "green" "✅ PulseAudio is already running"
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
  
  return 0
}

# Display audio system status
show_audio_status() {
  color_echo "blue" "🎵 Audio System Status"
  echo "=================================="
  
  local current_audio=$(detect_audio_system)
  case "$current_audio" in
    "pipewire")
      color_echo "green" "✅ PipeWire is running"
      if command_exists pw-cli; then
        color_echo "blue" "ℹ️ PipeWire info:"
        pw-cli info 2>/dev/null | head -5 || echo "Unable to get detailed info"
      fi
      ;;
    "pulseaudio")
      color_echo "green" "✅ PulseAudio is running"
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

# Function to get the current user even when run with sudo
get_real_user() {
  if [ -n "$SUDO_USER" ]; then
    echo "$SUDO_USER"
  else
    whoami
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

# Function to create an alias for RF-Swift in the user's shell configuration
create_alias() {
  local bin_path="$1"
  color_echo "blue" "🔗 Setting up an alias for RF-Swift..."
  
  # Get the real user even when run with sudo
  REAL_USER=$(get_real_user)
  USER_HOME=$(eval echo ~${REAL_USER})
  
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
      # Check for .bash_profile first (macOS preference), then .bashrc (Linux preference)
      if [ -f "${USER_HOME}/.bash_profile" ]; then
        SHELL_RC="${USER_HOME}/.bash_profile"
      elif [ -f "${USER_HOME}/.bashrc" ]; then
        SHELL_RC="${USER_HOME}/.bashrc"
      else
        # Default to .bashrc if neither exists
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
  if command_exists sudo && command_exists groups; then
    current_user=$(get_real_user)
    if ! groups "$current_user" | grep -q docker; then
      color_echo "blue" "🔧 Adding '$current_user' to Docker group..."
      sudo usermod -aG docker "$current_user"
      color_echo "yellow" "⚡ You may need to log out and log back in for this to take effect."
    fi
  fi
  
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

# Enhanced Docker check with Arch Linux optimization
check_docker() {
  color_echo "blue" "🔍 Checking if Docker is installed..."

  if command_exists docker; then
    color_echo "green" "✅ Docker is already installed. You're all set for RF-Swift!"
    return 0
  fi

  color_echo "yellow" "⚠️ Docker is not installed on your system."
  color_echo "blue" "ℹ️ Docker is required for RF-Swift to work properly."
  
  # Provide advice on running Docker with reduced privileges
  color_echo "cyan" "📝 Docker Security Advice:"
  color_echo "cyan" "   - Consider using Docker Desktop which provides a user-friendly interface"
  color_echo "cyan" "   - On Linux, add your user to the 'docker' group to avoid using sudo with each Docker command"
  color_echo "cyan" "   - Use rootless Docker mode if you need enhanced security"
  color_echo "cyan" "   - Always pull container images from trusted sources"
  
  # Check if this is a Steam Deck and offer Steam Deck specific installation
  if [ "$(uname -s)" = "Linux" ]; then
    if is_steam_deck; then
      color_echo "magenta" "🎮 Steam Deck detected!"
      if prompt_yes_no "Would you like to install Docker using Steam Deck optimized method?" "y"; then
        install_docker_steamdeck
        return $?
      fi
    else
      if prompt_yes_no "Are you installing on a Steam Deck?" "n"; then
        install_docker_steamdeck
        return $?
      fi
    fi
  fi
  
  # Ask if the user wants to install Docker using standard method
  if prompt_yes_no "Would you like to install Docker now?" "n"; then
    install_docker
    return $?
  else
    color_echo "yellow" "⚠️ Docker installation skipped. You'll need to install Docker manually before using RF-Swift."
    return 1
  fi
}

# Enhanced Docker installation with Arch Linux support
install_docker() {
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
        sudo pacman -S --noconfirm --needed docker docker-compose
        
        # Enable and start Docker service
        if command_exists systemctl; then
          color_echo "blue" "🚀 Enabling and starting Docker service..."
          sudo systemctl enable docker
          sudo systemctl start docker
        fi
        
        # Add user to docker group
        current_user=$(get_real_user)
        if ! groups "$current_user" 2>/dev/null | grep -q docker; then
          color_echo "blue" "🔧 Adding '$current_user' to Docker group..."
          sudo usermod -aG docker "$current_user"
          color_echo "yellow" "⚡ You may need to log out and log back in for Docker group changes to take effect."
        fi
        
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
        
        if command_exists curl; then
          curl -fsSL "https://get.docker.com/" | sudo sh
        elif command_exists wget; then
          wget -qO- "https://get.docker.com/" | sudo sh
        else
          color_echo "red" "🚨 Missing curl/wget. Please install one of them."
          return 1
        fi

        if command_exists sudo && command_exists groups; then
          current_user=$(get_real_user)
          if ! groups "$current_user" 2>/dev/null | grep -q docker; then
            color_echo "blue" "🔧 Adding you to the Docker group..."
            sudo usermod -aG docker "$current_user"
            color_echo "yellow" "⚡ You may need to log out and log back in for this to take effect."
          fi
        fi
        
        if command_exists systemctl; then
          color_echo "blue" "🚀 Starting Docker service..."
          sudo systemctl start docker
          sudo systemctl enable docker
        fi

        color_echo "green" "🎉 Docker is now installed and running!"
      fi
      ;;
      
    *)
      color_echo "red" "🚨 Unsupported OS detected: $(uname -s). Docker can't be installed automatically here."
      return 1
      ;;
  esac
}

# Function to get the latest release information
get_latest_release() {
  color_echo "blue" "🔍 Detecting the latest RF-Swift release..."

  # Default version as fallback
  DEFAULT_VERSION="0.6.3"
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
        color_echo "green" "✅ Successfully retrieved latest version using GitHub API"
      fi
    else
      color_echo "yellow" "GitHub API query didn't return expected results. Trying alternative method..."
    fi
  fi
  
  # Second try: Parse the releases page directly if API method failed
  if [ "${VERSION}" = "${DEFAULT_VERSION}" ] && command_exists curl; then
    color_echo "blue" "Trying direct HTML parsing method..."
    
    RELEASES_PAGE=$(curl -s -L -H "User-Agent: RF-Swift-Installer" "https://github.com/${GITHUB_REPO}/releases/latest")
    
    # Look for version in the page title and URL
    DETECTED_VERSION=$(echo "${RELEASES_PAGE}" | grep -o "${GITHUB_REPO}/releases/tag/v[0-9][0-9.]*" | head -1 | sed 's/.*tag\/v//')
    
    if [ -n "${DETECTED_VERSION}" ]; then
      VERSION="${DETECTED_VERSION}"
      color_echo "green" "✅ Retrieved version ${VERSION} by parsing GitHub releases page"
    else
      # One last attempt - look for version in the title
      DETECTED_VERSION=$(echo "${RELEASES_PAGE}" | grep -o '<title>Release v[0-9][0-9.]*' | head -1 | sed 's/.*Release v//')
      
      if [ -n "${DETECTED_VERSION}" ]; then
        VERSION="${DETECTED_VERSION}"
        color_echo "green" "✅ Retrieved version ${VERSION} from page title"
      else
        color_echo "yellow" "⚠️ Using default version ${DEFAULT_VERSION} as a fallback"
      fi
    fi
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
  
  color_echo "blue" "🏠 Detected system: ${OS} ${ARCH}"
  color_echo "blue" "📂 Will download: ${FILENAME}"
}

# Download the files and display checksum information
download_files() {
  color_echo "blue" "🌟 Preparing to download RF-Swift..."

  # Create temporary directory and store it in a global variable
  TMP_DIR=$(mktemp -d)
  color_echo "blue" "🔽 Downloading RF-Swift binary from ${DOWNLOAD_URL}..."
  
  # Download the file
  if command_exists curl; then
    curl -L -o "${TMP_DIR}/${FILENAME}" "${DOWNLOAD_URL}" --progress-bar
  elif command_exists wget; then
    wget -q --show-progress -O "${TMP_DIR}/${FILENAME}" "${DOWNLOAD_URL}"
  else
    color_echo "red" "🚨 Missing curl or wget. Please install one of them."
    exit 1
  fi
  
  # Calculate and display checksum
  color_echo "blue" "Downloaded file: ${TMP_DIR}/${FILENAME}"
  
  CALCULATED_CHECKSUM=""
  if command_exists shasum; then
    CALCULATED_CHECKSUM=$(shasum -a 256 "${TMP_DIR}/${FILENAME}" | cut -d ' ' -f 1)
  elif command_exists sha256sum; then
    CALCULATED_CHECKSUM=$(sha256sum "${TMP_DIR}/${FILENAME}" | cut -d ' ' -f 1)
  fi
  
  if [ -n "$CALCULATED_CHECKSUM" ]; then
    color_echo "blue" "Calculated checksum: $CALCULATED_CHECKSUM"
  else
    color_echo "yellow" "⚠️ Could not calculate checksum (missing shasum/sha256sum tools)"
  fi
  
  # Set the exact checksums file URL format
  CHECKSUMS_URL="https://github.com/${GITHUB_REPO}/releases/download/v${VERSION}/RF-Swift_${VERSION}_checksums.txt"
  color_echo "blue" "GitHub checksums file: ${CHECKSUMS_URL}"
  
  # GitHub release page for manual verification
  RELEASE_PAGE_URL="https://github.com/${GITHUB_REPO}/releases/tag/v${VERSION}"
  color_echo "yellow" "If needed, verify the checksum by visiting the GitHub release page: ${RELEASE_PAGE_URL}"
  
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
  color_echo "blue" "🏠 Choose where to install RF-Swift..."
  color_echo "cyan" "You have two options:"
  color_echo "cyan" "1. System-wide installation (/usr/local/bin) - requires sudo"
  color_echo "cyan" "2. User-local installation (~/.rfswift/bin) - doesn't require sudo"
  
  if prompt_yes_no "Install system-wide (requires sudo)?" "n"; then
    INSTALL_DIR="/usr/local/bin"
    if ! have_sudo_access; then
      color_echo "red" "🚨 System-wide installation requires sudo. You don't seem to have sudo access."
      color_echo "yellow" "Falling back to user-local installation."
      INSTALL_DIR="$HOME/.rfswift/bin"
    fi
  else
    INSTALL_DIR="$HOME/.rfswift/bin"
  fi
  
  color_echo "green" "👍 Will install RF-Swift to: ${INSTALL_DIR}"
  return 0
}

# Install the binary
install_binary() {
  color_echo "blue" "🔧 Installing RF-Swift..."
  
  # Create installation directory if needed
  if [ "$INSTALL_DIR" = "/usr/local/bin" ]; then
    if ! have_sudo_access; then
      color_echo "red" "🚨 System-wide installation requires sudo. Please run with sudo or choose user-local installation."
      exit 1
    fi
    sudo mkdir -p "$INSTALL_DIR"
  else
    mkdir -p "$INSTALL_DIR"
  fi
  
  color_echo "blue" "📦 Extracting archive..."
  tar -xzf "${TMP_DIR}/${FILENAME}" -C "${TMP_DIR}"
  
  RFSWIFT_BIN=$(find "${TMP_DIR}" -name "rfswift" -type f)
  if [ -z "${RFSWIFT_BIN}" ]; then
    color_echo "red" "🚨 Couldn't find the binary in the archive."
    exit 1
  fi

  color_echo "blue" "🚀 Moving RF-Swift to ${INSTALL_DIR}..."
  if [ "$INSTALL_DIR" = "/usr/local/bin" ]; then
    sudo cp "${RFSWIFT_BIN}" "${INSTALL_DIR}/rfswift"
    sudo chmod +x "${INSTALL_DIR}/rfswift"
  else
    cp "${RFSWIFT_BIN}" "${INSTALL_DIR}/rfswift"
    chmod +x "${INSTALL_DIR}/rfswift"
  fi
  
  # Clean up
  rm -rf "${TMP_DIR}"
  
  color_echo "green" "🎉 RF-Swift has been installed successfully to ${INSTALL_DIR}/rfswift!"
}

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
    
    # Clear the screen for better presentation
    clear
    
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
            sudo dnf install -y \
              powerline-fonts \
              fira-code-fonts \
              hack-fonts \
              google-noto-fonts \
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
      color_echo "cyan" "• Terminal.app: Preferences → Profiles → Text → Font"
      color_echo "cyan" "• iTerm2: Preferences → Profiles → Text → Font"
      color_echo "cyan" "• Recommended fonts: 'Fira Code Nerd Font', 'Hack Nerd Font'"
      ;;
    Linux*)
      color_echo "blue" "🐧 Linux Terminal Configuration:"
      color_echo "cyan" "• GNOME Terminal: Preferences → Profiles → Text → Custom font"
      color_echo "cyan" "• Konsole: Settings → Edit Current Profile → Appearance → Font"
      color_echo "cyan" "• Alacritty: Edit ~/.config/alacritty/alacritty.yml"
      color_echo "cyan" "• Terminator: Right-click → Preferences → Profiles → Font"
      color_echo "cyan" "• VS Code: Settings → Terminal → Font Family"
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
      unzip -q "$font_zip"
      cp *.ttf *.otf "$FONTS_DIR/" 2>/dev/null || true
    fi
  done
  
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

# Function to test font installation
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

# Main function
main() {
  display_rainbow_logo_animated

  fun_welcome
  
  # Verify system requirements first
  if ! verify_system_requirements; then
    color_echo "red" "🚨 Cannot proceed due to missing system requirements."
    exit 1
  fi
  
  # Show Steam Deck detection status
  if is_steam_deck; then
    color_echo "magenta" "🎮 Steam Deck detected! Special optimizations will be applied."
  fi
  
  # Check if Docker is installed and offer to install it if not
  check_docker
  
  # Check and install audio system
  check_audio_system
  
  # Get latest release info
  get_latest_release
  
  # Detect system architecture
  detect_system
  
  # Download files
  download_files
  
  # Choose installation directory
  choose_install_dir
  
  # Install binary
  install_binary

  # check and install agnoster deps
  check_agnoster_dependencies
  
  # Checking xhost
  check_xhost

  # Set up alias if requested
  if prompt_yes_no "Would you like to set up an alias for RF-Swift?" "y"; then
    create_alias "$INSTALL_DIR"
  fi
  
  # Show audio system status
  show_audio_status
  
  thank_you_message
  
  # Final instructions
  if [ "$INSTALL_DIR" != "/usr/local/bin" ]; then
    color_echo "cyan" "🚀 To use RF-Swift, you can:"
    color_echo "cyan" "   - Run it directly: ${INSTALL_DIR}/rfswift"
    color_echo "cyan" "   - Add ${INSTALL_DIR} to your PATH"
    if is_arch_linux; then
      color_echo "cyan" "   - Or use the alias if you set it up: rfswift"
    fi
  else
    color_echo "cyan" "🚀 You can now run RF-Swift by simply typing: rfswift"
  fi
  
  color_echo "magenta" "🎵 Audio system is configured and ready for RF-Swift containers!"
  
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

  color_echo "cyan" "📡 Happy RF hacking! 🚀"
}

# Run the main function
main