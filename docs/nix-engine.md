# The Nix engine (RF Swift v4.0.0)

RF Swift v4.0.0 adds a fourth engine alongside Docker, Podman and Lima: **Nix**.

Where the container engines run your tools inside an image, the Nix engine installs
them straight onto the host as a reproducible, pinned environment. There is no
daemon and no container boundary, so USB radios and audio work without any device
or socket plumbing. The tool sets are the same ones the Docker images ship
(`sdr_light`, `rfid`, `wifi`, ...), defined in the companion repository
[RF-Swift-nix](https://github.com/PentHertz/RF-Swift-nix).

## Requirements

A working [Nix](https://nixos.org/download) install with flakes. The multi-user
install is recommended:

```bash
sh <(curl -L https://nixos.org/nix/install) --daemon
```

RF Swift enables the `nix-command` and `flakes` features on every call, so you do
not need to edit `nix.conf`.

## Quick start

```bash
# Interactive wizard: pick an environment, name it, choose a workspace
rfswift run --engine nix

# Or the full command
rfswift run --engine nix -i sdr_light -n mysdr

# Re-enter it later
rfswift exec --engine nix -c mysdr

# Run a one-off command in it
rfswift exec --engine nix -c mysdr -e "gqrx"
```

`RFSWIFT_ENGINE=nix` works too, so you can set it once for a shell session.

## Managing environments

```bash
rfswift nix catalog                 # environments you can create
rfswift nix list                    # environments you have created
rfswift nix info mysdr              # details + package list for one environment
rfswift nix remove mysdr           # delete it (frees the pin; nix store gc reclaims space)
rfswift nix run sdr_light gqrx     # build and run a single tool on demand
```

The newer resource-first spelling is `rfswift env ...`; `rfswift nix ...`
remains compatible and prints a deprecation notice.

## Updating, rebuilding and rolling back

Check whether the pinned flake inputs have changed without modifying anything:

```bash
rfswift env update --check mysdr
```

Run the command without a name to open the guided update wizard:

```bash
rfswift env update
```

The wizard provides a searchable environment picker, lets you check only,
update every input, or select one input from the environment's real
`flake.lock`, displays a recap including existing rollback points, and asks for
confirmation before changing anything.

Update every input in the local `flake.lock`, then rebuild:

```bash
rfswift env update mysdr
# Non-interactive/CI:
rfswift env update --yes mysdr
```

Update only nixpkgs:

```bash
rfswift env update --input nixpkgs mysdr
```

`--input` requires a writable local flake checkout. A GitHub flake reference
has no local lock file to edit, so RF Swift can refresh and rebuild it but cannot
selectively rewrite one of its inputs.

To rebuild using the lock that is already pinned—without looking for newer
nixpkgs or GitHub revisions—run:

```bash
rfswift env rebuild mysdr
```

Updates and rebuilds are transactional for eager environments: prerequisites
and a candidate closure are built before the active profile changes. If the
build fails, the current environment remains active; after a failed local flake
update, the previous `flake.lock` is restored. A successful switch preserves
the former closure as a Nix GC root:

```bash
rfswift env generations mysdr
rfswift env rollback mysdr                    # newest previous generation
rfswift env rollback mysdr <listed-generation> # selected listed generation
```

Rollback generations are stored under
`~/.rfswift/nix/environments/<name>/generations/`, so `rfswift env gc` cannot
remove them. Updating invalidates the environment security audit while retaining
the old report as a stale report beside the environment metadata. Run
`rfswift env audit mysdr` again after updating.

These generation guarantees apply to eager environments. Lazy and pure
environments do not have a complete pinned profile to preserve; recreate them
as eager environments when transactional update and rollback are required.

The requested legacy-compatible forms are all available too:

```bash
rfswift nix update --check <name>
rfswift nix update --input nixpkgs <name>
rfswift nix rebuild <name>
rfswift nix generations <name>
rfswift nix rollback <name>
```

They behave identically to `rfswift env ...`; only a migration notice is
printed because `env` is the canonical resource-first command group.

## Build modes: all at once, or step by step

Creating an environment can work two ways:

- **Eager (default):** `rfswift run --engine nix -i sdr_light -n mysdr` builds the
  whole tool set once and pins it. The first run downloads the cached tools and
  compiles the few that are not cached; after that every entry is instant and
  works offline.
- **On-demand:** `rfswift run --engine nix -i sdr_light -n mysdr --lazy` does not
  prebuild anything. Each tool becomes a shim that builds it the first time you
  call it. Type `gqrx` and gqrx is built and run; type `inspectrum` next and only
  that is built. Nothing you never use is ever built. The interactive wizard also
  offers this as a "build mode" choice.

You can also run a single tool without creating an environment at all:

```bash
rfswift nix run sdr_light gqrx            # build+run gqrx from the pinned set
rfswift nix run mysdr inspectrum -- x.iq  # scoped to an environment's pinned flake
```

Both modes build the same tool from the same pinned definition, so results are
identical; the only difference is when the work happens.

## Guided installation of additional tools

Run the installer without a package name to open the wizard:

```bash
rfswift nix install
# Equivalent unified entry point:
rfswift --engine nix install
```

The wizard asks whether the package should be shared by every RF Swift Nix
environment or installed only for one environment, including lazy environments.
It can search either the curated RF Swift tool set or the complete pinned
`nixpkgs` package set. The selected package is added to a persistent profile and
appears on `PATH` when the relevant environment is entered.

For automation, the existing non-interactive form remains unchanged:

```bash
rfswift nix install ffmpeg
rfswift nix install gnuradioPackages.gr-foo --env mysdr
```

For containers, `rfswift install` with no arguments inspects `/root/scripts` in
the selected container and opens a searchable picker of available installer
functions. The function name does not need to be looked up in documentation.

### Downloading instead of compiling (binary cache)

"Build" mostly means "download a prebuilt binary from the Nix cache", not
"compile from source". Standard nixpkgs tools (GNU Radio, GQRX, Wireshark, ...)
are fetched prebuilt. Only tools not in a cache compile locally, which for RF
Swift is the handful of custom derivations in RF-Swift-nix/pkgs. If the project
publishes those to its own binary cache (the RF-Swift-nix CI does this), even
those download prebuilt and no local compilation is needed.

## How it works

- An **environment** is the Nix analogue of a container: created once, re-entered,
  removed. Each lives under `~/.rfswift/nix/environments/<name>/`.
- `run` resolves the image to a flake output, builds its tool closure with
  `nix build`, and pins it with a gcroot symlink (`.../<name>/profile`). The first
  build fetches and compiles; later entries are instant and work offline.
- Entering the environment starts your shell with the tools prepended to `PATH`
  and a workspace as the working directory (`~/rfswift-workspace/<name>/` by
  default; change with `--workspace`, `--cwd`, or `--no-workspace`).

## Flags specific to the Nix engine

| Flag | Meaning |
|------|---------|
| `--lazy` | On-demand: build each tool the first time it is called, not all up front |
| `--pure` | Enter a pure environment (`nix develop --ignore-environment`), not inheriting the host environment |
| `--rebuild` | Force re-realisation during creation (eager mode); for an existing environment use `rfswift env rebuild <name>` |
| `--flake <ref>` | Use a specific flake reference instead of the default |

## Choosing where environments come from

The flake reference is resolved in this order:

1. `--flake <ref>`
2. `RFSWIFT_NIX_FLAKE` (a flake URL or a local path)
3. a local `RF-Swift-nix` checkout next to the working directory or the binary
4. the published default, `github:PentHertz/RF-Swift-nix`

So if you clone RF-Swift-nix next to RF-Swift and hack on `environments.nix`, RF
Swift uses your local copy automatically.

## Notes

- Not every tool in the Docker images is in nixpkgs yet. RF-Swift-nix carries its
  own derivations for the source-built tools and the PentHertz/HydraSDR forks;
  proprietary vendor SDKs are opt-in and need a manual download. Anything not yet
  packaged is listed per environment and dropped from the shell with a trace
  rather than failing the build.
- The Nix engine is Linux and macOS only (Nix does not run natively on Windows;
  use WSL2).
