#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
WORKBENCH_DIR=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)

if ! command -v go >/dev/null 2>&1; then
  if command -v nix >/dev/null 2>&1 && [ "${RFSWIFT_SECURITY_IN_NIX:-0}" != 1 ]; then
    export RFSWIFT_SECURITY_IN_NIX=1
    exec nix --extra-experimental-features 'nix-command flakes' \
      shell nixpkgs#go nixpkgs#nodejs --command "$0" "$@"
  fi
  echo "error: Go is required (or install Nix so the script can provide it)" >&2
  exit 1
fi

cd "$WORKBENCH_DIR"
go test ./...
go vet ./...

if command -v node >/dev/null 2>&1; then
  node --check frontend/dist/app.js
  node scripts/frontend-security-test.mjs
fi

FUZZ_TIME=${FUZZ_TIME:-5s}
go test ./internal/workbench -run '^$' -fuzz '^FuzzValidWorkspaceName$' -fuzztime "$FUZZ_TIME"
go test ./internal/workbench -run '^$' -fuzz '^FuzzProjectArchiveEntryPath$' -fuzztime "$FUZZ_TIME"

cd "$WORKBENCH_DIR/../rfswift"
go test ./...
go vet ./...
go test ./remote -run '^$' -fuzz '^FuzzDecryptPrivateKeyPEM$' -fuzztime "$FUZZ_TIME"
go test ./remote -run '^$' -fuzz '^FuzzCertificateFingerprint$' -fuzztime "$FUZZ_TIME"
