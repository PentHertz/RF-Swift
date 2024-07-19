#!/bin/bash

### Part picket from Exegol project with love <3 (https://github.com/ThePorgs/Exegol)

export RED='\033[1;31m'
export BLUE='\033[1;34m'
export GREEN='\033[1;32m'
export NOCOLOR='\033[0m'

### Echo functions

function colorecho () {
    echo -e "${BLUE}$*${NOCOLOR}"
}

function criticalecho () {
    echo -e "${RED}$*${NOCOLOR}" 2>&1
    exit 1
}

function criticalecho-noexit () {
    echo -e "${RED}$*${NOCOLOR}" 2>&1
}

### </3 Love comes to an end

function goodecho () {
    echo -e "${GREEN}$*${NOCOLOR}" 2>&1
}

function installfromnet() {
    n=0
    until [ "$n" -ge 5 ]
    do
        colorecho "[Internet][Download] Try number: $n"
        $* && break 
        n=$((n+1)) 
        sleep 15
    done
}

function install_dependencies() {
    local dependencies=$1
    goodecho "[+] Installing dependencies: ${dependencies}"
    installfromnet "apt-fast install -y ${dependencies}"
}

function grclone_and_build() {
    local repo_url=$1
    local repo_dir=$2
    goodecho "[+] Cloning ${repo_dir}"
    [ -d /rftools/sdr/oot ] || mkdir -p /rftools/sdr/oot
    cd /rftools/sdr/oot
    installfromnet "git clone ${repo_url}"
    goodecho "[+] Building and installing ${repo_dir}"
    cd ${repo_dir}
    mkdir build
    cd build
    cmake -DCMAKE_INSTALL_PREFIX=/usr ../
    make -j$(nproc); sudo make install
    cd ..
    rm -R build
}