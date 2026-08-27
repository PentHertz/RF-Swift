package nix

import (
	"errors"
	"strings"
	"testing"
)

func TestInstallFailureReasonPlatform(t *testing.T) {
	stderr := `building ...
       error: Refusing to evaluate package 'libhydrasdr-unstable' in /nix/store/x/pkgs/libhydrasdr.nix:25 because it is not available on the requested hostPlatform:
         hostPlatform.system = "aarch64-darwin"`
	got := installFailureReason("libhydrasdr", "aarch64-darwin", stderr, errors.New("exit status 1"))
	if !strings.Contains(got, "not available on aarch64-darwin") || !strings.Contains(got, "Lima") {
		t.Fatalf("platform reason not surfaced: %q", got)
	}
	if strings.Contains(got, "exit status 1") {
		t.Fatalf("should replace the opaque exit code: %q", got)
	}
}

func TestInstallFailureReasonMissingAttr(t *testing.T) {
	stderr := `error: flake 'x' does not provide attribute 'legacyPackages.aarch64-darwin.libydrasdr'`
	got := installFailureReason("libydrasdr", "aarch64-darwin", stderr, errors.New("exit status 1"))
	if !strings.Contains(got, "no package named") || !strings.Contains(got, "nix search") {
		t.Fatalf("missing-attr reason not surfaced: %q", got)
	}
}

func TestInstallFailureReasonFallsBackToNixError(t *testing.T) {
	stderr := "building foo...\nerror: builder for '/nix/store/x.drv' failed with exit code 2"
	got := installFailureReason("foo", "aarch64-darwin", stderr, errors.New("exit status 1"))
	if !strings.HasPrefix(got, "error: builder for") {
		t.Fatalf("expected the last nix error line, got: %q", got)
	}
}

func TestInstallFailureReasonNoStderr(t *testing.T) {
	got := installFailureReason("foo", "aarch64-darwin", "", errors.New("exit status 1"))
	if got != "exit status 1" {
		t.Fatalf("expected raw error when no stderr, got: %q", got)
	}
}
