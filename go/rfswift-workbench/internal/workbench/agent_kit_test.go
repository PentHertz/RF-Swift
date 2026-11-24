package workbench

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAgentKitIsWellFormed(t *testing.T) {
	skills, err := agentKitSkills()
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) < 5 {
		t.Fatalf("only %d skills embedded", len(skills))
	}
	found := false
	for _, sk := range skills {
		if len(sk.Description) > 1024 {
			t.Errorf("%s: description of %d characters exceeds the Agent Skills limit", sk.Name, len(sk.Description))
		}
		if strings.Contains(string(sk.body), "run_tool") || strings.Contains(string(sk.body), "list_tool_recipes") {
			t.Errorf("%s refers to MCP tools the server does not provide", sk.Name)
		}
		found = found || sk.Name == "rfswift-mission-workflow"
	}
	if !found {
		t.Fatal("rfswift-mission-workflow is missing")
	}
	agents, err := agentKitAgents()
	if err != nil {
		t.Fatal(err)
	}
	if len(agents) == 0 {
		t.Fatal("no subagents embedded")
	}
}

func TestWriteAgentKitInstallsForEveryClientAndKeepsForeignFiles(t *testing.T) {
	dir := t.TempDir()
	foreign := filepath.Join(dir, ".claude", "skills", "my-skill", "SKILL.md")
	stale := filepath.Join(dir, ".agents", "skills", "rfswift-removed-skill")
	staleAgent := filepath.Join(dir, ".opencode", "agents", "rfswift-old.md")
	for _, p := range []string{foreign, filepath.Join(stale, "SKILL.md"), staleAgent} {
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	skills, agents, err := writeAgentKit(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, base := range []string{".claude/skills", ".agents/skills"} {
		for _, sk := range skills {
			if _, err := os.Stat(filepath.Join(dir, base, sk.Name, "SKILL.md")); err != nil {
				t.Errorf("%s/%s not installed: %v", base, sk.Name, err)
			}
		}
	}
	for _, ag := range agents {
		claude, err := os.ReadFile(filepath.Join(dir, ".claude", "agents", ag.Name+".md"))
		if err != nil || !strings.Contains(string(claude), "name: "+ag.Name) {
			t.Errorf("Claude Code agent %s not installed: %v", ag.Name, err)
		}
		oc, err := os.ReadFile(filepath.Join(dir, ".opencode", "agents", ag.Name+".md"))
		if err != nil || !strings.Contains(string(oc), "mode: subagent") || strings.Contains(string(oc), "\nname:") {
			t.Errorf("OpenCode agent %s not converted: %v\n%s", ag.Name, err, oc)
		}
	}
	if _, err := os.Stat(foreign); err != nil {
		t.Fatal("a skill RF Swift does not own was removed")
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatal("a stale rfswift skill was kept")
	}
	if _, err := os.Stat(staleAgent); !os.IsNotExist(err) {
		t.Fatal("a stale rfswift agent was kept")
	}
}

func TestWriteAgentInstructionsListsSkillsAndTransmitState(t *testing.T) {
	dir := t.TempDir()
	if err := writeAgentInstructions(dir, "lab", AgentCfg{AllowExec: true}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{"rfswift-mission-workflow", "job_output", "RF transmission is disabled", "board.md"} {
		if !strings.Contains(text, want) {
			t.Errorf("AGENTS.md lacks %q", want)
		}
	}
	if err := writeAgentInstructions(dir, "lab", AgentCfg{AllowExec: true, AllowTransmit: true}); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	if !strings.Contains(string(data), "RF transmission is enabled") {
		t.Error("CLAUDE.md does not describe enabled transmission")
	}
}

func TestMCPSaveFindingKeepsOperatorFindings(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := store.SaveMission("default", Mission{ID: "lab"}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveFindings("default", "lab", []Finding{{ID: "F-op", Title: "Operator finding", Status: "confirmed", Origin: "operator"}, {ID: "F-legacy", Title: "Older finding", Status: "open"}}); err != nil {
		t.Fatal(err)
	}
	s := &mcpServer{store: store, eng: &fakeEngine{}, opts: MCPOptions{Workspace: "default", Mission: "lab", AllowWrite: true}}
	for _, id := range []string{"F-op", "F-legacy"} {
		if _, err := s.call("save_finding", map[string]any{"finding": map[string]any{"id": id, "title": "overwrite", "status": "confirmed"}}); err == nil {
			t.Fatalf("an agent changed operator finding %s", id)
		}
	}
	got, err := s.call("save_finding", map[string]any{"finding": map[string]any{"id": "F-ai", "title": "Candidate", "status": "confirmed"}})
	if err != nil {
		t.Fatal(err)
	}
	if m, ok := got.(map[string]any); !ok || !strings.Contains(m["notice"].(string), "draft") {
		t.Fatalf("no notice about the replaced status: %#v", got)
	}
	if _, err := s.call("save_finding", map[string]any{"finding": map[string]any{"id": "F-ai", "title": "Candidate, updated"}}); err != nil {
		t.Fatalf("agent could not update its own candidate: %v", err)
	}
	findings, _ := store.LoadFindings("default", "lab")
	for _, f := range findings {
		switch f.ID {
		case "F-op":
			if f.Title != "Operator finding" || f.Status != "confirmed" {
				t.Fatalf("operator finding changed: %#v", f)
			}
		case "F-ai":
			if f.Status != "draft" || f.Origin != "agent" || f.Title != "Candidate, updated" {
				t.Fatalf("agent finding not stored as a draft candidate: %#v", f)
			}
		}
	}
}

func TestMCPToolsWithAllPermissionsHaveObjectSchemas(t *testing.T) {
	s := &mcpServer{opts: MCPOptions{AllowWrite: true, AllowExec: true, AllowTransmit: true}}
	names := map[string]bool{}
	for _, tool := range s.tools() {
		names[tool["name"].(string)] = true
		schema, ok := tool["inputSchema"].(map[string]any)
		if !ok || schema["properties"] == nil {
			t.Fatalf("tool %v has no object schema", tool["name"])
		}
	}
	for _, want := range []string{"execute_command", "job_output", "cancel_job", "list_jobs", "session_start", "session_send", "session_close"} {
		if !names[want] {
			t.Errorf("tool %s missing", want)
		}
	}
	readOnly := &mcpServer{}
	for _, tool := range readOnly.tools() {
		if strings.HasPrefix(tool["name"].(string), "session_") || tool["name"] == "execute_command" || tool["name"] == "job_output" {
			t.Errorf("exec tool %v listed without exec permission", tool["name"])
		}
	}
}

func TestClassifyCaptureRecognisesIQSampleFormats(t *testing.T) {
	for _, name := range []string{"433.92M_250k.cu8", "gate.cs8", "x.sc16", "y.cf32", "z.fc32", "w.cf64", "v.cu16", "u.sigmf-data"} {
		if got := ClassifyCapture(name); got != "iq" {
			t.Errorf("%s classified as %q, want iq", name, got)
		}
	}
}
