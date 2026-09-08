package dock

import (
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
