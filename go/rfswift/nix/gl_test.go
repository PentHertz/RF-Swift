package nix

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func testGLHost(goos string, existing map[string]bool, env map[string]string, files map[string]string) glHost {
	return glHost{
		goos:   goos,
		exists: func(p string) bool { return existing[p] },
		getenv: func(k string) string { return env[k] },
		readFile: func(p string) ([]byte, error) {
			if content, ok := files[p]; ok {
				return []byte(content), nil
			}
			return nil, errors.New("not found")
		},
	}
}

func TestGLNeededOnlyOffNixOSLinux(t *testing.T) {
	if !testGLHost("linux", nil, nil, nil).needed() {
		t.Fatal("a Linux host without /run/opengl-driver needs the runtime")
	}
	if testGLHost("linux", map[string]bool{"/run/opengl-driver": true}, nil, nil).needed() {
		t.Fatal("NixOS provides drivers itself")
	}
	if testGLHost("darwin", nil, nil, nil).needed() {
		t.Fatal("not a Linux concern")
	}
	if testGLHost("linux", nil, map[string]string{"RFSWIFT_NIX_GL": "off"}, nil).needed() {
		t.Fatal("RFSWIFT_NIX_GL=off must disable it")
	}
}

func TestNvidiaDriverVersion(t *testing.T) {
	proc := "NVRM version: NVIDIA UNIX x86_64 Kernel Module  550.120  Fri Sep 13 10:10:01 UTC 2024\nGCC version:  gcc version 13.2.0\n"
	h := testGLHost("linux", nil, nil, map[string]string{"/proc/driver/nvidia/version": proc})
	if got := h.nvidiaDriverVersion(); got != "550.120" {
		t.Fatalf("version = %q", got)
	}
	if got := testGLHost("linux", nil, nil, nil).nvidiaDriverVersion(); got != "" {
		t.Fatalf("no driver file must give no version, got %q", got)
	}
	if got := testGLHost("linux", nil, nil, map[string]string{"/proc/driver/nvidia/version": "garbage"}).nvidiaDriverVersion(); got != "" {
		t.Fatalf("unparseable file must give no version, got %q", got)
	}
}

func TestParseAndMergeGLEnv(t *testing.T) {
	data := []byte("# comment\n\nLIBGL_DRIVERS_PATH=/nix/store/mesa/lib/dri\nLD_LIBRARY_PATH=/nix/store/mesa/lib:/nix/store/glvnd/lib\n=nokey\nbroken line\n")
	vars := parseGLEnv(data)
	if len(vars) != 2 || vars["LIBGL_DRIVERS_PATH"] != "/nix/store/mesa/lib/dri" {
		t.Fatalf("vars = %#v", vars)
	}
	host := map[string]string{"LD_LIBRARY_PATH": "/opt/host/lib"}
	merged := mergeGLEnv(vars, func(k string) string { return host[k] })
	if merged["LD_LIBRARY_PATH"] != "/nix/store/mesa/lib:/nix/store/glvnd/lib:/opt/host/lib" {
		t.Fatalf("host value must stay behind ours: %q", merged["LD_LIBRARY_PATH"])
	}
	if merged["LIBGL_DRIVERS_PATH"] != "/nix/store/mesa/lib/dri" {
		t.Fatalf("unset host value must not add a separator: %q", merged["LIBGL_DRIVERS_PATH"])
	}
	again := mergeGLEnv(vars, func(k string) string { return merged[k] })
	if again["LIBGL_DRIVERS_PATH"] != "/nix/store/mesa/lib/dri" {
		t.Fatalf("re-entering must not duplicate an identical value: %q", again["LIBGL_DRIVERS_PATH"])
	}
	if keys := glEnvKeys(vars); len(keys) != 2 || keys[0] != "LD_LIBRARY_PATH" {
		t.Fatalf("keys = %#v", keys)
	}
}

func TestGLEnvFilePrefersProfileThenCache(t *testing.T) {
	t.Setenv("RFSWIFT_NIX_HOME", t.TempDir())
	profile := "/nix/store/abc-rfswift-sdr_light"
	h := testGLHost("linux", map[string]bool{profile + "/" + glEnvRelPath: true}, nil, nil)
	file, err := glEnvFile(h, "", profile, true)
	if err != nil || file != profile+"/"+glEnvRelPath {
		t.Fatalf("file = %q err = %v", file, err)
	}
	cached := glDir() + "/mesa/" + glEnvRelPath
	h = testGLHost("linux", map[string]bool{cached: true}, nil, nil)
	if file, err := glEnvFile(h, "", "", false); err != nil || file != cached {
		t.Fatalf("cached runtime must be reused: %q %v", file, err)
	}
	if _, err := glEnvFile(testGLHost("linux", nil, nil, nil), "", "", false); err == nil {
		t.Fatal("nothing realised and no flake must be an error, not a build")
	}
	nv := glDir() + "/nvidia-550.120/" + glEnvRelPath
	h = testGLHost("linux", map[string]bool{nv: true, profile + "/" + glEnvRelPath: true}, nil,
		map[string]string{"/proc/driver/nvidia/version": "NVRM version: NVIDIA UNIX x86_64 Kernel Module  550.120  Fri Sep 13 10:10:01 UTC 2024\n"})
	if file, err := glEnvFile(h, "", profile, true); err != nil || file != nv {
		t.Fatalf("NVIDIA runtime must win over the profile's Mesa one: %q %v", file, err)
	}
	h.getenv = func(k string) string {
		if k == "RFSWIFT_NIX_GL" {
			return "mesa"
		}
		return ""
	}
	if file, err := glEnvFile(h, "", profile, true); err != nil || file != profile+"/"+glEnvRelPath {
		t.Fatalf("RFSWIFT_NIX_GL=mesa must ignore the NVIDIA driver: %q %v", file, err)
	}
	// NVIDIA driver present but nothing realised and no flake: Mesa from the
	// profile is the fallback, not an error.
	h = testGLHost("linux", map[string]bool{profile + "/" + glEnvRelPath: true}, nil,
		map[string]string{"/proc/driver/nvidia/version": "NVRM version: NVIDIA UNIX x86_64 Kernel Module  550.120  Fri Sep 13 10:10:01 UTC 2024\n"})
	if file, err := glEnvFile(h, "", profile, false); err != nil || file != profile+"/"+glEnvRelPath {
		t.Fatalf("Mesa fallback expected: %q %v", file, err)
	}
}

func TestGLStatusFor(t *testing.T) {
	t.Setenv("RFSWIFT_NIX_HOME", t.TempDir())
	t.Setenv("RFSWIFT_NIX_GL", "")
	if runtime.GOOS != "linux" {
		st := GLStatusFor(nil)
		if st.Needed {
			t.Fatal("only Linux needs the runtime")
		}
		return
	}
	st := GLStatusFor(&Environment{Name: "x", ProfilePath: t.TempDir()})
	if _, err := os.Stat("/run/opengl-driver"); err == nil {
		if st.Needed {
			t.Fatal("a host with /run/opengl-driver must not need the runtime")
		}
		return
	}
	if !st.Needed || st.File != "" || st.Mode != "auto" {
		t.Fatalf("status = %+v", st)
	}
}

// The two shell launch paths (CLI enter and Workbench PTY) must both carry the
// runtime variables when the profile ships a gl.env.
func TestInteractiveCommandCarriesGLRuntime(t *testing.T) {
	if runtime.GOOS != "linux" || !IsAvailable() {
		t.Skip("Linux with nix only")
	}
	if _, err := os.Stat("/run/opengl-driver"); err == nil {
		t.Skip("NixOS host: runtime not needed")
	}
	home := t.TempDir()
	t.Setenv("RFSWIFT_NIX_HOME", home)
	t.Setenv("RFSWIFT_NIX_GL", "")
	t.Setenv("LD_LIBRARY_PATH", "/host/lib")
	t.Setenv("DISPLAY", "")
	profile := t.TempDir()
	os.MkdirAll(profile+"/bin", 0o755)
	os.MkdirAll(profile+"/share/rfswift", 0o755)
	os.WriteFile(profile+"/share/rfswift/gl.env", []byte("LIBGL_DRIVERS_PATH=/nix/store/mesa/lib/dri\nLD_LIBRARY_PATH=/nix/store/mesa/lib\n"), 0o644)
	env := &Environment{Name: "gltest", Image: "rfid", FlakeRef: "/nowhere", ProfilePath: profile, Workspace: home}
	if err := writeManifest(env); err != nil {
		t.Fatal(err)
	}
	// Under WSL 2 the runtime appends WSLg's GPU library directory (see
	// withWSLGPULibs); the expectation follows the host this runs on.
	wantLD := "/nix/store/mesa/lib:/host/lib"
	if dir := currentGLHost().wslGPULibs(); dir != "" {
		wantLD += ":" + dir
	}
	vars := GLEnvironment(env, false)
	if vars["LIBGL_DRIVERS_PATH"] != "/nix/store/mesa/lib/dri" || vars["LD_LIBRARY_PATH"] != wantLD || vars[GLRuntimeVar] != profile+"/share/rfswift/gl.env" {
		t.Fatalf("vars = %#v", vars)
	}
	cmd, err := InteractiveCommand("gltest", "/bin/sh")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, kv := range cmd.Env {
		k, v, _ := strings.Cut(kv, "=")
		got[k] = v
	}
	if got["LIBGL_DRIVERS_PATH"] != "/nix/store/mesa/lib/dri" || got["LD_LIBRARY_PATH"] != wantLD || got[GLRuntimeVar] == "" || !strings.HasPrefix(got["PATH"], profile+"/bin") {
		t.Fatalf("Workbench PTY command lacks the runtime: %#v", got)
	}
	if st := GLStatusFor(env); !st.Needed || st.File != profile+"/share/rfswift/gl.env" {
		t.Fatalf("status = %+v", st)
	}
}

func TestGPUsFromSysfs(t *testing.T) {
	h := testGLHost("linux", nil, nil, map[string]string{
		"/sys/class/drm/card0/device/vendor": "0x8086\n",
		"/sys/class/drm/card1/device/vendor": "0x10de\n",
		"/sys/class/drm/card2/device/vendor": "0x15ad\n",
	})
	h.glob = func(string) ([]string, error) {
		return []string{"/sys/class/drm/card0", "/sys/class/drm/card0-eDP-1", "/sys/class/drm/card1", "/sys/class/drm/card2"}, nil
	}
	h.readlink = func(p string) (string, error) {
		switch p {
		case "/sys/class/drm/card0/device/driver":
			return "../../../../bus/pci/drivers/i915", nil
		case "/sys/class/drm/card1/device/driver":
			return "../../../../bus/pci/drivers/nvidia", nil
		}
		return "", errors.New("no driver bound")
	}
	gpus := h.gpus()
	if len(gpus) != 3 {
		t.Fatalf("gpus = %#v", gpus)
	}
	if gpus[0].Vendor != "Intel" || gpus[0].Driver != "i915" || gpus[0].VendorID != "0x8086" {
		t.Fatalf("card0 = %#v", gpus[0])
	}
	if gpus[1].Vendor != "NVIDIA" || gpus[1].Driver != "nvidia" {
		t.Fatalf("card1 = %#v", gpus[1])
	}
	if gpus[2].Vendor != "VMware" || gpus[2].Driver != "" {
		t.Fatalf("card2 (no driver) = %#v", gpus[2])
	}
	if got := testGLHost("darwin", nil, nil, nil).gpus(); got != nil {
		t.Fatalf("sysfs is Linux only, got %#v", got)
	}
	advice := GPUAdvice(GLStatus{GPUs: gpus, NvidiaVersion: "550.120", Mode: "auto"})
	if len(advice) != 3 || !strings.Contains(advice[0], "Mesa") || !strings.Contains(advice[1], "550.120") {
		t.Fatalf("advice = %q", advice)
	}
}

func TestGLFallbackExpression(t *testing.T) {
	file := "/home/u/.rfswift/nix/gl/rfswift-gl.nix"
	got := glFallbackExpression("github:PentHertz/RF-Swift-nix", file, "")
	want := `let flake = builtins.getFlake "github:PentHertz/RF-Swift-nix"; pkgs = flake.legacyPackages.${builtins.currentSystem}; in pkgs.callPackage /home/u/.rfswift/nix/gl/rfswift-gl.nix { withNvidia = false; nvidiaVersion = null; }`
	if got != want {
		t.Fatalf("expression = %s", got)
	}
	nvidia := glFallbackExpression("/srv/RF-Swift-nix", file, "550.120")
	if !strings.Contains(nvidia, `builtins.getFlake "/srv/RF-Swift-nix"`) || !strings.Contains(nvidia, `withNvidia = true; nvidiaVersion = "550.120";`) {
		t.Fatalf("nvidia expression = %s", nvidia)
	}
	if got := nixString(`a"b${c}\d`); got != `"a\"b\${c}\\d"` {
		t.Fatalf("nixString = %s", got)
	}
	if !missingFlakeAttribute("error: flake 'github:PentHertz/RF-Swift-nix' does not provide attribute 'packages.x86_64-linux.pkg-rfswift-gl', 'legacyPackages.x86_64-linux.pkg-rfswift-gl' or 'pkg-rfswift-gl'") {
		t.Fatal("Nix's missing-output message must be recognised")
	}
	if missingFlakeAttribute("error: builder for '/nix/store/x-rfswift-gl.drv' failed with exit code 1") {
		t.Fatal("a failed build is not a missing attribute")
	}
}

func TestEmbeddedGLExpressionMatchesFlake(t *testing.T) {
	if !strings.Contains(embeddedGLExpression, "share/rfswift/gl.env") || !strings.Contains(embeddedGLExpression, "rfswift-gl-probe") {
		t.Fatal("embedded rfswift-gl.nix must produce gl.env and the probe")
	}
	// Developer layout: RF-Swift and RF-Swift-nix side by side.
	sibling := filepath.Join("..", "..", "..", "..", "RF-Swift-nix", "pkgs", "rfswift-gl.nix")
	data, err := os.ReadFile(sibling)
	if err != nil {
		t.Skip("RF-Swift-nix checkout not beside this repository")
	}
	if string(data) != embeddedGLExpression {
		t.Fatal("nix/rfswift-gl.nix differs from RF-Swift-nix/pkgs/rfswift-gl.nix: copy the flake's file over the embedded one")
	}
}

func TestPluginPathEnvFromProfiles(t *testing.T) {
	base := t.TempDir()
	t.Setenv("RFSWIFT_NIX_HOME", base)
	t.Setenv("SOAPY_SDR_PLUGIN_PATH", "/opt/host/modules")
	profile := filepath.Join(base, "profile")
	extras := filepath.Join(EnvExtrasProfile("sdr"), "lib", "SoapySDR", "modules0.8-3")
	for _, d := range []string{filepath.Join(profile, "lib", "SoapySDR", "modules0.8-3"), extras} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	env := &Environment{Name: "sdr", ProfilePath: profile}
	vars := pluginPathEnv(env)
	want := filepath.Join(profile, "lib", "SoapySDR", "modules0.8-3") + string(os.PathListSeparator) + extras + string(os.PathListSeparator) + "/opt/host/modules"
	if vars["SOAPY_SDR_PLUGIN_PATH"] != want {
		t.Fatalf("SOAPY_SDR_PLUGIN_PATH = %q, want %q", vars["SOAPY_SDR_PLUGIN_PATH"], want)
	}
	if pluginPathEnv(&Environment{Name: "none", ProfilePath: filepath.Join(base, "missing")}) != nil {
		t.Fatal("no module directory anywhere must export nothing")
	}
	if pluginPathEnv(nil) != nil {
		t.Fatal("nil environment must export nothing")
	}
}
