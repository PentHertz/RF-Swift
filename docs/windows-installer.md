# Windows installer (MSI + dependency bundle)

RF Swift ships two Windows deliverables, built and signed by the release
pipeline (`.github/workflows/windows-installer.yml`):

| Artifact | What it is | Who it is for |
|----------|------------|---------------|
| `RFSwift-<version>-<arch>.msi` | RF Swift itself: the `rfswift` CLI/TUI and the Workbench GUI, on the system `PATH`, with a Start Menu shortcut. Installs **no** dependencies. | Enterprise deployment (Intune, GPO, SCCM) where WSL 2 and the container engine are managed centrally; anyone who already has the prerequisites. |
| `RFSwift-Setup-<version>-<arch>.exe` | A WiX **Burn** bundle that installs the prerequisites the user selects and then the MSI, all under a **single** UAC elevation. | Everyone else — the one-click path from a fresh Windows machine to a working RF Swift. |

Both are `x64` and `arm64`.

## Why a bundle and not one big MSI

RF Swift on Windows drives containers that run **inside the WSL 2 virtual
machine** (Docker Desktop / Podman machine). That imposes a dependency stack the
CLI cannot install for itself:

| Dependency | Why RF Swift needs it | Source |
|------------|-----------------------|--------|
| **WSL 2 + WSLg** | Containers live in the WSL 2 VM. WSLg supplies the X11 and PulseAudio sockets RF Swift mounts into every container for GUI/audio (`DISPLAY=:0`, `PULSE_SERVER=unix:/mnt/wslg/PulseServer`). | `wsl.exe --install` / Windows optional features |
| **Container engine** | Actually runs the images. Docker Desktop or Podman Desktop. | vendor installer |
| **usbipd-win** | Forwards host USB devices (SDR dongles, HackRF, RTL-SDR, Proxmark, …) into the WSL 2 VM so containers can use them. This is the whole point of RF Swift on Windows. | usbipd-win MSI |

The Windows Installer engine cannot nest one MSI inside another, and running
external installers from an MSI custom action is fragile and elevates
repeatedly. A Burn bundle is the supported way to chain several installers
(the usbipd MSI, the engine's own installer, the RF Swift MSI) behind one
elevation and one progress UI. That directly serves the two goals here: let the
user **choose** what to install, and make them **approve once**.

## What the user chooses

The bundle presents these options on its install page (see `Bundle.wxs` and the
custom `theme/`), each wired to a Burn variable that gates the matching package:

- **Enable WSL 2 + WSLg** — default on; skipped automatically when WSL 2 is
  already present.
- **usbipd-win (USB passthrough)** — default on; the reason to run RF Swift on
  Windows at all. Skipped when already installed.
- **Container engine** — a picker: **Docker Desktop** (default), **Podman
  Desktop**, or **I already have one** (install neither). Skipped when the
  chosen engine is already present.
- **Set up Nix in WSL 2** — optional, default off. Provisions a WSL 2 Linux
  distro with Nix (flakes) and the Linux `rfswift` CLI for the native,
  container-free engine (`rfswift run --engine nix`). See below.
- **RF Swift CLI + Workbench** — always installed (this is the product).

Everything the user leaves selected installs under the single elevation the
bundle already holds. Detection conditions mean re-running the bundle on a
machine that already has a piece is a no-op for that piece, so the bundle
doubles as a repair/top-up tool.

### One-time USB elevation is by design, not this installer

Sharing a USB device the first time (`usbipd bind`) needs administrator rights
**once per device**; attaching/detaching never does. RF Swift raises that UAC
prompt for `usbipd.exe` itself at runtime (see `rfutils/winusb.go`). The
installer does not and cannot remove that prompt — it is the security boundary
that keeps day-to-day `rfswift usb attach` unprivileged.

## Docker Desktop licensing

Docker Desktop requires a **paid Docker subscription** for use in larger
organizations (per Docker's licensing terms). The bundle therefore *offers* it
rather than forcing it, and ships **Podman Desktop** as a fully open-source
alternative that RF Swift supports equally (`--engine podman`). Pick the engine
that matches your licensing. Choosing "I already have one" installs no engine.

## Nix through WSL 2 (native environments, no containers)

RF Swift's Nix engine (`--engine nix`) runs tools in native environments instead
of containers, and it is **Linux-only** — on Windows it runs the Linux `rfswift`
inside a WSL 2 distribution. The optional **Set up Nix in WSL 2** bundle step
(`SetupNixWSL.ps1`) provisions that for you:

1. Installs a WSL 2 Linux distribution (Ubuntu) if none exists.
2. Enables systemd in it, so the Nix daemon runs cleanly.
3. Installs Nix with flakes via the **Determinate** installer (the path the RF
   Swift docs endorse; see [nix-engine.md](nix-engine.md)) and the Linux
   `rfswift` CLI.

After it finishes:

```powershell
wsl -d Ubuntu
rfswift run --engine nix -i sdr_light -n lab   # inside WSL
```

This step is **best-effort and optional**: it is off by default, and any
failure is logged without rolling back the rest of the install (the container
path is unaffected). It downloads Nix and RF Swift from the network at install
time, so it needs connectivity; on an air-gapped machine, skip it and set Nix up
manually inside WSL. Set `RFSWIFT_WSL_DISTRO` before running the bundle to target
a distribution other than `Ubuntu`.

## The MSI on its own

The MSI is self-contained RF Swift with a standard feature tree. For unattended
enterprise deployment:

```powershell
# Everything, silent:
msiexec /i RFSwift-<version>-x64.msi /qn /norestart

# CLI only (no Workbench GUI):
msiexec /i RFSwift-<version>-x64.msi /qn ADDLOCAL=CliFeature

# Log an install for troubleshooting:
msiexec /i RFSwift-<version>-x64.msi /qn /l*v rfswift-install.log
```

Feature IDs are `CliFeature` and `WorkbenchFeature`. The CLI feature adds the
install directory to the **system** `PATH`. The MSI installs to
`%ProgramFiles%\RF Swift`.

Prerequisites (WSL 2, an engine, usbipd-win) are **not** handled by the bare MSI
— manage them through your normal channels, or use the bundle.

## Trust and provenance

The Windows installer follows the same model as the macOS `.dmg` and the shell
installer (see [installer-security.md](installer-security.md)):

1. Every dependency the bundle downloads is pinned to an exact version and
   verified against a **SHA-256 hash** baked into `Bundle.wxs`. A hash mismatch
   aborts the install before the payload runs.
2. Release artifacts (`.msi`, `.exe`) carry a Sigstore-backed **build
   provenance attestation**. Verify with the GitHub CLI:
   ```powershell
   gh attestation verify RFSwift-Setup-<version>-x64.exe --repo PentHertz/RF-Swift
   ```
3. When the Authenticode signing secrets are configured (see below), the MSI and
   the bundle are code-signed with the PentHertz certificate and RFC-3161
   timestamped, so SmartScreen and enterprise app-control policies accept them
   without a right-click workaround. Forks and unsigned dry runs still build; the
   artifacts are simply unsigned.

### Residual trust

A pinned hash protects against a tampered or corrupted download but not against
a compromise of both the upstream release **and** the value in `Bundle.wxs`.
The dependency installers (Docker Desktop, Podman Desktop, usbipd-win) are
themselves vendor-signed; Windows verifies their Authenticode signatures when
they run. Review `Bundle.wxs` before trusting a build you did not cut yourself,
and prefer attested official releases for sensitive deployments.

## Building it yourself

Everything lives in `windows/installer/` and builds with one script (WiX v5 is
provisioned automatically via the .NET tool):

```powershell
# From a folder that has rfswift.exe and rfswift-workbench.exe:
cd windows\installer

# MSI only:
./build.ps1 -Version 4.0.1-dev -Arch x64 -BinDir C:\path\to\bin

# MSI + dependency bundle (x64):
./build.ps1 -Version 4.0.1-dev -Arch x64 -BinDir C:\path\to\bin -Bundle
```

The bundle is built for both `x64` and `arm64`. Build the **arm64** bundle on an
**arm64 Windows host** (`-Arch arm64 -Bundle`): each bundle carries a copy of the
host's `powershell.exe` for its WSL/Nix prerequisite steps, so it must match the
target architecture. CI does this by running the arm64 leg on a `windows-11-arm`
runner. The MSI itself cross-builds fine for either arch on any host.

Outputs land in `windows/installer/build/`. The first bundle build downloads the
dependency installers into `windows/installer/deps/` and prints their SHA-256 so
you can pin them in the `$deps` table at the top of `build.ps1`. The pinned
versions and download URLs live in `Bundle.wxs` (documented there) and are
mirrored by `build.ps1`; bump them together.

### Signing

Signing is off unless you pass a certificate; the artifacts still build
(unsigned) without one. Locally:

```powershell
./build.ps1 -Version 4.0.1 -Arch x64 -BinDir .\bin -Bundle `
    -CertFile mycert.pfx -CertPassword $env:CERT_PW
# or a cert already in the machine store:
./build.ps1 ... -CertThumbprint <THUMBPRINT>
```

`build.ps1` signs the MSI, then the Burn engine and the outer bundle separately
(`wix burn detach` / `reattach`) so the engine cached for repair/uninstall is
itself signed and SmartScreen stays quiet.

In CI, add two repository secrets and the release pipeline signs automatically:

| Secret | Value |
|--------|-------|
| `WINDOWS_CERT_BASE64` | base64 of the code-signing `.pfx` (`[Convert]::ToBase64String([IO.File]::ReadAllBytes('cert.pfx'))`) |
| `WINDOWS_CERT_PASSWORD` | the password protecting the `.pfx` |

Without them, `.github/workflows/windows-installer.yml` still produces unsigned
artifacts (forks, dry runs). Trigger a dry run from the Actions tab
("Windows installer") to build without attaching to a release.

## Icons

The MSI Add/Remove-Programs icon, the Workbench shortcut, the bundle
executable, and the bundle UI all use the official RF Swift icon and logo from
[rfswift.io](https://rfswift.io/) (`windows/installer/assets/rfswift.ico`,
`rfswift-logo.png`).
