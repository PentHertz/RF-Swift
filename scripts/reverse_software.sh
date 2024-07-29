#!/bin/bash

function kataistruct_soft_install() {
	goodecho "[+] Installing Katai Struct"
	[ -d /root/thirdparty ] || mkdir -p /root/thirdparty
	cd /root/thirdparty
	installfromnet "curl -LO https://github.com/kaitai-io/kaitai_struct_compiler/releases/download/0.10/kaitai-struct-compiler_0.10_all.deb"
	installfromnet "apt-fast install -y ./kaitai-struct-compiler_0.10_all.deb"
}

function unicorn_soft_install() {
	goodecho "[+] Cloning Unicorn Engine project"
	[ -d /root/thirdparty ] || mkdir -p /root/thirdparty
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
	[ -d /root/thirdparty ] || mkdir -p /root/thirdparty
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
	[ -d /root/thirdparty ] || mkdir -p /root/thirdparty
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

    ghidra_version="11.1.2"
	prog="ghidra_${ghidra_version}_PUBLIC_20240709"

	installfromnet "wget https://github.com/NationalSecurityAgency/ghidra/releases/download/Ghidra_${ghidra_version}_build/${prog}.zip"
	unzip "$prog"
	cd "ghidra_${ghidra_version}_PUBLIC"
	ln -s ghidraRun /usr/sbin/ghidraRun
	cd ..
	rm "$prog.zip"
}

function qiling_soft_install() {
	goodecho "[+] Installing Qiling's dependencies"
	installfromnet "apt-fast install -y ack antlr3 aria2 asciidoc autoconf automake autopoint binutils bison build-essential bzip2 ccache cmake cpio curl device-tree-compiler fastjar flex gawk gettext gcc-multilib g++-multilib git gperf haveged help2man intltool libc6-dev-i386 libelf-dev libglib2.0-dev libgmp3-dev libltdl-dev libmpc-dev libmpfr-dev libncurses5-dev libncursesw5-dev libreadline-dev libssl-dev libtool lrzsz mkisofs msmtp nano ninja-build p7zip p7zip-full patch pkgconf python2.7 python3 python3-pip libpython3-dev qemu-utils rsync scons squashfs-tools subversion swig texinfo uglifyjs upx-ucl unzip vim wget xmlto xxd zlib1g-dev"
	goodecho "[+] Cloning and installing Qiling"
	[ -d /root/thirdparty ] || mkdir -p /root/thirdparty
	cd /root/thirdparty
	git clone -b dev https://github.com/qilingframework/qiling.git
	cd qiling && git submodule update --init --recursive
	pip3 install .
}

### TODO: more More!
