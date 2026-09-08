package workbench

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// A Nix audit whose CVE scanner failed (vulnix racing on its database on the
// first run) reports zero CVEs: the result must say the audit is incomplete
// rather than let a zero pass for clean.
func TestParseAuditFileReportsScannersThatDidNotComplete(t *testing.T) {
	report := `{"tool":"rfswift security-audit","worst_severity":"medium","ok":false,
	  "issues":["pkgs/ contains 8 placeholder/unpinned source hash(es)","env:ad: vulnix did not complete (tool/database error)"],
	  "targets":[{"label":"env:ad","vulnix_cves":-1,"sbom_components":0,"grype":{"critical":0,"high":0,"medium":0,"low":0},"osv_advisories":-1,"vulnerabilities":[]}]}`
	path := filepath.Join(t.TempDir(), "report.json")
	if err := os.WriteFile(path, []byte(report), 0o600); err != nil {
		t.Fatal(err)
	}
	r := parseAuditFile(path)
	want := []string{"env:ad: vulnix did not complete (tool/database error)", "env:ad: osv-scanner did not complete"}
	if !reflect.DeepEqual(r.ScannerErrors, want) {
		t.Fatalf("scanner errors = %#v, want %#v", r.ScannerErrors, want)
	}
	if r.Posture.Med != 2 {
		t.Fatalf("posture = %+v", r.Posture)
	}
	clean := `{"worst_severity":"high","ok":false,"issues":["env:ad: vulnix reports 3 CVE match(es)"],
	  "targets":[{"label":"env:ad","vulnix_cves":3,"grype":{"critical":0},"osv_advisories":0,"vulnerabilities":[{"id":"CVE-1","severity":"high","package":"x","runtime_closure":true}]}]}`
	if err := os.WriteFile(path, []byte(clean), 0o600); err != nil {
		t.Fatal(err)
	}
	if r := parseAuditFile(path); len(r.ScannerErrors) != 0 {
		t.Fatalf("a complete report must not list scanner errors: %v", r.ScannerErrors)
	}
}
