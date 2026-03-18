#!/bin/bash
# This code is part of RF Swift by @Penthertz
# Author(s): Sébastien Dudek (@FlUxIuS)
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/common.sh"
# Call the new functions as part of the setup process
echo -e "${YELLOW}[+] Checking xhost installation${NC}"
check_xhost
echo -e "${YELLOW}[+] Checking PulseAudio installation${NC}"
check_pulseaudio
echo -e "${YELLOW}[+] Checking cURL installation${NC}"
check_curl
echo -e "${YELLOW}[+] Checking container engine (Docker / Podman)${NC}"
check_container_engine
echo -e "${YELLOW}[+] Installing Go${NC}"
install_go
# Ensure Go binary is in the PATH for the current script session
export PATH=$PATH:/usr/local/go/bin
# On macOS, check Lima for USB passthrough
if [[ "$(uname -s)" == "Darwin" ]]; then
    echo -e "${YELLOW}[+] Checking Lima VM for USB passthrough${NC}"
    check_lima
fi
# Check config file
check_config_file
# Offer to update default profiles
update_profiles
# Ask the user if they want to create an alias after the installation
install_binary_alias