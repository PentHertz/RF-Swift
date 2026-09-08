package workbench

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMCPReadAuditReturnsStoredGroundTruth(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := store.SaveMission("default", Mission{ID: "rfid"}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(store.missionDir("default", "rfid"), "audits", "latest.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"vulnerabilities":[{"cve":"CVE-2026-4242","severity":"high","package":"librf"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s := &mcpServer{store: store, eng: &fakeEngine{}, opts: MCPOptions{Workspace: "default", Mission: "rfid"}}
	got, err := s.call("read_audit", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	root, ok := got.(map[string]any)
	if !ok || root["vulnerabilities"] == nil {
		t.Fatalf("ground-truth audit was not returned: %#v", got)
	}
}

func TestMCPReadAuditRequiresScannerReport(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := store.SaveMission("default", Mission{ID: "rfid"}); err != nil {
		t.Fatal(err)
	}
	s := &mcpServer{store: store, eng: &fakeEngine{}, opts: MCPOptions{Workspace: "default", Mission: "rfid"}}
	if _, err := s.call("read_audit", map[string]any{}); err == nil {
		t.Fatal("expected missing scanner report error")
	}
}
