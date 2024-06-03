#!/bin/bash

function kataistruct_soft_install() {
	goodecho "[+] Installing Katai Struct"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	installfromnet "curl -LO https://github.com/kaitai-io/kaitai_struct_compiler/releases/download/0.10/kaitai-struct-compiler_0.10_all.deb"
	installfromnet "apt-fast install -y ./kaitai-struct-compiler_0.10_all.deb"
}

function unicorn_soft_install() {
	goodecho "[+] Cloning Unicorn Engine project"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	installfromnet "git clone https://github.com/unicorn-engine/unicorn.git"
	cd unicorn
	mkdir build; cd build
	cmake .. -DCMAKE_BUILD_TYPE=Release
	make -j$(nproc)
	make install
	goodecho "[+] Installing Python bindings"
	installfromnet "pip3 install unicorn"
}

function keystone_soft_install() {
	goodecho "[+] Cloning Keystone Engine project"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	installfromnet "git clone https://github.com/keystone-engine/keystone.git"
	cd keystone
	mkdir build; cd build
	cmake .. -DCMAKE_BUILD_TYPE=Release
	make -j$(nproc)
	make install
	goodecho "[+] Installing Python bindings"
	installfromnet "pip3 install keystone-engine"
}

function radare2_soft_install() {
	goodecho "[+] Installing Radare"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	installfromnet "git clone https://github.com/radareorg/radare2"
	cd radare2 ; sys/install.sh
}

function binwalk_soft_install() {
	goodecho "[+] Installing Binwalk"
	installfromnet "apt-fast install -y binwalk"
}

function cutter_soft_install() { # TODO: fix installation
	goodecho "[+] Installing Cutter dependencies"
	installfromnet "apt-fast install -y build-essential cmake meson libzip-dev zlib1g-dev qt5-default libqt5svg5-dev qttools5-dev qttools5-dev-tools libkf5syntaxhighlighting-dev libgraphviz-dev libshiboken2-dev libpyside2-dev  qtdeclarative5-dev"
	goodecho "[+] Cloning Cutter"
	[ -d /reverse ] || mkdir /reverse
	cd /reverse
	installfromnet "git clone --recurse-submodules https://github.com/rizinorg/cutter"
	cd cutter
	mkdir build && cd build
	cmake ..
	cmake --build .
}

function ghidra_soft_install() {
	goodecho "[+] Installing Ghidra dependencies"
	installfromnet "apt-fast install -y openjdk-21-jdk"
	goodecho "[+] Downloading Ghidra"
	[ -d /reverse ] || mkdir /reverse
	cd /reverse
	prog="ghidra_11.0.3_PUBLIC_20240410"
	installfromnet "wget https://github.com/NationalSecurityAgency/ghidra/releases/download/Ghidra_11.0.3_build/$prog.zip"
	unzip "$prog"
	rm "$prog.zip"
}

### TODO: more More!