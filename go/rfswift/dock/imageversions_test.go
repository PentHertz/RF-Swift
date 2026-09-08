package dock

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestImageVersionEntriesOrderAndRefs(t *testing.T) {
	day := func(d int) time.Time { return time.Date(2026, 9, d, 0, 0, 0, 0, time.UTC) }
	entries := imageVersionEntries("penthertz/rfswift_resolute", "sdr_full", []VersionInfo{
		{Version: "0.1.1", Digest: "sha256:b", Date: day(2)},
		{Version: "latest", Digest: "sha256:c", Date: day(3)},
		{Version: "0.1.1", Digest: "sha256:b", Date: day(2)}, // duplicate from a second repo
		{Version: "0.0.9", Digest: "sha256:a", Date: day(1)},
		{Version: "0.1.10", Digest: "sha256:d", Date: day(4)},
	})
	got := make([]string, 0, len(entries))
	for _, e := range entries {
		got = append(got, e.Version+"="+e.Ref)
	}
	want := []string{
		"latest=penthertz/rfswift_resolute:sdr_full",
		"0.1.10=penthertz/rfswift_resolute:sdr_full_0.1.10",
		"0.1.1=penthertz/rfswift_resolute:sdr_full_0.1.1",
		"0.0.9=penthertz/rfswift_resolute:sdr_full_0.0.9",
	}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("entries = %v, want %v", got, want)
	}
	if !entries[1].Newest || entries[0].Newest || entries[2].Newest {
		t.Fatalf("the highest release must be the only one flagged newest: %+v", entries)
	}
	if entries[0].Digest != "sha256:c" || !entries[0].Date.Equal(day(3)) {
		t.Fatalf("latest lost its digest or date: %+v", entries[0])
	}
}

func TestImageVersionsRejectsCustomImages(t *testing.T) {
	for _, name := range []string{"myregistry.example/team/tools:latest", "localhost/rfswift-imported:20260907"} {
		if _, err := ImageVersions(name); err == nil || !strings.Contains(err.Error(), "not an official") {
			t.Errorf("%s: expected the custom-image error, got %v", name, err)
		}
	}
	if _, err := ImageVersions(""); err == nil {
		t.Error("empty image name accepted")
	}
}

func TestLocalImageVersionFromTagAndDigest(t *testing.T) {
	published := ImageVersionMap{"sdr_full": {
		{Version: "0.0.9", Digest: "sha256:old"},
		{Version: "latest", Digest: "sha256:new"},
		{Version: "0.1.1", Digest: "sha256:new"},
	}}
	// A pinned tag names its release without touching the engine (nil client).
	version, latest, pinned := localImageVersion(context.Background(), nil, "penthertz/rfswift_resolute", "sdr_full_0.0.9", published)
	if version != "0.0.9" || latest != "0.1.1" || !pinned {
		t.Fatalf("pinned tag: got %q latest %q pinned %v", version, latest, pinned)
	}
	// A rolling tag is identified by digest, never by the "latest" alias.
	if got := matchPublishedVersion([]string{"sha256:new"}, published["sdr_full"]); got != "0.1.1" {
		t.Fatalf("digest match = %q, want 0.1.1", got)
	}
	if got := matchPublishedVersion([]string{"sha256:unlisted"}, published["sdr_full"]); got != "" {
		t.Fatalf("unlisted digest matched %q", got)
	}
	if got := newestPublished(nil); got != "" {
		t.Fatalf("newest of nothing = %q", got)
	}
}

// The Workbench frontend reads these keys; Wails and the agent both encode
// with encoding/json, so the names must be tagged, not Go-cased.
func TestImageAvailabilityJSONKeysMatchWorkbench(t *testing.T) {
	raw, err := json.Marshal(ImageAvailability{Resolved: "x", Present: true, Version: "0.1.1", Latest: "0.1.2", Pinned: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"resolved"`, `"present"`, `"updateAvailable"`, `"custom"`, `"version"`, `"latest"`, `"pinned"`} {
		if !strings.Contains(string(raw), key) {
			t.Errorf("missing key %s in %s", key, raw)
		}
	}
}
