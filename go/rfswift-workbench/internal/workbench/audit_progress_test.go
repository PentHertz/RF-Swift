package workbench

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	rfnix "penthertz/rfswift/nix"
)

func TestAuditStageForLineFollowsTheScript(t *testing.T) {
	lines := []string{
		"RF Swift Nix security audit",
		"=== repository supply-chain & configuration ===",
		"  ✅ flake.lock:  all inputs pinned by narHash",
		"realising env:sdr_light (downloads or builds the closure when it is not in the store yet)",
		"=== env:sdr_light ===",
		"  closure: /nix/store/xxx-rfswift-sdr_light",
		"  ⚠️  vulnix:      162 CVE match(es) -> /x/vulnix-env_sdr_light.json",
		"  ✅ syft:        SBOM -> /x/sbom.cdx.json (0 components)",
		"  ✅ grype:       no findings -> /x/grype.json",
		"  ✅ osv-scanner: no advisories",
		"  ✅ integrity:   closure verified, no corruption",
		"  ℹ️  provenance:  0 closure path(s) without a trusted signature",
		"  ✅ config:      no evaluation warnings",
		"================ AUDIT REPORT ================",
	}
	var percents []int
	for _, line := range lines {
		if st, ok := auditStageForLine(line); ok {
			percents = append(percents, st.Percent)
		}
	}
	want := []int{12, 15, 30, 45, 55, 62, 70, 85, 92, 96}
	if len(percents) != len(want) {
		t.Fatalf("stages = %v, want %v", percents, want)
	}
	for i := range want {
		if percents[i] != want[i] {
			t.Fatalf("stages = %v, want %v", percents, want)
		}
	}
}

// The follower skips what an earlier audit left in summary.txt, announces the
// realise step (with the lazy-environment warning) and reports each new
// stage once, in order.
func TestFollowAuditSummaryReportsNewStages(t *testing.T) {
	dir := t.TempDir()
	summary := filepath.Join(dir, "summary.txt")
	if err := os.WriteFile(summary, []byte("=== env:old ===\n  ✅ vulnix: old run\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var got []string
	e := &LocalEngine{AuditProgress: func(mission, stage string, percent int) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, stage)
	}}
	stop := e.followAuditSummary("m", &rfnix.Environment{Image: "sdr_light", Lazy: true}, summary)
	f, _ := os.OpenFile(summary, os.O_APPEND|os.O_WRONLY, 0o600)
	f.WriteString("realising env:sdr_light (downloads ...)\n")
	f.WriteString("=== env:sdr_light ===\n  closure: /nix/store/x\n  ⚠️  vulnix:      3 CVE match(es)\n")
	f.Close()
	time.Sleep(1500 * time.Millisecond)
	stop()
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 4 {
		t.Fatalf("stages = %q", got)
	}
	if got[0] == "" || got[0][:len("Realising the full sdr_light")] != "Realising the full sdr_light" {
		t.Fatalf("lazy realise must be announced first: %q", got[0])
	}
	if got[3] != "Generating the SBOM (syft)" {
		t.Fatalf("last stage = %q", got[3])
	}
}
