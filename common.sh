#!/bin/bash

# This code is part of RF Switch by @Penthertz
# Author(s): Sébastien Dudek (@FlUxIuS)

# Stop the script if any command fails
set -euo pipefail

GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[0;33m'
NC='\033[0m' # No Color

check_xhost() {
    if ! command -v xhost &> /dev/null; then
        echo -e "${RED}xhost is not installed on this system.${NC}"
        if command -v pacman &> /dev/null; then
            echo -e "${YELLOW}Installing xorg-xhost using pacman...${NC}"
            sudo pacman -Syu --noconfirm xorg-xhost
        elif command -v apt &> /dev/null; then
            echo -e "${YELLOW}Installing x11-xserver-utils using apt...${NC}"
            sudo apt update
            sudo apt install -y x11-xserver-utils
        elif command -v yum &> /dev/null; then
            echo -e "${YELLOW}Installing xorg-x11-utils using yum...${NC}"
            sudo yum install -y xorg-x11-utils
        else
            echo -e "${RED}Unsupported package manager. Please install xhost manually.${NC}"
            exit 1
        fi
        echo -e "${GREEN}xhost installed successfully.${NC}"
    else
        echo -e "${GREEN}xhost is already installed. Moving on.${NC}"
    fi
}

check_pulseaudio() {
    if ! command -v pulseaudio &> /dev/null; then
        echo -e "${RED}PulseAudio is not installed on this system.${NC}"
        
        if [[ "$OSTYPE" == "darwin"* ]]; then
            echo -e "${YELLOW}Detected macOS. Checking for Homebrew...${NC}"
            if ! command -v brew &> /dev/null; then
                echo -e "${RED}Homebrew is not installed. Please install Homebrew first.${NC}"
                echo -e "${YELLOW}You can install Homebrew by running:${NC}"
                echo -e "${BLUE}/bin/bash -c \"\$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)\"${NC}"
                exit 1
            fi
            echo -e "${YELLOW}Installing PulseAudio using Homebrew...${NC}"
            brew install pulseaudio
        elif command -v pacman &> /dev/null; then
            echo -e "${YELLOW}Installing PulseAudio using pacman...${NC}"
            sudo pacman -Syu --noconfirm pulseaudio pulseaudio-alsa
        elif command -v apt &> /dev/null; then
            echo -e "${YELLOW}Installing PulseAudio using apt...${NC}"
            sudo apt update
            sudo apt install -y pulseaudio pulseaudio-utils
        elif command -v yum &> /dev/null; then
            echo -e "${YELLOW}Installing PulseAudio using yum...${NC}"
            sudo yum install -y pulseaudio
        else
            echo -e "${RED}Unsupported package manager. Please install PulseAudio manually.${NC}"
            exit 1
        fi
        
        echo -e "${GREEN}PulseAudio installed successfully.${NC}"
    else
        echo -e "${GREEN}PulseAudio is already installed. Moving on.${NC}"
    fi

    if [[ "$OSTYPE" == "darwin"* ]]; then
        echo -e "${YELLOW}Detected macOS. PulseAudio server will not be started.${NC}"
        return
    fi

    echo -e "${YELLOW}Starting PulseAudio...${NC}"
    pulseaudio --check &> /dev/null || pulseaudio --start
    echo -e "${GREEN}PulseAudio is running.${NC}"
}

check_curl() {
    if ! command -v curl &> /dev/null; then
        echo -e "${RED}curl is not installed on this system.${NC}"
        if [ "$(uname -s)" == "Darwin" ]; then
            echo -e "${YELLOW}Attempting to install curl on macOS using Homebrew...${NC}"
            if ! command -v brew &> /dev/null; then
                echo -e "${RED}Homebrew is not installed. Please install Homebrew first.${NC}"
                echo "Visit https://brew.sh/ for installation instructions."
                exit 1
            fi
            brew install curl
        elif [ "$(uname -s)" == "Linux" ]; then
            if command -v apt &> /dev/null; then
                echo -e "${YELLOW}Attempting to install cURL using apt...${NC}"
                sudo apt update
                sudo apt install -y curl
            elif command -v yum &> /dev/null; then
                echo -e "${YELLOW}Attempting to install cURL using yum...${NC}"
                sudo yum install -y curl
            elif command -v pacman &> /dev/null; then
                echo -e "${YELLOW}Attempting to install cURL using pacman...${NC}"
                sudo pacman -Syu curl
            else
                echo -e "${RED}Unable to detect package manager. Please install cURL manually.${NC}"
                exit 1
            fi
        else
            echo -e "${RED}Unsupported operating system. Please install cURL manually.${NC}"
            exit 1
        fi
        echo -e "${GREEN}curl installed successfully.${NC}"
    else
        echo -e "${GREEN}curl is already installed. Moving on.${NC}"
    fi
}

check_docker() {
    # Check if the system is Linux and not Darwin (macOS)
    if [ "$(uname -s)" == "Linux" ]; then
        echo -e "${YELLOW}Are you installing on a Steam Deck? (yes/no)${NC}"
        read -p "Choose an option: " steamdeck_install
        if [ "$steamdeck_install" == "yes" ]; then
            install_docker_steamdeck
            return
        fi
    fi

    # Check if Docker is installed
    if ! command -v docker &> /dev/null; then
        echo -e "${RED}Docker is not installed. Do you want to install it now? (yes/no)${NC}"
        read -p "Choose an option: " install_docker
        if [ "$install_docker" == "yes" ]; then
            install_docker_standard
        else
            echo -e "${RED}Docker is required to proceed. Exiting.${NC}"
            exit 1
        fi
    else
        echo -e "${GREEN}Docker is already installed. Moving on.${NC}"
        install_buildx
        install_docker_compose
    fi
}

install_docker_standard() {
    arch=$(uname -m)
    os=$(uname -s)
    distro=$(grep "^ID_LIKE=" /etc/os-release | cut -d= -f2 | tr -d '"')

    if [ "$os" == "Darwin" ]; then
        # macOS installation using Homebrew
        if ! command -v brew &> /dev/null; then
            echo -e "${RED}Homebrew is not installed. Please install Homebrew first.${NC}"
            echo "Visit https://brew.sh/ for installation instructions."
            exit 1
        fi
        echo -e "${YELLOW}Installing Docker using Homebrew...${NC}"
        brew install --cask docker
        echo -e "${GREEN}Docker installed successfully on macOS.${NC}"
        echo -e "${YELLOW}Please launch Docker from Applications to start the Docker daemon.${NC}"
    elif [ "$os" == "Linux" ]; then
        if [ "$distro" == "arch" ] || [ "$distro" == "archlinux" ]; then
            # Arch Linux installation using pacman
            echo -e "${YELLOW}Installing Docker for Arch Linux using pacman...${NC}"
            sudo pacman -Sy --noconfirm docker
            sudo systemctl start docker
            sudo systemctl enable docker
            echo -e "${GREEN}Docker installed successfully on Arch Linux.${NC}"
        elif command -v apt &> /dev/null && [ "$distro" == "debian" ] || [ "$distro" == "ubuntu" ]; then
            # Debian-based installation using apt
            echo -e "${YELLOW}Installing Docker for Debian-based systems using apt...${NC}"
            sudo apt update
            sudo apt install -y docker.io
            sudo systemctl start docker
            sudo systemctl enable docker
            echo -e "${GREEN}Docker installed successfully on Debian-based system.${NC}"
        elif [ "$arch" == "riscv64" ]; then
            # riscv64 installation using apt
            if command -v apt &> /dev/null; then
                echo -e "${YELLOW}Installing Docker for riscv64 using apt...${NC}"
                sudo apt update
                sudo apt install -y docker.io
                sudo systemctl start docker
                sudo systemctl enable docker
                echo -e "${GREEN}Docker installed successfully on riscv64.${NC}"
            else
                echo -e "${RED}apt is not available on this system. Unable to install Docker for riscv64.${NC}"
                exit 1
            fi
        else
            # Standard Linux installation
            echo -e "${YELLOW}Installing Docker for Linux...${NC}"
            sudo curl -fsSL "https://get.docker.com/" | sh
            sudo systemctl start docker
            sudo systemctl enable docker
            echo -e "${GREEN}Docker installed successfully.${NC}"
        fi
        install_buildx
        install_docker_compose
    else
        echo -e "${RED}Unsupported operating system: $os${NC}"
        exit 1
    fi
}

install_docker_steamdeck() {
    # Installation steps for Docker on Steam Deck
    echo -e "${YELLOW}[+] Disabling read-only mode on Steam Deck${NC}"
    sudo steamos-readonly disable

    echo -e "${YELLOW}[+] Initializing pacman keyring${NC}"
    sudo pacman-key --init
    sudo pacman-key --populate archlinux

    echo -e "${YELLOW}[+] Installing Docker${NC}"
    sudo pacman -Syu docker

    echo -e "${YELLOW}[+] Re-enabling read-only mode on Steam Deck${NC}"
    sudo steamos-readonly enable

    install_docker_compose_steamdeck
}

install_docker_compose_steamdeck() {
    echo -e "${YELLOW}[+] Installing Docker Compose v2 plugin${NC}"
    DOCKER_CONFIG=${DOCKER_CONFIG:-$HOME/.docker}
    mkdir -p $DOCKER_CONFIG/cli-plugins
    sudo curl -SL https://github.com/docker/compose/releases/download/v2.28.1/docker-compose-linux-x86_64 -o $DOCKER_CONFIG/cli-plugins/docker-compose
    chmod +x $DOCKER_CONFIG/cli-plugins/docker-compose

    echo -e "${YELLOW}[+] Adding 'deck' user to Docker user group${NC}"
    sudo usermod -a -G docker deck

    echo -e "${GREEN}Docker and Docker Compose v2 installed successfully on Steam Deck.${NC}"
}

install_buildx() {
    arch=$(uname -m)
    os=$(uname -s | tr '[:upper:]' '[:lower:]') # Convert OS to lowercase
    version="v0.19.2"

    # Map architecture to buildx naming convention
    case "$arch" in
        x86_64|amd64)
            arch="amd64";;
        arm64|aarch64)
            arch="arm64";;
        riscv64)
            arch="riscv64";;
        *)
            printf "${RED}Unsupported architecture: \"%s\" -> Unable to install Buildx${NC}\n" "$arch" >&2; exit 2;;
    esac

    # Check if Buildx is already installed
    if ! sudo docker buildx version &> /dev/null; then
        echo -e "${YELLOW}[+] Installing Docker Buildx${NC}"

        # Additional setup for Linux
        if [ "$os" = "linux" ]; then
            sudo docker run --privileged --rm tonistiigi/binfmt --install all
        fi

        # Create CLI plugins directory if it doesn't exist
        mkdir -p ~/.docker/cli-plugins/

        # Determine the Buildx binary URL based on OS and architecture
        buildx_url="https://github.com/docker/buildx/releases/download/${version}/buildx-${version}.${os}-${arch}"

        # Download the Buildx binary
        echo -e "${YELLOW}[+] Downloading Buildx from ${buildx_url}${NC}"
        sudo curl -sSL "$buildx_url" -o "${HOME}/.docker/cli-plugins/docker-buildx"

        # Make the binary executable
        sudo chmod +x "${HOME}/.docker/cli-plugins/docker-buildx"

        echo -e "${GREEN}Docker Buildx installed successfully.${NC}"
    else
        echo -e "${GREEN}Docker Buildx is already installed. Moving on.${NC}"
    fi
}

install_docker_compose() {
    arch=$(uname -m)
    os=$(uname -s | tr '[:upper:]' '[:lower:]') # Convert OS to lowercase
    version="v2.31.0"

    # Map architecture to Docker Compose naming convention
    case "$arch" in
        x86_64|amd64)
            arch="x86_64";;
        arm64|aarch64)
            arch="aarch64";;
        riscv64)
            arch="riscv64";;
        *)
            printf "${RED}Unsupported architecture: \"%s\" -> Unable to install Docker Compose${NC}\n" "$arch" >&2; exit 2;;
    esac

    # Check if Docker Compose is already installed
    if ! sudo docker compose version &> /dev/null; then
        echo -e "${YELLOW}[+] Installing Docker Compose v2${NC}"

        # Determine the Docker Compose binary URL based on OS and architecture
        compose_url="https://github.com/docker/compose/releases/download/${version}/docker-compose-${os}-${arch}"

        # Set the Docker CLI plugins directory
        DOCKER_CONFIG=${DOCKER_CONFIG:-$HOME/.docker}
        mkdir -p $DOCKER_CONFIG/cli-plugins

        # Download the Docker Compose binary
        echo -e "${YELLOW}[+] Downloading Docker Compose from ${compose_url}${NC}"
        sudo curl -sSL "$compose_url" -o "$DOCKER_CONFIG/cli-plugins/docker-compose"

        # Make the binary executable
        sudo chmod +x "$DOCKER_CONFIG/cli-plugins/docker-compose"

        echo -e "${GREEN}Docker Compose v2 installed successfully.${NC}"
    else
        echo -e "${GREEN}Docker Compose v2 is already installed. Moving on.${NC}"
    fi
}

install_go() {
    if command -v go &> /dev/null; then
        echo -e "${GREEN}golang is already installed and in PATH. Moving on.${NC}"
        return 0
    fi

    if [ -x "/usr/local/go/bin/go" ]; then
        echo -e "${GREEN}golang is already installed in /usr/local/go/bin. Moving on.${NC}"
        export PATH=$PATH:/usr/local/go/bin
        return 0
    fi

    [ -d thirdparty ] || mkdir thirdparty
    cd thirdparty
    arch=$(uname -m)
    os=$(uname -s | tr '[:upper:]' '[:lower:]') # Normalize OS name to lowercase
    prog=""
    version="1.23.3"

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
            printf "${RED}Unsupported architecture: \"%s\" -> Unable to install Go${NC}\n" "$arch" >&2; exit 2;;
    esac

    case "$os" in
        linux|darwin)
            prog="go${version}.${os}-${arch}.tar.gz";;
        *)
            printf "${RED}Unsupported OS: \"%s\" -> Unable to install Go${NC}\n" "$os" >&2; exit 2;;
    esac

    # Download and install Go
    echo -e "${YELLOW}[+] Downloading Go from https://go.dev/dl/${prog}${NC}"
    wget "https://go.dev/dl/${prog}"
    sudo rm -rf /usr/local/go && sudo tar -C /usr/local -xzf $prog
    export PATH=$PATH:/usr/local/go/bin
    cd ..
    rm -rf thirdparty
    echo -e "${GREEN}Go installed successfully.${NC}"
}

building_rfswift() {
    cd go/rfswift/
    go build .
    mv rfswift ../.. # moving compiled file to project's root
    cd ../..
    echo -e "${GREEN}RF Switch Go Project built successfully.${NC}"
}

build_docker_image() {
    # Prompt the user to choose the architecture(s)
    echo "Select the architecture(s) to build for:"
    echo "1) amd64"
    echo "2) arm64/v8"
    echo "3) riscv64"
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
            echo -e "${RED}Invalid option. Exiting.${NC}"
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

    echo -e "${YELLOW}[+] Building the Docker container for $PLATFORM${NC}"
    sudo docker buildx build --platform $PLATFORM -t $imagename -f $dockerfile ressourcesdir
}

pull_docker_image() {
    sudo ./rfswift images remote
    read -p "Enter the image tag to pull (default: penthertz/rfswift:latest): " pull_image
    pull_image=${pull_image:-penthertz/rfswift:latest}

    echo -e "${YELLOW}[+] Pulling the Docker image${NC}"
    sudo docker pull $pull_image
}

install_binary_alias() {
    read -p "Do you want to create an alias for the binary? (yes/no): " create_alias
    if [ "$create_alias" == "yes" ]; then
        read -p "Enter the alias name for the binary (default: rfswift): " alias_name
        alias_name=${alias_name:-rfswift}

        # Assuming the binary is in the root of the project
        BINARY_PATH=$(pwd)/rfswift

        if [ -f "$BINARY_PATH" ]; then
            echo -e "${YELLOW}[+] Installing alias '${alias_name}' for the binary${NC}"

            # Copy binary to /usr/local/bin for system-wide access
            echo -e "${YELLOW}[+] Copying binary to /usr/local/bin for system-wide access...${NC}"
            sudo cp "$BINARY_PATH" /usr/local/bin/
            sudo chmod +x /usr/local/bin/rfswift

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
                echo -e "${YELLOW}[+] Alias file $ALIAS_FILE does not exist. Creating it...${NC}"
                touch "$ALIAS_FILE"
            fi

            # Add the alias to the appropriate shell configuration file for the user
            echo "alias $alias_name='/usr/local/bin/rfswift'" >> "$ALIAS_FILE"

            # Provide instructions to apply changes
            if [ "$SHELL_NAME" = "zsh" ]; then
                echo -e "${YELLOW}Zsh configuration updated. Please restart your terminal or run 'exec zsh' to apply the changes.${NC}"
            elif [ "$SHELL_NAME" = "bash" ]; then
                # Source the Bash configuration file
                if [ -f "$ALIAS_FILE" ]; then
                    source "$ALIAS_FILE"
                fi
            else
                echo -e "${YELLOW}Please restart your terminal or source the ${ALIAS_FILE} manually to apply the alias.${NC}"
            fi

            echo -e "${GREEN}Alias '${alias_name}' installed successfully.${NC}"
        else
            echo -e "${RED}Binary not found at $BINARY_PATH. Make sure the binary is built correctly.${NC}"
            exit 1
        fi
    else
        echo -e "${GREEN}Skipping alias creation.${NC}"
    fi
}