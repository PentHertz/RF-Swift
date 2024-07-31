#!/bin/bash

function fzf_soft_install() {
	goodecho "[+] Installing fzf"
	installfromnet "apt-fast -y install fzf"
}

function zsh_tools_install() {
	goodecho "[+] Installing zsh"
	installfromnet "apt-fast -y install zsh"
	chsh -s /bin/zsh 
	goodecho "[+] Installing oh-my-zsh"
	sh -c "$(curl -fsSL https://raw.githubusercontent.com/ohmyzsh/ohmyzsh/master/tools/install.sh)"
	goodecho "[+] Installing pluggins"
	git clone https://github.com/zsh-users/zsh-autosuggestions ${ZSH_CUSTOM:-~/.oh-my-zsh/custom}/plugins/zsh-autosuggestions
	git clone https://github.com/zsh-users/zsh-syntax-highlighting.git ${ZSH_CUSTOM:-~/.oh-my-zsh/custom}/plugins/zsh-syntax-highlighting
}

function arsenal_soft_install() {
	goodecho "[+] Installing arsenal"
	cd /opt
	installfromnet "git clone https://github.com/Orange-Cyberdefense/arsenal.git"
	cd arsenal
	installfromnet "python3 -m pip install -r requirements.txt"
	#./addalias.sh
	echo "alias a='/opt/arsenal/run'" >> ~/.zshrc
	echo "alias a='/opt/arsenal/run'" >> ~/.bashrc
}