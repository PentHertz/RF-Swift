common:
	docker build -f SDR/corebuild.docker -t corebuild:latest ..

sdrlight: common
	docker build -f SDR/sdr_light.docker -t sdr_light:latest ..

sdrfull: common sdrlight
	docker build -f SDR/sdr_full.docker -t sdr_full:latest ..

rfid: common
	docker build -f rfid.docker -t rfid:latest ..

wifi: common
	docker build -f wifi.docker -t wifi:latest ..

bluetooth: common
	docker build -f bluetooth.docker -t bluetooth:latest ..

reversing: common
	docker build -f reversing.docker -t reversing:latest ..

automotive: common
	docker build -f automotive.docker -t automotive:latest ..

telecom: common
	docker build -f telecom.docker -t telecom:latest ..

build: sdrfull bluetooth wifi rfid reversing automotive telecom
	echo "Done!"
