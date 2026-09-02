package nix

import (
	"os"
	"strings"
	"testing"
)

func TestGLDetectsWSL(t *testing.T) {
	byEnv := testGLHost("linux", nil, map[string]string{"WSL_DISTRO_NAME": "Ubuntu-24.04"}, nil)
	if !byEnv.isWSL() {
		t.Fatal("WSL_DISTRO_NAME marks a WSL distribution")
	}
	byKernel := testGLHost("linux", nil, nil, map[string]string{"/proc/sys/kernel/osrelease": "6.6.87.2-microsoft-standard-WSL2\n"})
	if !byKernel.isWSL() {
		t.Fatal("the Microsoft kernel marks a WSL distribution")
	}
	if testGLHost("linux", nil, nil, map[string]string{"/proc/sys/kernel/osrelease": "6.12.3-arch1-1\n"}).isWSL() {
		t.Fatal("a regular kernel is not WSL")
	}
	if testGLHost("windows", nil, map[string]string{"WSL_DISTRO_NAME": "x"}, nil).isWSL() {
		t.Fatal("only Linux can be WSL")
	}
}

func TestGLWSLAppendsGPULibsBehindNixLibraries(t *testing.T) {
	glEnv := "LIBGL_DRIVERS_PATH=/nix/store/mesa/lib/dri\nLD_LIBRARY_PATH=/nix/store/mesa/lib:/nix/store/glvnd/lib\n"
	h := testGLHost("linux",
		map[string]bool{wslGPULibDir: true},
		map[string]string{"WSL_DISTRO_NAME": "Ubuntu-24.04", "LD_LIBRARY_PATH": "/opt/host/lib"},
		map[string]string{"/nix/store/gl/share/rfswift/gl.env": glEnv})
	vars, err := loadGLEnvFor(h, "/nix/store/gl/share/rfswift/gl.env")
	if err != nil {
		t.Fatal(err)
	}
	// The host merge uses the OS list separator (":" on the Linux this runs on
	// in production); the WSLg directory is always appended Linux-style.
	if got := vars["LD_LIBRARY_PATH"]; got != "/nix/store/mesa/lib:/nix/store/glvnd/lib"+string(os.PathListSeparator)+"/opt/host/lib:"+wslGPULibDir {
		t.Fatalf("LD_LIBRARY_PATH = %q: WSLg libraries must come last", got)
	}
	if vars[GLRuntimeVar] != "/nix/store/gl/share/rfswift/gl.env" {
		t.Fatalf("runtime marker = %q", vars[GLRuntimeVar])
	}
	// Idempotent, and absent when the directory does not exist or off WSL.
	if again := withWSLGPULibs(vars, wslGPULibDir); strings.Count(again["LD_LIBRARY_PATH"], wslGPULibDir) != 1 {
		t.Fatalf("duplicated: %q", again["LD_LIBRARY_PATH"])
	}
	noDir := testGLHost("linux", nil, map[string]string{"WSL_DISTRO_NAME": "Ubuntu-24.04"}, map[string]string{"/nix/store/gl/share/rfswift/gl.env": glEnv})
	if vars, _ := loadGLEnvFor(noDir, "/nix/store/gl/share/rfswift/gl.env"); strings.Contains(vars["LD_LIBRARY_PATH"], wslGPULibDir) {
		t.Fatalf("no %s, nothing to append: %q", wslGPULibDir, vars["LD_LIBRARY_PATH"])
	}
	plain := testGLHost("linux", map[string]bool{wslGPULibDir: true}, nil, map[string]string{"/nix/store/gl/share/rfswift/gl.env": glEnv})
	if vars, _ := loadGLEnvFor(plain, "/nix/store/gl/share/rfswift/gl.env"); strings.Contains(vars["LD_LIBRARY_PATH"], wslGPULibDir) {
		t.Fatalf("a non-WSL Linux never gets the WSLg directory: %q", vars["LD_LIBRARY_PATH"])
	}
}

func TestGLAdviceMentionsWSL(t *testing.T) {
	lines := GPUAdvice(GLStatus{WSL: true, WSLGPULibs: wslGPULibDir})
	if len(lines) != 2 || !strings.Contains(lines[0], "X11") || !strings.Contains(lines[0], WSLWaylandVar) || !strings.Contains(lines[1], wslGPULibDir) {
		t.Fatalf("advice = %q", lines)
	}
	lines = GPUAdvice(GLStatus{WSL: true})
	if len(lines) != 2 || !strings.Contains(lines[1], "wsl --update") {
		t.Fatalf("missing libraries advice = %q", lines)
	}
	if lines := GPUAdvice(GLStatus{}); len(lines) != 0 {
		t.Fatalf("no GPUs, no WSL, no advice: %q", lines)
	}
}

// Under WSLg GLFW prefers Wayland and stalls at window creation; the engine
// steers GUI tools to Xwayland unless asked not to.
func TestWSLDisplayEnvPrefersX11(t *testing.T) {
	wsl := map[string]string{"WSL_DISTRO_NAME": "Ubuntu-24.04", "DISPLAY": ":0", "WAYLAND_DISPLAY": "wayland-0"}
	got := wslDisplayEnv(testGLHost("linux", nil, wsl, nil))
	if got["WAYLAND_DISPLAY"] != wslX11Marker || len(got) != 1 {
		t.Fatalf("WSL with an X display must steer to X11: %v", got)
	}
	if got := wslDisplayEnv(testGLHost("linux", nil, map[string]string{"WSL_DISTRO_NAME": "u", "WAYLAND_DISPLAY": "wayland-0"}, nil)); got != nil {
		t.Fatalf("without an X display there is nothing to fall back to: %v", got)
	}
	optOut := map[string]string{"WSL_DISTRO_NAME": "u", "DISPLAY": ":0", WSLWaylandVar: "1"}
	if got := wslDisplayEnv(testGLHost("linux", nil, optOut, nil)); got != nil {
		t.Fatalf("%s=1 keeps Wayland: %v", WSLWaylandVar, got)
	}
	if got := wslDisplayEnv(testGLHost("linux", nil, map[string]string{"DISPLAY": ":0", "WAYLAND_DISPLAY": "wayland-0"}, nil)); got != nil {
		t.Fatalf("a regular Linux desktop is left alone: %v", got)
	}
	if got := wslDisplayEnv(testGLHost("windows", nil, wsl, nil)); got != nil {
		t.Fatalf("the Windows side runs no GUI tool itself: %v", got)
	}
}
