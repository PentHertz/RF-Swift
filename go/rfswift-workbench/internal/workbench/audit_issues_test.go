package workbench

import (
	"os"
	"path/filepath"
	"testing"
)

// TestReportLevelIssuesMatchPosture reproduces the nix report shape where the
// posture counts N string issues (via worst_severity) but there are no
// structured CVE objects — the detail list must still show those N issues, so
// the summary and detail agree.
func TestReportLevelIssuesMatchPosture(t *testing.T) {
	report := `{
      "worst_severity": "medium",
      "ok": false,
      "targets": [],
      "issues": [
        "pkgs/ contains 8 placeholder/unpinned source hash(es)",
        "env:sdr_light: could not realise closure"
      ]
    }`
	dir := t.TempDir()
	path := filepath.Join(dir, "report.json")
	if err := os.WriteFile(path, []byte(report), 0o644); err != nil {
		t.Fatal(err)
	}
	res := parseAuditFile(path)
	total := res.Posture.Crit + res.Posture.High + res.Posture.Med + res.Posture.Low
	if total != 2 || res.Posture.Med != 2 {
		t.Fatalf("posture = %+v, want 2 medium", res.Posture)
	}
	if len(res.Issues) != total {
		t.Fatalf("detail records = %d, posture total = %d; they must match", len(res.Issues), total)
	}
	for _, is := range res.Issues {
		if is.Severity != "medium" {
			t.Fatalf("issue severity = %q, want medium (to match posture bucketing)", is.Severity)
		}
		if is.Title == "" {
			t.Fatalf("issue has no title: %+v", is)
		}
	}
}
