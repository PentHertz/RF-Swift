# Set default architecture to all if ARCH is not provided
ARCH ?= all

# Determine the platform based on the ARCH value
ifeq ($(ARCH),amd64)
    PLATFORM=linux/amd64
else ifeq ($(ARCH),arm64)
    PLATFORM=linux/arm64/v8
else ifeq ($(ARCH),riscv64)
    PLATFORM=linux/riscv64
else ifeq ($(ARCH),all)
    PLATFORM=linux/amd64,linux/arm64/v8,linux/riscv64
else
    $(error Unsupported architecture: $(ARCH). Supported values are amd64, arm64, riscv64, all)
endif

common:
	docker buildx build --platform $(PLATFORM) -f SDR/corebuild.docker -t corebuild:latest ..

sdrlight: common
	docker buildx build --platform $(PLATFORM) -f SDR/sdr_light.docker -t sdr_light:latest ..

sdrfull1: common sdrlight
	docker buildx build --platform $(PLATFORM) -f SDR/sdr_full.docker --target extraoot ..

sdrfull2: common sdrlight
	docker buildx build --platform $(PLATFORM) -f SDR/sdr_full.docker --target extrasofts ..

sdrfull3: common sdrlight
	docker buildx build --platform $(PLATFORM) -f SDR/sdr_full.docker --target mldlsofts ..

sdrfull: common sdrlight
	docker buildx build --platform $(PLATFORM) -f SDR/sdr_full.docker -t sdr_full:latest ..

rfid: common
	docker buildx build --platform $(PLATFORM) -f rfid.docker -t rfid:latest ..

wifi: common sdrlight
	docker buildx build --platform $(PLATFORM) -f wifi.docker -t wifi:latest ..

bluetooth: common sdrlight
	docker buildx build --platform $(PLATFORM) -f bluetooth.docker -t bluetooth:latest ..

reversing: common
	docker buildx build --platform $(PLATFORM) -f reversing.docker -t reversing:latest ..

automotive: common
	docker buildx build --platform $(PLATFORM) -f automotive.docker -t automotive:latest ..

telecom: common sdrlight
	docker buildx build --platform $(PLATFORM) -f telecom.docker -t telecom:latest ..

deeptempest: common sdrlight
	docker buildx build --platform $(PLATFORM) -f SDR/sdr_deeptemptest_beta.docker -t deeptempest_beta:latest ..

build: bluetooth wifi rfid reversing automotive telecom sdrfull
	echo "Done!"