/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
*
*  `rfswift update`: upgrades rfswift the way it was installed. A deb/rpm/
*  pacman package is replaced by the release's package through the
*  distribution's package manager, the Homebrew cask through brew, and a
*  standalone binary in place. Everything downloaded is checked against the
*  release's SHA-256 list before it is installed.
 */

package rfutils

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	common "penthertz/rfswift/common"
	"penthertz/rfswift/tui"
)

// UpdateOptions tunes SelfUpdate.
type UpdateOptions struct {
	Yes   bool // update without asking (scripts, no terminal)
	Check bool // only report whether an update exists and how it would be applied
}

// updatePlan is what `rfswift update` will do for this install.
type updatePlan struct {
	method installMethod
	exe    string   // resolved path of the running binary
	asset  string   // release file to download ("" for Homebrew)
	sudo   bool     // the install step goes through sudo
	steps  []string // what the update does, shown before asking
	// manual is set when rfswift cannot apply the update itself: why, then
	// the commands or links the user should use instead.
	manualWhy string
	manual    []string
}

// SelfUpdate checks GitHub for a newer RF Swift release and, once the user
// agrees (or opts.Yes), installs it the way this rfswift was installed.
//
//	in(1): UpdateOptions opts
//	out: error  non-nil when the check or the update failed
func SelfUpdate(opts UpdateOptions) error {
	if common.Disconnected {
		return fmt.Errorf("'rfswift update' needs to reach GitHub: run it without -q/--disconnect")
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot locate the running rfswift: %v", err)
	}
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	if runtime.GOOS == "windows" {
		// Left behind by the previous update (a running .exe can be renamed
		// but not deleted).
		os.Remove(exe + ".old")
	}

	release, err := getLatestRelease(common.Owner, common.Repo, 15*time.Second)
	if err != nil {
		return fmt.Errorf("could not check GitHub for the latest release: %v", err)
	}
	current := strings.TrimPrefix(common.Version, "v")
	latest := strings.TrimPrefix(release.TagName, "v")

	if VersionCompare(current, latest) >= 0 {
		common.PrintSuccessMessage(fmt.Sprintf("rfswift %s is up to date (latest release: %s).", current, latest))
		return nil
	}

	plan := planUpdate(exe, detectInstallMethod(exe, runtime.GOOS), latest, release.TagName)

	common.PrintInfoBoxTitled("Update available", fmt.Sprintf("rfswift %s → %s\nInstalled as: %s at %s\nRelease notes: %s",
		current, latest, plan.method, exe, releaseNotesURL(release.TagName)))

	if plan.manualWhy != "" {
		common.PrintWarningMessage(plan.manualWhy)
		for _, line := range plan.manual {
			fmt.Printf("    %s\n", line)
		}
		fmt.Println()
		return nil
	}

	common.PrintInfoMessage("The update will:")
	for i, s := range plan.steps {
		fmt.Printf("    %d. %s\n", i+1, s)
	}
	fmt.Println()

	if opts.Check {
		common.PrintInfoMessage("Run 'rfswift update' to apply it.")
		return nil
	}
	if !opts.Yes {
		if !tui.IsInteractive() {
			return fmt.Errorf("no terminal to confirm the update: re-run with 'rfswift update --yes'")
		}
		if !tui.ConfirmDefault(fmt.Sprintf("Update rfswift %s → %s?", current, latest), true) {
			common.PrintInfoMessage("Update cancelled.")
			return nil
		}
	}

	if err := plan.apply(release.TagName, latest); err != nil {
		return err
	}
	reportUpdated(plan, current, latest)
	return nil
}

// releaseNotesURL is the GitHub page of the release tagged tag.
func releaseNotesURL(tag string) string {
	return fmt.Sprintf("https://github.com/%s/%s/releases/tag/%s", common.Owner, common.Repo, tag)
}

// planUpdate decides how to update the rfswift at exe, installed by method,
// to version (tag is the release tag).
func planUpdate(exe string, method installMethod, version, tag string) updatePlan {
	p := updatePlan{method: method, exe: exe}
	releasesPage := releaseNotesURL(tag)
	_, sudoErr := exec.LookPath("sudo")
	haveSudo := sudoErr == nil
	verifyStep := func(asset string) string {
		return fmt.Sprintf("download %s and check its SHA-256 against the release checksums", asset)
	}

	switch method {
	case installHomebrew:
		if isRoot() {
			p.manualWhy = "Homebrew refuses to run as root: update from your own account with:"
			p.manual = []string{"brew upgrade --cask rfswift"}
			return p
		}
		if _, err := exec.LookPath("brew"); err != nil {
			p.manualWhy = "This rfswift comes from Homebrew, but brew is not on PATH. Update it with:"
			p.manual = []string{"brew upgrade --cask rfswift"}
			return p
		}
		p.steps = []string{"run: brew upgrade --cask rfswift"}

	case installWindowsSetup:
		p.manualWhy = fmt.Sprintf("%s belongs to the RF Swift Windows installer, so it is updated by running the new installer:", exe)
		if asset, ok := windowsSetupAssetName(version, runtime.GOARCH); ok {
			p.manual = []string{ConstructDownloadURL(common.Owner, common.Repo, tag, asset)}
		} else {
			p.manual = []string{releasesPage}
		}

	case installDeb, installRPM, installPacman:
		asset, ok := packageAssetName(method, version, runtime.GOARCH)
		if !ok {
			p.manualWhy = fmt.Sprintf("The %s release has no %s for %s; pick an install from:", version, method, runtime.GOARCH)
			p.manual = []string{releasesPage}
			return p
		}
		p.asset = asset
		argv := packageInstallArgs(method, "./"+asset, exec.LookPath)
		if !isRoot() {
			if !haveSudo {
				p.manualWhy = fmt.Sprintf("Installing the %s needs root, and sudo is not available. As root, download and install it:", method)
				p.manual = []string{ConstructDownloadURL(common.Owner, common.Repo, tag, asset), strings.Join(argv, " ")}
				return p
			}
			p.sudo = true
			argv = append([]string{"sudo"}, argv...)
		}
		p.steps = []string{verifyStep(asset), "install it: " + strings.Join(argv, " ")}

	default:
		asset, ok := archiveAssetName(runtime.GOOS, runtime.GOARCH)
		if !ok {
			p.manualWhy = fmt.Sprintf("RF Swift publishes no build for %s/%s; see:", runtime.GOOS, runtime.GOARCH)
			p.manual = []string{releasesPage}
			return p
		}
		p.asset = asset
		dir := filepath.Dir(exe)
		switch {
		case dirWritable(dir):
			p.steps = []string{verifyStep(asset), "replace " + exe}
		case runtime.GOOS == "windows":
			p.manualWhy = fmt.Sprintf("%s is not writable from this account. Run the update from an Administrator terminal:", dir)
			p.manual = []string{"rfswift update"}
		case !isRoot() && haveSudo:
			p.sudo = true
			p.steps = []string{verifyStep(asset), fmt.Sprintf("replace %s with sudo (%s is not writable by you)", exe, dir)}
		default:
			p.manualWhy = fmt.Sprintf("%s is not writable. Update as a user who can write there, or reinstall with:", dir)
			p.manual = []string{`curl -fsSL "https://raw.githubusercontent.com/PentHertz/RF-Swift/refs/heads/main/scripts/get_rfswift.sh" | sh`}
		}
	}
	return p
}

// apply carries out the plan for the release tagged tag (version is the tag
// without its "v").
func (p updatePlan) apply(tag, version string) error {
	if p.method == installHomebrew {
		return runVisible("brew", "upgrade", "--cask", "rfswift")
	}

	tmp, err := os.MkdirTemp("", "rfswift-update-")
	if err != nil {
		return fmt.Errorf("cannot create a download directory: %v", err)
	}
	defer os.RemoveAll(tmp)
	// apt-get hands local packages to its unprivileged _apt user, which must
	// be able to read them; MkdirTemp's 0700 makes it fall back to an
	// unsandboxed read with a notice.
	_ = os.Chmod(tmp, 0o755)

	sums, err := fetchChecksums(tag, version)
	if err != nil {
		return err
	}
	file, err := downloadVerified(tag, p.asset, tmp, sums)
	if err != nil {
		return err
	}

	switch p.method {
	case installDeb, installRPM, installPacman:
		argv := packageInstallArgs(p.method, file, exec.LookPath)
		if p.sudo {
			argv = append([]string{"sudo"}, argv...)
		}
		return runVisible(argv...)
	}

	binName := "rfswift"
	if runtime.GOOS == "windows" {
		binName = "rfswift.exe"
	}
	newBin := filepath.Join(tmp, binName)
	if err := extractBinary(file, binName, newBin); err != nil {
		return fmt.Errorf("cannot unpack %s: %v", p.asset, err)
	}
	if p.sudo {
		// One privileged step: install(1) unlinks the old file and writes
		// the new one, which is safe while the old one is running.
		return runVisible("sudo", "install", "-m", "0755", newBin, p.exe)
	}
	if err := replaceExecutable(newBin, p.exe); err != nil {
		return fmt.Errorf("cannot replace %s: %v", p.exe, err)
	}
	return nil
}

// runVisible runs argv attached to the terminal (sudo can ask for a
// password, the package manager shows its progress), after showing it.
func runVisible(argv ...string) error {
	common.PrintInfoMessage("Running: " + strings.Join(argv, " "))
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("'%s' failed: %v", strings.Join(argv, " "), err)
	}
	return nil
}

// reportUpdated confirms the new version by asking the installed binary, and
// warns when the shell would still start another copy.
func reportUpdated(p updatePlan, current, latest string) {
	target := p.exe
	if p.method == installHomebrew {
		// The cask moved the binary to a new Caskroom directory.
		if onPath, err := exec.LookPath("rfswift"); err == nil {
			target = onPath
		}
	}
	got, err := binaryVersion(target)
	switch {
	case err != nil:
		common.PrintSuccessMessage(fmt.Sprintf("rfswift %s installed.", latest))
	case VersionCompare(got, latest) >= 0:
		common.PrintSuccessMessage(fmt.Sprintf("rfswift updated: %s → %s", current, got))
	case p.method == installHomebrew:
		common.PrintWarningMessage(fmt.Sprintf("Homebrew finished, but rfswift still reports %s. The cask is published once the macOS build of %s is notarized; try 'brew upgrade --cask rfswift' again later.", got, latest))
		return
	default:
		common.PrintWarningMessage(fmt.Sprintf("The update ran, but %s still reports %s instead of %s.", target, got, latest))
		return
	}
	if p.method != installHomebrew {
		warnIfShadowed(p.exe)
	}
}

// binaryVersion runs `bin --version` ("rfswift version 4.0.5") and returns
// the version.
func binaryVersion(bin string) (string, error) {
	cmd := exec.Command(bin, "--version")
	cmd.Env = append(os.Environ(), "RFSWIFT_NO_BANNER=1")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	fields := strings.Fields(string(out))
	if len(fields) == 0 {
		return "", fmt.Errorf("no version in %q", out)
	}
	return strings.TrimPrefix(fields[len(fields)-1], "v"), nil
}

// warnIfShadowed warns when `rfswift` on PATH is another file than the one
// just updated (typically an old tarball copy in /usr/local/bin in front of
// the packaged /usr/bin/rfswift), so the user would keep running the old one.
func warnIfShadowed(updated string) {
	if isRoot() && os.Getenv("SUDO_USER") != "" {
		return // sudo's secure_path is not the user's PATH
	}
	onPath, err := exec.LookPath("rfswift")
	if err != nil {
		return
	}
	real := onPath
	if r, err := filepath.EvalSymlinks(onPath); err == nil {
		real = r
	}
	if real == updated || (runtime.GOOS == "windows" && strings.EqualFold(real, updated)) {
		return
	}
	common.PrintWarningMessage(fmt.Sprintf("Your shell runs %s, not the updated %s. Remove that older copy (or put %s first in PATH) so 'rfswift' starts the new version.",
		onPath, updated, filepath.Dir(updated)))
}

// checksumsAssetName is the release's SHA-256 list (goreleaser's
// "<project>_<version>_checksums.txt"), which covers the archives and the
// Linux packages.
func checksumsAssetName(version string) string {
	return fmt.Sprintf("RF-Swift_%s_checksums.txt", version)
}

// fetchChecksums downloads and parses the release's SHA-256 list.
func fetchChecksums(tag, version string) (map[string]string, error) {
	url := ConstructDownloadURL(common.Owner, common.Repo, tag, checksumsAssetName(version))
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("cannot fetch the release checksums (%s): %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cannot fetch the release checksums (%s): %s", url, resp.Status)
	}
	sums, err := parseChecksums(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("cannot read the release checksums: %v", err)
	}
	return sums, nil
}

// parseChecksums reads sha256sum output ("<hex>  <file>", or "<hex> *<file>"
// in binary mode) into file -> lowercase hex digest.
func parseChecksums(r io.Reader) (map[string]string, error) {
	sums := map[string]string{}
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) != 2 || len(f[0]) != sha256.Size*2 {
			continue
		}
		if _, err := hex.DecodeString(f[0]); err != nil {
			continue
		}
		sums[strings.TrimPrefix(f[1], "*")] = strings.ToLower(f[0])
	}
	return sums, sc.Err()
}

// downloadVerified downloads the release asset into dir and checks it
// against sums. It fails closed: an asset missing from the list, or one whose
// digest differs, is deleted and never installed.
//
//	out: string  path of the verified file
func downloadVerified(tag, asset, dir string, sums map[string]string) (string, error) {
	want, ok := sums[asset]
	if !ok {
		return "", fmt.Errorf("the release checksums do not list %s, so it cannot be verified; nothing was installed", asset)
	}
	dest := filepath.Join(dir, asset)
	if err := DownloadFile(ConstructDownloadURL(common.Owner, common.Repo, tag, asset), dest); err != nil {
		return "", fmt.Errorf("cannot download %s: %v", asset, err)
	}
	got, err := sha256File(dest)
	if err != nil {
		return "", err
	}
	if got != want {
		os.Remove(dest)
		return "", fmt.Errorf("SHA-256 mismatch for %s (expected %s, got %s): the download is corrupt or was tampered with; nothing was installed", asset, want, got)
	}
	common.PrintInfoMessage(fmt.Sprintf("SHA-256 verified: %s", asset))
	return dest, nil
}

// sha256File returns the lowercase hex SHA-256 of the file at p.
func sha256File(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// maxBinarySize bounds what extractBinary writes, so a malformed archive
// cannot fill the disk.
const maxBinarySize = 1 << 30

// extractBinary copies the file named name at the top level of the .tar.gz
// or .zip archive to dst (created, mode 0755). Only that one entry is read,
// so the archive's other paths are never written anywhere.
func extractBinary(archive, name, dst string) error {
	isEntry := func(entry string) bool {
		clean := path.Clean(strings.TrimPrefix(entry, "./"))
		return clean == name
	}
	write := func(r io.Reader) error {
		out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o755)
		if err != nil {
			return err
		}
		n, err := io.Copy(out, io.LimitReader(r, maxBinarySize+1))
		if cerr := out.Close(); err == nil {
			err = cerr
		}
		if err == nil && n > maxBinarySize {
			err = fmt.Errorf("%s is larger than %d bytes", name, maxBinarySize)
		}
		return err
	}

	if strings.HasSuffix(archive, ".zip") {
		zr, err := zip.OpenReader(archive)
		if err != nil {
			return err
		}
		defer zr.Close()
		for _, f := range zr.File {
			if f.FileInfo().Mode().IsRegular() && isEntry(f.Name) {
				rc, err := f.Open()
				if err != nil {
					return err
				}
				defer rc.Close()
				return write(rc)
			}
		}
		return fmt.Errorf("no %s in the archive", name)
	}

	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return fmt.Errorf("no %s in the archive", name)
		}
		if err != nil {
			return err
		}
		if hdr.Typeflag == tar.TypeReg && isEntry(hdr.Name) {
			return write(tr)
		}
	}
}

// replaceExecutable puts newBin in place of exe. The new binary is first
// copied next to exe, then renamed over it, so exe is never half-written and
// a running copy keeps working. Windows cannot rename over a running .exe,
// but can rename it away: it becomes exe.old, removed by the next update.
func replaceExecutable(newBin, exe string) error {
	tmp, err := os.CreateTemp(filepath.Dir(exe), ".rfswift-update-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once renamed

	src, err := os.Open(newBin)
	if err != nil {
		tmp.Close()
		return err
	}
	_, err = io.Copy(tmp, src)
	src.Close()
	if err == nil {
		err = tmp.Sync()
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o755); err != nil {
		return err
	}

	if runtime.GOOS != "windows" {
		return os.Rename(tmpName, exe)
	}
	old := exe + ".old"
	os.Remove(old)
	if err := os.Rename(exe, old); err != nil {
		return err
	}
	if err := os.Rename(tmpName, exe); err != nil {
		os.Rename(old, exe)
		return err
	}
	return nil
}
