package nix

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The embedded catalog must always parse and contain the flagship environments.
func TestEmbeddedCatalogParses(t *testing.T) {
	cat, err := parseCatalog(embeddedCatalog)
	if err != nil {
		t.Fatalf("embedded catalog does not parse: %v", err)
	}
	if cat.Version == "" {
		t.Error("catalog version is empty")
	}
	for _, want := range []string{"sdr_light", "rfid", "wifi", "bluetooth"} {
		if cat.Find(want) == nil {
			t.Errorf("embedded catalog is missing environment %q", want)
		}
	}
	sdr := cat.Find("sdr_light")
	if sdr == nil || len(sdr.Packages) == 0 {
		t.Fatal("sdr_light has no packages")
	}
}

func TestCatalogFindCaseInsensitive(t *testing.T) {
	cat, err := parseCatalog(embeddedCatalog)
	if err != nil {
		t.Fatal(err)
	}
	if cat.Find("SDR_LIGHT") == nil {
		t.Error("Find should be case-insensitive")
	}
	if cat.Find("  rfid  ") == nil {
		t.Error("Find should trim whitespace")
	}
	if cat.Find("does-not-exist") != nil {
		t.Error("Find should return nil for unknown names")
	}
}

func TestParseCatalogDefaultsFlake(t *testing.T) {
	c, err := parseCatalog([]byte(`{"version":"1","environments":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if c.Flake != DefaultFlakeRef {
		t.Errorf("empty flake should default to %q, got %q", DefaultFlakeRef, c.Flake)
	}
}

func TestResolveFlakeRefPriority(t *testing.T) {
	t.Setenv("RFSWIFT_NIX_FLAKE", "github:someone/other")

	// Explicit override wins over everything.
	if got := ResolveFlakeRef("path:/tmp/x"); got != "path:/tmp/x" {
		t.Errorf("override should win, got %q", got)
	}
	// Otherwise the env var wins over the default.
	if got := ResolveFlakeRef(""); got != "github:someone/other" {
		t.Errorf("env var should be used, got %q", got)
	}
}

func TestResolveWorkspace(t *testing.T) {
	if got := resolveWorkspace("env1", "none"); got != "" {
		t.Errorf(`"none" should disable the workspace, got %q`, got)
	}
	def := resolveWorkspace("env1", "")
	if filepath.Base(def) != "env1" {
		t.Errorf("default workspace should end in the env name, got %q", def)
	}
	if got := resolveWorkspace("env1", "/tmp/custom"); got != "/tmp/custom" {
		t.Errorf("custom workspace should be honored, got %q", got)
	}
}

func TestBaseDirOverride(t *testing.T) {
	t.Setenv("RFSWIFT_NIX_HOME", "/tmp/rfnix-test")
	if BaseDir() != "/tmp/rfnix-test" {
		t.Errorf("RFSWIFT_NIX_HOME should override BaseDir, got %q", BaseDir())
	}
	if EnvDir("foo") != filepath.Join("/tmp/rfnix-test", "environments", "foo") {
		t.Errorf("EnvDir layout wrong, got %q", EnvDir("foo"))
	}
}

func TestManifestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RFSWIFT_NIX_HOME", dir)

	env := &Environment{Name: "rt", Image: "sdr_light", FlakeRef: "path:/x", Packages: []string{"gnuradio"}}
	if err := writeManifest(env); err != nil {
		t.Fatalf("writeManifest: %v", err)
	}
	if _, err := os.Stat(manifestPath("rt")); err != nil {
		t.Fatalf("manifest not written: %v", err)
	}
	got, err := GetEnvironment("rt")
	if err != nil {
		t.Fatalf("GetEnvironment: %v", err)
	}
	if got.Image != "sdr_light" || len(got.Packages) != 1 || got.Packages[0] != "gnuradio" {
		t.Errorf("round-trip mismatch: %+v", got)
	}
}

func TestNotFoundErrorsAreRecognisable(t *testing.T) {
	t.Setenv("RFSWIFT_NIX_HOME", t.TempDir())
	_, err := GetEnvironment("nope")
	if !IsNotFound(err) {
		t.Fatalf("GetEnvironment on a missing environment must be a NotFoundError, got %v", err)
	}
	if !strings.Contains(err.Error(), "rfswift nix list") {
		t.Fatalf("the CLI hint must survive: %v", err)
	}
	if err := RemoveEnvironment("nope"); !IsNotFound(err) {
		t.Fatalf("RemoveEnvironment on a missing environment must be a NotFoundError, got %v", err)
	}
	if IsNotFound(os.ErrPermission) {
		t.Fatal("other errors are not NotFound")
	}
}

func TestValidateEnvironmentNameRejectsUnsafeNames(t *testing.T) {
	// Names flow unquoted into the sourced bashrc and into filesystem paths.
	for _, ok := range []string{"rt", "sdr_light", "lab-1", "env.2"} {
		if err := ValidateEnvironmentName(ok); err != nil {
			t.Errorf("valid name %q rejected: %v", ok, err)
		}
	}
	for _, bad := range []string{
		"", ".", "..", "../evil", "a/b", "a b",
		"pwn$(id)", "x`reboot`", "a;rm -rf ~", "a\nrm", "-flag", ".hidden",
	} {
		if err := ValidateEnvironmentName(bad); err == nil {
			t.Errorf("unsafe name %q was accepted", bad)
		}
	}
}

func TestGetEnvironmentRejectsUnsafeManifestName(t *testing.T) {
	// An imported/hand-crafted manifest must not smuggle a shell-injecting
	// name past the loader (the name is later sourced into bash on entry).
	dir := t.TempDir()
	t.Setenv("RFSWIFT_NIX_HOME", dir)
	env := &Environment{Name: "rt", Image: "sdr_light", FlakeRef: "path:/x"}
	if err := writeManifest(env); err != nil {
		t.Fatalf("writeManifest: %v", err)
	}
	// Rewrite the on-disk manifest with a malicious Name, as an attacker
	// shipping an environment directory would.
	evil := []byte(`{"name":"pwn$(touch /tmp/pwned)","image":"sdr_light","flakeRef":"path:/x"}`)
	if err := os.WriteFile(manifestPath("rt"), evil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := GetEnvironment("rt"); err == nil {
		t.Fatal("GetEnvironment accepted a manifest with an unsafe name")
	}
}

func TestWriteShimsSkipsNewlineInjectingAttr(t *testing.T) {
	t.Setenv("RFSWIFT_NIX_HOME", t.TempDir())
	t.Setenv("RFSWIFT_NIX_BIN", "/opt/nix/bin/nix")
	env := &Environment{
		Name:     "lazy",
		Image:    "sdr_light",
		FlakeRef: "path:/rf-swift-nix",
		Commands: map[string]string{"evil": "attr\necho pwned"},
	}
	if err := writeShims(env); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(shimsDir(env.Name), "evil")); err == nil {
		if strings.Contains(string(data), "echo pwned") {
			t.Fatalf("shim contains injected line:\n%s", data)
		}
	}
}

func TestWriteShimsUsesResolvedMainProgram(t *testing.T) {
	t.Setenv("RFSWIFT_NIX_HOME", t.TempDir())
	t.Setenv("RFSWIFT_NIX_BIN", "/opt/nix/bin/nix")
	env := &Environment{
		Name:          "lazy",
		Image:         "sdr_light",
		FlakeRef:      "path:/rf-swift-nix",
		Prerequisites: []string{"soapysdr-with-plugins"},
		Commands:      map[string]string{"gnuradio-companion": "gnuradio-rfswift-light"},
	}
	if err := writeShims(env); err != nil {
		t.Fatal(err)
	}
	shim := filepath.Join(shimsDir(env.Name), "gnuradio-companion")
	data, err := os.ReadFile(shim)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{
		"/opt/nix/bin/nix",
		"path:/rf-swift-nix#sdr_light-prerequisites",
		"path:/rf-swift-nix#gnuradio-rfswift-light",
		`-- "$@"`,
		shimFormatMarker,
		// Built once into a pinned out-link (a gcroot), then exec'd from there.
		`build --out-link "$link"`,
		filepath.Join("tools", "gnuradio-rfswift-light"),
		`exec "$link/bin/gnuradio-companion" "$@"`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("shim does not contain %q:\n%s", want, text)
		}
	}
	if !shimsCurrent(env) {
		t.Error("freshly written shims must be current")
	}
	if _, err := os.Stat(toolsDir(env.Name)); err != nil {
		t.Errorf("tools dir not created: %v", err)
	}
	info, err := os.Stat(shim)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Error("shim is not executable")
	}
}

func TestLazyHandlerCoversSecondaryCommands(t *testing.T) {
	t.Setenv("RFSWIFT_NIX_BIN", "nix-test")
	env := &Environment{
		Name:          "sdr",
		Image:         "sdr_light",
		FlakeRef:      "path:/rf-swift-nix",
		Packages:      []string{"gnuradio-rfswift-light", "rtl-sdr-osmocom"},
		Prerequisites: []string{"rtl-sdr-osmocom"},
	}
	handler := lazyHandler(env)
	for _, want := range []string{
		`"gnuradio-rfswift-light"`,
		`"rtl-sdr-osmocom"`,
		`path:/rf-swift-nix#sdr_light-prerequisites`,
		`command_not_found_handle()`,
		`[ -x "$out/bin/$cmd" ]`,
		`nix-test`,
		// What it builds is pinned under the environment like a shim's tool.
		`build --out-link "$link"`,
		`"$link/bin/$cmd" "$@"`,
	} {
		if !strings.Contains(handler, want) {
			t.Errorf("lazy handler does not contain %q", want)
		}
	}
}

func TestShimsCurrentDetectsOlderFormat(t *testing.T) {
	t.Setenv("RFSWIFT_NIX_HOME", t.TempDir())
	t.Setenv("RFSWIFT_NIX_BIN", "/opt/nix/bin/nix")
	env := &Environment{Name: "lazy", Image: "sdr_light", FlakeRef: "path:/rf-swift-nix", Commands: map[string]string{"gqrx": "gqrx"}}
	if shimsCurrent(env) {
		t.Fatal("no shims yet: not current")
	}
	if err := writeShims(env); err != nil {
		t.Fatal(err)
	}
	if !shimsCurrent(env) {
		t.Fatal("just written shims are current")
	}
	// A shim written by RF Swift 4.0.0 (format 1: nix run on every call).
	legacy := "#!/bin/sh\n# RF Swift lazy tool shim: builds gqrx on first call, then runs it.\nexec nix run \"path:/rf-swift-nix#gqrx\" -- \"$@\"\n"
	if err := os.WriteFile(filepath.Join(shimsDir(env.Name), "gqrx"), []byte(legacy), 0o755); err != nil {
		t.Fatal(err)
	}
	if shimsCurrent(env) {
		t.Fatal("a format-1 shim must be reported outdated so ensureShims regenerates it")
	}
}

func TestWriteShimsRejectsUnsafeNames(t *testing.T) {
	t.Setenv("RFSWIFT_NIX_HOME", t.TempDir())
	t.Setenv("RFSWIFT_NIX_BIN", "/opt/nix/bin/nix")
	env := &Environment{Name: "lazy", Image: "sdr_light", FlakeRef: "path:/rf-swift-nix", Commands: map[string]string{
		`a"b`:        "ok-attr",
		"ok-cmd":     `attr"; touch /tmp/pwned; "`,
		"$(id)":      "attr",
		"gqrx":       "gqrx",
		"grc":        "gnuradioPackages.gnuradio-rfswift",
		"python3.14": "python3",
	}}
	if err := writeShims(env); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(shimsDir(env.Name))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if len(names) != 3 {
		t.Fatalf("only the plain names get a shim, got %v", names)
	}
}

func TestParseLockedFlakeRef(t *testing.T) {
	github := `{"description":"RF Swift","lastModified":1788362768,"locked":{"lastModified":1788362768,"owner":"PentHertz","repo":"RF-Swift-nix","rev":"ff12ceb11c7d369b3047dfbfcc18c7a774ee5f32","type":"github"},"original":{"owner":"PentHertz","repo":"RF-Swift-nix","type":"github"},"originalUrl":"github:PentHertz/RF-Swift-nix","resolved":{"owner":"PentHertz","repo":"RF-Swift-nix","type":"github"},"resolvedUrl":"github:PentHertz/RF-Swift-nix","revision":"ff12ceb11c7d369b3047dfbfcc18c7a774ee5f32","url":"github:PentHertz/RF-Swift-nix/ff12ceb11c7d369b3047dfbfcc18c7a774ee5f32"}`
	locked, ok, err := parseLockedFlakeRef([]byte(github))
	if err != nil || !ok || locked != "github:PentHertz/RF-Swift-nix/ff12ceb11c7d369b3047dfbfcc18c7a774ee5f32" {
		t.Fatalf("github metadata: %q %v %v", locked, ok, err)
	}
	path := `{"locked":{"lastModified":1,"path":"/home/u/RF-Swift-nix","type":"path"},"original":{"path":"/home/u/RF-Swift-nix","type":"path"},"url":"path:/home/u/RF-Swift-nix?lastModified=1"}`
	if _, ok, err := parseLockedFlakeRef([]byte(path)); err != nil || ok {
		t.Fatalf("a path flake has no revision to pin: %v %v", ok, err)
	}
	if _, _, err := parseLockedFlakeRef([]byte("not json")); err == nil {
		t.Fatal("garbage must be an error")
	}
	if got := shortRev("github:PentHertz/RF-Swift-nix/ff12ceb11c7d369b3047dfbfcc18c7a774ee5f32"); got != "github:PentHertz/RF-Swift-nix/ff12ceb11c7d" {
		t.Fatalf("shortRev = %q", got)
	}
	if got := shortRev("github:PentHertz/RF-Swift-nix"); got != "github:PentHertz/RF-Swift-nix" {
		t.Fatalf("no revision, unchanged: %q", got)
	}
}

func TestLinkedToolsListsPinnedAttrs(t *testing.T) {
	t.Setenv("RFSWIFT_NIX_HOME", t.TempDir())
	if got := linkedTools("lazy"); got != nil {
		t.Fatalf("no tools dir: %v", got)
	}
	if err := ensureDir(toolsDir("lazy")); err != nil {
		t.Fatal(err)
	}
	for _, attr := range []string{"sdrpp-hydrasdr", "gqrx", ".hidden"} {
		if err := os.Symlink("/nix/store/aaaa-"+attr, toolLink("lazy", attr)); err != nil {
			if runtime.GOOS == "windows" {
				t.Skip("symlinks need privileges on Windows; the links live on the Linux side anyway")
			}
			t.Fatal(err)
		}
	}
	got := linkedTools("lazy")
	if len(got) != 2 || got[0] != "gqrx" || got[1] != "sdrpp-hydrasdr" {
		t.Fatalf("linkedTools = %v", got)
	}
	if _, err := ListEnvironmentTools("lazy"); err == nil {
		t.Fatal("no manifest: expected an error")
	}
}

func TestInteractiveCommandForExistingEnvironment(t *testing.T) {
	if !IsAvailable() {
		t.Skip("nix unavailable")
	}
	envs, err := ListEnvironments()
	if err != nil || len(envs) == 0 {
		t.Skip("no persisted RF Swift Nix environment")
	}
	cmd, err := InteractiveCommand(envs[0].Name, "/bin/bash")
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Path == "" || cmd.Dir == "" {
		t.Fatalf("PTY command is incomplete: %#v", cmd)
	}
}
