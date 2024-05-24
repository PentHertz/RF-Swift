#!/bin/bash

# Quick and dirty building shell script which will evolve with more time make inside a Makefile

install_go() {
	[ -d thirdparty ] || mkdir thirdparty
	cd thirdparty
	arch=`uname -i`
	prog="" # default Go binary tar.gz
	case "$arch" in
  		x86_64|amd64)
    		prog="go1.22.3.linux-amd64.tar.gz";;
  		i?86)
    		prog="go1.22.3.linux-386.tar.gz";;
  		arm64|aarch64|unknown) # Let assume from now unknown is RPi 5 => TODO: fix
    		prog="go1.22.3.linux-arm64.tar.gz";;
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