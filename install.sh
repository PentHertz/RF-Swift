#!/bin/bash

# This code is part of RF Switch by @Penthertz
# Author(s): Sébastien Dudek (@FlUxIuS)

# Stop the script if any command fails
set -euo pipefail

GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[0;33m'
NC='\033[0m' # No Color

check_docker() {
    echo -e "${YELLOW}Are you installing on a Steam Deck? (yes/no)${NC}"
    read -p "Choose an option: " steamdeck_install
    if [ "$steamdeck_install" == "yes" ]; then
        install_docker_steamdeck
    else
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
    fi
}

install_docker_standard() {
    # Install Docker for all Linux distributions
    curl -fsSL "https://get.docker.com/" | sh
    sudo systemctl start docker
    sudo systemctl enable docker
    echo -e "${GREEN}Docker installed successfully.${NC}"
    install_buildx
    install_docker_compose
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
    curl -SL https://github.com/docker/compose/releases/download/v2.28.1/docker-compose-linux-x86_64 -o $DOCKER_CONFIG/cli-plugins/docker-compose
    chmod +x $DOCKER_CONFIG/cli-plugins/docker-compose

    echo -e "${YELLOW}[+] Adding 'deck' user to Docker user group${NC}"
    sudo usermod -a -G docker deck

    echo -e "${GREEN}Docker and Docker Compose v2 installed successfully on Steam Deck.${NC}"
}

install_buildx() {
    arch=$(uname -m)
    version="v0.16.2"

    case "$arch" in
        x86_64|amd64)
            arch="amd64";;
        arm64|aarch64)
            arch="arm64";;
        *)
            printf "${RED}Unsupported architecture: \"%s\" -> Download or build Go instead${NC}\n" "$arch" >&2; exit 2;;
    esac
    if ! docker buildx version &> /dev/null; then
        echo -e "${YELLOW}[+] Installing Docker Buildx${NC}"
        docker run --privileged --rm tonistiigi/binfmt --install all
        mkdir -p ~/.docker/cli-plugins/
        curl -sSL https://github.com/docker/buildx/releases/download/${version}/buildx-${version}.linux-${arch} -o "${HOME}/.docker/cli-plugins/docker-buildx"
        chmod +x "${HOME}/.docker/cli-plugins/docker-buildx"
        echo -e "${GREEN}Docker Buildx installed successfully.${NC}"
    else
        echo -e "${GREEN}Docker Buildx is already installed. Moving on.${NC}"
    fi
}

install_docker_compose() {
    arch=$(uname -m)
    version="v2.29.2"

    case "$arch" in
        x86_64|amd64)
            arch="x86_64";;
        arm64|aarch64)
            arch="aarch64";;
        *)
            printf "${RED}Unsupported architecture: \"%s\" -> Download or build Go instead${NC}\n" "$arch" >&2; exit 2;;
    esac
    if ! docker compose version &> /dev/null; then
        echo -e "${YELLOW}[+] Installing Docker Compose v2${NC}"
        DOCKER_CONFIG=${DOCKER_CONFIG:-$HOME/.docker}
        mkdir -p $DOCKER_CONFIG/cli-plugins
        curl -SL https://github.com/docker/compose/releases/download/v2.29.2/docker-compose-linux-$(uname -m) -o $DOCKER_CONFIG/cli-plugins/docker-compose
        chmod +x $DOCKER_CONFIG/cli-plugins/docker-compose
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
    prog="" # default Go binary tar.gz
    version="1.22.5"

    case "$arch" in
        x86_64|amd64)
            prog="go${version}.linux-amd64.tar.gz";;
        i?86)
            prog="go${version}.linux-386.tar.gz";;
        arm64|aarch64)
            prog="go${version}.linux-arm64.tar.gz";;
        *)
            printf "${RED}Unsupported architecture: \"%s\" -> Download or build Go instead${NC}\n" "$arch" >&2; exit 2;;
    esac
    wget "https://go.dev/dl/$prog"
    sudo rm -rf /usr/local/go && sudo tar -C /usr/local -xzf $prog
    export PATH=$PATH:/usr/local/go/bin
    cd ..
    rm -R thirdparty
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

            # Detecting the user's shell
            SHELL_NAME=$(basename "$SHELL")

            case "$SHELL_NAME" in
                bash)
                    ALIAS_FILE="$HOME/.bashrc"
                    ;;
                zsh)
                    ALIAS_FILE="$HOME/.zshrc"
                    ;;
                *)
                    ALIAS_FILE="$HOME/.${SHELL_NAME}rc"
                    ;;
            esac

            echo "alias $alias_name='$BINARY_PATH'" >> "$ALIAS_FILE"

            # Source the appropriate shell config file, avoiding errors
            if [ -f "$ALIAS_FILE" ]; then
                if [ "$SHELL_NAME" = "zsh" ] || [ "$SHELL_NAME" = "bash" ]; then
                    source "$ALIAS_FILE"
                else
                    echo -e "${YELLOW}Please restart your terminal or source the ${ALIAS_FILE} manually to apply the alias.${NC}"
                fi
            fi

            echo -e "${GREEN}Alias '${alias_name}' installed successfully in $ALIAS_FILE.${NC}"
        else
            echo -e "${RED}Binary not found at $BINARY_PATH. Make sure the binary is built correctly.${NC}"
            exit 1
        fi
    else
        echo -e "${GREEN}Skipping alias creation.${NC}"
    fi
}

echo -e "${YELLOW}[+] Checking Docker installation${NC}"
check_docker

echo -e "${YELLOW}[+] Installing Go${NC}"
install_go

# Ensure Go binary is in the PATH for the current script session
export PATH=$PATH:/usr/local/go/bin

echo -e "${YELLOW}[+] Building RF Switch Go Project${NC}"
building_rfswift


# Ask the user if they want to create an alias after the installation
install_binary_alias

# Prompt the user if they want to build a Docker container, pull an image, or exit
echo "Do you want to build a Docker container, pull an existing image, or exit?"
echo "1) Build Docker container"
echo "2) Pull Docker image"
echo "3) Exit"
read -p "Choose an option (1, 2, or 3): " option

if [ "$option" -eq 1 ]; then
    build_docker_image
elif [ "$option" -eq 2 ]; then
    pull_docker_image
elif [ "$option" -eq 3 ]; then
    echo -e "${GREEN}Exiting without additional actions.${NC}"
    exit 0
else
    echo -e "${RED}Invalid option. Exiting.${NC}"
    exit 1
fi

echo -e "${GREEN}Installation and setup completed.${NC}"