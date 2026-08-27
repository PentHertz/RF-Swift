package workbench

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// exportMissionNotes writes all portable Workbench-owned mission data next to
// the target archive. Credential values remain in the OS vault, and secrets
// metadata is excluded because its vault references are machine-local.
func (a *App) exportMissionNotes(mission, targetArchive, password string) (string, error) {
	source := a.store.missionDir(a.ws, mission)
	entries, err := os.ReadDir(source)
	if os.IsNotExist(err) || (err == nil && len(entries) == 0) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	base := strings.TrimSuffix(targetArchive, ".age")
	destination := base + ".mission.zip"
	if password != "" {
		destination += ".age"
	}
	plain, cleanup, err := exportWorkingPath(destination, password)
	if err != nil {
		return "", err
	}
	defer cleanup()
	if err := writeMissionZip(source, plain); err != nil {
		return "", err
	}
	if password != "" {
		if err := encryptArchive(plain, destination, password); err != nil {
			return "", err
		}
	}
	return destination, nil
}

func writeMissionZip(source, destination string) (err error) {
	out, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	zw := zip.NewWriter(out)
	walkErr := filepath.Walk(source, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		rel, err := filepath.Rel(source, path)
		if err != nil || rel == "." || info.IsDir() {
			return err
		}
		if rel == "mission.json" || rel == "secrets.json" {
			return nil
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(filepath.Join("mission", rel))
		header.Method = zip.Deflate
		writer, err := zw.CreateHeader(header)
		if err != nil {
			return err
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(writer, in)
		closeErr := in.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
	zipErr := zw.Close()
	closeErr := out.Close()
	if walkErr != nil {
		return walkErr
	}
	if zipErr != nil {
		return zipErr
	}
	return closeErr
}

func notesCompanionPath(targetArchive string) string {
	base := strings.TrimSuffix(targetArchive, ".age")
	mission := base + ".mission.zip"
	if _, err := os.Stat(mission + ".age"); err == nil {
		return mission + ".age"
	}
	if _, err := os.Stat(mission); err == nil {
		return mission
	}
	plain := base + ".notes.zip"
	if _, err := os.Stat(plain + ".age"); err == nil {
		return plain + ".age"
	}
	if _, err := os.Stat(plain); err == nil {
		return plain
	}
	return ""
}

func (a *App) HasMissionCompanion(targetArchive string) bool {
	return notesCompanionPath(targetArchive) != ""
}

// ImportTargetNotes replaces a mission notebook from the companion produced by
// ExportTarget. The UI must obtain overwrite consent before calling it.
func (a *App) ImportTargetNotes(mission, targetArchive, password string) error {
	if !validWorkspaceName(mission) {
		return errors.New("invalid mission name")
	}
	companion := notesCompanionPath(targetArchive)
	if companion == "" {
		return errors.New("no notes companion was found next to the imported archive")
	}
	input, cleanup, err := importWorkingPath(companion, password)
	if err != nil {
		return err
	}
	defer cleanup()
	staging, err := os.MkdirTemp("", "rfswift-notes-import-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	if err := extractNotesZip(input, staging); err != nil {
		return err
	}
	source := filepath.Join(staging, "mission")
	legacyNotesOnly := false
	if _, err := os.Stat(source); err != nil {
		source = filepath.Join(staging, "notes")
		legacyNotesOnly = true
		if _, err := os.Stat(source); err != nil {
			return errors.New("mission companion contains no portable mission data")
		}
	}
	destination := a.store.missionDir(a.ws, mission)
	if legacyNotesOnly {
		destination = a.store.notesDir(a.ws, mission)
	} else {
		// Keep the current target identity and machine-local credential-vault
		// references while replacing every portable archived artifact.
		for _, name := range []string{"mission.json", "secrets.json"} {
			current := filepath.Join(destination, name)
			if data, readErr := os.ReadFile(current); readErr == nil {
				if writeErr := os.WriteFile(filepath.Join(source, name), data, 0o600); writeErr != nil {
					return writeErr
				}
			}
		}
	}
	backup := destination + ".import-backup"
	_ = os.RemoveAll(backup)
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return err
	}
	if _, err := os.Stat(destination); err == nil {
		if err := os.Rename(destination, backup); err != nil {
			return err
		}
	}
	if err := os.Rename(source, destination); err != nil {
		_ = os.Rename(backup, destination)
		return err
	}
	return os.RemoveAll(backup)
}

func extractNotesZip(source, destination string) error {
	zr, err := zip.OpenReader(source)
	if err != nil {
		return fmt.Errorf("invalid notes companion: %w", err)
	}
	defer zr.Close()
	for _, entry := range zr.File {
		name := filepath.Clean(filepath.FromSlash(entry.Name))
		portable := strings.HasPrefix(name, "notes"+string(filepath.Separator)) || strings.HasPrefix(name, "mission"+string(filepath.Separator))
		if name == "." || !portable {
			return fmt.Errorf("unsafe notes archive entry %q", entry.Name)
		}
		if entry.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("notes archive contains a symlink: %q", entry.Name)
		}
		target := filepath.Join(destination, name)
		if entry.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o700); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		in, err := entry.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			in.Close()
			return err
		}
		_, copyErr := io.Copy(out, io.LimitReader(in, 256<<20))
		closeInErr, closeOutErr := in.Close(), out.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeInErr != nil {
			return closeInErr
		}
		if closeOutErr != nil {
			return closeOutErr
		}
	}
	return nil
}
