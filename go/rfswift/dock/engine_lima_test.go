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

func TestParseLimaUSB(t *testing.T) {
	info := "  Device 0.1, Port 1, Speed 480 Mb/s, Product RTL2838UHIDIR, ID: usb-0bda-2838\r\n" +
		"  Device 0.2, Port 2, Speed 480 Mb/s, Product HackRF One, ID: usb-1D50-6089\r\n" +
		"  Device 0.3, Port 3, Speed 12 Mb/s, Product QEMU USB Tablet, ID: tablet\r\n" +
		"  Device 0.4, Port 4, Speed 480 Mb/s, Product RTL2838UHIDIR, ID: usb-0bda-2838\r\n"
	got := parseLimaUSB(info)
	want := [][2]string{{"0x0bda", "0x2838"}, {"0x1d50", "0x6089"}}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
	if parseLimaUSB("") != nil || parseLimaUSB("No USB devices attached") != nil {
		t.Fatal("empty info yielded devices")
	}
}
