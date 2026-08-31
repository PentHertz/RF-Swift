# Security audit - August 2026 (v4.0.0-dev packaging cycle)

> Historical audit evidence. See the
> [current security ground truth](security-ground-truth-2026-08-31.md) for the
> consolidated baseline, later fixes, and residual risks.

This is the trace of the security audit run over the v4.0.0-dev packaging and
installer work: what was reviewed, what was found, what was fixed, and how each
fix was verified. It covers the native Linux packaging, the macOS setup flow,
the Homebrew delivery, the cross-platform installer, a focused review of the
new Go code, and the supply-chain CI coverage.

## Method

Each area was reviewed against ground truth rather than by inspection alone:
the installer flows were exercised with the real `goreleaser`, `nfpm`,
`osv-scanner`, and `govulncheck` binaries; findings were confirmed by reading
the exact code paths; and the higher-risk items were re-checked adversarially
before any fix. Every fix ships with a regression test or a build/package
verification, and the whole tree builds and tests green.

## Findings and fixes

Severity reflects concrete, in-scope exploitability under the project threat
model (end users run the installer with sudo; packages install as root; the
release pipeline runs on maintainer-pushed tags; network responses are
attacker-influenced only via MITM or a compromised account).

| # | Area | Severity | Issue | Status |
|---|------|----------|-------|--------|
| 1 | remote/protocol.go | High | `http://` endpoint silently bypassed TLS, pinning, and mTLS while the probe still reported "TLS 1.3, pinned" | Fixed |
| 2 | nix/environment.go | High | Environment name interpolated unquoted into a sourced bash rcfile: command injection (RCE) on interactive entry | Fixed |
| 3 | scripts/setup-macos.sh | Medium | Under `curl \| bash`, companion files were resolved from the caller's CWD and sudo-installed / executed without verification | Fixed |
| 4 | get_rfswift.sh | Medium | Native package path fed unverified downloads to `sudo <pm> install` when no SHA-256 tool was present (fail-open) | Fixed |
| 5 | nix/environment.go | Medium | `xhost +local:` granted the X server to every local user on native Nix entry | Fixed |
| 6 | get_rfswift.sh | Low | Attestation failure was a warning on the native path, weaker than the tarball path's fatal gate | Fixed |
| 7 | nfpm configs | Low | Package file modes inherited the build tree's umask (group-writable files, or a non-executable binary) | Fixed |
| 8 | get_rfswift.sh | Low | Network-derived version string was unconstrained before use in URLs and filesystem paths | Fixed |
| 9 | nix/environment.go | Low | Manifest-derived shim `attr` written into a comment line: newline could inject a shell line | Fixed |
| 10 | dock/transfer.go | Low | Tar extraction did no `header.Name` traversal check (not reachable today; the source is the container engine's own archive) | Fixed (defense in depth) |

### 1. Cleartext transport downgrade (remote agent)

The endpoint normalizer only rewrote `rfswifts://` to `https://` and prepended
`https://` when no scheme was present, so an explicit `http://host` passed
through and `http.Transport` sent the whole control channel in plaintext, while
`ProbeAgent`'s raw TLS dial still reported a green "TLS 1.3, pinned, mTLS"
posture. Fix: a single `normalizeEndpoint` helper rejects any non-`https`
scheme, and all five endpoint-normalization sites route through it and build
request URLs from the validated `*url.URL`. Regression tests reject
`http://`, `ws://`, and `ftp://` and confirm `rfswifts://` and bare hosts still
normalize to https.

### 2. Command injection via environment name (Nix engine)

`RunEnvironment` only checked the name was non-empty, and the Workbench's
`validWorkspaceName` permits shell metacharacters. The name is interpolated
unquoted into the generated bashrc that is sourced on interactive entry, and
`GetEnvironment` reads it from an on-disk manifest that import/export copies
between machines. A name like `` pwn`cmd` `` from the remote-agent create
handler, the Workbench, or an imported environment executes as the operator.
Fix: `ValidateEnvironmentName` (strict `^[A-Za-z0-9][A-Za-z0-9._-]*$`, which
also blocks path traversal through the name) is enforced both at creation
(`RunEnvironment`) and at manifest load (`GetEnvironment`). Tests cover the
validator, the malicious-manifest rejection, and that legitimate names pass.

### 3. Planted-binary execution under curl | bash (macOS setup)

`setup-macos.sh` is shipped in the notarized DMG and also documented as a
`curl -fsSL ... | bash` command. Under the pipe, `dirname "$0"` is the caller's
CWD, and the script would sudo-install a `rfswift` binary or execute a
`setup-xquartz-macos.sh` found there without any verification. On a shared host
another local user could seed those files. Fix: companion files are trusted
only when the script runs from a real file on disk (`$0` contains a slash and
exists); in a piped run `SCRIPT_DIR` is empty and both branches are skipped.

### 4-9. Installer and Nix hardening

- Native package installs now refuse to run without a SHA-256 tool (fail
  closed) instead of installing unverified packages as root.
- The native path fetches and verifies every package, then runs the same
  opt-in-but-fatal attestation gate as the tarball path, then installs, so
  nothing is installed before all downloads pass.
- Package file modes are pinned via nfpm `file_info` (0755 binaries, 0644
  data), verified with `dpkg -c` regardless of the build tree's umask.
- The network-derived version string is constrained to `[0-9A-Za-z.-]` before
  it reaches any URL or path, with tests for `../`, `$( )`, `;`, and whitespace
  payloads.
- `xhost +local:` was replaced with server-interpreted grants for exactly the
  operator's own user and root (`+si:localuser:<user>`, `+si:localuser:root`),
  matching the scoped grant the container path already uses. This is
  Linux-only; the macOS XQuartz path is unchanged.
- Manifest-derived shim attributes containing a newline are skipped so they
  cannot inject a shell line into the generated shim comment.

### 10. Tar extraction traversal guard (defense in depth)

`extractTarArchive` gained a check that rejects any entry whose cleaned path
escapes the destination. It is not reachable today (the only caller feeds it
the container engine's own archive of a real filesystem, which cannot emit
`..`, and link entries are ignored), but the check was absent.

## Supply-chain CI coverage

The dependency and supply-chain checks in `modules-audit.yml` and
`security.yml` were already strong: third-party GitHub Actions pinned by full
commit SHA, scanners installed via `go install <module>@<version>` (verified
against the Go checksum database), `go mod verify` and tidy-drift on the CLI
module, a banned-module guard, reachability-aware `govulncheck`, `osv-scanner`
(CVEs plus OpenSSF MAL-* malicious packages), and PR-time dependency review.

The gap was scope: those checks covered only the CLI module (`go/rfswift`).
Two dependency surfaces were unscanned:

- the Workbench GUI Go module (`go/rfswift-workbench`, the large wails tree),
- the Workbench frontend npm lockfile (`@xterm/*` and its transitive deps).

Both are now covered:

- `osv-scan` additionally scans the frontend `package-lock.json` (npm CVEs and
  MAL-* packages).
- A new `workbench-modules` job runs tidy-drift, the banned-module guard,
  reachability-aware `govulncheck`, and `osv-scanner` on the Workbench Go
  module. It installs gtk3 + webkit2gtk-4.1 first because that module is cgo
  and osv-scanner's call analysis compiles it. `go mod verify` is intentionally
  omitted here: the module carries a first-party `replace penthertz/rfswift =>
  ../rfswift` with no cache hash, which aborts `go mod verify`; third-party
  integrity is covered by go.sum's sumdb hashes and the osv scan.

Each manifest is scanned in a separate osv-scanner invocation on purpose:
osv-scanner v2.3.8 panics in its SSA call-graph builder when made to analyze
multiple Go modules in one run, while one module per invocation is stable.

### Known accepted advisory

`GO-2026-5932` (golang.org/x/crypto 0.55.0) is present in the Workbench module
but is not reachable: `govulncheck` reports zero reachable vulnerabilities and
osv-scanner's reachability analysis filters it out, so it does not fail the
gate. It is tracked for a routine version bump (Dependabot manages these) and
requires no code change. If a future refactor makes it reachable, the
`workbench-modules` job turns red.

## Verifying locally

```bash
# CLI + Nix engine + remote agent
cd go/rfswift && go build ./... && go test ./nix/ ./dock/ ./remote/ ./cli/

# Workbench GUI (needs gtk3 + webkit2gtk-4.1)
cd go/rfswift-workbench && go build ./... && go test ./internal/workbench/

# Installer
sh -n get_rfswift.sh
scripts/test-installer.sh
shellcheck --severity=error get_rfswift.sh scripts/*.sh

# Packaging (needs goreleaser, nfpm, ImageMagick)
goreleaser release --snapshot --clean --skip=publish,announce

# Dependency and supply-chain scans (per module; see modules-audit.yml)
cd go/rfswift            && osv-scanner scan source -L go.mod && govulncheck ./...
cd go/rfswift-workbench  && osv-scanner scan source -L go.mod && govulncheck ./...
osv-scanner scan source -L go/rfswift-workbench/frontend/package-lock.json
```

The same checks run in GitHub Actions through the CI, security, and
modules-audit workflows, and gate the release workflow.
