#Requires -Version 5.1
<#
.SYNOPSIS
  Build the RF Swift Windows installer: the MSI (RF Swift itself) and the Burn
  dependency bundle (WSL 2 + usbipd + engine + the MSI).

.DESCRIPTION
  Used both locally and by .github/workflows/windows-installer.yml. Provisions
  the pinned WiX v5 toolset, renders the repo LICENSE to RTF for the MSI license
  page, builds the per-arch MSI, and (with -Bundle) downloads the pinned
  dependency installers and builds the bundle. Build an arm64 bundle on an arm64
  runner so the bundled powershell.exe stub is arm64-native. Authenticode
  signing runs when -CertFile/-CertPassword or -CertThumbprint is supplied;
  otherwise the artifacts are produced unsigned (forks / dry runs).

.EXAMPLE
  # MSI only, from a folder holding rfswift.exe + rfswift-workbench.exe:
  ./build.ps1 -Version 4.0.1-dev -Arch x64 -BinDir C:\path\to\bin

.EXAMPLE
  # MSI + dependency bundle (x64), signed:
  ./build.ps1 -Version 4.0.1 -Arch x64 -BinDir .\bin -Bundle `
      -CertFile cert.pfx -CertPassword $env:CERT_PW
#>
[CmdletBinding()]
param(
    [string]$Version = "4.0.1-dev",
    [ValidateSet("x64", "arm64")][string]$Arch = "x64",
    [Parameter(Mandatory = $true)][string]$BinDir,
    [switch]$Bundle,
    [switch]$SkipDeps,
    [string]$WixVersion = "5.0.2",
    [string]$CertFile,
    [string]$CertPassword,
    [string]$CertThumbprint,
    [string]$TimestampUrl = "http://timestamp.digicert.com"
)

$ErrorActionPreference = "Stop"
$here = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Location $here
$buildDir = Join-Path $here "build"
$depsDir = Join-Path $here "deps\$Arch"   # per-arch so x64/arm64 deps never collide
New-Item -ItemType Directory -Force -Path $buildDir | Out-Null

# MSI ProductVersion must be numeric x.y.z; strip any -dev/-rc suffix. The full
# version string stays in file names.
$msiVersion = ($Version -split '[-+]')[0]
$bundleVersion = "$msiVersion.0"
$binFull = (Resolve-Path $BinDir).Path
$signing = [bool]($CertThumbprint -or $CertFile)

Write-Host "==> RF Swift installer build: version=$Version (msi=$msiVersion) arch=$Arch bundle=$Bundle signing=$signing"

# The pinned dependency set, per architecture. Bump versions together with the
# defaults documented in Bundle.wxs. Docker uses the same build number for both
# arches; only the amd64/arm64 path segment differs. sha256 is verified after
# download when non-empty; leave "" to have the build PRINT the hash to paste
# back here.
$dockerArch = if ($Arch -eq 'arm64') { 'arm64' } else { 'amd64' }
$dockerBuild = "237512"
$usbipdVer = "5.3.0"
$podmanVer = "1.29.1"
$deps = [ordered]@{
    "usbipd-win_${usbipdVer}_$Arch.msi" = @{
        Url    = "https://github.com/dorssel/usbipd-win/releases/download/v$usbipdVer/usbipd-win_${usbipdVer}_$Arch.msi"
        Sha256 = ""
        WixVar = "UsbipdMsi"
        WixUrl = "UsbipdUrl"
    }
    "Docker Desktop Installer.exe" = @{
        # Build-numbered permalink (immutable => hash stable). Update the build
        # number from the Docker Desktop release notes when bumping.
        Url    = "https://desktop.docker.com/win/main/$dockerArch/$dockerBuild/Docker%20Desktop%20Installer.exe"
        Sha256 = ""
        WixVar = "DockerExe"
        WixUrl = "DockerUrl"
    }
    "podman-desktop-${podmanVer}-setup-$Arch.exe" = @{
        Url    = "https://github.com/podman-desktop/podman-desktop/releases/download/v$podmanVer/podman-desktop-${podmanVer}-setup-$Arch.exe"
        Sha256 = ""
        WixVar = "PodmanExe"
        WixUrl = "PodmanUrl"
    }
}

# --------------------------------------------------------------------------
# Toolset
# --------------------------------------------------------------------------
function Ensure-Wix {
    $ok = $false
    try {
        $v = (& wix --version) 2>$null
        if ($LASTEXITCODE -eq 0 -and $v -like "$($WixVersion.Split('.')[0]).*") { $ok = $true }
    } catch { }
    if (-not $ok) {
        Write-Host "==> installing WiX $WixVersion (dotnet tool)"
        & dotnet tool update --global wix --version $WixVersion 2>$null
        if ($LASTEXITCODE -ne 0) { & dotnet tool install --global wix --version $WixVersion }
        $env:PATH = "$env:USERPROFILE\.dotnet\tools;$env:PATH"
    }
    foreach ($ext in @(
            "WixToolset.UI.wixext",
            "WixToolset.BootstrapperApplications.wixext",
            "WixToolset.Util.wixext")) {
        Write-Host "==> wix extension add -g $ext/$WixVersion"
        & wix extension add -g "$ext/$WixVersion" | Out-Null
    }
}

# --------------------------------------------------------------------------
# License.rtf from the repo LICENSE (GPLv3), for the MSI license page.
# --------------------------------------------------------------------------
function New-LicenseRtf {
    param([string]$OutFile)
    $licensePath = Resolve-Path (Join-Path $here "..\..\LICENSE")
    $text = Get-Content -Raw -LiteralPath $licensePath
    $esc = $text -replace '\\', '\\\\' -replace '\{', '\{' -replace '\}', '\}'
    $esc = $esc -replace "`r`n", "`n" -replace "`r", "`n"
    $esc = $esc -replace "`n", "\par`r`n"
    $rtf = "{\rtf1\ansi\ansicpg1252\deff0{\fonttbl{\f0\fnil\fcharset0 Consolas;}}`r`n\f0\fs16 " + $esc + "`r`n}"
    Set-Content -LiteralPath $OutFile -Value $rtf -Encoding ASCII
    Write-Host "==> rendered License.rtf ($((Get-Item $OutFile).Length) bytes)"
}

# --------------------------------------------------------------------------
# Authenticode signing helpers
# --------------------------------------------------------------------------
function Get-SignTool {
    # Prefer a signtool matching the current process architecture (the SDK ships
    # x64/arm64/x86 copies), so this works on both x64 and arm64 runners.
    $want = if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64') { 'arm64' } else { 'x64' }
    $all = Get-ChildItem "C:\Program Files (x86)\Windows Kits\10\bin\*\*\signtool.exe" -ErrorAction SilentlyContinue
    $pref = $all | Where-Object { $_.Directory.Name -eq $want } | Sort-Object FullName -Descending
    if ($pref) { return $pref[0].FullName }
    if ($all) { return ($all | Sort-Object FullName -Descending)[0].FullName }
    $cmd = Get-Command signtool.exe -ErrorAction SilentlyContinue
    if ($cmd) { return $cmd.Source }
    throw "signtool.exe not found (install the Windows SDK)."
}
function Invoke-Sign {
    param([string]$File)
    $st = Get-SignTool
    $a = @("sign", "/fd", "SHA256", "/td", "SHA256", "/tr", $TimestampUrl)
    if ($CertThumbprint) { $a += @("/sha1", $CertThumbprint) }
    elseif ($CertFile) { $a += @("/f", (Resolve-Path $CertFile).Path); if ($CertPassword) { $a += @("/p", $CertPassword) } }
    else { throw "no signing credentials" }
    $a += $File
    & $st @a
    if ($LASTEXITCODE -ne 0) { throw "signtool failed on $File" }
    Write-Host "==> signed $([System.IO.Path]::GetFileName($File))"
}

# --------------------------------------------------------------------------
# Bundle dependency download + pinned-hash verification
# --------------------------------------------------------------------------
function Get-Deps {
    New-Item -ItemType Directory -Force -Path $depsDir | Out-Null
    $ProgressPreference = 'SilentlyContinue'
    foreach ($name in $deps.Keys) {
        $d = $deps[$name]
        $dest = Join-Path $depsDir $name
        if (-not (Test-Path -LiteralPath $dest)) {
            Write-Host "==> downloading $name"
            Invoke-WebRequest -Uri $d.Url -OutFile $dest -UseBasicParsing
        }
        $actual = (Get-FileHash -LiteralPath $dest -Algorithm SHA256).Hash.ToLower()
        if ([string]::IsNullOrWhiteSpace($d.Sha256)) {
            Write-Warning "no pinned SHA-256 for '$name'. Computed: $actual  (paste into `$deps in build.ps1 to enforce)"
        } elseif ($actual -ne $d.Sha256.ToLower()) {
            throw "SHA-256 mismatch for '$name': expected $($d.Sha256), got $actual"
        } else {
            Write-Host "==> verified $name"
        }
    }
}

# --------------------------------------------------------------------------
Ensure-Wix
$rtf = Join-Path $here "License.rtf"
New-LicenseRtf -OutFile $rtf

# 1) MSI (RF Swift itself)
$msi = Join-Path $buildDir "RFSwift-$Version-$Arch.msi"
Write-Host "==> building MSI $([System.IO.Path]::GetFileName($msi))"
& wix build -arch $Arch `
    -ext "WixToolset.UI.wixext/$WixVersion" `
    -d "RFSwiftVersion=$msiVersion" `
    -d "BinDir=$binFull" `
    -d "AssetsDir=assets" `
    -d "LicenseRtf=$rtf" `
    (Join-Path $here "Package.wxs") `
    -o $msi
if ($LASTEXITCODE -ne 0) { throw "wix build (MSI) failed" }
if ($signing) { Invoke-Sign $msi }
Write-Host "==> MSI: $msi"

# 2) Bundle (x64 and arm64). arm64 must be built on an arm64 runner so the
#    bundled powershell.exe stub (WSL/Nix prerequisite steps) is arm64-native.
if ($Bundle) {
    if (-not $SkipDeps) { Get-Deps }

    # A stable-named copy of the MSI for the bundle to embed, kept out of the
    # top-level build dir so release collection does not pick it up.
    $embedDir = Join-Path $buildDir "_embed"
    New-Item -ItemType Directory -Force -Path $embedDir | Out-Null
    $msiForBundle = Join-Path $embedDir "RFSwift-$Arch.msi"
    Copy-Item -LiteralPath $msi -Destination $msiForBundle -Force

    $depArgs = @()
    foreach ($name in $deps.Keys) {
        $d = $deps[$name]
        $depArgs += @("-d", "$($d.WixVar)=$name", "-d", "$($d.WixUrl)=$($d.Url)")
    }

    $bundle = Join-Path $buildDir "RFSwift-Setup-$Version-$Arch.exe"
    Write-Host "==> building bundle $([System.IO.Path]::GetFileName($bundle))"
    & wix build -arch $Arch `
        -ext "WixToolset.BootstrapperApplications.wixext/$WixVersion" `
        -ext "WixToolset.Util.wixext/$WixVersion" `
        -d "RFSwiftVersion=$msiVersion" `
        -d "RFSwiftTag=$Version" `
        -d "BundleVersion=$bundleVersion" `
        -d "MsiFile=$msiForBundle" `
        -d "DepsDir=$depsDir" `
        -d "AssetsDir=assets" `
        -d "ThemeDir=theme" `
        @depArgs `
        (Join-Path $here "Bundle.wxs") `
        -o $bundle
    if ($LASTEXITCODE -ne 0) { throw "wix build (bundle) failed" }

    if ($signing) {
        # Sign the Burn engine and the outer bundle separately (detach/reattach)
        # so the extracted engine used for repair/uninstall is itself signed.
        $engine = Join-Path $buildDir "burnengine.exe"
        & wix burn detach $bundle -engine $engine
        if ($LASTEXITCODE -ne 0) { throw "wix burn detach failed" }
        Invoke-Sign $engine
        $tmp = "$bundle.reattached"
        & wix burn reattach $bundle -engine $engine -o $tmp
        if ($LASTEXITCODE -ne 0) { throw "wix burn reattach failed" }
        Move-Item -LiteralPath $tmp -Destination $bundle -Force
        Invoke-Sign $bundle
    }
    Write-Host "==> Bundle: $bundle"
}

Write-Host "==> done. Artifacts in $buildDir"
Get-ChildItem $buildDir -Filter "RFSwift*" | ForEach-Object { "    {0}  {1:N0} bytes" -f $_.Name, $_.Length }
