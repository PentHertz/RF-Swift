# Changelog

All notable changes to RF Swift are recorded here. The format is based on
Keep a Changelog (https://keepachangelog.com), and the project aims to follow
semantic versioning. Dates are ISO-8601 (YYYY-MM-DD).

## [Unreleased] - v4.0.0-dev

Development toward the 4.0.0 "Nucleus" release. The Nix engine and its
environment flake (RF-Swift-nix) track a parallel v1.0.0-dev from its `main`
branch.

### Added

- Nix engine on Windows, through WSL 2. Nix has no Windows port, so the engine
  now lives inside a WSL 2 distribution that RF Swift provisions and drives:
  every Nix command typed in a Windows console (`rfswift run --engine nix`,
  `rfswift nix install`, `rfswift env update`, ...) is served by the Linux
  `rfswift` inside the distribution with the same wizards, builds and shells,
  and the `nix` Go package serves the Workbench the same way (state read over
  `\\wsl.localhost\`, builds and installs delegated, the mission terminal on
  `wsl.exe` under ConPTY), so Nix missions now exist in the Windows Workbench
  too. Radios forwarded with `rfswift usb attach` (offered before `run`,
  `exec` and `shell`, and from the Workbench's **USB passthrough...** action,
  now available on Nix missions on Windows), WSLg's display and sound, and its
  GPU libraries reach the environments; udev rules install without a password
  prompt (WSL grants root), from the CLI or the Workbench's **Install device
  rules**. New
  commands: `rfswift nix wsl status|setup|use|shell` (setup enables systemd,
  installs Nix with flakes and the Linux `rfswift` at the Windows version, and
  is offered automatically when a Nix command finds the distribution
  unprovisioned); `rfswift doctor` reports the backend; the Workbench's engine
  doctor shows it with a **Set up Nix in WSL 2** button. The installer's
  "Set up Nix in WSL 2" step now really enables systemd (its previous `cp`
  call had no destination), installs the Linux CLI of the same release and
  needs no interactive installer script. Distribution choice:
  `RFSWIFT_WSL_DISTRO`, `[nix] wsl_distro` in `config.ini`, else the default
  WSL 2 distribution.
- Nix engine: `rfswift nix tools <name>` lists an environment's on-demand shims
  and installed extras (`--installed`, `--json`), `rfswift nix search <term>`
  finds tools in the curated set or, with `--nixpkgs`, in the flake's whole
  pinned nixpkgs, `rfswift nix update --tool <tool> <name>` refreshes a single
  tool, `rfswift run --engine nix --create-only` realises an environment
  without entering it, `rfswift nix run --flake <ref> <tool>` runs a tool with
  an explicit flake, and `list`, `info`, `generations`, `gl` and `udev --list`
  gained `--json`. `rfswift --version` prints the version without touching the
  network.
- Nix engine: under WSL 2 the OpenGL runtime appends WSLg's GPU libraries
  (`/usr/lib/wsl/lib`) behind the nixpkgs ones, so Mesa's `d3d12` driver can
  reach the host GPU through WSLg's virtual GPU instead of falling back to
  llvmpipe; `rfswift nix gl` reports the WSL case. The `--isolate` jail binds
  WSLg's `/mnt/wslg` (display and sound sockets) and `/dev/dxg` back in, and
  now also the target of a `/etc/resolv.conf` symlink (systemd-resolved,
  resolvconf, NetworkManager), so name resolution works inside the jail on
  those hosts too - verified inside WSL 2 with an on-demand tool building
  from the jail.
- The ASCII banner is printed on interactive terminals only: not when stdout
  is a pipe (scripts, `--json` consumers) and not when `RFSWIFT_NO_BANNER` is
  set, which the Windows front end sets for the Linux rfswift it drives. A
  missing `config.ini` is created with the shipped defaults, without a
  prompt, when there is no terminal to answer one.
- Nix engine: `rfswift run --engine nix --isolate` enters the environment shell
  inside a bubblewrap jail (Linux). It hides the real `$HOME` and the rest of
  the host filesystem behind a private per-environment home, gives the shell its
  own PID/IPC/UTS namespaces and a private `/tmp`, while deliberately keeping
  what RF tooling needs: the `/nix` store, USB and serial devices
  (`/dev/bus/usb`, `/dev/tty{USB,ACM,S}*`), sysfs/udev for enumeration, the
  X11/Wayland display and the network. Verified against a real HydraSDR RFOne:
  the device stays visible to `lsusb` and opens `O_RDWR` (libusb) inside the
  jail, while `$HOME`/SSH keys and host processes are hidden. bubblewrap is
  taken from the host `PATH` or built from nixpkgs on demand. macOS is not
  supported (bubblewrap is Linux-only); `--isolate` there errors with guidance
  to use the container engine rather than running unisolated. The choice is
  offered everywhere the engine is: the `--isolate` flag, the interactive nix
  wizard, and an "Isolate (jail)" toggle in the Workbench create form. It is
  persisted on the environment, so `exec` and the Workbench terminal re-enter
  the same jail. The installer (`get_rfswift.sh`) now offers to install
  bubblewrap alongside the Nix engine. Inside the jail the home is genuinely
  empty (nothing is bound under it), the working directory is mounted at
  `/workspace` and RF Swift's state dir at `/rfswift`, with PATH, the shell
  rc file and the cwd remapped to match - so the shell shows only the
  workspace and the tools, not the host's files.
- Nix engine: `rfswift nix gl [name] --check` creates an OpenGL context with
  the runtime an environment gets (no window) and prints the driver that
  answered or the EGL error a GUI tool would hit; `rfswift nix gl` now also
  lists the host's GPUs (vendor and bound kernel driver) with the driver stack
  the runtime will use for each: Mesa for Intel, AMD, VMware/virtio and other
  open drivers, the matching proprietary libraries for NVIDIA, nothing needed
  on macOS.
- Nix engine: environment shells (CLI and Workbench) export
  `SOAPY_SDR_PLUGIN_PATH` with the profile's merged SoapySDR module
  directories and the extras profiles, so Soapy modules installed later with
  `rfswift nix install` are found by SDR++, gqrx, SigDigger, SatDump and
  rtl_433 as well.
- Nix engine: `rfswift nix udev <name>` installs the udev rules shipped by the
  environment's packages (HackRF, RTL-SDR, bladeRF, Airspy, LimeSDR, USRP,
  Proxmark, ...) into `/etc/udev/rules.d`, creates the groups they rely on and
  adds the user to them, all in one `sudo`; `--list` shows the state per rule
  and `--remove` takes them out again. `run --engine nix` lists the rules that
  are missing on the host and offers the installation before entering the
  shell, so devices no longer silently need root on native environments. The
  device prerequisite layer is now pinned under the environment directory so
  its rules are found for on-demand environments too.

- Workbench: the Configuration and Network cards now show the full container
  summary the CLI prints after run/exec (image version and freshness, size on
  disk, shell, X display, privileged mode, bind mounts, devices, extra hosts,
  seccomp profile, ulimits, GPUs, network mode, NAT subnet, exposed ports and
  port bindings) via the new `dock.ContainerSummaryFor`.
- Workbench: container creation reports what the engine could not honour (for
  example under rootless Podman) in a dialog after the mission is created,
  through `CreateOptions.Warn` and `Mission.Warnings`.
- Device mapping check before creation (`dock.CheckDeviceMappings`): the
  Workbench form warns under the device fields and asks before creating when
  the selected engine cannot map a device on this host, with a one-click
  "Remove unsupported devices"; `rfswift run` lists the same devices with the
  reason and asks once before dropping them. Covers rootless Podman (root-only
  nodes such as `/dev/console`, nodes the user cannot open such as
  `/dev/ttyACM0` without the dialout group), Docker/OrbStack/Podman machine on
  macOS (no USB, serial, audio or GPU passthrough into the VM; points to the
  Lima engine) and Lima (device absent from the VM).
- macOS signed disk image: `.github/workflows/macos-dmg.yml` builds a
  universal `rfswift` CLI and the universal Workbench `.app` on every `v*`
  tag, packs them into `rfswift_Darwin_universal.dmg` and, when the Apple
  secrets are configured, Developer ID signs, notarizes and staples the app
  and the image (hardened runtime, inside-out bundle signing, Sigstore
  provenance). Forks and dry runs get an unsigned image. The Workbench
  bundle now carries the `com.penthertz.rfswift-workbench` identifier via
  `go/rfswift-workbench/build/darwin/Info.plist`. Setup and troubleshooting
  in [docs/macos-signing.md](docs/macos-signing.md).
- Windows USB passthrough rebuilt on usbipd-win with the least privilege the
  tool allows. Containers on Windows run in the WSL 2 VM, so a host device is
  forwarded there; only *sharing* it the first time needs administrator
  rights, which RF Swift now requests through a single UAC prompt for
  `usbipd.exe` itself (never a shell), while attach and detach stay
  unprivileged.
  - CLI/TUI: `rfswift usb list|status|attach|detach|bind|unbind|vm-devices`
    (also under `winusb`) read the machine-readable `usbipd state` output.
    `attach`, `detach`, `bind` and `unbind` open an interactive device picker
    when no `--busid` is given, ask before raising the UAC prompt (`--yes`
    skips the question for scripts), warn before forwarding keyboard/mouse-like
    devices, and print friendly names for common RF hardware (RTL-SDR, HackRF,
    bladeRF, Proxmark, LimeSDR, USRP, ...).
  - `rfswift run` on Windows offers the same picker when it detects shared or
    known RF hardware, so a device can be forwarded before the container
    starts; plain keyboards and webcams never trigger the question.
  - `rfswift doctor` reports the usbipd-win version, connected/shared/attached
    counts and the default WSL 2 distribution.
- The Workbench **USB passthrough...** dialog now also works on Windows for
  Docker and Podman missions: it lists host devices with their usbipd state,
  shares and attaches in one click (administrator approval requested once per
  device), detaches, unshares, shows what WSL 2 currently sees, and can apply
  the USB hotplug defaults to a container being created.
- Pseudo-terminals on Windows through ConPTY (`ptyx` package, backed by
  creack/pty on Unix and UserExistsError/conpty on Windows). The remote agent
  can now serve interactive shells from a Windows lab machine (`terminal.start`
  previously failed with "unsupported"), and the Workbench's coding-agent
  terminal uses the same abstraction, so it runs on a Windows Workbench too.
  The agent's certificate generation, mTLS serving, control methods and
  command relay were verified on Windows with Credential Manager as the vault.
- Sound on Windows through WSLg. Containers on Docker Desktop and Podman
  machine now get `PULSE_SERVER=unix:/mnt/wslg/PulseServer` with WSLg's
  `/mnt/wslg` tree mounted (also for `--no-x11` containers), which is the
  PulseAudio server WSLg already runs for the WSL 2 VM - verified with `pactl`
  inside an RF Swift container. No PulseAudio for Windows and no
  `rfswift host audio enable` step: that command now just checks WSLg, and
  `rfswift doctor` reports the WSLg X11 and audio sockets by asking WSL instead
  of looking for Linux paths on the Windows filesystem. Workbench-created
  containers on Windows now receive the same WSLg display and audio mounts as
  CLI containers, and on Linux/macOS they get the CLI's `PULSE_SERVER` target.
- A USB reachability check when a container is created, shared by the CLI and
  the Workbench (`common.CheckUSBAccess`). Verified on Docker Desktop/WSL 2: a
  forwarded device is reachable only when `/dev/bus/usb` is mapped **and** USB
  device major 189 is allowed (`c 189:* rwm`); a bare bind mount lists the
  nodes but `open()` fails with "Permission denied", and privileged mode is not
  required. The mission form shows a live status line under **USB
  passthrough...** and asks before creating a container that cannot reach
  USB devices (one click applies the hotplug defaults); `rfswift run` prints
  the same warning before creation, and both say explicitly that
  `--privileged` is not needed.
- Native Linux, portable AppImage, Windows amd64/arm64, and universal macOS
  Workbench release artifacts with SHA-256 manifests and GitHub build-provenance
  attestations. Tags such as `v4.0.0-dev` are explicitly published as
  prereleases.
- `get_rfswift.sh` now offers stable/development channels and CLI, Workbench,
  or combined installation. Linux users can choose the smaller native build or
  a portable AppImage; automation can set `RFSWIFT_CHANNEL`,
  `RFSWIFT_INSTALL`, and `RFSWIFT_WORKBENCH_FORMAT`.

- A push/PR security gate for Workbench that runs backend and integration
  tests, `go vet`, frontend sink guards, and short Go fuzz campaigns against
  project archives, persisted identifiers, encrypted keys, and certificates.
  The same suite runs locally with `make security-test` or
  `scripts/security-test.sh` from `go/rfswift-workbench`.
- MCP evidence responses now carry an explicit `untrusted_evidence` trust
  label and instruct agents to treat notes, artifacts, recordings, filenames,
  metadata, embedded role text, and tool requests as evidence - not instructions.

### Security

- Installer downloads now fail closed on missing/mismatched checksums and
  reject absolute, parent-traversing, or link-bearing archive members before
  extraction. Installer guards are exercised in CI.

- Reject mission path traversal at both GUI and persistence boundaries, and
  prevent MCP write tools from creating or modifying nonexistent missions.
- Prevent the terminal player and note-image IPC methods from reading arbitrary
  local files or following managed-asset symlinks outside the Workbench store.
- Escape imported/remote mission metadata and custom capture icons before DOM
  insertion, and reject active or unsupported URI schemes in rendered Markdown
  links and images.
- Enforce archive expanded-size limits against bytes actually written, not only
  ZIP header declarations.
- Correct remote-agent endpoint parsing for IPv6 and validate certificate
  lifetime plus the CA chain when a fingerprint and CA are both configured.
- Stop advertising an agent rate limiter that the protocol does not implement,
  and bound the authenticated `/v1/info` response consumed by clients.

- Workbench panels can now be arranged by drag and drop into multiple rows as
  well as columns. Top/bottom drop bands create rows, left/right bands create
  columns, row heights and column widths are resizable, layouts persist per
  mission, and existing column-only layouts migrate automatically.
- The Notebook action and formatting bars now remain stacked and sticky at the
  top of their panel while long notes scroll, keeping view, media, export,
  heading, list, code, link, clear-style, and AI actions accessible.
- Saved remote-agent profiles are now listed directly in the Workbench
  connection panel with one-click **Connect**, **Edit**, and confirmed
  **Forget** actions.
  Forgetting a profile leaves certificate and encrypted private-key files
  untouched and does not silently terminate an active authenticated session.
- A local artifact decoder for live `/workspace` files, including binary-safe
  previews, hexadecimal/ASCII, text, strings and Base64 views, decimal or hex
  byte-range selection, maximization, and a pinned offline CyberChef v11.4.0
  workspace for stacking complete decoding recipes against the selected bytes.
  Its native decoder stack supports per-step offsets and lengths, reordering,
  extraction, bidirectional Hex/Base64URL/Base32/Base58/ASCII85 and numeric-byte
  codecs, URL/quoted-printable/HTML/JSON/UTF-16 codecs, XOR, byte arithmetic,
  endian swaps, reversal, ROT13, and gzip/zlib/raw-deflate compression.
  Selected byte ranges now open in the complete private CyberChef workspace in
  the system browser; the constrained embedded WebView has been removed.
- A structured MIFARE Classic dump view modelled on the NXP EV1 memory and
  access-condition tables: 1K/2K/4K sector layouts, manufacturer and value-block
  heuristics, sector keys, C1/C2/C3 permission descriptions, complement-bit
  validation, user bytes, and warnings for known default/common keys.
- A MIFARE Ultralight/EV1 and NTAG-compatible dump view with four-byte pages,
  7-byte UID and BCC validation, OTP and static/dynamic lock interpretation,
  user-memory boundaries, NDEF TLV/URI/Text summaries, EV1 authentication and
  configuration fields, PWD/PACK visibility warnings, and cautious size-based
  identification for MF0ICU1, MF0UL11, MF0UL21, Ultralight C and NTAG21x dumps.
- Workbench tool installation now uses theme-aware progress, success, warning,
  and failure cards, with green/red result notifications that remain legible in
  both light and dark themes.
- Per-client AI **YOLO mode** settings for Codex, Claude Code, Kimi Code and
  GLM CLI. The mode is off by default, visibly warned, and maps to each CLI's
  native approval-bypass flag without expanding RF Swift MCP permissions.
- The Workbench footer now identifies the build as **Community Edition** next
  to the version sourced from the shared RF Swift core.
- Transactional Nix environment lifecycle commands: `env update` (with
  `--check`, `--input`, and `--yes`), `env rebuild`, `env generations`, and
  `env rollback`. Successful builds retain the prior closure as a GC-rooted
  generation; failed local updates restore `flake.lock` and never replace the
  active profile. The legacy `nix` command path exposes the same operations.
- Running `rfswift env update` without a name now opens a guided, searchable
  wizard for environment, operation and flake-input selection, followed by a
  rollback-aware recap and confirmation.
- Canonical `rfswift env shell [name]` (`env enter`) command. Entering a Nix
  environment now prints a container-style summary with its exact workspace,
  flake, build mode, tool count, and native-user execution model, and warns when
  visible serial devices cannot be opened by the current login session.
- The interactive Nix creation wizard now opens its environment catalog with a
  visible, focused search field so large RF Swift catalogs can be filtered by
  typing immediately.
- Native Nix serial-device warnings now identify the affected device and group
  concisely and show both `newgrp` and full login-session refresh options.
- Workbench mission Secrets panel with manual credential entry, masked
  metadata/provenance, explicit reveal/copy/delete actions, and values held only
  by the native OS credential vault. Mission-scoped MCP agents can collect exact
  credentials from notes, approved captures/artifacts, and completed terminal
  recordings through the write-gated `save_secret` tool; secret values remain
  excluded from project exports, findings, reports, and evidence indexes.
- Completed GUI terminal recordings can be explicitly registered as AI-readable
  mission evidence and reviewed by the connected agent without manually pasting
  or confirming a prompt in the agent terminal.
- AI evidence reads now decode approved asciinema `.cast` recordings into a
  bounded, ANSI-free input/output transcript instead of exposing raw event JSON.
- Embedded terminal players now expose an explicit **Use as AI evidence** action
  that registers the recording and approves its decoded transcript without
  requiring the operator to return to the terminal recording library.
- Managed `.cast` recordings embedded in an AI-readable mission note are now
  registered automatically. Evidence-index reads also migrate older recording
  directives, so existing notes no longer resolve to metadata-only pointers.
- AI finding review can now persist candidates directly proven by approved
  evidence and collect exact observed credentials into the OS vault. Unproven
  leads remain unsaved, and secret values stay out of chat and reports.
- Findings now provide a dedicated **Collect credentials** action that grounds
  extraction in saved findings and approved evidence, then stores exact values
  in the native credential vault through the mission-scoped MCP bridge.
- The Note panel now includes a mission recording drawer for selecting and
  attaching recordings, renaming them, approving AI evidence, and opening an
  inline asciinema player without requiring the Console panel.
- Opening a capture or playing a recording no longer changes focus to the
  Console panel; command diagnostics remain available there if needed.
- Registered recording capture cards now open the built-in asciinema player in
  place and provide an **Add to note** action from the same card.
- Recording playback now opens in a focused modal with Play/Restart, Add to
  note, and AI-evidence approval actions instead of expanding a capture card.

- Remote-agent security core in the lean `rfswift` binary:
  - TLS 1.3-only transport, SHA-256 server-certificate pinning, and mandatory
    mutual TLS. A CA-verified client certificate is the authentication boundary.
  - `rfswift agent certs init` generates a private CA and server/client
    certificates. Every private key is a password-encrypted PKCS#8 file; random
    key passwords are referenced from the native OS vault and are not written
    into connection profiles.
  - Client private keys remain password-encrypted at rest. Their random
    decryption passwords stay in the native OS vault and are never sent to the
    agent; no separate network password or FIDO2 flow is required.
  - Local `scripts/test-remote.sh unit|fuzz|all` runner and push/PR CI fuzz smoke
    tests for encrypted-key parsing, malformed input, fingerprints, and policy
    downgrade rejection.
  - Workbench **Connection & security → Add agent** panel calls the shared
    certificate generator and connects through pinned TLS 1.3 + mTLS using an
    encrypted client key from the native vault. The authenticated control plane
    covers remote engines, terminals, audits, image operations and artifacts.
  - Existing-agent connection is reduced to agent address, pinned fingerprint,
    and one client-credential directory; filenames and the vault reference are
    inferred. An authenticated command panel can invoke the remote `rfswift`
    binary through a bounded JSON argument-array endpoint without a shell layer.
  - Unauthenticated clients are rejected during mTLS before HTTP. Unknown routes
    are closed without a response or route hint. Newly generated certificates use neutral public
    subjects without RF Swift/Penthertz branding or friendly agent names.

- Nix engine (`--engine nix`): run RF Swift tool sets as native, reproducible
  Nix environments instead of containers. Engine selectable via `--engine`, the
  `RFSWIFT_ENGINE` env var, or the config file (`[general] engine`).
  - `rfswift nix catalog` / `list` / `info` / `run` / `remove`.
  - `rfswift nix export` / `import`: pack an environment (closure + workspace)
    into a portable `.rfenv` archive and restore it elsewhere.
  - Lazy mode: build each tool the first time it is called.
- `rfswift nix gc`: reclaim disk by garbage-collecting unused store paths
  (created environments keep a gcroot and are never deleted). `--dry-run`,
  `--max <size>`.
- Security / vulnerability audit, surfaced across every engine:
  - `rfswift nix audit <env>`: vulnix (Nix-closure CVEs), syft SBOM, grype,
    osv-scanner, plus store-integrity, signature provenance and flake/config
    hygiene, via the flake `audit` app.
  - `rfswift images audit <ref>`: container-image CVEs with trivy (host binary
    if present, else trivy run as a container via the active engine using
    `save` + `--input`, working on Linux/macOS/Windows without a socket mount);
    host grype second opinion.
  - `rfswift audit <container>`: a container's attack surface - host exposure
    (--privileged, host net/PID/IPC, sensitive bind mounts incl. the engine
    socket, added capabilities, disabled seccomp/apparmor, root, device
    passthrough), network exposure (published/exposed ports, flagged when bound
    to all interfaces), the image's CVEs, and attack-enabling binaries in a
    running container.
  - All three report to stdout and can emit json, html and pdf via `--format`,
    with a `--fail-on <severity>` gate.
  - Routine awareness: `rfswift nix info` shows a Security posture line from the
    last audit, and building a Nix environment prints it too.
- Discovery and onboarding for the Nix engine:
  - `rfswift nix catalog --search TERM` (`-s`): find environments by name,
    category, description, or a tool they bundle.
  - `rfswift nix info --versions`: resolve each tool's version.
  - The first-run wizard offers a name/tool filter for large catalogs.
- Search and install tools:
  - `rfswift nix search [term]`: search the curated RF Swift tool universe (or
    all of nixpkgs with `--nixpkgs`), showing which environments include each;
    interactively, pick a result to install it.
  - `rfswift nix install <pkg> [--env NAME]`: install any nixpkgs package - even
    one that belongs to no environment - into a persistent Nix profile, shared
    across environments or scoped to one (on PATH when entering it). Shell
    completion suggests package names.
  - Container `install`: run with no function and pick one from a filterable TUI
    list of the install functions the container ships.
  - `rfswift nix install` with no package now opens a guided installer covering
    curated RF Swift tools or all pinned nixpkgs, shared or environment-specific
    profiles, and eager or lazy environments. `rfswift --engine nix install`
    provides the same wizard through the unified install command.
  - Workbench mission menus provide the same searchable installer for local or
    remote targets: container script discovery for containers and curated/all
    pinned nixpkgs search with environment-scoped persistence for Nix missions.
- `CHANGELOG.md` (this file); the flake repo keeps its own.
- Workbench GUI scaffold (`go/rfswift-workbench/`): a Wails-based research and
  assessment workspace, built as a separate binary and Go module so the lean CLI
  and agent stay free of the cgo + webview dependency. Ships the on-disk
  workspace store, per-mission branded reports, pwndoc export/import, capture
  classification, the connection security audit and the AI config; the engine is
  behind an interface awaiting wiring to the `nix`/`dock` packages. See
  `docs/workspace-gui.md` and `docs/remote-agent.md`. Builds with `wails build`
  (verified on Linux and as a cross-built Windows `.exe`); a `Makefile` and
  `.github/workflows/workbench.yml` provide the cross-platform release matrix
  (self-contained `.exe`/`.app`, AppImage on Linux). `make deps`
  (`scripts/install-deps.sh`) installs the Linux GTK/WebKit build dependencies
  across apt/dnf/pacman/zypper/apk; the Wails CLI is fetched via `go run` so no
  separate install is needed. The engine is wired to the `dock`/`nix` packages
  (live containers + Nix environments, inspect/start/stop/audit) and the frontend
  calls the generated bindings for missions, notes, findings (pwndoc-editable),
  captures, audit and report/pwndoc export, with a mock fallback for the
  standalone prototype. The console runs real commands inside a mission's
  container, and the AI assistant calls the configured provider (Claude / ChatGPT
  / Kimi / Z.ai, or a local Ollama-compatible endpoint).

### Fixed

- Nix engine: on-demand (`--lazy`) environments did not realise their
  prerequisite device/driver layer, so hardware udev rules were never available
  in lazy mode - a device (e.g. a HydraSDR added to an rfid environment) stayed
  invisible without root. Lazy now realises the prerequisites up front (only the
  application tools stay on-demand), so `rfswift env udev` and the entry-time
  rule offer see the rules as they do in eager mode.
- Nix engine: `rfswift env install` (adding a tool such as a device library into
  an environment) now offers to install any udev rules the new package ships,
  instead of only doing so at the next `run` - so a freshly installed device
  library's hardware is usable right away.
- Nix engine: SDR++ (and every other OpenGL tool) still crashed with
  `EGL: Failed to get EGL display` on environments whose flake revision does
  not carry the `rfswift-gl` package (the published flake before it landed):
  the engine tried `nix build <flake>#pkg-rfswift-gl`, printed "does not
  provide attribute" and exported nothing. It now embeds the runtime
  expression (`nix/rfswift-gl.nix`, a copy of the flake's file) and builds it
  against that flake's own nixpkgs when the package is absent, so the Mesa (or
  NVIDIA) drivers always match the libraries the tools were built with.
- Nix engine: GUI tools crashed on every host that is not NixOS (SDR++: `EGL:
  Failed to get EGL display`, `OpenGL 3.0 was not supported`, then a segfault)
  because nixpkgs' Mesa only looks for GPU drivers in `/run/opengl-driver`.
  Entering an environment (`run`, `exec`, `nix run`, Workbench terminals and
  commands) now exports the `rfswift-gl` runtime shipped by RF-Swift-nix (Mesa's drivers
  from the same pin, the nixGL approach); hosts running the proprietary NVIDIA
  driver get the matching user-space libraries built once per driver version,
  with Mesa kept as fallback for hybrid laptops. `RFSWIFT_NIX_GL=off|mesa`
  overrides the detection, `rfswift nix gl [name]` shows what applies, and
  `rfsudo` now passes the display and the GL runtime through to root. The
  Workbench terminal also gained the `xhost` step the CLI shell already had.
- Image pulls that fail with `unable to retrieve auth token: invalid
  username/password` (CLI and Workbench, Docker and Podman) now explain the
  cause: RF Swift sends no credentials, the engine presents a stored `docker
  login`/`podman login` for Docker Hub that is no longer valid (or Docker Hub's
  token service hiccuped); the message names the credential file that holds
  the login and the `logout` command to clear it.

- Workbench on Linux: containers created from the GUI had `DISPLAY` and the X
  socket but no authorization (`Authorization required, but no authorization
  protocol specified`). The GUI/API creation path now opens the local display
  with xhost exactly like the CLI does; SSH-forwarded displays keep the cookie
  mount.
- Rootless Podman from the Workbench: creation failed first on device cgroup
  rules, then on start with `container create failed (no logs from conmon)`,
  which is how Podman reports an OCI runtime failure. `CreateContainer` now
  applies the same rootless handling as the CLI (shared
  `restrictRootlessPodmanHostConfig`): cgroup rules are dropped with a
  warning, root-only device nodes (`/dev/console`, `/dev/tty*`, `/dev/vhci`,
  `/dev/uinput`, ...) and their mounts are left out, and ulimits above the
  user's host hard limits (realtime `rtprio`/`memlock`/`nice`) are skipped
  instead of aborting the start. The GUI also retries automatically after
  removing unsupported rules instead of asking for another click.
- Container tool installer (`rfswift exec -i`, Workbench right-click install):
  the exit status of the install function was never checked, so a failed build
  (for example `sdrpp_soft_fromsource_install` on a non-SDR image) was reported
  as installed. Failures now surface with the tail of the build output; apt
  housekeeping errors are downgraded to warnings.
- `rfswift winusb list` parsed the pre-4.0 `usbipd list` column layout and
  printed nonsense for current usbipd-win releases, and `winusb detach` ran the
  bind-and-attach routine instead of detaching. Both now use the usbipd JSON
  state and the correct verbs.

- Embedded CyberChef now runs on an ephemeral IPv4 loopback listener with a
  random 192-bit path capability and clean shutdown. This avoids blank Wails
  nested-document and Blob routing; selected bytes remain solely in the URL
  fragment and therefore never reach the local HTTP server. The cross-origin
  iframe relies on browser same-origin isolation instead of a worker-breaking
  sandbox, and an **Open in browser** fallback uses the same private endpoint.
- Remote Workbench terminals no longer forward xterm OSC 10/11/12 colour-query
  replies into the remote PTY, preventing occasional `11;rgb:...` fragments at
  zsh prompts while preserving normal escape sequences and pasted input.

### Changed

- The Workbench Go implementation and its tests now live in the private
  `internal/workbench` application package. The executable root contains only
  Wails/platform bootstrap files, embedded assets are injected explicitly, and
  the frontend recognizes the new `window.go.workbench.App` binding namespace
  while retaining an incremental-build compatibility fallback.
- Public-API and cross-component Workbench tests now live under
  `tests/integration`; white-box security-invariant tests remain colocated with
  `internal/workbench` so private implementation details stay private.

- The Artifacts panel no longer offers metadata-only captures. Its primary
  action imports a real evidence file, while existing legacy metadata records
  remain readable and custom capture types remain configurable.

- Container creation, reconnect, remote-agent creation and property-rebuild
  paths now consistently default to `/bin/zsh`, with a Bash fallback only when
  Zsh is unavailable.
- SSH-forwarded X11 sessions now mount the active Xauthority cookie read-only
  into newly created containers. Local `:0`/`:1` Unix-socket sessions keep the
  existing local X11 behavior.
- The canonical CLI is resource-first:
  - `rfswift container create|shell|stop|rm|rename|commit|install|upgrade`
  - `rfswift image local|remote|pull|rm|build|tag|download|export|import|audit`
  - `rfswift env catalog|list|info|remove|run|install|audit|export|import|gc`
  Root help is organized around these resources plus runtime configuration,
  networking, security, remote access, and host maintenance. Every previous flat
  command and the `images`/`nix` trees remain executable and emit a replacement
  notice; their flags and nested subcommands are covered by compatibility tests.

- The mTLS remote agent now exposes a typed, authenticated control plane used by
  Workbench for container/Nix lifecycle and creation, remote profile defaults,
  image checks/pulls, interactive PTYs, and mission-scoped artifact inventory and
  transfer. Remote selection is fail-closed with a live heartbeat and never
  silently falls back to local IPC. The GUI can persist multiple non-secret agent
  profiles while encrypted key passwords remain in the OS vault.

- CLI reorganization for readability (all previous command paths still work):
  - `rfswift --help` is grouped into Containers, Images, Native Nix environments,
    Runtime configuration, Networking, Devices, Security, Remote access, and
    System & maintenance.
  - New convenience parents: `rfswift config <bindings|capabilities|cgroups|gpus|ports|ulimits>`,
    `rfswift system <doctor|cleanup|update|upgrade|log|report>`, and a
    cross-platform `rfswift usb` that dispatches to the macOS/Windows backend.
  - `rfswift audit <target>` is now a unified entry point that auto-detects a
    Nix environment, a container image, or a container (`nix audit` and
    `images audit` remain).
- `get_rfswift` install script (and `scripts/get_rfswift.sh`) can install Nix
  for the native engine.

### Documentation

- `docs/roadmap.md`: positioning vs Kali/DragonOS and Exegol, plus the
  UX improvement backlog.
- `docs/nix-engine.md`: the `--engine nix` architecture.

## Notes

The Nix environment flake changelog lives in the RF-Swift-nix repository.
