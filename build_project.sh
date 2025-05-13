#!/bin/bash

# This code is part of RF Swift by @Penthertz
# Author(s): Sébastien Dudek (@FlUxIuS)

source common.sh

# Call the new functions as part of the setup process
echo -e "${YELLOW}[+] Checking xhost installation${NC}"
check_xhost

echo -e "${YELLOW}[+] Checking PulseAudio installation${NC}"
check_pulseaudio

echo -e "${YELLOW}[+] Checking cURL installation${NC}"
check_curl

echo -e "${YELLOW}[+] Checking Docker installation${NC}"
check_docker

echo -e "${YELLOW}[+] Installing Go${NC}"
install_go

# Ensure Go binary is in the PATH for the current script session
export PATH=$PATH:/usr/local/go/bin

echo -e "${YELLOW}[+] Building RF Swift Go Project${NC}"
building_rfswift


# Ask the user if they want to create an alias after the installation
install_binary_alias

# Prompt the user if they want to build a Docker container, pull an image, or exit
echo "Do you want to build a Docker container, pull an existing image, or exit?"
echo "1) Build Docker container"
echo "2) Pull Docker image"
echo "3) Exit"
read -p "Choose an option (1, 2, or 3): " option

if [ "$option" -eq 1 ]; then
    build_docker_image
elif [ "$option" -eq 2 ]; then
    pull_docker_image
elif [ "$option" -eq 3 ]; then
    echo -e "${GREEN}Exiting without additional actions.${NC}"
    exit 0
else
    echo -e "${RED}Invalid option. Exiting.${NC}"
    exit 1
fi

echo -e "${GREEN}Installation and setup completed.${NC}"