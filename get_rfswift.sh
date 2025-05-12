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
    *) echo "${text}" ;;
  esac
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
  color_echo "blue" "Checking if Docker is installed..."
  
  if command_exists docker; then
    color_echo "green" "Docker is already installed."
    return 0
  fi
  
  color_echo "yellow" "Docker is not installed. Installing Docker..."
  
  case "$(uname -s)" in
    Darwin*)
      # macOS - use Homebrew
      if command_exists brew; then
        color_echo "blue" "Installing Docker using Homebrew..."
        brew install --cask docker
        
        # Start Docker application
        color_echo "blue" "Starting Docker application..."
        open -a Docker
        
        # Wait for Docker to start
        color_echo "yellow" "Waiting for Docker to start. This may take a minute..."
        for i in {1..30}; do
          if command_exists docker && docker info >/dev/null 2>&1; then
            color_echo "green" "Docker is now running."
            return 0
          fi
          sleep 2
        done
        
        color_echo "yellow" "Docker has been installed but may not be running yet."
        color_echo "yellow" "Please open the Docker application manually to complete setup."
      else
        color_echo "red" "Homebrew is not installed. Please install Homebrew first:"
        color_echo "yellow" '/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"'
        color_echo "yellow" "Then run this script again."
        return 1
      fi
      ;;
      
    Linux*)
      # Linux - use the official Docker install script
      color_echo "blue" "Installing Docker using the official Docker install script..."
      
      # Check for sudo privileges
      if command_exists sudo; then
        sudo_cmd="sudo"
      else
        sudo_cmd=""
        color_echo "yellow" "The 'sudo' command was not found. Installation may fail if you don't have root privileges."
      fi
      
      # Install Docker
      if command_exists curl; then
        curl -fsSL "https://get.docker.com/" | $sudo_cmd sh
      elif command_exists wget; then
        wget -qO- "https://get.docker.com/" | $sudo_cmd sh
      else
        color_echo "red" "Neither curl nor wget found. Please install one of them and try again."
        return 1
      fi
      
      # Add current user to docker group to avoid sudo for docker commands
      if command_exists sudo && command_exists groups; then
        if ! groups | grep -q docker; then
          color_echo "blue" "Adding your user to the docker group..."
          $sudo_cmd usermod -aG docker "$(get_real_user)"
          color_echo "yellow" "You may need to log out and log back in for the group changes to take effect."
        fi
      fi
      
      # Start Docker service
      if command_exists systemctl; then
        color_echo "blue" "Starting Docker service..."
        $sudo_cmd systemctl start docker
        $sudo_cmd systemctl enable docker
      fi
      
      color_echo "green" "Docker has been installed successfully."
      ;;
      
    *)
      color_echo "red" "Unsupported operating system: $(uname -s). Cannot install Docker automatically."
      return 1
      ;;
  esac
}

# Function to get the latest release information
get_latest_release() {
  color_echo "blue" "Detecting latest release..."
  
  # Default version as fallback
  DEFAULT_VERSION="0.6.3" # Replace with your actual latest version
  
  if command_exists curl && command_exists jq; then
    # Using curl and jq (preferred method) - Fixed to handle control characters
    LATEST_INFO=$(curl -s "https://api.github.com/repos/${GITHUB_REPO}/releases/latest" | tr -d '\000-\037')
    
    # Check if the result is valid JSON
    if ! echo "${LATEST_INFO}" | jq -e . > /dev/null 2>&1; then
      color_echo "yellow" "Could not parse GitHub API response with jq, falling back to grep method."
      # Fall back to grep method
      if command_exists curl && command_exists grep && command_exists sed; then
        RELEASES_PAGE=$(curl -s "https://github.com/${GITHUB_REPO}/releases/latest")
        VERSION=$(echo "${RELEASES_PAGE}" | grep -o "${GITHUB_REPO}/releases/tag/v[0-9.]*" | head -1 | sed 's/.*tag\/v//')
        RELEASE_URL="https://github.com/${GITHUB_REPO}/releases/tag/v${VERSION}"
        DOWNLOAD_BASE_URL="https://github.com/${GITHUB_REPO}/releases/download/v${VERSION}"
      elif command_exists wget && command_exists grep && command_exists sed; then
        RELEASES_PAGE=$(wget -qO- "https://github.com/${GITHUB_REPO}/releases/latest")
        VERSION=$(echo "${RELEASES_PAGE}" | grep -o "${GITHUB_REPO}/releases/tag/v[0-9.]*" | head -1 | sed 's/.*tag\/v//')
        RELEASE_URL="https://github.com/${GITHUB_REPO}/releases/tag/v${VERSION}"
        DOWNLOAD_BASE_URL="https://github.com/${GITHUB_REPO}/releases/download/v${VERSION}"
      else
        # Fall back to default version
        VERSION="${DEFAULT_VERSION}"
        RELEASE_URL="https://github.com/${GITHUB_REPO}/releases/tag/v${VERSION}"
        DOWNLOAD_BASE_URL="https://github.com/${GITHUB_REPO}/releases/download/v${VERSION}"
        color_echo "yellow" "Falling back to default version: ${VERSION}"
      fi
    else
      # Successfully parsed JSON, proceed with jq
      VERSION=$(echo "${LATEST_INFO}" | jq -r .tag_name | sed 's/^v//')
      RELEASE_URL=$(echo "${LATEST_INFO}" | jq -r .html_url)
      DOWNLOAD_BASE_URL="https://github.com/${GITHUB_REPO}/releases/download/v${VERSION}"
    fi
  elif command_exists curl && command_exists grep && command_exists sed; then
    # Fallback method using curl and grep (less reliable)
    RELEASES_PAGE=$(curl -s "https://github.com/${GITHUB_REPO}/releases/latest")
    VERSION=$(echo "${RELEASES_PAGE}" | grep -o "${GITHUB_REPO}/releases/tag/v[0-9.]*" | head -1 | sed 's/.*tag\/v//')
    RELEASE_URL="https://github.com/${GITHUB_REPO}/releases/tag/v${VERSION}"
    DOWNLOAD_BASE_URL="https://github.com/${GITHUB_REPO}/releases/download/v${VERSION}"
  elif command_exists wget && command_exists grep && command_exists sed; then
    # Fallback method using wget and grep
    RELEASES_PAGE=$(wget -qO- "https://github.com/${GITHUB_REPO}/releases/latest")
    VERSION=$(echo "${RELEASES_PAGE}" | grep -o "${GITHUB_REPO}/releases/tag/v[0-9.]*" | head -1 | sed 's/.*tag\/v//')
    RELEASE_URL="https://github.com/${GITHUB_REPO}/releases/tag/v${VERSION}"
    DOWNLOAD_BASE_URL="https://github.com/${GITHUB_REPO}/releases/download/v${VERSION}"
  else
    color_echo "red" "Requires curl/wget and either jq or grep to detect the latest version."
    color_echo "yellow" "Falling back to default version..."
    # Use default version
    VERSION="${DEFAULT_VERSION}"
    RELEASE_URL="https://github.com/${GITHUB_REPO}/releases/tag/v${VERSION}"
    DOWNLOAD_BASE_URL="https://github.com/${GITHUB_REPO}/releases/download/v${VERSION}"
  fi
  
  # Final fallback if all methods failed to set a version
  if [ -z "${VERSION}" ]; then
    color_echo "yellow" "Could not determine the latest version. Using default."
    VERSION="${DEFAULT_VERSION}"
    RELEASE_URL="https://github.com/${GITHUB_REPO}/releases/tag/v${VERSION}"
    DOWNLOAD_BASE_URL="https://github.com/${GITHUB_REPO}/releases/download/v${VERSION}"
  fi
  
  color_echo "green" "Using version: ${VERSION}"
}

# Function to detect OS and architecture
detect_system() {
  # Detect operating system
  case "$(uname -s)" in
    Linux*)  OS="Linux" ;;
    Darwin*) OS="Darwin" ;;
    *)       color_echo "red" "Unsupported operating system: $(uname -s)"; exit 1 ;;
  esac

  # Detect architecture
  case "$(uname -m)" in
    x86_64)  ARCH="x86_64" ;;
    arm64|aarch64) ARCH="arm64" ;;
    riscv64) ARCH="riscv64" ;;
    *)       color_echo "red" "Unsupported architecture: $(uname -m)"; exit 1 ;;
  esac

  # Check if riscv64 is supported on this OS
  if [ "$OS" = "Darwin" ] && [ "$ARCH" = "riscv64" ]; then
    color_echo "red" "riscv64 architecture is not supported on macOS"
    exit 1
  fi

  FILENAME="rfswift_${OS}_${ARCH}.tar.gz"
  DOWNLOAD_URL="${DOWNLOAD_BASE_URL}/${FILENAME}"
  CHECKSUM_URL="${DOWNLOAD_BASE_URL}/RF-Swift_${VERSION}_checksums.txt"
  
  color_echo "blue" "Detected system: ${OS} ${ARCH}"
  color_echo "blue" "Will download: ${FILENAME}"
}

# Download the files
download_files() {
  color_echo "blue" "Creating temporary directory..."
  TMP_DIR=$(mktemp -d)
  
  color_echo "blue" "Downloading RF-Swift binary package..."
  if command_exists curl; then
    curl -L -o "${TMP_DIR}/${FILENAME}" "${DOWNLOAD_URL}" --progress-bar
    curl -L -o "${TMP_DIR}/checksums.txt" "${CHECKSUM_URL}" --progress-bar
  elif command_exists wget; then
    wget -q --show-progress -O "${TMP_DIR}/${FILENAME}" "${DOWNLOAD_URL}"
    wget -q --show-progress -O "${TMP_DIR}/checksums.txt" "${CHECKSUM_URL}"
  else
    color_echo "red" "Neither curl nor wget found. Please install one of them."
    exit 1
  fi
}

# Verify checksums
verify_checksums() {
  color_echo "blue" "Verifying checksums..."
  cd "${TMP_DIR}"
  
  # Extract the expected checksum for our file
  EXPECTED_CHECKSUM=$(grep "${FILENAME}" checksums.txt | awk '{print $1}')
  
  if [ -z "${EXPECTED_CHECKSUM}" ]; then
    color_echo "red" "Could not find checksum for ${FILENAME} in the checksums file."
    exit 1
  fi
  
  # Calculate actual checksum
  if command_exists shasum; then
    ACTUAL_CHECKSUM=$(shasum -a 256 "${FILENAME}" | awk '{print $1}')
  elif command_exists sha256sum; then
    ACTUAL_CHECKSUM=$(sha256sum "${FILENAME}" | awk '{print $1}')
  else
    color_echo "red" "Neither shasum nor sha256sum found. Cannot verify checksums."
    exit 1
  fi
  
  if [ "${EXPECTED_CHECKSUM}" != "${ACTUAL_CHECKSUM}" ]; then
    color_echo "red" "Checksum verification failed!"
    color_echo "red" "Expected: ${EXPECTED_CHECKSUM}"
    color_echo "red" "Actual:   ${ACTUAL_CHECKSUM}"
    exit 1
  fi
  
  color_echo "green" "Checksum verification passed!"
}

# Install the binary
install_binary() {
  color_echo "blue" "Installing RF-Swift..."
  
  # Check for sudo access - required for system directory installation
  if ! have_sudo_access; then
    color_echo "red" "This script requires sudo privileges to install RF-Swift to ${INSTALL_DIR}."
    color_echo "red" "Please run this script with sudo or as root."
    exit 1
  fi
  
  # Extract the archive
  tar -xzf "${TMP_DIR}/${FILENAME}" -C "${TMP_DIR}"
  
  # Find the rfswift binary and move it to the installation directory
  RFSWIFT_BIN=$(find "${TMP_DIR}" -name "rfswift" -type f)
  if [ -z "${RFSWIFT_BIN}" ]; then
    color_echo "red" "Could not find rfswift binary in the extracted archive."
    exit 1
  fi
  
  # Install to system directory
  color_echo "blue" "Installing RF-Swift to ${INSTALL_DIR}..."
  sudo mkdir -p "${INSTALL_DIR}"
  sudo cp "${RFSWIFT_BIN}" "${INSTALL_DIR}/rfswift"
  sudo chmod +x "${INSTALL_DIR}/rfswift"
  
  # Clean up
  rm -rf "${TMP_DIR}"
  
  color_echo "green" "RF-Swift version ${VERSION} has been installed to ${INSTALL_DIR}/rfswift"
}

# Add shell alias
setup_alias() {
  color_echo "blue" "Setting up shell alias..."
  
  # Get the actual user even when run with sudo
  REAL_USER=$(get_real_user)
  
  # If we're running as root but not via sudo, we don't know the real user's home directory
  if [ "$(whoami)" = "root" ] && [ -z "$SUDO_USER" ]; then
    color_echo "yellow" "Running as root directly. Cannot determine user's home directory for alias setup."
    color_echo "yellow" "Please run 'alias rfswift=\"${INSTALL_DIR}/rfswift\"' manually in your shell."
    return 1
  fi
  
  USER_HOME=$(eval echo ~${REAL_USER})
  
  # Detect shell for the real user
  if [ -n "$SUDO_USER" ]; then
    # Get the default shell for the sudo user
    USER_SHELL=$(getent passwd $SUDO_USER | cut -d: -f7)
  else
    USER_SHELL=$SHELL
  fi
  
  SHELL_NAME=$(basename "$USER_SHELL")
  
  # Determine the correct rc file
  case "${SHELL_NAME}" in
    bash)
      if [ -f "${USER_HOME}/.bashrc" ]; then
        RC_FILE="${USER_HOME}/.bashrc"
      elif [ -f "${USER_HOME}/.bash_profile" ]; then
        RC_FILE="${USER_HOME}/.bash_profile"
      else
        RC_FILE="${USER_HOME}/.bashrc"
        touch "${RC_FILE}"
        chown ${REAL_USER}: "${RC_FILE}"
      fi
      ;;
    zsh)
      RC_FILE="${USER_HOME}/.zshrc"
      if [ ! -f "${RC_FILE}" ]; then
        touch "${RC_FILE}"
        chown ${REAL_USER}: "${RC_FILE}"
      fi
      ;;
    *)
      color_echo "yellow" "Unsupported shell: ${SHELL_NAME}. You'll need to manually add an alias for rfswift."
      color_echo "yellow" "You can run rfswift by typing ${INSTALL_DIR}/rfswift"
      return 1
      ;;
  esac
  
  # Make sure we can write to the RC file
  if [ ! -w "${RC_FILE}" ]; then
    if have_sudo_access; then
      TMP_RC_FILE=$(mktemp)
      sudo cp "${RC_FILE}" "${TMP_RC_FILE}"
      sudo chown $(whoami): "${TMP_RC_FILE}"
      NEED_SUDO_CP=true
    else
      color_echo "red" "Cannot write to ${RC_FILE}. Please add the alias manually."
      color_echo "yellow" "Run: echo 'alias rfswift=\"${INSTALL_DIR}/rfswift\"' >> ${RC_FILE}"
      return 1
    fi
  else
    TMP_RC_FILE="${RC_FILE}"
    NEED_SUDO_CP=false
  fi
  
  # Check for existing alias and remove it
  if grep -q "alias.*rfswift" "${TMP_RC_FILE}"; then
    sed -i.bak '/alias.*rfswift/d' "${TMP_RC_FILE}"
  fi
  
  # Add new alias
  echo "" >> "${TMP_RC_FILE}"
  echo "# RF-Swift alias" >> "${TMP_RC_FILE}"
  echo "alias rfswift=\"${INSTALL_DIR}/rfswift\"" >> "${TMP_RC_FILE}"
  
  # If we used a temp file, copy it back with sudo
  if [ "$NEED_SUDO_CP" = "true" ]; then
    sudo cp "${TMP_RC_FILE}" "${RC_FILE}"
    sudo chown ${REAL_USER}: "${RC_FILE}"
    rm "${TMP_RC_FILE}"
  fi
  
  color_echo "green" "Added RF-Swift alias in ${RC_FILE}"
  color_echo "yellow" "You may need to restart your shell or run 'source ${RC_FILE}' to use the rfswift alias."
  
  ALIAS_ADDED=true
}

# Installation complete message
show_success() {
  color_echo "green" "=== Installation Complete! ==="
  color_echo "green" "RF-Swift ${VERSION} has been successfully installed to ${INSTALL_DIR}/rfswift"
  
  echo ""
  color_echo "blue" "Usage:"
  echo "  ${INSTALL_DIR}/rfswift     - Run RF-Swift directly"
  
  if [ "$ALIAS_ADDED" = "true" ]; then
    echo "  rfswift                 - Run RF-Swift using the alias (after restarting shell or sourcing RC file)"
  fi
  
  echo "  sudo ${INSTALL_DIR}/rfswift - Run RF-Swift with sudo privileges"
  
  # Remind about Docker setup if needed
  if [ "$DOCKER_INSTALLED" = "true" ]; then
    echo ""
    color_echo "yellow" "Docker Installation Notes:"
    if [ "$OS" = "Linux" ]; then
      color_echo "yellow" "- You might need to log out and log back in for Docker group changes to take effect."
      color_echo "yellow" "- Until then, you may need to use 'sudo' with Docker commands."
    elif [ "$OS" = "Darwin" ]; then
      color_echo "yellow" "- Make sure the Docker application is running before using Docker commands."
    fi
  fi
  
  echo ""
  color_echo "blue" "For more information, visit: https://github.com/${GITHUB_REPO}"
}

# Check for important commands
check_dependencies() {
  local missing_deps=false
  
  # Required for downloading
  if ! command_exists curl && ! command_exists wget; then
    color_echo "red" "Missing required dependency: curl or wget is required for downloading files."
    missing_deps=true
  fi
  
  # Required for checksum verification
  if ! command_exists shasum && ! command_exists sha256sum; then
    color_echo "red" "Missing required dependency: shasum or sha256sum is required for verifying file integrity."
    missing_deps=true
  fi
  
  if [ "$missing_deps" = "true" ]; then
    if [ "$OS" = "Darwin" ]; then
      color_echo "yellow" "Please install missing dependencies using Homebrew:"
      color_echo "yellow" "brew install curl"
    else
      color_echo "yellow" "Please install missing dependencies using your package manager."
      color_echo "yellow" "For example, on Ubuntu/Debian: sudo apt-get install curl"
    fi
    exit 1
  fi
}

# Main function
main() {
  echo "================ RF-Swift Installer ================"
  echo "Installing RF-Swift - The Radio Frequency Tool Suite"
  echo "==================================================="
  
  # Detect system early for other checks
  case "$(uname -s)" in
    Linux*)  OS="Linux" ;;
    Darwin*) OS="Darwin" ;;
    *)       color_echo "red" "Unsupported operating system: $(uname -s)"; exit 1 ;;
  esac
  
  # Check dependencies
  check_dependencies
  
  # Install Docker if not present
  install_docker
  if [ $? -eq 0 ] && ! command_exists docker; then
    DOCKER_INSTALLED="true"
  fi
  
  # Continue with RF-Swift installation
  get_latest_release
  detect_system
  download_files
  verify_checksums
  install_binary
  setup_alias
  show_success
}

# Run the main function - no arguments needed when piped to shell
main
