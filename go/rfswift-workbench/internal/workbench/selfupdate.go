/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
 */

package workbench

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
	common "penthertz/rfswift/common"
	"penthertz/rfswift/hostsetup"
	"penthertz/rfswift/rfutils"
)

// Workbench self-update, driven from the Engine doctor. The latest release is
// read from the GitHub API; the asset matching how this Workbench was
// installed is downloaded to a private directory, checked against the digest
// GitHub publishes, and installed only once its build provenance verifies
// (selfupdate_verify.go):
//
//   - AppImage: the file named by $APPIMAGE is replaced in place.
//   - deb/rpm/pacman: the package is installed by the system package manager,
//     as root through the polkit or sudo prompt. Root copies the file into a
//     directory only it can write and checks the digest again there, so the
//     file cannot be swapped between the check and the install.
//   - tar.gz binary: the binary is replaced in place, through the same root
//     path when its directory is not writable.
//   - macOS / Windows: the signed DMG or MSI is opened for the user; macOS
//     Gatekeeper and Windows check their code signatures before installing.

// WorkbenchUpdate is the update state shown in the Engine doctor.
type WorkbenchUpdate struct {
	Current         string `json:"current"`
	Latest          string `json:"latest"`
	UpdateAvailable bool   `json:"updateAvailable"`
	CanInstall      bool   `json:"canInstall"`
	Method          string `json:"method"`
	Asset           string `json:"asset"`
	Detail          string `json:"detail"`
	ReleaseURL      string `json:"releaseURL"`
}

type ghAsset struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	Digest string `json:"digest"`
	URL    string `json:"browser_download_url"`
}

type ghRelease struct {
	TagName string    `json:"tag_name"`
	HTMLURL string    `json:"html_url"`
	Assets  []ghAsset `json:"assets"`
}

const (
	updateStatusTTL = 15 * time.Minute
	maxUpdateSize   = 1 << 30
	releasesPage    = "https://github.com/PentHertz/RF-Swift/releases"
)

var (
	releaseTagPattern = regexp.MustCompile(`^v\d+\.\d+\.\d+$`)
	assetNamePattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]*$`)
)

type updateState struct {
	mu      sync.Mutex
	rel     *ghRelease
	checked time.Time
	running bool
	once    sync.Once
	method  string
	target  string
}

// updateTransport is nil (the default transport) except in tests.
var updateTransport http.RoundTripper

// updateHTTPClient refuses redirects to anything but HTTPS.
func updateHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: updateTransport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if req.URL.Scheme != "https" {
				return errors.New("refusing a non-HTTPS redirect")
			}
			if len(via) >= 10 {
				return errors.New("too many redirects")
			}
			return nil
		},
	}
}

func fetchLatestRelease(ctx context.Context, hc *http.Client) (*ghRelease, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/releases/latest", githubAPI, updateOwner, updateRepo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub answered %s", resp.Status)
	}
	var rel ghRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&rel); err != nil {
		return nil, err
	}
	if !releaseTagPattern.MatchString(rel.TagName) {
		return nil, fmt.Errorf("unexpected release tag %q", rel.TagName)
	}
	return &rel, nil
}

// latestRelease returns the cached latest release, refreshed when older than
// updateStatusTTL or when refresh is set. The Engine doctor polls every few
// seconds and GitHub allows 60 anonymous API calls an hour.
func (a *App) latestRelease(refresh bool) (*ghRelease, error) {
	a.update.mu.Lock()
	if !refresh && a.update.rel != nil && time.Since(a.update.checked) < updateStatusTTL {
		rel := a.update.rel
		a.update.mu.Unlock()
		return rel, nil
	}
	a.update.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	rel, err := fetchLatestRelease(ctx, updateHTTPClient(15*time.Second))
	if err != nil {
		return nil, err
	}
	a.update.mu.Lock()
	a.update.rel, a.update.checked = rel, time.Now()
	a.update.mu.Unlock()
	return rel, nil
}

// installMethod tells how this Workbench was installed, and the file an
// in-place update replaces. Asking the package managers takes a moment, so
// it is done once.
func (a *App) installMethod() (string, string) {
	a.update.once.Do(func() { a.update.method, a.update.target = detectInstallMethod() })
	return a.update.method, a.update.target
}

func detectInstallMethod() (string, string) {
	exe, err := os.Executable()
	if err == nil {
		if real, err := filepath.EvalSymlinks(exe); err == nil {
			exe = real
		}
	}
	switch runtime.GOOS {
	case "linux":
		if p := os.Getenv("APPIMAGE"); p != "" && filepath.IsAbs(p) {
			return "appimage", p
		}
		for _, o := range []struct {
			tool, method string
			args         []string
		}{
			{"dpkg", "deb", []string{"-S", exe}},
			{"rpm", "rpm", []string{"-qf", exe}},
			{"pacman", "pacman", []string{"-Qo", exe}},
		} {
			if _, err := exec.LookPath(o.tool); err != nil {
				continue
			}
			if exec.Command(o.tool, o.args...).Run() == nil {
				return o.method, exe
			}
		}
		return "binary", exe
	case "darwin":
		return "macos-dmg", exe
	case "windows":
		return "windows-msi", exe
	}
	return "", exe
}

// updateAssetName is the release asset an install method updates from.
func updateAssetName(method, version, goarch string) (string, error) {
	if goarch != "amd64" && goarch != "arm64" {
		return "", fmt.Errorf("no Workbench release is built for %s", goarch)
	}
	pick := func(amd64, arm64 string) string {
		if goarch == "arm64" {
			return arm64
		}
		return amd64
	}
	switch method {
	case "appimage":
		return "rfswift-workbench_Linux_" + pick("x86_64", "arm64") + ".AppImage", nil
	case "binary":
		return "rfswift-workbench_Linux_" + pick("x86_64", "arm64") + ".tar.gz", nil
	case "deb":
		return "rfswift-workbench_" + version + "_" + pick("amd64", "arm64") + ".deb", nil
	case "rpm":
		return "rfswift-workbench-" + version + "-1." + pick("x86_64", "aarch64") + ".rpm", nil
	case "pacman":
		return "rfswift-workbench-" + version + "-1-" + pick("x86_64", "aarch64") + ".pkg.tar.zst", nil
	case "macos-dmg":
		return "rfswift_Darwin_universal.dmg", nil
	case "windows-msi":
		return "RFSwift-" + version + "-" + pick("x64", "arm64") + ".msi", nil
	}
	return "", errors.New("this Workbench was not installed from a release, so it cannot update itself")
}

func findAsset(rel *ghRelease, name string) (ghAsset, bool) {
	for _, a := range rel.Assets {
		if a.Name == name {
			return a, true
		}
	}
	return ghAsset{}, false
}

// WorkbenchUpdateStatus compares this Workbench with the latest release.
// A failed check is reported in Detail rather than as an error, so the rest
// of the Engine doctor still renders offline.
func (a *App) WorkbenchUpdateStatus(refresh bool) WorkbenchUpdate {
	method, _ := a.installMethod()
	st := WorkbenchUpdate{Current: common.Version, Method: method, ReleaseURL: releasesPage}
	rel, err := a.latestRelease(refresh)
	if err != nil {
		st.Detail = "Could not check for updates: " + err.Error()
		return st
	}
	st.Latest = strings.TrimPrefix(rel.TagName, "v")
	if strings.HasPrefix(rel.HTMLURL, releasesPage+"/") {
		st.ReleaseURL = rel.HTMLURL
	}
	switch cmp := rfutils.VersionCompare(common.Version, rel.TagName); {
	case cmp > 0:
		st.Detail = "This build is newer than the latest release (development build)."
		return st
	case cmp == 0:
		st.Detail = "Up to date."
		return st
	}
	st.UpdateAvailable = true
	name, err := updateAssetName(method, st.Latest, runtime.GOARCH)
	if err != nil {
		st.Detail = err.Error() + " Download it from the release page."
		return st
	}
	st.Asset = name
	if _, ok := findAsset(rel, name); !ok {
		st.Detail = name + " is not part of release " + rel.TagName + ". Download the update from the release page."
		return st
	}
	st.CanInstall = true
	st.Detail = updateMethodHint(method)
	return st
}

func updateMethodHint(method string) string {
	switch method {
	case "appimage":
		return "Replaces this AppImage after verifying the download."
	case "deb", "rpm", "pacman":
		return "Installs the verified " + method + " package with your package manager (asks for your password)."
	case "binary":
		return "Replaces this binary after verifying the download."
	case "macos-dmg":
		return "Downloads and verifies the signed disk image, then opens it for you to install."
	case "windows-msi":
		return "Downloads and verifies the signed installer, then starts it."
	}
	return ""
}

func (a *App) emitUpdateProgress(stage string, done, total int64) {
	if a.ctx == nil {
		return
	}
	wruntime.EventsEmit(a.ctx, "rfswift:update:progress", map[string]any{"stage": stage, "done": done, "total": total})
}

// UpdateWorkbench installs the latest release when its download verifies.
// The returned state's Detail says what happened and what to do next.
func (a *App) UpdateWorkbench() (WorkbenchUpdate, error) {
	a.update.mu.Lock()
	if a.update.running {
		a.update.mu.Unlock()
		return WorkbenchUpdate{}, errors.New("an update is already running")
	}
	a.update.running = true
	a.update.mu.Unlock()
	defer func() {
		a.update.mu.Lock()
		a.update.running = false
		a.update.mu.Unlock()
	}()

	st := a.WorkbenchUpdateStatus(true)
	if !st.UpdateAvailable {
		return st, nil
	}
	if !st.CanInstall {
		return st, errors.New(st.Detail)
	}
	rel, err := a.latestRelease(false)
	if err != nil {
		return st, err
	}
	asset, _ := findAsset(rel, st.Asset)
	dir, err := os.MkdirTemp("", "rfswift-workbench-update-")
	if err != nil {
		return st, err
	}
	keep := false // the DMG must outlive this call: Finder mounts it after `open` returns
	defer func() {
		if !keep {
			_ = os.RemoveAll(dir)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	hc := updateHTTPClient(30 * time.Minute)
	path, digest, err := downloadAsset(ctx, hc, rel.TagName, asset, dir, func(done, total int64) {
		a.emitUpdateProgress("download", done, total)
	})
	if err != nil {
		return st, err
	}
	a.emitUpdateProgress("verify", 0, 0)
	bundles, err := fetchAttestations(ctx, updateHTTPClient(30*time.Second), digest)
	if err != nil {
		return st, err
	}
	tm, err := sigstoreTrustedRoot()
	if err != nil {
		return st, err
	}
	if err := verifyProvenance(tm, bundles, digest, asset.Name, rel.TagName); err != nil {
		return st, err
	}
	a.emitUpdateProgress("install", 0, 0)
	method, target := a.installMethod()
	detail, err := installUpdate(method, target, path, asset.Name, hex.EncodeToString(digest), dir)
	if err != nil {
		return st, err
	}
	keep = method == "macos-dmg"
	st.Detail = detail
	return st, nil
}

// downloadAsset fetches a release asset into dir and returns its path and
// SHA-256. It accepts only the asset's own release URL, its announced size,
// and the digest GitHub publishes for it.
func downloadAsset(ctx context.Context, hc *http.Client, tag string, a ghAsset, dir string, progress func(done, total int64)) (string, []byte, error) {
	if !assetNamePattern.MatchString(a.Name) {
		return "", nil, fmt.Errorf("unexpected asset name %q", a.Name)
	}
	want := fmt.Sprintf("https://github.com/%s/%s/releases/download/%s/%s", updateOwner, updateRepo, tag, a.Name)
	if a.URL != want {
		return "", nil, fmt.Errorf("unexpected download location for %s", a.Name)
	}
	if a.Size <= 0 || a.Size > maxUpdateSize {
		return "", nil, fmt.Errorf("unexpected size for %s", a.Name)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.URL, nil)
	if err != nil {
		return "", nil, err
	}
	resp, err := hc.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("download failed: %s", resp.Status)
	}
	path := filepath.Join(dir, a.Name)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", nil, err
	}
	h := sha256.New()
	pw := &progressWriter{total: a.Size, report: progress}
	n, copyErr := io.Copy(io.MultiWriter(f, h, pw), io.LimitReader(resp.Body, a.Size+1))
	if closeErr := f.Close(); copyErr == nil {
		copyErr = closeErr
	}
	if copyErr != nil {
		return "", nil, fmt.Errorf("download failed: %w", copyErr)
	}
	if n != a.Size {
		return "", nil, fmt.Errorf("download of %s is %d bytes, expected %d", a.Name, n, a.Size)
	}
	sum := h.Sum(nil)
	if a.Digest != "" && a.Digest != "sha256:"+hex.EncodeToString(sum) {
		return "", nil, fmt.Errorf("%s does not match the digest GitHub published for it", a.Name)
	}
	return path, sum, nil
}

// progressWriter reports download progress at most every 250 ms.
type progressWriter struct {
	total, done int64
	last        time.Time
	report      func(done, total int64)
}

func (p *progressWriter) Write(b []byte) (int, error) {
	p.done += int64(len(b))
	if p.report != nil && (time.Since(p.last) > 250*time.Millisecond || p.done == p.total) {
		p.last = time.Now()
		p.report(p.done, p.total)
	}
	return len(b), nil
}

// installUpdate installs a verified download according to the install method.
func installUpdate(method, target, path, name, hexDigest, dir string) (string, error) {
	restart := "Restart the Workbench to use the new version."
	switch method {
	case "appimage":
		if err := replaceFile(path, target, 0o755); err != nil {
			return "", fmt.Errorf("could not replace %s: %w", target, err)
		}
		return "The AppImage was updated. " + restart, nil
	case "binary":
		bin, binDigest, err := extractWorkbenchBinary(path, dir)
		if err != nil {
			return "", err
		}
		if err := replaceFile(bin, target, 0o755); err == nil {
			return "The Workbench binary was updated. " + restart, nil
		} else if !errors.Is(err, os.ErrPermission) {
			return "", fmt.Errorf("could not replace %s: %w", target, err)
		}
		script := privilegedInstallScript(bin, "rfswift-workbench", binDigest, "install -m 0755 \"$f\" "+hostsetup.ShellQuote(target))
		if out, err := hostsetup.RunPrivilegedCommand("/bin/sh", "-c", script); err != nil {
			return "", fmt.Errorf("installing as root failed: %v %s", err, strings.TrimSpace(out))
		}
		return "The Workbench binary was updated. " + restart, nil
	case "deb", "rpm", "pacman":
		script := privilegedInstallScript(path, name, hexDigest, packageInstallCommand(method))
		if out, err := hostsetup.RunPrivilegedCommand("/bin/sh", "-c", script); err != nil {
			return "", fmt.Errorf("the package manager did not install the update: %v %s", err, strings.TrimSpace(out))
		}
		return "The " + method + " package was installed. " + restart, nil
	case "macos-dmg":
		if err := exec.Command("open", path).Run(); err != nil {
			return "", fmt.Errorf("could not open the disk image: %w", err)
		}
		return "The verified disk image is open: drag RF Swift Workbench to Applications, then restart it.", nil
	case "windows-msi":
		if err := exec.Command("msiexec", "/i", path).Start(); err != nil {
			return "", fmt.Errorf("could not start the installer: %w", err)
		}
		return "The verified installer has started. Follow it, then restart the Workbench.", nil
	}
	return "", errors.New("this Workbench cannot update itself")
}

func packageInstallCommand(method string) string {
	switch method {
	case "deb":
		return `apt-get install -y "$f"`
	case "rpm":
		// The packages are not GPG-signed; their provenance was verified instead.
		return `if command -v dnf >/dev/null 2>&1; then dnf install -y "$f"; ` +
			`elif command -v zypper >/dev/null 2>&1; then zypper --non-interactive install --allow-unsigned-rpm "$f"; ` +
			`elif command -v yum >/dev/null 2>&1; then yum install -y "$f"; else rpm -Uvh "$f"; fi`
	case "pacman":
		return `pacman -U --noconfirm "$f"`
	}
	return "false"
}

// privilegedInstallScript is the root side of an install: copy the verified
// file into a fresh directory only root can write, check its digest there,
// then run the install command on that copy ($f).
func privilegedInstallScript(src, name, hexDigest, installCmd string) string {
	return "set -eu\numask 077\n" +
		"d=$(mktemp -d /var/tmp/rfswift-workbench-update.XXXXXX)\n" +
		"trap 'rm -rf \"$d\"' EXIT\n" +
		"f=\"$d\"/" + hostsetup.ShellQuote(name) + "\n" +
		"cp -- " + hostsetup.ShellQuote(src) + " \"$f\"\n" +
		"printf '%s  %s\\n' " + hostsetup.ShellQuote(hexDigest) + " \"$f\" | sha256sum -c --status - || " +
		"{ echo 'The update changed after it was verified; nothing was installed.' >&2; exit 1; }\n" +
		installCmd + "\n"
}

// replaceFile atomically replaces target with a copy of src.
func replaceFile(src, target string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp, err := os.CreateTemp(filepath.Dir(target), "."+filepath.Base(target)+".update-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op once renamed
	if _, err := io.Copy(tmp, in); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), target)
}

// extractWorkbenchBinary extracts the rfswift-workbench binary from a verified
// release tarball and returns its path and the SHA-256 of the bytes written,
// which the root install path checks again.
func extractWorkbenchBinary(archive, dir string) (string, string, error) {
	f, err := os.Open(archive)
	if err != nil {
		return "", "", err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", "", err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return "", "", errors.New("the release archive has no rfswift-workbench binary")
		}
		if err != nil {
			return "", "", err
		}
		if hdr.Typeflag != tar.TypeReg || strings.TrimPrefix(hdr.Name, "./") != "rfswift-workbench" {
			continue
		}
		if hdr.Size <= 0 || hdr.Size > maxUpdateSize {
			return "", "", errors.New("unexpected binary size in the release archive")
		}
		out := filepath.Join(dir, "rfswift-workbench")
		w, err := os.OpenFile(out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o700)
		if err != nil {
			return "", "", err
		}
		var h hash.Hash = sha256.New()
		_, copyErr := io.Copy(io.MultiWriter(w, h), io.LimitReader(tr, hdr.Size))
		if closeErr := w.Close(); copyErr == nil {
			copyErr = closeErr
		}
		if copyErr != nil {
			return "", "", copyErr
		}
		return out, hex.EncodeToString(h.Sum(nil)), nil
	}
}
