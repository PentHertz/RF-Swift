#!/bin/bash

# Installing Go
[ -d thirdparty ] || mkdir thirdparty
cd thirdparty
wget https://go.dev/dl/go1.22.3.linux-amd64.tar.gz
sudo rm -rf /usr/local/go && sudo tar -C /usr/local -xzf go1.22.3.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin
go version

# Installing Cobra CLI
cd ../src
go install github.com/spf13/cobra-cli@latest
cobra-cli init
go build
go install
