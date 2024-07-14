#!/bin/bash

# This code is part of RF Switch by @Penthertz
# Author(s): Sébastien Dudek (@FlUxIuS)

# stop the script if any command fails
set -euo pipefail

GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[0;33m'
NC='\033[0m' # No Color

check_docker() {
    if ! command -v docker &> /dev/null
    then
        echo -e "${RED}Docker is not installed. Do you want to install it now? (yes/no)${NC}"
        read -p "Choose an option: " install_docker
        if [ "$install_docker" == "yes" ]; then
            # Install Docker for all Linux distributions
            curl -fsSL "https://get.docker.com/" | sh
            sudo systemctl start docker
            sudo systemctl enable docker
            echo -e "${GREEN}Docker installed successfully.${NC}"
        else
            echo -e "${RED}Docker is required to proceed. Exiting.${NC}"
            exit 1
        fi
    else
        echo -e "${GREEN}Docker is already installed. Moving on.${NC}"
    fi
}

install_go() {
    if command -v go &> /dev/null; then
        echo -e "${GREEN}golang is already installed and in PATH. Moving on.${NC}"
        return 0
    fi

    if [ -x "/usr/local/go/bin/go" ]; then
        echo -e "${GREEN}golang is already installed in /usr/local/go/bin. Moving on.${NC}"
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

echo -e "${YELLOW}[+] Checking Docker installation${NC}"
check_docker

echo -e "${YELLOW}[+] Installing Go${NC}"
install_go

echo -e "${YELLOW}[+] Building RF Switch Go Project${NC}"
building_rfswift

# Prompt the user if they want to build a Docker container, pull an image, or exit
echo "Do you want to build a Docker container, pull an existing image, or exit?"
echo "1) Build Docker container"
echo "2) Pull Docker image"
echo "3) Exit"
read -p "Choose an option (1, 2, or 3): " option

if [ "$option" -eq 1 ]; then
    # Set default values
    DEFAULT_IMAGE="myrfswift:latest"
    DEFAULT_DOCKERFILE="Dockerfile"

    # Prompt the user for input with default values
    read -p "Enter image tag value (default: $DEFAULT_IMAGE): " imagename
    read -p "Enter value for Dockerfile to use (default: $DEFAULT_DOCKERFILE): " dockerfile

    # Use default values if variables are empty
    imagename=${imagename:-$DEFAULT_IMAGE}
    dockerfile=${dockerfile:-$DEFAULT_DOCKERFILE}

    echo -e "${YELLOW}[+] Building the Docker container${NC}"
    sudo docker build . -t $imagename -f $dockerfile
elif [ "$option" -eq 2 ]; then
    read -p "Enter the image tag to pull (default: penthertz/rfswift:latest): " pull_image
    pull_image=${pull_image:-penthertz/rfswift:latest}

    echo -e "${YELLOW}[+] Pulling the Docker image${NC}"
    sudo docker pull $pull_image
elif [ "$option" -eq 3 ]; then
    echo -e "${GREEN}Exiting without building or pulling Docker images.${NC}"
    exit 0
else
    echo -e "${RED}Invalid option. Exiting.${NC}"
    exit 1
fi