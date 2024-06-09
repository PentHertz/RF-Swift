# Define the Go source files and binary name
SRC := main.go
BIN := rfswift

# Define the output directories
OUT_DIR := bin

# Define the Go build command
GO_BUILD := go build

# Define a static build tag to ensure static linking
STATIC_BUILD_TAG := netgo

# Default target (Build all targets)
.PHONY: all
all: linux windows

# Targets for Linux
.PHONY: linux
linux: linux_amd64 linux_arm64

.PHONY: linux_amd64
linux_amd64:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GO_BUILD) -tags '$(STATIC_BUILD_TAG)' -o $(OUT_DIR)/$(BIN)_linux_amd64 $(SRC)

.PHONY: linux_arm64
linux_arm64:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 $(GO_BUILD) -tags '$(STATIC_BUILD_TAG)' -o $(OUT_DIR)/$(BIN)_linux_arm64 $(SRC)

# Targets for Windows
.PHONY: windows
windows: windows_amd64 windows_arm64

.PHONY: windows_amd64
windows_amd64:
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 $(GO_BUILD) -tags '$(STATIC_BUILD_TAG)' -o $(OUT_DIR)/$(BIN)_windows_amd64.exe $(SRC)

.PHONY: windows_arm64
windows_arm64:
	GOOS=windows GOARCH=arm64 CGO_ENABLED=0 $(GO_BUILD) -tags '$(STATIC_BUILD_TAG)' -o $(OUT_DIR)/$(BIN)_windows_arm64.exe $(SRC)

# Clean target
.PHONY: clean
clean:
	rm -rf $(OUT_DIR)

# Create the output directory if it doesn't exist
$(shell mkdir -p $(OUT_DIR))

