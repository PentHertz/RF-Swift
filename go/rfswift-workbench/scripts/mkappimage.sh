#!/usr/bin/env bash
# Package a built rfswift-workbench Linux binary into a portable AppImage that
# bundles gtk3 + webkit2gtk, so it runs across distros without installing deps.
#
# Requirements on PATH: linuxdeploy, linuxdeploy-plugin-gtk.sh, and either
# ImageMagick's `magick` or `convert` for deterministic icon sizing.
# Usage: scripts/mkappimage.sh build/bin/rfswift-workbench_linux_amd64
set -euo pipefail

BIN="${1:?usage: mkappimage.sh <path-to-binary>}"
APP="rfswift-workbench"
ARCH="${ARCH:-x86_64}"
APPDIR="build/${APP}.AppDir"

for tool in linuxdeploy linuxdeploy-plugin-gtk.sh; do
  command -v "$tool" >/dev/null 2>&1 || { echo "missing tool: $tool (see https://github.com/linuxdeploy)"; exit 1; }
done

if command -v magick >/dev/null 2>&1; then
  IMAGE_CONVERT=(magick)
elif command -v convert >/dev/null 2>&1; then
  IMAGE_CONVERT=(convert)
else
  echo "missing tool: ImageMagick (magick or convert)" >&2
  exit 1
fi

rm -rf "$APPDIR"
mkdir -p "$APPDIR/usr/bin" "$APPDIR/usr/share/applications" "$APPDIR/usr/share/icons/hicolor/256x256/apps"
cp "$BIN" "$APPDIR/usr/bin/$APP"

# Same desktop entry as the distro packages (packaging/nfpm.yaml) - one
# canonical file for both delivery paths.
cp "packaging/linux/$APP.desktop" "$APPDIR/usr/share/applications/$APP.desktop"

# linuxdeploy validates that the pixel dimensions match the icon theme path.
if [ -f "build/appicon.png" ]; then
  "${IMAGE_CONVERT[@]}" build/appicon.png -resize 256x256! \
    "$APPDIR/usr/share/icons/hicolor/256x256/apps/$APP.png"
else
  echo "missing application icon: build/appicon.png" >&2
  exit 1
fi

export ARCH
# The gtk plugin bundles gtk3 + its loaders; webkit is pulled in as a dependency
# of the binary. WEBKIT env workarounds are already compiled into the binary.
linuxdeploy --appdir "$APPDIR" --plugin gtk \
  --desktop-file "$APPDIR/usr/share/applications/$APP.desktop" \
  --icon-file "$APPDIR/usr/share/icons/hicolor/256x256/apps/$APP.png" \
  --output appimage

echo "AppImage written next to this script's working directory."
