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

RELEASE_CHANNEL=dev
get_latest_release
[ "$VERSION" = "4.0.0-dev" ]
[ "$DOWNLOAD_BASE_URL" = "https://github.com/PentHertz/RF-Swift/releases/download/v4.0.0-dev" ]

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

echo "installer security, development-channel and native-package tests: ok"
