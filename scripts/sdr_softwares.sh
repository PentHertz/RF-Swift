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
	installfromnet "wget https://github.com/f4exb/sdrangel/releases/download/v7.20.0/sdrangel-2563-master.tar.gz"
	tar xvzf sdrangel-2563-master.tar.gz
	cd sdrangel-2563-master
	dpkg -i sdrangel_7.20.0-1_amd64.deb
	cd /root
}

function sdrpp_soft_install () {
	goodecho "[+] Installing dependencies"
	installfromnet "apt-fast install libfftw3-dev libglfw3-dev libvolk2-dev libzstd-dev libairspyhf-dev libiio-dev libad9361-dev librtaudio-dev libhackrf-dev -y"
	goodecho "[+] Installing SDR++"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	installfromnet "wget https://github.com/AlexandreRouma/SDRPlusPlus/releases/download/nightly/sdrpp_ubuntu_jammy_amd64.deb"
	dpkg -i sdrpp_ubuntu_jammy_amd64.deb
	cd /root
}

function sigdigger_soft_install () {
	goodecho "[+] Installing dependencies"
	installfromnet "apt-fast install -y libxml2-dev libxml2-utils libfftw3-dev libasound-dev"
	goodecho "[+] Downloading and launching auto-script"
	[ -d /sdrtools ] || mkdir -p /opt/sdrtools
	cd /sdrtools
	installfromnet "wget https://actinid.org/blsd"
	chmod +x blsd \ 
	./blsd
	cd /root
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

function ice9_bluetooth_soft_install () {
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
	installfromnet "apt-fast install -y libusb-1.0-0 qt5-default"
	goodecho "[+] Installing nfc-laboratory"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	installfromnet "git clone https://github.com/josevcm/nfc-laboratory.git"
	cmake -DCMAKE_BUILD_TYPE=Release -S nfc-laboratory -B cmake-build-release
	cmake --build cmake-build-release --target nfc-lab -- -j $(nproc)
	cp nfc-laboratory/dat/config/nfc-lab.conf /root
	[ -d /rftools ] || mkdir /rftools/
	cp ./cmake-build-release/src/nfc-app/app-qt/nfc-lab /rftools/
}