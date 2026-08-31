# RF Swift bundle option "Set up Nix in WSL 2": provisions a WSL 2 Linux distro
# with Nix (flakes, via the Determinate installer the RF Swift docs endorse) and
# the Linux rfswift CLI, so `rfswift run --engine nix` works inside WSL for
# native, container-free environments.
#
# Chained by Bundle.wxs as an ExePackage (powershell.exe entry payload + this
# script as a secondary payload), gated on the SetupNix checkbox and marked
# non-vital: every step is best-effort so a hiccup here never rolls back the
# container install. Exit 3010 asks Burn to reboot-and-resume when WSL was just
# installed and needs a restart.
#
# The Nix engine is Linux-only; on Windows it runs the Linux rfswift inside this
# distro. See docs/windows-installer.md and docs/nix-engine.md.

$ErrorActionPreference = 'Stop'
$wsl = Join-Path $env:SystemRoot 'System32\wsl.exe'
if (-not (Test-Path $wsl)) {
    Write-Output "wsl.exe not found; enable WSL 2 first. Skipping Nix setup."
    exit 0
}

$distro = if ($env:RFSWIFT_WSL_DISTRO) { $env:RFSWIFT_WSL_DISTRO } else { 'Ubuntu' }
$utf8 = New-Object System.Text.UTF8Encoding($false)

function Invoke-WslFile {
    param([string]$Content, [string]$LocalName, [string[]]$WslArgs)
    $tmp = Join-Path $env:TEMP $LocalName
    [System.IO.File]::WriteAllText($tmp, ($Content -replace "`r`n", "`n"), $utf8)
    $wslPath = (& $wsl -d $distro -e wslpath -a "$tmp").Trim()
    & $wsl @WslArgs $wslPath
}

# 1) Ensure a WSL 2 distribution exists (the container path uses Docker's own
#    --no-distribution VM; Nix needs a real distro).
$have = $false
try { if (& $wsl --list --quiet 2>$null) { $have = $true } } catch { }
if (-not $have) {
    Write-Output "Installing WSL distribution '$distro'..."
    & $wsl --install -d $distro --no-launch
    if ($LASTEXITCODE -ne 0) {
        Write-Output "wsl --install returned $LASTEXITCODE; a restart is likely needed before the distro is usable."
        exit 3010
    }
}

# 2) Enable systemd so the Nix daemon runs cleanly, then restart the distro.
Invoke-WslFile -Content "[boot]`nsystemd=true`n" -LocalName 'wsl.conf' `
    -WslArgs @('-d', $distro, '-u', 'root', '-e', 'cp') 2>$null | Out-Null
& $wsl --terminate $distro 2>$null

# 3) Provision Nix (Determinate, flakes on by default) + the Linux rfswift CLI.
#    Best-effort: a failure to fetch rfswift must not fail Nix itself.
$provision = @'
set -e
export DEBIAN_FRONTEND=noninteractive
if ! command -v curl >/dev/null 2>&1 || ! command -v xz >/dev/null 2>&1; then
  apt-get update -y && apt-get install -y curl xz-utils ca-certificates
fi
if ! command -v nix >/dev/null 2>&1; then
  curl --proto =https --tlsv1.2 -sSf -L https://install.determinate.systems/nix | sh -s -- install --no-confirm
fi
if ! command -v rfswift >/dev/null 2>&1; then
  curl -fsSL https://raw.githubusercontent.com/PentHertz/RF-Swift/refs/heads/main/scripts/get_rfswift.sh | RFSWIFT_INSTALL=cli sh || echo "rfswift CLI not installed; run get_rfswift.sh inside WSL later"
fi
echo "Nix (flakes) and RF Swift CLI are ready in this distro."
'@
Invoke-WslFile -Content $provision -LocalName 'rfswift-nix-provision.sh' `
    -WslArgs @('-d', $distro, '-u', 'root', '-e', 'bash')

Write-Output "Nix in WSL 2 setup finished (distro: $distro). Use:  wsl -d $distro   then   rfswift run --engine nix -i sdr_light -n lab"
exit 0
