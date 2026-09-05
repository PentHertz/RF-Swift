package workbench

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDeleteMissionCompletelyWorkspaceOption(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // no real Nix environments: removal is a no-op
	for _, deleteWs := range []bool{false, true} {
		ws := filepath.Join(t.TempDir(), "lab")
		if err := os.MkdirAll(ws, 0o755); err != nil {
			t.Fatal(err)
		}
		a := testApp(t, &fakeEngine{targets: []Mission{{ID: "lab", Engine: "nix", Workspace: ws}}})
		if err := a.store.SaveMission(a.ws, Mission{ID: "lab", Engine: "nix"}); err != nil {
			t.Fatal(err)
		}
		if err := a.DeleteMissionCompletely("lab", "nix", deleteWs); err != nil {
			t.Fatalf("deleteWs=%v: %v", deleteWs, err)
		}
		_, err := os.Stat(ws)
		if deleteWs && !os.IsNotExist(err) {
			t.Fatalf("workspace kept although deletion was requested: %v", err)
		}
		if !deleteWs && err != nil {
			t.Fatalf("workspace removed without being asked: %v", err)
		}
		if _, err := os.Stat(a.store.missionDir(a.ws, "lab")); !os.IsNotExist(err) {
			t.Fatal("mission data kept")
		}
	}
	// A target without a workspace cannot honour the request.
	a := testApp(t, &fakeEngine{targets: []Mission{{ID: "lab", Engine: "nix"}}})
	_ = a.store.SaveMission(a.ws, Mission{ID: "lab", Engine: "nix"})
	if err := a.DeleteMissionCompletely("lab", "nix", true); err == nil {
		t.Fatal("expected an error for a target without a workspace")
	}
}
