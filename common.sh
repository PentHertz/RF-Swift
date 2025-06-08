#!/bin/bash

# This code is part of RF Swift by @Penthertz
# Author(s): Sébastien Dudek (@FlUxIuS)

# Stop the script if any command fails
set -euo pipefail

GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[0;33m'
NC='\033[0m' # No Color

check_xhost() {
    if ! command -v xhost &> /dev/null; then
        echo -e "${RED}❌ xhost is not installed on this system. ❌${NC}"
        if command -v pacman &> /dev/null; then
            echo -e "${YELLOW}📦 Installing xorg-xhost using pacman... 📦${NC}"
            sudo pacman -Syu --noconfirm xorg-xhost
        elif command -v apt &> /dev/null; then
            echo -e "${YELLOW}📦 Installing x11-xserver-utils using apt... 📦${NC}"
            sudo apt update
            sudo apt install -y x11-xserver-utils
        elif command -v yum &> /dev/null; then
            echo -e "${YELLOW}📦 Installing xorg-x11-utils using yum... 📦${NC}"
            sudo yum install -y xorg-x11-utils
        else
            echo -e "${RED}❌ Unsupported package manager. Please install xhost manually. ❌${NC}"
            exit 1
        fi
        echo -e "${GREEN}✅ xhost installed successfully. ✅${NC}"
    else
        echo -e "${GREEN}✅ xhost is already installed. Moving on. ✅${NC}"
    fi
}

check_pulseaudio() {
    if ! command -v pulseaudio &> /dev/null; then
        echo -e "${RED}❌ PulseAudio is not installed on this system. ❌${NC}"
        
        if [[ "$OSTYPE" == "darwin"* ]]; then
            echo -e "${YELLOW}🍎 Detected macOS. Checking for Homebrew... 🍎${NC}"
            if ! command -v brew &> /dev/null; then
                echo -e "${RED}❌ Homebrew is not installed. Please install Homebrew first. ❌${NC}"
                echo -e "${YELLOW}ℹ️ You can install Homebrew by running: ℹ️${NC}"
                echo -e "${BLUE}🍺 /bin/bash -c \"\$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)\" 🍺${NC}"
                exit 1
            fi
            echo -e "${YELLOW}🔊 Installing PulseAudio using Homebrew... 🔊${NC}"
            brew install pulseaudio
        elif command -v pacman &> /dev/null; then
            echo -e "${YELLOW}🔊 Installing PulseAudio using pacman... 🔊${NC}"
            sudo pacman -Syu --noconfirm pulseaudio pulseaudio-alsa
        elif command -v apt &> /dev/null; then
            echo -e "${YELLOW}🔊 Installing PulseAudio using apt... 🔊${NC}"
            sudo apt update
            sudo apt install -y pulseaudio pulseaudio-utils
        elif command -v yum &> /dev/null; then
            echo -e "${YELLOW}🔊 Installing PulseAudio using yum... 🔊${NC}"
            sudo yum install -y pulseaudio
        else
            echo -e "${RED}❌ Unsupported package manager. Please install PulseAudio manually. ❌${NC}"
            exit 1
        fi
        
        echo -e "${GREEN}✅ PulseAudio installed successfully. ✅${NC}"
    else
        echo -e "${GREEN}✅ PulseAudio is already installed. Moving on. ✅${NC}"
    fi

    if [[ "$OSTYPE" == "darwin"* ]]; then
        echo -e "${YELLOW}🍎 Detected macOS. PulseAudio server will not be started. 🍎${NC}"
        return
    fi

    echo -e "${YELLOW}🎵 Starting PulseAudio... 🎵${NC}"
    pulseaudio --check &> /dev/null || pulseaudio --start
    echo -e "${GREEN}🎧 PulseAudio is running. 🎧${NC}"
}

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
            if command -v apt &> /dev/null; then
                echo -e "${YELLOW}🐧 Attempting to install cURL using apt... 🐧${NC}"
                sudo apt update
                sudo apt install -y curl
            elif command -v yum &> /dev/null; then
                echo -e "${YELLOW}🐧 Attempting to install cURL using yum... 🐧${NC}"
                sudo yum install -y curl
            elif command -v pacman &> /dev/null; then
                echo -e "${YELLOW}🐧 Attempting to install cURL using pacman... 🐧${NC}"
                sudo pacman -Syu curl
            else
                echo -e "${RED}❌ Unable to detect package manager. Please install cURL manually. ❌${NC}"
                exit 1
            fi
        else
            echo -e "${RED}❌ Unsupported operating system. Please install cURL manually. ❌${NC}"
            exit 1
        fi
        echo -e "${GREEN}✅ curl installed successfully. ✅${NC}"
    else
        echo -e "${GREEN}✅ curl is already installed. Moving on. ✅${NC}"
    fi
}

check_docker() {
    # Check if this is a Steam Deck installation (Linux only)
    if [ "$(uname -s)" == "Linux" ]; then
        echo -e "${YELLOW}🎮 Are you installing on a Steam Deck? (yes/no) 🎮${NC}"
        read -p "Choose an option: " steamdeck_install
        if [ "$steamdeck_install" == "yes" ]; then
            install_docker_steamdeck
            return
        fi
    fi
    
    # Check if Docker is installed
    if ! command -v docker &> /dev/null; then
        echo -e "${RED}🐳 Docker is not installed. Do you want to install it now? (yes/no) 🐳${NC}"
        read -p "Choose an option: " install_docker
        if [ "$install_docker" == "yes" ]; then
            install_docker_standard
        else
            echo -e "${RED}❌ Docker is required to proceed. Exiting. ❌${NC}"
            exit 1
        fi
    else
        echo -e "${GREEN}✅ Docker is already installed. Moving on. ✅${NC}"
        # This part only runs in the full check_docker function
        if [ "${FUNCNAME[0]}" == "check_docker" ]; then
            install_buildx
            install_docker_compose
        fi
    fi
}

# Create an alias for the user-only version
check_docker_user_only() {
    check_docker
}

install_docker_standard() {
    arch=$(uname -m)
    os=$(uname -s)
    
    if [ "$os" == "Darwin" ]; then
        # macOS installation using Homebrew
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
        # Make sure os-release exists
        if [ ! -f /etc/os-release ]; then
            echo -e "${YELLOW}🐧 Cannot determine Linux distribution. Using standard Docker installation. 🐧${NC}"
            sudo curl -fsSL "https://get.docker.com/" | sh
            sudo systemctl start docker
            sudo systemctl enable docker
            echo -e "${GREEN}✅ Docker installed successfully. ✅${NC}"
            install_buildx
            install_docker_compose
            return
        fi
        
        distro=$(grep "^ID=" /etc/os-release | cut -d= -f2 | tr -d '"')
        
        if [ "$distro" == "arch" ] || [ "$distro" == "archlinux" ]; then
            # Arch Linux installation using pacman
            echo -e "${YELLOW}🏹 Installing Docker for Arch Linux using pacman... 🏹${NC}"
            sudo pacman -Sy --noconfirm docker
            sudo systemctl start docker
            sudo systemctl enable docker
            echo -e "${GREEN}✅ Docker installed successfully on Arch Linux. ✅${NC}"
        elif command -v apt &> /dev/null && [ "$distro" == "debian" ] || [ "$distro" == "ubuntu" ] || [ "$distro" == "kali" ]; then
            # Debian-based installation using apt
            echo -e "${YELLOW}🐧 Installing Docker for Debian-based systems using apt... 🐧${NC}"
            sudo apt update
            sudo apt install -y docker.io
            sudo systemctl start docker
            sudo systemctl enable docker
            echo -e "${GREEN}✅ Docker installed successfully on Debian-based system. ✅${NC}"
        elif [ "$arch" == "riscv64" ]; then
            # riscv64 installation using apt
            if command -v apt &> /dev/null; then
                echo -e "${YELLOW}🔧 Installing Docker for riscv64 using apt... 🔧${NC}"
                sudo apt update
                sudo apt install -y docker.io
                sudo systemctl start docker
                sudo systemctl enable docker
                echo -e "${GREEN}✅ Docker installed successfully on riscv64. ✅${NC}"
            else
                echo -e "${RED}❌ apt is not available on this system. Unable to install Docker for riscv64. ❌${NC}"
                exit 1
            fi
        else
            # Standard Linux installation
            echo -e "${YELLOW}🐧 Installing Docker for Linux... 🐧${NC}"
            sudo curl -fsSL "https://get.docker.com/" | sh
            sudo systemctl start docker
            sudo systemctl enable docker
            echo -e "${GREEN}✅ Docker installed successfully. ✅${NC}"
        fi
        install_buildx
        install_docker_compose
    else
        echo -e "${RED}❌ Unsupported operating system: $os ❌${NC}"
        exit 1
    fi
}

install_docker_steamdeck() {
    # Installation steps for Docker on Steam Deck
    echo -e "${YELLOW}[+] 🎮 Disabling read-only mode on Steam Deck 🎮${NC}"
    sudo steamos-readonly disable

    echo -e "${YELLOW}[+] 🔑 Initializing pacman keyring 🔑${NC}"
    sudo pacman-key --init
    sudo pacman-key --populate archlinux

    echo -e "${YELLOW}[+] 🐳 Installing Docker 🐳${NC}"
    sudo pacman -Syu docker

    echo -e "${YELLOW}[+] 🔒 Re-enabling read-only mode on Steam Deck 🔒${NC}"
    sudo steamos-readonly enable

    install_docker_compose_steamdeck
}

install_docker_compose_steamdeck() {
    echo -e "${YELLOW}[+] 🧩 Installing Docker Compose v2 plugin 🧩${NC}"
    DOCKER_CONFIG=${DOCKER_CONFIG:-$HOME/.docker}
    mkdir -p $DOCKER_CONFIG/cli-plugins
    sudo curl -SL https://github.com/docker/compose/releases/download/v2.36.0/docker-compose-linux-x86_64 -o $DOCKER_CONFIG/cli-plugins/docker-compose
    chmod +x $DOCKER_CONFIG/cli-plugins/docker-compose

    echo -e "${YELLOW}[+] 👥 Adding 'deck' user to Docker user group 👥${NC}"
    sudo usermod -a -G docker deck

    echo -e "${GREEN}✅ Docker and Docker Compose v2 installed successfully on Steam Deck. ✅${NC}"
}

install_buildx() {
    arch=$(uname -m)
    os=$(uname -s | tr '[:upper:]' '[:lower:]') # Convert OS to lowercase
    version="v0.24.0"

    # Map architecture to buildx naming convention
    case "$arch" in
        x86_64|amd64)
            arch="amd64";;
        arm64|aarch64)
            arch="arm64";;
        riscv64)
            arch="riscv64";;
        *)
            printf "${RED}❌ Unsupported architecture: \"%s\" -> Unable to install Buildx ❌${NC}\n" "$arch" >&2; exit 2;;
    esac

    # Check if Buildx is already installed
    if ! sudo docker buildx version &> /dev/null; then
        echo -e "${YELLOW}[+] 🏗️ Installing Docker Buildx 🏗️${NC}"

        # Additional setup for Linux
        if [ "$os" = "linux" ]; then
            sudo docker run --privileged --rm tonistiigi/binfmt --install all
        fi

        # Create CLI plugins directory if it doesn't exist
        mkdir -p ~/.docker/cli-plugins/

        # Determine the Buildx binary URL based on OS and architecture
        buildx_url="https://github.com/docker/buildx/releases/download/${version}/buildx-${version}.${os}-${arch}"

        # Download the Buildx binary
        echo -e "${YELLOW}[+] 📥 Downloading Buildx from ${buildx_url} 📥${NC}"
        sudo curl -sSL "$buildx_url" -o "${HOME}/.docker/cli-plugins/docker-buildx"

        # Make the binary executable
        sudo chmod +x "${HOME}/.docker/cli-plugins/docker-buildx"

        echo -e "${GREEN}✅ Docker Buildx installed successfully. ✅${NC}"
    else
        echo -e "${GREEN}✅ Docker Buildx is already installed. Moving on. ✅${NC}"
    fi
}

install_docker_compose() {
    arch=$(uname -m)
    os=$(uname -s | tr '[:upper:]' '[:lower:]') # Convert OS to lowercase
    version="v2.37.0"

    # Map architecture to Docker Compose naming convention
    case "$arch" in
        x86_64|amd64)
            arch="x86_64";;
        arm64|aarch64)
            arch="aarch64";;
        riscv64)
            arch="riscv64";;
        *)
            printf "${RED}❌ Unsupported architecture: \"%s\" -> Unable to install Docker Compose ❌${NC}\n" "$arch" >&2; exit 2;;
    esac

    # Check if Docker Compose is already installed
    if ! sudo docker compose version &> /dev/null; then
        echo -e "${YELLOW}[+] 🧩 Installing Docker Compose v2 🧩${NC}"

        # Determine the Docker Compose binary URL based on OS and architecture
        compose_url="https://github.com/docker/compose/releases/download/${version}/docker-compose-${os}-${arch}"

        # Set the Docker CLI plugins directory
        DOCKER_CONFIG=${DOCKER_CONFIG:-$HOME/.docker}
        mkdir -p $DOCKER_CONFIG/cli-plugins

        # Download the Docker Compose binary
        echo -e "${YELLOW}[+] 📥 Downloading Docker Compose from ${compose_url} 📥${NC}"
        sudo curl -sSL "$compose_url" -o "$DOCKER_CONFIG/cli-plugins/docker-compose"

        # Make the binary executable
        sudo chmod +x "$DOCKER_CONFIG/cli-plugins/docker-compose"

        echo -e "${GREEN}✅ Docker Compose v2 installed successfully. ✅${NC}"
    else
        echo -e "${GREEN}✅ Docker Compose v2 is already installed. Moving on. ✅${NC}"
    fi
}

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

    [ -d thirdparty ] || mkdir thirdparty
    cd thirdparty
    arch=$(uname -m)
    os=$(uname -s | tr '[:upper:]' '[:lower:]') # Normalize OS name to lowercase
    prog=""
    version="1.24.4"

    # Map architecture and OS to Go binary tar.gz naming convention
    case "$arch" in
        x86_64|amd64)
            arch="amd64";;
        i?86)
            arch="386";;
        arm64|aarch64)
            arch="arm64";;
        riscv64)
            arch="riscv64";;
        *)
            printf "${RED}❌ Unsupported architecture: \"%s\" -> Unable to install Go ❌${NC}\n" "$arch" >&2; exit 2;;
    esac

    case "$os" in
        linux|darwin)
            prog="go${version}.${os}-${arch}.tar.gz";;
        *)
            printf "${RED}❌ Unsupported OS: \"%s\" -> Unable to install Go ❌${NC}\n" "$os" >&2; exit 2;;
    esac

    # Download and install Go
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
    mv rfswift ../.. # moving compiled file to project's root
    cd ../..
    echo -e "${GREEN}✅ RF Swift Go Project built successfully. ✅${NC}"
}

build_docker_image() {
    # Prompt the user to choose the architecture(s)
    echo -e "${YELLOW}🏗️ Select the architecture(s) to build for: 🏗️${NC}"
    echo "1) amd64 💻"
    echo "2) arm64/v8 📱"
    echo "3) riscv64 🔬"
    read -p "Choose an option (1, 2, or 3): " arch_option

    case "$arch_option" in
        1)
            PLATFORM="linux/amd64"
            ;;
        2)
            PLATFORM="linux/arm64/v8"
            ;;
        3)
            PLATFORM="linux/riscv64"
            ;;
        *)
            echo -e "${RED}❌ Invalid option. Exiting. ❌${NC}"
            exit 1
            ;;
    esac

    # Set default values
    DEFAULT_IMAGE="myrfswift:latest"
    DEFAULT_DOCKERFILE="Dockerfile"

    # Prompt the user for input with default values
    read -p "Enter you ressources directory (where configs, and scripts are placed): " ressourcesdir
    read -p "Enter image tag value (default: $DEFAULT_IMAGE): " imagename
    read -p "Enter value for Dockerfile to use (default: $DEFAULT_DOCKERFILE): " dockerfile

    # Use default values if variables are empty
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

install_binary_alias() {
    # First, ask where to install the binary
    echo -e "${YELLOW}📦 Where would you like to install the rfswift binary? 📦${NC}"
    echo -e "1) /usr/local/bin (requires sudo privileges) 🔐"
    echo -e "2) $HOME/.rfswift/bin/ (user-only installation) 👤"
    read -p "Choose an option (1 or 2): " install_location

    # Set the binary installation path based on user's choice
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
        
        # Create the directory if it doesn't exist
        mkdir -p "$INSTALL_DIR"
    fi

    # Copy the binary to the installation directory
    SOURCE_BINARY=$(pwd)/rfswift
    if [ -f "$SOURCE_BINARY" ]; then
        echo -e "${YELLOW}[+] 📋 Copying binary to $INSTALL_DIR 📋${NC}"
        $SUDO_CMD cp "$SOURCE_BINARY" "$BINARY_PATH"
        $SUDO_CMD chmod +x "$BINARY_PATH"
    else
        echo -e "${RED}❌ Binary not found at $SOURCE_BINARY. Make sure the binary is built correctly. ❌${NC}"
        exit 1
    fi

    # Ask if user wants to create an alias
    read -p "Do you want to create an alias for the binary? (yes/no): " create_alias
    if [ "$create_alias" == "yes" ]; then
        read -p "Enter the alias name for the binary (default: rfswift): " alias_name
        alias_name=${alias_name:-rfswift}
        
        # Detect the current user and home directory
        if [ -n "${SUDO_USER-}" ]; then
            CURRENT_USER="$SUDO_USER"
            HOME_DIR=$(eval echo "~$SUDO_USER")
        else
            CURRENT_USER=$(whoami)
            HOME_DIR=$HOME
        fi
        
        # Detect the shell for the current user
        SHELL_NAME=$(basename "$SHELL")
        
        # Choose the alias file based on the detected shell
        case "$SHELL_NAME" in
            bash)
                if [[ "$OSTYPE" == "darwin"* ]]; then
                    ALIAS_FILE="$HOME_DIR/.bash_profile"  # macOS
                else
                    ALIAS_FILE="$HOME_DIR/.bashrc"        # Linux
                fi
                ;;
            zsh)
                ALIAS_FILE="$HOME_DIR/.zshrc"
                ;;
            *)
                ALIAS_FILE="$HOME_DIR/.${SHELL_NAME}rc"
                ;;
        esac
        
        # Create the alias file if it doesn't exist
        if [[ ! -f "$ALIAS_FILE" ]]; then
            echo -e "${YELLOW}[+] 📝 Alias file $ALIAS_FILE does not exist. Creating it... 🆕${NC}"
            touch "$ALIAS_FILE"
        fi
        
        # Check if the alias already exists in the config file
        ALIAS_EXISTS=false
        ALIAS_NEEDS_UPDATE=false
        if [ -f "$ALIAS_FILE" ]; then
            # Extract the current path if the alias already exists
            EXISTING_ALIAS=$(grep "^alias $alias_name=" "$ALIAS_FILE" 2>/dev/null)
            if [ -n "$EXISTING_ALIAS" ]; then
                ALIAS_EXISTS=true
                # Extract the path from the existing alias
                EXISTING_PATH=$(echo "$EXISTING_ALIAS" | sed -E "s/^alias $alias_name='?([^']*)'?$/\1/")
                if [ "$EXISTING_PATH" != "$BINARY_PATH" ]; then
                    ALIAS_NEEDS_UPDATE=true
                    echo -e "${YELLOW}[!] ⚠️ Alias '$alias_name' already exists but points to a different path: ⚠️${NC}"
                    echo -e "${YELLOW}    Current: $EXISTING_PATH${NC}"
                    echo -e "${YELLOW}    New: $BINARY_PATH${NC}"
                    read -p "Do you want to update the alias to the new path? (yes/no): " update_alias
                    if [ "$update_alias" == "yes" ]; then
                        # Remove the existing alias line
                        sed -i.bak "/^alias $alias_name=/d" "$ALIAS_FILE"
                        # Add the new alias
                        echo "alias $alias_name='$BINARY_PATH'" >> "$ALIAS_FILE"
                        echo -e "${GREEN}✅ Alias '$alias_name' updated successfully. ✅${NC}"
                    else
                        echo -e "${GREEN}👍 Keeping existing alias configuration. 👍${NC}"
                    fi
                else
                    echo -e "${GREEN}✅ Alias '$alias_name' already exists with the correct path. ✅${NC}"
                fi
            fi
        fi
        
        # Only add the alias if it doesn't exist and doesn't need an update
        if [ "$ALIAS_EXISTS" = false ] && [ "$ALIAS_NEEDS_UPDATE" = false ]; then
            # Add the alias to the appropriate shell configuration file for the user
            echo "alias $alias_name='$BINARY_PATH'" >> "$ALIAS_FILE"
            echo -e "${GREEN}✅ Alias '$alias_name' installed successfully! ✅${NC}"
        fi
        
        # Provide instructions to apply changes
        if [ "$SHELL_NAME" = "zsh" ]; then
            echo -e "${YELLOW}🔄 Zsh configuration updated. Please restart your terminal or run 'exec zsh' to apply the changes. 🔄${NC}"
        elif [ "$SHELL_NAME" = "bash" ]; then
            # Source the Bash configuration file
            if [ -f "$ALIAS_FILE" ]; then
                echo -e "${YELLOW}🔄 Bash configuration updated. Please run 'source $ALIAS_FILE' to apply the changes. 🔄${NC}"
            fi
        else
            echo -e "${YELLOW}🔄 Please restart your terminal or source the ${ALIAS_FILE} manually to apply the alias. 🔄${NC}"
        fi
        
        # If installed to user directory, add path to PATH if needed
        if [ "$install_location" == "2" ]; then
            # Check if the directory is already in PATH
            if [[ ":$PATH:" != *":$INSTALL_DIR:"* ]]; then
                echo -e "${YELLOW}[+] 🔀 Adding $INSTALL_DIR to your PATH 🔀${NC}"
                echo "export PATH=\$PATH:$INSTALL_DIR" >> "$ALIAS_FILE"
                echo -e "${GREEN}✅ PATH updated successfully. Please restart your terminal or source your shell config file. ✅${NC}"
            fi
        fi
    else
        echo -e "${GREEN}⏭️ Skipping alias creation. ⏭️${NC}"
        
        # If user-only installation and no alias, still add to PATH if needed
        if [ "$install_location" == "2" ]; then
            # Detect the shell configuration file
            if [[ "$OSTYPE" == "darwin"* ]]; then
                [ -f "$HOME/.bash_profile" ] && RC_FILE="$HOME/.bash_profile" || RC_FILE="$HOME/.profile"
                [ -f "$HOME/.zshrc" ] && [ "$SHELL" = *"zsh"* ] && RC_FILE="$HOME/.zshrc"
            else
                [ -f "$HOME/.bashrc" ] && RC_FILE="$HOME/.bashrc" || RC_FILE="$HOME/.profile"
                [ -f "$HOME/.zshrc" ] && [ "$SHELL" = *"zsh"* ] && RC_FILE="$HOME/.zshrc"
            fi
            
            # Check if the directory is already in PATH
            if [[ ":$PATH:" != *":$INSTALL_DIR:"* ]]; then
                echo -e "${YELLOW}[+] 🔀 Would you like to add $INSTALL_DIR to your PATH? (yes/no) 🔀${NC}"
                read -p "Choose an option: " add_to_path
                if [ "$add_to_path" == "yes" ]; then
                    echo "export PATH=\$PATH:$INSTALL_DIR" >> "$RC_FILE"
                    echo -e "${GREEN}✅ PATH updated in $RC_FILE. Please restart your terminal or run 'source $RC_FILE'. ✅${NC}"
                else
                    echo -e "${YELLOW}ℹ️ Note: You'll need to run $BINARY_PATH using its full path. ℹ️${NC}"
                fi
            fi
        fi
    fi
    
    echo -e "${GREEN}🎉 Installation complete! You can now use rfswift. 🎉${NC}"
}

check_config_file() {
    # Determine config file location based on OS
    if [[ "$OSTYPE" == "darwin"* ]]; then
        CONFIG_DIR="$HOME/Library/Application Support/rfswift"
    else
        CONFIG_DIR="$HOME/.config/rfswift"
    fi
    CONFIG_FILE="$CONFIG_DIR/config.ini"
    
    echo -e "${YELLOW}🔍 Checking configuration file at: $CONFIG_FILE 🔍${NC}"
    
    # Check if config file exists
    if [ ! -f "$CONFIG_FILE" ]; then
        echo -e "${YELLOW}📝 Config file not found at $CONFIG_FILE 📝${NC}"
        echo -e "${GREEN}✨ A new config file will be created on first run ;) ✨${NC}"
        return 0
    fi
    
    # Define required sections and keys - without using declare -A which is not supported in older bash
    GENERAL_KEYS="imagename repotag"
    CONTAINER_KEYS="shell bindings network exposedports portbindings x11forward xdisplay extrahost extraenv devices privileged caps seccomp cgroups"
    AUDIO_KEYS="pulse_server"
    
    missing_fields=0
    current_section=""
    
    # For debugging
    echo -e "${YELLOW}🔎 Scanning config file for keys... 🔎${NC}"
    
    # Read config file line by line
    while IFS= read -r line || [ -n "$line" ]; do
        # Trim leading/trailing whitespace
        line=$(echo "$line" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')
        
        # Skip empty lines and comments
        if [[ -z "$line" || "$line" == \#* ]]; then
            continue
        fi
        
        # Check if line is a section header
        if [[ "$line" =~ ^\[([a-zA-Z0-9_]+)\]$ ]]; then
            current_section="${BASH_REMATCH[1]}"
            echo -e "${YELLOW}📂 Found section: [$current_section] 📂${NC}"
            continue
        fi
        
        # Check if line contains a key (regardless of value)
        if [[ "$line" =~ ^([a-zA-Z0-9_]+)[[:space:]]*= ]]; then
            key="${BASH_REMATCH[1]}"
            echo -e "${GREEN}🔑 Found key: $key in section [$current_section] 🔑${NC}"
            
            # Remove the key from the required keys list based on section
            if [[ "$current_section" == "general" ]]; then
                GENERAL_KEYS=$(echo "$GENERAL_KEYS" | sed -E "s/(^| )$key( |$)/ /g" | tr -s ' ' | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')
            elif [[ "$current_section" == "container" ]]; then
                CONTAINER_KEYS=$(echo "$CONTAINER_KEYS" | sed -E "s/(^| )$key( |$)/ /g" | tr -s ' ' | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')
            elif [[ "$current_section" == "audio" ]]; then
                AUDIO_KEYS=$(echo "$AUDIO_KEYS" | sed -E "s/(^| )$key( |$)/ /g" | tr -s ' ' | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')
            fi
        fi
    done < "$CONFIG_FILE"
    
    # Debug: show remaining required keys after parsing
    echo -e "${YELLOW}📋 Remaining required keys in [general]: ${GENERAL_KEYS} 📋${NC}"
    echo -e "${YELLOW}📋 Remaining required keys in [container]: ${CONTAINER_KEYS} 📋${NC}"
    echo -e "${YELLOW}📋 Remaining required keys in [audio]: ${AUDIO_KEYS} 📋${NC}"
    
    # Check for missing fields in each section
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
    
    # Add option to show the config file content for debugging
    if [ "$1" = "--debug" ]; then
        echo -e "${YELLOW}🔍 === Config File Content ==== 🔍${NC}"
        cat "$CONFIG_FILE"
        echo -e "${YELLOW}🔍 ========================== 🔍${NC}"
    fi
}

display_rainbow_logo_animated() {
    # Define an array of colors for rainbow effect
    colors=(
        '\033[1;31m' # Red
        '\033[1;33m' # Orange/Yellow
        '\033[1;32m' # Green
        '\033[1;36m' # Cyan
        '\033[1;34m' # Blue
        '\033[1;35m' # Purple
    )
    NC='\033[0m' # No Color
    
    # The logo text as an array of lines
    logo=(
        "   888~-_   888~~        ,d88~~\\                ,e,   88~\\   d8   "
        "   888   \\  888___       8888    Y88b    e    /  \"  *888*_ *d88*_ "
        "   888    | 888          'Y88b    Y88b  d8b  /  888  888    888   "
        "   888   /  888           'Y88b,   Y888/Y88b/   888  888    888   "
        "   888_-~   888             8888    Y8/  Y8/    888  888    888   "
        "   888 ~-_  888          \\__88P'     Y    Y     888  888    \"88_/"
    )
    
    # Clear the screen for better presentation
    clear
    
    # First, print each line with its own color
    for i in {0..5}; do
        echo -e "${colors[$i]}${logo[$i]}${NC}"
        sleep 0.1  # Small delay between lines
    done
    
    sleep 0.5  # Pause before the animation
    
    # Now animate by cycling through colors
    if [ -t 1 ]; then  # Only run animation if in an interactive terminal
        for cycle in {1..3}; do  # Run the cycle 3 times
            # Move cursor back up 6 lines to the start of the logo
            for i in {1..6}; do
                echo -en "\033[1A"
            done
            
            # Print each line with the next color in the cycle
            for i in {0..5}; do
                color_index=$(( (i + cycle) % 6 ))
                echo -e "${colors[$color_index]}${logo[$i]}${NC}"
            done
            
            sleep 0.3  # Wait before the next cycle
        done
    fi
    
    # Add a tagline
    echo -e "\n${colors[5]}🔥 RF Swift by @Penthertz - Radio Frequency Swiss Army Knife 🔥${NC}\n"
    
    # Add a slight delay before continuing
    sleep 0.5
}
