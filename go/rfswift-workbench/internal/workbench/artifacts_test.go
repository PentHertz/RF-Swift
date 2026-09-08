package workbench

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func artifactTestApp(t *testing.T) (*App, string) {
	t.Helper()
	workspace := t.TempDir()
	a := testApp(t, &fakeEngine{targets: []Mission{{ID: "lab", Engine: "nix", Mounts: []string{workspace}}}})
	if err := a.store.SaveMission(a.ws, Mission{ID: "lab", Engine: "nix"}); err != nil {
		t.Fatal(err)
	}
	return a, workspace
}

func TestWorkspaceArtifactRegistrationAndAIPermission(t *testing.T) {
	a, workspace := artifactTestApp(t)
	if err := os.MkdirAll(filepath.Join(workspace, "logs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "logs", "scan.txt"), []byte("ground truth\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	items, err := a.ListWorkspaceArtifacts("lab")
	if err != nil || len(items) != 1 || items[0].Registered {
		t.Fatalf("initial inventory = %#v, %v", items, err)
	}
	c, err := a.RegisterWorkspaceArtifact("lab", "logs/scan.txt", false)
	if err != nil {
		t.Fatal(err)
	}
	if c.Meta["AI content access"] != "denied" || c.Meta["SHA-256"] == "" {
		t.Fatalf("capture metadata = %#v", c.Meta)
	}
	s := &mcpServer{store: a.store, eng: a.eng, opts: MCPOptions{Workspace: a.ws, Mission: "lab"}}
	if _, err := s.call("read_artifact_content", map[string]any{"name": c.Name}); err == nil || !strings.Contains(err.Error(), "not approved") {
		t.Fatalf("unapproved MCP read error = %v", err)
	}
	if err := a.SetArtifactAIContentAccess("lab", c.Name, true); err != nil {
		t.Fatal(err)
	}
	result, err := s.call("read_artifact_content", map[string]any{"name": c.Name})
	if err != nil || !strings.Contains(result.(map[string]any)["content"].(string), "ground truth") {
		t.Fatalf("approved MCP read = %#v, %v", result, err)
	}
}

func TestWorkspaceArtifactRejectsTraversalAndBinaryAI(t *testing.T) {
	a, workspace := artifactTestApp(t)
	if _, err := a.PreviewWorkspaceArtifact("lab", "../secret"); err == nil {
		t.Fatal("path traversal was accepted")
	}
	if err := os.WriteFile(filepath.Join(workspace, "radio.bin"), []byte{0, 1, 2, 3}, 0o600); err != nil {
		t.Fatal(err)
	}
	preview, err := a.PreviewWorkspaceArtifact("lab", "radio.bin")
	if err != nil || preview.Kind != "binary" || preview.Content != "AAECAw==" {
		t.Fatalf("binary preview = %#v, %v", preview, err)
	}
	if _, err := a.RegisterWorkspaceArtifact("lab", "radio.bin", true); err == nil || !strings.Contains(err.Error(), "binary artifact") {
		t.Fatalf("binary AI approval error = %v", err)
	}
}

func TestAttachWorkspaceArtifactAddsNoteLink(t *testing.T) {
	a, workspace := artifactTestApp(t)
	if err := os.WriteFile(filepath.Join(workspace, "trace.pcap"), []byte("test capture"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := a.AttachWorkspaceArtifactToNote("lab", "trace.pcap"); err != nil {
		t.Fatal(err)
	}
	note, err := a.store.GetNote(a.ws, "lab", "note.md")
	if err != nil || !strings.Contains(note, "[Artifact: trace.pcap](../captures/trace.pcap)") {
		t.Fatalf("note = %q, %v", note, err)
	}
}

func TestCastArtifactIsDecodedForAI(t *testing.T) {
	a, workspace := artifactTestApp(t)
	cast := strings.Join([]string{
		`{"version":2,"width":100,"height":30,"timestamp":1}`,
		`[0.1,"i","pm3\r"]`,
		`[0.2,"o","\u001b[32mdevice found\u001b[0m\r\n"]`,
	}, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(workspace, "session.cast"), []byte(cast), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := a.RegisterWorkspaceArtifact("lab", "session.cast", true)
	if err != nil {
		t.Fatal(err)
	}
	s := &mcpServer{store: a.store, eng: a.eng, opts: MCPOptions{Workspace: a.ws, Mission: "lab"}}
	result, err := s.call("read_artifact_content", map[string]any{"name": c.Name})
	if err != nil {
		t.Fatal(err)
	}
	content := result.(map[string]any)["content"].(string)
	if !strings.Contains(content, "[input]\npm3") || !strings.Contains(content, "[output]\ndevice found") {
		t.Fatalf("decoded cast = %q", content)
	}
	if strings.Contains(content, "\x1b") || strings.Contains(content, `\"version\"`) {
		t.Fatalf("cast leaked terminal controls or raw JSON: %q", content)
	}
}

func TestMalformedCastCannotBeApprovedForAI(t *testing.T) {
	a, workspace := artifactTestApp(t)
	if err := os.WriteFile(filepath.Join(workspace, "broken.cast"), []byte("not a cast\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := a.RegisterWorkspaceArtifact("lab", "broken.cast", true); err == nil || !strings.Contains(err.Error(), "asciinema") {
		t.Fatalf("malformed cast approval error = %v", err)
	}
}

func TestManagedTerminalRecordingRegistrationPersistsForMCP(t *testing.T) {
	a, _ := artifactTestApp(t)
	dir := filepath.Join(a.store.missionDir(a.ws, "lab"), "recordings")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	name := "lab-terminal.cast"
	cast := "{\"version\":2,\"width\":80,\"height\":24}\n[0.1,\"o\",\"verified output\\r\\n\"]\n"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(cast), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := a.RegisterTerminalRecordingEvidence("lab", name)
	if err != nil {
		t.Fatal(err)
	}
	if c.Name != name || c.Meta["AI content access"] != "approved" {
		t.Fatalf("registered recording = %#v", c)
	}
	resolved, err := a.RegisteredTerminalRecordingPath("lab", name)
	if err != nil || filepath.Base(resolved) != name {
		t.Fatalf("player path = %q, %v", resolved, err)
	}
	s := &mcpServer{store: a.store, eng: a.eng, opts: MCPOptions{Workspace: a.ws, Mission: "lab"}}
	result, err := s.call("read_artifact_content", map[string]any{"name": name})
	if err != nil {
		t.Fatal(err)
	}
	if content := result.(map[string]any)["content"].(string); !strings.Contains(content, "verified output") {
		t.Fatalf("MCP transcript = %q", content)
	}
}

func TestEvidenceIndexRegistersRecordingReferencedByExistingNote(t *testing.T) {
	a, _ := artifactTestApp(t)
	dir := filepath.Join(a.store.missionDir(a.ws, "lab"), "recordings")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	name := "attached.cast"
	cast := "{\"version\":2,\"width\":80,\"height\":24}\n[0.1,\"o\",\"note evidence\\r\\n\"]\n"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(cast), 0o600); err != nil {
		t.Fatal(err)
	}
	// Simulate a note made by an older Workbench release, before automatic
	// registration was performed by App.SaveNote.
	if err := a.store.SaveNote(a.ws, "lab", "note.md", "# Evidence\n\n:::terminal-recording "+filepath.Join(dir, name)+" :::\n"); err != nil {
		t.Fatal(err)
	}
	s := &mcpServer{store: a.store, eng: a.eng, opts: MCPOptions{Workspace: a.ws, Mission: "lab"}}
	index, err := s.call("read_evidence_index", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	artifacts := index.(map[string]any)["artifacts"].([]Capture)
	if len(artifacts) != 1 || artifacts[0].Name != name || artifacts[0].Meta["AI content access"] != "approved" {
		t.Fatalf("migrated artifacts = %#v", artifacts)
	}
	if _, err := s.call("read_artifact_content", map[string]any{"name": name}); err != nil {
		t.Fatalf("attached recording is not AI-readable: %v", err)
	}
}
