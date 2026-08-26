package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkspaceRecapUsesResolvedEnvironmentName(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, "rfswift-workspace", "testerverse")
	if got := wsLabel("testerverse", ""); !strings.Contains(got, want) {
		t.Fatalf("workspace recap %q does not contain %q", got, want)
	}
}

func TestWorkspaceRecapDescribesModes(t *testing.T) {
	if got := wsLabel("lab", "none"); !strings.Contains(got, "no workspace") {
		t.Fatalf("disabled label is ambiguous: %q", got)
	}
	if got := wsLabel("lab", "relative/path"); !strings.HasPrefix(got, "custom (") {
		t.Fatalf("custom label is ambiguous: %q", got)
	}
}
