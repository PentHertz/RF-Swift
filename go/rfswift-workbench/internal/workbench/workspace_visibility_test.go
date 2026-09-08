package workbench

import (
	"strings"
	"testing"
)

// The mission workspace is taken from the explicit field first, then from a
// container-style mount, then from the bare path older callers pass.
func TestMissionWorkspaceResolution(t *testing.T) {
	ws := t.TempDir()
	cases := []struct {
		name    string
		mission Mission
		want    string
		wantErr string
	}{
		{"explicit field", Mission{ID: "lab", Engine: "nix", Workspace: ws, Mounts: []string{ws + " -> /workspace (rw, jail)"}}, ws, ""},
		{"container mount", Mission{ID: "lab", Engine: "docker", Mounts: []string{ws + " -> /workspace (rw)"}}, ws, ""},
		{"bare nix path", Mission{ID: "lab", Engine: "nix", Mounts: []string{ws}}, ws, ""},
		{"labelled native path", Mission{ID: "lab", Engine: "nix", Mounts: []string{ws + " (workspace, native path)"}}, ws, ""},
		{"no workspace", Mission{ID: "lab", Engine: "nix", Mounts: []string{"none (created without a workspace)"}}, "", "without a workspace"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := testApp(t, &fakeEngine{targets: []Mission{tc.mission}})
			if err := a.store.SaveMission(a.ws, Mission{ID: "lab", Engine: tc.mission.Engine}); err != nil {
				t.Fatal(err)
			}
			got, err := a.missionWorkspace("lab")
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("missionWorkspace = %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}
