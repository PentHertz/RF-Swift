# Installer security and trust model

`get_rfswift.sh` can install the RF Swift CLI, RF Swift Workbench, or both. It
supports the stable release channel and the `v4.0.0-dev` prerelease channel.

## Controls applied before installation

1. Downloads use HTTPS, fail on HTTP errors, and retry transient failures.
2. Every CLI and Workbench asset must match its release SHA-256 manifest.
   A missing manifest entry or mismatch stops installation.
3. When the GitHub CLI is available, the installer offers GitHub artifact
   attestation verification as an additional provenance check.
4. Tar and ZIP member names are checked before extraction. Absolute paths,
   parent traversal, and archive links are rejected. ZIP symlinks are rejected
   when `zipinfo` is available.
5. Downloads are staged in a private temporary directory and cleaned on exit.
6. The test suite sources the installer without executing `main`, exercises a
   safe archive and a traversal archive, and verifies development-channel URL
   selection.

Run the local checks with:

```bash
sh -n get_rfswift.sh
scripts/test-installer.sh
shellcheck --severity=error get_rfswift.sh scripts/test-installer.sh
```

The same checks run in GitHub Actions through the security workflow.

## Non-interactive installation

Automation should pin the desired channel and components explicitly:

```bash
RFSWIFT_CHANNEL=dev \
RFSWIFT_INSTALL=both \
RFSWIFT_WORKBENCH_FORMAT=appimage \
sh get_rfswift.sh
```

Accepted values are `stable|dev`, `cli|workbench|both`, and
`native|appimage`. AppImage is currently offered for Linux x86-64; native
Workbench archives are offered for Linux x86-64 and macOS universal builds.

## Residual trust

SHA-256 manifests hosted beside a release protect against corruption but do
not independently protect against compromise of both the release asset and its
manifest. Artifact attestation adds an independently verifiable link to the
GitHub Actions build identity and should be enabled for sensitive deployments.
Users should download the installer, review it, and execute the local file
instead of piping network content directly into a shell.

The optional Determinate Nix installation path is maintained upstream and uses
its upstream bootstrap command. Review or preinstall Nix separately when that
trust boundary is unsuitable for an organization.
