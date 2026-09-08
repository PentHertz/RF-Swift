package cli

import (
	"encoding/json"
	"testing"
)

func TestRegressionRemoteCreationPreservesIsolation(t *testing.T) {
	var req agentCreate
	if err := json.Unmarshal([]byte(`{"name":"review","engine":"nix","isolate":true}`), &req); err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(b, &fields); err != nil {
		t.Fatal(err)
	}
	if fields["Isolate"] != true && fields["isolate"] != true {
		t.Fatal("remote agent creation schema silently discards isolate=true")
	}
	if opts := agentNixCreateOptions(req); !opts.Isolate || !opts.CreateOnly {
		t.Fatalf("agent did not forward isolation to Nix: %+v", opts)
	}
}
