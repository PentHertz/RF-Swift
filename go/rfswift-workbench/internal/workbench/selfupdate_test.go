package workbench

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/sigstore/sigstore-go/pkg/root"
	common "penthertz/rfswift/common"
)

// Real v4.0.2 release data: digests published by GitHub for the assets whose
// attestation bundles are stored in testdata/selfupdate.
const (
	appImageDigest = "cdd4ba40138573a874f659e7d51c1719edbaa3d0b57048d100b0b39773c07a29"
	dmgDigest      = "80ba667078fa0ea5a11bc44b8ac870b4290657afe7319ba75797f0e7fc3348b1"
)

func fixtureTrust(t *testing.T) root.TrustedMaterial {
	t.Helper()
	tr, err := root.NewTrustedRootFromPath(filepath.Join("testdata", "selfupdate", "trusted_root.json"))
	if err != nil {
		t.Fatal(err)
	}
	return tr
}

func fixtureBundle(t *testing.T, name string) [][]byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "selfupdate", name))
	if err != nil {
		t.Fatal(err)
	}
	return [][]byte{b}
}

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestVerifyProvenanceAcceptsTheReleaseWorkflow(t *testing.T) {
	tm := fixtureTrust(t)
	if err := verifyProvenance(tm, fixtureBundle(t, "appimage-x86_64-v4.0.2.bundle.json"), mustHex(t, appImageDigest), "rfswift-workbench_Linux_x86_64.AppImage", "v4.0.2"); err != nil {
		t.Fatalf("AppImage: %v", err)
	}
	if err := verifyProvenance(tm, fixtureBundle(t, "dmg-v4.0.2.bundle.json"), mustHex(t, dmgDigest), "rfswift_Darwin_universal.dmg", "v4.0.2"); err != nil {
		t.Fatalf("DMG: %v", err)
	}
}

func TestVerifyProvenanceRejects(t *testing.T) {
	tm := fixtureTrust(t)
	tampered := mustHex(t, appImageDigest)
	tampered[0] ^= 1
	cases := []struct {
		name, bundle, digest, asset, tag string
		digestBytes                      []byte
	}{
		{name: "another tag", bundle: "appimage-x86_64-v4.0.2.bundle.json", digest: appImageDigest, asset: "rfswift-workbench_Linux_x86_64.AppImage", tag: "v4.0.3"},
		{name: "a file the bundle does not cover", bundle: "appimage-x86_64-v4.0.2.bundle.json", digestBytes: tampered, asset: "rfswift-workbench_Linux_x86_64.AppImage", tag: "v4.0.2"},
		{name: "the DMG workflow's attestation for a Linux asset", bundle: "dmg-v4.0.2.bundle.json", digest: dmgDigest, asset: "rfswift-workbench_Linux_x86_64.AppImage", tag: "v4.0.2"},
		{name: "the release workflow's attestation for the DMG", bundle: "appimage-x86_64-v4.0.2.bundle.json", digest: appImageDigest, asset: "rfswift_Darwin_universal.dmg", tag: "v4.0.2"},
	}
	for _, c := range cases {
		d := c.digestBytes
		if d == nil {
			d = mustHex(t, c.digest)
		}
		if err := verifyProvenance(tm, fixtureBundle(t, c.bundle), d, c.asset, c.tag); err == nil {
			t.Errorf("%s: accepted", c.name)
		}
	}
	if err := verifyProvenance(tm, [][]byte{[]byte(`{"mediaType":"nope"}`)}, mustHex(t, appImageDigest), "rfswift-workbench_Linux_x86_64.AppImage", "v4.0.2"); err == nil {
		t.Error("a malformed bundle was accepted")
	}
}

func TestUpdateAssetNamesMatchTheRelease(t *testing.T) {
	published := map[string]bool{}
	for _, n := range []string{
		"rfswift-workbench_Linux_x86_64.AppImage", "rfswift-workbench_Linux_arm64.AppImage",
		"rfswift-workbench_Linux_x86_64.tar.gz", "rfswift-workbench_Linux_arm64.tar.gz",
		"rfswift-workbench_4.0.2_amd64.deb", "rfswift-workbench_4.0.2_arm64.deb",
		"rfswift-workbench-4.0.2-1.x86_64.rpm", "rfswift-workbench-4.0.2-1.aarch64.rpm",
		"rfswift-workbench-4.0.2-1-x86_64.pkg.tar.zst", "rfswift-workbench-4.0.2-1-aarch64.pkg.tar.zst",
		"rfswift_Darwin_universal.dmg", "RFSwift-4.0.2-x64.msi", "RFSwift-4.0.2-arm64.msi",
	} {
		published[n] = true
	}
	for _, method := range []string{"appimage", "binary", "deb", "rpm", "pacman", "macos-dmg", "windows-msi"} {
		for _, arch := range []string{"amd64", "arm64"} {
			name, err := updateAssetName(method, "4.0.2", arch)
			if err != nil || !published[name] {
				t.Errorf("%s/%s: %q (%v) is not a v4.0.2 asset", method, arch, name, err)
			}
		}
	}
	if _, err := updateAssetName("", "4.0.2", "amd64"); err == nil {
		t.Error("an unknown install method got an asset")
	}
	if _, err := updateAssetName("deb", "4.0.2", "riscv64"); err == nil {
		t.Error("an unbuilt architecture got an asset")
	}
}

// githubRedirect sends every request to the test server, keeping the path.
type githubRedirect struct{ base *url.URL }

func (g githubRedirect) RoundTrip(r *http.Request) (*http.Response, error) {
	r2 := r.Clone(r.Context())
	r2.URL.Scheme, r2.URL.Host = g.base.Scheme, g.base.Host
	return http.DefaultTransport.RoundTrip(r2)
}

func testRelease(t *testing.T, payload []byte) (*httptest.Server, ghAsset) {
	t.Helper()
	name := "rfswift-workbench_Linux_x86_64.AppImage"
	sum := sha256.Sum256(payload)
	asset := ghAsset{
		Name:   name,
		Size:   int64(len(payload)),
		Digest: "sha256:" + hex.EncodeToString(sum[:]),
		URL:    "https://github.com/PentHertz/RF-Swift/releases/download/v9.9.9/" + name,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/PentHertz/RF-Swift/releases/download/v9.9.9/" + name:
			_, _ = w.Write(payload)
		case "/repos/PentHertz/RF-Swift/releases/latest":
			_ = json.NewEncoder(w).Encode(ghRelease{TagName: "v9.9.9", HTMLURL: releasesPage + "/tag/v9.9.9", Assets: []ghAsset{asset}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	old := updateTransport
	updateTransport = githubRedirect{base: u}
	t.Cleanup(func() { updateTransport = old })
	return srv, asset
}

func TestDownloadAssetChecks(t *testing.T) {
	payload := bytes.Repeat([]byte("appimage"), 4096)
	_, asset := testRelease(t, payload)
	hc := updateHTTPClient(10 * time.Second)
	ctx := context.Background()

	path, sum, err := downloadAsset(ctx, hc, "v9.9.9", asset, t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); !bytes.Equal(got, payload) {
		t.Fatal("downloaded content differs")
	}
	if want := sha256.Sum256(payload); !bytes.Equal(sum, want[:]) {
		t.Fatal("wrong digest returned")
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
		t.Fatalf("download is readable by others: %v", info.Mode())
	}

	bad := asset
	bad.Digest = "sha256:" + strings.Repeat("0", 64)
	if _, _, err := downloadAsset(ctx, hc, "v9.9.9", bad, t.TempDir(), nil); err == nil {
		t.Error("a digest mismatch was accepted")
	}
	bad = asset
	bad.Size++
	if _, _, err := downloadAsset(ctx, hc, "v9.9.9", bad, t.TempDir(), nil); err == nil {
		t.Error("a size mismatch was accepted")
	}
	bad = asset
	bad.URL = "https://example.org/rfswift-workbench_Linux_x86_64.AppImage"
	if _, _, err := downloadAsset(ctx, hc, "v9.9.9", bad, t.TempDir(), nil); err == nil {
		t.Error("a download outside the release was accepted")
	}
	bad = asset
	bad.Name = "../evil"
	if _, _, err := downloadAsset(ctx, hc, "v9.9.9", bad, t.TempDir(), nil); err == nil {
		t.Error("a path in the asset name was accepted")
	}
}

func TestWorkbenchUpdateStatus(t *testing.T) {
	testRelease(t, []byte("x"))
	oldVersion := common.Version
	t.Cleanup(func() { common.Version = oldVersion })
	a := &App{}
	a.update.once.Do(func() { a.update.method, a.update.target = "appimage", "/tmp/rfswift.AppImage" })

	common.Version = "4.0.2"
	st := a.WorkbenchUpdateStatus(true)
	if !st.UpdateAvailable || st.Latest != "9.9.9" {
		t.Fatalf("expected an available update, got %+v", st)
	}
	if st.CanInstall != (updateArchSupported()) {
		t.Fatalf("CanInstall=%v on %s: %+v", st.CanInstall, runtimeArch(), st)
	}

	common.Version = "9.9.9"
	if st := a.WorkbenchUpdateStatus(true); st.UpdateAvailable || st.CanInstall {
		t.Fatalf("same version reported as an update: %+v", st)
	}
	common.Version = "10.0.0"
	if st := a.WorkbenchUpdateStatus(false); st.UpdateAvailable {
		t.Fatalf("development build reported as outdated: %+v", st)
	}
}

// TestPrivilegedInstallScript runs the root-side script as the current user,
// with an install command that copies $f out, to check the digest guard.
func TestPrivilegedInstallScript(t *testing.T) {
	if _, err := exec.LookPath("sha256sum"); err != nil {
		t.Skip("sha256sum not available")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "pkg.deb")
	if err := os.WriteFile(src, []byte("verified package"), 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte("verified package"))
	out := filepath.Join(dir, "installed")
	install := `cp "$f" ` + out

	script := privilegedInstallScript(src, "pkg.deb", hex.EncodeToString(sum[:]), install)
	if b, err := exec.Command("/bin/sh", "-c", script).CombinedOutput(); err != nil {
		t.Fatalf("valid file refused: %v %s", err, b)
	}
	if got, _ := os.ReadFile(out); string(got) != "verified package" {
		t.Fatal("install command did not see the verified copy")
	}

	_ = os.Remove(out)
	if err := os.WriteFile(src, []byte("swapped after verification"), 0o600); err != nil {
		t.Fatal(err)
	}
	b, err := exec.Command("/bin/sh", "-c", script).CombinedOutput()
	if err == nil || !strings.Contains(string(b), "changed after it was verified") {
		t.Fatalf("swapped file not refused: %v %s", err, b)
	}
	if _, err := os.Stat(out); err == nil {
		t.Fatal("swapped file was installed")
	}
}

func writeTarGz(t *testing.T, path string, files map[string]string, symlink string) {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if symlink != "" {
		_ = tw.WriteHeader(&tar.Header{Name: symlink, Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd"})
	}
	for name, body := range files {
		_ = tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg})
		_, _ = tw.Write([]byte(body))
	}
	_ = tw.Close()
	_ = gz.Close()
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestExtractWorkbenchBinary(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "wb.tar.gz")
	writeTarGz(t, archive, map[string]string{"./README.md": "readme", "./rfswift-workbench": "ELF binary"}, "")
	out := t.TempDir()
	bin, digest, err := extractWorkbenchBinary(archive, out)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(bin); string(got) != "ELF binary" {
		t.Fatal("wrong file extracted")
	}
	if sum := sha256.Sum256([]byte("ELF binary")); digest != hex.EncodeToString(sum[:]) {
		t.Fatal("digest does not cover the extracted bytes")
	}

	writeTarGz(t, archive, map[string]string{"./README.md": "readme"}, "./rfswift-workbench")
	if _, _, err := extractWorkbenchBinary(archive, t.TempDir()); err == nil {
		t.Fatal("a symlink named rfswift-workbench was extracted")
	}
}

func TestReplaceFile(t *testing.T) {
	dir := t.TempDir()
	src, target := filepath.Join(dir, "new"), filepath.Join(dir, "app.AppImage")
	_ = os.WriteFile(src, []byte("new"), 0o600)
	_ = os.WriteFile(target, []byte("old"), 0o644)
	if err := replaceFile(src, target, 0o755); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(target)
	if got, _ := os.ReadFile(target); string(got) != "new" || info.Mode().Perm() != 0o755 {
		t.Fatalf("target not replaced: %q %v", got, info.Mode())
	}
	if left, _ := filepath.Glob(filepath.Join(dir, ".app.AppImage.update-*")); len(left) != 0 {
		t.Fatalf("temporary file left behind: %v", left)
	}
}

func updateArchSupported() bool { return runtime.GOARCH == "amd64" || runtime.GOARCH == "arm64" }

func runtimeArch() string { return runtime.GOARCH }
