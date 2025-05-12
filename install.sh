#!/bin/bash
# RF-Swift Installer Script
# Usage: curl -fsSL "https://get.rfswift.io/" | sh
# or: wget -qO- "https://get.rfswift.io/" | sh

set -e

# Configuration
GITHUB_REPO="PentHertz/RF-Swift"
INSTALL_DIR="/usr/local/bin"  # Only install to system directory

# Function to output colored text - fixed to work in sh
color_echo() {
  local color=$1
  local text=$2
  case $color in
    "red") printf "\033[31m%s\033[0m\n" "${text}" ;;
    "green") printf "\033[32m%s\033[0m\n" "${text}" ;;
    "yellow") printf "\033[33m%s\033[0m\n" "${text}" ;;
    "blue") printf "\033[34m%s\033[0m\n" "${text}" ;;
    "magenta") printf "\033[35m%s\033[0m\n" "${text}" ;;
    "cyan") printf "\033[36m%s\033[0m\n" "${text}" ;;
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

# Function to create an alias for RF-Swift in the user's shell configuration
create_alias() {
  color_echo "blue" "🔗 Setting up an alias for RF-Swift..."
  
  # Get the real user even when run with sudo
  REAL_USER=$(get_real_user)
  USER_HOME=$(eval echo ~${REAL_USER})
  
  # Determine shell from the user's default shell
  USER_SHELL=$(getent passwd "${REAL_USER}" 2>/dev/null | cut -d: -f7 | xargs basename)
  if [ -z "${USER_SHELL}" ]; then
    # macOS fallback for getent
    USER_SHELL=$(dscl . -read /Users/${REAL_USER} UserShell 2>/dev/null | sed 's/UserShell: //' | xargs basename)
  fi
  if [ -z "${USER_SHELL}" ]; then
    USER_SHELL=$(basename "${SHELL}")
  fi
  
  SHELL_RC=""
  ALIAS_LINE="alias rfswift='${INSTALL_DIR}/rfswift'"
  
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
      ALIAS_LINE="alias rfswift '${INSTALL_DIR}/rfswift'"  # fish has different syntax
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
  if grep -q "alias rfswift=" "${SHELL_RC}" 2>/dev/null; then
    color_echo "green" "✅ RF-Swift alias already exists in ${SHELL_RC}"
    return 0
  fi
  
  # Add the alias to the shell configuration file
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
    color_echo "green" "🎉 Docker is already installed. You're all set for RF-Swift!"
    return 0
  fi

  color_echo "yellow" "Oops! It looks like Docker is missing. Don't worry, I'll help you install it..."

  case "$(uname -s)" in
    Darwin*)
      # For macOS, we need to be careful about sudo
      if [ "$(id -u)" = "0" ]; then
        # We're running as root (likely via sudo)
        REAL_USER=$(get_real_user)
        color_echo "yellow" "⚠️ Homebrew should not be run as root. Let's switch to user ${REAL_USER}..."
        
        if command_exists su; then
          # Create a temporary script to run as the normal user
          TMP_SCRIPT=$(mktemp)
          cat > "${TMP_SCRIPT}" << EOF
#!/bin/bash
export PATH="/usr/local/bin:${PATH}" # Ensure brew is in PATH if it exists
if command -v brew >/dev/null 2>&1; then
  echo "brew exists"
  brew install --cask docker
else
  echo "brew not found"
  echo "Please run the following commands manually after this script completes:"
  echo "/bin/bash -c \"\$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)\""
  echo "brew install --cask docker"
fi
EOF
          chmod +x "${TMP_SCRIPT}"
          
          # Run the script as the regular user
          su - "${REAL_USER}" -c "${TMP_SCRIPT}"
          BREW_RESULT=$?
          rm "${TMP_SCRIPT}"
          
          if [ "${BREW_RESULT}" -ne 0 ]; then
            color_echo "red" "🚨 Failed to install Docker through Homebrew."
            color_echo "yellow" "Please install manually with these commands:"
            color_echo "yellow" "/bin/bash -c \"\$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)\""
            color_echo "yellow" "brew install --cask docker"
            color_echo "yellow" "After installation, run the Docker application and then run this script again."
            return 1
          fi
        else
          color_echo "red" "🚨 Cannot switch to regular user mode. Please install Docker manually:"
          color_echo "yellow" "1. Exit this sudo session"
          color_echo "yellow" "2. Run: brew install --cask docker"
          color_echo "yellow" "3. Open Docker from Applications"
          color_echo "yellow" "4. Run this script again"
          return 1
        fi
      else
        # We're already running as a regular user
        if command_exists brew; then
          color_echo "blue" "🍏 Installing Docker via Homebrew..."
          brew install --cask docker
        else
          color_echo "red" "🚨 Homebrew is not installed! Please install Homebrew first:"
          color_echo "yellow" '/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"'
          color_echo "yellow" "Then, run the script again!"
          return 1
        fi
      fi
      
      # Launch Docker (as the real user, not root)
      REAL_USER=$(get_real_user)
      color_echo "blue" "🚀 Launching Docker now... Hold tight!"
      
      if [ "$(id -u)" = "0" ]; then
        # If running as root, we need to launch Docker as the real user
        su - "${REAL_USER}" -c "open -a Docker"
      else
        # Already running as regular user
        open -a Docker
      fi
      
      color_echo "yellow" "⏳ Give it a moment, Docker is warming up!"
      for i in {1..30}; do
        if command_exists docker && docker info >/dev/null 2>&1; then
          color_echo "green" "✅ Docker is up and running!"
          return 0
        fi
        sleep 2
      done
      
      color_echo "yellow" "Docker is installed but still starting. Please open Docker manually if needed."
      ;;
      
    Linux*)
      color_echo "blue" "🐧 Installing Docker on your Linux machine..."
      
      if command_exists sudo; then
        sudo_cmd="sudo"
      else
        sudo_cmd=""
        color_echo "yellow" "It seems like you don't have 'sudo'. Installation might not work without root privileges."
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

# Function to get the latest release information - FIXED
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

# Download the files
download_files() {
  color_echo "blue" "🌟 Preparing to download RF-Swift..."

  TMP_DIR=$(mktemp -d)
  color_echo "blue" "🔽 Downloading RF-Swift binary from ${DOWNLOAD_URL}..."
  
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
  
  if [ "$(id -u)" != "0" ] && ! have_sudo_access; then
    color_echo "red" "🚨 This requires sudo. Please run with sudo or as root."
    exit 1
  fi
  
  color_echo "blue" "📦 Extracting archive..."
  tar -xzf "${TMP_DIR}/${FILENAME}" -C "${TMP_DIR}"
  
  RFSWIFT_BIN=$(find "${TMP_DIR}" -name "rfswift" -type f)
  if [ -z "${RFSWIFT_BIN}" ]; then
    color_echo "red" "🚨 Couldn't find the binary in the archive."
    exit 1
  fi

  # Make sure installation directory exists
  if [ "$(id -u)" = "0" ]; then
    mkdir -p "${INSTALL_DIR}"
  else
    sudo mkdir -p "${INSTALL_DIR}"
  fi
  
  color_echo "blue" "🚀 Moving RF-Swift to ${INSTALL_DIR}..."
  if [ "$(id -u)" = "0" ]; then
    cp "${RFSWIFT_BIN}" "${INSTALL_DIR}/rfswift"
    chmod +x "${INSTALL_DIR}/rfswift"
  else
    sudo cp "${RFSWIFT_BIN}" "${INSTALL_DIR}/rfswift"
    sudo chmod +x "${INSTALL_DIR}/rfswift"
  fi
  
  # Clean up
  rm -rf "${TMP_DIR}"
  
  color_echo "green" "🎉 RF-Swift has been installed successfully!"
}

# Main function
main() {
  fun_welcome
  
  # Detect system early to help with dependency checks
  case "$(uname -s)" in
    Linux*)  OS="Linux" ;;
    Darwin*) OS="Darwin" ;;
    *)       color_echo "red" "Unsupported operating system: $(uname -s)"; exit 1 ;;
  esac
  
  install_docker
  get_latest_release
  detect_system
  download_files
  install_binary
  create_alias
  thank_you_message
  
  # Check if alias was created successfully
  if grep -q "alias rfswift=" "$(eval echo ~$(get_real_user))/.bash*" 2>/dev/null || \
     grep -q "alias rfswift=" "$(eval echo ~$(get_real_user))/.zshrc" 2>/dev/null || \
     grep -q "alias rfswift " "$(eval echo ~$(get_real_user))/.config/fish/config.fish" 2>/dev/null; then
    color_echo "cyan" "🚀 You can now run RF-Swift by simply typing: rfswift"
    color_echo "yellow" "   (You may need to restart your terminal or run 'source ~/.bashrc' or equivalent first)"
  else
    color_echo "cyan" "🚀 You can run RF-Swift by typing: ${INSTALL_DIR}/rfswift"
  fi
}

# Run the main function
main
