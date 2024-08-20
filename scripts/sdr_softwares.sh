#!/bin/bash

function gnuradio_soft_install() {
	goodecho "[+] GNU Radio tools"
	installfromnet "apt-fast install -y gnuradio gnuradio-dev"
}

function sdrangel_soft_install() {
	goodecho "[+] Installing dependencies"
	installfromnet "apt-fast update"
	installfromnet "apt-fast install -y git cmake g++ pkg-config autoconf automake libtool libfftw3-dev libusb-1.0-0-dev libusb-dev libhidapi-dev libopengl-dev"
	installfromnet "apt-fast install -y qtbase5-dev qtchooser libqt5multimedia5-plugins qtmultimedia5-dev libqt5websockets5-dev"
	installfromnet "apt-fast install -y qttools5-dev qttools5-dev-tools libqt5opengl5-dev libqt5quick5 libqt5charts5-dev"
	installfromnet "apt-fast install -y qml-module-qtlocation  qml-module-qtpositioning qml-module-qtquick-window2"
	installfromnet "apt-fast install -y qml-module-qtquick-dialogs qml-module-qtquick-controls qml-module-qtquick-controls2 qml-module-qtquick-layouts"
	installfromnet "apt-fast install -y libqt5serialport5-dev qtdeclarative5-dev qtpositioning5-dev qtlocation5-dev libqt5texttospeech5-dev"
	installfromnet "apt-fast install -y qtwebengine5-dev qtbase5-private-dev libqt5gamepad5-dev libqt5svg5-dev"
	installfromnet "apt-fast install -y libfaad-dev zlib1g-dev libboost-all-dev libasound2-dev pulseaudio libopencv-dev libxml2-dev bison flex"
	installfromnet "apt-fast install -y ffmpeg libavcodec-dev libavformat-dev libopus-dev doxygen graphviz"
	installfromnet "apt-fast install -y libhamlib4 libgl1-mesa-glx qtspeech5-speechd-plugin gstreamer1.0-libav libairspy0"

	goodecho "[+] Downloading and unpacking SDR Angel"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	installfromnet "wget https://github.com/f4exb/sdrangel/releases/download/v7.21.3/sdrangel-2700-master.tar.gz"
	tar xvzf sdrangel-2700-master.tar.gz
	cd sdrangel-2700-master
	dpkg -i sdrangel_7.21.3-1_amd64.deb
	cd /root
}

function sdrangel_soft_fromsource_install() {
	goodecho "[+] Installing dependencies"
	installfromnet "apt-fast update"
	installfromnet "apt-fast install -y libsndfile-dev git cmake g++ pkg-config autoconf automake libtool libfftw3-dev libusb-1.0-0-dev libusb-dev libhidapi-dev libopengl-dev qtbase5-dev qtchooser libqt5multimedia5-plugins qtmultimedia5-dev libqt5websockets5-dev qttools5-dev qttools5-dev-tools libqt5opengl5-dev libqt5quick5 libqt5charts5-dev qml-module-qtlocation qml-module-qtpositioning qml-module-qtquick-window2 qml-module-qtquick-dialogs qml-module-qtquick-controls qml-module-qtquick-controls2 qml-module-qtquick-layouts libqt5serialport5-dev qtdeclarative5-dev qtpositioning5-dev qtlocation5-dev libqt5texttospeech5-dev qtwebengine5-dev qtbase5-private-dev libqt5gamepad5-dev libqt5svg5-dev libfaad-dev zlib1g-dev libboost-all-dev libasound2-dev pulseaudio libopencv-dev libxml2-dev bison flex ffmpeg libavcodec-dev libavformat-dev libopus-dev doxygen graphviz"
	goodecho "[+] APT"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	installfromnet "git clone https://github.com/srcejon/aptdec.git"
	cd aptdec
	git checkout libaptdec
	git submodule update --init --recursive
	mkdir build; cd build
	cmake -Wno-dev -DCMAKE_INSTALL_PREFIX=/opt/install/aptdec ..
	make -j $(nproc) install

	goodecho "[+] CM265cc"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	installfromnet "git clone https://github.com/f4exb/cm256cc.git"
	cd cm256cc
	git reset --hard 6f4a51802f5f302577d6d270a9fc0cb7a1ee28ef
	mkdir build; cd build
	cmake -Wno-dev -DCMAKE_INSTALL_PREFIX=/opt/install/cm256cc ..
	make -j $(nproc) install

	goodecho "[+] LibDAB"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	installfromnet "git clone https://github.com/srcejon/dab-cmdline"
	cd dab-cmdline/library
	git checkout msvc
	mkdir build; cd build
	cmake -Wno-dev -DCMAKE_INSTALL_PREFIX=/opt/install/libdab ..
	make -j $(nproc) install

	goodecho "[+] MBElib"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	installfromnet "git clone https://github.com/szechyjs/mbelib.git"
	cd mbelib
	git reset --hard 9a04ed5c78176a9965f3d43f7aa1b1f5330e771f
	mkdir build; cd build
	cmake -Wno-dev -DCMAKE_INSTALL_PREFIX=/opt/install/mbelib ..
	make -j $(nproc) install

	goodecho "[+] serialdv"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	installfromnet "git clone https://github.com/f4exb/serialDV.git"
	cd serialDV
	git reset --hard "v1.1.4"
	mkdir build; cd build
	cmake -Wno-dev -DCMAKE_INSTALL_PREFIX=/opt/install/serialdv ..
	make -j $(nproc) install

	goodecho "[+] DSDcc"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	installfromnet "git clone https://github.com/f4exb/dsdcc.git"
	cd dsdcc
	git reset --hard "v1.9.5"
	mkdir build; cd build
	cmake -Wno-dev -DCMAKE_INSTALL_PREFIX=/opt/install/dsdcc -DUSE_MBELIB=ON -DLIBMBE_INCLUDE_DIR=/opt/install/mbelib/include -DLIBMBE_LIBRARY=/opt/install/mbelib/lib/libmbe.so -DLIBSERIALDV_INCLUDE_DIR=/opt/install/serialdv/include/serialdv -DLIBSERIALDV_LIBRARY=/opt/install/serialdv/lib/libserialdv.so ..
	make -j $(nproc) install

	goodecho "[+] Codec2"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	installfromnet "apt-fast -y install libspeexdsp-dev libsamplerate0-dev"
	git clone https://github.com/drowe67/codec2-dev.git codec2
	cd codec2
	git reset --hard "v1.0.3"
	mkdir build_linux; cd build_linux
	cmake -Wno-dev -DCMAKE_INSTALL_PREFIX=/opt/install/codec2 ..
	make -j $(nproc) install

	goodecho "[+] SGP4"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	installfromnet "git clone https://github.com/dnwrnr/sgp4.git"
	cd sgp4
	mkdir build; cd build
	cmake -Wno-dev -DCMAKE_INSTALL_PREFIX=/opt/install/sgp4 ..
	make -j $(nproc) install

	goodecho "[+] libsigmf"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	installfromnet "git clone https://github.com/f4exb/libsigmf.git"
	cd libsigmf
	git checkout "new-namespaces"
	mkdir build; cd build
	cmake -Wno-dev -DCMAKE_INSTALL_PREFIX=/opt/install/libsigmf .. 
	make -j $(nproc) install

	goodecho "[+] ggmorse"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	installfromnet "git clone https://github.com/ggerganov/ggmorse.git"
	cd ggmorse
	mkdir build; cd build
	cmake -Wno-dev -DCMAKE_INSTALL_PREFIX=/opt/install/ggmorse -DGGMORSE_BUILD_TESTS=OFF -DGGMORSE_BUILD_EXAMPLES=OFF ..
	make -j $(nproc) install

	goodecho "[+] Installing SDR Angel"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	installfromnet "git clone https://github.com/f4exb/sdrangel.git"
	cd sdrangel
	mkdir build; cd build
	cmake -Wno-dev -DDEBUG_OUTPUT=ON -DRX_SAMPLE_24BIT=ON \
	-DCMAKE_BUILD_TYPE=RelWithDebInfo \
	-DAPT_DIR=/opt/install/aptdec \
	-DCM256CC_DIR=/opt/install/cm256cc \
	-DDSDCC_DIR=/opt/install/dsdcc \
	-DSERIALDV_DIR=/opt/install/serialdv \
	-DMBE_DIR=/opt/install/mbelib \
	-DCODEC2_DIR=/opt/install/codec2 \
	-DSGP4_DIR=/opt/install/sgp4 \
	-DLIBSIGMF_DIR=/opt/install/libsigmf \
	-DDAB_DIR=/opt/install/libdab \
	-DGGMORSE_DIR=/opt/install/ggmorse \
	-DCMAKE_INSTALL_PREFIX=/opt/install/sdrangel ..
	make -j $(nproc) install
	ln -s /opt/install/sdrangel/bin/sdrangel /usr/bin/sdrangel
}

function sdrpp_soft_fromsource_install () { # Beta test, but should work on almost all platforms
	goodecho "[+] Installing dependencies"
	installfromnet "apt-fast install libfftw3-dev libglfw3-dev libvolk2-dev libzstd-dev libairspyhf-dev libiio-dev libad9361-dev librtaudio-dev libhackrf-dev portaudio19-dev libcodec2-dev -y"
	goodecho "[+] Installing SDR++"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	goodecho "[+] Cloning SDR++ project"
	installfromnet "git clone https://github.com/AlexandreRouma/SDRPlusPlus.git"
	cd SDRPlusPlus/
	mkdir build
	cd build
	cmake .. -DOPT_BUILD_SOAPY_SOURCE=ON -DOPT_BUILD_AIRSPY_SOURCE=ON -DOPT_BUILD_AIRSPYHF_SOURCE=ON -DOPT_BUILD_NETWORK_SINK=ON \
			-DOPT_BUILD_FREQUENCY_MANAGER=ON -DOPT_BUILD_IQ_EXPORTER=ON -DOPT_BUILD_RECORDER=ON -DOPT_BUILD_RIGCTL_SERVER=ON -DOPT_BUILD_METEOR_DEMODULATOR=ON \
			-DOPT_BUILD_RADIO=ON -DOPT_BUILD_USRP_SOURCE=ON -DOPT_BUILD_FILE_SOURCE=ON -DOPT_BUILD_HACKRF_SOURCE=ON -DOPT_BUILD_RTL_SDR_SOURCE=ON -DOPT_BUILD_RTL_TCP_SOURCE=ON \
			-DOPT_BUILD_SDRPP_SERVER_SOURCE=ON -DOPT_BUILD_SOAPY_SOURCE=ON -DOPT_BUILD_SPECTRAN_SOURCE=OFF -DOPT_BUILD_SPECTRAN_HTTP_SOURCE=OFF  -DOPT_BUILD_LIMESDR_SOURCE=ON \
			-DOPT_BUILD_PLUTOSDR_SOURCE=ON -DOPT_BUILD_BLADERF_SOURCE=ON -DOPT_BUILD_AUDIO_SOURCE=ON -DOPT_BUILD_AUDIO_SINK=ON -DOPT_BUILD_PORTAUDIO_SINK=OFF \
			-DOPT_BUILD_NEW_PORTAUDIO_SINK=OFF -DOPT_BUILD_M17_DECODER=ON -DUSE_BUNDLE_DEFAULTS=ON -DCMAKE_BUILD_TYPE=Release # TODO: support Spectran devices on Docker creation
	make -j$(nproc)
	make install
	mkdir -p "/root/Library/Application Support/sdrpp/"
	cp /root/config/sdrpp-config.json "/root/Library/Application Support/sdrpp/config.json"
	cd /root
}

function sdrpp_soft_install () { # Working but not compatible with aarch64
	goodecho "[+] Installing dependencies"
	installfromnet "apt-fast install libfftw3-dev libglfw3-dev libvolk2-dev libzstd-dev libairspyhf-dev libiio-dev libad9361-dev librtaudio-dev libhackrf-dev -y"
	goodecho "[+] Installing SDR++"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	arch=`uname -i`
	prog=""
	case "$arch" in
  		x86_64|amd64)
    		prog="sdrpp_ubuntu_jammy_amd64.deb";;
  		arm*) # For Raspberry Pi for now
    		prog="sdrpp_raspios_bullseye_armhf.deb";;
  		*)
    		printf 'Unsupported architecture: "%s" -> use sdrpp_soft_fromsource_install instead\n' "$arch" >&2; exit 2;;
	esac
	installfromnet "wget https://github.com/AlexandreRouma/SDRPlusPlus/releases/download/nightly/$prog"
	dpkg -i $prog
	cd /root
}

function sigdigger_soft_install () {
	goodecho "[+] Installing dependencies"
	installfromnet "apt-fast install -y libxml2-dev libxml2-utils libfftw3-dev libasound-dev"
	goodecho "[+] Downloading and launching auto-script"
	[ -d /rftools/sdr ] || mkdir -p /rftools/sdr
	cd /rftools/sdr
	installfromnet "wget https://actinid.org/blsd"
	chmod +x blsd \ 
	./blsd
	cd /root
	ln -s /rftools/sdr/blsd-dir/SigDigger/SigDigger /usr/sbin/SigDigger
}

function cyberther_soft_install() {
	goodecho "[+] Installing Cyber Ether"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	installfromnet "git clone https://github.com/catchorg/Catch2.git"
	cd Catch2/
	mkdir build/ \
	&& cd build/ \
	&& cmake ../ \
	&& make -j$(nproc) \
	&& make install
	cd ../..
	goodecho "[CyberEther][+] Installing core dependencies"
	installfromnet "apt-fast install -y git build-essential cmake pkg-config ninja-build meson git zenity curl"
	installfromnet "apt-fast install -y rustc"
	goodecho "[CyberEther][+] Installing graphical dependencies"
	installfromnet "apt-fast install -y spirv-cross glslang-tools libglfw3-dev"
	goodecho "[CyberEther][+] Installing backend dependencies"
	installfromnet "apt-fast install -y mesa-vulkan-drivers libvulkan-dev vulkan-validationlayers cargo"
	goodecho "[CyberEther][+] Installing remote caps"
	installfromnet "apt-fast install -y gstreamer1.0-plugins-base libgstreamer-plugins-bad1.0-dev"
	installfromnet "apt-fast install -y libgstreamer-plugins-base1.0-dev libgstreamer-plugins-good1.0-dev"
	installfromnet "apt-fast install -y gstreamer1.0-plugins-good gstreamer1.0-plugins-bad gstreamer1.0-plugins-ugly"
	installfromnet "apt-fast install -y python3-yaml"
	goodecho "[CyberEther][+] Cloning GitHub repository"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	installfromnet "git clone https://github.com/luigifcruz/CyberEther.git"
	cd CyberEther
	meson setup -Dbuildtype=debugoptimized build && cd build
	ninja install
}

function inspection_decoding_tools () {
	goodecho "[+] Installing common inspection and decoding tools from package manager"
	installfromnet "apt-fast install -y audacity inspectrum sox multimon-ng gqrx-sdr"
	installfromnet "pip3 install cython"
	installfromnet "pip3 install urh"
	goodecho "[+] Installing rtl_433 tools"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	installfromnet "git clone https://github.com/merbanan/rtl_433.git"
	cd rtl_433/ \
	&& mkdir build \
	&& cd build \
	&& cmake ../ \
	&& make -j$(nproc) && sudo make install
	cd /root
}

function qsstv_soft_install () {
	goodecho "[+] Installing dependencies for qsstv_soft_install"
	installfromnet "apt-fast install -y pkg-config g++ libfftw3-dev qtbase5-dev qtchooser qt5-qmake qtbase5-dev-tools libhamlib++-dev libasound2-dev libpulse-dev libopenjp2-7 libopenjp2-7-dev libv4l-dev build-essential doxygen libqwt-qt5-dev"
	goodecho "[+] Cloning QSSTV"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	installfromnet "git clone https://github.com/ON4QZ/QSSTV.git"
	cd QSSTV/
	mkdir src/build
	cd src/build
	qmake ..
	make -j$(nproc)
	sudo make install
}

function ice9_bluetooth_soft_install() {
    local ARCH=$(uname -m)

    case "$ARCH" in
        x86_64|amd64)
            ice9_bluetooth_soft_install_call
            ;;
        i?86)
            ice9_bluetooth_soft_install_call
            ;;
        *)
            criticalecho "[-] Unsupported architecture: $ARCH. OpenBTS UMTS installation is not supported on this architecture."
            return 1
            ;;
    esac
}

function ice9_bluetooth_soft_install_call () {
	goodecho "[+] Installing dependencies for ice9_bluetooth"
	installfromnet "apt-fast install -y libliquid-dev libhackrf-dev libbladerf-dev libuhd-dev libfftw3-dev"
	goodecho "[+] Cloning ice9-bluetooth-sniffer"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	installfromnet "git clone https://github.com/mikeryan/ice9-bluetooth-sniffer.git"
	cd ice9-bluetooth-sniffer
	mkdir build
	cd build
	cmake ..
	make -j$(nproc)
	make install
}

function nfclaboratory_soft_install () {
	goodecho "[+] Installing dependencies for nfc-laboratory"
	installfromnet "apt-fast install -y libusb-1.0-0"
	goodecho "[+] Installing nfc-laboratory"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	installfromnet "git clone https://github.com/josevcm/nfc-laboratory.git"
	cmake -DCMAKE_BUILD_TYPE=Release -S nfc-laboratory -B cmake-build-release
	cmake --build cmake-build-release --target nfc-lab -- -j $(nproc)
	cp nfc-laboratory/dat/config/nfc-lab.conf /root
	[ -d /rftools/sdr ] || mkdir /rftools/sdr
	cp ./cmake-build-release/src/nfc-app/app-qt/nfc-lab /rftools/sdr/
	ln -s /rftools/sdr/nfc-lab /usr/bin/nfc-lab
}

function retrogram_soapysdr_soft_install () {
	goodecho "[+] Installing dependencies for retrogram"
	installfromnet "apt-fast install -y libsoapysdr-dev libncurses5-dev libboost-program-options-dev"
	goodecho "[+] Installing retrogram_soapysdr"
	[ -d /rftools/sdr ] || mkdir -p /rftools/sdr
	cd /rftools/sdr
	installfromnet "git clone https://github.com/r4d10n/retrogram-soapysdr.git"
	cd retrogram-soapysdr
	make -j$(nproc)
	ln -s /rftool/sdr/retrogram-soapysdr/retrogram-soapysdr /usr/bin/retrogram-soapysdr
}

function gps_sdr_sim_soft_install () {
	goodecho "[+] Installing gps-sdr-sim"
	[ -d /rftools/sdr ] || mkdir -p /rftools/sdr
	cd /rftools/sdr
	installfromnet "git clone https://github.com/osqzss/gps-sdr-sim.git"
	cd gps-sdr-sim
	gcc gpssim.c -lm -O3 -o gps-sdr-sim
	ln -s /rftool/sdr/gps-sdr-sim/gps-sdr-sim /usr/bin/gps-sdr-sim
}

function acarsdec_soft_install () {
	goodecho "[+] Installing acarsdec dependencies"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	installfromnet "apt-fast install -y zlib1g-dev libjansson-dev libxml2-dev"
	installfromnet "git clone https://github.com/szpajder/libacars.git"
	cd libacars
	mkdir build
	cd build
	cmake ../
	make -j$(nproc)
	make install
	ldconfig

	goodecho "[+] Installing acarsdec"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	installfromnet "git clone https://github.com/TLeconte/acarsdec.git"
	cd acarsdec
	mkdir build
	cd build
	cmake .. -Drtl=ON -Dairspy=ON -Dsoapy=ON #-Dsdrplay=ON
	make -j$(nproc)
	make install
}

function meshtastic_sdr_soft_install () {
	goodecho "[+] Installing Meshtastic_SDR dependencies"
	installfromnet "pip3 install meshtastic"
	[ -d /rftools/sdr ] || mkdir -p /rftools/sdr
	cd /rftools/sdr
	goodecho "[+] Cloning Meshtastic_SDR"
	installfromnet "git clone https://gitlab.com/crankylinuxuser/meshtastic_sdr.git"
}

function gpredict_sdr_soft_install () {
	goodecho "[+] Installing GPredict dependencies"
	installfromnet "apt-fast install -y libtool intltool autoconf automake libcurl4-openssl-dev pkg-config libglib2.0-dev libgtk-3-dev libgoocanvas-2.0-dev"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	goodecho "[+] Cloning Meshtastic_SDR"
	installfromnet "git clone https://github.com/csete/gpredict.git"
	cd gpredict
	./autogen.sh
	./configure
	make -j$(nproc)
	make install
}

function v2verifier_sdr_soft_install () {
	goodecho "[+] Installing v2verifier dependencies"
	installfromnet "apt-fast install -y swig libgmp3-dev python3-pip python3-tk python3-pil libssl-dev python3-pil.imagetk"
	[ -d /rftools/sdr ] || mkdir -p /rftools/sdr
	cd /rftools/sdr
	goodecho "[+] Cloning v2verifier"
	installfromnet "git clone https://github.com/FlUxIuS/v2verifier.git"
	cd v2verifier
	mkdir build
	cd build
	cmake ../
	make -j$(nproc)
}

function wavingz_sdr_soft_install () {
	[ -d /rftools/sdr ] || mkdir -p /rftools/sdr
	cd /rftools/sdr
	goodecho "[+] Cloning waving-z"
	installfromnet "git clone https://github.com/baol/waving-z.git"
	cd waving-z
	mkdir build
	cd build
	cmake .. -DCMAKE_BUILD_TYPE=Release
	cmake --build .
}