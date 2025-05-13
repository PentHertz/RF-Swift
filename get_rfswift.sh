#!/bin/sh
# RF-Swift Installer Script
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

# Function to prompt user for yes/no with terminal redirection solution
prompt_yes_no() {
  local prompt="$1"
  local default="$2"  # Optional default (y/n)
  local response
  
  # Try to use /dev/tty for interactive input even in pipe scenarios
  if [ -t 0 ]; then
    # Standard terminal input is available
    tty_device="/dev/stdin"
  elif [ -e "/dev/tty" ]; then
    # We're in a pipe but /dev/tty might be available
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

# Function to check if Docker is installed
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
  
  # Ask if the user wants to install Docker
  if prompt_yes_no "Would you like to install Docker now?" "n"; then
    install_docker
    return $?
  else
    color_echo "yellow" "⚠️ Docker installation skipped. You'll need to install Docker manually before using RF-Swift."
    return 1
  fi
}

# Function to install Docker
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
        if ! groups | grep -q docker; then
          color_echo "blue" "🔧 Adding you to the Docker group..."
          sudo usermod -aG docker "$(get_real_user)"
          color_echo "yellow" "⚡ You may need to log out and log back in for this to take effect."
        fi
      fi
      
      if command_exists systemctl; then
        color_echo "blue" "🚀 Starting Docker service..."
        sudo systemctl start docker
        sudo systemctl enable docker
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
    
    # Add a tagline
    printf "\n%b🔥 RF Swift by @Penthertz - Radio Frequency Swiss Army Knife 🔥%b\n\n" "$PURPLE" "$NC"
    
    # Add a slight delay before continuing
    sleep 0.5
}

# Main function
main() {
  display_rainbow_logo_animated

  fun_welcome
  
  # Check if Docker is installed and offer to install it if not
  check_docker
  
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
  
  # Set up alias if requested
  if prompt_yes_no "Would you like to set up an alias for RF-Swift?" "y"; then
    create_alias "$INSTALL_DIR"
  fi
  
  thank_you_message
  
  # Final instructions
  if [ "$INSTALL_DIR" != "/usr/local/bin" ]; then
    color_echo "cyan" "🚀 To use RF-Swift, you can:"
    color_echo "cyan" "   - Run it directly: ${INSTALL_DIR}/rfswift"
    color_echo "cyan" "   - Add ${INSTALL_DIR} to your PATH"
  else
    color_echo "cyan" "🚀 You can now run RF-Swift by simply typing: rfswift"
  fi
}

# Run the main function
main
