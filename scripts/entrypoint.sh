#!/bin/bash

source common.sh
source sdr_peripherals.sh
source sdr_softwares.sh
source gr_oot_modules.sh
source lab_software.sh
source sa_devices.sh
source rf_tools.sh
source cal_devices.sh
source reverse_software.sh
source sast_software.sh
source automotive_software.sh
source telecom_software.sh
source terminal_harness.sh

# Part picket from Exegol project with love <3 (https://github.com/ThePorgs/Exegol)
if [[ $EUID -ne 0 ]]; then
  criticalecho "You must be a root user"
else
  if declare -f "$1" > /dev/null
  then
    if [[ -f '/.dockerenv' ]]; then
      echo -e "${GREEN}"
      echo "This script is running in docker, as it should :)"
      echo "If you see things in red, don't panic, it's usually not errors, just badly handled colors"
      echo -e "${NOCOLOR}"
      "$@"
    else
      echo -e "${RED}"
      echo "[!] Careful : this script is supposed to be run inside a docker/VM, do not run this on your host unless you know what you are doing and have done backups. You have been warned :)"
      echo -e "${NOCOLOR}"
      "$@"
    fi
  else
    echo "'$1' is not a known function name" >&2
    exit 1
  fi
fi
