#!/bin/bash
# This code is part of RF Swift by @Penthertz
# Author(s): Sébastien Dudek (@FlUxIuS)
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/common.sh"
display_rainbow_logo_animated
echo -e "${YELLOW}📡 This script will set up your RF Swift environment 📡${NC}"
# Call the new functions as part of the setup process
echo -e "${YELLOW}[+] Checking xhost installation${NC}"
check_xhost
echo -e "${YELLOW}[+] Checking PulseAudio installation${NC}"
check_pulseaudio
echo -e "${YELLOW}[+] Checking cURL installation${NC}"
check_curl
echo -e "${YELLOW}[+] Checking container engine (Docker / Podman)${NC}"
check_container_engine
check_agnoster_dependencies
# Ensure Go binary is in the PATH for the current script session
export PATH=$PATH:/usr/local/go/bin
# Check config file
check_config_file
# Ask the user if they want to create an alias after the installation
install_binary_alias
echo -e "${GREEN}🎉 RF Swift setup complete! 🎉${NC}"
echo -e "${YELLOW}🙏 Thank you for using RF Swift by @Penthertz 🙏${NC}"
echo -e "${YELLOW}👨‍💻 Author(s): Sébastien Dudek (@FlUxIuS) 👨‍💻${NC}"
echo -e "${GREEN}🔧 Happy hacking! 🔧${NC}"