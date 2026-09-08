package dock

import (
	"archive/tar"
	"bytes"
	"errors"
	"io"
	"os"
	"testing"
)

func TestUpgradeArchivePreservesMetadata(t *testing.T) {
	var b bytes.Buffer
	tw := tar.NewWriter(&b)
	body := []byte("#!/bin/sh\nexit 0\n")
	for _, h := range []*tar.Header{
		{Name: "repo/run", Typeflag: tar.TypeReg, Mode: 0755, Uid: 123, Gid: 456, Size: int64(len(body))},
		{Name: "repo/alias", Typeflag: tar.TypeSymlink, Linkname: "run", Mode: 0777},
		{Name: "repo/hard", Typeflag: tar.TypeLink, Linkname: "repo/run", Mode: 0755},
	} {
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if h.Typeflag == tar.TypeReg {
			if _, err := tw.Write(body); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	want := append([]byte(nil), b.Bytes()...)
	path, err := saveUpgradeArchive(&b, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("upgrade archive did not preserve the exact contents and metadata")
	}
}

type failingArchiveReader struct{}

func (failingArchiveReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func TestUpgradeArchiveRejectsIncompleteBackup(t *testing.T) {
	dir := t.TempDir()
	path, err := saveUpgradeArchive(io.MultiReader(bytes.NewBufferString("partial"), failingArchiveReader{}), dir)
	if !errors.Is(err, io.ErrUnexpectedEOF) || path != "" {
		t.Fatalf("partial archive accepted: %q %v", path, err)
	}
	files, err := os.ReadDir(dir)
	if err != nil || len(files) != 0 {
		t.Fatalf("partial backup left behind: %v %v", files, err)
	}
}
