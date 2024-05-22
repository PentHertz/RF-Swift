#!/bin/bash

function leobodnarv1_cal_device() {
	goodecho "[+] Installing dependencies for Leobodnar v1 GPSDO"
	[ -d /rftools ] || mkdir /rftools
	cd /rftools
	installfromnet "apt-fast install -y libhidapi-libusb0 libhidapi-hidraw0"
	goodecho "[+] Cloning repository for Leobodnar v1 GPSDO"
	installfromnet "git clone https://github.com/hamarituc/lbgpsdo.git"
	cd /root/
}