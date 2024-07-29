#!/bin/bash

function canutils_soft_install() {
	goodecho "[+] Installing can-utils"
	installfromnet "apt-fast -y install can-utils"
}

function cantact_soft_install() {
	goodecho "[+] Installing cantact dependencies"
	installfromnet "apt-fast -y install cargo"
	goodecho "[+] Installing cantact"
	installfromnet "cargo install cantact"
}

function caringcaribou_soft_install() {
	goodecho "[+] Cloning and installing caringcaribou"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	installfromnet "git clone https://github.com/CaringCaribou/caringcaribou.git"
	cd caringcaribou
	python3 setup.py install
}

function savvycan_soft_install() {
	goodecho "[+] Cloning and installing SavvyCAN"
	[ -d /automotive ] || mkdir /automotive
	cd /automotive
	installfromnet "git clone https://github.com/collin80/SavvyCAN.git"
	cd SavvyCAN
	qmake -makefile
	make -j$(nproc)
	ln -s SavvyCAN /usr/bin/SavvyCAN
}

function gallia_soft_install() {
	goodecho "[+] Installing Gallia"
	installfromnet "pip3 install gallia"
}

function v2ginjector_soft_install() {
	goodecho "[+] Installing V2G Injector"
	[ -d /automotive ] || mkdir /automotive
	cd /automotive
	installfromnet "git clone https://github.com/FlUxIuS/V2GInjector.git"
	cd V2GInjector
	chmod +x install.sh
	./install.sh
}

### TODO: more More!