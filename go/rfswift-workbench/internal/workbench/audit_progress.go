/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
*
*  Live stages of a Nix audit for the Workbench's progress bar.
*
*  The audit script (RF-Swift-nix scripts/security-audit.sh) prints nothing
*  on stdout when it has no terminal, but it appends every status line to
*  <out>/summary.txt as it goes. Following that file gives the GUI the real
*  stage - realising the closure, vulnix, syft, grype, osv, integrity,
*  provenance - instead of a timer that stops at 85%. The slow parts are the
*  first two: realising a lazy environment downloads its whole tool set, and
*  the integrity check hashes every path of the closure.
 */

package workbench

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	rfnix "penthertz/rfswift/nix"
)

// auditStage is what a summary line means for the progress bar: the stage
// that starts once this line is written, and how far along that is.
type auditStage struct {
	Marker  string
	Percent int
	Stage   string
}

// auditStages is in script order; a line is matched by its marker.
var auditStages = []auditStage{
	{"=== repository", 12, "Checking flake pins, source hashes and insecure allowances"},
	{"realising env:", 15, "Realising the environment closure (downloads the tools not in the store yet)"},
	{"=== env:", 30, "Closure ready, scanning it for CVEs (vulnix)"},
	{"vulnix:", 45, "Generating the SBOM (syft)"},
	{"syft:", 55, "Scanning the SBOM (grype)"},
	{"grype:", 62, "Checking OSV / GHSA advisories"},
	{"osv-scanner:", 70, "Verifying closure integrity (hashes every store path; slow on large closures)"},
	{"integrity:", 85, "Checking provenance signatures"},
	{"provenance:", 92, "Collecting evaluation warnings"},
	{"config:", 96, "Writing the report"},
}

// auditStageForLine maps one summary line to a stage, ok=false for lines
// that carry no stage (headers, findings, blank).
func auditStageForLine(line string) (auditStage, bool) {
	line = strings.TrimSpace(line)
	if line == "" {
		return auditStage{}, false
	}
	// Status lines start with an emoji, then the label: match on contains.
	for _, st := range auditStages {
		if strings.HasPrefix(line, st.Marker) || strings.Contains(line, " "+st.Marker) {
			return st, true
		}
	}
	return auditStage{}, false
}

// followAuditSummary reports the audit's stages for mission until the
// returned stop function is called. It announces the realise step at once
// (the script is silent while nix downloads), then follows summary.txt.
func (e *LocalEngine) followAuditSummary(mission string, env *rfnix.Environment, summary string) func() {
	if e.AuditProgress == nil {
		return func() {}
	}
	if env != nil && env.Lazy {
		e.AuditProgress(mission, fmt.Sprintf("Realising the full %s closure first: this lazy environment fetches its whole tool set now, which can take several minutes", env.Image), 10)
	} else {
		e.AuditProgress(mission, "Realising the environment closure", 10)
	}
	// Lines already in the file belong to an earlier audit: skip them (the
	// script appends). Measured before the audit starts, so nothing it
	// writes is missed. A file that shrinks was rewritten: start over.
	var offset int64
	if info, err := os.Stat(summary); err == nil {
		offset = info.Size()
	}
	done := make(chan struct{})
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		last := -1
		ticker := time.NewTicker(700 * time.Millisecond)
		defer ticker.Stop()
		read := func() {
			f, err := os.Open(summary)
			if err != nil {
				return
			}
			defer f.Close()
			if info, err := f.Stat(); err == nil && info.Size() < offset {
				offset = 0
			}
			if _, err := f.Seek(offset, 0); err != nil {
				return
			}
			sc := bufio.NewScanner(f)
			for sc.Scan() {
				line := sc.Text()
				offset += int64(len(line)) + 1
				if st, ok := auditStageForLine(line); ok && st.Percent > last {
					last = st.Percent
					e.AuditProgress(mission, st.Stage, st.Percent)
				}
			}
		}
		for {
			select {
			case <-done:
				read()
				return
			case <-ticker.C:
				read()
			}
		}
	}()
	return func() {
		close(done)
		<-finished
	}
}
