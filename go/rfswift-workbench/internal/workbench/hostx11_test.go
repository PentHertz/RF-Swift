package workbench

import (
	"testing"
	"time"
)

func TestEnsureMissionHostServicesRechecksOnlyAfterInterval(t *testing.T) {
	a := &App{eng: &fakeEngine{}} // not a local engine: the checks themselves are no-ops
	a.ensureMissionHostServices("m1")
	first, ok := a.hostServicesAt.Load("m1")
	if !ok {
		t.Fatal("first call did not record the check")
	}
	a.ensureMissionHostServices("m1")
	if again, _ := a.hostServicesAt.Load("m1"); again != first {
		t.Fatal("a call within the recheck interval ran the checks again")
	}
	stale := time.Now().Add(-2 * hostServicesRecheck)
	a.hostServicesAt.Store("m1", stale)
	a.ensureMissionHostServices("m1")
	if at, _ := a.hostServicesAt.Load("m1"); !at.(time.Time).After(stale) {
		t.Fatal("an expired check was not run again")
	}
}
