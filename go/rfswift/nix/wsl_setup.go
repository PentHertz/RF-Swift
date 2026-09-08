/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
*
*  Nix engine on Windows: provisioning the WSL 2 distribution. This is what the
*  Windows installer's "Set up Nix in WSL 2" step does, available from the CLI
*  (`rfswift nix wsl setup`) and to the Workbench, so a machine that skipped
*  that step, or a new distribution, can be made ready without the installer:
*
*    1. a WSL 2 distribution exists (offer to install Ubuntu when none does);
*    2. systemd is enabled in it, so the nix daemon runs as a service;
*    3. Nix is installed with flakes (the Determinate Systems installer, the
*       path the Nix engine docs endorse), as root;
*    4. the Linux rfswift CLI is installed under /usr/local/bin, matching this
*       binary's version when that release exists, else the latest release.
*
*  Every step is idempotent and asks before changing anything unless Yes is
*  set; output streams to the console so a slow download is visible.
 */

package nix

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	common "penthertz/rfswift/common"
	rfutils "penthertz/rfswift/rfutils"
)

// WSLSetupOptions drives SetupWSL.
type WSLSetupOptions struct {
	// Distro is the distribution to provision. "" means the one the engine
	// resolves (RFSWIFT_WSL_DISTRO, config, then the default distribution), or
	// InstallDistro when none is installed yet.
	Distro string
	// InstallDistro is installed with `wsl --install -d` when no distribution
	// exists (default "Ubuntu").
	InstallDistro string
	// Yes answers every question with yes (scripts, the installer, the GUI).
	Yes bool
	// Confirm is asked before each change when Yes is false; nil refuses.
	Confirm func(question string) bool
	// SkipNix / SkipRFSwift leave those steps out.
	SkipNix     bool
	SkipRFSwift bool
	// Update reinstalls the Linux rfswift even when one is present, so a
	// distribution provisioned by an older RF Swift catches up.
	Update bool
	// RFSwiftVersion is the release tag of the Linux CLI to install ("" = this
	// binary's version, then the latest release).
	RFSwiftVersion string
	// RFSwiftBinary is a local Linux rfswift binary to install instead of a
	// release download (developers).
	RFSwiftBinary string
	// Log receives progress lines (default: the CLI's info messages).
	Log func(string)
	// Output, when set, receives the provisioning commands' output instead of
	// this process's console (GUI callers). The one step that needs a console
	// - the first boot of a freshly installed distribution, which asks for a
	// Linux user - is refused in that mode with instructions.
	Output io.Writer
}

// run executes a provisioning command on the console, or into Output.
func (o WSLSetupOptions) run(cmd *exec.Cmd) error {
	if o.Output == nil {
		return runInteractive(cmd)
	}
	cmd.Stdout, cmd.Stderr = o.Output, o.Output
	return cmd.Run()
}

func (o WSLSetupOptions) log(msg string) {
	if o.Log != nil {
		o.Log(msg)
		return
	}
	common.PrintInfoMessage(msg)
}

func (o WSLSetupOptions) ask(question string) bool {
	if o.Yes {
		return true
	}
	if o.Confirm != nil {
		return o.Confirm(question)
	}
	return false
}

// wslEnableSystemdScript makes /etc/wsl.conf request systemd, whatever the
// file already holds: an existing systemd= line is rewritten, an existing
// [boot] section gains the key, otherwise the section is appended.
const wslEnableSystemdScript = `set -e
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
`

// wslInstallNixScript installs Nix with the Determinate Systems installer as
// root. Without systemd the installer needs the `--init none` planner.
const wslInstallNixScript = `set -e
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
`

// wslTuneNixScript makes `nix store gc` collect the derivation files (.drv)
// and the patches and tarballs they reference once their outputs are live.
// Nix keeps them by default (keep-derivations = true; Determinate ships that
// too), which is what a store looks like after a collection: thousands of
// small recipe files next to a few hundred real outputs. RF Swift users
// build environments, not Nix packages, so the recipes are regenerated by
// the next evaluation and nothing is lost. Determinate reads its user
// settings from nix.custom.conf; a plain multi-user install from nix.conf.
const wslTuneNixScript = `set -e
f=/etc/nix/nix.custom.conf
if [ -f /etc/nix/nix.conf ] && ! grep -q 'nix.custom.conf' /etc/nix/nix.conf; then f=/etc/nix/nix.conf; fi
if grep -qE '^[[:space:]]*keep-derivations[[:space:]]*=' "$f" 2>/dev/null; then
  if grep -qE '^[[:space:]]*keep-derivations[[:space:]]*=[[:space:]]*false' "$f"; then exit 0; fi
  sed -i -E 's/^[[:space:]]*keep-derivations[[:space:]]*=.*/keep-derivations = false/' "$f"
else
  printf '\n# RF Swift: do not keep the recipes (.drv) of live paths; they clutter the store and are regenerated on evaluation.\nkeep-derivations = false\n' >> "$f"
fi
if command -v systemctl >/dev/null 2>&1 && systemctl is-active --quiet nix-daemon 2>/dev/null; then systemctl restart nix-daemon; fi
echo "keep-derivations = false set in $f"
`

// wslInstallRFSwiftScript downloads the first release tag (argument list)
// that has a Linux asset for the distribution's architecture and installs the
// rfswift binary it contains.
const wslInstallRFSwiftScript = `set -e
arch=$(uname -m)
case "$arch" in
  x86_64|amd64) asset=rfswift_Linux_x86_64.tar.gz ;;
  aarch64|arm64) asset=rfswift_Linux_arm64.tar.gz ;;
  riscv64) asset=rfswift_Linux_riscv64.tar.gz ;;
  *) echo "unsupported architecture: $arch" >&2; exit 1 ;;
esac
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
for tag in "$@"; do
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
echo "no RF Swift release asset found for: $*" >&2
exit 1
`

// wslInstallBinaryScript installs a Linux rfswift binary given by path.
const wslInstallBinaryScript = `set -e
install -m 0755 "$1" /usr/local/bin/rfswift
echo "installed $1 to /usr/local/bin/rfswift"
`

// runAsRoot runs a shell script as root inside distro (console or Output).
func (o WSLSetupOptions) runAsRoot(distro, script string, args ...string) error {
	argv := append([]string{"sh", "-c", script, "rfswift-wsl-setup"}, translateArgs(args)...)
	cmd := rfutils.WSLExecAs(distro, "root", argv...)
	if o.Output != nil {
		// A GUI caller reads Output; a terminal window for wsl.exe would only
		// flash on the desktop, empty.
		rfutils.HideConsoleWindow(cmd)
	}
	return o.run(cmd)
}

// SetupWSL provisions a WSL 2 distribution for the Nix engine (see the file
// comment) and returns its state afterwards.
func SetupWSL(opts WSLSetupOptions) (rfutils.WSLNixStatus, error) {
	if !useWSL() {
		return rfutils.WSLNixStatus{}, errNotWindows
	}
	if _, err := rfutils.WSLExePath(); err != nil {
		return rfutils.WSLNixStatus{}, fmt.Errorf("%v\n  Enable WSL 2 first (an administrator PowerShell): wsl --install\n  then reboot and re-run: rfswift nix wsl setup", err)
	}
	if opts.InstallDistro == "" {
		opts.InstallDistro = "Ubuntu"
	}

	// 1) A WSL 2 distribution to work in.
	distro := strings.TrimSpace(opts.Distro)
	if distro == "" {
		distro = strings.TrimSpace(os.Getenv("RFSWIFT_WSL_DISTRO"))
	}
	if distro == "" {
		distro = rfutils.ConfiguredNixWSLDistro(common.ConfigFileByPlatform())
	}
	list, listErr := rfutils.WSLDistributions()
	if distro == "" {
		if listErr == nil {
			if candidates := rfutils.WSLNixCandidates(list); len(candidates) > 0 {
				distro = candidates[0].Name
			}
		}
	}
	if distro == "" {
		opts.log("No WSL 2 Linux distribution is installed (Docker Desktop's utility VM cannot host Nix).")
		if opts.Output != nil {
			return rfutils.WSLNixStatus{}, fmt.Errorf("no WSL 2 Linux distribution is installed. In a terminal run 'wsl --install -d %s' (it asks for a Linux user name on first start), then retry", opts.InstallDistro)
		}
		if !opts.ask(fmt.Sprintf("Install %s with 'wsl --install -d %s' now? You will be asked to create a Linux user.", opts.InstallDistro, opts.InstallDistro)) {
			return rfutils.WSLNixStatus{}, fmt.Errorf("no distribution to provision; install one with: wsl --install -d %s", opts.InstallDistro)
		}
		wsl, _ := rfutils.WSLExePath()
		if err := runInteractive(exec.Command(wsl, "--install", "-d", opts.InstallDistro)); err != nil {
			return rfutils.WSLNixStatus{}, fmt.Errorf("wsl --install -d %s failed (%v); a reboot may be required, then re-run: rfswift nix wsl setup", opts.InstallDistro, err)
		}
		distro = opts.InstallDistro
		list, listErr = rfutils.WSLDistributions()
	}
	if listErr == nil {
		for _, d := range list.Distros {
			if strings.EqualFold(d.Name, distro) {
				distro = d.Name
				if d.Version != 2 {
					return rfutils.WSLNixStatus{}, fmt.Errorf("distribution %s runs WSL %d; the Nix engine needs WSL 2: wsl --set-version %s 2", d.Name, d.Version, d.Name)
				}
			}
		}
	}
	opts.log(fmt.Sprintf("Provisioning the Nix engine in WSL 2 distribution %q ...", distro))
	st, err := rfutils.ProbeWSLNix(distro)
	if err != nil {
		return st, err
	}

	// 2) systemd, so nix-daemon and udev run as services.
	if !st.Systemd {
		if opts.ask(fmt.Sprintf("Enable systemd in %s? (writes [boot] systemd=true to /etc/wsl.conf and restarts the distribution)", distro)) {
			if err := opts.runAsRoot(distro, wslEnableSystemdScript); err != nil {
				return st, fmt.Errorf("enabling systemd failed: %w", err)
			}
			if err := rfutils.WSLTerminate(distro); err != nil {
				opts.log(fmt.Sprintf("Could not restart %s (%v); run 'wsl --terminate %s' yourself before the first use.", distro, err, distro))
			}
			if st, err = rfutils.ProbeWSLNix(distro); err != nil {
				return st, err
			}
			if !st.Systemd {
				opts.log("systemd is still not PID 1; Nix will be installed with its no-init planner (works, without the daemon as a service).")
			}
		} else {
			opts.log("Skipped systemd. Nix works without it; udev rules for hardware need it to be applied automatically.")
		}
	}

	// 3) Nix with flakes.
	if !opts.SkipNix && !st.HasNix() {
		if opts.ask(fmt.Sprintf("Install Nix (Determinate Systems installer, flakes enabled) in %s as root?", distro)) {
			if err := opts.runAsRoot(distro, wslInstallNixScript); err != nil {
				return st, fmt.Errorf("Nix installation failed: %w", err)
			}
		} else {
			opts.log("Skipped Nix. Install it later from https://nixos.org/download inside the distribution.")
		}
	}

	// 3b) Store hygiene: collections drop derivation files too. Idempotent,
	// and applied to an existing installation as well, so a distribution
	// provisioned before this existed catches up on the next setup.
	if !opts.SkipNix {
		if err := opts.runAsRoot(distro, wslTuneNixScript); err != nil {
			opts.log(fmt.Sprintf("Could not set keep-derivations = false in the distribution's nix config (%v); 'rfswift nix gc' will keep .drv files.", err))
		}
	}

	// 4) The Linux rfswift CLI.
	if !opts.SkipRFSwift && (opts.Update || !st.HasRFSwift()) {
		what := "Install"
		if st.HasRFSwift() {
			what = "Update"
		}
		if opts.ask(fmt.Sprintf("%s the Linux rfswift CLI in %s (/usr/local/bin/rfswift)?", what, distro)) {
			if opts.RFSwiftBinary != "" {
				if err := opts.runAsRoot(distro, wslInstallBinaryScript, opts.RFSwiftBinary); err != nil {
					return st, fmt.Errorf("installing %s failed: %w", opts.RFSwiftBinary, err)
				}
			} else if err := opts.runAsRoot(distro, wslInstallRFSwiftScript, rfswiftReleaseTags(opts.RFSwiftVersion)...); err != nil {
				return st, fmt.Errorf("installing the Linux rfswift failed: %w", err)
			}
		} else {
			opts.log("Skipped the Linux rfswift CLI. Inside the distribution: curl -fsSL https://raw.githubusercontent.com/PentHertz/RF-Swift/refs/heads/main/scripts/get_rfswift.sh | RFSWIFT_INSTALL=cli sh")
		}
	}

	// Remember an explicit choice so every later command targets it.
	if strings.TrimSpace(opts.Distro) != "" {
		if err := rfutils.SetConfigValue(common.ConfigFileByPlatform(), "nix", "wsl_distro", distro); err != nil {
			opts.log(fmt.Sprintf("Could not record the distribution in the config file: %v (set RFSWIFT_WSL_DISTRO=%s instead)", err, distro))
		}
	}
	ResetWSLBackend()
	st, err = rfutils.ProbeWSLNix(distro)
	if err != nil {
		return st, err
	}
	if st.Ready() {
		opts.log(fmt.Sprintf("WSL 2 distribution %q is ready for the Nix engine: %s, rfswift %s. Try: rfswift run --engine nix", distro, st.NixVersion, st.RFSwiftVersion))
	} else {
		opts.log(fmt.Sprintf("Distribution %q still lacks %s.", distro, strings.Join(st.Missing(), " and ")))
	}
	return st, nil
}

// rfswiftReleaseTags lists the release tags to try for the Linux CLI, most
// desirable first: the requested one, else this binary's version (so both
// sides agree on flags and JSON shapes) and then the latest published release
// (a development build has no release of its own).
func rfswiftReleaseTags(requested string) []string {
	var tags []string
	add := func(tag string) {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			return
		}
		if !strings.HasPrefix(tag, "v") {
			tag = "v" + tag
		}
		for _, t := range tags {
			if t == tag {
				return
			}
		}
		tags = append(tags, tag)
	}
	if requested != "" {
		add(requested)
		return tags
	}
	add(common.Version)
	if release, err := rfutils.GetLatestRelease(common.Owner, common.Repo); err == nil {
		add(release.TagName)
	}
	return tags
}
