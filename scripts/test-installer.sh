#!/usr/bin/env bash
set -euo pipefail

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
export RFSWIFT_INSTALLER_LIB_ONLY=1
# shellcheck source=../get_rfswift.sh
. "$ROOT/get_rfswift.sh"

tmp=$(mktemp -d)
trap 'find "$tmp" -type f -delete; find "$tmp" -depth -type d -empty -delete' EXIT
mkdir -p "$tmp/good"
printf 'ok\n' > "$tmp/good/rfswift"
tar -C "$tmp/good" -czf "$tmp/good.tar.gz" rfswift
validate_tar_archive "$tmp/good.tar.gz"

mkdir -p "$tmp/source"
printf 'bad\n' > "$tmp/source/payload"
tar -C "$tmp/source" --transform='s|payload|../escape|' -czf "$tmp/bad.tar.gz" payload
if (validate_tar_archive "$tmp/bad.tar.gz") >/dev/null 2>&1; then
  echo "unsafe tar path was accepted" >&2
  exit 1
fi

# --- Dev channel: newest GitHub prerelease; DEV_VERSION only as a fallback ---
# (v4.0.0-dev was retired while the script still pinned it, so every
# dev-channel install ended on a 404.)
RELEASE_CHANNEL=dev
http_get() { printf '[{"tag_name":"v9.9.9","prerelease":false,"assets":[{"name":"a"},{"name":"b"}]},{"tag_name":"v9.9.9-dev","prerelease":true,"assets":[{"name":"c"}]}]'; }
get_latest_release >/dev/null
[ "$VERSION" = "9.9.9-dev" ] || { echo "dev channel did not pick the newest prerelease: $VERSION" >&2; exit 1; }
[ "$DOWNLOAD_BASE_URL" = "https://github.com/PentHertz/RF-Swift/releases/download/v9.9.9-dev" ]
http_get() { return 1; }
get_latest_release >/dev/null
[ "$VERSION" = "$DEV_VERSION" ] || { echo "dev channel fallback is not DEV_VERSION: $VERSION" >&2; exit 1; }
http_get() { printf '[{"tag_name":"v1;id","prerelease":true}]'; }
get_latest_release >/dev/null
[ "$VERSION" = "$DEV_VERSION" ] || { echo "an unsafe prerelease tag was accepted: $VERSION" >&2; exit 1; }
unset -f http_get

# --- Version string validation (network-derived, flows into paths/URLs) ------
validate_version_string "4.0.1"
validate_version_string "4.1.0-rc.2"
for bad in "" "4.0.1/../evil" '$(reboot)' "4.0.1 rm" "v4;id"; do
  if validate_version_string "$bad"; then
    echo "accepted malicious version string: $bad" >&2
    exit 1
  fi
done

# --- Native package flow guards and package naming ---------------------------
# The dev channel must never take the native-package path (prerelease versions
# are tilde-mangled in package filenames).
OS=Linux ARCH=x86_64 VERSION=9.9.9 INSTALL_COMPONENTS=cli PKG_FORMAT=""
if try_native_package_install >/dev/null 2>&1; then
  echo "native path accepted the dev channel" >&2
  exit 1
fi

# An explicit tarball choice must short-circuit before any network probe.
RELEASE_CHANNEL=stable PKG_FORMAT=tarball
release_asset_exists() { echo "unexpected network probe" >&2; exit 1; }
get_package_manager() { echo apt; }
if try_native_package_install >/dev/null 2>&1; then
  echo "native path ignored PKG_FORMAT=tarball" >&2
  exit 1
fi

# Package names must match the release artifacts (goreleaser nfpms for the
# CLI, mkpackages.sh for the Workbench). Stub sudo away so the function
# returns after constructing the names without touching the system.
have_sudo_access() { return 1; }
PKG_FORMAT="" INSTALL_COMPONENTS=both
try_native_package_install >/dev/null 2>&1 || true
[ "$cli_pkg" = "rfswift_9.9.9_amd64.deb" ] || { echo "bad deb CLI name: $cli_pkg" >&2; exit 1; }
[ "$wb_pkg" = "rfswift-workbench_9.9.9_amd64.deb" ] || { echo "bad deb Workbench name: $wb_pkg" >&2; exit 1; }

get_package_manager() { echo dnf; }
ARCH=arm64 PKG_FORMAT=""
try_native_package_install >/dev/null 2>&1 || true
[ "$cli_pkg" = "rfswift-9.9.9-1.aarch64.rpm" ] || { echo "bad rpm CLI name: $cli_pkg" >&2; exit 1; }
[ "$wb_pkg" = "rfswift-workbench-9.9.9-1.aarch64.rpm" ] || { echo "bad rpm Workbench name: $wb_pkg" >&2; exit 1; }

get_package_manager() { echo pacman; }
PKG_FORMAT=""
try_native_package_install >/dev/null 2>&1 || true
[ "$cli_pkg" = "rfswift-9.9.9-1-aarch64.pkg.tar.zst" ] || { echo "bad pacman CLI name: $cli_pkg" >&2; exit 1; }

# riscv64 has no Workbench package; a workbench request must fall back whole.
get_package_manager() { echo apt; }
ARCH=riscv64 PKG_FORMAT="" INSTALL_COMPONENTS=both
if try_native_package_install >/dev/null 2>&1; then
  echo "native path accepted riscv64 workbench" >&2
  exit 1
fi

# --- udev rules offer: drives the installed CLI, honours RFSWIFT_UDEV --------
if [ "$(uname -s)" = Linux ]; then
  fake="$tmp/fakebin"; mkdir -p "$fake"
  printf '#!/bin/sh\necho "$@" >> "%s/calls"\n' "$fake" > "$fake/rfswift"
  chmod +x "$fake/rfswift"
  NATIVE_INSTALLED=false INSTALL_DIR="$fake" INSTALL_COMPONENTS=cli
  UDEV_RULES=1 offer_udev_rules >/dev/null 2>&1
  grep -q '^host udev --yes$' "$fake/calls" || { echo "RFSWIFT_UDEV=1 did not run 'rfswift host udev --yes'" >&2; exit 1; }
  rm -f "$fake/calls"
  UDEV_RULES=0 offer_udev_rules >/dev/null 2>&1
  [ ! -e "$fake/calls" ] || { echo "RFSWIFT_UDEV=0 must not run rfswift" >&2; exit 1; }
  INSTALL_COMPONENTS=workbench UDEV_RULES=1 offer_udev_rules >/dev/null 2>&1
  [ ! -e "$fake/calls" ] || { echo "a Workbench-only install has no CLI to run" >&2; exit 1; }
fi

# --- Existing tarball locations are retained during an upgrade ---------------
cli_target="$tmp/old-cli"; wb_target="$tmp/old-workbench"
mkdir -p "$cli_target" "$wb_target" "$tmp/pathbin"
printf '#!/bin/sh\n' > "$cli_target/rfswift"
printf '#!/bin/sh\n' > "$wb_target/rfswift-workbench"
chmod +x "$cli_target/rfswift" "$wb_target/rfswift-workbench"
ln -s "$cli_target/rfswift" "$tmp/pathbin/rfswift"
ln -s "$wb_target/rfswift-workbench" "$tmp/pathbin/rfswift-workbench"
old_path=$PATH; PATH="$tmp/pathbin:$PATH"
INSTALL_COMPONENTS=both INSTALL_DIR=""
choose_install_dir >/dev/null
PATH=$old_path
[ "$CLI_INSTALL_DIR" = "$cli_target" ] || { echo "CLI upgrade path was not retained: $CLI_INSTALL_DIR" >&2; exit 1; }
[ "$WORKBENCH_INSTALL_DIR" = "$wb_target" ] || { echo "Workbench upgrade path was not retained: $WORKBENCH_INSTALL_DIR" >&2; exit 1; }

# --- Nix installer: sbin directories reach PATH; no group tool stops early ---
# Debian's `su` (without `-`) and user shells have no /usr/sbin on PATH, where
# groupadd lives, and the Determinate installer only searches PATH (seen on
# Debian 13: "Could not find a supported command to create groups").
if [ "$(uname -s)" = Linux ] && [ -d /usr/sbin ]; then
  old_path=$PATH; PATH=/usr/local/bin:/usr/bin:/bin
  ensure_sbin_on_path
  case ":$PATH:" in
    *":/usr/sbin:"*) ;;
    *) echo "ensure_sbin_on_path did not add /usr/sbin: $PATH" >&2; exit 1 ;;
  esac
  once=$PATH; ensure_sbin_on_path
  [ "$PATH" = "$once" ] || { echo "ensure_sbin_on_path is not idempotent: $PATH" >&2; exit 1; }
  # Without any group-creation tool the installer must stop before downloading.
  curl() { echo "unexpected network access" >&2; exit 1; }
  command_exists() { case "$1" in nix|groupadd|addgroup) return 1 ;; esac; command -v "$1" >/dev/null 2>&1; }
  if install_nix >/dev/null 2>&1; then
    echo "install_nix proceeded without groupadd/addgroup" >&2; exit 1
  fi
  PATH=$old_path
fi

# --- Engine / Nix / install-dir knobs; optional steps never end the run ------
if (RFSWIFT_CHANNEL=stable RFSWIFT_INSTALL=cli ENGINE_PREF=bogus choose_release_and_components) >/dev/null 2>&1; then
  echo "RFSWIFT_ENGINE=bogus was accepted" >&2; exit 1
fi
if (RFSWIFT_CHANNEL=stable RFSWIFT_INSTALL=cli INSTALL_DIR_PREF=relative/dir choose_release_and_components) >/dev/null 2>&1; then
  echo "a relative RFSWIFT_INSTALL_DIR was accepted" >&2; exit 1
fi
(RFSWIFT_CHANNEL=stable RFSWIFT_INSTALL=cli ENGINE_PREF=skip NIX_PREF=0 INSTALL_DIR_PREF=/opt/rfswift choose_release_and_components) >/dev/null
# The knobs answer without a prompt; a failed engine or Nix install is
# reported and the function still returns success (the RF Swift install must
# go on: a plain call under `set -e` used to end the whole script there).
detect_container_engines() { HAS_DOCKER=false; HAS_PODMAN=false; }
prompt_choice() { echo "unexpected prompt: $1" >&2; exit 1; }
install_docker() { echo "unexpected Docker install" >&2; exit 1; }
install_podman() { echo "unexpected Podman install" >&2; exit 1; }
ENGINE_PREF=skip check_container_engine >/dev/null
install_docker() { return 1; }
ENGINE_PREF=docker check_container_engine >/dev/null || { echo "a failed Docker install ended check_container_engine" >&2; exit 1; }
command_exists() { case "$1" in nix|bwrap) return 1 ;; esac; command -v "$1" >/dev/null 2>&1; }
install_nix() { return 1; }
NIX_PREF=1 check_nix_engine >/dev/null || { echo "a failed Nix install ended check_nix_engine" >&2; exit 1; }
NIX_PREF=0 check_nix_engine >/dev/null
# With Docker present, RFSWIFT_ENGINE also answers "install Podman as well?".
detect_container_engines() { HAS_DOCKER=true; HAS_PODMAN=false; }
prompt_yes_no() { echo "unexpected prompt: $1" >&2; exit 1; }
second=""; install_podman() { second=podman; return 0; }
ENGINE_PREF=both check_container_engine >/dev/null
[ "$second" = podman ] || { echo "RFSWIFT_ENGINE=both did not add Podman next to Docker" >&2; exit 1; }
second=""; ENGINE_PREF=docker check_container_engine >/dev/null
[ -z "$second" ] || { echo "RFSWIFT_ENGINE=docker installed Podman" >&2; exit 1; }
install_podman() { return 1; }
ENGINE_PREF=podman check_container_engine >/dev/null || { echo "a failed second-engine install ended check_container_engine" >&2; exit 1; }
# Restore the script's own definitions for the tests that follow.
. "$ROOT/get_rfswift.sh"

# A CLI that cannot install the udev rules (no udev daemon: WSL, containers)
# must reach the fallback message, not end the installer.
if [ "$(uname -s)" = Linux ]; then
  fake2="$tmp/fakebin2"; mkdir -p "$fake2"
  printf '#!/bin/sh\nexit 1\n' > "$fake2/rfswift"; chmod +x "$fake2/rfswift"
  NATIVE_INSTALLED=false INSTALL_DIR="$fake2" INSTALL_COMPONENTS=cli
  UDEV_RULES=1 offer_udev_rules >/dev/null 2>&1 || { echo "a failing 'rfswift host udev' ended offer_udev_rules" >&2; exit 1; }
fi

# Nerd Fonts need unzip; without it the step is skipped, not fatal, and
# nothing is downloaded.
command_exists() { [ "$1" = unzip ] && return 1; command -v "$1" >/dev/null 2>&1; }
curl() { echo "unexpected download without unzip" >&2; exit 1; }
wget() { curl; }
install_nerd_fonts_linux >/dev/null || { echo "a missing unzip ended install_nerd_fonts_linux" >&2; exit 1; }
command_exists() { command -v "$1" >/dev/null 2>&1; }
unset -f curl wget

# Root without the sudo package (Debian's `su -` route): sudo passes through.
out=$( id() { echo 0; }; command_exists() { [ "$1" = sudo ] && return 1; command -v "$1" >/dev/null 2>&1; }; provide_sudo_shim; sudo -v && sudo echo shim-ok )
[ "$out" = shim-ok ] || { echo "root without sudo: the shim did not pass the command through" >&2; exit 1; }

# No alias for a directory that is already on PATH; on Linux the alias goes
# to .bashrc (terminals open non-login shells), even when .bash_profile exists.
mkdir -p "$tmp/onpath"
(PATH="/usr/bin:$tmp/onpath:/bin"; dir_on_path "$tmp/onpath") || { echo "dir_on_path missed a PATH entry" >&2; exit 1; }
if (PATH="/usr/bin:/bin"; dir_on_path "$tmp/onpath"); then echo "dir_on_path matched a directory that is not on PATH" >&2; exit 1; fi
if [ "$(uname -s)" = Linux ]; then
  home="$tmp/home"; mkdir -p "$home"; : > "$home/.bash_profile"; : > "$home/.bashrc"
  getent() { return 1; }
  prompt_yes_no() { return 0; }
  HOME="$home" SHELL=/bin/bash create_alias "$tmp/bin" >/dev/null || { echo "create_alias failed" >&2; exit 1; }
  unset -f getent prompt_yes_no
  grep -q "alias rfswift='$tmp/bin/rfswift'" "$home/.bashrc" || { echo "alias was not written to .bashrc" >&2; exit 1; }
  if grep -q rfswift "$home/.bash_profile"; then echo "alias was written to .bash_profile on Linux" >&2; exit 1; fi
fi

echo "installer security, development-channel and native-package tests: ok"
