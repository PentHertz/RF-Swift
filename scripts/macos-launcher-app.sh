#!/usr/bin/env bash
# This code is part of RF Swift by @Penthertz
# Author(s): Sébastien Dudek (@FlUxIuS)
#
# Wraps a shell script into a minimal macOS app bundle behind the dmglauncher
# executable (go/rfswift/cmd/dmglauncher). This is how the double-clickable
# helpers in the release .dmg are built: "Install RF Swift CLI.app" and
# "RF Swift Setup.app" (.github/workflows/macos-dmg.yml).
#
# Why an app and not a .command file: since macOS 15 Gatekeeper assesses a
# double-clicked script out of a downloaded image, and a notarization ticket
# only ever lists Mach-O code, so a bare script is refused ("Apple could not
# verify ... is free of malware") even when it is signed and the image around
# it is notarized. The launcher binary IS listed by the image's notarization,
# and the script travels as a resource the bundle's signature seals.
#
# Usage:
#   scripts/macos-launcher-app.sh <out.app> <bundle-id> <launcher> <script> <version> [icon.icns]
#
#   out.app     bundle to create; its basename minus .app is the app name
#   bundle-id   CFBundleIdentifier, e.g. com.penthertz.rfswift.setup
#   launcher    the (universal) dmglauncher Mach-O
#   script      the shell script the launcher runs in Terminal
#   version     x.y.z[-suffix]; CFBundleShortVersionString takes the x.y.z part
#   icon.icns   optional icon, installed as AppIcon.icns
#
# Signing (codesign --options runtime on the bundle) and notarization are the
# caller's job; the workflow does both after staging.
set -euo pipefail

if [[ $# -lt 5 || $# -gt 6 ]]; then
    echo "usage: $0 <out.app> <bundle-id> <launcher> <script> <version> [icon.icns]" >&2
    exit 2
fi
out="$1"; bundle_id="$2"; launcher="$3"; script="$4"; version="$5"; icon="${6:-}"

[[ "$out" == *.app ]] || { echo "error: $out must end in .app" >&2; exit 2; }
[[ -f "$launcher" ]] || { echo "error: launcher not found: $launcher" >&2; exit 2; }
[[ -f "$script" ]] || { echo "error: script not found: $script" >&2; exit 2; }
[[ -z "$icon" || -f "$icon" ]] || { echo "error: icon not found: $icon" >&2; exit 2; }
name="$(basename "$out" .app)"
short_version="${version%%-*}"

xml_escape() { sed -e 's/&/\&amp;/g' -e 's/</\&lt;/g' -e 's/>/\&gt;/g' <<<"$1"; }
plist_string() { printf '    <key>%s</key>\n    <string>%s</string>\n' "$1" "$(xml_escape "$2")"; }

rm -rf "$out"
mkdir -p "$out/Contents/MacOS" "$out/Contents/Resources"
install -m 0755 "$launcher" "$out/Contents/MacOS/$name"
# Not executable on purpose: the launcher runs it through bash, nothing
# execs it, and a plain file is one less thing for Gatekeeper to look at.
install -m 0644 "$script" "$out/Contents/Resources/main.sh"
if [[ -n "$icon" ]]; then
    install -m 0644 "$icon" "$out/Contents/Resources/AppIcon.icns"
fi

{
    printf '%s\n' \
        '<?xml version="1.0" encoding="UTF-8"?>' \
        '<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">' \
        '<plist version="1.0">' \
        '<dict>'
    plist_string CFBundleDevelopmentRegion en
    plist_string CFBundleExecutable "$name"
    plist_string CFBundleIdentifier "$bundle_id"
    plist_string CFBundleInfoDictionaryVersion 6.0
    plist_string CFBundleName "$name"
    plist_string CFBundleDisplayName "$name"
    plist_string CFBundlePackageType APPL
    plist_string CFBundleShortVersionString "$short_version"
    plist_string CFBundleVersion "$version"
    if [[ -n "$icon" ]]; then
        plist_string CFBundleIconFile AppIcon
    fi
    # Go 1.24+ binaries need macOS 12; the Gatekeeper behaviour this bundle
    # exists for starts with macOS 15.
    plist_string LSMinimumSystemVersion 12.0
    # No Dock icon and no menu bar: the launcher hands over to Terminal and
    # exits within milliseconds.
    printf '    <key>LSUIElement</key>\n    <true/>\n'
    plist_string NSHumanReadableCopyright "Copyright PentHertz. Licensed under the GNU GPL v3."
    printf '%s\n' '</dict>' '</plist>'
} > "$out/Contents/Info.plist"

# plutil is macOS-only; the script is also usable on Linux for staging tests.
if command -v plutil >/dev/null 2>&1; then
    plutil -lint "$out/Contents/Info.plist"
fi
echo "==> $out ($bundle_id, v$version)"
