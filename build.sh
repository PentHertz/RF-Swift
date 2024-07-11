#!/bin/bash

# Quick and dirty building shell script which will evolve with more time make inside a Makefile

# stop the script if any command fails
set -euo pipefail

install_go() {
    go version && { echo "golang is already installed. moving on" && return 0 ; }

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
            printf 'Unsupported architecture: "%s" -> Download or build Go instead\n' "$arch" >&2; exit 2;;
    esac
    wget "https://go.dev/dl/$prog"
    sudo rm -rf /usr/local/go && sudo tar -C /usr/local -xzf $prog
    export PATH=$PATH:/usr/local/go/bin
    cd ..
    rm -R thirdparty
}

building_rfswift() {
    cd go/rfswift/
    go build .
    mv rfswift ../.. # moving compiled file to project's root
    cd ../..
}

echo "[+] Installing Go"
install_go

echo "[+] Building RF Switch Go Project"
building_rfswift

# Prompt the user if they want to build a Docker container or pull an image
echo "Do you want to build a Docker container or pull an existing image?"
echo "1) Build Docker container"
echo "2) Pull Docker image"
read -p "Choose an option (1 or 2): " option

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

    echo "[+] Building the Docker container"
    sudo docker build . -t $imagename -f $dockerfile
elif [ "$option" -eq 2 ]; then
    read -p "Enter the image tag to pull (default: penthertz/rfswift:latest): " pull_image
    pull_image=${pull_image:-penthertz/rfswift:latest}

    echo "[+] Pulling the Docker image"
    sudo ./rfswift pull -i $pull_image
else
    echo "Invalid option. Exiting."
    exit 1
fi