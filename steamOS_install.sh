#!/bin/bash

# from: https://gist.githubusercontent.com/Sharrnah/aa4ba2880789d78d3d4e165aae112aae/raw/69d3e0adbb0accb7feaab09dcdd2c6c0703bd6f8/Install%2520Docker%2520-%2520Pacman.sh
# Author: Sharrnah

sudo steamos-readonly disable

sudo pacman-key --init
sudo pacman-key --populate archlinux
#sudo pacman-key --refresh-keys

# install docker package
echo -e "\rInstalling Docker..."
sudo pacman -Syu docker

sudo steamos-readonly enable

# install docker compose 2
echo -e "\rInstalling Docker compose v2 plugin..."
DOCKER_CONFIG=${DOCKER_CONFIG:-$HOME/.docker}
mkdir -p $DOCKER_CONFIG/cli-plugins
curl -SL https://github.com/docker/compose/releases/download/v2.28.1/docker-compose-linux-x86_64 -o $DOCKER_CONFIG/cli-plugins/docker-compose

chmod +x $DOCKER_CONFIG/cli-plugins/docker-compose

# add deck user do docker usergroup
sudo usermod -a -G docker deck

echo -e "\r\rFinshed"
