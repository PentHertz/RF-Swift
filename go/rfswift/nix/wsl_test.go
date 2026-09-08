package nix

import (
	"reflect"
	"strings"
	"testing"
)

func TestTranslateArgsRewritesWindowsPathsOnly(t *testing.T) {
	in := []string{
		"run", "--engine", "nix", "-i", "sdr_light", "-n", "lab",
		"--workspace", `C:\Users\fluxius\rfswift-workspace\lab`,
		`--flake=D:\src\RF-Swift-nix`,
		"--out", `\\wsl.localhost\Ubuntu-24.04\home\fluxius\.rfswift\nix\environments\lab\security-report`,
		"github:PentHertz/RF-Swift-nix", "/home/fluxius/x", "c 189:* rwm", "--lazy",
	}
	got := translateArgs(in)
	want := []string{
		"run", "--engine", "nix", "-i", "sdr_light", "-n", "lab",
		"--workspace", "/mnt/c/Users/fluxius/rfswift-workspace/lab",
		"--flake=/mnt/d/src/RF-Swift-nix",
		"--out", "/home/fluxius/.rfswift/nix/environments/lab/security-report",
		"github:PentHertz/RF-Swift-nix", "/home/fluxius/x", "c 189:* rwm", "--lazy",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("translateArgs =\n%q\nwant\n%q", got, want)
	}
}

func TestComposeWSLENV(t *testing.T) {
	got := composeWSLENV("USERPROFILE/p:RFSWIFT_ENGINE", []string{"RFSWIFT_ENGINE", "RFSWIFT_NIX_GL", "NO_COLOR"})
	if got != "USERPROFILE/p:RFSWIFT_ENGINE:RFSWIFT_NIX_GL:NO_COLOR" {
		t.Fatalf("WSLENV = %q", got)
	}
	if got := composeWSLENV("", nil); got != "" {
		t.Fatalf("empty must stay empty, got %q", got)
	}
	if got := composeWSLENV("::A::", []string{"A", "B"}); got != "A:B" {
		t.Fatalf("stray separators must be dropped, got %q", got)
	}
}

func TestWSLRunArgsMirrorsRunOptions(t *testing.T) {
	args := wslRunArgs(RunOptions{Name: "lab", Image: "sdr_light", Command: "gqrx", Workspace: `C:\ws`, FlakeRef: "github:a/b", Rebuild: true, Pure: true, Lazy: true, Isolate: true, CreateOnly: true})
	want := []string{"run", "--engine", "nix", "-i", "sdr_light", "-n", "lab", "-e", "gqrx", "--workspace", `C:\ws`, "--flake", "github:a/b", "--rebuild", "--pure", "--lazy", "--isolate", "--create-only"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("args = %q, want %q", args, want)
	}
	minimal := wslRunArgs(RunOptions{Name: "lab", Image: "rfid", Workspace: "none"})
	if want := []string{"run", "--engine", "nix", "-i", "rfid", "-n", "lab", "--no-workspace"}; !reflect.DeepEqual(minimal, want) {
		t.Fatalf("minimal = %q, want %q", minimal, want)
	}
	if got := wslRunArgs(RunOptions{Name: "lab", Image: "rfid"}); len(got) != 7 {
		t.Fatalf("default workspace must add no flag: %q", got)
	}
}

func TestWSLBridgeArgsMakesEngineExplicit(t *testing.T) {
	got := WSLBridgeArgs([]string{"run", "-i", "sdr_light", "-n", "lab"}, true)
	if want := []string{"--engine", "nix", "run", "-i", "sdr_light", "-n", "lab"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("engine command = %q, want %q", got, want)
	}
	for _, args := range [][]string{{"--engine", "nix", "run"}, {"run", "--engine=nix"}} {
		if got := WSLBridgeArgs(args, true); !reflect.DeepEqual(got, args) {
			t.Fatalf("an explicit engine must be kept as is: %q -> %q", args, got)
		}
	}
	if got := WSLBridgeArgs([]string{"nix", "list"}, false); !reflect.DeepEqual(got, []string{"nix", "list"}) {
		t.Fatalf("nix group commands need no engine flag: %q", got)
	}
}

func TestExtractJSONSkipsNotices(t *testing.T) {
	out := "[i] Some notice with [brackets]\nCommand \"list\" is deprecated\n[\n  {\"name\": \"lab\"}\n]\n"
	if got := extractJSON(out); got != "[\n  {\"name\": \"lab\"}\n]" {
		t.Fatalf("array = %q", got)
	}
	if got := extractJSON("[i] nothing built\n{}\n"); got != "{}" {
		t.Fatalf("empty object = %q", got)
	}
	if got := extractJSON(`{"rules": []}`); got != `{"rules": []}` {
		t.Fatalf("compact object = %q", got)
	}
	if got := extractJSON("plain text"); got != "plain text" {
		t.Fatalf("no JSON must pass through for the error message, got %q", got)
	}
}

func TestLastBoxedMessageRejoinsTheErrorBox(t *testing.T) {
	out := "[i] Installing x into environment 'lab' ...\n" +
		"┌──────────────────────────────┐\n" +
		"│  ✗ Error                     │\n" +
		"├──────────────────────────────┤\n" +
		"│ failed to install x: no      │\n" +
		"│ package named \"x\" in the     │\n" +
		"│ flake (check the name)       │\n" +
		"└──────────────────────────────┘\n\n"
	want := `failed to install x: no package named "x" in the flake (check the name)`
	if got := lastNonEmptyLine(out); got != want {
		t.Fatalf("boxed message = %q, want %q", got, want)
	}
	if got := lastNonEmptyLine("plain\nlast line\n"); got != "last line" {
		t.Fatalf("no box: %q", got)
	}
	if got := lastBoxedMessage("└──┘\n"); got != "" {
		t.Fatalf("a border without a box body yields nothing, got %q", got)
	}
}

func TestStripANSIAndLastLine(t *testing.T) {
	colored := "\x1b[38;5;208mwarning\x1b[0m\n\x1b[31merror: build failed\x1b[0m\n\n"
	if got := stripANSI(colored); strings.Contains(got, "\x1b") {
		t.Fatalf("escapes left: %q", got)
	}
	if got := lastNonEmptyLine(stripANSI(colored)); got != "error: build failed" {
		t.Fatalf("last line = %q", got)
	}
}

func TestWSLWorkspaceHintOnlyInsideWSL(t *testing.T) {
	t.Setenv("WSL_DISTRO_NAME", "")
	if got := wslWorkspaceHint("/home/u/rfswift-workspace/lab"); got != "" {
		t.Fatalf("outside WSL there is no hint, got %q", got)
	}
	t.Setenv("WSL_DISTRO_NAME", "Ubuntu-24.04")
	got := wslWorkspaceHint("/home/u/rfswift-workspace/lab")
	if !strings.Contains(got, `\\wsl.localhost\Ubuntu-24.04\home\u\rfswift-workspace\lab`) {
		t.Fatalf("hint = %q", got)
	}
	if got := wslWorkspaceHint("/mnt/c/Users/u/lab"); got != "" {
		t.Fatalf("a workspace on a Windows drive needs no hint, got %q", got)
	}
	if got := wslWorkspaceHint(""); got != "" {
		t.Fatalf("no workspace, no hint, got %q", got)
	}
}

func TestRFSwiftReleaseTagsPrefersRequested(t *testing.T) {
	if got := rfswiftReleaseTags("4.0.0"); !reflect.DeepEqual(got, []string{"v4.0.0"}) {
		t.Fatalf("requested = %q", got)
	}
	if got := rfswiftReleaseTags("v4.0.1-dev"); !reflect.DeepEqual(got, []string{"v4.0.1-dev"}) {
		t.Fatalf("requested with prefix = %q", got)
	}
}

func TestWSLReadyErrorGuidance(t *testing.T) {
	st := ParseProbeForTest("Ubuntu", false, true)
	err := WSLReadyError(st, nil)
	if err == nil || !strings.Contains(err.Error(), "rfswift nix wsl setup --distro Ubuntu") || !strings.Contains(err.Error(), "nix") {
		t.Fatalf("not-ready error = %v", err)
	}
	if WSLReadyError(ParseProbeForTest("Ubuntu", true, true), nil) != nil {
		t.Fatal("a ready backend has no error")
	}
}
