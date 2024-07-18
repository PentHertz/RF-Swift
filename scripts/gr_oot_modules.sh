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

function graistx_grmod_install () {
	goodecho "[+] Cloning gr-ais-tx"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	installfromnet "git clone -b maint-3.10 https://github.com/bkerler/ais.git"
	goodecho "[+] Building and installing gr-ais-tx"
	cd ais/gr-aistx \
	&& mkdir build \
	&& cd build/ \
	&& cmake -DCMAKE_INSTALL_PREFIX=/usr ../ \
	&& make -j$(nproc); sudo make install
	cd ../..
}

function grairmodes_grmod_install () {
	goodecho "[+] Cloning gr-air-modes"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	installfromnet "git clone -b gr3.9 https://github.com/bistromath/gr-air-modes.git"
	goodecho "[+] Building and installing gr-air-modes"
	cd gr-air-modes \
	&& mkdir build \
	&& cd build/ \
	&& cmake -DCMAKE_INSTALL_PREFIX=/usr ../ \
	&& make -j$(nproc); sudo make install
	cd ../..
}

function grj2497_grmod_install () {
	goodecho "[+] Cloning gr-j2497"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	installfromnet "git clone https://github.com/ainfosec/gr-j2497.git"
	goodecho "[+] Building and installing gr-j2497"
	cd gr-j2497 \
	&& mkdir build \
	&& cd build/ \
	&& cmake -DCMAKE_INSTALL_PREFIX=/usr ../ \
	&& make -j$(nproc); sudo make install
	cd ../..
}

function grzwavepoore_grmod_install () {
	goodecho "[+] Cloning gr-zwave_poore"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	installfromnet "git clone https://github.com/cpoore1/gr-zwave_poore.git"
	goodecho "[+] Building and installing gr-zwave_poore"
	cd gr-zwave_poore \
	&& mkdir build \
	&& cd build/ \
	&& cmake -DCMAKE_INSTALL_PREFIX=/usr ../ \
	&& make -j$(nproc); sudo make install
	cd ../..
}

function grmixalot_grmod_install () {
	goodecho "[+] Cloning gr-mixalot"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	installfromnet "git clone https://github.com/unsynchronized/gr-mixalot.git"
	goodecho "[+] Building and installing gr-mixalot"
	cd gr-mixalot \
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

function grtempest_grmod_install () { # Original gr-tempest mod
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

function deeptempest_grmod_install () { # extended gr-tempest with DL
	goodecho "[+] Cloning deep-tempest"
	[ -d /rftools/sdr ] || mkdir /rftools/sdr
	cd /rftools/sdr
	installfromnet "git clone https://github.com/PentHertz/deep-tempest.git"
	goodecho "[+] Building and installing deep-tempest"
	cd deep-tempest/gr-tempest \
	&& mkdir build \
	&& cd build/ \
	&& cmake -DCMAKE_INSTALL_PREFIX=/usr ../ \
	&& make -j$(nproc); sudo make install
	cd ../
	cd examples
	grcc *.grc
	mkdir -p /root/.grc_gnuradio
	cp *.block.yml /root/.grc_gnuradio
	cd ../..
	goodecho "[+] Installing requirements for deep-tempest"
	cd end-to-end/
	installfromnet "pip3 install -r requirement.txt"
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
	cd turbofec 
	autoreconf -i
	./configure
	make -j$(nproc); sudo make install
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

function gradsb_grmod_install () {
	goodecho "[+] Installing gr-adsb dependencies (especially for webserver)"
	installfromnet "pip3 install zmq flask flask-socketio gevent gevent-websocket"
	goodecho "[+] Installing gr-adsb"
	[ -d /rftools ] || mkdir /rftools
	cd /rftools
	installfromnet "git clone -b maint-3.10 https://github.com/mhostetter/gr-adsb"
	goodecho "[+] Building and installing gr-adsb"
	cd gr-adsb \
	&& mkdir build \
	&& cd build/ \
	&& cmake -DCMAKE_INSTALL_PREFIX=/usr ../ \
	&& make -j$(nproc); sudo make install
	cd ../..
}

function grkeyfob_grmod_install () {
	goodecho "[+] Cloning gr-keyfob"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	installfromnet "git clone https://github.com/bastibl/gr-keyfob.git"
	goodecho "[+] Building and installing gr-keyfob"
	cd gr-keyfob \
	&& mkdir build \
	&& cd build/ \
	&& cmake -DCMAKE_INSTALL_PREFIX=/usr ../ \
	&& make -j$(nproc); sudo make install
	cd ../..
}

function grradar_grmod_install () {
	goodecho "[+] Cloning gr-radar"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	installfromnet "git clone https://github.com/radioconda/gr-radar.git"
	goodecho "[+] Building and installing gr-radar"
	cd gr-radar \
	&& mkdir build \
	&& cd build/ \
	&& cmake -DCMAKE_INSTALL_PREFIX=/usr ../ \
	&& make -j$(nproc); sudo make install
	cd ../..
}

function grnordic_grmod_install () {
	goodecho "[+] Cloning gr-nordic"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	installfromnet "git clone https://github.com/bkerler/gr-nordic.git"
	goodecho "[+] Building and installing gr-radar"
	cd gr-nordic \
	&& mkdir build \
	&& cd build/ \
	&& cmake -DCMAKE_INSTALL_PREFIX=/usr ../ \
	&& make -j$(nproc); sudo make install
	cd ../..
}

function grpaint_grmod_install () {
	goodecho "[+] Cloning gr-paint"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	installfromnet "git clone https://github.com/drmpeg/gr-paint.git"
	goodecho "[+] Building and installing gr-paint"
	cd gr-paint \
	&& mkdir build \
	&& cd build/ \
	&& cmake -DCMAKE_INSTALL_PREFIX=/usr ../ \
	&& make -j$(nproc); sudo make install
	cd ../..
}

function gr_DCF77_Receiver_grmod_install () {
	goodecho "[+] Cloning gr_DCF77_Receiver"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	installfromnet "git clone https://github.com/henningM1r/gr_DCF77_Receiver.git"
	goodecho "[+] Building and installing gr_DCF77_Receiver"
	cd gr_DCF77_Receiver \
	&& mkdir build \
	&& cd build/ \
	&& cmake -DCMAKE_INSTALL_PREFIX=/usr ../ \
	&& make -j$(nproc); sudo make install
	cd ../..
}

function grbb60_Receiver_grmod_install () { # TODO: ask SH for ARM64 support
	goodecho "[+] Installing gr-bb60 dependencies"
	installfromnet "apt install -y libusb-1.0-0"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	installfromnet "wget https://ftdichip.com/wp-content/uploads/2022/07/libftd2xx-x86_64-1.4.27.tgz"
	cd tar xvfz libftd2xx-x86_64-1.4.27.tgz
	cd release/build
	cp libftd2xx.* /usr/local/lib
	chmod 0755 /usr/local/lib/libftd2xx.so.1.4.27
	ln -sf /usr/local/lib/libftd2xx.so.1.4.27 /usr/local/lib/libftd2xx.so
	cd ..
	cp ftd2xx.h  /usr/local/include
	cp WinTypes.h  /usr/local/include
	ldconfig -v
	installfromnet "wget https://signalhound.com/sigdownloads/SDK/signal_hound_sdk_06_24_24.zip"
	unzip signal_hound_sdk_06_24_24.zip
	cd "signal_hound_sdk/device_apis/bb_series/lib/linux/Ubuntu 18.04"
	cp libbb_api.* /usr/local/lib
	ldconfig -v -n /usr/local/lib
	ln -sf /usr/local/lib/libbb_api.so.5 /usr/local/lib/libbb_api.so
	goodecho "[+] Cloning gr-bb60"
	installfromnet "git clone https://github.com/SignalHound/gr-bb60.git"
	goodecho "[+] Building and installing gr-bb60"
	cd gr-bb60 \
	&& mkdir build \
	&& cd build/ \
	&& cmake -DCMAKE_INSTALL_PREFIX=/usr ../ \
	&& make -j$(nproc); sudo make install
	cd ../..
}