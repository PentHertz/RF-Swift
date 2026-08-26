# Changelog

All notable changes to RF Swift are recorded here. The format is based on
Keep a Changelog (https://keepachangelog.com), and the project aims to follow
semantic versioning. Dates are ISO-8601 (YYYY-MM-DD).

## [Unreleased] - v4.0.0-dev

Development toward the 4.0.0 "Nucleus" release. The Nix engine and its
environment flake (RF-Swift-nix) track a parallel v1.0.0-dev from its `main`
branch.

### Added

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
  metadata, embedded role text, and tool requests as evidence—not instructions.

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
