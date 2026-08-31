package workbench

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Store is the on-disk workspace: a dedicated, portable directory holding every
// mission and everything it produces. Layout:
//
//	<root>/<workspace>/
//	  workspace.json
//	  missions/<id>/
//	    mission.json
//	    notes/<name>.md
//	    captures/<name>          + captures/<name>.meta.json
//	    findings.json
//	    reports/<file>
type Store struct {
	Root string
}

// NewStore returns a store rooted at root, or ~/.rfswift/workspaces when empty.
func NewStore(root string) *Store {
	if root == "" {
		home, err := os.UserHomeDir()
		if err == nil {
			root = filepath.Join(home, ".rfswift", "workspaces")
		} else {
			root = filepath.Join(".", "rfswift-workspaces")
		}
	}
	return &Store{Root: root}
}

func (s *Store) wsDir(ws string) string          { return filepath.Join(s.Root, ws) }
func (s *Store) missionsDir(ws string) string    { return filepath.Join(s.wsDir(ws), "missions") }
func (s *Store) missionDir(ws, id string) string { return filepath.Join(s.missionsDir(ws), id) }
func (s *Store) notesDir(ws, id string) string   { return filepath.Join(s.missionDir(ws, id), "notes") }
func (s *Store) capturesDir(ws, id string) string {
	return filepath.Join(s.missionDir(ws, id), "captures")
}
func (s *Store) reportsDir(ws, id string) string {
	return filepath.Join(s.missionDir(ws, id), "reports")
}
func (s *Store) findingsPath(ws, id string) string {
	return filepath.Join(s.missionDir(ws, id), "findings.json")
}

// ListWorkspaces returns the workspace names under the root.
func (s *Store) ListWorkspaces() ([]string, error) {
	entries, err := os.ReadDir(s.Root)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out, nil
}

// CreateWorkspace creates the directory skeleton for a new workspace.
func (s *Store) CreateWorkspace(name string) error {
	if !validWorkspaceName(name) {
		return errors.New("workspace name must be a single non-empty path component")
	}
	if err := os.MkdirAll(s.missionsDir(name), 0o700); err != nil {
		return err
	}
	meta := map[string]any{"name": name, "created": now()}
	return writeJSON(filepath.Join(s.wsDir(name), "workspace.json"), meta)
}

// DeleteWorkspace permanently removes one validated Workbench project tree.
func (s *Store) DeleteWorkspace(name string) error {
	if !validWorkspaceName(name) {
		return errors.New("workspace name must be a single non-empty path component")
	}
	return os.RemoveAll(s.wsDir(name))
}

// DeleteMission removes only Workbench-owned data for one mission. External
// bind-mounted workspaces, container volumes and images are deliberately not
// followed or removed here.
func (s *Store) DeleteMission(ws, id string) error {
	if !validWorkspaceName(ws) || !validWorkspaceName(id) {
		return errors.New("workspace and mission IDs must be single non-empty path components")
	}
	return os.RemoveAll(s.missionDir(ws, id))
}

// ListMissions reads the mission.json of each mission in a workspace.
func (s *Store) ListMissions(ws string) ([]Mission, error) {
	if !validWorkspaceName(ws) {
		return nil, errors.New("workspace name must be a single non-empty path component")
	}
	entries, err := os.ReadDir(s.missionsDir(ws))
	if err != nil {
		if os.IsNotExist(err) {
			return []Mission{}, nil
		}
		return nil, err
	}
	var out []Mission
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		var m Mission
		if err := readJSON(filepath.Join(s.missionDir(ws, e.Name()), "mission.json"), &m); err == nil {
			out = append(out, m)
		}
	}
	return out, nil
}

// SaveMission writes a mission's mission.json and ensures its subdirs exist.
func (s *Store) SaveMission(ws string, m Mission) error {
	if !validWorkspaceName(ws) || !validWorkspaceName(m.ID) {
		return errors.New("workspace and mission IDs must be single non-empty path components")
	}
	for _, d := range []string{s.notesDir(ws, m.ID), s.capturesDir(ws, m.ID), s.reportsDir(ws, m.ID)} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return err
		}
	}
	// Creation warnings and the live property sheet describe the engine state
	// at one moment; they are re-read from the engine, not from the record.
	m.Warnings = nil
	m.Summary = nil
	return writeJSON(filepath.Join(s.missionDir(ws, m.ID), "mission.json"), m)
}

// GetNote returns a note's markdown body ("" if it does not exist yet).
func (s *Store) GetNote(ws, id, name string) (string, error) {
	if err := validateMissionPath(ws, id); err != nil {
		return "", err
	}
	b, err := os.ReadFile(filepath.Join(s.notesDir(ws, id), safeName(name)))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(b), nil
}

// SaveNote writes a note's markdown body.
func (s *Store) SaveNote(ws, id, name, body string) error {
	if err := validateMissionPath(ws, id); err != nil {
		return err
	}
	if err := os.MkdirAll(s.notesDir(ws, id), 0o700); err != nil {
		return err
	}
	return writePrivateFile(filepath.Join(s.notesDir(ws, id), safeName(name)), []byte(body))
}

// ListNotes returns the note file names in a mission's notebook.
func (s *Store) ListNotes(ws, id string) ([]string, error) {
	if err := validateMissionPath(ws, id); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(s.notesDir(ws, id))
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out, nil
}

// LoadFindings reads a mission's findings.json.
func (s *Store) LoadFindings(ws, id string) ([]Finding, error) {
	if err := validateMissionPath(ws, id); err != nil {
		return nil, err
	}
	var fs []Finding
	err := readJSON(s.findingsPath(ws, id), &fs)
	if err != nil {
		if os.IsNotExist(err) {
			return []Finding{}, nil
		}
		return nil, err
	}
	return fs, nil
}

// SaveFindings writes a mission's findings.json.
func (s *Store) SaveFindings(ws, id string, fs []Finding) error {
	if err := validateMissionPath(ws, id); err != nil {
		return err
	}
	if err := os.MkdirAll(s.missionDir(ws, id), 0o700); err != nil {
		return err
	}
	return writeJSON(s.findingsPath(ws, id), fs)
}

// ListCaptures reads the *.meta.json sidecars in a mission's captures/.
func (s *Store) ListCaptures(ws, id string) ([]Capture, error) {
	if err := validateMissionPath(ws, id); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(s.capturesDir(ws, id))
	if err != nil {
		if os.IsNotExist(err) {
			return []Capture{}, nil
		}
		return nil, err
	}
	var out []Capture
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".meta.json") {
			var c Capture
			if err := readJSON(filepath.Join(s.capturesDir(ws, id), e.Name()), &c); err == nil {
				out = append(out, c)
			}
		}
	}
	return out, nil
}

// AddCapture writes a capture's metadata sidecar (the file itself is expected to
// already sit in captures/, or be copied there by the caller).
func (s *Store) AddCapture(ws, id string, c Capture) error {
	if err := validateMissionPath(ws, id); err != nil {
		return err
	}
	if err := os.MkdirAll(s.capturesDir(ws, id), 0o700); err != nil {
		return err
	}
	return writeJSON(filepath.Join(s.capturesDir(ws, id), safeName(c.Name)+".meta.json"), c)
}

// ImportCapture copies an existing artifact into the workspace and updates c.Path
// to the resulting workspace-relative path before saving its metadata.
func (s *Store) ImportCapture(ws, id string, c *Capture) error {
	if err := validateMissionPath(ws, id); err != nil {
		return err
	}
	if c == nil || c.Path == "" {
		return nil
	}
	if err := os.MkdirAll(s.capturesDir(ws, id), 0o700); err != nil {
		return err
	}
	src, err := os.Open(c.Path)
	if err != nil {
		return err
	}
	defer src.Close()
	name := safeName(c.Name)
	dstPath := filepath.Join(s.capturesDir(ws, id), name)
	dst, err := os.OpenFile(dstPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(dst, src)
	closeErr := dst.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	c.Name = name
	c.Path = filepath.Join("captures", name)
	return s.AddCapture(ws, id, *c)
}

// SaveReport writes a generated report into a mission's reports/ and returns the
// absolute path.
func (s *Store) SaveReport(ws, id, filename string, data []byte) (string, error) {
	if err := validateMissionPath(ws, id); err != nil {
		return "", err
	}
	if err := os.MkdirAll(s.reportsDir(ws, id), 0o700); err != nil {
		return "", err
	}
	p := filepath.Join(s.reportsDir(ws, id), safeName(filename))
	if err := writePrivateFile(p, data); err != nil {
		return "", err
	}
	return p, nil
}

// --- helpers ---

func now() string { return time.Now().UTC().Format(time.RFC3339) }

func safeName(name string) string {
	name = filepath.Base(name)
	if name == "." || name == "/" || name == "" {
		return "unnamed"
	}
	return name
}

func validWorkspaceName(name string) bool {
	return name != "" && name != "." && name != ".." && filepath.Base(name) == name
}

func validateMissionPath(ws, id string) error {
	if !validWorkspaceName(ws) || !validWorkspaceName(id) {
		return errors.New("workspace and mission IDs must be single non-empty path components")
	}
	return nil
}

func writeJSON(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return writePrivateFile(path, b)
}

func writePrivateFile(path string, data []byte) error {
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

// SecurePermissions migrates only the Workbench data tree. It does not touch
// external mission bind mounts such as ~/rfswift-workspace.
func (s *Store) SecurePermissions() error {
	if err := os.MkdirAll(s.Root, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(s.Root, 0o700); err != nil {
		return err
	}
	return filepath.WalkDir(s.Root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if entry.IsDir() {
			return os.Chmod(path, 0o700)
		}
		if entry.Type().IsRegular() {
			return os.Chmod(path, 0o600)
		}
		return nil
	})
}

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}
