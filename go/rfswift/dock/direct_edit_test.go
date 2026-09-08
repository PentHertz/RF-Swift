package dock

import (
	"os"
	"runtime"
	"testing"
)

func TestConfigEditModeDecidesRootAndPath(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("direct edit is a Linux Docker feature")
	}
	if !(&DockerEngine{}).SupportsDirectConfigEdit() {
		t.Fatal("Docker on Linux supports the direct configuration edit")
	}
	SetPreferredEngine("docker")
	defer SetConfigEditMode(EditModeAuto)
	SetConfigEditMode(EditModeAuto)
	if !useDirectConfigEdit() {
		t.Fatal("Docker rewrites the container's files by default")
	}
	if os.Geteuid() != 0 && !NeedsRootForConfigEdit() {
		t.Fatal("a plain user must be asked to elevate for the file edit")
	}
	SetConfigEditMode(EditModeDirect)
	if !useDirectConfigEdit() {
		t.Fatal("direct mode must edit the files")
	}
	SetConfigEditMode(EditModeRecreate)
	if useDirectConfigEdit() || NeedsRootForConfigEdit() {
		t.Fatal("recreate mode never edits the files")
	}
	if (ConfigChange{Mode: "direct"}).EditMode() != EditModeDirect || (ConfigChange{}).EditMode() != EditModeAuto || (ConfigChange{Mode: "recreate"}).EditMode() != EditModeRecreate {
		t.Fatal("mode strings")
	}
	if err := ApplyConfigChange(ConfigChange{Kind: "volume"}); err == nil {
		t.Error("a change without a container must be refused")
	}
	if err := ApplyConfigChangeJSON("{not json"); err == nil {
		t.Error("invalid JSON must be refused")
	}
}
