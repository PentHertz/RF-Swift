#!/bin/bash
# RF-Swift Installer Script
# Usage: curl -fsSL "https://get.rfswift.io/" | sudo sh
# or: wget -qO- "https://get.rfswift.io/" | sudo sh

set -e

# Configuration
GITHUB_REPO="PentHertz/RF-Swift"
INSTALL_DIR="/usr/local/bin"  # Only install to system directory

# Function to output colored text
color_echo() {
  local color=$1
  local text=$2
  case $color in
    "red") echo -e "\033[31m${text}\033[0m" ;;
    "green") echo -e "\033[32m${text}\033[0m" ;;
    "yellow") echo -e "\033[33m${text}\033[0m" ;;
    "blue") echo -e "\033[34m${text}\033[0m" ;;
    "magenta") echo -e "\033[35m${text}\033[0m" ;;
    "cyan") echo -e "\033[36m${text}\033[0m" ;;
    *) echo "${text}" ;;
  esac
}

# Fun welcome message
fun_welcome() {
  color_echo "cyan" "🎉 WELCOME TO THE RF-Swift Installer! 🚀"
  color_echo "yellow" "Prepare yourself for an epic journey in the world of radio frequencies! 📡"
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

# Function to check and install Docker
install_docker() {
  color_echo "blue" "🔍 Checking if Docker is installed..."

  if command_exists docker; then
    color_echo "green" "🎉 Docker is already installed. You’re all set for RF-Swift!"
    return 0
  fi

  color_echo "yellow" "Oops! It looks like Docker is missing. Don't worry, I'm installing it for you..."

  case "$(uname -s)" in
    Darwin*)
      if command_exists brew; then
        color_echo "blue" "🍏 Installing Docker via Homebrew..."
        brew install --cask docker
        
        color_echo "blue" "🚀 Launching Docker now... Hold tight!"
        open -a Docker
        
        color_echo "yellow" "⏳ Give it a moment, Docker is warming up!"
        for i in {1..30}; do
          if command_exists docker && docker info >/dev/null 2>&1; then
            color_echo "green" "✅ Docker is up and running!"
            return 0
          fi
          sleep 2
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
      
      if command_exists sudo; then
        sudo_cmd="sudo"
      else
        sudo_cmd=""
        color_echo "yellow" "It seems like you don’t have 'sudo'. Installation might not work without root privileges."
      fi

      if command_exists curl; then
        curl -fsSL "https://get.docker.com/" | $sudo_cmd sh
      elif command_exists wget; then
        wget -qO- "https://get.docker.com/" | $sudo_cmd sh
      else
        color_echo "red" "🚨 Missing curl/wget. Please install one of them."
        return 1
      fi

      if command_exists sudo && command_exists groups; then
        if ! groups | grep -q docker; then
          color_echo "blue" "🔧 Adding you to the Docker group..."
          $sudo_cmd usermod -aG docker "$(get_real_user)"
          color_echo "yellow" "⚡ You may need to log out and log back in for this to take effect."
        fi
      fi
      
      if command_exists systemctl; then
        color_echo "blue" "🚀 Starting Docker service..."
        $sudo_cmd systemctl start docker
        $sudo_cmd systemctl enable docker
      fi

      color_echo "green" "🎉 Docker is now installed and running!"
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

  DEFAULT_VERSION="0.6.3"

  if command_exists curl && command_exists jq; then
    LATEST_INFO=$(curl -s "https://api.github.com/repos/${GITHUB_REPO}/releases/latest" | tr -d '\000-\037')

    if ! echo "${LATEST_INFO}" | jq -e . >/dev/null 2>&1; then
      color_echo "yellow" "Uh oh! Couldn’t parse the GitHub API response. Let’s try a fallback method..."
      # Fallback method
      if command_exists curl && command_exists grep && command_exists sed; then
        RELEASES_PAGE=$(curl -s "https://github.com/${GITHUB_REPO}/releases/latest")
        VERSION=$(echo "${RELEASES_PAGE}" | grep -o "${GITHUB_REPO}/releases/tag/v[0-9.]*" | head -1 | sed 's/.*tag\/v//')
        RELEASE_URL="https://github.com/${GITHUB_REPO}/releases/tag/v${VERSION}"
        DOWNLOAD_BASE_URL="https://github.com/${GITHUB_REPO}/releases/download/v${VERSION}"
      fi
    else
      VERSION=$(echo "${LATEST_INFO}" | jq -r .tag_name | sed 's/^v//')
      RELEASE_URL=$(echo "${LATEST_INFO}" | jq -r .html_url)
      DOWNLOAD_BASE_URL="https://github.com/${GITHUB_REPO}/releases/download/v${VERSION}"
    fi
  fi
  
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

  color_echo "blue" "🏠 Detected system: ${OS} ${ARCH}"
}

# Download the files
download_files() {
  color_echo "blue" "🌟 Preparing to download RF-Swift..."

  TMP_DIR=$(mktemp -d)
  color_echo "blue" "🔽 Downloading RF-Swift binary..."
  
  if command_exists curl; then
    curl -L -o "${TMP_DIR}/${FILENAME}" "${DOWNLOAD_URL}" --progress-bar
  elif command_exists wget; then
    wget -q --show-progress -O "${TMP_DIR}/${FILENAME}" "${DOWNLOAD_URL}"
  else
    color_echo "red" "🚨 Missing curl or wget. Please install one of them."
    exit 1
  fi
}

# Install the binary
install_binary() {
  color_echo "blue" "🔧 Installing RF-Swift..."
  
  if ! have_sudo_access; then
    color_echo "red" "🚨 This requires sudo. Please run with sudo or as root."
    exit 1
  fi
  
  tar -xzf "${TMP_DIR}/${FILENAME}" -C "${TMP_DIR}"
  RFSWIFT_BIN=$(find "${TMP_DIR}" -name "rfswift" -type f)
  if [ -z "${RFSWIFT_BIN}" ]; then
    color_echo "red" "🚨 Couldn’t find the binary in the archive."
    exit 1
  fi

  color_echo "blue" "🚀 Moving RF-Swift to ${INSTALL_DIR}..."
  sudo cp "${RFSWIFT_BIN}" "${INSTALL_DIR}/rfswift"
  sudo chmod +x "${INSTALL_DIR}/rfswift"
  
  color_echo "green" "🎉 RF-Swift has been installed successfully!"
}

# Main function
main() {
  fun_welcome
  
  install_docker
  get_latest_release
  detect_system
  download_files
  install_binary
  thank_you_message
}

main
