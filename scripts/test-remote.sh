#!/usr/bin/env bash
set -euo pipefail

mode="${1:-all}"
fuzz_time="${RFSWIFT_FUZZ_TIME:-10s}"
root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
module_dir="${root_dir}/go/rfswift"

run_go() {
  if command -v go >/dev/null 2>&1; then
    (cd "${module_dir}" && go "$@")
  elif command -v nix >/dev/null 2>&1; then
    nix --extra-experimental-features 'nix-command flakes' shell nixpkgs#go -c sh -c 'cd "$1" && shift && go "$@"' sh "${module_dir}" "$@"
  else
    echo "error: Go or Nix is required" >&2
    exit 1
  fi
}

case "${mode}" in
  unit)
    run_go test -race ./remote ./cli
    ;;
  fuzz)
    run_go test ./remote -run '^$' -fuzz '^FuzzDecryptPrivateKeyPEM$' -fuzztime "${fuzz_time}"
    run_go test ./remote -run '^$' -fuzz '^FuzzCertificateFingerprint$' -fuzztime "${fuzz_time}"
    ;;
  all)
    "$0" unit
    "$0" fuzz
    ;;
  *)
    echo "usage: $0 [unit|fuzz|all]" >&2
    exit 2
    ;;
esac
