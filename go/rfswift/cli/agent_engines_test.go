package cli

import (
	"encoding/json"
	"testing"
)

// The engine report must always be answerable, with or without a daemon on
// the test host, and say something about Nix either way.
func TestAgentEngineReportIsAlwaysAnswerable(t *testing.T) {
	report := agentEngineReport()
	if report.OS == "" {
		t.Error("report lacks the OS")
	}
	if report.Engines == nil {
		t.Error("engines must be an array, not null, for the Workbench")
	}
	for _, e := range report.Engines {
		if e.Name == "" || e.Label == "" || e.State == "" {
			t.Errorf("engine entry incomplete: %+v", e)
		}
		if !e.Available && e.State != "not installed" {
			t.Errorf("an unavailable engine must say so: %+v", e)
		}
		if e.State == "unreachable" && e.Detail == "" {
			t.Errorf("an unreachable engine must carry a reason: %+v", e)
		}
	}
	if report.Nix.Detail == "" {
		t.Error("nix state lacks a detail")
	}
	if _, err := json.Marshal(report); err != nil {
		t.Errorf("report does not marshal: %v", err)
	}
	// Listing targets must not fail just because no engine is reachable.
	if _, err := agentListTargets(); err != nil {
		t.Errorf("targets.list failed without an engine: %v", err)
	}
}
