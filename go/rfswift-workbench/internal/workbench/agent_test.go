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
	written, err := writeAgentMCPConfig(dir, "claude", "/opt/rfswift workbench", []string{"--mcp", "--mission", "rfid"}, false)
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
	path, err := writeAgentMCPConfig(dir, "codex", "/opt/RF Swift/bin", []string{"--mcp", "--mission", "rfid"}, false)
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
	path, err := writeAgentMCPConfig(dir, "kimi", "/bin/rfswift", []string{"--mcp"}, false)
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
		"codex":   "--dangerously-bypass-approvals-and-sandbox",
		"claude":  "--dangerously-skip-permissions",
		"kimi":    "--yolo",
		"glm":     "--yolo",
		"mistral": "--auto-approve",
	}
	for client, want := range wants {
		args := agentYoloArgs(client)
		if len(args) != 1 || args[0] != want {
			t.Fatalf("%s YOLO args = %#v, want %q", client, args, want)
		}
	}
	// OpenCode's auto-approval is set in its opencode.json, not by a flag.
	for _, client := range []string{"unknown", "opencode"} {
		if args := agentYoloArgs(client); len(args) != 0 {
			t.Fatalf("%s received YOLO args: %#v", client, args)
		}
	}
}

func TestAgentClientsIncludeOpenCodeAndMistralVibe(t *testing.T) {
	want := map[string]string{"opencode": "opencode", "mistral": "vibe"}
	for _, c := range AgentClients() {
		if cmd, ok := want[c.ID]; ok {
			if c.Command != cmd {
				t.Fatalf("%s runs %q, want %q", c.ID, c.Command, cmd)
			}
			delete(want, c.ID)
		}
	}
	if len(want) != 0 {
		t.Fatalf("missing clients: %v", want)
	}
}

func TestWriteAgentMCPConfigOpenCode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "opencode.json")
	if err := os.WriteFile(path, []byte(`{"model":"keep/me","instructions":["AGENTS.md"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	read := func() map[string]any {
		t.Helper()
		var root map[string]any
		data, _ := os.ReadFile(path)
		if err := json.Unmarshal(data, &root); err != nil {
			t.Fatal(err)
		}
		return root
	}
	written, err := writeAgentMCPConfig(dir, "opencode", "/opt/RF Swift/wb", []string{"--mcp", "--mission", "rfid"}, false)
	if err != nil || written != path {
		t.Fatalf("written %q, %v", written, err)
	}
	root := read()
	server := root["mcp"].(map[string]any)["rfswift"].(map[string]any)
	command := server["command"].([]any)
	if server["type"] != "local" || server["enabled"] != true || server["cwd"] != dir || len(command) != 4 || command[0] != "/opt/RF Swift/wb" || command[3] != "rfid" {
		t.Fatalf("unexpected server entry: %#v", server)
	}
	if root["model"] != "keep/me" || len(root["instructions"].([]any)) != 1 {
		t.Fatalf("existing keys not kept or instructions duplicated: %#v", root)
	}
	perm, ok := root["permission"].(map[string]any)
	if !ok || perm["bash"] != "ask" || perm["edit"] != "ask" {
		t.Fatalf("OpenCode must ask before edits and commands without YOLO: %#v", root["permission"])
	}
	if _, err := writeAgentMCPConfig(dir, "opencode", "/opt/RF Swift/wb", []string{"--mcp"}, true); err != nil {
		t.Fatal(err)
	}
	if read()["permission"] != "allow" {
		t.Fatalf("YOLO should allow everything: %#v", read()["permission"])
	}
}

func TestWriteAgentMCPConfigMistralVibe(t *testing.T) {
	home := t.TempDir()
	t.Setenv("VIBE_HOME", home)
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte("active_model = \"devstral-medium\"\n[[mcp_servers]]\nname = \"fetch\"\ntransport = \"stdio\"\ncommand = \"uvx\""), 0o600); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	for i := 0; i < 2; i++ { // a reconnect rebuilds the file instead of stacking entries
		path, err := writeAgentMCPConfig(dir, "mistral", "/opt/RF Swift/wb", []string{"--mcp", "--mission", "rfid"}, false)
		if err != nil {
			t.Fatal(err)
		}
		if path != filepath.Join(dir, ".vibe", "config.toml") {
			t.Fatalf("path = %q", path)
		}
		data, _ := os.ReadFile(path)
		text := string(data)
		for _, want := range []string{`active_model = "devstral-medium"`, `name = "fetch"`, `name = "rfswift"`, `transport = "stdio"`, `command = "/opt/RF Swift/wb"`, `args = ["--mcp", "--mission", "rfid"]`} {
			if !strings.Contains(text, want) {
				t.Fatalf("missing %q in:\n%s", want, text)
			}
		}
		if strings.Count(text, `name = "rfswift"`) != 1 || !strings.Contains(text, "command = \"uvx\"\n\n#") {
			t.Fatalf("global config not copied cleanly or server stacked:\n%s", text)
		}
	}
	t.Setenv("VIBE_HOME", t.TempDir()) // no global config at all
	path, err := writeAgentMCPConfig(t.TempDir(), "mistral", "/bin/wb", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(path); !strings.HasPrefix(string(data), "# Added by RF Swift Workbench") || !strings.Contains(string(data), "args = []") {
		t.Fatalf("standalone config:\n%s", data)
	}
}
