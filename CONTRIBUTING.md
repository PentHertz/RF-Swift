# Contributing to RF Swift

RF Swift is a cross-platform CLI tool for managing containerised RF/SDR and hardware-hacking toolboxes, written in Go. This guide explains the project structure and how to modify the code.

## Prerequisites

- Go 1.22+
- Docker or Podman running on the host
- `make` (for cross-compilation targets)

## Building

From the Go module root:

```bash
cd go/rfswift

# Build for current platform
go build -o rfswift .

# Cross-compile (Linux, Windows, macOS)
make all        # all targets
make linux      # linux amd64/arm64/riscv64
make windows    # windows amd64/arm64

# Output goes to bin/rfswift_<os>_<arch>
```

Builds are fully static (`CGO_ENABLED=0`, `-tags netgo`).

## Verify your changes

```bash
go build ./...   # must compile cleanly
go vet ./...     # must pass with no issues
go test ./...    # run tests
```

## Project layout

```
RF-Swift/
├── go/rfswift/           # Go source (the binary)
│   ├── main.go           # Entry point → cli.Execute()
│   ├── cli/              # Cobra command tree (thin UI layer)
│   ├── common/           # Shared constants, version, colored output
│   ├── dock/             # Container engine logic (the core)
│   └── rfutils/          # Host utilities (X11, audio, self-update, USB)
├── recipes/              # YAML build recipes for container images
├── build_project.sh      # Interactive bootstrap script
├── common.sh             # Shared bash library for shell scripts
├── .goreleaser.yml       # Release automation config
└── .github/workflows/    # CI/CD (build, test, release)
```

## Architecture

```
main.go
  └─▶ cli.Execute()
        │
  ┌─────▼───────────────────────────────────────────┐
  │  package cli   (Cobra commands, flag parsing)   │
  │  No business logic — just maps flags to calls   │
  └─────┬──────────────────────────┬────────────────┘
        │                          │
  ┌─────▼───────────────┐    ┌─────▼───────────┐
  │  package dock       │    │ package rfutils │
  │  Container ops      │    │ Host utilities  │
  │  Engine abstraction │    | X11, audio, USB │
  │  Docker/Podman      │    │ GitHub updates  │
  └─────┬───────────────┘    └─────────────────┘
        │
  Docker / Podman engine
```

**Key rule:** `cli` never contains business logic. It parses flags and delegates to `dock` or `rfutils`.

## Package guide

### `cli/` — Command definitions

Each file defines a group of related commands and a `registerXxxCommands()` function:

| File | Commands | Domain |
|------|----------|--------|
| `root.go` | root, host, audio, update, engine | App-level + wiring |
| `container.go` | run, exec, last, stop, remove, install, commit, rename | Container lifecycle |
| `images.go` | images local/remote/versions, pull, retag, delete, download | Image management |
| `properties.go` | bindings, capabilities, cgroups, ports (add/rm) | Container config |
| `upgrade_build.go` | upgrade, build | Upgrade + recipe builds |
| `transfer.go` | export/import container/image | Tar-based transfer |
| `cleanup.go` | cleanup all/containers/images | Pruning |
| `logging.go` | log start/stop/replay/list | Session recording |
| `ulimits.go` | ulimits add/rm/list, realtime enable/disable/status | Resource limits |
| `completion.go` | completion bash/zsh/fish/powershell | Shell completion |
| `winusb.go` | winusb list/attach/detach | Windows USB (conditional) |

A single `init()` in `root.go` calls all `registerXxxCommands()` functions. This avoids fragile multi-file `init()` ordering.

### `dock/` — Container engine core

The largest package. Key files:

| File | Purpose |
|------|---------|
| `engine.go` | `ContainerEngine` interface, auto-detection, singleton |
| `engine_docker.go` | Docker implementation (socket discovery, service mgmt) |
| `engine_podman.go` | Podman implementation (rootless/rootful, socket activation) |
| `podman.go` | Podman CLI fallback for features not in compat API |
| `types.go` | All data structures (`ContainerConfig`, `HostConfigFull`, `BuildRecipe`) |
| `setters.go` | Fluent setters for the global `containerCfg` singleton |
| `container.go` | Create, run, exec, attach, recording |
| `images.go` | Local image listing, pull, tag, delete |
| `dockerhub.go` | Remote registry queries (Docker Hub API) |
| `properties.go` | Container inspection and property display |
| `helpers.go` | Low-level Docker API wrappers, JSON config R/W |
| `recipe.go` | YAML recipe → Dockerfile → build |
| `upgrade.go` | Container migration to new image |
| `transfer.go` | Host ↔ container file transfer |
| `cleanup.go` | Container/image pruning |
| `logging.go` | Session recording (asciinema/script) |
| `ulimits.go` | Ulimit string parsing |
| `display.go` | Terminal title, text formatting |
| `terminal_linux.go` | Platform-specific terminal size (Linux) |
| `terminal_darwin.go` | Platform-specific terminal size (macOS) |
| `terminal_windows.go` | Platform-specific terminal size (Windows) |

### `common/` — Shared utilities

Single file `common.go`:
- Version metadata (`Version`, `Codename`, `Branch`)
- `Disconnected` global flag (skip network calls)
- `PrintASCII()` — ASCII banner
- `PrintErrorMessage()`, `PrintSuccessMessage()`, `PrintWarningMessage()`, `PrintInfoMessage()` — colored output
- `ConfigFileByPlatform()` — platform-correct config path

### `rfutils/` — Host utilities

| File | Purpose |
|------|---------|
| `configs.go` | INI config file management, interactive first-run setup |
| `rfutils.go` | X11 forwarding (`XHostEnable`), display env, version display |
| `githutils.go` | GitHub release API, self-update, download with progress bar |
| `hostcli.go` | PulseAudio/PipeWire management, Windows USB (usbipd) |
| `notifications.go` | Unicode box-drawing notification panels |

## How to add a new command

1. **Choose the right file** in `cli/` based on the domain, or create a new one.

2. **Define the command variable:**
   ```go
   var myNewCmd = &cobra.Command{
       Use:   "mycommand",
       Short: "Short description",
       Long:  `Detailed description`,
       Run: func(cmd *cobra.Command, args []string) {
           // Parse flags, delegate to dock or rfutils
           name, _ := cmd.Flags().GetString("name")
           rfdock.MyNewFunction(name)
       },
   }
   ```

3. **Wire it in the register function:**
   ```go
   func registerMyCommands() {
       rootCmd.AddCommand(myNewCmd)
       myNewCmd.Flags().StringP("name", "n", "", "the name")
       myNewCmd.MarkFlagRequired("name")
   }
   ```

4. **Call the register function** from `init()` in `root.go`:
   ```go
   func init() {
       // ... existing registrations ...
       registerMyCommands()
   }
   ```

5. **Implement the logic** in `dock/` or `rfutils/` — not in `cli/`.

## How to add a new container engine feature

The `ContainerEngine` interface in `dock/engine.go` defines what each engine must support. To add a new capability:

1. Add the method to the `ContainerEngine` interface.
2. Implement it in both `engine_docker.go` and `engine_podman.go`.
3. If Podman needs CLI fallback (common for features not in the compat API), add it to `podman.go`.

## Container config flow

When a user runs `rfswift run`, the flow is:

1. `cli/container.go` parses all flags
2. Calls `dock.ContainerSetXxx()` setters to populate the global `containerCfg`
3. Calls `dock.ContainerRun(name)` which reads `containerCfg` to build a Docker `HostConfig` and `ContainerConfig`
4. Creates and starts the container via the Docker SDK

The `containerCfg` singleton in `dock/types.go` holds defaults loaded from the user's config file at package init time.

## YAML build recipes

Recipes in `recipes/` define container images declaratively:

```yaml
name: my_image
base_image: penthertz/rfswift_noble:latest
tag: my_custom_tag
labels:
  org.container.project: rfswift
steps:
  - type: run
    commands:
      - apt-get update
      - apt-get install -y some-package
  - type: workdir
    path: /opt/tools
  - type: copy
    items:
      - source: ./local_file
        destination: /opt/tools/
  - type: cleanup
    paths:
      - /tmp/*
    apt_clean: true
```

`dock/recipe.go` converts these into a Dockerfile at runtime and submits it to the Docker Build API.

## Release process

Releases are automated via GoReleaser (`.goreleaser.yml`):

1. Tag a commit: `git tag v1.x.x`
2. Push the tag: `git push origin v1.x.x`
3. GitHub Actions runs GoReleaser, which cross-compiles and publishes binaries as release assets.

## Code style

- Use `common.PrintErrorMessage()` / `common.PrintSuccessMessage()` etc. for user-facing output
- Use `os.Exit(1)` after fatal errors in CLI command handlers
- Keep `cli/` files thin — extract logic into `dock/` or `rfutils/`
- Follow existing naming: exported commands use `XxxCmd`, register functions use `registerXxxCommands()`
- No build tags needed for platform-conditional commands — use `runtime.GOOS` checks at registration time (see `winusb.go`)
