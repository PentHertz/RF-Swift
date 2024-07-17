#!/bin/bash

source common.sh

function ad_devices_install() {
	goodecho "[+] Installing AD libs and tools from package manager"
	installfromnet "apt-fast install -y libad9361-dev libiio-utils libiio-dev"
}

function uhd_devices_install() {
	goodecho "[+] Installing UHD's libs and tools from package manager"
	installfromnet "apt-fast install -y libuhd4.1.0 libuhd-dev uhd-host"
	goodecho "[+] Copying rules sets"
	cp /root/rules/uhd-usrp.rules  /etc/udev/rules.d/
	goodecho "[+] Downloading Hardware Driver firmware/FPGA"
    installfromnet "/usr/bin/uhd_images_downloader"
}

function check_neon() {
    if grep -q 'Features.*neon' /proc/cpuinfo; then
        return 0 # NEON is present
    else
        return 1 # NEON is not present
    fi
}

function uhd_devices_fromsource_install() {
	goodecho "[+] Installing UHD's dependencies"
	installfromnet "apt-fast install -y dpdk dpdk-dev autoconf automake build-essential ccache cmake cpufrequtils doxygen ethtool g++ git inetutils-tools libboost-all-dev libncurses5 libncurses5-dev libusb-1.0-0 libusb-1.0-0-dev libusb-dev python3-dev python3-mako python3-numpy python3-requests python3-scipy python3-setuptools \
python3-ruamel.yaml"
	goodecho "[+] Copying rules sets"
	cp /root/rules/uhd-usrp.rules  /etc/udev/rules.d/
	goodecho "[+] Cloning and compiling UHD"
	[ -d /root/thirdparty ] || mkdir -p /root/thirdparty
	cd /root/thirdparty
	installfromnet "git clone https://github.com/EttusResearch/uhd.git"
	cd uhd/host
	mkdir build
	cd build
	# Detect if the architecture is ARM
	ARCH=$(uname -m)

	if [[ "$ARCH" == arm* || "$ARCH" == aarch64 ]]; then
	    echo "Architecture is ARM."

	    if check_neon; then
	        echo "NEON extension is present."
	        cmake -DCMAKE_FIND_ROOT_PATH=/usr ..
	    else
	        echo "NEON extension is not present."
	        cmake -DCMAKE_FIND_ROOT_PATH=/usr -DNEON_SIMD_ENABLE=OFF ..
	    fi
	else
	    echo "Architecture is not ARM."
	    cmake -DCMAKE_FIND_ROOT_PATH=/usr ..
	fi
	make -j$(nproc)
	sudo make install
	sudo ldconfig
	goodecho "[+] Downloading Hardware Driver firmware/FPGA"
    	installfromnet "uhd_images_downloader"
}

function antsdr_uhd_devices_install() { # Is replacing original one for now
	goodecho "[+] Installing dependencies for ANTSDR UHD"
	installfromnet "apt-fast install -y autoconf automake build-essential ccache cmake cpufrequtils doxygen ethtool"
	installfromnet "apt-fast install -y g++ git inetutils-tools libboost-all-dev libncurses5 libncurses5-dev libusb-1.0-0 libusb-1.0-0-dev"
	installfromnet "apt-fast install -y python3-dev python3-mako python3-numpy python3-requests python3-scipy python3-setuptools"
	installfromnet "apt-fast install -y python3-ruamel.yaml"
	[ -d /root/thirdparty ] || mkdir -p /root/thirdparty
	cd /root/thirdparty
	installfromnet "git clone https://github.com/MicroPhase/antsdr_uhd.git"
	cd antsdr_uhd
	cd host/
	mkdir build
	cd build
	cmake ../
	make -j$(nproc)
	make install
	ldconfig
}

function nuand_devices_install() {
	goodecho "[+] Installing Nuand's libs and tools from package manager"
	installfromnet "add-apt-repository ppa:nuandllc/bladerf"
	installfromnet "apt-fast update"
	installfromnet "apt-fast install -y bladerf libbladerf-dev bladerf-firmware-fx3"
	goodecho "[+] Copying rules sets"
	cp /root/rules/88-nuand-bladerf1.rules.in /etc/udev/rules.d/
	cp /root/rules/88-nuand-bladerf2.rules.in /etc/udev/rules.d/
	cp /root/rules/88-nuand-bootloader.rules.in /etc/udev/rules.d/
}

function nuand_devices_fromsource_install() {
        goodecho "[+] Installing bladeRF dependencies"
	installfromnet "apt-fast install -y libusb-1.0-0-dev libusb-1.0-0 build-essential cmake libncurses5-dev libtecla1 libtecla-dev pkg-config git wget"
        goodecho "[+] Cloning, building and installing Nuand's repository"
	[ -d /root/thirdparty ] || mkdir -p /root/thirdparty
    cd /root/thirdparty
	installfromnet "git clone https://github.com/Nuand/bladeRF.git ./bladeRF"
	cd ./bladeRF
	mkdir build
	cd build
	cmake -DCMAKE_BUILD_TYPE=Release -DCMAKE_INSTALL_PREFIX=/usr/local -DINSTALL_UDEV_RULES=ON ../
	make && sudo make install && sudo ldconfig
	goodecho "[+] Copying rules sets"
        cp /root/rules/88-nuand-bladerf1.rules.in /etc/udev/rules.d/
        cp /root/rules/88-nuand-bladerf2.rules.in /etc/udev/rules.d/
        cp /root/rules/88-nuand-bootloader.rules.in /etc/udev/rules.d/
}

function hackrf_devices_install() {
	goodecho "[+] Installing hackRF's libs and tools from package manager"
	installfromnet "apt-fast install -y hackrf libhackrf-dev"
}

function airspy_devices_install() {
	goodecho "[+] Installing airspy from package manager"
	installfromnet "apt-fast install -y airspy libairspy-dev airspyhf libairspyhf-dev"
}

function limesdr_devices_install() {
	goodecho "[+] Installing LimeSDR's libs and tools from package manager"
	installfromnet "apt-fast install -y soapysdr-module-lms7 libsoapysdr-dev liblimesuite-dev limesuite limesuite-udev"
}

function install_soapy_modules() {
	goodecho "[+] Installing Soapy extra modules"
	installfromnet "apt-fast install -y libsoapysdr-dev soapysdr-module-osmosdr soapysdr-module-rtlsdr soapysdr-module-bladerf soapysdr-module-hackrf soapysdr-module-uhd soapysdr-module-mirisdr soapysdr-module-rfspace soapysdr-module-airspy"
}

function install_soapyPlutoSDR_modules() {
	goodecho "[+] Installing Soapy PlutoSDR module"
	installfromnet "apt-fast install -y libad9361-dev libiio-utils libiio-dev"
	[ -d /root/thirdparty ] || mkdir -p /root/thirdparty
	cd /root/thirdparty
	installfromnet "git clone https://github.com/pothosware/SoapyPlutoSDR"
	cd SoapyPlutoSDR
	mkdir build
	cd build
	cmake -DCMAKE_INSTALL_PREFIX=/usr ../
	make
	sudo make install
}

function rtlsdr_devices_install() {
	goodecho "[+] Installing RTL-SDR's libs and tools from package manager"
	installfromnet "apt-fast install -y librtlsdr-dev librtlsdr0 rtl-sdr"
}

function rtlsdrv4_devices_install() {
	goodecho "[+] Installing RTL-SDR v4's libs and tools from package manager"
	apt purge -y ^librtlsdr
	rm -rvf /usr/lib/librtlsdr* /usr/include/rtl-sdr* /usr/local/lib/librtlsdr* /usr/local/include/rtl-sdr* /usr/local/include/rtl_* /usr/local/bin/rtl_*
	installfromnet "apt-fast install -y libusb-1.0-0-dev git cmake pkg-config"
	[ -d /root/thirdparty ] || mkdir -p /root/thirdparty
	cd /root/thirdparty
	installfromnet "git clone https://github.com/rtlsdrblog/rtl-sdr-blog"
	cd rtl-sdr-blog
	mkdir build
	cd build
	cmake ../ -DINSTALL_UDEV_RULES=ON
	make
	sudo make install
	sudo cp ../rtl-sdr.rules /etc/udev/rules.d/
	sudo ldconfig
	cd /root
	rm -R /root/thirdparty
}

function osmofl2k_devices_install() {
	goodecho "[+] Installing osmo-fl2k dependencies"
	installfromnet "apt-fast install -y libusb-1.0-0-dev sox pv"
	goodecho "[+] Cloning and Installing osmo-fl2k"
	apt purge -y ^librtlsdr
	rm -rvf /usr/lib/librtlsdr* /usr/include/rtl-sdr* /usr/local/lib/librtlsdr* /usr/local/include/rtl-sdr* /usr/local/include/rtl_* /usr/local/bin/rtl_*
	installfromnet "apt-fast install -y libusb-1.0-0-dev git cmake pkg-config"
	[ -d /root/thirdparty ] || mkdir -p /root/thirdparty
	cd /root/thirdparty
	installfromnet "git clone https://gitea.osmocom.org/sdr/osmo-fl2k"
	mkdir osmo-fl2k/build
	cd osmo-fl2k/build
	cmake ../ -DINSTALL_UDEV_RULES=ON
	make -j 3
	sudo make install
	sudo ldconfig
	cd /root
	rm -R /root/thirdparty
	[ -d /rftools/sdr ] || mkdir -p /rftools/sdr
	cd /rftools/sdr
	goodecho "[+] Cloning a few examples"
	installfromnet "git clone https://github.com/steve-m/fl2k-examples.git"
}

function xtrx_devices_install() {
	goodecho "[+] Installing xtrx from package manager"
	installfromnet "apt-fast install -y libusb-1.0-0-dev cmake dkms python3 python3-pip gpsd gpsd-clients pps-tools libboost-all-dev git qtbase5-dev libqcustomplot-dev libqt5printsupport5 doxygen swig"
	installfromnet "pip3 install cheetah3"
	installfromnet "apt-fast install -y soapysdr-module-xtrx xtrx-dkms xtrx-fft libxtrxll0 libxtrxll-dev libxtrxll-dev libxtrx-dev libxtrxdsp-dev"
}

function funcube_devices_install() {
	goodecho "[+] Installing funcube from package manager"
	installfromnet "apt-fast install -y gr-funcube libgnuradio-funcube1.0.0 qthid-fcd-controller"
}