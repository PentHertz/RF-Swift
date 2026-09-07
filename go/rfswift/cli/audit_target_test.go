package cli

import (
	"testing"

	rfnix "penthertz/rfswift/nix"
)

// A name that is not a registered environment is audited as a flake
// environment of that name on the default flake, with the report under the
// environments tree. (A registered environment resolves to its image and
// pinned flake instead; that path needs an environment on disk.)
func TestNixAuditTargetFallsBackToFlakeEnvironment(t *testing.T) {
	flakeRef, image, out := nixAuditTarget("rfswift-no-such-environment-xyz")
	if image != "rfswift-no-such-environment-xyz" {
		t.Fatalf("image = %q", image)
	}
	if flakeRef != rfnix.ResolveFlakeRef("") {
		t.Fatalf("flakeRef = %q", flakeRef)
	}
	if out != rfnix.EnvReportDir("rfswift-no-such-environment-xyz") {
		t.Fatalf("out = %q", out)
	}
}
