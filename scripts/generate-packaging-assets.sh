#!/usr/bin/env bash
# Generate the arch-independent files shipped inside the rfswift Linux
# packages (deb/rpm/pacman): shell completions and man pages for the CLI/TUI.
# Output lands in packaging/cli/generated/ (gitignored); goreleaser's nfpms
# section picks the files up from there at release time.
#
# Requires only the Go toolchain. Run from anywhere in the repo.
set -euo pipefail

cd "$(dirname "$0")/.."
OUT="packaging/cli/generated"
rm -rf "$OUT"
mkdir -p "$OUT/completions" "$OUT/man"

(
  cd go/rfswift
  # main.go suppresses the ASCII banner for `completion`, so stdout is the
  # bare script each shell expects.
  go run . completion bash > "../../$OUT/completions/rfswift.bash"
  go run . completion zsh  > "../../$OUT/completions/_rfswift"
  go run . completion fish > "../../$OUT/completions/rfswift.fish"
  go run ./tools/genman "../../$OUT/man"
)

# deb/rpm policy wants man pages compressed; -n drops the gzip timestamp so
# the archives stay reproducible.
gzip -9 -n "$OUT"/man/*.1

echo "Packaging assets written to $OUT"
