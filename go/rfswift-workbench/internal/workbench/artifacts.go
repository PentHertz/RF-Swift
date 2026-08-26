package workbench

import (
	"bufio"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

const artifactPreviewLimit = 256 << 10

var (
	ansiCSI = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)
	ansiOSC = regexp.MustCompile(`\x1b\][^\x07]*(?:\x07|\x1b\\)`)
	ansiESC = regexp.MustCompile(`\x1b[()][0-2A-Z0-9]|\x1b[@-_]`)
)

func (a *App) missionWorkspace(mission string) (string, error) {
	if err := a.requireMission(mission); err != nil {
		return "", err
	}
	m, err := a.eng.Inspect(mission)
	if err != nil {
		return "", err
	}
	for _, mount := range m.Mounts {
		if strings.Contains(mount, " -> /workspace ") {
			return strings.TrimSpace(strings.SplitN(mount, " -> ", 2)[0]), nil
		}
	}
	if m.Engine == "nix" && len(m.Mounts) > 0 && m.Mounts[0] != "" && m.Mounts[0] != "none" {
		return m.Mounts[0], nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "rfswift-workspace", mission), nil
}

func (a *App) workspaceArtifactPath(mission, relative string) (string, error) {
	root, err := a.missionWorkspace(mission)
	if err != nil {
		return "", err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return "", err
	}
	relative = filepath.Clean(filepath.FromSlash(relative))
	if relative == "." || filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("artifact path escapes the mission workspace")
	}
	path := filepath.Join(root, relative)
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	if resolved != root && !strings.HasPrefix(resolved, root+string(filepath.Separator)) {
		return "", errors.New("artifact symlink escapes the mission workspace")
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("artifact is not a regular file")
	}
	return resolved, nil
}

func (a *App) ListWorkspaceArtifacts(mission string) ([]WorkspaceArtifact, error) {
	if remoteEngine, ok := a.eng.(*RemoteEngine); ok {
		var out []WorkspaceArtifact
		if err := remoteEngine.call("artifacts.list", map[string]string{"mission": mission}, &out); err != nil {
			return nil, err
		}
		registered, _ := a.store.ListCaptures(a.ws, mission)
		bySource := map[string]Capture{}
		for _, c := range registered {
			bySource[c.Meta["Workspace path"]] = c
		}
		for i := range out {
			if c, yes := bySource[out[i].Path]; yes {
				out[i].Registered = true
				out[i].AIAllowed = c.Meta["AI content access"] == "approved"
			}
			out[i].Type = ClassifyCapture(out[i].Path)
		}
		return out, nil
	}
	root, err := a.missionWorkspace(mission)
	if err != nil {
		return nil, err
	}
	registered, _ := a.store.ListCaptures(a.ws, mission)
	bySource := map[string]Capture{}
	for _, c := range registered {
		if source := c.Meta["Workspace path"]; source != "" {
			bySource[filepath.ToSlash(source)] = c
		}
	}
	var out []WorkspaceArtifact
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			if path != root && strings.HasPrefix(entry.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !entry.Type().IsRegular() || len(out) >= 5000 {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		cap, exists := bySource[rel]
		out = append(out, WorkspaceArtifact{Path: rel, Name: entry.Name(), Size: info.Size(), Modified: info.ModTime().UTC().Format("2006-01-02T15:04:05Z"), Type: ClassifyCapture(rel), Registered: exists, AIAllowed: exists && cap.Meta["AI content access"] == "approved"})
		return nil
	})
	if os.IsNotExist(err) {
		return []WorkspaceArtifact{}, nil
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, err
}

func readArtifactText(path string) (string, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", false, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, artifactPreviewLimit+1))
	if err != nil {
		return "", false, err
	}
	truncated := len(b) > artifactPreviewLimit
	if truncated {
		b = b[:artifactPreviewLimit]
	}
	if !utf8.Valid(b) || strings.IndexByte(string(b), 0) >= 0 {
		return "", false, errors.New("binary artifact: use its recommended analysis tool")
	}
	return string(b), truncated, nil
}

// readArtifactAIContent converts structured text evidence into useful,
// terminal-safe content before exposing it to a mission-scoped AI client.
func readArtifactAIContent(path string) (string, bool, error) {
	if strings.EqualFold(filepath.Ext(path), ".cast") {
		return readAsciinemaTranscript(path)
	}
	return readArtifactText(path)
}

func readAsciinemaTranscript(path string) (string, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", false, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	// Individual terminal writes can be large; cap a single JSON record while
	// keeping the generated transcript itself bounded by artifactPreviewLimit.
	scanner.Buffer(make([]byte, 64<<10), 2<<20)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return "", false, err
		}
		return "", false, errors.New("invalid asciinema recording: missing header")
	}
	var header struct {
		Version int `json:"version"`
		Width   int `json:"width"`
		Height  int `json:"height"`
	}
	if err := json.Unmarshal(scanner.Bytes(), &header); err != nil || header.Version != 2 {
		return "", false, errors.New("invalid asciinema recording: expected a v2 header")
	}

	var out strings.Builder
	fmt.Fprintf(&out, "Terminal recording (asciinema v2, %dx%d)\n", header.Width, header.Height)
	lastKind := ""
	truncated := false
	for scanner.Scan() {
		var event []json.RawMessage
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil || len(event) != 3 {
			return "", false, errors.New("invalid asciinema recording: malformed event")
		}
		var kind, payload string
		if json.Unmarshal(event[1], &kind) != nil || json.Unmarshal(event[2], &payload) != nil {
			return "", false, errors.New("invalid asciinema recording: malformed event fields")
		}
		if kind != "i" && kind != "o" {
			continue
		}
		payload = cleanTerminalTranscript(payload)
		if payload == "" {
			continue
		}
		if kind != lastKind {
			label := "output"
			if kind == "i" {
				label = "input"
			}
			fmt.Fprintf(&out, "\n[%s]\n", label)
			lastKind = kind
		}
		out.WriteString(payload)
		if out.Len() > artifactPreviewLimit {
			truncated = true
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return "", false, fmt.Errorf("invalid asciinema recording: %w", err)
	}
	result := out.String()
	if len(result) > artifactPreviewLimit {
		result = result[:artifactPreviewLimit]
		for !utf8.ValidString(result) {
			result = result[:len(result)-1]
		}
	}
	return result, truncated, nil
}

func cleanTerminalTranscript(value string) string {
	value = ansiOSC.ReplaceAllString(value, "")
	value = ansiCSI.ReplaceAllString(value, "")
	value = ansiESC.ReplaceAllString(value, "")
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' || r >= 0x20 {
			return r
		}
		return -1
	}, value)
}

func (a *App) PreviewWorkspaceArtifact(mission, relative string) (ArtifactPreview, error) {
	if remoteEngine, ok := a.eng.(*RemoteEngine); ok {
		data, truncated, err := remoteArtifactBytes(remoteEngine, mission, relative)
		if err != nil {
			return ArtifactPreview{}, err
		}
		if len(data) > artifactPreviewLimit {
			data = data[:artifactPreviewLimit]
			truncated = true
		}
		return artifactPreview(relative, data, truncated), nil
	}
	path, err := a.workspaceArtifactPath(mission, relative)
	if err != nil {
		return ArtifactPreview{}, err
	}
	f, err := os.Open(path)
	if err != nil {
		return ArtifactPreview{}, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, artifactPreviewLimit+1))
	if err != nil {
		return ArtifactPreview{}, err
	}
	truncated := len(data) > artifactPreviewLimit
	if truncated {
		data = data[:artifactPreviewLimit]
	}
	return artifactPreview(relative, data, truncated), nil
}

func artifactPreview(path string, data []byte, truncated bool) ArtifactPreview {
	if utf8.Valid(data) && !strings.ContainsRune(string(data), 0) {
		return ArtifactPreview{Path: path, Kind: "text", Content: string(data), Truncated: truncated}
	}
	return ArtifactPreview{Path: path, Kind: "binary", Content: base64.StdEncoding.EncodeToString(data), Truncated: truncated}
}

func (a *App) RegisterWorkspaceArtifact(mission, relative string, allowAI bool) (Capture, error) {
	if remoteEngine, ok := a.eng.(*RemoteEngine); ok {
		return a.registerRemoteArtifact(remoteEngine, mission, relative, allowAI)
	}
	path, err := a.workspaceArtifactPath(mission, relative)
	if err != nil {
		return Capture{}, err
	}
	f, err := os.Open(path)
	if err != nil {
		return Capture{}, err
	}
	h := sha256.New()
	_, hashErr := io.Copy(h, f)
	_ = f.Close()
	if hashErr != nil {
		return Capture{}, hashErr
	}
	info, err := os.Stat(path)
	if err != nil {
		return Capture{}, err
	}
	meta := map[string]string{"Workspace path": filepath.ToSlash(relative), "Size": strconv.FormatInt(info.Size(), 10), "SHA-256": hex.EncodeToString(h.Sum(nil)), "AI content access": "denied"}
	if allowAI {
		if _, _, err := readArtifactAIContent(path); err != nil {
			return Capture{}, fmt.Errorf("cannot allow AI content access: %w", err)
		}
		meta["AI content access"] = "approved"
	}
	c := Capture{Mission: mission, Name: filepath.Base(path), Path: path, Type: ClassifyCapture(path), Tool: "workspace", Meta: meta, Note: "Registered from the live mission workspace"}
	if err := a.store.ImportCapture(a.ws, mission, &c); err != nil {
		if os.IsExist(err) {
			return Capture{}, errors.New("an artifact with this filename is already registered")
		}
		return Capture{}, err
	}
	return c, nil
}

func remoteArtifactBytes(engine *RemoteEngine, mission, relative string) ([]byte, bool, error) {
	var result struct {
		Data      string `json:"data"`
		Truncated bool   `json:"truncated"`
	}
	if err := engine.call("artifacts.read", map[string]string{"mission": mission, "path": relative}, &result); err != nil {
		return nil, false, err
	}
	data, err := base64.StdEncoding.DecodeString(result.Data)
	return data, result.Truncated, err
}

func (a *App) registerRemoteArtifact(engine *RemoteEngine, mission, relative string, allowAI bool) (Capture, error) {
	data, truncated, err := remoteArtifactBytes(engine, mission, relative)
	if err != nil {
		return Capture{}, err
	}
	if truncated {
		return Capture{}, errors.New("remote artifact exceeds the 16 MiB transfer limit")
	}
	if allowAI && (!utf8.Valid(data) || strings.IndexByte(string(data), 0) >= 0) {
		return Capture{}, errors.New("cannot allow AI content access for a binary artifact")
	}
	dir := a.store.capturesDir(a.ws, mission)
	if err = os.MkdirAll(dir, 0700); err != nil {
		return Capture{}, err
	}
	name := safeName(filepath.Base(relative))
	path := filepath.Join(dir, name)
	if _, err = os.Stat(path); err == nil {
		return Capture{}, errors.New("an artifact with this filename is already registered")
	}
	if err = os.WriteFile(path, data, 0600); err != nil {
		return Capture{}, err
	}
	sum := sha256.Sum256(data)
	meta := map[string]string{"Workspace path": filepath.ToSlash(relative), "Size": strconv.Itoa(len(data)), "SHA-256": hex.EncodeToString(sum[:]), "AI content access": "denied", "Remote source": engine.Config.Endpoint}
	if allowAI {
		meta["AI content access"] = "approved"
	}
	c := Capture{Mission: mission, Name: name, Path: path, Type: ClassifyCapture(relative), Tool: "remote workspace", Meta: meta, Note: "Copied from the authenticated remote mission workspace"}
	if err = a.store.AddCapture(a.ws, mission, c); err != nil {
		return Capture{}, err
	}
	return c, nil
}

func (a *App) AttachWorkspaceArtifactToNote(mission, relative string) (Capture, error) {
	c, err := a.RegisterWorkspaceArtifact(mission, relative, false)
	if err != nil {
		return Capture{}, err
	}
	body, err := a.store.GetNote(a.ws, mission, "note.md")
	if err != nil {
		return Capture{}, err
	}
	link := "[Artifact: " + c.Name + "](../captures/" + c.Name + ")"
	if body != "" && !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	body += "\n" + link + "\n"
	return c, a.store.SaveNote(a.ws, mission, "note.md", body)
}

func (a *App) SetArtifactAIContentAccess(mission, name string, allowed bool) error {
	name = safeName(name)
	captures, err := a.store.ListCaptures(a.ws, mission)
	if err != nil {
		return err
	}
	for _, c := range captures {
		if c.Name != name {
			continue
		}
		if allowed {
			if _, _, err := readArtifactAIContent(filepath.Join(a.store.capturesDir(a.ws, mission), name)); err != nil {
				return fmt.Errorf("cannot allow AI content access: %w", err)
			}
			c.Meta["AI content access"] = "approved"
		} else {
			c.Meta["AI content access"] = "denied"
		}
		return a.store.AddCapture(a.ws, mission, c)
	}
	return errors.New("registered artifact not found")
}
