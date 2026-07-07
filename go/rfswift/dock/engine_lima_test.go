/* This code is part of RF Swift by @Penthertz
*  Tests for the Lima engine's GPU-variant selection (--gpu / rfswift-gpu).
 */

package dock

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLimaEngineIsGPU(t *testing.T) {
	cases := map[string]bool{
		"rfswift":     false,
		"rfswift-gpu": true,
		"myvm-gpu":    true,
		"gpu":         false, // must end in "-gpu", not just contain it
	}
	for inst, want := range cases {
		t.Setenv("RFSWIFT_LIMA_INSTANCE", inst)
		e := &LimaEngine{}
		if got := e.isGPU(); got != want {
			t.Errorf("isGPU(instance=%q) = %v, want %v", inst, got, want)
		}
	}
}

func TestUserTemplatePath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	t.Setenv("RFSWIFT_LIMA_INSTANCE", "rfswift")
	if got, want := (&LimaEngine{}).UserTemplatePath(),
		filepath.Join(home, ".config", "rfswift", "lima.yaml"); got != want {
		t.Errorf("UserTemplatePath (default) = %q, want %q", got, want)
	}

	t.Setenv("RFSWIFT_LIMA_INSTANCE", "rfswift-gpu")
	if got, want := (&LimaEngine{}).UserTemplatePath(),
		filepath.Join(home, ".config", "rfswift", "lima-gpu.yaml"); got != want {
		t.Errorf("UserTemplatePath (gpu) = %q, want %q", got, want)
	}
}

func TestFindLimaTemplateVariant(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfg := filepath.Join(home, ".config", "rfswift")
	if err := os.MkdirAll(cfg, 0755); err != nil {
		t.Fatal(err)
	}
	reg := filepath.Join(cfg, "lima.yaml")
	gpu := filepath.Join(cfg, "lima-gpu.yaml")
	if err := os.WriteFile(reg, []byte("vmType: qemu\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(gpu, []byte("vmType: krunkit\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if got := findLimaTemplate(false); got != reg {
		t.Errorf("findLimaTemplate(false) = %q, want %q", got, reg)
	}
	if got := findLimaTemplate(true); got != gpu {
		t.Errorf("findLimaTemplate(true) = %q, want %q", got, gpu)
	}
}
