/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
 */

package workbench

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// RF Swift agent kit: the skills and subagents a coding-agent CLI gets in a
// mission's agent workspace.
//
// Skills follow the Agent Skills layout (<id>/SKILL.md with name and
// description frontmatter) and are written where each client looks for project
// skills: .claude/skills (Claude Code, also read by OpenCode) and
// .agents/skills (Codex, OpenCode, Kimi Code, Mistral Vibe). Subagents are
// written for Claude Code (.claude/agents) and OpenCode (.opencode/agents,
// mode: subagent). Everything RF Swift writes is named rfswift-*; other files
// in those folders are never touched, and rfswift-* entries that are no longer
// in the kit are removed.

//go:embed agentkit/skills agentkit/agents
var agentKitFS embed.FS

const agentKitPrefix = "rfswift-"

type agentKitItem struct {
	Name        string
	Description string
	body        []byte // whole file (agents) or SKILL.md (skills)
	dir         string // skills: embedded directory
}

func parseKitFrontmatter(data []byte) (name, description string, body []byte, err error) {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return "", "", nil, errors.New("missing frontmatter")
	}
	end := strings.Index(text[4:], "\n---\n")
	if end < 0 {
		return "", "", nil, errors.New("unterminated frontmatter")
	}
	for _, line := range strings.Split(text[4:4+end], "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		switch strings.TrimSpace(key) {
		case "name":
			name = strings.TrimSpace(value)
		case "description":
			description = strings.TrimSpace(value)
		}
	}
	if name == "" || description == "" {
		return "", "", nil, errors.New("frontmatter needs name and description")
	}
	return name, description, []byte(text[4+end+5:]), nil
}

func agentKitSkills() ([]agentKitItem, error) {
	entries, err := fs.ReadDir(agentKitFS, "agentkit/skills")
	if err != nil {
		return nil, err
	}
	var out []agentKitItem
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := path.Join("agentkit/skills", e.Name())
		data, err := fs.ReadFile(agentKitFS, path.Join(dir, "SKILL.md"))
		if err != nil {
			return nil, fmt.Errorf("skill %s: %w", e.Name(), err)
		}
		name, desc, _, err := parseKitFrontmatter(data)
		if err != nil {
			return nil, fmt.Errorf("skill %s: %w", e.Name(), err)
		}
		if name != e.Name() || !strings.HasPrefix(name, agentKitPrefix) {
			return nil, fmt.Errorf("skill %s: name must equal the directory and start with %s", e.Name(), agentKitPrefix)
		}
		out = append(out, agentKitItem{Name: name, Description: desc, body: data, dir: dir})
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Name < out[b].Name })
	return out, nil
}

func agentKitAgents() ([]agentKitItem, error) {
	entries, err := fs.ReadDir(agentKitFS, "agentkit/agents")
	if err != nil {
		return nil, err
	}
	var out []agentKitItem
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		data, err := fs.ReadFile(agentKitFS, path.Join("agentkit/agents", e.Name()))
		if err != nil {
			return nil, err
		}
		name, desc, _, err := parseKitFrontmatter(data)
		if err != nil {
			return nil, fmt.Errorf("agent %s: %w", e.Name(), err)
		}
		if name+".md" != e.Name() || !strings.HasPrefix(name, agentKitPrefix) {
			return nil, fmt.Errorf("agent %s: name must equal the file name and start with %s", e.Name(), agentKitPrefix)
		}
		out = append(out, agentKitItem{Name: name, Description: desc, body: data})
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Name < out[b].Name })
	return out, nil
}

// openCodeAgent rewrites a Claude Code subagent file for OpenCode: the same
// prompt, with OpenCode's frontmatter (description, mode: subagent).
func openCodeAgent(item agentKitItem) []byte {
	_, _, body, _ := parseKitFrontmatter(item.body)
	desc := strings.ReplaceAll(strings.ReplaceAll(item.Description, `\`, `\\`), `"`, `\"`)
	return []byte("---\ndescription: \"" + desc + "\"\nmode: subagent\n---\n" + string(body))
}

// agentHandoff is added to the prompt of clients whose custom agents run
// without the client's own subagent framing (Codex, Kimi Code).
const agentHandoff = "\n## Hand-off\n\nYour final message is the complete hand-off to the agent that delegated to you: it sees nothing else of your work. Put the results, the evidence file names and what remains open in that message.\n"

// jsonString encodes s as a double-quoted string, which is valid in TOML basic
// strings and YAML alike.
func jsonString(s string) string {
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s)
	return strings.TrimSuffix(b.String(), "\n")
}

// tomlMultiline encodes a prompt as a TOML multi-line literal string, or as a
// basic string when the text contains the literal delimiter.
func tomlMultiline(s string) string {
	if !strings.Contains(s, "\x27\x27\x27") {
		return "\x27\x27\x27\n" + s + "\x27\x27\x27"
	}
	return jsonString(s)
}

// codexAgent writes a Codex custom agent (.codex/agents/*.toml). It inherits
// the session's MCP servers, so it reaches the mission through rfswift.
func codexAgent(item agentKitItem) []byte {
	_, _, body, _ := parseKitFrontmatter(item.body)
	name := strings.ReplaceAll(item.Name, "-", "_")
	return []byte("# Generated by RF Swift Workbench; rewritten on every connect.\n" +
		"name = " + jsonString(name) + "\n" +
		"description = " + jsonString(item.Description) + "\n" +
		"developer_instructions = " + tomlMultiline(strings.TrimSpace(string(body))+"\n"+agentHandoff) + "\n")
}

// kimiAgent writes a Kimi Code agent (.kimi-code/agents/*.md): Markdown with
// a frontmatter; all tools, the rfswift MCP tools included, stay available.
func kimiAgent(item agentKitItem) []byte {
	_, _, body, _ := parseKitFrontmatter(item.body)
	return []byte("---\nname: " + item.Name + "\ndescription: " + jsonString(item.Description) + "\n---\n" +
		strings.TrimSpace(string(body)) + "\n" + agentHandoff)
}

// vibeSafety is the badge Mistral Vibe shows for each agent (visual only).
var vibeSafety = map[string]string{
	"rfswift-signal-analyst": "safe", "rfswift-evidence-reviewer": "safe", "rfswift-rf-recon": "neutral",
	"rfswift-wireless-tester": "destructive", "rfswift-hardware-hacker": "destructive", "rfswift-cellular-lab": "destructive",
}

// vibeAgent writes a Mistral Vibe agent (.vibe/agents/*.toml). Vibe subagents
// can only use built-in tools, never MCP ones, so a subagent could not reach
// the mission: the specialists are user-facing agents instead, started with
// `vibe --agent <name>` (the Workbench's "Start as" setting does that).
func vibeAgent(item agentKitItem) []byte {
	_, _, body, _ := parseKitFrontmatter(item.body)
	safety := vibeSafety[item.Name]
	if safety == "" {
		safety = "neutral"
	}
	display := "RF Swift " + strings.ReplaceAll(strings.TrimPrefix(item.Name, agentKitPrefix), "-", " ")
	return []byte("# Generated by RF Swift Workbench; rewritten on every connect.\n" +
		"agent_type = \"agent\"\n" +
		"display_name = " + jsonString(display) + "\n" +
		"description = " + jsonString(item.Description) + "\n" +
		"safety = " + jsonString(safety) + "\n" +
		"instructions = " + tomlMultiline(strings.TrimSpace(string(body))+"\n") + "\n")
}

// removeStaleKitEntries deletes rfswift-* entries of dir that are not in keep.
func removeStaleKitEntries(dir string, keep map[string]bool) error {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), agentKitPrefix) && !keep[e.Name()] {
			if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}

// writeAgentKit installs the kit's skills and subagents into agentDir.
func writeAgentKit(agentDir string) ([]agentKitItem, []agentKitItem, error) {
	skills, err := agentKitSkills()
	if err != nil {
		return nil, nil, err
	}
	agents, err := agentKitAgents()
	if err != nil {
		return nil, nil, err
	}
	for _, base := range []string{filepath.Join(agentDir, ".claude", "skills"), filepath.Join(agentDir, ".agents", "skills")} {
		keep := map[string]bool{}
		for _, sk := range skills {
			keep[sk.Name] = true
			err := fs.WalkDir(agentKitFS, sk.dir, func(p string, d fs.DirEntry, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				rel := strings.TrimPrefix(strings.TrimPrefix(p, sk.dir), "/")
				target := filepath.Join(base, sk.Name, filepath.FromSlash(rel))
				if d.IsDir() {
					return os.MkdirAll(target, 0o700)
				}
				data, err := fs.ReadFile(agentKitFS, p)
				if err != nil {
					return err
				}
				return os.WriteFile(target, data, 0o600)
			})
			if err != nil {
				return nil, nil, err
			}
		}
		if err := removeStaleKitEntries(base, keep); err != nil {
			return nil, nil, err
		}
	}
	for _, target := range []struct {
		dir     string
		ext     string
		convert func(agentKitItem) []byte
	}{
		{filepath.Join(agentDir, ".claude", "agents"), ".md", func(ag agentKitItem) []byte { return ag.body }},
		{filepath.Join(agentDir, ".opencode", "agents"), ".md", openCodeAgent},
		{filepath.Join(agentDir, ".codex", "agents"), ".toml", codexAgent},
		{filepath.Join(agentDir, ".kimi-code", "agents"), ".md", kimiAgent},
		{filepath.Join(agentDir, ".vibe", "agents"), ".toml", vibeAgent},
	} {
		if err := os.MkdirAll(target.dir, 0o700); err != nil {
			return nil, nil, err
		}
		keep := map[string]bool{}
		for _, ag := range agents {
			keep[ag.Name+target.ext] = true
			if err := os.WriteFile(filepath.Join(target.dir, ag.Name+target.ext), target.convert(ag), 0o600); err != nil {
				return nil, nil, err
			}
		}
		if err := removeStaleKitEntries(target.dir, keep); err != nil {
			return nil, nil, err
		}
	}
	return skills, agents, nil
}

// firstSentence shortens a description for the instructions index.
func firstSentence(text string) string {
	if i := strings.Index(text, " - "); i > 0 && i < 200 {
		return text[:i]
	}
	if i := strings.Index(text, ". "); i > 0 && i < 240 {
		return text[:i+1]
	}
	if len(text) > 240 {
		return text[:240] + "..."
	}
	return text
}

// AgentKitAgent is a specialist agent as the Workbench settings list it.
type AgentKitAgent struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// AgentKitAgents lists the RF Swift specialist agents ("Start as" setting).
func (a *App) AgentKitAgents() ([]AgentKitAgent, error) {
	agents, err := agentKitAgents()
	if err != nil {
		return nil, err
	}
	out := make([]AgentKitAgent, 0, len(agents))
	for _, ag := range agents {
		out = append(out, AgentKitAgent{Name: ag.Name, Description: firstSentence(ag.Description)})
	}
	return out, nil
}

// agentStartAsArgs returns the arguments that start a client as an RF Swift
// specialist: Mistral Vibe with --agent (the kit's .vibe/agents files) and Kimi
// Code with --agent-file. Other clients delegate to the specialists as
// subagents instead, so a choice for them is refused.
func agentStartAsArgs(agentDir, clientID, name string) ([]string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, nil
	}
	agents, err := agentKitAgents()
	if err != nil {
		return nil, err
	}
	known := false
	for _, ag := range agents {
		known = known || ag.Name == name
	}
	if !known {
		return nil, fmt.Errorf("unknown RF Swift agent %q: choose another one in the Agent settings", name)
	}
	switch clientID {
	case "mistral":
		return []string{"--agent", name}, nil
	case "kimi":
		return []string{"--agent-file", filepath.Join(agentDir, ".kimi-code", "agents", name+".md")}, nil
	}
	return nil, fmt.Errorf("starting as an RF Swift agent is available for Mistral Vibe and Kimi Code; %s delegates to them as subagents", clientID)
}
