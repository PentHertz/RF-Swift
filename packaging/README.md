# Linux packaging

Native Linux packages for both binaries, attached to every GitHub release:

| Package             | Contents                                                        | deb (Debian/Ubuntu) | rpm (Fedora) | pacman (Arch) |
|---------------------|-----------------------------------------------------------------|---------------------|--------------|---------------|
| `rfswift`           | CLI/TUI binary, man pages, bash/zsh/fish completions            | amd64, arm64, riscv64 | amd64, arm64, riscv64 | amd64, arm64 |
| `rfswift-workbench` | GUI binary, .desktop entry, hicolor icons, AppStream metainfo   | amd64, arm64        | amd64, arm64 | amd64, arm64  |

## How they are built

Two paths, because the two binaries build differently:

- **`rfswift` (CLI/TUI)** is a static pure-Go binary, cross-compiled for every
  arch in one job. Its packages come from the `nfpms` section of
  `.goreleaser.yml`. A goreleaser before-hook
  (`scripts/generate-packaging-assets.sh`) generates the completions (via
  `rfswift completion <shell>`) and man pages (via `go run ./tools/genman`,
  a small in-repo generator with no extra dependencies) into
  `packaging/cli/generated/` (gitignored).
- **`rfswift-workbench` (GUI)** is a cgo binary that dynamically links
  gtk3 + webkit2gtk-4.1, so it is built natively per-arch in its own CI jobs
  (`.github/workflows/release.yml`). Each job then runs
  `go/rfswift-workbench/scripts/mkpackages.sh`, which drives
  [nfpm](https://nfpm.goreleaser.com) with
  `go/rfswift-workbench/packaging/nfpm.yaml`. The desktop entry in
  `go/rfswift-workbench/packaging/linux/` is also the one bundled into the
  AppImage.

Dependency policy: the CLI package has **no hard dependencies** (a container
engine or Nix is Recommended, not Depended on - the Nix engine needs no
daemon). The Workbench package depends on the gtk3/webkit2gtk-4.1 runtime
libraries; on deb the `t64` alternatives cover Debian 12/13 and
Ubuntu 22.04/24.04+.

All packages are covered by the release checksums and the Sigstore build
provenance attestation, like every other release artifact:
`gh attestation verify <file> --repo PentHertz/RF-Swift`.

`get_rfswift.sh` (the curl-pipe installer) prefers these native packages
automatically when it finds apt/dnf/yum/zypper/pacman plus sudo (Homebrew
cask on macOS), verifying checksums and, when `gh` is present, attestations
before installing; any miss falls back to its classic tarball flow. Stable
channel only - prerelease versions are tilde-mangled in package filenames.
`RFSWIFT_PKG_FORMAT=native|tarball` selects the path non-interactively, and
`scripts/test-installer.sh` pins the predicted filenames to the ones the
release pipeline actually produces.

## Building locally

```bash
# CLI packages (all formats, all arches) - needs Go only:
goreleaser release --snapshot --clean --skip=publish,announce

# Workbench packages - needs Go, gtk3/webkit2gtk-4.1 dev libs, nfpm, ImageMagick:
cd go/rfswift-workbench
make linux
scripts/mkpackages.sh build/bin/rfswift-workbench_linux_$(go env GOARCH) $(go env GOARCH)
```

## macOS

Two delivery paths, both from the signed + notarized universal `.dmg`:

- **The .dmg itself**: drag-install the Workbench, double-click
  "Install RF Swift CLI.command" for the CLI, and double-click
  "RF Swift Setup.command" to pick and install an engine (Docker Desktop,
  OrbStack, Podman, Lima with USB passthrough, or native Nix) plus
  XQuartz/PulseAudio. The setup script is `scripts/setup-macos.sh` - the
  macOS counterpart of the Windows installer's dependency bundle
  (`windows/installer/Bundle.wxs`), also runnable non-interactively
  (`RFSWIFT_SETUP_ENGINE=... RFSWIFT_SETUP_XQUARTZ=0|1 RFSWIFT_SETUP_AUDIO=0|1`).
- **Homebrew**: `brew install --cask penthertz/rfswift/rfswift` installs the
  CLI and the Workbench in one go. The cask is rendered and pushed to the
  `PentHertz/homebrew-rfswift` tap by `.github/workflows/macos-dmg.yml`
  after notarization succeeds, and points at the notarized `.dmg` so
  Homebrew's cask quarantine passes Gatekeeper cleanly (this is why there is
  deliberately no goreleaser brews/casks section - the tar.gz binaries are
  unsigned). Stable tags only; prereleases are skipped.

One-time setup for the tap (until then the publish step self-skips):

1. Create an empty public repo `PentHertz/homebrew-rfswift`.
2. Add a repo secret `HOMEBREW_TAP_GITHUB_TOKEN` in RF-Swift: a fine-grained
   PAT with contents read/write on `homebrew-rfswift` only.

## Not covered (on purpose)

- **openSUSE** names the WebKit package differently (`libwebkit2gtk-4_1-0`);
  its users are served by the AppImage. An OBS (Open Build Service) project
  could later build and host repos for Debian/Ubuntu/Fedora/openSUSE from one
  spec, giving users `apt`/`dnf` upgrades instead of manual downloads.
- **AUR**: the `.pkg.tar.zst` here installs directly with `pacman -U`; a
  proper `rfswift-bin` AUR PKGBUILD pointing at the release artifacts is the
  idiomatic Arch delivery and can be added once release naming is stable.
- **Snap/Flatpak**: their sandboxing conflicts with what RF Swift needs
  (USB/serial hardware, the container engine socket, host networking).
