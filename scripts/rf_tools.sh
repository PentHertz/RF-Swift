#!/bin/bash

# General

function kismet_soft_install() {
	goodecho "[+] Installing Kismet"
	installfromnet "wget -O - https://www.kismetwireless.net/repos/kismet-release.gpg.key --quiet | gpg --dearmor | sudo tee /usr/share/keyrings/kismet-archive-keyring.gpg >/dev/null"
	echo 'deb [signed-by=/usr/share/keyrings/kismet-archive-keyring.gpg] https://www.kismetwireless.net/repos/apt/release/jammy jammy main' | sudo tee /etc/apt/sources.list.d/kismet.list >/dev/null
	installfromnet "apt-fast update"
	installfromnet "apt-fast -y install kismet"
}

# Bluetooth Classic and LE
function blueztools_soft_install() {
	goodecho "[+] Installing bluez tools"
	installfromnet "apt-fast install -y bluez bluez-tools bluez-hcidump bluez-btsco bluez-obexd libbluetooth-dev"
}

function mirage_soft_install() {
	goodecho "[+] Installing bettercap dependencies"
	echo apt-fast keyboard-configuration/variant string "English (US)" | debconf-set-selections
	echo apt-fast keyboard-configuration/layout string "English (US)" | debconf-set-selections
	echo apt-fast console-setup/codeset47 string "Guess optimal character set" | debconf-set-selections
	echo apt-fast console-setup/charmap47 string "UTF-8" | debconf-set-selections
	installfromnet "apt-fast install -y libpcsclite-dev pcsc-tools kmod kbd"
	goodecho "[+] Installing Mirage"
	[ -d /root/thirdparty ] || mkdir -p /root/thirdparty
	cd /root/thirdparty
	installfromnet "git clone https://github.com/RCayre/mirage"
	cd mirage/
	python3 setup.py install
}

function bettercap_soft_install() {
	goodecho "[+] Installing bettercap dependencies"
	installfromnet "apt-fast install -y golang git build-essential libpcap-dev libusb-1.0-0-dev libnetfilter-queue-dev"
	goodecho "[+] Installing bettercap software"
	[ -d /rftools/bluetooth ] || mkdir -p /rftools/bluetooth
	cd /rftools/bluetooth
	installfromnet "git clone https://github.com/bettercap/bettercap.git"
	cd bettercap
	./build.sh
	make
}

function sniffle_soft_install() {
	goodecho "[+] Installing Sniffle with OpenDroneID decoder/encoder"
	[ -d /rftools/bluetooth ] || mkdir -p /rftools/bluetooth
	cd /rftools/bluetooth
	installfromnet "git clone https://github.com/bkerler/Sniffle.git"
	cd Sniffle/python_cli
	installfromnet "pip3 install -r requirements.txt"
}

function bluing_soft_install() {
	goodecho "[+] Installing bdaddr"
	[ -d /root/thirdparty ] || mkdir -p /root/thirdparty
	cd /root/thirdparty
	goodecho "[+] Installing Python3.10 for bluing"
	installfromnet "wget https://www.python.org/ftp/python/3.10.0/Python-3.10.0.tgz"
	tar -xvf Python-3.10.0.tgz
	cd Python-3.10.0
	./configure --enable-optimizations
	make -j $(nproc)
	sudo make altinstall
	[ -d /rftools/bluetooth ] || mkdir -p /rftools/bluetooth
	cd /rftools/bluetooth
	mkdir bluing
	installfromnet "apt-fast -y install libgirepository1.0-dev"
	python3.10 -m pip install --upgrade pip
	python3.10 -m pip install venv
	python3.10 -m vevn bluing
	source bluing/bin/activate
	python3.10 -m pip install dbus-python==1.2.18
	python3.10 -m pip install --no-dependencies bluing PyGObject docopt btsm btatt bluepy configobj btl2cap pkginfo xpycommon halo pyserial bthci btgatt log_symbols colorama spinners six termcolor
}

function bdaddr_soft_install() {
	goodecho "[+] Installing bluing"
	[ -d /rftools/bluetooth ] || mkdir /rftools/bluetooth
	cd /rftools/bluetooth
	installfromnet "git clone https://github.com/thxomas/bdaddr"
	cd bdaddr
	make
}

# RFID package
function proxmark3_soft_install() {
	goodecho "[+] Installing proxmark3 dependencies"
	installfromnet "apt-fast install -y --no-install-recommends git ca-certificates build-essential pkg-config libreadline-dev arm-none-eabi"
	installfromnet "apt-fast install -y  gcc-arm-none-eabi libnewlib-dev qtbase5-dev libbz2-dev liblz4-dev libbluetooth-dev libpython3-dev libssl-dev libgd-dev"
	goodecho "[+] Installing proxmark3"
	[ -d /rftools/rfid ] || mkdir -p /rftools/rfid
	cd /rftools/rfid
	installfromnet "git clone https://github.com/RfidResearchGroup/proxmark3.git"
	cd proxmark3/
	make clean && make -j$(nproc)
}

function libnfc_soft_install() {
	goodecho "[+] Installing libnfc dependencies"
	installfromnet "apt-fast install -y autoconf libtool libusb-dev libpcsclite-dev build-essential pcsc-tools"
	goodecho "[+] Installing libnfc"
	installfromnet "apt-fast install -y libnfc-dev libnfc-bin"
}

function mfoc_soft_install() {
	goodecho "[+] Installing mfoc"
	installfromnet "apt-fast install -y mfoc"
}

function mfcuk_soft_install() {
	goodecho "[+] Installing mfcuk"
	installfromnet "apt-fast install -y mfcuk"
}

function mfread_soft_install() {
	goodecho "[+] Installing mfread dependencies"
	installfromnet "pip3 install bitstring"
	installfromnet "apt-fast install -y  gcc-arm-none-eabi libnewlib-dev qtbase5-dev libbz2-dev liblz4-dev libbluetooth-dev libpython3-dev libssl-dev libgd-dev"
	goodecho "[+] Installing mfdread"
	[ -d /rftools/rfid ] || mkdir -p /rftools/rfid
	cd /rftools/rfid
	installfromnet "git clone https://github.com/zhovner/mfdread.git"
}

# Wi-Fi Package
function common_nettools() {
	installfromnet "apt-fast install -y iproute2"
	echo apt-fast macchanger/automatically_run  boolean false | debconf-set-selections
	installfromnet "apt-fast install -y -q macchanger"
	echo apt-fast wireshark-common/install-setuid boolean true | debconf-set-selections
	installfromnet "apt-fast install -y -q tshark"
}

function aircrack_soft_install() {
	goodecho "[+] Installing aircrack-ng"
	installfromnet "apt-fast install -y aircrack-ng"
}

function reaver_soft_install() {
	goodecho "[+] Installing reaver"
	installfromnet "apt-fast install -y reaver"
}

function bully_soft_install() {
	goodecho "[+] Installing bully"
	installfromnet "apt-fast install -y bully"
}

function pixiewps_soft_install() {
	goodecho "[+] Installing pixiewps"
	installfromnet "apt-fast install -y pixiewps"
}

function Pyrit_soft_install() {
	goodecho "[+] Installing Pyrit"
	installfromnet "pip3 install pyrit"
}

function eaphammer_soft_install() {
	goodecho "[+] Installing eaphammer"
	[ -d /root/thirdparty ] || mkdir -p /root/thirdparty
	cd /root/thirdparty
	installfromnet "git clone https://github.com/s0lst1c3/eaphammer.git"
	cd eaphammer/
	./ubuntu-unattended-setup
}

function airgeddon_soft_install() { # TODO: install all dependencies
	goodecho "[+] Installing airgeddon"
	[ -d /rftools/wifi ] || mkdir -p /rftools/wifi
	cd /rftools/wifi
	installfromnet "git clone https://github.com/v1s1t0r1sh3r3/airgeddon.git"
	cd airgeddon/
}

function wifite2_soft_install () {
	goodecho "[+] Installing wifite2"
	[ -d /rftools/wifi ] || mkdir -p /rftools/wifi
	cd /rftools/wifi
	installfromnet "git clone https://github.com/derv82/wifite2.git"
	cd wifite2/
}