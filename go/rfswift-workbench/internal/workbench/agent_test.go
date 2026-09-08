package workbench

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteAgentMCPConfigMergesClaudeServers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".mcp.json")
	if err := os.WriteFile(path, []byte(`{"mcpServers":{"existing":{"command":"keep"}},"custom":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	written, err := writeAgentMCPConfig(dir, "claude", "/opt/rfswift workbench", []string{"--mcp", "--mission", "rfid"})
	if err != nil {
		t.Fatal(err)
	}
	if written != path {
		t.Fatalf("config path = %q, want %q", written, path)
	}
	var root map[string]any
	data, _ := os.ReadFile(path)
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatal(err)
	}
	servers := root["mcpServers"].(map[string]any)
	if servers["existing"] == nil || servers["rfswift"] == nil || root["custom"] != true {
		t.Fatalf("existing configuration was not preserved: %s", data)
	}
}

func TestWriteAgentMCPConfigCodexProjectScope(t *testing.T) {
	dir := t.TempDir()
	path, err := writeAgentMCPConfig(dir, "codex", "/opt/RF Swift/bin", []string{"--mcp", "--mission", "rfid"})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	text := string(data)
	for _, want := range []string{"[mcp_servers.rfswift]", `command = "/opt/RF Swift/bin"`, `args = ["--mcp", "--mission", "rfid"]`, `default_tools_approval_mode = "prompt"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in:\n%s", want, text)
		}
	}
}

func TestWriteAgentMCPConfigKimiProjectScope(t *testing.T) {
	dir := t.TempDir()
	path, err := writeAgentMCPConfig(dir, "kimi", "/bin/rfswift", []string{"--mcp"})
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, ".kimi-code", "mcp.json")
	if path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
}

func TestWriteAgentInstructionsRoutesNotesThroughMCP(t *testing.T) {
	dir := t.TempDir()
	if err := writeAgentInstructions(dir, "rfid-lab"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"AGENTS.md", "CLAUDE.md"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		if !strings.Contains(text, "call `write_note`") || !strings.Contains(text, "mission `rfid-lab`") || !strings.Contains(strings.ToLower(text), "do not create assessment notes as ordinary files") {
			t.Fatalf("%s does not route note requests through MCP:\n%s", name, text)
		}
	}
}

func TestAgentYoloArgsAreClientSpecific(t *testing.T) {
	wants := map[string]string{
		"codex":  "--dangerously-bypass-approvals-and-sandbox",
		"claude": "--dangerously-skip-permissions",
		"kimi":   "--yolo",
		"glm":    "--yolo",
	}
	for client, want := range wants {
		args := agentYoloArgs(client)
		if len(args) != 1 || args[0] != want {
			t.Fatalf("%s YOLO args = %#v, want %q", client, args, want)
		}
	}
	if args := agentYoloArgs("unknown"); len(args) != 0 {
		t.Fatalf("unsupported client received YOLO args: %#v", args)
	}
}
