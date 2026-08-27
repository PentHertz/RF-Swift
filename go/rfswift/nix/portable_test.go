package nix

import (
	"encoding/json"
	"testing"
)

func TestPortableManifestExtrasCompatibility(t *testing.T) {
	want := exportManifest{
		Version:    2,
		StorePath:  "/nix/store/base-environment",
		ExtrasPath: "/nix/store/installed-tools-profile",
		Env:        Environment{Name: "radio", Image: "sdr_light"},
	}
	data, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	var got exportManifest
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.ExtrasPath != want.ExtrasPath {
		t.Fatalf("extras path was not preserved: got %q", got.ExtrasPath)
	}

	// Archives created before extras support have no extrasPath and remain valid.
	got = exportManifest{}
	if err := json.Unmarshal([]byte(`{"version":1,"storePath":"/nix/store/base","env":{"name":"legacy"}}`), &got); err != nil {
		t.Fatal(err)
	}
	if got.ExtrasPath != "" {
		t.Fatalf("legacy manifest unexpectedly acquired extras path %q", got.ExtrasPath)
	}
}
