/* This code is part of RF Swift by @Penthertz
*  Tests for `rfswift update`: release file names, checksum parsing, install
*  detection, the package-manager command, archive extraction and the
*  in-place binary swap.
 */

package rfutils

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// The names below are the assets of the v4.0.5 release: a drift between
// these templates and the release naming would send `rfswift update` to a 404.
func TestPackageAssetNameMatchesReleaseAssets(t *testing.T) {
	cases := []struct {
		m      installMethod
		goarch string
		want   string
		ok     bool
	}{
		{installDeb, "amd64", "rfswift_4.0.5_amd64.deb", true},
		{installDeb, "arm64", "rfswift_4.0.5_arm64.deb", true},
		{installDeb, "riscv64", "rfswift_4.0.5_riscv64.deb", true},
		{installRPM, "amd64", "rfswift-4.0.5-1.x86_64.rpm", true},
		{installRPM, "arm64", "rfswift-4.0.5-1.aarch64.rpm", true},
		{installRPM, "riscv64", "rfswift-4.0.5-1.riscv64.rpm", true},
		{installPacman, "amd64", "rfswift-4.0.5-1-x86_64.pkg.tar.zst", true},
		{installPacman, "arm64", "rfswift-4.0.5-1-aarch64.pkg.tar.zst", true},
		{installPacman, "riscv64", "", false}, // no Arch package for riscv64
		{installDeb, "386", "", false},
		{installStandalone, "amd64", "", false},
	}
	for _, c := range cases {
		got, ok := packageAssetName(c.m, "4.0.5", c.goarch)
		if got != c.want || ok != c.ok {
			t.Errorf("packageAssetName(%v, %s) = %q, %v; want %q, %v", c.m, c.goarch, got, ok, c.want, c.ok)
		}
	}
}

func TestArchiveAssetName(t *testing.T) {
	cases := []struct {
		goos, goarch, want string
		ok                 bool
	}{
		{"linux", "amd64", "rfswift_Linux_x86_64.tar.gz", true},
		{"linux", "arm64", "rfswift_Linux_arm64.tar.gz", true},
		{"linux", "riscv64", "rfswift_Linux_riscv64.tar.gz", true},
		{"darwin", "amd64", "rfswift_Darwin_x86_64.tar.gz", true},
		{"darwin", "arm64", "rfswift_Darwin_arm64.tar.gz", true},
		{"windows", "amd64", "rfswift_Windows_x86_64.zip", true},
		{"windows", "arm64", "rfswift_Windows_arm64.zip", true},
		{"darwin", "riscv64", "", false},
		{"freebsd", "amd64", "", false},
		{"linux", "386", "", false},
	}
	for _, c := range cases {
		got, ok := archiveAssetName(c.goos, c.goarch)
		if got != c.want || ok != c.ok {
			t.Errorf("archiveAssetName(%s, %s) = %q, %v; want %q, %v", c.goos, c.goarch, got, ok, c.want, c.ok)
		}
	}
}

func TestWindowsSetupAssetName(t *testing.T) {
	if got, _ := windowsSetupAssetName("4.0.5", "amd64"); got != "RFSwift-Setup-4.0.5-x64.exe" {
		t.Errorf("amd64: %q", got)
	}
	if got, _ := windowsSetupAssetName("4.0.5", "arm64"); got != "RFSwift-Setup-4.0.5-arm64.exe" {
		t.Errorf("arm64: %q", got)
	}
	if _, ok := windowsSetupAssetName("4.0.5", "riscv64"); ok {
		t.Error("riscv64 has no Windows installer")
	}
}

func TestParseChecksums(t *testing.T) {
	in := strings.Join([]string{
		"0b68ac4c566daeb1f62c1e4f87a9c41df0cbe2b52d14582b672ceabefa02d269  rfswift_4.0.5_amd64.deb",
		"1FD2474516A9B63725FF7407044A7A7F28F56A8B0296A368C6564A3672450959 *rfswift_Linux_x86_64.tar.gz",
		"",
		"not-a-digest  something.txt",
		"zz68ac4c566daeb1f62c1e4f87a9c41df0cbe2b52d14582b672ceabefa02d269  bad-hex.deb",
		"0b68ac4c  too-short.deb",
	}, "\n")
	got, err := parseChecksums(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"rfswift_4.0.5_amd64.deb":     "0b68ac4c566daeb1f62c1e4f87a9c41df0cbe2b52d14582b672ceabefa02d269",
		"rfswift_Linux_x86_64.tar.gz": "1fd2474516a9b63725ff7407044a7a7f28f56a8b0296a368c6564a3672450959",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseChecksums = %v; want %v", got, want)
	}
}

func TestDownloadVerifiedRefusesUnlistedAsset(t *testing.T) {
	_, err := downloadVerified("v9.9.9", "rfswift_9.9.9_amd64.deb", t.TempDir(), map[string]string{})
	if err == nil || !strings.Contains(err.Error(), "cannot be verified") {
		t.Fatalf("an asset missing from the checksums must be refused before any download, got %v", err)
	}
}

func TestDetectInstallMethod(t *testing.T) {
	saved := ownedByPackage
	defer func() { ownedByPackage = saved }()

	owner := ""
	ownedByPackage = func(tool string, args ...string) bool { return tool == owner }
	for tool, want := range map[string]installMethod{
		"dpkg": installDeb, "rpm": installRPM, "pacman": installPacman, "": installStandalone,
	} {
		owner = tool
		if got := detectInstallMethod("/usr/bin/rfswift", "linux"); got != want {
			t.Errorf("owned by %q: got %v, want %v", tool, got, want)
		}
	}

	owner = ""
	if got := detectInstallMethod("/opt/homebrew/Caskroom/rfswift/4.0.5/rfswift", "darwin"); got != installHomebrew {
		t.Errorf("Caskroom binary: got %v", got)
	}
	if got := detectInstallMethod("/usr/local/bin/rfswift", "darwin"); got != installStandalone {
		t.Errorf("/usr/local/bin on macOS: got %v", got)
	}

	t.Setenv("ProgramFiles", `C:\Program Files`)
	t.Setenv("ProgramW6432", "")
	exe := filepath.Join(`C:\Program Files`, "RF Swift", "rfswift.exe")
	if got := detectInstallMethod(exe, "windows"); got != installWindowsSetup {
		t.Errorf("Program Files install: got %v", got)
	}
	if got := detectInstallMethod(filepath.Join("C:", "tools", "rfswift.exe"), "windows"); got != installStandalone {
		t.Errorf("standalone Windows copy: got %v", got)
	}
}

func TestPackageInstallArgs(t *testing.T) {
	look := func(have ...string) func(string) (string, error) {
		return func(tool string) (string, error) {
			for _, h := range have {
				if h == tool {
					return "/usr/bin/" + tool, nil
				}
			}
			return "", errors.New("not found")
		}
	}
	cases := []struct {
		m    installMethod
		have []string
		want []string
	}{
		{installDeb, []string{"apt-get"}, []string{"apt-get", "install", "-y", "/t/p.deb"}},
		{installDeb, nil, []string{"dpkg", "-i", "/t/p.deb"}},
		{installRPM, []string{"dnf", "yum"}, []string{"dnf", "install", "-y", "/t/p.deb"}},
		{installRPM, []string{"yum"}, []string{"yum", "install", "-y", "/t/p.deb"}},
		{installRPM, []string{"zypper"}, []string{"zypper", "--non-interactive", "install", "--allow-unsigned-rpm", "/t/p.deb"}},
		{installRPM, nil, []string{"rpm", "-U", "/t/p.deb"}},
		{installPacman, nil, []string{"pacman", "-U", "--noconfirm", "/t/p.deb"}},
		{installStandalone, nil, nil},
	}
	for _, c := range cases {
		if got := packageInstallArgs(c.m, "/t/p.deb", look(c.have...)); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%v with %v: got %v, want %v", c.m, c.have, got, c.want)
		}
	}
}

// archiveEntries is what the release archives hold, plus a traversal entry
// that must never be picked or written.
var archiveEntries = []struct{ name, body string }{
	{"README.md", "readme"},
	{"../rfswift", "evil"},
	{"lima/rfswift.yaml", "lima"},
	{"rfswift", "new-binary"},
}

func TestExtractBinaryTarGz(t *testing.T) {
	dir := t.TempDir()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range archiveEntries {
		tw.WriteHeader(&tar.Header{Name: e.name, Mode: 0o755, Size: int64(len(e.body)), Typeflag: tar.TypeReg})
		tw.Write([]byte(e.body))
	}
	tw.Close()
	gz.Close()
	archive := filepath.Join(dir, "rfswift_Linux_x86_64.tar.gz")
	os.WriteFile(archive, buf.Bytes(), 0o644)

	dst := filepath.Join(dir, "out", "rfswift")
	os.Mkdir(filepath.Dir(dst), 0o755)
	if err := extractBinary(archive, "rfswift", dst); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(dst); string(got) != "new-binary" {
		t.Errorf("extracted %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "rfswift")); !os.IsNotExist(err) {
		t.Error("the ../rfswift entry was written outside the destination")
	}
	if err := extractBinary(archive, "rfswift.exe", filepath.Join(dir, "none")); err == nil {
		t.Error("a missing entry must be an error")
	}
}

func TestExtractBinaryZip(t *testing.T) {
	dir := t.TempDir()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range archiveEntries {
		name := e.name
		if name == "rfswift" {
			name = "rfswift.exe"
		}
		w, _ := zw.Create(name)
		w.Write([]byte(e.body))
	}
	zw.Close()
	archive := filepath.Join(dir, "rfswift_Windows_x86_64.zip")
	os.WriteFile(archive, buf.Bytes(), 0o644)

	dst := filepath.Join(dir, "rfswift.exe")
	if err := extractBinary(archive, "rfswift.exe", dst); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(dst); string(got) != "new-binary" {
		t.Errorf("extracted %q", got)
	}
}

func TestReplaceExecutable(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "bin", "rfswift")
	os.Mkdir(filepath.Dir(exe), 0o755)
	os.WriteFile(exe, []byte("old"), 0o755)
	newBin := filepath.Join(dir, "new")
	os.WriteFile(newBin, []byte("new"), 0o644)

	if err := replaceExecutable(newBin, exe); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(exe); string(got) != "new" {
		t.Errorf("exe holds %q", got)
	}
	if runtime.GOOS != "windows" {
		if fi, _ := os.Stat(exe); fi.Mode().Perm() != 0o755 {
			t.Errorf("mode %v, want 0755", fi.Mode().Perm())
		}
	}
	left, _ := filepath.Glob(filepath.Join(filepath.Dir(exe), ".rfswift-update-*"))
	if len(left) != 0 {
		t.Errorf("temporary files left behind: %v", left)
	}
}

func TestPlanUpdateStandaloneWritable(t *testing.T) {
	if _, ok := archiveAssetName(runtime.GOOS, runtime.GOARCH); !ok {
		t.Skip("no release archive for this platform")
	}
	exe := filepath.Join(t.TempDir(), "rfswift")
	p := planUpdate(exe, installStandalone, "4.0.5", "v4.0.5")
	if p.manualWhy != "" || p.sudo {
		t.Fatalf("a writable standalone install updates in place without sudo: %+v", p)
	}
	if len(p.steps) != 2 || !strings.Contains(p.steps[1], exe) {
		t.Errorf("steps %v", p.steps)
	}
}

func TestPlanUpdateWindowsSetupPointsAtInstaller(t *testing.T) {
	p := planUpdate(`C:\Program Files\RF Swift\rfswift.exe`, installWindowsSetup, "4.0.5", "v4.0.5")
	if p.manualWhy == "" || len(p.manual) != 1 {
		t.Fatalf("the Windows installer's binary is never replaced in place: %+v", p)
	}
	if !strings.Contains(p.manual[0], "/releases/") {
		t.Errorf("manual step %q", p.manual[0])
	}
}
