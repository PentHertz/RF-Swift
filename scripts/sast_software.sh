#!/bin/env bash

# Adapted from (tested on ubuntu 22.04):
# https://github.com/20urc3/Talks/tree/main/leHack

# set -euo pipefail // -> some scripts fails to install properly, and binaries are normally working. TODO: Need see why some install processes are failing with it.


function LLVM_install() { # expects llvm version
    LLVM_VERSION=17

    goodecho "[+] installing LLVM ${LLVM_VERSION}"

    [ -d /root/thirdparty ] || mkdir -p /root/thirdparty
    cd /root/thirdparty

    installfromnet "wget -c https://apt.llvm.org/llvm.sh"

    chmod +x llvm.sh
    ./llvm.sh ${LLVM_VERSION}

    export LLVM_CONFIG=llvm-config-${LLVM_VERSION}
    echo "Defaults env_keep += \"${LLVM_CONFIG}\"" | sudo EDITOR='tee -a' visudo
}

function semgrep_install() {
    goodecho "[+] installing AFL deps"
    installfromnet "pip3 install semgrep"
}

function cppcheck_install() {
    goodecho "[+] installing cppcheck"
    installfromnet "apt-fast install -y cppcheck"
}

function AFL_install() {
    goodecho "[+] installing AFL++"

    installfromnet "apt-fast install -y clang build-essential afl++"
}

function honggfuzz_install() {
    echo "[+] installing honggfuzz"

    installfromnet "apt-fast install -y binutils-dev libunwind-dev libblocksruntime-dev git"

    [ -d /root/thirdparty ] || mkdir -p /root/thirdparty
    cd /root/thirdparty

    git clone https://github.com/google/honggfuzz

    cd honggfuzz && make && make install
}

function clang_static_analyzer_install() {
    echo "[+] installing clang-static-analyzer"

    installfromnet "apt-fast install -y clang clang-tools"
}
