#!/bin/bash

function kc908_sa_device() {
	goodecho "[+] Downloading bin from DEEPACE"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	installfromnet "wget https://deepace.net/wp-content/uploads/2024/04/KC908-GNURadio24.4.06.zip"
	unzip KC908-GNURadio24.4.06.zip
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
	[ -d /rftools ] || mkdir /rftools
	cd /rftools/
	installfromnet "wget https://signalhound.com/sigdownloads/Spike/Spike(Ubuntu22.04x64)_3_9_6.zip"
	unzip Spike\(Ubuntu22.04x64\)_3_9_6.zip
	cd Spike\(Ubuntu22.04x64\)_3_9_6/
	chmod +x setup.sh
	sh -c ./setup.sh
}