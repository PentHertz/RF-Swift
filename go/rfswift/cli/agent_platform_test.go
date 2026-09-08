package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestRemoteTerminalPreservesSplitUnicode(t *testing.T) {
	id := "unicode-regression"
	s := &remotePTY{output: []byte{0xe2, 0x82}}
	remotePTYs.Lock()
	remotePTYs.sessions[id] = s
	remotePTYs.Unlock()
	t.Cleanup(func() { remotePTYs.Lock(); delete(remotePTYs.sessions, id); remotePTYs.Unlock() })
	first, err := agentTerminalRead(id)
	if err != nil || first["data"] != "" {
		t.Fatalf("partial character emitted: %v %v", first, err)
	}
	s.mu.Lock()
	s.output = append(s.output, 0xac)
	s.closed = true
	s.mu.Unlock()
	last, err := agentTerminalRead(id)
	if err != nil || last["data"] != "€" || last["closed"] != true {
		t.Fatalf("unicode corrupted: %v %v", last, err)
	}
	remotePTYs.Lock()
	remaining := remotePTYs.sessions[id]
	remotePTYs.Unlock()
	if remaining != nil {
		t.Fatal("closed session leaked")
	}
}

func TestArtifactWorkspacePaths(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "capture with spaces.txt")
	if err := os.WriteFile(file, []byte("capture"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := safeAgentArtifactPath(root, "capture with spaces.txt"); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"../outside", "/outside", "."} {
		if _, err := safeAgentArtifactPath(root, rel); err == nil {
			t.Fatalf("accepted unsafe path %q", rel)
		}
	}
	if runtime.GOOS == "windows" {
		for _, rel := range []string{`C:outside`, `C:\outside`, `\\server\share\outside`, `NUL`} {
			if _, err := safeAgentArtifactPath(root, rel); err == nil {
				t.Fatalf("accepted unsafe Windows path %q", rel)
			}
		}
	}
	alias := filepath.Join(t.TempDir(), "workspace-link")
	if err := os.Symlink(root, alias); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	got, err := safeAgentArtifactPath(alias, "capture with spaces.txt")
	want, _ := filepath.EvalSymlinks(file)
	if err != nil || got != want {
		t.Fatalf("symlinked workspace failed: %q %v", got, err)
	}
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	if _, err := safeAgentArtifactPath(root, "escape"); err == nil {
		t.Fatal("escaping link accepted")
	}
}
