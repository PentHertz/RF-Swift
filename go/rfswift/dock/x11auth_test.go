package dock

import (
	"os"
	"path/filepath"
	"testing"
)

func TestForwardedXAuthority(t *testing.T) {
	auth := filepath.Join(t.TempDir(), "authority")
	if err := os.WriteFile(auth, []byte("cookie"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XAUTHORITY", auth)
	got, ok := forwardedXAuthority("DISPLAY=localhost:12.0")
	if !ok || got != auth {
		t.Fatalf("got (%q,%v), want (%q,true)", got, ok, auth)
	}
	if _, ok := forwardedXAuthority(":0"); ok {
		t.Fatal("local display must not mount an SSH authority file")
	}
}

func TestAddForwardedXAuthority(t *testing.T) {
	auth := filepath.Join(t.TempDir(), "authority")
	if err := os.WriteFile(auth, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XAUTHORITY", auth)
	binds, env := addForwardedXAuthority("localhost:10.0", nil, nil)
	if len(binds) != 1 || binds[0] != auth+":"+containerXAuthority+":ro" {
		t.Fatalf("unexpected binds: %v", binds)
	}
	if len(env) != 1 || env[0] != "XAUTHORITY="+containerXAuthority {
		t.Fatalf("unexpected env: %v", env)
	}
}
