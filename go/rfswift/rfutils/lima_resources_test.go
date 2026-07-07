/* This code is part of RF Swift by @Penthertz
*  Tests for the Lima resource-editing helpers used by `rfswift engine lima set`.
 */

package rfutils

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetLimaResources_ReplacesAndPreserves(t *testing.T) {
	tmpl := `# header comment
vmType: qemu
video:
  display: "vnc"
cpus: 4
memory: "8GiB"
disk: "100GiB"
provision:
  - mode: system
    script: |
      # cpus: 999 nested must be untouched
      echo hi
`
	f := filepath.Join(t.TempDir(), "lima.yaml")
	if err := os.WriteFile(f, []byte(tmpl), 0644); err != nil {
		t.Fatal(err)
	}

	changes, err := SetLimaResources(f, 8, "16GiB", "200GiB")
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 3 {
		t.Fatalf("want 3 changes, got %v", changes)
	}

	b, _ := os.ReadFile(f)
	out := string(b)
	for _, w := range []string{
		"cpus: 8", `memory: "16GiB"`, `disk: "200GiB"`,
		"# header comment", `display: "vnc"`, "# cpus: 999 nested must be untouched",
	} {
		if !strings.Contains(out, w) {
			t.Errorf("output missing %q\n%s", w, out)
		}
	}
	if strings.Contains(out, "cpus: 4") {
		t.Errorf("old cpus value was not replaced")
	}
	if n := strings.Count(out, "cpus: 8"); n != 1 {
		t.Errorf("want exactly one top-level cpus, got %d", n)
	}
}

func TestSetLimaResources_PartialAndNoop(t *testing.T) {
	tmpl := "vmType: qemu\ncpus: 4\nmemory: \"8GiB\"\ndisk: \"100GiB\"\n"
	f := filepath.Join(t.TempDir(), "lima.yaml")
	if err := os.WriteFile(f, []byte(tmpl), 0644); err != nil {
		t.Fatal(err)
	}

	// Only memory requested - cpus/disk left untouched.
	changes, err := SetLimaResources(f, 0, "32GiB", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 {
		t.Fatalf("want 1 change, got %v", changes)
	}
	b, _ := os.ReadFile(f)
	if !strings.Contains(string(b), `memory: "32GiB"`) {
		t.Error("memory not updated")
	}
	if !strings.Contains(string(b), "cpus: 4") {
		t.Error("cpus should be untouched")
	}

	// Nothing requested - no-op, no changes reported.
	changes, err = SetLimaResources(f, 0, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if changes != nil {
		t.Errorf("want nil changes for no-op, got %v", changes)
	}
}

func TestSetLimaResources_AppendsMissing(t *testing.T) {
	tmpl := "vmType: krunkit\nmemory: \"8GiB\"\n" // no cpus/disk keys present
	f := filepath.Join(t.TempDir(), "lima.yaml")
	if err := os.WriteFile(f, []byte(tmpl), 0644); err != nil {
		t.Fatal(err)
	}

	changes, err := SetLimaResources(f, 6, "", "50GiB")
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 2 {
		t.Fatalf("want 2 changes, got %v", changes)
	}
	b, _ := os.ReadFile(f)
	out := string(b)
	if !strings.Contains(out, "cpus: 6") {
		t.Error("cpus not appended")
	}
	if !strings.Contains(out, `disk: "50GiB"`) {
		t.Error("disk not appended")
	}
}

func TestIsValidLimaSize(t *testing.T) {
	valid := []string{"8GiB", "16GiB", "512MiB", "100GB", "2.5GiB", "1024", "1T"}
	invalid := []string{"", "GiB", "abc", "16 GiB", "1..2GiB", "-4GiB", "8gib?"}
	for _, v := range valid {
		if !IsValidLimaSize(v) {
			t.Errorf("want valid: %q", v)
		}
	}
	for _, v := range invalid {
		if IsValidLimaSize(v) {
			t.Errorf("want invalid: %q", v)
		}
	}
}

func TestCopyFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	if err := os.WriteFile(src, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	// Destination directory does not exist yet - CopyFile must create it.
	dst := filepath.Join(dir, "sub", "nested", "dst.txt")
	if err := CopyFile(src, dst); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "hello" {
		t.Errorf("content mismatch: %q", b)
	}
}
