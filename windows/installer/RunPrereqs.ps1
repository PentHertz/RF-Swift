# RF Swift bundle prerequisite step: enable the Windows features WSL 2 needs and
# install WSL (no distribution) + WSLg. Chained by Bundle.wxs as an ExePackage
# whose entry payload is a bundled copy of powershell.exe; this script is a
# secondary payload cached beside it (Burn sets the working directory there).
#
# dism.exe / wsl.exe are run IN PLACE from System32 - never bundled, because they
# are servicing- and architecture-locked to the running OS build.
#
# Exit code contract (mapped in Bundle.wxs <ExitCode> children):
#   0    -> success
#   3010 -> a restart is pending; Burn schedules reboot-and-resume
#   other-> error
#
# wsl.exe --install always returns 0 even when a restart is pending, so DISM
# (which returns 3010) is what drives the reboot signal.

$ErrorActionPreference = 'Stop'
$rebootRequired = $false

$dism = Join-Path $env:SystemRoot 'System32\dism.exe'

# VirtualMachinePlatform is the feature WSL 2 actually runs on;
# Microsoft-Windows-Subsystem-Linux is the WSL host. /norestart so Burn, not
# DISM, owns the restart decision.
& $dism /online /enable-feature /featurename:VirtualMachinePlatform `
        /featurename:Microsoft-Windows-Subsystem-Linux /all /norestart
$dismCode = $LASTEXITCODE
if ($dismCode -eq 3010) {
    $rebootRequired = $true
} elseif ($dismCode -ne 0) {
    Write-Output "dism enable-feature failed with exit code $dismCode"
    exit $dismCode
}

# Install/refresh the WSL app, kernel and WSLg (no distribution). Best effort:
# on older builds the flags may be unavailable or need a network, but the
# features enabled above are what containers require, so a failure here must not
# fail the whole bundle - the user can run 'wsl --update' later.
$wsl = Join-Path $env:SystemRoot 'System32\wsl.exe'
if (Test-Path $wsl) {
    try { & $wsl --install --no-distribution } catch { Write-Output "wsl --install skipped: $($_.Exception.Message)" }
    try { & $wsl --update }                    catch { Write-Output "wsl --update skipped: $($_.Exception.Message)" }
}

if ($rebootRequired) { exit 3010 }
exit 0
