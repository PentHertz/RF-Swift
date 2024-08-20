#!/bin/bash

function leobodnarv1_cal_device() {
	goodecho "[+] Installing dependencies for Leobodnar v1 GPSDO"
	[ -d /rftools/calibration ] || mkdir -p /rftools/calibration
	cd /rftools/calibration
	installfromnet "apt-fast install -y libhidapi-libusb0 libhidapi-hidraw0"
	goodecho "[+] Cloning repository for Leobodnar v1 GPSDO"
	installfromnet "git clone https://github.com/hamarituc/lbgpsdo.git"
	cd /root/
}

function KCSDI_cal_device() {
	goodecho "[+] Installing dependencies for KCSDI"
	[ -d /rftools/calibration ] || mkdir -p /rftools/calibration
	cd /rftools/calibration
	mkdir Deepace
	cd Deepace
	installfromnet "apt-fast install -y libnss3-dev libfuse-dev"
	goodecho "[+] Downloading KCSDI from penthertz repo"
	installfromnet "wget https://github.com/PentHertz/rfswift_deepace_install/releases/download/nightly/KCSDI-v0.4.5-45-linux-x86_64.AppImage"
	chmod +x KCSDI-v0.4.5-45-linux-x86_64.AppImage
	ln -s KCSDI-v0.4.5-45-linux-x86_64.AppImage /usr/bin/KCSDI
}

function NanoVNASaver_cal_device() {
    local ARCH=$(uname -m)

    case "$ARCH" in
        x86_64|amd64)
            NanoVNASaver_cal_device_call
            ;;
        i?86)
            NanoVNASaver_cal_device_call
            ;;
        *)
            criticalecho "[-] Unsupported architecture: $ARCH. OpenBTS UMTS installation is not supported on this architecture."
            return 1
            ;;
    esac
}

function NanoVNASaver_cal_device_call() {
	goodecho "[+] Installing dependencies for NanoVNASaver"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	installfromnet "apt-fast install -y libxcb-cursor0 xcb"
	goodecho "[+] Cloning and installing NanoVNASaver"
	installfromnet "git clone https://github.com/NanoVNA-Saver/nanovna-saver.git"
	cd nanovna-saver
	installfromnet "pip3 install -U setuptools setuptools_scm wheel"
	installfromnet "pip3 install -r requirements.txt"
	python3 setup.py install
}

function NanoVNA_QT_cal_device() {
	goodecho "[+] Installing dependencies for NanoVNA-QT"
	[ -d /rftools/calibration ] || mkdir -p /rftools/calibration
	cd /rftools/calibration
	installfromnet "apt-fast install -y automake libtool make g++ libeigen3-dev libfftw3-dev libqt5charts5-dev"
	goodecho "[+] Cloning and installing NanoVNA-QT"
	installfromnet "git clone https://github.com/nanovna-v2/NanoVNA-QT.git"
	cd NanoVNA-QT
	autoreconf --install
	./configure
	make -j$(nproc)
	cd libxavna/xavna_mock_ui/
	qmake
 	make -j$(nproc)
 	cd ../..
 	cd vna_qt
	qmake
	make -j$(nproc)
}