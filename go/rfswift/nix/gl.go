/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
*
*  Nix engine - OpenGL runtime for hosts that are not NixOS.
*
*  nixpkgs programs look for GPU drivers in /run/opengl-driver, which only
*  NixOS provides. Everywhere else EGL/GLX find nothing and GUI tools fail
*  (SDR++: "EGL: Failed to get EGL display", then a segfault on the window it
*  never got). The flake ships pkgs/rfswift-gl.nix, whose share/rfswift/gl.env
*  lists the variables that point the loaders at Mesa's drivers from the same
*  pin; this file finds or builds that file and merges its variables into
*  every environment shell. Hosts running the proprietary NVIDIA driver get
*  the matching user-space libraries instead (pkg-rfswift-gl-nvidia, realised
*  once per driver version).
*
*  Driver matrix: Intel, AMD, VMware, virtio and every other open kernel
*  driver are served by Mesa from the environment's own nixpkgs pin; the
*  proprietary NVIDIA driver by user-space libraries matching the loaded
*  kernel module, with Mesa behind them for hybrid laptops. macOS needs none
*  of this: nixpkgs programs use Apple's OpenGL/Metal directly.
*
*  The flake may predate the runtime package (an environment created from a
*  published revision without it). The same Nix expression is embedded here
*  and built against that flake's nixpkgs in that case, so the drivers still
*  match the libraries the tools were linked with.
 */

package nix

import (
	"bufio"
	"bytes"
	_ "embed"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"

	common "penthertz/rfswift/common"
)

// embeddedGLExpression is a copy of RF-Swift-nix/pkgs/rfswift-gl.nix, the
// package that produces gl.env. Keep the two files identical
// (TestEmbeddedGLExpressionMatchesFlake checks that in the sibling layout).
//
//go:embed rfswift-gl.nix
var embeddedGLExpression string

// glEnvRelPath is where the runtime package stores its variables.
const glEnvRelPath = "share/rfswift/gl.env"

// glProbeRelPath is the runtime's context probe (see rfswift-gl.nix).
const glProbeRelPath = "bin/rfswift-gl-probe"

// glHost is the injectable host view for the decision logic.
type glHost struct {
	goos     string
	exists   func(string) bool
	getenv   func(string) string
	readFile func(string) ([]byte, error)
	glob     func(string) ([]string, error)
	readlink func(string) (string, error)
}

func currentGLHost() glHost {
	return glHost{
		goos:     runtime.GOOS,
		exists:   pathExists,
		getenv:   os.Getenv,
		readFile: os.ReadFile,
		glob:     filepath.Glob,
		readlink: os.Readlink,
	}
}

// mode is RFSWIFT_NIX_GL: "" (auto), "off" (never set anything), or "mesa"
// (ignore a proprietary NVIDIA driver and use Mesa, e.g. on a hybrid laptop
// whose display runs on the Intel/AMD GPU).
func (h glHost) mode() string {
	return strings.ToLower(strings.TrimSpace(h.getenv("RFSWIFT_NIX_GL")))
}

// needed reports whether nixpkgs binaries need help finding GPU drivers on
// this host: Linux, not NixOS (no /run/opengl-driver), and not disabled with
// RFSWIFT_NIX_GL=off.
func (h glHost) needed() bool {
	if h.goos != "linux" || h.mode() == "off" {
		return false
	}
	return !h.exists("/run/opengl-driver")
}

var nvidiaVersionRe = regexp.MustCompile(`Module\s+([0-9][0-9.]*)\s`)

// nvidiaDriverVersion returns the proprietary NVIDIA kernel module version, or
// "" when that driver is not loaded (Mesa serves nouveau, Intel and AMD).
func (h glHost) nvidiaDriverVersion() string {
	data, err := h.readFile("/proc/driver/nvidia/version")
	if err != nil {
		return ""
	}
	m := nvidiaVersionRe.FindSubmatch(data)
	if m == nil {
		return ""
	}
	return string(m[1])
}

// GPU is one DRM device of the host, as the kernel exposes it.
type GPU struct {
	Card     string `json:"card"`     // card0, card1, ...
	Vendor   string `json:"vendor"`   // Intel, AMD, NVIDIA, VMware, ... or the PCI id
	VendorID string `json:"vendorId"` // 0x8086, 0x1002, 0x10de, ...
	Driver   string `json:"driver"`   // kernel driver bound to it: i915, xe, amdgpu, nvidia, nouveau, vmwgfx, ...
}

// pciVendors names the GPU vendors seen on RF Swift hosts.
var pciVendors = map[string]string{
	"0x8086": "Intel",
	"0x1002": "AMD",
	"0x1022": "AMD",
	"0x10de": "NVIDIA",
	"0x15ad": "VMware",
	"0x1af4": "virtio",
	"0x1b36": "QEMU",
	"0x1234": "QEMU",
	"0x1013": "Cirrus",
	"0x14e4": "Broadcom",
	"0x5143": "Qualcomm",
	"0x1d0f": "Amazon",
	"0x1a03": "ASPEED",
	"0x102b": "Matrox",
}

// gpus lists the host's DRM devices from sysfs. Missing pieces are left empty
// rather than failing: this is diagnostics, not a gate.
func (h glHost) gpus() []GPU {
	if h.goos != "linux" || h.glob == nil {
		return nil
	}
	cards, _ := h.glob("/sys/class/drm/card[0-9]*")
	var out []GPU
	seen := map[string]bool{}
	for _, card := range cards {
		name := filepath.Base(card)
		// card0-HDMI-A-1 style connector entries share the card's device.
		if strings.Contains(name, "-") {
			continue
		}
		device := filepath.Join(card, "device")
		g := GPU{Card: name}
		if data, err := h.readFile(filepath.Join(device, "vendor")); err == nil {
			g.VendorID = strings.TrimSpace(string(data))
			if v, ok := pciVendors[strings.ToLower(g.VendorID)]; ok {
				g.Vendor = v
			} else {
				g.Vendor = g.VendorID
			}
		}
		if h.readlink != nil {
			if target, err := h.readlink(filepath.Join(device, "driver")); err == nil {
				g.Driver = filepath.Base(target)
			}
		}
		key := g.VendorID + "/" + g.Driver
		if g.VendorID == "" && g.Driver == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, g)
	}
	return out
}

// parseGLEnv reads KEY=VALUE lines; blank lines and # comments are skipped.
func parseGLEnv(data []byte) map[string]string {
	vars := map[string]string{}
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(key) == "" {
			continue
		}
		vars[strings.TrimSpace(key)] = value
	}
	return vars
}

// mergeGLEnv prepends the runtime's values to whatever the host already has
// (every variable is a path list), so a user's own settings stay effective.
func mergeGLEnv(vars map[string]string, getenv func(string) string) map[string]string {
	out := make(map[string]string, len(vars))
	for k, v := range vars {
		if cur := getenv(k); cur != "" && cur != v {
			v = v + string(os.PathListSeparator) + cur
		}
		out[k] = v
	}
	return out
}

// glEnvKeys lists the variables in a stable order (for command lines).
func glEnvKeys(vars map[string]string) []string {
	keys := make([]string, 0, len(vars))
	for k := range vars {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// glDir holds the realised runtimes: ~/.rfswift/nix/gl/{mesa,nvidia-<version>}.
func glDir() string {
	return filepath.Join(BaseDir(), "gl")
}

// GLEnvironment returns the variables to add to a shell of env, or nil when
// the host needs none (NixOS, non-Linux, disabled) or the runtime could not be
// realised (a warning is printed; GUI tools then behave as before).
//
// refresh re-realises the Mesa runtime from the flake, a no-op when nothing
// changed: `run` passes true, `exec` and single tool runs reuse what exists.
func GLEnvironment(env *Environment, refresh bool) map[string]string {
	if env == nil {
		return nil
	}
	return glEnvironmentFor(env.FlakeRef, env.ProfilePath, refresh)
}

// GLEnvironmentForFlake is the same for a tool run outside any environment
// (`rfswift nix run <image> <tool>`).
func GLEnvironmentForFlake(flakeRef string) map[string]string {
	return glEnvironmentFor(flakeRef, "", false)
}

// GLRuntimeVar names the variable that records which gl.env a shell got, so
// the banner and `rfswift nix gl` can show it.
const GLRuntimeVar = "RFSWIFT_NIX_GL_RUNTIME"

func glEnvironmentFor(flakeRef, profilePath string, refresh bool) map[string]string {
	h := currentGLHost()
	if !h.needed() {
		return nil
	}
	file, err := glEnvFile(h, flakeRef, profilePath, refresh)
	if err != nil {
		common.PrintWarningMessage(fmt.Sprintf("OpenGL runtime unavailable, GUI tools may fail to open a window: %v", err))
		return nil
	}
	vars, err := loadGLEnv(file)
	if err != nil {
		common.PrintWarningMessage(fmt.Sprintf("OpenGL runtime unavailable, GUI tools may fail to open a window: %v", err))
		return nil
	}
	return vars
}

// loadGLEnv reads a gl.env and merges it with the host's variables.
func loadGLEnv(file string) (map[string]string, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	vars := mergeGLEnv(parseGLEnv(data), os.Getenv)
	vars[GLRuntimeVar] = file
	return vars, nil
}

// GLStatus describes what the runtime would do on this host, for diagnostics.
type GLStatus struct {
	Needed        bool              `json:"needed"`        // host is Linux, not NixOS, not disabled
	Reason        string            `json:"reason"`        // why it is or is not needed
	Mode          string            `json:"mode"`          // RFSWIFT_NIX_GL value (auto when empty)
	NvidiaVersion string            `json:"nvidiaVersion"` // proprietary driver loaded, if any
	GPUs          []GPU             `json:"gpus"`          // DRM devices seen in sysfs
	Runtime       string            `json:"runtime"`       // which runtime applies: mesa, nvidia-<version>, none
	File          string            `json:"file"`          // gl.env that would be used, if realised
	Vars          map[string]string `json:"vars"`          // its variables (before merging with the host's)
}

// GLStatusFor inspects the host and the environment without building anything.
func GLStatusFor(env *Environment) GLStatus {
	h := currentGLHost()
	st := GLStatus{Mode: h.mode(), NvidiaVersion: h.nvidiaDriverVersion(), GPUs: h.gpus(), Runtime: "none", Vars: map[string]string{}}
	if st.Mode == "" {
		st.Mode = "auto"
	}
	switch {
	case h.goos == "darwin":
		st.Reason = "macOS: nixpkgs programs use Apple's OpenGL/Metal directly, the GPU works without a runtime"
	case h.goos != "linux":
		st.Reason = "not Linux: nixpkgs programs use the system's OpenGL directly"
	case h.mode() == "off":
		st.Reason = "disabled with RFSWIFT_NIX_GL=off"
	case h.exists("/run/opengl-driver"):
		st.Reason = "/run/opengl-driver exists (NixOS or equivalent): the host provides drivers to nixpkgs programs"
	default:
		st.Needed = true
		st.Reason = "host is not NixOS: nixpkgs Mesa/libglvnd find no drivers without the runtime"
	}
	if !st.Needed {
		return st
	}
	st.Runtime = "mesa"
	candidates := []string{}
	if st.NvidiaVersion != "" && h.mode() != "mesa" {
		st.Runtime = "nvidia-" + st.NvidiaVersion
		candidates = append(candidates, filepath.Join(glDir(), "nvidia-"+st.NvidiaVersion, glEnvRelPath))
	}
	if env != nil && env.ProfilePath != "" {
		candidates = append(candidates, filepath.Join(env.ProfilePath, glEnvRelPath))
	}
	candidates = append(candidates, filepath.Join(glDir(), "mesa", glEnvRelPath))
	for _, file := range candidates {
		if h.exists(file) {
			st.File = file
			if data, err := h.readFile(file); err == nil {
				st.Vars = parseGLEnv(data)
			}
			break
		}
	}
	return st
}

// GPUAdvice explains, per detected GPU, which driver stack the runtime uses
// and what to do when that is not the right one. Empty when nothing is known.
func GPUAdvice(st GLStatus) []string {
	var lines []string
	for _, g := range st.GPUs {
		switch {
		case g.Vendor == "NVIDIA" && g.Driver == "nvidia":
			if st.NvidiaVersion != "" && st.Mode != "mesa" {
				lines = append(lines, fmt.Sprintf("%s: NVIDIA on the proprietary driver %s: matching user-space libraries are realised once (downloads the NVIDIA installer); RFSWIFT_NIX_GL=mesa forces Mesa instead (hybrid laptops rendering on the other GPU).", g.Card, st.NvidiaVersion))
			} else {
				lines = append(lines, fmt.Sprintf("%s: NVIDIA on the proprietary driver, served by Mesa (%s): software rendering unless RFSWIFT_NIX_GL is unset.", g.Card, st.Mode))
			}
		case g.Vendor == "NVIDIA":
			lines = append(lines, fmt.Sprintf("%s: NVIDIA on %s (open driver): Mesa nouveau from the environment's nixpkgs.", g.Card, orUnknown(g.Driver)))
		case g.Vendor == "Intel":
			lines = append(lines, fmt.Sprintf("%s: Intel on %s: Mesa (iris/crocus/i915) from the environment's nixpkgs.", g.Card, orUnknown(g.Driver)))
		case g.Vendor == "AMD":
			lines = append(lines, fmt.Sprintf("%s: AMD on %s: Mesa (radeonsi/r600) from the environment's nixpkgs; the proprietary AMDGPU-PRO OpenGL is not used.", g.Card, orUnknown(g.Driver)))
		case g.Vendor == "VMware" || g.Vendor == "virtio" || g.Vendor == "QEMU":
			lines = append(lines, fmt.Sprintf("%s: %s virtual GPU on %s: Mesa (svga/virgl or llvmpipe software rendering).", g.Card, g.Vendor, orUnknown(g.Driver)))
		default:
			lines = append(lines, fmt.Sprintf("%s: %s on %s: Mesa from the environment's nixpkgs (llvmpipe when no hardware driver matches).", g.Card, g.Vendor, orUnknown(g.Driver)))
		}
	}
	return lines
}

func orUnknown(s string) string {
	if s == "" {
		return "an unknown kernel driver"
	}
	return s
}

// glEnvFile locates the gl.env to use, realising the runtime when missing.
func glEnvFile(h glHost, flakeRef, profilePath string, refresh bool) (string, error) {
	if version := h.nvidiaDriverVersion(); version != "" && h.mode() != "mesa" {
		// The user-space driver must match the kernel module, so it is keyed
		// by version and built once; a driver upgrade yields a new build.
		// When that build is impossible (version not downloadable, offline)
		// fall through to Mesa, which still renders (in software if the GPU
		// is not one of its own).
		link := filepath.Join(glDir(), "nvidia-"+version)
		file := filepath.Join(link, glEnvRelPath)
		if h.exists(file) {
			return file, nil
		}
		if flakeRef != "" {
			common.PrintInfoMessage(fmt.Sprintf("OpenGL: NVIDIA driver %s on a non-NixOS host; realising the matching user-space libraries once (this downloads the NVIDIA installer)...", version))
			if err := buildGLRuntime(flakeRef, link, version); err == nil {
				return file, nil
			} else {
				common.PrintWarningMessage(fmt.Sprintf("OpenGL: NVIDIA %s user-space libraries could not be realised (%v); using Mesa instead (RFSWIFT_NIX_GL=mesa silences this).", version, err))
			}
		}
	}
	// Eager profiles carry the Mesa runtime of their own pin.
	if profilePath != "" {
		if file := filepath.Join(profilePath, glEnvRelPath); h.exists(file) {
			return file, nil
		}
	}
	link := filepath.Join(glDir(), "mesa")
	file := filepath.Join(link, glEnvRelPath)
	if h.exists(file) && (!refresh || flakeRef == "") {
		return file, nil
	}
	if flakeRef == "" {
		return "", fmt.Errorf("no flake to build the Mesa runtime from")
	}
	if !h.exists(file) {
		common.PrintInfoMessage("OpenGL: host is not NixOS; realising Mesa drivers for the environment's GUI tools (RFSWIFT_NIX_GL=off disables this).")
	}
	if err := buildGLRuntime(flakeRef, link, ""); err != nil {
		if h.exists(file) {
			// Keep the previous runtime rather than losing GUI tools.
			common.PrintWarningMessage(fmt.Sprintf("OpenGL runtime refresh failed, keeping the current one: %v", err))
			return file, nil
		}
		return "", err
	}
	return file, nil
}

// buildGLRuntime realises the Mesa runtime (nvidiaVersion == "") or the
// NVIDIA one and pins it at outLink. The flake's own package is preferred;
// a flake that does not carry it gets the embedded copy of the expression,
// evaluated against that flake's nixpkgs.
func buildGLRuntime(flakeRef, outLink, nvidiaVersion string) error {
	if err := ensureDir(filepath.Dir(outLink)); err != nil {
		return err
	}
	attr := "pkg-rfswift-gl"
	var extraEnv []string
	impure := false
	if nvidiaVersion != "" {
		attr = "pkg-rfswift-gl-nvidia"
		extraEnv = []string{"RFSWIFT_NVIDIA_VERSION=" + nvidiaVersion}
		impure = true
	}
	stderr, err := nixBuildOutLink([]string{fmt.Sprintf("%s#%s", flakeRef, attr)}, outLink, extraEnv, impure)
	if err == nil {
		return nil
	}
	if !missingFlakeAttribute(stderr) {
		return fmt.Errorf("nix build %s#%s: %w", flakeRef, attr, err)
	}
	common.PrintInfoMessage(fmt.Sprintf("OpenGL: %s does not provide %s (older revision); building RF Swift's own copy of the runtime against that flake's nixpkgs.", flakeRef, attr))
	file, err := glExpressionFile()
	if err != nil {
		return err
	}
	if _, err := nixBuildOutLink([]string{"--expr", glFallbackExpression(flakeRef, file, nvidiaVersion)}, outLink, extraEnv, true); err != nil {
		return fmt.Errorf("nix build of the embedded OpenGL runtime against %s: %w", flakeRef, err)
	}
	return nil
}

// nixBuildOutLink runs `nix build <args> --out-link <outLink>` streaming its
// output, and also returns what it wrote to stderr so callers can tell an
// absent attribute from a failed build.
func nixBuildOutLink(args []string, outLink string, extraEnv []string, impure bool) (string, error) {
	full := append(experimentalArgs(), "build")
	full = append(full, args...)
	full = append(full, "--out-link", outLink)
	if impure {
		full = append(full, "--impure")
	}
	cmd := exec.Command(NixBinary(), full...)
	cmd.Env = append(os.Environ(), extraEnv...)
	var stderr bytes.Buffer
	cmd.Stdout = os.Stdout
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderr)
	cmd.Stdin = os.Stdin
	err := cmd.Run()
	return stderr.String(), err
}

// missingFlakeAttribute recognises Nix's message for a flake output that does
// not exist, as opposed to one that failed to build.
func missingFlakeAttribute(stderr string) bool {
	return strings.Contains(stderr, "does not provide attribute")
}

// glExpressionFile writes the embedded runtime expression under the GL state
// directory (Nix evaluates files, not strings, for callPackage).
func glExpressionFile() (string, error) {
	if err := ensureDir(glDir()); err != nil {
		return "", err
	}
	path := filepath.Join(glDir(), "rfswift-gl.nix")
	if cur, err := os.ReadFile(path); err == nil && string(cur) == embeddedGLExpression {
		return path, nil
	}
	return path, os.WriteFile(path, []byte(embeddedGLExpression), 0o644)
}

// glFallbackExpression evaluates the embedded package against the flake's
// nixpkgs (legacyPackages, which every RF-Swift-nix revision exposes), so the
// drivers match the libraries the environment's tools were built with.
func glFallbackExpression(flakeRef, file, nvidiaVersion string) string {
	version := "null"
	if nvidiaVersion != "" {
		version = nixString(nvidiaVersion)
	}
	return fmt.Sprintf(
		"let flake = builtins.getFlake %s; pkgs = flake.legacyPackages.${builtins.currentSystem}; in pkgs.callPackage %s { withNvidia = %t; nvidiaVersion = %s; }",
		nixString(flakeRef), file, nvidiaVersion != "", version)
}

// nixString quotes s as a Nix string literal.
func nixString(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "${", `\${`, "\n", `\n`)
	return `"` + r.Replace(s) + `"`
}

// GLProbe creates an OpenGL context with the runtime the environment would
// get, using the probe shipped in the runtime package, and returns its report
// (driver vendor/renderer/version) or the error EGL raised. This is what
// `rfswift nix gl --check` prints: the same failure SDR++ would hit, in one
// line and without a window.
func GLProbe(env *Environment) (string, error) {
	h := currentGLHost()
	if !h.needed() {
		return "", fmt.Errorf("no runtime applies on this host (%s)", GLStatusFor(env).Reason)
	}
	flakeRef, profile := "", ""
	if env != nil {
		flakeRef, profile = env.FlakeRef, env.ProfilePath
	}
	file, err := glEnvFile(h, flakeRef, profile, false)
	if err != nil {
		return "", err
	}
	probe := filepath.Join(filepath.Dir(filepath.Dir(filepath.Dir(file))), glProbeRelPath)
	if !h.exists(probe) {
		return "", fmt.Errorf("the runtime at %s carries no probe (built from a flake older than it); re-run 'rfswift run --engine nix' on the environment to refresh it", filepath.Dir(filepath.Dir(filepath.Dir(file))))
	}
	vars, err := loadGLEnv(file)
	if err != nil {
		return "", err
	}
	cmd := exec.Command(probe)
	cmd.Env = withEnv(os.Environ(), vars)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return strings.TrimSpace(string(out)), fmt.Errorf("no OpenGL context with %s: %w", file, err)
	}
	return strings.TrimSpace(string(out)), nil
}
