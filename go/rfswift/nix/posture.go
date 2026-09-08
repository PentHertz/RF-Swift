/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
*
*  Nix engine - security posture surfacing.
*
*  Makes a tool set's security posture routine: `rfswift nix audit <name>` writes
*  a machine-readable report into the environment's own state dir, and this reads
*  it back so `rfswift nix info` and the post-build summary can show a one-line
*  posture without re-running the (slow, network-bound) scanners.
 */

package nix

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// EnvReportDir is the canonical location for an environment's audit report, so
// `nix audit`, `nix info` and the post-build summary all agree on where it lives.
func EnvReportDir(name string) string {
	return filepath.Join(EnvDir(name), "security-report")
}

// auditReport mirrors the subset of scripts/security-audit.sh's report.json that
// the posture line needs.
type auditReport struct {
	Generated     string   `json:"generated"`
	WorstSeverity string   `json:"worst_severity"`
	OK            bool     `json:"ok"`
	Issues        []string `json:"issues"`
}

// SecurityPosture returns a one-line, emoji-tagged summary of the last audit of
// the named environment, or a nudge to run one if none exists.
func SecurityPosture(name string) string {
	path := filepath.Join(EnvReportDir(name), "report.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Sprintf("not yet audited - run: rfswift nix audit %s", name)
	}
	var r auditReport
	if err := json.Unmarshal(data, &r); err != nil {
		return fmt.Sprintf("audit report unreadable - re-run: rfswift nix audit %s", name)
	}
	when := ""
	if t, e := time.Parse(time.RFC3339, r.Generated); e == nil {
		when = t.Local().Format("2006-01-02 15:04")
	}
	if r.OK {
		return fmt.Sprintf("✅ no known issues (audited %s)", when)
	}
	icon := "⚠️"
	switch r.WorstSeverity {
	case "critical", "high":
		icon = "❌"
	}
	return fmt.Sprintf("%s %d issue(s), worst: %s (audited %s)", icon, len(r.Issues), r.WorstSeverity, when)
}

// PostureIsStale reports whether no audit report exists for the environment yet.
func PostureIsStale(name string) bool {
	_, err := os.Stat(filepath.Join(EnvReportDir(name), "report.json"))
	return err != nil
}
