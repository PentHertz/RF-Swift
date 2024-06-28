#!/bin/bash


function yatebts_blade2_soft_install() { # TODO: make few tests with new Nuand libs, if unstable: fetch 3a411c87c2416dc68030d5823d73ebf3f797a145 
	goodecho "[+] Feching YateBTS from Nuand for firwmares"
	installfromnet "apt-fast install -y qtmultimedia5-dev libqt5multimediawidgets5 libqt5multimedia5-plugins libqt5multimedia5 qttools5-dev qttools5-dev-tools"
	[ -d /telecom/2G ] || mkdir -p /telecom/2G
	cd /telecom/2G
	installfromnet "wget https://nuand.com/downloads/yate-rc-3.tar.gz"
	tar xvzf yate-rc-3.tar.gz
	rm -R yate
	goodecho "[+] Fetching Yate"
	installfromnet "git clone https://github.com/svedm/yate.git" # TODO: maybe needs to be updated to rc3? 
	cd yate
	./autogen.sh
	./configure --prefix=/usr/local
	make -j$(nproc)
	make install
	ldconfig
	cd ..
	#goodecho "[+] Feching YateBTS"
	#installfromnet "git clone https://github.com/yatevoip/yatebts.git"
	cd yatebts
	./autogen.sh
	./configure --prefix=/usr/local
	make -j$(nproc)
	make install
	ldconfig
	goodecho "[+] Creating some confs"
	touch /usr/local/etc/yate/snmp_data.conf /usr/local/etc/yate/tmsidata.conf
	# chown root:yate /usr/local/etc/yate/*.conf # TODO: next when dropping root privs
	chmod g+w /usr/local/etc/yate/*.conf
	colorecho "[+] Now it's time for you to configure ;)"
}

# TODO: move to QT5
function yatebts_blade2_soft_install_toreview() { # TODO: make few tests with new Nuand libs, if unstable: fetch 3a411c87c2416dc68030d5823d73ebf3f797a145 
	goodecho "[+] Feching YateBTS from Nuand"
	[ -d /root/thirdparty ] || mkdir -p /root/thirdparty
	cd /root/thirdparty
	installfromnet "wget https://nuand.com/downloads/yate-rc-3.tar.gz"
	goodecho "[+] Installing Yate"
	cd yate
	./autogen.sh
	./configure --prefix=/usr/local
	make -j$(nproc)
	make install
	ldconfig
	cd ..
	goodecho "[+] Installing YateBTS"
	cd yatebts
	./autogen.sh
	./configure --prefix=/usr/local
	make -j$(nproc)
	make install
	ldconfig
	goodecho "[+] Creating some confs"
	touch /usr/local/etc/yate/snmp_data.conf /usr/local/etc/yate/tmsidata.conf
	# chown root:yate /usr/local/etc/yate/*.conf # TODO: next when dropping root privs
	chmod g+w /usr/local/etc/yate/*.conf
	colorecho "[+] Now it's time for you to configure ;)"
}

function openbts_uhd_soft_install() {
	goodecho "[+] Feching OpenBTS from penthertz"
	[ -d /telecom/2G ] || mkdir -p /telecom/2G
	cd /telecom/2G
	goodecho "[+] Cloninig OpenBTS"
	installfromnet "git clone https://github.com/PentHertz/OpenBTS.git"
	cd OpenBTS
	./preinstall.sh
	./autogen.sh
	./configure --with-uhd
	make -j$(nproc)
	make install
	ldconfig
	ln -s Transceiver52M/transceiver .
}

function openbts_umts_soft_install() {
	goodecho "[+] Feching OpenBTS UMTS from penthertz"
	[ -d /telecom/3G ] || mkdir -p /telecom/3G
	cd /telecom/3G
	goodecho "[+] Cloninig OpenBTS UMTS"
	installfromnet "git clone https://github.com/PentHertz/OpenBTS-UMTS.git"
	cd OpenBTS-UMTS
	git submodule init
	git submodule update
	./install_dependences.sh
	./autogen.sh
	./configure
	make
	sudo make install
}

function srsran4G_5GNSA_soft_install() {
	goodecho "[+] Installing srsRAN_4G dependencies"
	installfromnet "apt-fast -y install build-essential cmake libfftw3-dev libmbedtls-dev libboost-program-options-dev libconfig++-dev libsctp-dev"
	goodecho "[+] Feching srsRAN_4G"
	[ -d /telecom/4G ] || mkdir -p /telecom/4G
	cd /telecom/4G
	goodecho "[+] Cloninig and installing srsRAN 4G"
	installfromnet "git clone https://github.com/srsran/srsRAN_4G.git"
	cd srsRAN_4G
	mkdir build
	cd build
	cmake ../
	make -j$(nproc)
	#make test
}

function srsran5GSA_soft_install() {
	goodecho "[+] Installing srsran_project dependencies"
	installfromnet "apt-fast -y install cmake make gcc g++ pkg-config libfftw3-dev libmbedtls-dev libsctp-dev libyaml-cpp-dev libgtest-dev"
	goodecho "[+] Feching srsran_project"
	[ -d /telecom/5G ] || mkdir -p /telecom/5G
	cd /telecom/5G
	goodecho "[+] Cloninig and installing srsran_project"
	installfromnet "git clone https://github.com/srsRAN/srsRAN_Project.git"
	cd srsRAN_Project
	mkdir build
	cd build
	cmake ../
	make -j $(nproc)
	#make test -j $(nproc)
}

function Open5GS_soft_install() {
	goodecho "[+] Installing Open5GS dependencies"
	echo "deb [ arch=amd64,arm64 signed-by=/usr/share/keyrings/mongodb-server-6.0.gpg] https://repo.mongodb.org/apt/ubuntu jammy/mongodb-org/6.0 multiverse" | sudo tee /etc/apt/sources.list.d/mongodb-org-6.0.list
	installfromnet "apt-fast -y update"
	installfromnet "apt-fast install -y mongodb-org python3-pip python3-setuptools python3-wheel ninja-build build-essential flex bison git cmake libsctp-dev libgnutls28-dev libgcrypt-dev libssl-dev libidn11-dev libmongoc-dev libbson-dev libyaml-dev libnghttp2-dev libmicrohttpd-dev libcurl4-gnutls-dev libnghttp2-dev libtins-dev libtalloc-dev meson"
	goodecho "[+] Feching Open5GS"
	[ -d /telecom/5G ] || mkdir -p /telecom/5G
	cd /telecom/5G
	goodecho "[+] Cloninig and installing Open5GS"
	installfromnet "git clone https://github.com/open5gs/open5gs"
	cd open5gs
	meson build --prefix=`pwd`/install
	ninja -C build
	goodecho "[+] Building Web GUI"
	installfromnet "apt-fast install -y ca-certificates curl gnupg"
	mkdir -p /etc/apt/keyrings
	curl -fsSL https://deb.nodesource.com/gpgkey/nodesource-repo.gpg.key | sudo gpg --dearmor -o /etc/apt/keyrings/nodesource.gpg
	NODE_MAJOR=20
	echo "deb [signed-by=/etc/apt/keyrings/nodesource.gpg] https://deb.nodesource.com/node_$NODE_MAJOR.x nodistro main" | sudo tee /etc/apt/sources.list.d/nodesource.list
	installfromnet "apt-fast update"
	installfromnet "apt-fast install nodejs -y"
	cd webui
	npm ci
}

function pycrate_soft_install() {
	[ -d /telecom/5G ] || mkdir -p /telecom
	cd /telecom
	goodecho "[+] Cloninig and installing pycrate"
	installfromnet "git clone https://github.com/pycrate-org/pycrate.git"
	cd pycrate
	python3 setup.py install
}

### TODO: more More!