package nix

import (
	"reflect"
	"strings"
	"testing"
)

func TestAncestorDirs(t *testing.T) {
	if got := ancestorDirs("/Users/u/.rfswift/nix/environments/e"); !reflect.DeepEqual(got, []string{"/Users", "/Users/u", "/Users/u/.rfswift", "/Users/u/.rfswift/nix", "/Users/u/.rfswift/nix/environments"}) {
		t.Fatalf("ancestorDirs = %q", got)
	}
	if got := ancestorDirs("/x"); len(got) != 0 {
		t.Fatalf("ancestorDirs(/x) = %q", got)
	}
}

func TestDarwinSandboxProfileAncestorsAreMetadataOnly(t *testing.T) {
	t.Setenv("HOME", "/Users/u")
	env := &Environment{Name: "e", Workspace: "/Users/u/rfswift-workspace/e", Isolate: true}
	p := darwinSandboxProfile(env, "", "/Users/u/.rfswift/nix/environments/e/jail-home")
	for _, want := range []string{
		`(deny file-read* file-write* (subpath "/Users"))`,
		`(allow file-read-metadata (literal "/Users"))`,
		`(allow file-read-metadata (literal "/Users/u"))`,
		`(allow file-read-metadata (literal "/Users/u/.rfswift/nix/environments"))`,
		`(allow file-read* file-write* (subpath "/Users/u/.rfswift/nix/environments/e/jail-home"))`,
	} {
		if !strings.Contains(p, want) {
			t.Errorf("profile lacks %s\n%s", want, p)
		}
	}
	// Ancestors never get more than metadata, and each appears once.
	if strings.Contains(p, `(allow file-read* (subpath "/Users/u"))`) || strings.Count(p, `(literal "/Users")`) != 1 {
		t.Fatalf("ancestor rules too broad or duplicated:\n%s", p)
	}
	// The deny must come first so the literal allows win (SBPL is last-match-wins).
	if strings.Index(p, `(deny file-read*`) > strings.Index(p, `(allow file-read-metadata`) {
		t.Fatal("ancestor allows are emitted before the deny")
	}
}
