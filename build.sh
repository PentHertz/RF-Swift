#!/bin/bash

# Quick and dirty building shell script which will evolve with more time make inside a Makefile

install_go() {
	[ -d thirdparty ] || mkdir thirdparty
	cd thirdparty
	wget https://go.dev/dl/go1.22.3.linux-amd64.tar.gz
	sudo rm -rf /usr/local/go && sudo tar -C /usr/local -xzf go1.22.3.linux-amd64.tar.gz
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