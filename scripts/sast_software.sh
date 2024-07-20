#!/bin/env bash

# Adapted from (tested on ubuntu 22.04):
# https://github.com/20urc3/Talks/tree/main/leHack

set -euo pipefail


function LLVM_install() { # expects llvm version
    echo "[+] installing LLVM ${1}"

    wget -c https://apt.llvm.org/llvm.sh && chmod +x llvm.sh
    sudo ./llvm.sh "${1}"

    export LLVM_CONFIG=llvm-config-${1}
    echo "Defaults env_keep += \"${LLVM_CONFIG}\"" | sudo EDITOR='tee -a' visudo
}

function AFL_install() {
    echo "[+] installing AFL deps"

    sudo apt-get install -y gcc

    sudo apt-get install -y build-essential make python3-dev automake cmake git flex bison libglib2.0-dev libpixman-1-dev python3-setuptools python3-pip \
        gcc-$(gcc --version|head -n1|sed 's/\..*//'|sed 's/.* //')-plugin-dev libstdc++-$(gcc --version|head -n1|sed 's/\..*//'|sed 's/.* //')-dev \
        ninja-build pipx binutils-dev cppcheck

    pipx ensurepath

    pip install unicorn
    pipx install semgrep
    echo "[+] installing libAFL"

    curl --proto '=https' --tlsv1.2 https://sh.rustup.rs -sSf > install-rust.sh
    chmod +x install-rust.sh

    ./install-rust.sh -y

    . "$HOME/.cargo/env"

    cargo install cargo-make
    git clone https://github.com/AFLplusplus/LibAFL libafl --branch=main
    cd libafl ; cargo build --release ; cd ..

     echo "[+] installing AFL++"

    git clone https://github.com/AFLplusplus/AFLplusplus aflpp --branch=stable
    cd aflpp ; make && sudo make install ; cd -
}

function honggfuzz_install() {
    echo "[+] installing honggfuzz"

    sudo apt-get -y  install binutils-dev libunwind-dev libblocksruntime-dev
    git clone https://github.com/google/honggfuzz

    cd honggfuzz ; make ; cd ..
}

function clang_static_analyzer_install() {
    echo "[+] installing clang-static-analyzer"

    sudo apt-get install -y clang clang-tools
}
