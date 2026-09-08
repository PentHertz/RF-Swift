package workbench

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestParsePostureCountsFindingSeverityStrings(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "report.json")
	b, _ := json.Marshal(map[string]any{"findings": []any{map[string]any{"severity": "critical"}, map[string]any{"severity": "medium"}}})
	if err := os.WriteFile(p, b, 0o600); err != nil {
		t.Fatal(err)
	}
	got := parsePostureFile(p)
	if got.Crit != 1 || got.Med != 1 {
		t.Fatalf("unexpected posture: %#v", got)
	}
}

func TestParseAuditFilePreservesCVEAndComponentEvidence(t *testing.T) {
	p := filepath.Join(t.TempDir(), "report.json")
	data := `{"vulnerabilities":[{"cve":"CVE-2026-1234","severity":"high","title":"USB parser overflow","package":"librf","installed_version":"1.2.0","fixed_version":"1.2.1","scanner":"vulnix","cvss_score":8.1,"scope":"runtime","runtime_closure":true,"disposition":"scanner-match-unvalidated"},{"cve":"CVE-2026-9999","severity":"critical","package":"build-helper","scope":"build-time","runtime_closure":false}]}`
	if err := os.WriteFile(p, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	got := parseAuditFile(p)
	if len(got.Issues) != 2 {
		t.Fatalf("issues = %#v", got.Issues)
	}
	issue := got.Issues[0]
	if issue.ID != "CVE-2026-1234" || issue.Component != "librf" || issue.Installed != "1.2.0" || issue.Fixed != "1.2.1" || issue.Source != "vulnix" || issue.Score != "8.1" || issue.Scope != "runtime" || issue.Disposition != "scanner-match-unvalidated" {
		t.Fatalf("normalized issue = %#v", issue)
	}
	if got.Posture.High != 1 || got.Posture.Crit != 0 {
		t.Fatalf("build-time-only issue inflated runtime posture: %#v", got.Posture)
	}
}
