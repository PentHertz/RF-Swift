package workbench

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

const maxProjectExpandedSize int64 = 20 << 30

// ExportWorkbenchProject writes the complete persisted workspace to a portable
// archive. Runtime images and Nix store paths remain references in mission.json.
func (a *App) ExportWorkbenchProject() (string, error) {
	if !validWorkspaceName(a.ws) {
		return "", errors.New("invalid current workspace")
	}
	destination, err := wruntime.SaveFileDialog(a.ctx, wruntime.SaveDialogOptions{
		Title:           "Export RF Swift Workbench project",
		DefaultFilename: a.ws + ".rfswift-workbench.zip",
		Filters:         []wruntime.FileFilter{{DisplayName: "RF Swift Workbench project", Pattern: "*.rfswift-workbench.zip"}},
	})
	if err != nil || destination == "" {
		return "", err
	}
	if err := writeProjectArchive(a.store.wsDir(a.ws), destination); err != nil {
		return "", err
	}
	return destination, nil
}

func writeProjectArchive(source, destination string) error {
	out, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	zw := zip.NewWriter(out)
	walkErr := filepath.Walk(source, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("project contains unsupported symbolic link %s", path)
		}
		rel, err := filepath.Rel(source, path)
		if err != nil || rel == "." {
			return err
		}
		name := filepath.ToSlash(rel)
		if info.IsDir() {
			_, err = zw.Create(name + "/")
			return err
		}
		h, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		h.Name, h.Method = name, zip.Deflate
		w, err := zw.CreateHeader(h)
		if err != nil {
			return err
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(w, in)
		closeErr := in.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
	closeZipErr := zw.Close()
	closeFileErr := out.Close()
	if walkErr != nil || closeZipErr != nil || closeFileErr != nil {
		_ = os.Remove(destination)
		if walkErr != nil {
			return walkErr
		}
		if closeZipErr != nil {
			return closeZipErr
		}
		return closeFileErr
	}
	return nil
}

// ImportWorkbenchProject safely restores an exported workspace and switches to
// it. Name collisions receive a deterministic -imported-N suffix.
func (a *App) ImportWorkbenchProject() (string, error) {
	archive, err := wruntime.OpenFileDialog(a.ctx, wruntime.OpenDialogOptions{
		Title:   "Import RF Swift Workbench project",
		Filters: []wruntime.FileFilter{{DisplayName: "RF Swift Workbench project", Pattern: "*.rfswift-workbench.zip;*.zip"}},
	})
	if err != nil || archive == "" {
		return "", err
	}
	name, err := importProjectArchive(a.store, archive)
	if err != nil {
		return "", err
	}
	a.ws = name
	return name, nil
}

func importProjectArchive(store *Store, archive string) (string, error) {
	zr, err := zip.OpenReader(archive)
	if err != nil {
		return "", fmt.Errorf("open project archive: %w", err)
	}
	defer zr.Close()
	name := strings.TrimSuffix(filepath.Base(archive), ".rfswift-workbench.zip")
	if raw, err := readZipEntry(zr.File, "workspace.json", 1<<20); err == nil {
		var meta struct {
			Name string `json:"name"`
		}
		if json.Unmarshal(raw, &meta) == nil && validWorkspaceName(meta.Name) {
			name = meta.Name
		}
	}
	if !validWorkspaceName(name) {
		name = "imported-project"
	}
	base := name
	for i := 1; ; i++ {
		if _, err := os.Stat(store.wsDir(name)); os.IsNotExist(err) {
			break
		}
		name = fmt.Sprintf("%s-imported-%d", base, i)
	}
	target := store.wsDir(name)
	if err := os.MkdirAll(target, 0o700); err != nil {
		return "", err
	}
	var expanded int64
	for _, f := range zr.File {
		clean := filepath.Clean(filepath.FromSlash(f.Name))
		if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			_ = os.RemoveAll(target)
			return "", fmt.Errorf("unsafe archive path %q", f.Name)
		}
		if f.Mode()&os.ModeSymlink != 0 {
			_ = os.RemoveAll(target)
			return "", fmt.Errorf("symbolic links are not allowed in project archives")
		}
		expanded += int64(f.UncompressedSize64)
		if expanded > maxProjectExpandedSize {
			_ = os.RemoveAll(target)
			return "", errors.New("project archive expands beyond the 20 GiB safety limit")
		}
		dst := filepath.Join(target, clean)
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(dst, 0o700); err != nil {
				_ = os.RemoveAll(target)
				return "", err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
			_ = os.RemoveAll(target)
			return "", err
		}
		r, err := f.Open()
		if err != nil {
			_ = os.RemoveAll(target)
			return "", err
		}
		w, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
		if err == nil {
			remaining := maxProjectExpandedSize - (expanded - int64(f.UncompressedSize64))
			written, copyErr := io.Copy(w, io.LimitReader(r, remaining+1))
			if copyErr != nil {
				err = copyErr
			} else if written > remaining {
				err = errors.New("project archive expands beyond the 20 GiB safety limit")
			}
			_ = w.Close()
		}
		_ = r.Close()
		if err != nil {
			_ = os.RemoveAll(target)
			return "", err
		}
	}
	if _, err := os.Stat(filepath.Join(target, "workspace.json")); err != nil {
		_ = os.RemoveAll(target)
		return "", errors.New("not an RF Swift Workbench project: workspace.json is missing")
	}
	return name, nil
}

func readZipEntry(files []*zip.File, name string, limit int64) ([]byte, error) {
	for _, f := range files {
		if filepath.ToSlash(filepath.Clean(f.Name)) != name || int64(f.UncompressedSize64) > limit {
			continue
		}
		r, err := f.Open()
		if err != nil {
			return nil, err
		}
		defer r.Close()
		return io.ReadAll(io.LimitReader(r, limit+1))
	}
	return nil, os.ErrNotExist
}
