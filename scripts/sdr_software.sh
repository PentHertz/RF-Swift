#!/bin/bash

function sdrangel_soft_install() {
	goodecho "[+] Installing dependencies"
	installfromnet "apt-get update && sudo apt-get -y install
	git cmake g++ pkg-config autoconf automake libtool libfftw3-dev libusb-1.0-0-dev libusb-dev libhidapi-dev libopengl-dev
	qtbase5-dev qtchooser libqt5multimedia5-plugins qtmultimedia5-dev libqt5websockets5-dev
	qttools5-dev qttools5-dev-tools libqt5opengl5-dev libqt5quick5 libqt5charts5-dev
	qml-module-qtlocation  qml-module-qtpositioning qml-module-qtquick-window2
	qml-module-qtquick-dialogs qml-module-qtquick-controls qml-module-qtquick-controls2 qml-module-qtquick-layouts
	libqt5serialport5-dev qtdeclarative5-dev qtpositioning5-dev qtlocation5-dev libqt5texttospeech5-dev
	qtwebengine5-dev qtbase5-private-dev libqt5gamepad5-dev libqt5svg5-dev
	libfaad-dev zlib1g-dev libboost-all-dev libasound2-dev pulseaudio libopencv-dev libxml2-dev bison flex
	ffmpeg libavcodec-dev libavformat-dev libopus-dev doxygen graphviz 
	libhamlib4 libgl1-mesa-glx qtspeech5-speechd-plugin gstreamer1.0-libav"



	goodecho "[+] Downloading and unpacking SDR Angel"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
	cd /root/thirdparty
	installfromnet "wget https://github.com/f4exb/sdrangel/releases/download/v7.20.0/sdrangel-2563-master.tar.gz"
	tar xvzf sdrangel-2563-master.tar.gz
	goodecho "[+] Building and installing Airspy libs"
	cmake -DCMAKE_INSTALL_PREFIX=/usr ../
	make -j$(nproc); sudo make install
	cd ../..
}