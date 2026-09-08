#!/usr/bin/env bash
# Install the build dependencies for the RF Swift Workbench GUI.
#
# A webview app links GTK + WebKit at build time, so Linux needs those dev
# packages and a C toolchain. Build against the SYSTEM WebKit (not a foreign one
# like a Nix-linked build) or the webview can fail to init GL and render blank.
# macOS and Windows ship their webview with the OS and need no extra packages.
set -euo pipefail

C='\033[36m'; R='\033[31m'; Y='\033[33m'; Z='\033[0m'
log(){ printf "${C}[deps]${Z} %s\n" "$*"; }
warn(){ printf "${Y}[deps]${Z} %s\n" "$*"; }
err(){ printf "${R}[deps]${Z} %s\n" "$*" >&2; }

SUDO=""
if [ "$(id -u)" -ne 0 ] && command -v sudo >/dev/null 2>&1; then SUDO="sudo"; fi
have(){ command -v "$1" >/dev/null 2>&1; }

case "$(uname -s)" in
  Darwin)
    log "macOS: WKWebView ships with the OS - no GTK/WebKit packages needed."
    have go || warn "Install Go (https://go.dev/dl) to build."
    xcode-select -p >/dev/null 2>&1 || warn "Install the Xcode Command Line Tools: xcode-select --install"
    log "Then: make darwin_amd64  (or darwin_arm64 / darwin_universal)"
    exit 0 ;;
  MINGW*|MSYS*|CYGWIN*|Windows_NT)
    log "Windows: WebView2 ships with modern Windows - no build packages needed."
    have go || warn "Install Go (https://go.dev/dl) to build."
    log "Then: make windows_amd64"
    exit 0 ;;
esac

# ---- Linux ----
ID=""; ID_LIKE=""
[ -r /etc/os-release ] && . /etc/os-release || true
ids=" ${ID:-} ${ID_LIKE:-} "

install_apt() {
  $SUDO apt-get update
  local wk="libwebkit2gtk-4.1-dev"
  if ! apt-cache show libwebkit2gtk-4.1-dev >/dev/null 2>&1; then
    wk="libwebkit2gtk-4.0-dev"
    warn "webkit2gtk-4.1 unavailable; installing 4.0 - build WITHOUT '-tags webkit2_41' (set LINUX_TAGS= in the Makefile)."
  fi
  $SUDO apt-get install -y build-essential pkg-config libgtk-3-dev "$wk"
}
install_dnf() {
  $SUDO dnf install -y gcc pkg-config gtk3-devel webkit2gtk4.1-devel \
    || $SUDO dnf install -y gcc pkg-config gtk3-devel webkit2gtk3-devel
}
install_pacman() {
  $SUDO pacman -S --needed --noconfirm base-devel pkgconf gtk3 webkit2gtk-4.1 \
    || $SUDO pacman -S --needed --noconfirm base-devel pkgconf gtk3 webkit2gtk
}
install_zypper() {
  $SUDO zypper install -y gcc pkg-config gtk3-devel libwebkit2gtk-4_1-devel \
    || $SUDO zypper install -y gcc pkg-config gtk3-devel webkit2gtk3-soup2-devel
}
install_apk() {
  $SUDO apk add build-base pkgconf gtk+3.0-dev webkit2gtk-4.1-dev \
    || $SUDO apk add build-base pkgconf gtk+3.0-dev webkit2gtk-dev
}

picked=""
case "$ids" in
  *" debian "*|*" ubuntu "*|*" linuxmint "*|*" pop "*) have apt-get && { install_apt; picked=apt; } ;;
  *" fedora "*|*" rhel "*|*" centos "*|*" rocky "*|*" almalinux "*) have dnf && { install_dnf; picked=dnf; } ;;
  *" arch "*|*" manjaro "*|*" endeavouros "*) have pacman && { install_pacman; picked=pacman; } ;;
  *" suse "*|*" opensuse "*|*" sles "*) have zypper && { install_zypper; picked=zypper; } ;;
  *" alpine "*) have apk && { install_apk; picked=apk; } ;;
esac
if [ -z "$picked" ]; then
  warn "distro not matched by /etc/os-release; trying available package managers."
  if   have apt-get; then install_apt; picked=apt
  elif have dnf;     then install_dnf; picked=dnf
  elif have pacman;  then install_pacman; picked=pacman
  elif have zypper;  then install_zypper; picked=zypper
  elif have apk;     then install_apk; picked=apk
  else err "No supported package manager found. Install manually: a C toolchain, pkg-config, gtk3 dev, and webkit2gtk-4.1 dev."; exit 1
  fi
fi

# ---- verify ----
if have pkg-config && pkg-config --exists webkit2gtk-4.1; then
  log "OK ($picked): webkit2gtk-4.1 $(pkg-config --modversion webkit2gtk-4.1), gtk+-3.0 $(pkg-config --modversion gtk+-3.0)"
  log "Build with: make linux_amd64"
elif have pkg-config && pkg-config --exists webkit2gtk-4.0; then
  log "OK ($picked): webkit2gtk-4.0 present."
  warn "Build WITHOUT the 4.1 tag: make linux_amd64 LINUX_TAGS="
else
  err "webkit2gtk dev not detected after install - check your distro's package name."
  exit 1
fi

have go || warn "Go is not installed; get it from https://go.dev/dl to build."
log "The Wails CLI is fetched automatically (go run) - no separate install needed."
