#!/bin/bash

function common_sources_and_sinks() {
	goodecho "[+] Installing osmosdr OOT modules"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	installfromnet "git clone https://github.com/osmocom/gr-osmosdr.git"
	cd gr-osmosdr/ \
	&& mkdir build \
	&& cd build/ \
	&& cmake -DCMAKE_INSTALL_PREFIX=/usr ../ \
	&& make -j$(nproc); sudo make install
	cd ../..
}

function grgsm_grmod_install() {
	goodecho "[+] Installing osmocore"
	installfromnet "apt-fast install -y build-essential libtool libtalloc-dev libsctp-dev shtool autoconf automake git-core pkg-config make gcc gnutls-dev libusb-1.0-0-dev libmnl-dev"
	installfromnet "apt-fast install -y libosmocore libosmocore-dev"
	goodecho "[+] Cloning gr-gsm"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	installfromnet "git clone https://github.com/bkerler/gr-gsm.git"
	cd gr-gsm \
	&& mkdir build \
	&& cd build/ \
	&& cmake -DCMAKE_INSTALL_PREFIX=/usr ../ \
	&& make -j$(nproc); sudo make install
	cd ../..
}

function grlora_grmod_install () {
	goodecho "[+] Installing liquid-dsp"
	installfromnet "apt-fast install -y libliquid-dev libliquid2d"
	goodecho "[+] Cloning gr-lora"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	installfromnet "git clone https://github.com/rpp0/gr-lora.git"
	goodecho "[+] Building and installing gr-lora"
	cd gr-lora \
	&& mkdir build \
	&& cd build/ \
	&& cmake -DCMAKE_INSTALL_PREFIX=/usr ../ \
	&& make -j$(nproc); sudo make install
	cd ../..
}

function grlorasdr_grmod_install () {
	goodecho "[+] Cloning gr-lora_sdr"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	installfromnet "git clone https://github.com/tapparelj/gr-lora_sdr.git"
	goodecho "[+] Building and installing gr-lora_sdr"
	cd gr-lora_sdr \
	&& mkdir build \
	&& cd build/ \
	&& cmake -DCMAKE_INSTALL_PREFIX=/usr ../ \
	&& make -j$(nproc); sudo make install
	cd ../..
}

function grinspector_grmod_install () {
	installfromnet "apt-fast install -y libqwt-qt5-dev"
	goodecho "[+] Cloning gr-inspector"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	installfromnet "git clone https://github.com/gnuradio/gr-inspector.git"
	goodecho "[+] Building and installing gr-inspector"
	cd gr-inspector \
	&& mkdir build \
	&& cd build/ \
	&& cmake -DCMAKE_INSTALL_PREFIX=/usr ../ \
	&& make -j$(nproc); sudo make install
	cd ../..
}

function griridium_grmod_install () {
	goodecho "[+] Cloning gr-iridum"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	installfromnet "git clone https://github.com/muccc/gr-iridium.git"
	cd gr-iridium
	goodecho "[+] Building and installing gr-iridum"
	cmake -B build
	cmake --build build
	cmake --install build
	ldconfig
}

function gruaslink_grmod_install () { # TODO: Python2 to Python3 fixes to do
	goodecho "[+] Cloning gr-uaslink"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	installfromnet "git clone https://github.com/bkerler/gr-uaslink.git"
	goodecho "[+] Building and installing gr-uaslink"
	cd gr-uaslink \
	&& mkdir build \
	&& cd build/ \
	&& cmake -DCMAKE_INSTALL_PREFIX=/usr ../ \
	&& make -j$(nproc); sudo make install
	cd ../..
}

function grX10_grmod_install () {
	goodecho "[+] Cloning gr-X10"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	installfromnet "git clone https://github.com/cpoore1/gr-X10.git"
	goodecho "[+] Building and installing gr-X10"
	cd gr-X10 \
	&& mkdir build \
	&& cd build/ \
	&& cmake -DCMAKE_INSTALL_PREFIX=/usr ../ \
	&& make -j$(nproc); sudo make install
	cd ../..
}

function grgfdm_grmod_install () {
	goodecho "[+] Cloning gr-gfdm"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	installfromnet "git clone https://github.com/bkerler/gr-gfdm.git"
	goodecho "[+] Building and installing gr-gfdm"
	cd gr-gfdm \
	&& mkdir build \
	&& cd build/ \
	&& cmake -DCMAKE_INSTALL_PREFIX=/usr ../ \
	&& make -j$(nproc); sudo make install
	cd ../..
}

function graaronia_rtsa_grmod_install () {
	goodecho "[+] Installing gr-aaronia_rtsa dependencies"
	installfromnet "apt-fast install -y rapidjson-dev"
	goodecho "[+] Cloning and installing libspectranstream"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	installfromnet "git clone https://github.com/hb9fxq/libspectranstream.git"
	cd libspectranstream \
	&& mkdir build \
	&& cd build/ \
	&& cmake ../ \
	&& make -j$(nproc); sudo make install
	goodecho "[+] Cloning gr-aaronia_rtsa"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	installfromnet "git clone https://github.com/hb9fxq/gr-aaronia_rtsa.git"
	goodecho "[+] Building and installing gr-aaronia_rtsa"
	cd gr-aaronia_rtsa \
	&& mkdir build \
	&& cd build/ \
	&& cmake -DCMAKE_INSTALL_PREFIX=/usr ../ \
	&& make -j$(nproc); sudo make install
	cd ../..
}

function grccsds_move_rtsa_grmod_install () {
	goodecho "[+] Installing gr-ccsds_move dependencies"
	installfromnet "apt-fast install -y rapidjson-dev"
	goodecho "[+] Cloning gr-ccsds_move"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	installfromnet "git clone https://github.com/bkerler/gr-ccsds_move.git"
	goodecho "[+] Building and installing gr-ccsds_move"
	cd gr-ccsds_move \
	&& mkdir build \
	&& cd build/ \
	&& cmake -DCMAKE_INSTALL_PREFIX=/usr ../ \
	&& make -j$(nproc); sudo make install
	cd ../..
}

function grais_grmod_install () {
	goodecho "[+] Cloning gr-ais"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	installfromnet "git clone https://github.com/bkerler/gr-ais.git"
	goodecho "[+] Building and installing gr-ais"
	cd gr-ais \
	&& mkdir build \
	&& cd build/ \
	&& cmake -DCMAKE_INSTALL_PREFIX=/usr ../ \
	&& make -j$(nproc); sudo make install
	cd ../..
}

function grreveng_grmod_install () {
	goodecho "[+] Cloning gr-reveng"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	installfromnet "git clone https://github.com/paulgclark/gr-reveng.git"
	goodecho "[+] Building and installing gr-reveng"
	cd gr-reveng \
	&& mkdir build \
	&& cd build/ \
	&& cmake -DCMAKE_INSTALL_PREFIX=/usr ../ \
	&& make -j$(nproc); sudo make install
	cd ../..
}


function grpdu_utils_grmod_install () {
	goodecho "[+] Cloning gr-sandia_utils"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	installfromnet "git clone https://github.com/sandialabs/gr-pdu_utils.git"
	goodecho "[+] Building and installing gr-pdu_utils"
	cd gr-pdu_utils \
	&& mkdir build \
	&& cd build/ \
	&& cmake -DCMAKE_INSTALL_PREFIX=/usr ../ \
	&& make -j$(nproc); sudo make install
	cd ../..
}

function grsandia_utils_grmod_install () {
	goodecho "[+] Cloning gr-sandia_utils"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	installfromnet "git clone https://github.com/bkerler/gr-sandia_utils.git"
	goodecho "[+] Building and installing gr-sandia_utils"
	cd gr-sandia_utils \
	&& mkdir build \
	&& cd build/ \
	&& cmake -DCMAKE_INSTALL_PREFIX=/usr ../ \
	&& make -j$(nproc); sudo make install
	cd ../..
}

function grdvbs2_grmod_install () {
	goodecho "[+] Cloning gr-dvbs2"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	installfromnet "git clone https://github.com/bkerler/gr-dvbs2.git"
	goodecho "[+] Building and installing gr-dvbs2"
	cd gr-dvbs2 \
	&& mkdir build \
	&& cd build/ \
	&& cmake -DCMAKE_INSTALL_PREFIX=/usr ../ \
	&& make -j$(nproc); sudo make install
	cd ../..
}

function grtempest_grmod_install () {
	goodecho "[+] Cloning gr-tempest"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	installfromnet "git clone https://github.com/nash-pillai/gr-tempest.git"
	goodecho "[+] Building and installing gr-tempest"
	cd gr-tempest \
	&& mkdir build \
	&& cd build/ \
	&& cmake -DCMAKE_INSTALL_PREFIX=/usr ../ \
	&& make -j$(nproc); sudo make install
	cd ../..
}

function grfhss_utils_grmod_install () {
	goodecho "[+] Cloning gr-fhss_utils"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	installfromnet "git clone https://github.com/sandialabs/gr-fhss_utils.git"
	goodecho "[+] Building and installing gr-fhss_utils"
	cd gr-fhss_utils \
	&& mkdir build \
	&& cd build/ \
	&& cmake -DCMAKE_INSTALL_PREFIX=/usr ../ \
	&& make -j$(nproc); sudo make install
	cd ../..
}

function grtiming_utils_grmod_install () {
	goodecho "[+] Cloning gr-timing_utils"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	installfromnet "git clone https://github.com/sandialabs/gr-timing_utils.git"
	goodecho "[+] Building and installing gr-timing_utils"
	cd gr-timing_utils \
	&& mkdir build \
	&& cd build/ \
	&& cmake -DCMAKE_INSTALL_PREFIX=/usr ../ \
	&& make -j$(nproc); sudo make install
	cd ../..
}

function grdab_grmod_install () {
	goodecho "[+] Installing gr-dab dependencies"
	installfromnet "apt-fast install -y libfaad-dev"
	goodecho "[+] Cloning gr-dab"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	installfromnet "git clone https://github.com/bkerler/gr-dab.git"
	goodecho "[+] Building and installing gr-dab"
	cd gr-dab \
	&& mkdir build \
	&& cd build/ \
	&& cmake -DCMAKE_INSTALL_PREFIX=/usr ../ \
	&& make -j$(nproc); sudo make install
	cd ../..
}

function grdect2_grmod_install () {
	goodecho "[+] Cloning gr-dect2"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	installfromnet "git clone https://github.com/pavelyazev/gr-dect2.git"
	goodecho "[+] Building and installing gr-dect2"
	cd gr-dect2 \
	&& mkdir build \
	&& cd build/ \
	&& cmake -DCMAKE_INSTALL_PREFIX=/usr ../ \
	&& make -j$(nproc); sudo make install
	cd ../..
}

function grfoo_grmod_install () {
	goodecho "[+] Cloning gr-foo"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	installfromnet "git clone https://github.com/bastibl/gr-foo.git"
	goodecho "[+] Building and installing gr-foo"
	cd gr-foo \
	&& mkdir build \
	&& cd build/ \
	&& cmake -DCMAKE_INSTALL_PREFIX=/usr ../ \
	&& make -j$(nproc); sudo make install
	cd ../..
}

function grieee802-11_grmod_install () {
	goodecho "[+] Cloning gr-ieee802-11"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	installfromnet "git clone https://github.com/bastibl/gr-ieee802-11.git"
	goodecho "[+] Building and installing gr-ieee802-11"
	cd gr-ieee802-11 \
	&& mkdir build \
	&& cd build/ \
	&& cmake -DCMAKE_INSTALL_PREFIX=/usr ../ \
	&& make -j$(nproc); sudo make install
	cd ../..
}

function grieee802154_grmod_install () {
	goodecho "[+] Cloning gr-foo"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	installfromnet "git clone https://github.com/bastibl/gr-ieee802-15-4.git"
	goodecho "[+] Building and installing gr-ieee802-15-4"
	cd gr-ieee802-15-4 \
	&& mkdir build \
	&& cd build/ \
	&& cmake -DCMAKE_INSTALL_PREFIX=/usr ../ \
	&& make -j$(nproc); sudo make install
	cd ../..
}

function grrds_grmod_install () {
	goodecho "[+] Installing gr-rds dependencies"
	installfromnet "apt-fast install -y libboost-all-dev"
	goodecho "[+] Cloning gr-rds"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	installfromnet "git clone https://github.com/bastibl/gr-rds.git"
	goodecho "[+] Building and installing gr-rds"
	cd gr-rds \
	&& mkdir build \
	&& cd build/ \
	&& cmake -DCMAKE_INSTALL_PREFIX=/usr ../ \
	&& make -j$(nproc); sudo make install
	cd ../..
}

function grfosphor_grmod_install () {
	goodecho "[+] Installing dependencies"
	installfromnet "apt-fast install -y cmake xorg-dev libglu1-mesa-dev opencl-headers libwayland-dev libxkbcommon-dev"
	goodecho "[+] Cloning and building GLFW3"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	installfromnet "git clone https://github.com/glfw/glfw"
	cd glfw
	mkdir build
	cd build
	cmake ../ -DBUILD_SHARED_LIBS=true
	make -j$(nproc)
	make install
	ldconfig
	cd ..
	goodecho "[+] Cloning gr-fosphor"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	installfromnet "git clone https://github.com/osmocom/gr-fosphor.git"
	goodecho "[+] Building and installing gr-fosphor"
	cd gr-fosphor \
	&& mkdir build \
	&& cd build/ \
	&& cmake -DCMAKE_INSTALL_PREFIX=/usr ../ \
	&& make -j$(nproc); sudo make install
	cd ../..
}

function grdroineid_grmod_install () {
	goodecho "[+] Cloning turbofec"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	installfromnet "git clone https://github.com/zlinwei/turbofec.git"
	cd turbofec \
	&& mkdir build \
	&& cd build/ \
	&& cmake ../ \
	&& make -j$(nproc); sudo make install
	cd /root/thirdparty
	installfromnet "git clone https://github.com/d-bahr/CRCpp.git"
	cd CRCpp \
	&& mkdir build \
	&& cd build/ \
	&& cmake ../ \
	&& make -j$(nproc); sudo make install
	goodecho "[+] Cloning dji_droneid"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	installfromnet "git clone -b gr-droneid-update-3.10 https://github.com/proto17/dji_droneid.git"
	goodecho "[+] Building and installing dji_droneid"
	cd dji_droneid/gnuradio/gr-droneid \
	&& mkdir build \
	&& cd build/ \
	&& cmake -DCMAKE_INSTALL_PREFIX=/usr ../ \
	&& make -j$(nproc); sudo make install
}

function grsatellites_grmod_install () {
	goodecho "[+] Installing gr-satellites dependencies"
	installfromnet "apt-fast install -y liborc-0.4-dev"
	installfromnet "pip3 install --user --upgrade construct requests"
	goodecho "[+] Cloning gr-satellites"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	installfromnet "git clone https://github.com/daniestevez/gr-satellites.git"
	goodecho "[+] Building and installing gr-satellites"
	cd gr-satellites \
	&& mkdir build \
	&& cd build/ \
	&& cmake -DCMAKE_INSTALL_PREFIX=/usr ../ \
	&& make -j$(nproc); sudo make install
	cd ../..
}