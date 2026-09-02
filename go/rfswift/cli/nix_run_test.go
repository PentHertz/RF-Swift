package cli

import "testing"

// `rfswift nix run <image> <tool>` takes a command name; a name that is one
// of the image's packages is an attribute already, and an unknown image is
// left to nix to complain about. (The main-program lookup for other names
// needs a nix evaluation and is exercised on a provisioned host.)
func TestResolveCatalogToolAttrPassesThroughKnownNames(t *testing.T) {
	if got := resolveCatalogToolAttr("path:/nowhere", "sdr_light", "gqrx"); got != "gqrx" {
		t.Fatalf("a package of the image is its own attribute, got %q", got)
	}
	if got := resolveCatalogToolAttr("path:/nowhere", "no-such-image", "sdrpp"); got != "sdrpp" {
		t.Fatalf("unknown image must pass the name through, got %q", got)
	}
}
