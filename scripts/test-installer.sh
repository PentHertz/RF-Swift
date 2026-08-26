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
echo "installer security and development-channel tests: ok"
