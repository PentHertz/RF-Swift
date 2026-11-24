/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
 */

package workbench

import (
	"embed"
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
		dir      string
		opencode bool
	}{{filepath.Join(agentDir, ".claude", "agents"), false}, {filepath.Join(agentDir, ".opencode", "agents"), true}} {
		if err := os.MkdirAll(target.dir, 0o700); err != nil {
			return nil, nil, err
		}
		keep := map[string]bool{}
		for _, ag := range agents {
			keep[ag.Name+".md"] = true
			data := ag.body
			if target.opencode {
				data = openCodeAgent(ag)
			}
			if err := os.WriteFile(filepath.Join(target.dir, ag.Name+".md"), data, 0o600); err != nil {
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
