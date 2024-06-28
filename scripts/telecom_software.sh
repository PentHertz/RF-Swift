#!/bin/bash

function yatebts_blade2_soft_install() { # TODO: make few tests with new Nuand libs, if unstable: fetch 3a411c87c2416dc68030d5823d73ebf3f797a145 
	goodecho "[+] Feching YateBTS from Nuand"
	[ -d /root/thirdparty ] || mkdir /root/thirdparty
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



### TODO: more More!