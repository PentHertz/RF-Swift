# RF Swift bundle option "Set up Nix in WSL 2": provisions a WSL 2 Linux distro
# for the Nix engine, the same way `rfswift nix wsl setup` does once RF Swift is
# installed (go/rfswift/nix/wsl_setup.go is the reference implementation):
#
#   1. a WSL 2 distribution exists (Ubuntu is installed when none does; the
#      container engines' utility VMs such as docker-desktop never qualify);
#   2. systemd is enabled in it, so the nix daemon and udev run as services;
#   3. Nix is installed with flakes (the Determinate Systems installer, which the
#      RF Swift docs endorse), as root;
#   4. the Linux rfswift CLI is installed in /usr/local/bin, at the version of the
#      RF Swift being installed when that release exists, else the latest release.
#
# Afterwards the Windows rfswift.exe and the Workbench drive that distribution:
# `rfswift run --engine nix` works from any Windows console, environments live in
# the distro, WSLg provides display and sound, usbipd forwards the radios.
#
# Chained by Bundle.wxs as an ExePackage (powershell.exe entry payload + this
# script as a secondary payload), gated on the SetupNix checkbox and marked
# non-vital: every step is best-effort so a hiccup here never rolls back the
# container install. Exit 3010 asks Burn to reboot-and-resume when WSL was just
# installed and needs a restart. Bundle.wxs passes -RFSwiftVersion so the Linux
# CLI matches the Windows one. See docs/windows-installer.md, docs/nix-engine.md.

param(
    [string]$RFSwiftVersion = ""
)

$ErrorActionPreference = 'Stop'
$wsl = Join-Path $env:SystemRoot 'System32\wsl.exe'
if (-not (Test-Path $wsl)) {
    Write-Output "wsl.exe not found; enable WSL 2 first. Skipping Nix setup."
    exit 0
}

$requested = if ($env:RFSWIFT_WSL_DISTRO) { $env:RFSWIFT_WSL_DISTRO } else { 'Ubuntu' }
$utf8 = New-Object System.Text.UTF8Encoding($false)

# wsl.exe prints UTF-16 when redirected; read its listing with that encoding.
function Get-WslDistros {
    $prev = [Console]::OutputEncoding
    try {
        [Console]::OutputEncoding = [System.Text.Encoding]::Unicode
        $raw = & $wsl --list --quiet 2>$null
    } catch { $raw = @() } finally { [Console]::OutputEncoding = $prev }
    # Keep distribution names only: wsl.exe prints a sentence ("...has no
    # installed distributions.") instead of a list when there is none.
    $names = @()
    foreach ($line in @($raw)) {
        $n = ("$line" -replace "`0", '').Trim()
        if ($n -match '^[A-Za-z0-9._-]+$' -and $n -notmatch '^(docker-desktop|podman-machine|rancher-desktop)') { $names += $n }
    }
    return $names
}

# Runs a POSIX sh script as root inside the distro through a login shell (so
# nix lands on PATH via /etc/profile.d once installed). The script is copied
# to %TEMP% with LF endings and reached through /mnt; arguments follow it.
function Invoke-WslScript {
    param([string]$Distro, [string]$Script, [string]$LocalName, [string[]]$ScriptArgs = @())
    $tmp = Join-Path $env:TEMP $LocalName
    [System.IO.File]::WriteAllText($tmp, ($Script -replace "`r`n", "`n"), $utf8)
    $wslPath = (& $wsl -d $Distro -u root -e wslpath -a "$tmp").Trim()
    & $wsl -d $Distro -u root -e sh -l $wslPath @ScriptArgs
    return $LASTEXITCODE
}

# 1) A WSL 2 Linux distribution. Prefer one that already exists (the default is
#    listed first), else install the requested one.
$existing = Get-WslDistros
$distro = $null
if ($existing -contains $requested) { $distro = $requested }
elseif ($existing.Count -gt 0) { $distro = $existing[0] }
if (-not $distro) {
    Write-Output "Installing WSL distribution '$requested'..."
    & $wsl --install -d $requested --no-launch
    if ($LASTEXITCODE -ne 0) {
        Write-Output "wsl --install returned $LASTEXITCODE; a restart is likely needed before the distro is usable. Re-run 'rfswift nix wsl setup' afterwards."
        exit 3010
    }
    $distro = $requested
}
Write-Output "Provisioning the Nix engine in WSL 2 distribution '$distro'..."

# 2) systemd (idempotent edit of /etc/wsl.conf), then restart the distro so it
#    takes effect before Nix is installed.
$systemdScript = @'
set -e
f=/etc/wsl.conf
touch "$f"
if grep -qiE '^[[:space:]]*systemd[[:space:]]*=' "$f"; then
  sed -i -E 's/^[[:space:]]*[Ss][Yy][Ss][Tt][Ee][Mm][Dd][[:space:]]*=.*/systemd=true/' "$f"
elif grep -qiE '^[[:space:]]*\[boot\]' "$f"; then
  sed -i -E 's/^([[:space:]]*\[[Bb][Oo][Oo][Tt]\][[:space:]]*)$/\1\nsystemd=true/' "$f"
else
  printf '\n[boot]\nsystemd=true\n' >> "$f"
fi
echo "systemd enabled in /etc/wsl.conf"
'@
$code = Invoke-WslScript -Distro $distro -Script $systemdScript -LocalName 'rfswift-wsl-systemd.sh'
if ($code -ne 0) { Write-Output "Could not enable systemd (exit $code); continuing." }
& $wsl --terminate $distro 2>$null

# 3) Nix with flakes (Determinate installer; --init none when systemd is not PID 1).
$nixScript = @'
set -e
if ! command -v curl >/dev/null 2>&1; then
  if command -v apt-get >/dev/null 2>&1; then
    export DEBIAN_FRONTEND=noninteractive
    apt-get update -y && apt-get install -y curl ca-certificates xz-utils
  elif command -v dnf >/dev/null 2>&1; then dnf install -y curl xz
  elif command -v zypper >/dev/null 2>&1; then zypper --non-interactive install curl xz
  elif command -v pacman >/dev/null 2>&1; then pacman -Sy --noconfirm --needed curl xz
  else echo "curl is required to download the Nix installer" >&2; exit 1
  fi
fi
if command -v nix >/dev/null 2>&1; then echo "nix is already installed"; exit 0; fi
planner=""
if [ "$(cat /proc/1/comm 2>/dev/null)" != "systemd" ]; then planner="linux --init none"; fi
curl --proto '=https' --tlsv1.2 -sSf -L https://install.determinate.systems/nix -o /tmp/rfswift-nix-installer.sh
sh /tmp/rfswift-nix-installer.sh install $planner --no-confirm
rm -f /tmp/rfswift-nix-installer.sh
'@
$code = Invoke-WslScript -Distro $distro -Script $nixScript -LocalName 'rfswift-wsl-nix.sh'
if ($code -ne 0) { Write-Output "Nix installation did not complete (exit $code); run 'rfswift nix wsl setup' later." }

# 4) The Linux rfswift CLI: the release matching this installer, else the latest.
$rfswiftScript = @'
set -e
if command -v rfswift >/dev/null 2>&1 && [ "${1:-}" = "" ]; then echo "rfswift is already installed"; exit 0; fi
arch=$(uname -m)
case "$arch" in
  x86_64|amd64) asset=rfswift_Linux_x86_64.tar.gz ;;
  aarch64|arm64) asset=rfswift_Linux_arm64.tar.gz ;;
  *) echo "unsupported architecture: $arch" >&2; exit 1 ;;
esac
tags=""
[ -n "${1:-}" ] && tags="v${1#v}"
latest=$(curl -fsSL https://api.github.com/repos/PentHertz/RF-Swift/releases/latest 2>/dev/null | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -n 1)
[ -n "$latest" ] && tags="$tags $latest"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
for tag in $tags; do
  url="https://github.com/PentHertz/RF-Swift/releases/download/$tag/$asset"
  echo "Downloading $url"
  if curl -fsSL "$url" -o "$tmp/rfswift.tgz"; then
    tar -xzf "$tmp/rfswift.tgz" -C "$tmp" rfswift
    install -m 0755 "$tmp/rfswift" /usr/local/bin/rfswift
    echo "installed rfswift $tag to /usr/local/bin/rfswift"
    exit 0
  fi
  echo "  no release asset for $tag"
done
echo "no RF Swift release asset found; inside the distro run: curl -fsSL https://raw.githubusercontent.com/PentHertz/RF-Swift/refs/heads/main/scripts/get_rfswift.sh | RFSWIFT_INSTALL=cli sh" >&2
exit 1
'@
$code = Invoke-WslScript -Distro $distro -Script $rfswiftScript -LocalName 'rfswift-wsl-cli.sh' -ScriptArgs @($RFSwiftVersion)
if ($code -ne 0) { Write-Output "The Linux rfswift CLI was not installed (exit $code); run 'rfswift nix wsl setup' later." }

Write-Output "Nix in WSL 2 setup finished (distro: $distro)."
Write-Output "From any Windows console:  rfswift nix wsl status   then   rfswift run --engine nix -i sdr_light -n lab"
exit 0
