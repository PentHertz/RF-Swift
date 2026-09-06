package workbench

import (
	"testing"

	"penthertz/rfswift/remote"
)

func TestEngineDoctorShowsTheAgentHost(t *testing.T) {
	report := remote.EngineReport{Host: "lab", OS: "linux", Engines: []remote.EngineState{
		{Name: "docker", Label: "Docker", Available: true, Running: true, State: "running", Active: true, Socket: "/var/run/docker.sock", Containers: 3},
		{Name: "podman", Label: "Podman", Available: true, Running: false, State: "unreachable", Detail: "permission denied"},
		{Name: "lima", Label: "Lima", Available: false, State: "not installed"},
	}, Nix: remote.NixState{Available: true, Version: "nix (Nix) 2.35.2", Detail: "nix (Nix) 2.35.2"}}

	rows := engineStatusFromReport(report)
	if len(rows) != 2 {
		t.Fatalf("expected the two installed engines, got %+v", rows)
	}
	for _, r := range rows {
		if !r.Remote {
			t.Errorf("%s must be flagged remote so the doctor does not offer to manage it", r.Name)
		}
	}
	if rows[0].Containers != 3 || rows[0].State != "running" || !rows[0].Active {
		t.Errorf("docker row: %+v", rows[0])
	}
	if rows[1].State != "unreachable" || rows[1].Detail != "permission denied" {
		t.Errorf("podman row must carry the agent's reason: %+v", rows[1])
	}

	nix := nixStatusFromReport(report)
	if nix.Host != "agent" || !nix.Ready || nix.NixVersion != "nix (Nix) 2.35.2" || nix.Distro != "lab" {
		t.Errorf("nix status: %+v", nix)
	}
	report.Nix = remote.NixState{Detail: "nix is not installed on the agent host"}
	nix = nixStatusFromReport(report)
	if nix.Ready || len(nix.Missing) != 1 || nix.Detail == "" {
		t.Errorf("missing nix: %+v", nix)
	}
}
