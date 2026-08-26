# RF Swift Workbench (GUI) build matrix.
#
# The rfswift CLI is pure Go (CGO_ENABLED=0, -tags netgo) so it links into a
# single static binary for every OS/arch. A webview app cannot be that on Linux:
# it dynamically links GTK + WebKit, which cannot be statically linked in
# practice. The goal - one portable, self-contained artifact per OS/arch - is
# still met, per platform:
#
#   Windows : self-contained .exe (WebView2 ships with the OS). No cgo, so it is
#             cross-buildable from any host - as portable as the CLI.
#   macOS   : self-contained .app (WKWebView ships with the OS). Build on macOS.
#   Linux   : dynamically links gtk3 + webkit2gtk-4.1. Ship a single-file
#             AppImage (bundles the deps) for "run anywhere", plus the plain
#             dynamic binary for distro packages.
#
# Cross-compiling cgo targets is limited: build macOS on macOS and Linux on
# Linux (native runners in CI). Windows needs no cgo and builds from anywhere.

BIN     := rfswift-workbench
LDFLAGS := -s -w
WAILS_VERSION := v2.15.0
LINUX_TAGS := webkit2_41
OUT     := build/bin
HOSTARCH := $(shell go env GOARCH 2>/dev/null || echo amd64)

# Use the wails CLI if it is on PATH, otherwise fall back to `go run` (pinned to
# the module's version) so no separate install / PATH setup is needed.
WAILS_BIN := $(shell command -v wails 2>/dev/null)
ifeq ($(WAILS_BIN),)
WAILS_CMD := go run github.com/wailsapp/wails/v2/cmd/wails@$(WAILS_VERSION)
else
WAILS_CMD := wails
endif
WAILS := $(WAILS_CMD) build -trimpath -ldflags "$(LDFLAGS)"

.PHONY: all deps wails-cli clean security-test \
	windows windows_amd64 windows_arm64 \
	darwin darwin_amd64 darwin_arm64 darwin_universal \
	linux linux_amd64 linux_arm64 appimage

all: windows darwin linux

# Install the GUI build dependencies (GTK + WebKit on Linux; no-op on mac/win).
deps:
	scripts/install-deps.sh

# Optional: install the Wails CLI for hot-reload dev (`wails dev`). The build
# targets do not need it - they fall back to `go run`.
wails-cli:
	go install github.com/wailsapp/wails/v2/cmd/wails@$(WAILS_VERSION)

# ---- Windows: self-contained .exe, no cgo, cross-buildable from any host ----
windows: windows_amd64 windows_arm64

windows_amd64:
	$(WAILS) -platform windows/amd64 -o $(BIN)_windows_amd64.exe

windows_arm64:
	$(WAILS) -platform windows/arm64 -o $(BIN)_windows_arm64.exe

# ---- macOS: self-contained .app (build on macOS) ----
darwin: darwin_amd64 darwin_arm64

darwin_amd64:
	$(WAILS) -platform darwin/amd64

darwin_arm64:
	$(WAILS) -platform darwin/arm64

darwin_universal:
	$(WAILS) -platform darwin/universal

# ---- Linux: dynamic binary (build on Linux; needs gtk3 + webkit2gtk-4.1) ----
# `make linux` builds the HOST architecture only. The Linux GUI is a cgo target,
# so it cannot be cross-compiled between amd64 and arm64 without an aarch64 cross
# toolchain AND arm64 GTK/WebKit dev libs - build the other arch on that arch
# (a CI arm64 runner, an arm64 box, or an emulated container). To cross-compile
# arm64 anyway, install gcc-aarch64-linux-gnu + arm64 gtk3/webkit2gtk-4.1 and run:
#   make linux_arm64 CC=aarch64-linux-gnu-gcc PKG_CONFIG_PATH=/usr/lib/aarch64-linux-gnu/pkgconfig
linux: linux_$(HOSTARCH)

linux_amd64:
	$(WAILS) -platform linux/amd64 -tags $(LINUX_TAGS) -o $(BIN)_linux_amd64

linux_arm64:
	$(WAILS) -platform linux/arm64 -tags $(LINUX_TAGS) -o $(BIN)_linux_arm64

# ---- Linux portable single file: AppImage (bundles gtk3 + webkit) ----
# Requires linuxdeploy + linuxdeploy-plugin-gtk + ImageMagick on PATH.
appimage: linux_amd64
	scripts/mkappimage.sh $(OUT)/$(BIN)_linux_amd64

clean:
	rm -rf $(OUT)

security-test:
	scripts/security-test.sh
