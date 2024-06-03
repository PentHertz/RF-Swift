#!/bin/bash

function jupiter_soft_install() {
	goodecho "[+] Installing Jupyter lab"
	installfromnet "pip3 install jupyterlab"
	goodecho "[+] Installing Jupyter lab"
	installfromnet "pip3 install notebook"
}


function ml_and_dl_soft_install() {
	goodecho "[+] Installing ML/DL tools"
	installfromnet "pip3 install scikit-learn pandas seaborn Tensorflow"
}