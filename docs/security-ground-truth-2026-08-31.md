# RF Swift security ground truth

Date: 2026-08-31  
Scope: Go CLI and remote agent, Workbench backend and import/export paths,
shell installers, and GitHub Actions workflows in this repository.

This document supersedes the repository's earlier audit summaries as the
current security baseline. Those reports remain useful historical evidence,
but a security review is a point-in-time assessment, not a promise that no
attack is possible.

## Executive result

The reviewed boundaries now have explicit defenses against the concrete
high-impact issues found in this cycle: clear-text remote control, filesystem
traversal through mission identifiers and imported archives, unbounded remote
command output, unsafe installer username evaluation, mutable GitHub Action
references, and overly broad release-job permissions. Focused regression tests
for the affected Go packages, installer tests, ShellCheck, workflow YAML
validation, and the immutable-action guard passed.

No unresolved critical vulnerability was confirmed in the reviewed code.
Several powerful features remain high-trust by design and must be deployed as
such: container-engine sockets, privileged/device-enabled containers, imported
Nix closures, the authenticated remote command agent, and third-party bootstrap
installers.

## Threat model and trust boundaries

| Boundary | Security assumption |
|---|---|
| Workbench WebView to Go backend | Web input is untrusted. Every exposed backend filesystem operation must validate workspace, mission, filename, and archive paths. |
| Imported projects, `.rfenv` files, captures and notes | Treat as hostile data. Evidence can contain prompt injection. A portable Nix closure contains executable code and must come from a trusted source. |
| Docker/Podman/Lima socket | Engine access is effectively host-administration access. Do not expose the socket to untrusted users or workloads. |
| Remote agent | A client certificate authorizes remote command execution. Run on loopback by default, protect keys, and expose it only on a trusted network or authenticated tunnel. |
| Installer and release pipeline | Repository, release signing identity, pinned workflow actions, downloaded checksums, and external platform installers are supply-chain trust roots. |
| Hardware-enabled mission | Devices, capabilities, host networking, X11, mounts, and privileged mode deliberately reduce isolation. Enable only what the assessment requires. |

## Confirmed fixes in the current hardening pass

| Severity | Finding | Resolution |
|---|---|---|
| High | Workbench store methods accepted unvalidated workspace/mission components in note, finding, capture, report, and secret-metadata paths. | Central validation is applied before those operations; traversal regression tests cover notes and secret metadata. |
| High | `.rfenv` import used general-purpose tar extraction on a user-selected archive. | Native Go extraction rejects traversal, device entries, escaping links, duplicate writes, excessive entry counts, oversized manifests, invalid environment names, and malformed Nix store paths. Links are created only after regular files. |
| Medium | Remote command execution buffered unlimited combined output in agent memory. | Output is consumed through a bounded 16 MiB writer and visibly marked when truncated. |
| Medium | Mission companion ZIP extraction could expand without cumulative byte or entry bounds. | Import now rejects companion data above 20 GiB or 100,000 entries instead of silently truncating files. |
| Medium | The installer resolved a username's home directory through shell `eval`. | Usernames are constrained and home lookup uses `getent`, macOS `dscl`, or a safe current-user fallback. |
| Medium | Several GitHub Actions used mutable major-version tags. | Third-party actions in reviewed workflows are pinned to full commit SHAs; CI checks reject mutable `vN`, `main`, and `master` references. |
| Medium | Release workflow permissions granted `contents: write` to build jobs. | Default permission is `contents: read`; only publishing/signing jobs receive their required write scopes. |
| Low | Remote endpoints could carry credentials, paths, queries, or fragments even though callers require an origin. | Endpoint normalization accepts only a bare HTTPS origin (a trailing slash is allowed). |
| Low | The remote HTTP server lacked a complete set of resource timeouts. | Read and header timeouts plus a header-size bound complement the existing idle timeout and TLS 1.3 requirement. |

The previously confirmed fixes in
[the August audit](security-audit-2026-08.md) and
[the Workbench audit](workbench-security-audit.md) remain part of the baseline,
including HTTPS-only agent endpoints, environment-name validation, safer X11
authorization, installer verification, WebView output escaping, safe link
schemes, managed terminal-recording paths, and prompt-injection trust labels.

## Verification evidence

The following focused checks passed on 2026-08-31:

```text
go test ./nix ./remote ./dock ./cli
go test ./internal/workbench
bash -n get_rfswift.sh scripts/*.sh
./scripts/test-installer.sh
shellcheck --severity=error get_rfswift.sh scripts/*.sh
yamllint (workflow syntax; cosmetic style rules disabled)
immutable GitHub Action reference guard
```

These checks validate the touched controls. They are not a substitute for the
complete CI matrix, platform-specific builds, dependency advisory feeds,
fuzzing, or a recurring independent review.

## Residual risks and required policy

1. **Remote agent authorization is intentionally coarse.** Possession of a
   valid client identity grants command execution; there are no roles,
   per-command policies, revocation service, or request rate limits. Keep it
   loopback/private, use short-lived credentials where practical, and rotate
   immediately after loss or personnel changes.
2. **Portable Nix environments are executable imports.** Path-safe extraction
   prevents filesystem breakout but does not prove provenance. Import only
   trusted `.rfenv` exports. A signed archive manifest is a recommended future
   enhancement.
3. **Engine access and privileged missions can control the host.** Prefer
   rootless Podman and least privilege. Avoid `--privileged`, host networking,
   broad `/dev` mounts, `SYS_ADMIN`, and writable host binds unless required.
4. **Third-party bootstrap scripts remain external trust roots.** Docker's
   convenience installer and the Determinate Nix installer execute vendor
   scripts. Security-sensitive or air-gapped deployments should use audited,
   version-pinned packages instead.
5. **Release signing can be secret-dependent.** Stable releases should fail
   closed when required macOS/Windows signing credentials are absent. Tag and
   environment protection must prevent a release from an unreviewed commit.
6. **Security scanners are not all blocking.** Informational `gosec` findings
   require triage; dependency databases and CI services can also be stale or
   unavailable.
7. **Local same-user races remain possible around ordinary metadata files.**
   Atomic writes and no-follow opens should be extended to all sensitive state
   files where practical.

## Prioritized next work

- P0: enforce protected tags/releases and make mandatory signing fail closed
  for stable release channels.
- P0: publish and verify signed manifests for portable environments and mission
  exports before importing executable closures.
- P1: add remote-agent authorization profiles, credential revocation/expiry,
  audit logging, concurrency limits, and request rate limiting.
- P1: run archive parsers, protocol framing, environment-name handling, and
  WebView sanitizers continuously under fuzzing.
- P1: audit and apply consistent entry-count and metadata-size limits to the
  remaining project/container archive formats.
- P2: migrate security-sensitive state writes to atomic, owner-only, no-follow
  operations and document key rotation and incident response.

## Hardened deployment baseline

- Install only artifacts whose checksum, signature/attestation, repository,
  and version were independently verified.
- Use a dedicated non-administrator account; keep engine and hardware groups
  narrowly assigned.
- Prefer rootless Podman, read-only binds, explicit devices, and minimum
  capabilities.
- Keep the remote agent on loopback or a private authenticated tunnel and
  protect its key directory with owner-only permissions.
- Treat all imported missions, notes, captures, images, and Nix environments as
  untrusted until provenance is established.
- Apply branch, tag, and release-environment protection in GitHub; require the
  CI workflow before release approval.
- Re-run the full CI/security matrix and dependency advisory checks for every
  release candidate and after changes to installers, workflows, archives,
  rendering, engine access, or remote control.
