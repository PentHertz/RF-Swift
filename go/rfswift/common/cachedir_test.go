package common

import (
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCacheDirByPlatform(t *testing.T) {
	t.Setenv("SUDO_USER", "")
	base, err := os.UserCacheDir()
	if err != nil {
		t.Skipf("no user cache directory here: %v", err)
	}
	if got, want := CacheDirByPlatform(), filepath.Join(base, "rfswift"); got != want {
		t.Fatalf("CacheDirByPlatform() = %q, want %q", got, want)
	}
}

func TestCacheDirByPlatformUnderSudoUsesInvokingUser(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no sudo on Windows")
	}
	me, err := user.Current()
	if err != nil {
		t.Skip(err)
	}
	t.Setenv("SUDO_USER", me.Username)
	t.Setenv("HOME", t.TempDir()) // root's home under sudo
	t.Setenv("XDG_CACHE_HOME", "")
	want := filepath.Join(me.HomeDir, ".cache", "rfswift")
	if runtime.GOOS == "darwin" {
		want = filepath.Join(me.HomeDir, "Library", "Caches", "rfswift")
	}
	if got := CacheDirByPlatform(); got != want {
		t.Fatalf("CacheDirByPlatform() under sudo = %q, want %q", got, want)
	}
}

func TestInvokingHomeFollowsSudoUser(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no sudo on Windows")
	}
	u, err := user.Current()
	if err != nil || u.HomeDir == "" {
		t.Skip("no current user")
	}
	t.Setenv("HOME", "/root")
	t.Setenv("SUDO_USER", u.Username)
	if got := InvokingHome(); got != u.HomeDir {
		t.Fatalf("under sudo, InvokingHome = %q, want the caller's %q", got, u.HomeDir)
	}
	t.Setenv("SUDO_USER", "")
	t.Setenv("HOME", "/tmp/somewhere")
	if got := InvokingHome(); got != "/tmp/somewhere" {
		t.Fatalf("without sudo, InvokingHome = %q, want $HOME", got)
	}
}
