package nix

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestPortableManifestExtrasCompatibility(t *testing.T) {
	want := exportManifest{
		Version:    2,
		StorePath:  "/nix/store/base-environment",
		ExtrasPath: "/nix/store/installed-tools-profile",
		Env:        Environment{Name: "radio", Image: "sdr_light"},
	}
	data, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	var got exportManifest
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.ExtrasPath != want.ExtrasPath {
		t.Fatalf("extras path was not preserved: got %q", got.ExtrasPath)
	}

	// Archives created before extras support have no extrasPath and remain valid.
	got = exportManifest{}
	if err := json.Unmarshal([]byte(`{"version":1,"storePath":"/nix/store/base","env":{"name":"legacy"}}`), &got); err != nil {
		t.Fatal(err)
	}
	if got.ExtrasPath != "" {
		t.Fatalf("legacy manifest unexpectedly acquired extras path %q", got.ExtrasPath)
	}
}

func TestExtractPortableArchiveRejectsTraversal(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "hostile.rfenv")
	f, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	data := []byte("escape")
	if err := tw.WriteHeader(&tar.Header{Name: "../escape", Mode: 0o600, Size: int64(len(data)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if err := extractPortableArchive(archive, t.TempDir()); err == nil {
		t.Fatal("portable archive traversal was accepted")
	}
}

func TestExtractPortableArchiveRejectsEscapingSymlink(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "hostile-link.rfenv")
	f, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: "workspace/link", Linkname: "../../outside", Typeflag: tar.TypeSymlink}); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if err := extractPortableArchive(archive, t.TempDir()); err == nil {
		t.Fatal("portable archive escaping symlink was accepted")
	}
}

func TestExtractPortableArchiveRejectsOversizedManifest(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "oversized.rfenv")
	f, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: "manifest.json", Mode: 0o600, Size: maxPortableManifestSize + 1, Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(make([]byte, maxPortableManifestSize+1)); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if err := extractPortableArchive(archive, t.TempDir()); err == nil {
		t.Fatal("portable archive with oversized manifest was accepted")
	}
}

func TestValidateImportedStorePath(t *testing.T) {
	valid := "/nix/store/0123456789abcdfghijklmnpqrsvwxyz-environment"
	if err := validateImportedStorePath(valid); err != nil {
		t.Fatalf("valid store path rejected: %v", err)
	}
	for _, path := range []string{"", "nix/store/0123456789abcdfghijklmnpqrsvwxyz-x", "/nix/store/../../tmp/x", "/nix/store/0123456789abcdfghijklmnpqrsvwxyz-x/bin"} {
		if err := validateImportedStorePath(path); err == nil {
			t.Fatalf("unsafe store path %q was accepted", path)
		}
	}
}
