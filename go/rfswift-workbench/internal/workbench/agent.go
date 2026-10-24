package workbench

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// AgentCfg controls the optional MCP bridge. RF Swift never stores a model API
// key or calls a model provider; the selected external CLI owns authentication.
type AgentCfg struct {
	Enabled    bool            `json:"enabled"`
	Client     string          `json:"client"`
	AllowWrite bool            `json:"allowWrite"`
	AllowExec  bool            `json:"allowExec"`
	Yolo       map[string]bool `json:"yolo,omitempty"`
}

type AgentClient struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Command   string `json:"command"`
	Available bool   `json:"available"`
	Path      string `json:"path"`
}

type AgentConnectResult struct {
	Client     string `json:"client"`
	Workspace  string `json:"workspace"`
	ConfigPath string `json:"configPath"`
	Verified   bool   `json:"verified"`
}

type AgentTerminalResult struct {
	ID         string `json:"id"`
	Client     string `json:"client"`
	Workspace  string `json:"workspace"`
	ConfigPath string `json:"configPath"`
	Verified   bool   `json:"verified"`
}

func AgentClients() []AgentClient {
	defs := []AgentClient{
		{ID: "codex", Label: "Codex CLI", Command: "codex"},
		{ID: "claude", Label: "Claude Code", Command: "claude"},
		{ID: "kimi", Label: "Kimi Code", Command: "kimi"},
		{ID: "glm", Label: "GLM / Z.ai CLI", Command: "glm"},
		{ID: "opencode", Label: "OpenCode", Command: "opencode"},
		{ID: "mistral", Label: "Mistral Vibe", Command: "vibe"},
	}
	for i := range defs {
		if path, err := exec.LookPath(defs[i].Command); err == nil {
			defs[i].Available, defs[i].Path = true, path
		}
	}
	return defs
}

func (s *Store) agentCfgPath() string { return filepath.Join(s.Root, "agent.json") }

func (s *Store) LoadAgentCfg() AgentCfg {
	cfg := AgentCfg{Client: "codex"}
	_ = readJSON(s.agentCfgPath(), &cfg)
	return cfg
}

func (s *Store) SaveAgentCfg(cfg AgentCfg) error {
	if err := os.MkdirAll(s.Root, 0o700); err != nil {
		return err
	}
	if err := writeJSON(s.agentCfgPath(), cfg); err != nil {
		return err
	}
	return os.Chmod(s.agentCfgPath(), 0o600)
}

func agentPosture(cfg AgentCfg) []Check {
	if !cfg.Enabled {
		return []Check{{Severity: "ok", Label: "MCP agent bridge", Detail: "Disabled; no AI client can access Workbench data."}}
	}
	checks := []Check{{Severity: "ok", Label: "AI credentials", Detail: "RF Swift stores no model API key; authentication is owned by the selected external CLI."}}
	if cfg.AllowExec {
		checks = append(checks, Check{Severity: "warn", Label: "MCP command execution", Detail: "The selected agent may execute commands in its mission target.", Rec: "Enable only for an authorized mission and disable after use."})
	} else if cfg.AllowWrite {
		checks = append(checks, Check{Severity: "ok", Label: "MCP permissions", Detail: "Agent may update mission notes and findings but cannot execute commands."})
	} else {
		checks = append(checks, Check{Severity: "ok", Label: "MCP permissions", Detail: "Read-only mission access."})
	}
	if cfg.Yolo[cfg.Client] {
		checks = append(checks, Check{Severity: "warn", Label: "Agent YOLO mode", Detail: "The selected coding-agent CLI auto-approves its own tool actions.", Rec: "Use only in an isolated, trusted mission workspace and disable it after use."})
	}
	return checks
}

func agentYoloArgs(clientID string) []string {
	switch clientID {
	case "codex":
		return []string{"--dangerously-bypass-approvals-and-sandbox"}
	case "claude":
		return []string{"--dangerously-skip-permissions"}
	case "kimi", "glm":
		return []string{"--yolo"}
	case "mistral":
		return []string{"--auto-approve"}
	default:
		// OpenCode has no such flag: its permissions live in the generated
		// opencode.json (writeOpenCodeConfig).
		return nil
	}
}

func (a *App) agentLaunchSpec(mission, requestedClient string) (AgentConnectResult, string, []string, error) {
	if !validWorkspaceName(mission) {
		return AgentConnectResult{}, "", nil, errors.New("invalid mission scope")
	}
	cfg := a.store.LoadAgentCfg()
	if !cfg.Enabled {
		return AgentConnectResult{}, "", nil, errors.New("enable the mission-scoped MCP bridge first")
	}
	clientID := strings.TrimSpace(requestedClient)
	if clientID == "" {
		clientID = cfg.Client
	}
	var selected AgentClient
	for _, candidate := range AgentClients() {
		if candidate.ID == clientID {
			selected = candidate
			break
		}
	}
	if selected.ID == "" {
		return AgentConnectResult{}, "", nil, fmt.Errorf("unsupported coding-agent client %q", clientID)
	}
	if !selected.Available {
		return AgentConnectResult{}, "", nil, fmt.Errorf("%s is not installed or is not on PATH", selected.Label)
	}
	executable, err := os.Executable()
	if err != nil {
		return AgentConnectResult{}, "", nil, err
	}
	args := []string{"--mcp", "--workspace", a.workspace(), "--mission", mission}
	if cfg.AllowWrite {
		args = append(args, "--mcp-write")
	}
	if cfg.AllowExec {
		args = append(args, "--mcp-exec")
	}
	agentDir := filepath.Join(a.store.missionDir(a.workspace(), mission), "agent-workspace")
	if err := os.MkdirAll(agentDir, 0o700); err != nil {
		return AgentConnectResult{}, "", nil, err
	}
	if err := os.WriteFile(filepath.Join(agentDir, agentTerminalEventsFile), nil, 0o600); err != nil {
		return AgentConnectResult{}, "", nil, err
	}
	configPath, err := writeAgentMCPConfig(agentDir, clientID, executable, args, cfg.Yolo[clientID])
	if err != nil {
		return AgentConnectResult{}, "", nil, err
	}
	if err := verifyMCPCommand(executable, args); err != nil {
		return AgentConnectResult{}, "", nil, fmt.Errorf("MCP verification failed: %w", err)
	}
	if err := writeAgentInstructions(agentDir, mission); err != nil {
		return AgentConnectResult{}, "", nil, err
	}
	launchArgs := []string{}
	if cfg.Yolo[clientID] {
		launchArgs = agentYoloArgs(clientID)
	}
	return AgentConnectResult{Client: clientID, Workspace: agentDir, ConfigPath: configPath, Verified: true}, selected.Path, launchArgs, nil
}

func writeAgentInstructions(agentDir, mission string) error {
	body := "# RF Swift mission agent workspace\n\n" +
		"This directory is only a launcher workspace. Do not create assessment notes as ordinary files here.\n\n" +
		"Use the `rfswift` MCP tools for mission data. In particular:\n\n" +
		"- When asked to create, edit, append, or summarize a note, call `write_note` for mission `" + mission + "` and normally use name `note.md`.\n" +
		"- Read the current note with `read_note` before changing it.\n" +
		"- Store security findings with `save_finding`, not as loose files.\n" +
		"- When explicitly asked to collect secrets, search notes and approved captures/recordings, then call `save_secret` only for exact observed values with their precise source. Never guess credentials or repeat their values in notes, findings, reports, or chat.\n" +
		"- Never claim a note or finding was saved unless the MCP tool returned success.\n" +
		"- Ask before destructive or out-of-scope actions.\n"
	for _, name := range []string{"AGENTS.md", "CLAUDE.md"} {
		if err := os.WriteFile(filepath.Join(agentDir, name), []byte(body), 0o600); err != nil {
			return err
		}
	}
	return nil
}

func verifyMCPCommand(command string, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, command, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	request := map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "rfswift-workbench", "version": "1"}}}
	if err := json.NewEncoder(stdin).Encode(request); err != nil {
		_ = cmd.Process.Kill()
		return err
	}
	var response struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Result  json.RawMessage `json:"result"`
		Error   json.RawMessage `json:"error"`
	}
	if err := json.NewDecoder(stdout).Decode(&response); err != nil {
		_ = cmd.Process.Kill()
		return err
	}
	_ = cmd.Process.Kill()
	_, _ = cmd.Process.Wait()
	if response.JSONRPC != "2.0" || len(response.Result) == 0 || string(response.Result) == "null" || len(response.Error) > 0 {
		return fmt.Errorf("invalid initialize response")
	}
	return nil
}

func writeAgentMCPConfig(agentDir, clientID, command string, args []string, yolo bool) (string, error) {
	entry := map[string]any{"command": command, "args": args, "cwd": agentDir}
	switch clientID {
	case "opencode":
		return writeOpenCodeConfig(agentDir, command, args, yolo)
	case "mistral":
		return writeVibeConfig(agentDir, command, args)
	case "claude":
		return mergeMCPJSON(filepath.Join(agentDir, ".mcp.json"), entry)
	case "kimi":
		return mergeMCPJSON(filepath.Join(agentDir, ".kimi-code", "mcp.json"), entry)
	case "codex":
		path := filepath.Join(agentDir, ".codex", "config.toml")
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return "", err
		}
		quotedArgs := make([]string, len(args))
		for i, arg := range args {
			quotedArgs[i] = strconv.Quote(arg)
		}
		contents := "# Generated by RF Swift Workbench for this mission only.\n" +
			"[mcp_servers.rfswift]\ncommand = " + strconv.Quote(command) + "\nargs = [" + strings.Join(quotedArgs, ", ") + "]\n" +
			"cwd = " + strconv.Quote(agentDir) + "\nenabled = true\ndefault_tools_approval_mode = \"prompt\"\n"
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			return "", err
		}
		return path, nil
	default:
		return "", fmt.Errorf("automatic MCP configuration is not yet available for %s; use the displayed stdio command", clientID)
	}
}

// writeOpenCodeConfig writes the mission's opencode.json, which OpenCode reads
// from its start directory and merges over the user's global configuration.
// OpenCode allows most actions without asking by default, so unless YOLO is
// on the file makes it ask before edits, shell commands and web access, like
// the other clients do out of the box. Other keys already in the file stay.
func writeOpenCodeConfig(agentDir, command string, args []string, yolo bool) (string, error) {
	path := filepath.Join(agentDir, "opencode.json")
	root := map[string]any{}
	if data, err := os.ReadFile(path); err == nil && len(data) > 0 {
		if err := json.Unmarshal(data, &root); err != nil {
			return "", fmt.Errorf("parse existing %s: %w", path, err)
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	root["$schema"] = "https://opencode.ai/config.json"
	servers, _ := root["mcp"].(map[string]any)
	if servers == nil {
		servers = map[string]any{}
	}
	servers["rfswift"] = map[string]any{
		"type":    "local",
		"command": append([]string{command}, args...),
		"cwd":     agentDir,
		"enabled": true,
	}
	root["mcp"] = servers
	instructions, _ := root["instructions"].([]any)
	hasAgents := false
	for _, item := range instructions {
		if item == "AGENTS.md" {
			hasAgents = true
		}
	}
	if !hasAgents {
		root["instructions"] = append(instructions, "AGENTS.md")
	}
	if yolo {
		root["permission"] = "allow"
	} else {
		root["permission"] = map[string]any{"edit": "ask", "bash": "ask", "webfetch": "ask", "websearch": "ask"}
	}
	data, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return "", err
	}
	return path, nil
}

// writeVibeConfig writes the mission's .vibe/config.toml for Mistral Vibe.
// Vibe reads a project config instead of ~/.vibe/config.toml rather than on
// top of it, so the user's global file is copied first (model and provider
// settings; API keys live in a separate .env) and the RF Swift server is
// appended as one more [[mcp_servers]] table. Rebuilt on every connect.
func writeVibeConfig(agentDir, command string, args []string) (string, error) {
	path := filepath.Join(agentDir, ".vibe", "config.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	var contents strings.Builder
	if global := vibeGlobalConfig(); global != "" {
		if data, err := os.ReadFile(global); err == nil && len(data) > 0 {
			contents.Write(data)
			if data[len(data)-1] != '\n' {
				contents.WriteByte('\n')
			}
			contents.WriteByte('\n')
		}
	}
	quotedArgs := make([]string, len(args))
	for i, arg := range args {
		quotedArgs[i] = strconv.Quote(arg)
	}
	contents.WriteString("# Added by RF Swift Workbench for this mission only.\n" +
		"[[mcp_servers]]\nname = \"rfswift\"\ntransport = \"stdio\"\ncommand = " + strconv.Quote(command) +
		"\nargs = [" + strings.Join(quotedArgs, ", ") + "]\n")
	if err := os.WriteFile(path, []byte(contents.String()), 0o600); err != nil {
		return "", err
	}
	return path, nil
}

// vibeGlobalConfig is the user's Vibe configuration file ($VIBE_HOME, or
// ~/.vibe), or "" when it cannot be located.
func vibeGlobalConfig() string {
	if home := strings.TrimSpace(os.Getenv("VIBE_HOME")); home != "" {
		return filepath.Join(home, "config.toml")
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".vibe", "config.toml")
	}
	return ""
}

func mergeMCPJSON(path string, entry map[string]any) (string, error) {
	root := map[string]any{}
	if data, err := os.ReadFile(path); err == nil && len(data) > 0 {
		if err := json.Unmarshal(data, &root); err != nil {
			return "", fmt.Errorf("parse existing %s: %w", path, err)
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	servers, _ := root["mcpServers"].(map[string]any)
	if servers == nil {
		servers = map[string]any{}
	}
	servers["rfswift"] = entry
	root["mcpServers"] = servers
	data, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return "", err
	}
	return path, nil
}
