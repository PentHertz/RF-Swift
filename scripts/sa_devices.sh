#!/bin/bash

function kc908_sa_device() {
	goodecho "[+] Downloading bin from DEEPACE"
	[ -d /root/thirdparty ] || mkdir -p /root/thirdparty
	cd /root/thirdparty
	installfromnet "wget https://deepace.net/wp-content/uploads/2024/04/KC908-GNURadio24.4.06.zip"
	unzip KC908-GNURadio24.4.06.zip
	rm KC908-GNURadio24.4.06.zip
	cd KC908-GNURadio/lib
	INCLUDE_DIR="/usr/local/include/kcsdr"
	LIB_DIR="/usr/local/lib"
	mkdir ${INCLUDE_DIR}
	cp ./kcsdr.h ${INCLUDE_DIR}
	cp ./libkcsdr.so ${LIB_DIR}
	chmod 666 ${INCLUDE_DIR}/kcsdr.h
	chmod 666 ${LIB_DIR}/libkcsdr.so
	rm -f /usr/lib/libftd3xx.so
	cp ./linux/libftd3xx.so /usr/lib/
	cp ./linux/libftd3xx.so.0.5.21 /usr/lib/
	cp ./linux/51-ftd3xx.rules /etc/udev/rules.d/
	cd /root/thirdparty
	cd KC908-GNURadio/module3.9/gr-kc_sdr
	mkdir build \
	&& cd build/ \
	&& cmake -DCMAKE_INSTALL_PREFIX=/usr ../ \
	&& make -j$(nproc); sudo make install
	cd /root/
}

function signalhound_sa_device() {
	goodecho "[+] Downloading bin from SignalHound"
	[ -d /rftools/analysers ] || mkdir -p /rftools/analysers
	cd /rftools/analysers
	installfromnet "wget --no-check-certificate https://signalhound.com/sigdownloads/Spike/Spike(Ubuntu22.04x64)_3_9_6.zip"
	unzip Spike\(Ubuntu22.04x64\)_3_9_6.zip
	rm Spike\(Ubuntu22.04x64\)_3_9_6.zip
	cd Spike\(Ubuntu22.04x64\)_3_9_6/
	chmod +x setup.sh
	sh -c ./setup.sh
	ln -s Spike /usr/bin/Spike
}

function harogic_sa_device() {
	goodecho "[+] Downloading SAStudio4"
	[ -d /rftools/analysers ] || mkdir -p /rftools/analysers
	cd /rftools/analysers
	arch=`uname -i`
	prog=""
	case "$arch" in
  		x86_64|amd64)
    		prog="SAStudio4_x86_64_05_23_17_06";;
  		aarch64|unknown) # We asume unknwon would be RPi 5 for now...?
    		prog="SAStudio4_aarch64_05_22_17_41";;
  		*)
    		printf 'Unsupported architecture: "%s"!\n' "$arch" >&2; exit 2;;
	esac
	installfromnet "wget https://github.com/PentHertz/rfswift_harogic_install/releases/download/v05.23.17/$prog.zip"
	unzip "$prog"
	rm "$prog.zip"
	cd "$prog"
	sh -c ./install.sh
	case "$arch" in # quick fix for aarch64
  		aarch64|unknown) 
    		ln -s /usr/lib/aarch64-linux-gnu/libffi.so.8 /usr/lib/libffi.so.6;;
	esac
	ln -s /usr/local/bin/sastudio/.sastudio.sh /usr/sbin/sastudio
	colorecho "[+] Note: you'll have to put your calibration data after!"
}