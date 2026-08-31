#!/usr/bin/env bash
# Package a built rfswift-workbench Linux binary as .deb, .rpm and Arch
# .pkg.tar.zst with nfpm (https://nfpm.goreleaser.com), including the desktop
# entry, hicolor icons and AppStream metadata a desktop app needs.
#
# The CLI/TUI packages are produced by goreleaser's nfpms section instead;
# this script exists because the GUI is a cgo binary built natively per-arch
# in its own CI jobs (see .github/workflows/release.yml).
#
# Requirements on PATH: nfpm, and ImageMagick (`magick` or `convert`).
# Usage:   scripts/mkpackages.sh <path-to-binary> <goarch>
# Example: scripts/mkpackages.sh build/bin/rfswift-workbench_linux_amd64 amd64
# Env:     VERSION  package version without the leading v (default: derived
#          from `git describe`).
set -euo pipefail

BIN="${1:?usage: mkpackages.sh <path-to-binary> <goarch>}"
GOARCH="${2:?usage: mkpackages.sh <path-to-binary> <goarch>}"
OUT="build/bin"

command -v nfpm >/dev/null 2>&1 || {
  echo "missing tool: nfpm (go install github.com/goreleaser/nfpm/v2/cmd/nfpm@latest)" >&2
  exit 1
}
if command -v magick >/dev/null 2>&1; then
  IMAGE_CONVERT=(magick)
elif command -v convert >/dev/null 2>&1; then
  IMAGE_CONVERT=(convert)
else
  echo "missing tool: ImageMagick (magick or convert)" >&2
  exit 1
fi

# Hicolor icons at exactly the pixel sizes their theme directories promise.
ICON_DIR="build/package-icons"
rm -rf "$ICON_DIR"
mkdir -p "$ICON_DIR"
for size in 256 512; do
  "${IMAGE_CONVERT[@]}" build/appicon.png -resize "${size}x${size}!" \
    "$ICON_DIR/${size}x${size}.png"
done

# CI passes the release tag explicitly; local builds fall back to git.
if [ -z "${VERSION:-}" ]; then
  VERSION="$(git describe --tags --always 2>/dev/null | sed 's/^v//')"
fi

export NFPM_VERSION="$VERSION"
export NFPM_GOARCH="$GOARCH"
export NFPM_BINARY="$BIN"
export NFPM_ICON_DIR="$ICON_DIR"

mkdir -p "$OUT"
for format in deb rpm archlinux; do
  nfpm package -f packaging/nfpm.yaml -p "$format" -t "$OUT"
done
ls -l "$OUT"/rfswift-workbench*
