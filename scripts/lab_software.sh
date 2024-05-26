#!/bin/bash

function jupiter_soft_install() {
	goodecho "[+] Installing Jupyter lab"
	installfromnet "pip3 install jupyterlab"
	goodecho "[+] Installing Jupyter lab"
	installfromnet "pip3 install notebook"
}
