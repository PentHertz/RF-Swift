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

func TestWriteAgentKitWritesCodexKimiAndVibeAgents(t *testing.T) {
	dir := t.TempDir()
	_, agents, err := writeAgentKit(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, ag := range agents {
		codex, err := os.ReadFile(filepath.Join(dir, ".codex", "agents", ag.Name+".toml"))
		if err != nil {
			t.Fatal(err)
		}
		text := string(codex)
		for _, want := range []string{"name = \"" + strings.ReplaceAll(ag.Name, "-", "_") + "\"", "description = \"", "developer_instructions = '''\n", "## Hand-off"} {
			if !strings.Contains(text, want) {
				t.Errorf("codex %s lacks %q", ag.Name, want)
			}
		}
		kimi, err := os.ReadFile(filepath.Join(dir, ".kimi-code", "agents", ag.Name+".md"))
		if err != nil || !strings.HasPrefix(string(kimi), "---\nname: "+ag.Name+"\ndescription: \"") || !strings.Contains(string(kimi), "## Hand-off") {
			t.Errorf("kimi agent %s malformed: %v\n%.200s", ag.Name, err, kimi)
		}
		vibe, err := os.ReadFile(filepath.Join(dir, ".vibe", "agents", ag.Name+".toml"))
		if err != nil || !strings.Contains(string(vibe), "agent_type = \"agent\"") || !strings.Contains(string(vibe), "instructions = '''\n") {
			t.Errorf("vibe agent %s malformed: %v", ag.Name, err)
		}
		if strings.Count(string(codex), "'''") != 2 || strings.Count(string(vibe), "'''") != 2 {
			t.Errorf("%s: a prompt contains the TOML literal delimiter", ag.Name)
		}
	}
}

func TestAgentStartAsArgs(t *testing.T) {
	if args, err := agentStartAsArgs("/w", "mistral", "rfswift-rf-recon"); err != nil || strings.Join(args, " ") != "--agent rfswift-rf-recon" {
		t.Fatalf("vibe: %v %v", args, err)
	}
	if args, err := agentStartAsArgs("/w", "kimi", "rfswift-rf-recon"); err != nil || len(args) != 2 || args[0] != "--agent-file" || !strings.HasSuffix(args[1], filepath.Join(".kimi-code", "agents", "rfswift-rf-recon.md")) {
		t.Fatalf("kimi: %v %v", args, err)
	}
	if _, err := agentStartAsArgs("/w", "mistral", "rfswift-unknown"); err == nil {
		t.Fatal("unknown agent accepted")
	}
	if _, err := agentStartAsArgs("/w", "codex", "rfswift-rf-recon"); err == nil {
		t.Fatal("start-as accepted for a client that uses subagents")
	}
	if args, err := agentStartAsArgs("/w", "codex", ""); err != nil || args != nil {
		t.Fatalf("empty choice: %v %v", args, err)
	}
}
