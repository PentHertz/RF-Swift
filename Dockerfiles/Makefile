common:
	docker build -f SDR/corebuild.docker -t corebuild:latest ..

sdrlight: common
	docker build -f SDR/sdr_light.docker -t sdr_light:latest ..

sdrfull: common sdrlight
	docker build -f SDR/sdr_full.docker -t sdr_full:latest ..

rfid: common
	docker build -f SDR/rfid.docker -t rfid:latest ..

wifi: common
	docker build -f SDR/wifi.docker -t wifi:latest ..

bluetooth: common
	docker build -f SDR/bluetooth.docker -t bluetooth:latest ..

build: sdrfull bluetooth wifi rfid
	echo "Done!"
