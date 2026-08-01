/* This code is part of RF Swift by @Penthertz
*  Tests for detecting official-but-outdated RF-Swift container images.
 */

package rfutils

import (
	"testing"

	common "penthertz/rfswift/common"
)

func TestOfficialImageCodename(t *testing.T) {
	cases := []struct {
		image    string
		codename string
		official bool
	}{
		{"penthertz/rfswift_noble", "noble", true},
		{"penthertz/rfswift_noble:sdr_light", "noble", true},
		{"penthertz/rfswift_resolute:latest", "resolute", true},
		{"penthertz/rfswiftdev_resolute:cache_wifi", "resolute", true},
		{"penthertz/rfswift_noble@sha256:deadbeef", "noble", true},
		{"docker.io/penthertz/rfswift_noble:x", "noble", true},
		{"myrfswift:latest", "", false},      // custom local image
		{"ubuntu:24.04", "", false},          // unrelated
		{"someone/rfswift_noble", "", false}, // wrong namespace
		{"penthertz/rfswift_", "", false},    // empty codename
		{"penthertz/somethingelse:t", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		cn, official := officialImageCodename(c.image)
		if cn != c.codename || official != c.official {
			t.Errorf("officialImageCodename(%q) = (%q,%v), want (%q,%v)",
				c.image, cn, official, c.codename, c.official)
		}
	}
}

func TestIsOutdatedOfficialImage(t *testing.T) {
	if common.CurrentImageCodename != "resolute" {
		t.Fatalf("test assumes current codename 'resolute', got %q", common.CurrentImageCodename)
	}
	outdated := []string{
		"penthertz/rfswift_noble",
		"penthertz/rfswift_noble:sdr_light",
		"penthertz/rfswiftdev_noble:cache",
		"docker.io/penthertz/rfswift_bookworm:latest",
	}
	current := []string{
		"penthertz/rfswift_resolute",
		"penthertz/rfswift_resolute:sdr_light",
		"penthertz/rfswiftdev_resolute:cache",
		"myrfswift:latest", // not official -> never outdated
		"ubuntu:24.04",
		"",
	}
	for _, img := range outdated {
		if !IsOutdatedOfficialImage(img) {
			t.Errorf("IsOutdatedOfficialImage(%q) = false, want true", img)
		}
	}
	for _, img := range current {
		if IsOutdatedOfficialImage(img) {
			t.Errorf("IsOutdatedOfficialImage(%q) = true, want false", img)
		}
	}
}
