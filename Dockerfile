# This DockerFile is part of the RFSwift project
# Install type: Full all
# Author(s): Sébastien Dudek (@FlUxIuS) @Penthertz
# website: penthertz.com

FROM ubuntu:22.04

LABEL "org.container.project"="rfswift"
LABEL "org.container.author"="Sébastien Dudek (FlUxIuS)"

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
  	qtbase5-dev qtbase5-private-dev libqt5opengl5-dev libqt5svg5-dev \
  	libcanberra-gtk-module libcanberra-gtk3-module unity-tweak-tool libhdf5-dev \
	libreadline-dev automake qtdeclarative5-dev libqt5serialport5-dev \ 
	libqt5serialbus5-dev qttools5-dev golang-go

RUN DEBIAN_FRONTEND=noninteractive TZ=Etc/UTC \
	apt-get install tzdata

# Installing apt-fast wrapper
RUN DEBIAN_FRONTEND=noninteractive \
	apt-add-repository ppa:apt-fast/stable -y
RUN apt-get update && \
	echo apt-fast apt-fast/maxdownloads string 10 | debconf-set-selections && \
	echo apt-fast apt-fast/dlflag boolean true | debconf-set-selections && \
	echo apt-fast apt-fast/aptmanager string apt-get | debconf-set-selections

RUN apt-get -y install apt-fast python3-matplotlib

# Installing desktop features for next virtual desktop sessions
RUN echo apt-fast keyboard-configuration/layout string "English (US)" | debconf-set-selections && \
	echo apt-fast keyboard-configuration/variant string "English (US)" | debconf-set-selections && \
	apt-fast -y install task-lxqt-desktop && \
	apt-fast -y install language-pack-en && \
	update-locale

# Audio part
RUN apt-fast install -y pulseaudio-utils pulseaudio libasound2-dev libavahi-client-dev --no-install-recommends

COPY scripts /root/scripts/
COPY rules /root/rules/
COPY config /root/config/

WORKDIR /root/scripts/
RUN chmod +x entrypoint.sh

# Installing Terminal harnesses
RUN ./entrypoint.sh fzf_soft_install && \
	./entrypoint.sh zsh_tools_install && \
	./entrypoint.sh arsenal_soft_install
COPY config/.zshrc /root/.zshrc

# Installing Devices 

## Installing peripherals
RUN ./entrypoint.sh ad_devices_install && \
	./entrypoint.sh nuand_devices_install && \
	./entrypoint.sh hackrf_devices_install && \
	./entrypoint.sh airspy_devices_install && \
	./entrypoint.sh limesdr_devices_install && \
	./entrypoint.sh osmofl2k_devices_install && \
	./entrypoint.sh xtrx_devices_install && \
	./entrypoint.sh funcube_devices_install
#RUN ./entrypoint.sh uhd_devices_install # to install after
#RUN ./entrypoint.sh antsdr_uhd_devices_install # Disable orignal UHD
#RUN ./entrypoint.sh nuand_devices_fromsource_install
#RUN ./entrypoint.sh rtlsdr_devices_install # to install later
#RUN ./entrypoint.sh rtlsdrv4_devices_install # optionnal, remove rtlsdr_devices_install if you are using the v4 version

##################
# SDR1 
##################
## Installing extra peripherals
RUN ./entrypoint.sh uhd_devices_install && \
	./entrypoint.sh rtlsdr_devices_install
#RUN ./entrypoint.sh rtlsdrv4_devices_install # optionnal, remove rtlsdr_devices_install if you are using the v4 version
#RUN ./entrypoint.sh uhd_devices_fromsource_install
#RUN ./entrypoint.sh antsdr_uhd_devices_install # Disable orignal UHD

# Installing GNU Radio + some OOT modules
RUN ./entrypoint.sh gnuradio_soft_install && \ 
	./entrypoint.sh common_sources_and_sinks && \
	./entrypoint.sh install_soapy_modules && \
	./entrypoint.sh install_soapyPlutoSDR_modules

# SDR extra tools
RUN ./entrypoint.sh sdrpp_soft_fromsource_install && \
	./entrypoint.sh retrogram_soapysdr_soft_install

# Installing SA device modules
RUN ./entrypoint.sh kc908_sa_device && \
	./entrypoint.sh signalhound_sa_device && \
	./entrypoint.sh harogic_sa_device # working only on x86_64 and aarch64

# Calibration equipements
RUN ./entrypoint.sh leobodnarv1_cal_device && \
	./entrypoint.sh KCSDI_cal_device && \
	./entrypoint.sh NanoVNASaver_cal_device && \
	./entrypoint.sh NanoVNA_QT_cal_device

# Installing extra software
RUN ./entrypoint.sh jupiter_soft_install && \
	./entrypoint.sh inspection_decoding_tools

##################
# SDR2
##################
# Installing GNU Radio + extra OOT modules
RUN ./entrypoint.sh grgsm_grmod_install && \
	./entrypoint.sh grlora_grmod_install && \
	./entrypoint.sh grlorasdr_grmod_install && \
	./entrypoint.sh griridium_grmod_install && \
	./entrypoint.sh grinspector_grmod_install && \
	./entrypoint.sh gruaslink_grmod_install && \
	./entrypoint.sh grX10_grmod_install && \
	./entrypoint.sh grgfdm_grmod_install && \
	./entrypoint.sh graaronia_rtsa_grmod_install && \
	./entrypoint.sh grais_grmod_install && \
	./entrypoint.sh graistx_grmod_install && \
	./entrypoint.sh grreveng_grmod_install && \
	./entrypoint.sh grdvbs2_grmod_install && \
	./entrypoint.sh grtempest_grmod_install && \
	./entrypoint.sh grdab_grmod_install && \
	./entrypoint.sh grdect2_grmod_install && \
	./entrypoint.sh grfoo_grmod_install && \
	./entrypoint.sh grieee802-11_grmod_install && \
	./entrypoint.sh grieee802154_grmod_install && \
	./entrypoint.sh grrds_grmod_install && \
	./entrypoint.sh grdroineid_grmod_install && \
	./entrypoint.sh grsatellites_grmod_install && \
	./entrypoint.sh gradsb_grmod_install && \
	./entrypoint.sh grkeyfob_grmod_install && \
	./entrypoint.sh grradar_grmod_install && \
	./entrypoint.sh grnordic_grmod_install && \
	./entrypoint.sh grpaint_grmod_install && \
	./entrypoint.sh grzwavepoore_grmod_install && \
	./entrypoint.sh grmixalot_grmod_install && \
	./entrypoint.sh gr_DCF77_Receiver_grmod_install && \
	./entrypoint.sh grj2497_grmod_install && \
	./entrypoint.sh grairmodes_grmod_install && \
	./entrypoint.sh grbb60_Receiver_grmod_install # Only available for x86_64
#RUN ./entrypoint.sh grccsds_move_rtsa_grmod_install #TODO: fix problem with strtod_l dependency
#RUN ./entrypoint.sh deeptempest_grmod_install
## TODO: More more!

# Installing OOT modules from sandia
RUN ./entrypoint.sh grpdu_utils_grmod_install && \
	./entrypoint.sh grsandia_utils_grmod_install && \
	./entrypoint.sh grtiming_utils_grmod_install && \
	./entrypoint.sh grfhss_utils_grmod_install # depends on 'grpdu_utils_grmod_install' and 'grtiming_utils_grmod_install' and 'grsandia_utils_grmod_install'

# Installing OpenCL
## NVidia drivers
#RUN apt-fast install -y nvidia-opencl-dev nvidia-modprobe
## Installing Intel's OpenCL
#RUN apt-fast install -y intel-opencl-icd ocl-icd-dev ocl-icd-opencl-dev

# Installing gr-fosphor with OpenCL
#RUN ./entrypoint.sh grfosphor_grmod_install

# Installing CyberEther
RUN ./entrypoint.sh cyberther_soft_install # Enabe OpenCL for better exp

# Installing softwares
#RUN ./entrypoint.sh sdrangel_soft_install
RUN ./entrypoint.sh sdrangel_soft_fromsource_install && \
	./entrypoint.sh sigdigger_soft_install && \
	./entrypoint.sh qsstv_soft_install && \
	./entrypoint.sh ice9_bluetooth_soft_install && \
	./entrypoint.sh meshtastic_sdr_soft_install && \
	./entrypoint.sh gps_sdr_sim_soft_install && \
	./entrypoint.sh nfclaboratory_soft_install && \
	./entrypoint.sh gpredict_sdr_soft_install && \
	./entrypoint.sh v2verifier_sdr_soft_install && \
	./entrypoint.sh wavingz_sdr_soft_install

# Installing extra software
RUN ./entrypoint.sh ml_and_dl_soft_install

##################
# RFID
##################
# Tools for RFID
RUN ./entrypoint.sh proxmark3_soft_install && \
	./entrypoint.sh libnfc_soft_install && \
	./entrypoint.sh mfoc_soft_install && \
	./entrypoint.sh mfcuk_soft_install && \
	./entrypoint.sh mfread_soft_install


##################
# Wi-Fi
##################
# Tools for Wi-Fi
RUN ./entrypoint.sh common_nettools && \
	./entrypoint.sh aircrack_soft_install && \
	./entrypoint.sh reaver_soft_install && \
	./entrypoint.sh bully_soft_install && \
	./entrypoint.sh pixiewps_soft_install && \
	./entrypoint.sh Pyrit_soft_install && \
	./entrypoint.sh eaphammer_soft_install && \
	./entrypoint.sh airgeddon_soft_install && \
	./entrypoint.sh wifite2_soft_install

# Installing bettecap tool
#RUN ./entrypoint.sh bettercap_soft_install

# General monitoring software
RUN ./entrypoint.sh kismet_soft_install

##################
# Bluetooth
##################
# Installing bettecap tool
RUN ./entrypoint.sh bettercap_soft_install

# Tools for Bluetooth #TODO: more more!
RUN ./entrypoint.sh blueztools_soft_install && \
	./entrypoint.sh bluing_soft_install && \
	./entrypoint.sh bdaddr_soft_install

# Tools for Bluetooth LE
RUN ./entrypoint.sh mirage_soft_install && \
	./entrypoint.sh sniffle_soft_install

# General monitoring software
RUN ./entrypoint.sh kismet_soft_install

##################
# Reversing
##################
# Installing Reversing tools
RUN ./entrypoint.sh kataistruct_soft_install && \
	./entrypoint.sh unicorn_soft_install && \
	./entrypoint.sh keystone_soft_install && \
	./entrypoint.sh radare2_soft_install && \
	./entrypoint.sh ghidra_soft_install && \
	./entrypoint.sh binwalk_soft_install

# adding some SAST / DAST tools
RUN ./entrypoint.sh LLVM_install && \
	./entrypoint.sh AFL_install && \
	./entrypoint.sh honggfuzz_install && \
	./entrypoint.sh semgrep_install && \
	./entrypoint.sh cppcheck_install && \
	./entrypoint.sh clang_static_analyzer_install

#RUN ./entrypoint.sh cutter_soft_install #TODO: fix install
#RUN ./entrypoint.sh qiling_soft_install # TODO: resolve some debconf

##################
# Automotive
##################
# Installing Automotive tools
RUN ./entrypoint.sh canutils_soft_install && \
	./entrypoint.sh cantact_soft_install && \
	./entrypoint.sh caringcaribou_soft_install && \
	./entrypoint.sh savvycan_soft_install && \
	./entrypoint.sh gallia_soft_install && \
	./entrypoint.sh v2ginjector_soft_install

##################
# Telco
##################
# Tools for Telecom
RUN ./entrypoint.sh yatebts_blade2_soft_install && \
	./entrypoint.sh openbts_uhd_soft_install && \
	./entrypoint.sh openbts_umts_soft_install && \
	./entrypoint.sh srsran4G_5GNSA_soft_install && \
	./entrypoint.sh srsran5GSA_soft_install && \
	./entrypoint.sh Open5GS_soft_install && \
	./entrypoint.sh pycrate_soft_install && \
	./entrypoint.sh osmobts_suite_soft_install

RUN mkdir -p /sdrtools/
COPY run /sdrtools/run

# Cleaning and quitting
WORKDIR /root/
#RUN rm -rf /root/scripts/
RUN rm -rf /root/rules/
RUN rm -rf /root/thirdparty
RUN apt-fast clean
RUN DEBIAN_FRONTEND=noninteractive rm -rf /var/lib/apt/lists/*
