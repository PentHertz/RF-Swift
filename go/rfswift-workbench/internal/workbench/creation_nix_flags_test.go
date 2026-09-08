package workbench

import "testing"

// capturingEngine records the request the App hands to the engine.
type capturingEngine struct {
	fakeEngine
	got MissionCreate
}

func (e *capturingEngine) Create(req MissionCreate) (Mission, error) {
	e.got = req
	return e.fakeEngine.Create(req)
}

// The create dialog's Nix-only switches (lazy, pure, isolate, flake) must not
// reach a container engine: the remote agent refuses "isolate" on a container
// target, so a Docker mission created right after a jailed Nix environment
// failed with "isolate is supported only for Nix targets" until the app was
// restarted.
func TestCreateMissionDropsNixOnlySwitchesForContainers(t *testing.T) {
	e := &capturingEngine{}
	a := testApp(t, e)
	if _, err := a.CreateMission(MissionCreate{Name: "reader", Engine: "docker", Image: "rfid", Lazy: true, Pure: true, Isolate: true, FlakeRef: "github:PentHertz/RF-Swift-nix"}); err != nil {
		t.Fatal(err)
	}
	if e.got.Lazy || e.got.Pure || e.got.Isolate || e.got.FlakeRef != "" {
		t.Fatalf("Nix-only switches reached the container engine: %+v", e.got)
	}
	if _, err := a.CreateMission(MissionCreate{Name: "jail", Engine: "nix", Image: "rfid", Isolate: true, Lazy: true}); err != nil {
		t.Fatal(err)
	}
	if !e.got.Isolate || !e.got.Lazy {
		t.Fatalf("Nix switches must survive for a Nix environment: %+v", e.got)
	}
}
