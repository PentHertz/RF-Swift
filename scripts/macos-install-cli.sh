#!/bin/bash
# This code is part of RF Swift by @Penthertz
# Author(s): Sébastien Dudek (@FlUxIuS)
#
# Installs the rfswift CLI to /usr/local/bin. Shipped in the release .dmg as
# "Install RF Swift CLI.app": a signed launcher (go/rfswift/cmd/dmglauncher)
# opens this script in Terminal with RFSWIFT_DMG_ROOT set to the image root,
# where the universal rfswift binary sits. Run by hand, it looks for the
# binary next to itself instead.
set -e

SRC="${RFSWIFT_DMG_ROOT:-$(cd "$(dirname "$0")" && pwd)}/rfswift"
DEST="/usr/local/bin"

if [ ! -f "$SRC" ]; then
    echo "rfswift was not found at $SRC."
    echo "Run this from the opened RF Swift disk image, next to the rfswift binary."
    exit 1
fi

echo "Installing rfswift from $SRC to $DEST (sudo password may be asked)..."
sudo mkdir -p "$DEST"
sudo install -m 0755 "$SRC" "$DEST/rfswift"
echo ""
echo "Done. Open a new terminal and run: rfswift --help"
echo "For the GUI, drag rfswift-workbench.app onto Applications."
echo "To pick and install an engine (Docker Desktop, OrbStack, Podman, Lima"
echo "or Nix) plus XQuartz and audio, double-click \"RF Swift Setup\" next."
echo "Docs: https://rfswift.io/docs/getting-started/"
