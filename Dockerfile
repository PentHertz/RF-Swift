# This DockerFile is part of the RFSwift project
# Install type: full
# Author(s): Sébastien Dudek (@FlUxIuS) @Penthertz
# website: penthertz.com

FROM ubuntu:22.04
RUN echo 'APT::Install-Suggests "0";' >> /etc/apt/apt.conf.d/00-docker
RUN echo 'APT::Install-Recommends "0";' >> /etc/apt/apt.conf.d/00-docker

# Installing basic packages
RUN DEBIAN_FRONTEND=noninteractive \
  apt-get update \
  && apt-get install -y python3 wget curl sudo software-properties-common \
  	gpg-agent pulseaudio udev python3-packaging vim autoconf build-essential \
  	build-essential cmake python3-pip libsndfile-dev scapy screen tcpdump \
  	qt5-qmake qtbase5-dev xterm libusb-1.0-0-dev pkg-config git apt-utils \
  	libusb-1.0-0 libncurses5-dev libtecla1 libtecla-dev dialog procps unzip \
  	texlive liblog4cpp5-dev libcurl4-gnutls-dev libpcap-dev libgtk-3-dev \
  	qtcreator qtcreator-data qtcreator-doc qtbase5-examples qtbase5-doc-html \
  	qtbase5-dev qtbase5-private-dev libqt5opengl5-dev libqt5svg5-dev

RUN DEBIAN_FRONTEND=noninteractive TZ=Etc/UTC \
	apt-get install tzdata

# Installing apt-fast wrapper
RUN DEBIAN_FRONTEND=noninteractive \
	apt-add-repository ppa:apt-fast/stable -y
RUN apt-get update
RUN echo apt-fast apt-fast/maxdownloads string 10 | debconf-set-selections
RUN echo apt-fast apt-fast/dlflag boolean true | debconf-set-selections
RUN echo apt-fast apt-fast/aptmanager string apt-get | debconf-set-selections

RUN apt-get -y install apt-fast python3-matplotlib

COPY scripts /root/scripts/
COPY rules /root/rules/

WORKDIR /root/scripts/
RUN chmod +x entrypoint.sh

# Installing Devices 

## Installing peripherals
RUN ./entrypoint.sh ad_devices_install
RUN ./entrypoint.sh uhd_devices_install
RUN ./entrypoint.sh nuand_devices_install
RUN ./entrypoint.sh hackrf_devices_install
RUN ./entrypoint.sh airspy_devices_install
RUN ./entrypoint.sh limesdr_devices_install
RUN ./entrypoint.sh rtlsdr_devices_install
#RUN ./entrypoint.sh rtlsdrv4_devices_install # optionnal, remove rtlsdr_devices_install if you are using the v4 version

# Installing GNU Radio + extra OOT modules
RUN ./entrypoint.sh gnuradio_soft_install
RUN ./entrypoint.sh common_sources_and_sinks
RUN ./entrypoint.sh grgsm_grmod_install
RUN ./entrypoint.sh install_soapy_modules
RUN ./entrypoint.sh grlora_grmod_install
RUN ./entrypoint.sh grlorasdr_grmod_install
RUN ./entrypoint.sh griridium_grmod_install
RUN ./entrypoint.sh grinspector_grmod_install
RUN ./entrypoint.sh gruaslink_grmod_install #TODO: fix Python3 compat at least
RUN ./entrypoint.sh grX10_grmod_install
RUN ./entrypoint.sh grgfdm_grmod_install
RUN ./entrypoint.sh graaronia_rtsa_grmod_install
#RUN ./entrypoint.sh grccsds_move_rtsa_grmod_install #TODO: fix problem with strtod_l dependency
RUN ./entrypoint.sh grais_grmod_install
RUN ./entrypoint.sh grreveng_grmod_install
RUN ./entrypoint.sh grdvbs2_grmod_install
RUN ./entrypoint.sh grtempest_grmod_install
RUN ./entrypoint.sh grdab_grmod_install
RUN ./entrypoint.sh grdect2_grmod_install
RUN ./entrypoint.sh grfoo_grmod_install
RUN ./entrypoint.sh grieee802-11_grmod_install # depends on grfoo_grmod_install
RUN ./entrypoint.sh grieee802154_grmod_install # depends on grfoo_grmod_install
RUN ./entrypoint.sh grrds_grmod_install
RUN ./entrypoint.sh grdroineid_grmod_install
RUN ./entrypoint.sh grsatellites_grmod_install
## TODO: More more!

# Installing OOT modules from sandia
RUN ./entrypoint.sh grpdu_utils_grmod_install
RUN ./entrypoint.sh grsandia_utils_grmod_install # depends on grpdu_utils_grmod_install
RUN ./entrypoint.sh grtiming_utils_grmod_install
RUN ./entrypoint.sh grfhss_utils_grmod_install # depends on 'grpdu_utils_grmod_install' and 'grtiming_utils_grmod_install' and 'grsandia_utils_grmod_install'

# Installing OpenCL
## NVidia drivers
#RUN apt-fast install -y nvidia-opencl-dev nvidia-modprobe
## Installing Intel's OpenCL
#RUN apt-fast install -y intel-opencl-icd ocl-icd-dev ocl-icd-opencl-dev

# Installing gr-fosphor with OpenCL
#RUN ./entrypoint.sh grfosphor_grmod_install

# Installing cyberther with OpenCL
RUN ./entrypoint.sh cyberther_soft_install

# Installing softwares
RUN ./entrypoint.sh sdrpp_soft_install
RUN ./entrypoint.sh sigdigger_soft_install
RUN ./entrypoint.sh inspection_decoding_tools
RUN ./entrypoint.sh qsstv_soft_install
RUN ./entrypoint.sh ice9_bluetooth_soft_install

# Installing SA device modules
RUN ./entrypoint.sh kc908_sa_device

# Tools for RFID
RUN ./entrypoint.sh proxmark3_soft_install
RUN ./entrypoint.sh libnfc_soft_install
RUN ./entrypoint.sh mfoc_soft_install
RUN ./entrypoint.sh mfcuk_soft_install

# Tools for Wi-Fi
RUN ./entrypoint.sh common_nettools
RUN ./entrypoint.sh aircrack_soft_install
RUN ./entrypoint.sh reaver_soft_install
RUN ./entrypoint.sh bully_soft_install
RUN ./entrypoint.sh pixiewps_soft_install
RUN ./entrypoint.sh Pyrit_soft_install
RUN ./entrypoint.sh eaphammer_soft_install
RUN ./entrypoint.sh airgeddon_soft_install
RUN ./entrypoint.sh wifite2_soft_install

# Installing bettecap tool
RUN ./entrypoint.sh bettercap_soft_install

# Tools for Bluetooth
#RUN ./entrypoint.sh mirage_soft_install # TODO: In progress

# Installing extra software
RUN ./entrypoint.sh jupiter_soft_install

RUN mkdir /sdrtools/
COPY run /sdrtools/run

# Cleaning and quitting
WORKDIR /root/
#RUN rm -rf /root/scripts/
RUN rm -rf /root/rules/
RUN rm -rf /root/thirdparty
RUN apt-fast clean
RUN DEBIAN_FRONTEND=noninteractive rm -rf /var/lib/apt/lists/*
