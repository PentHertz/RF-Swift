# RF Swift Workbench (GUI)

A research and assessment workspace GUI for RF Swift, built with Wails (Go).
It is a **separate binary and module** from the lean `rfswift` CLI on purpose:
the GUI needs cgo + an OS webview (WebView2 / WKWebView / WebKitGTK), which the
CLI and a future `rfswift agent` must not carry. Today the GUI drives the local
RF Swift engines in-process; the remote-agent protocol remains roadmap work.

Design docs: `../../docs/workspace-gui.md`, `../../docs/remote-agent.md`,
`../../docs/ai-assistant.md`.

## Why a separate module

- The CLI/agent stays lean, static and cgo-free (runs on a headless Raspberry Pi).
- The GUI's Wails/webview dependency is isolated here and never enters the CLI's
  dependency graph.
- Shared Go code is reused through a module `replace` (see "Wiring the engine").

## Layout

```
main.go                    executable and Wails bootstrap; embeds frontend/dist
main_<platform>.go         platform-specific webview bootstrap
internal/workbench/        private Workbench application package
  app.go                   App lifecycle and dependency wiring
  bindings.go              Wails-facing API methods
  engine.go                local and remote engine adapters
  store.go                 workspace persistence
  terminal.go              PTY sessions and recordings
  agent*.go, mcp.go        coding-agent and mission-scoped MCP integration
  artifacts.go             workspace evidence discovery and registration
  report.go                reports and PwnDoc conversion
  project_archive.go       complete project import/export
  types.go                 application transport models
  *_test.go                white-box unit and security-invariant tests
tests/integration/          public-API and cross-component black-box tests
frontend/dist/             embedded dependency-free UI and pinned assets
scripts/                   build, packaging, and frontend-maintenance scripts
build/bin/                 generated binaries (ignored by Git)
```

The executable imports `internal/workbench`; the Go `internal` boundary keeps
GUI implementation details private to this module. Wails therefore exposes the
bound object as `window.go.workbench.App`. The frontend retains a temporary
fallback for the former `window.go.main.App` namespace so an older generated
binding cannot leave the window blank during an incremental rebuild.

Go unit tests that intentionally inspect unexported storage, PTY, MCP, and path
validation internals remain beside `internal/workbench`. Tests that need only
the supported public API live separately under `tests/integration`; production
internals are not exported merely to move a test file.

## Build and run

Prerequisites: Go, plus the platform's webview build toolchain. **On Linux you
must build against your system's GTK/WebKit** (a binary linked to a different
WebKit - e.g. one built inside Nix - can fail to initialise GL and render a blank
window on your GPU/driver).

One command detects your distro and installs everything:

```bash
make deps        # runs scripts/install-deps.sh (apt/dnf/pacman/zypper/apk)
```

Or install manually:

```bash
# Debian/Ubuntu:
sudo apt-get install -y build-essential pkg-config libgtk-3-dev libwebkit2gtk-4.1-dev
# Fedora:
sudo dnf install -y gcc pkg-config gtk3-devel webkit2gtk4.1-devel
# Arch:
sudo pacman -S --needed base-devel pkgconf gtk3 webkit2gtk-4.1
```

macOS/Windows need no extra packages - the webview ships with the OS. The Wails
CLI does NOT need to be installed or on your PATH: the `Makefile` runs it via
`go run` (pinned to Wails v2.15.0) when `wails` is absent.

```
cd go/rfswift-workbench
make linux_amd64     # or: make windows_amd64 / make darwin_amd64 / make all
```

Prefer the CLI for development (hot reload)? Install it and it will be used
automatically:

```
go install github.com/wailsapp/wails/v2/cmd/wails@v2.15.0   # adds $(go env GOPATH)/bin to PATH
wails doctor                                                # check the webview toolchain
wails dev -tags webkit2_41                                  # hot-reload dev
```

Verified on Go 1.27: `make linux_amd64` (webkit2gtk-4.1) and `make windows_amd64`
(cross-built from Linux, no cgo) both build with only Go present - Wails is
fetched via `go run`. The build also generates the typed JS bindings under
`frontend/wailsjs/`.

## Release matrix (`make`)

The CLI is pure Go (`CGO_ENABLED=0 -tags netgo`) so it links into a single
static binary everywhere. A webview app cannot be a static ELF on Linux (it
links GTK + WebKit), but the same goal - one portable, self-contained artifact
per OS/arch - is met per platform:

- `make windows` -> self-contained `.exe` (WebView2 ships with the OS); no cgo,
  cross-buildable from any host, so it is as portable as the CLI.
- `make darwin` -> self-contained `.app` (WKWebView ships with the OS); build on
  macOS.
- `make linux` -> dynamic binary linking gtk3 + webkit2gtk-4.1; then
  `make appimage` bundles it into a single-file **AppImage** that runs across
  distros, plus the plain binary for distro packages.

Cross-compiling cgo targets is limited: build macOS on macOS and Linux on Linux
(native CI runners). Windows needs no cgo and builds from anywhere. See the
`Makefile` and `.github/workflows/workbench.yml`.

## Running

Released builds can be installed together with the CLI:

```bash
# Interactive selection of stable/dev, CLI/Workbench/both, and Linux package.
curl -fsSL https://get.rfswift.io/ | sh

# Non-interactive v4 development channel with the portable Linux GUI.
curl -fsSL https://get.rfswift.io/ | \
  RFSWIFT_CHANNEL=dev RFSWIFT_INSTALL=both \
  RFSWIFT_WORKBENCH_FORMAT=appimage sh
```

`v4.0.0-dev` is a GitHub prerelease. RF-Swift-nix has its own independent
`v1.0.0-dev` version and is consumed from that repository's `main` branch.
Installer verification, archive-extraction controls, and residual trust are
documented in [`docs/installer-security.md`](../../docs/installer-security.md).

The built binary runs standalone like any GTK app - no Nix wrapper needed. A
webview app cannot be a single static binary like the CLI (it dynamically links
GTK + WebKit), so its runtime dependencies are:

- Linux: `gtk3` and `webkit2gtk-4.1` (install via your package manager; on
  Debian/Ubuntu `libgtk-3-0 libwebkit2gtk-4.1-0`). A binary built inside the Nix
  dev shell resolves these via its embedded RPATH on the build host; to ship to
  other machines, build against the system libraries so it uses the system's
  gtk3/webkit2gtk (declared as package dependencies).
- macOS / Windows: `wails build` produces a native `.app` / `.exe`; the webview
  (WKWebView / WebView2) ships with the OS.

Linux rendering: a blank window is almost always the WebKitGTK renderer. The
Linux build already sets `WEBKIT_DISABLE_DMABUF_RENDERER=1` and
`WEBKIT_DISABLE_COMPOSITING_MODE=1` (see `main_linux.go`). If a GPU/Wayland/VM
still shows a blank or an EGL error, run with `LIBGL_ALWAYS_SOFTWARE=1` (software
rendering) and/or `GDK_BACKEND=x11`.

## Engine integration

The Workbench links the existing RF Swift Go module directly rather than
launching the CLI and parsing terminal output:

```go
require penthertz/rfswift v0.0.0
replace penthertz/rfswift => ../rfswift
```

`LocalEngine` uses `penthertz/rfswift/dock` for Docker, Podman and Lima target
discovery, inspection, lifecycle, command execution and container audits. It
uses `penthertz/rfswift/nix` for persisted native environments, Nix audits and
non-interactive commands with eager, lazy and pure environment semantics.

Nix environments are native closures, not daemons, so the GUI intentionally
does not display a start/stop lifecycle for them. Their tools execute through
the console in the environment's workspace and with the same profile/shim PATH
as the CLI.

USB passthrough follows the CLI too. On macOS the **USB passthrough...** dialog
hot-plugs host devices into the Lima VM (`rfswift macusb`). On Windows it drives
usbipd-win for Docker/Podman missions (`rfswift usb`): devices are listed from
`usbipd state`, attach/detach run unprivileged, and a device that was never
shared is bound through one UAC prompt for `usbipd.exe` - the GUI never runs a
shell or asks for a blanket elevation. The manual probe
`RFSWIFT_ENGINE_PROBE=1 go test ./internal/workbench -run TestUSBProbeManual -v`
prints what the backend sees on the current host.

Display and sound on Windows come from WSLg: `dock.CreateContainer` mounts the
WSLg tree (`/run/desktop/mnt/host/wslg` on Docker Desktop, `/mnt/wslg` on a
Podman machine) into the container with `DISPLAY=:0` and
`PULSE_SERVER=unix:/mnt/wslg/PulseServer`, exactly like the CLI. The mission
form also shows a live USB reachability line (`/dev/bus/usb` + `c 189:* rwm`,
no privileged mode needed) and asks before creating a container that cannot
reach forwarded devices.

The remote-agent cards shown by the original prototype are no longer presented
as real connections. Only the in-process local engine is selectable until the
planned `rfswift agent` transport, TLS 1.3 authentication and remote `Engine`
exist.

## Frontend

`frontend/dist/index.html` is a dependency-free UI. In the desktop application
it binds mission discovery/inspection/lifecycle, console execution, audits,
workspaces, notes, findings, capture imports, reports, pwndoc, connection audits
and external-agent/MCP configuration to `window.go.workbench.App.*`. Theme and panel-layout
preferences remain in `localStorage` because they are presentation-only.

### Mission secrets

The **Secrets** panel collects credentials observed during an authorized
mission. Add values manually, or choose **Find with AI** to make the connected
mission-scoped agent inspect all Markdown notes, AI-approved text captures and
artifacts, and completed terminal recordings. The agent must preserve an exact
source and may only store values that are actually present; partial, redacted,
example, and speculative credentials are rejected by the workflow instructions.

Secret values are stored in the native OS credential vault (Keychain on macOS,
Credential Manager on Windows, and Secret Service/keyring on Linux). Only
masked metadata and provenance are written to `secrets.json`. Values are not
included in project exports, reports, findings, notes, or `read_evidence_index`.
MCP exposes metadata through `list_secrets`; `save_secret` exists only when the
operator enabled mission-scoped write access. Reveal and copy remain explicit
GUI actions. Deleting a secret removes both its metadata and vault entry, and a
project cannot be deleted if its vault entries cannot first be removed.

The file retains sample data solely as a browser-preview fallback. As soon as
Wails injects the backend, live targets replace the sample list and persistent
assessment data comes from `~/.rfswift/workspaces`. `wails build` generates the
typed JS bindings under `frontend/wailsjs/`.
