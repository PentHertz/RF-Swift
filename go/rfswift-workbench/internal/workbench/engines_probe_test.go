package workbench

import (
	"os"
	"testing"
)

// TestEngineProbeManual is a manual integration probe: it talks to the real
// engines on this host. Run with RFSWIFT_ENGINE_PROBE=1.
func TestEngineProbeManual(t *testing.T) {
	if os.Getenv("RFSWIFT_ENGINE_PROBE") == "" {
		t.Skip("manual probe; set RFSWIFT_ENGINE_PROBE=1")
	}
	a := &App{eng: NewLocalEngine()}
	for _, e := range a.ContainerEngines() {
		t.Logf("engine %-6s state=%-13s available=%-5v running=%-5v active=%-5v instance=%s socket=%s", e.Name, e.State, e.Available, e.Running, e.Active, e.Instance, e.Socket)
	}
	targets, err := a.eng.ListTargets()
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range targets {
		t.Logf("target %-20s engine=%-6s status=%-8s image=%s", m.ID, m.Engine, m.Status, m.Image)
	}
}

// TestLimaOpsProbeManual starts, execs into, and stops a container hosted in
// the Lima VM through the routed engine paths. Run with
// RFSWIFT_ENGINE_PROBE=1 RFSWIFT_PROBE_LIMA_TARGET=<container>.
func TestLimaOpsProbeManual(t *testing.T) {
	target := os.Getenv("RFSWIFT_PROBE_LIMA_TARGET")
	if os.Getenv("RFSWIFT_ENGINE_PROBE") == "" || target == "" {
		t.Skip("manual probe; set RFSWIFT_ENGINE_PROBE=1 and RFSWIFT_PROBE_LIMA_TARGET")
	}
	eng := NewLocalEngine()
	m, err := eng.Inspect(target)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("inspect: engine=%s status=%s image=%s net=%s", m.Engine, m.Status, m.Image, m.Net)
	if err := eng.Start(target); err != nil {
		t.Fatal(err)
	}
	t.Log("started")
	out, err := eng.Exec(target, "uname -a && cat /etc/os-release | head -2")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("exec output: %s", out)
	if err := eng.Stop(target); err != nil {
		t.Fatal(err)
	}
	t.Log("stopped (restored to original state)")
}

// TestUSBProbeManual lists host USB devices and their Lima attach state.
// Run with RFSWIFT_ENGINE_PROBE=1.
func TestUSBProbeManual(t *testing.T) {
	if os.Getenv("RFSWIFT_ENGINE_PROBE") == "" {
		t.Skip("manual probe; set RFSWIFT_ENGINE_PROBE=1")
	}
	a := &App{eng: NewLocalEngine()}
	t.Logf("USBSupported=%v", a.USBSupported())
	devs, err := a.ListHostUSB()
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range devs {
		t.Logf("usb %-32s %s:%s attached=%v serial=%s", d.Name, d.VendorID, d.ProductID, d.Attached, d.Serial)
	}
}
