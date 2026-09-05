package workbench

import (
	"testing"
)

type blockingCreationEngine struct {
	fakeEngine
	started chan struct{}
	resume  chan struct{}
}

func (e *blockingCreationEngine) Create(req MissionCreate) (Mission, error) {
	close(e.started)
	<-e.resume
	return Mission{ID: req.Name, Title: req.Title, Engine: req.Engine}, nil
}

func TestRegressionCreationKeepsOriginalWorkspace(t *testing.T) {
	e := &blockingCreationEngine{started: make(chan struct{}), resume: make(chan struct{})}
	a := testApp(t, e)
	done := make(chan error, 1)
	go func() {
		_, err := a.CreateMission(MissionCreate{Name: "new-mission", Engine: "nix"})
		done <- err
	}()
	<-e.started
	if err := a.OpenWorkspace("another-client"); err != nil {
		t.Fatal(err)
	}
	close(e.resume)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	original, err := a.store.ListMissions("default")
	if err != nil {
		t.Fatal(err)
	}
	other, err := a.store.ListMissions("another-client")
	if err != nil {
		t.Fatal(err)
	}
	if len(original) != 1 || len(other) != 0 {
		t.Fatalf("mission saved in wrong workspace: original=%v other=%v", original, other)
	}
}

func TestCreationPreparationKeepsOriginalScope(t *testing.T) {
	e := &blockingCreationEngine{started: make(chan struct{}), resume: make(chan struct{})}
	close(e.resume)
	a := testApp(t, e)
	if err := a.BeginMissionCreation("prepared"); err != nil {
		t.Fatal(err)
	}
	defer a.FinishMissionCreation("prepared")
	if err := a.OpenWorkspace("other"); err != nil {
		t.Fatal(err)
	}
	a.setEngine(&fakeEngine{})
	if _, err := a.CreateMission(MissionCreate{Name: "prepared", Engine: "nix"}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-e.started:
	default:
		t.Fatal("creation used the newly selected engine")
	}
	got, err := a.store.ListMissions("default")
	if err != nil || len(got) != 1 {
		t.Fatalf("creation left its original project: %v %v", got, err)
	}
}
