/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
*
*  Container image security audit.
*
*  The container-engine counterpart of `rfswift nix audit`: it scans a Docker /
*  Podman image (OS packages + language dependencies) for known vulnerabilities
*  so users can check an image's security posture on their own machine, on their
*  own engine, without leaving RF Swift and WITHOUT needing Nix.
*
*  Scanners are sourced engine-natively: a host-installed `trivy` is used if
*  present; otherwise trivy is run AS A CONTAINER via the active engine
*  (docker/podman), reading the target image from the daemon. `grype` is used as
*  an optional second opinion when installed on the host.
*
*  Reports can be emitted as stdout, JSON, HTML and PDF (--format), so a finding
*  can be surfaced, kept as an artifact, or shared.
 */

package dock

import (
	"encoding/json"
	"fmt"
	"html"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	common "penthertz/rfswift/common"
)

// ImageAuditOptions controls a container image audit.
type ImageAuditOptions struct {
	FailOn  string // none|low|medium|high|critical - exit non-zero at/above this
	OutDir  string // report directory (default ./security-report)
	Formats string // comma list: stdout,json,html,pdf (default stdout)
}

// imgResult is the per-image outcome, used for the aggregate report renderings.
type imgResult struct {
	Ref      string `json:"image"`
	Critical int    `json:"critical"`
	High     int    `json:"high"`
	Medium   int    `json:"medium"`
	Low      int    `json:"low"`
	Total    int    `json:"total"`
	Worst    string `json:"worst_severity"`
	Report   string `json:"report"`
	Error    string `json:"error,omitempty"`
}

func engineCLI(t EngineType) string {
	if t == EnginePodman {
		return "podman"
	}
	return "docker" // docker + lima (docker-compatible)
}

// trivyContext resolves how to run trivy: a host binary if present, else the
// active engine's CLI to run trivy as a container. cli is empty when neither a
// host trivy nor a usable engine is available.
func trivyContext() (hostTrivy bool, cli string) {
	if binaryExists("trivy") {
		return true, ""
	}
	eng := GetEngine()
	if eng != nil && eng.IsAvailable() {
		return false, engineCLI(eng.Type())
	}
	return false, ""
}

func sevRank(s string) int {
	switch strings.ToLower(s) {
	case "critical":
		return 4
	case "high":
		return 3
	case "medium", "moderate":
		return 2
	case "low":
		return 1
	default:
		return 0
	}
}

func rankName(r int) string {
	switch r {
	case 4:
		return "critical"
	case 3:
		return "high"
	case 2:
		return "medium"
	case 1:
		return "low"
	default:
		return "none"
	}
}

type trivyReport struct {
	Results []struct {
		Vulnerabilities []struct {
			Severity string `json:"Severity"`
		} `json:"Vulnerabilities"`
	} `json:"Results"`
}

// AuditImage scans one or more container images, prints a posture summary, and
// writes the requested report formats. Returns a non-zero error only when
// --fail-on is set and a finding at or above that severity is present.
func AuditImage(refs []string, opts ImageAuditOptions) error {
	outDir := opts.OutDir
	if outDir == "" {
		outDir = "security-report"
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("cannot create report dir %s: %w", outDir, err)
	}
	formats := opts.Formats
	if formats == "" {
		formats = "stdout"
	}
	want := func(f string) bool { return strings.Contains(","+formats+",", ","+f+",") }

	hostTrivy, cli := trivyContext()
	if !hostTrivy && cli == "" {
		return fmt.Errorf("no host `trivy` found and no container engine available to run it.\n" +
			"  Install trivy (https://aquasecurity.github.io/trivy) or start Docker/Podman.")
	}
	if !hostTrivy {
		common.PrintInfoMessage(fmt.Sprintf("No host trivy; running trivy as a container via %s ('%s save' exports the image, so it works on Linux/macOS/Windows).", cli, cli))
	}
	hostGrype := binaryExists("grype")

	gateRank := sevRank(opts.FailOn)
	worst := 0
	var results []imgResult

	for _, ref := range refs {
		safe := sanitizeRef(ref)
		jsonPath := filepath.Join(outDir, "trivy-"+safe+".json")
		fmt.Println()
		common.PrintInfoMessage(fmt.Sprintf("=== image: %s ===", ref))

		res := imgResult{Ref: ref, Worst: "none", Report: jsonPath}
		if err := runTrivy(ref, jsonPath, hostTrivy, cli, outDir); err != nil {
			common.PrintWarningMessage(fmt.Sprintf("trivy scan did not complete for %s: %v", ref, err))
			res.Error = err.Error()
			results = append(results, res)
			continue
		}
		res = summarizeTrivy(jsonPath, ref)
		results = append(results, res)
		if sevRank(res.Worst) > worst {
			worst = sevRank(res.Worst)
		}
		if hostGrype {
			runHostGrype(ref, filepath.Join(outDir, "grype-"+safe+".json"))
		}
	}

	// Aggregate renderings.
	generated := time.Now().UTC().Format(time.RFC3339)
	ok := worst == 0
	if want("json") {
		writeImageJSON(filepath.Join(outDir, "report.json"), generated, rankName(worst), ok, results)
		common.PrintInfoMessage("Wrote " + filepath.Join(outDir, "report.json"))
	}
	if want("html") || want("pdf") {
		htmlPath := filepath.Join(outDir, "report.html")
		writeImageHTML(htmlPath, generated, rankName(worst), ok, results)
		if want("html") {
			common.PrintInfoMessage("Wrote " + htmlPath)
		}
		if want("pdf") {
			if err := renderPDF(htmlPath, filepath.Join(outDir, "report.pdf")); err != nil {
				common.PrintWarningMessage("PDF not generated: " + err.Error() + " (HTML report is available)")
			} else {
				common.PrintInfoMessage("Wrote " + filepath.Join(outDir, "report.pdf"))
			}
			if !want("html") {
				_ = os.Remove(htmlPath)
			}
		}
	}

	fmt.Println()
	common.PrintInfoMessage(fmt.Sprintf("Image audit complete. Worst severity: %s. Reports in %s.", rankName(worst), outDir))

	if opts.FailOn != "none" && opts.FailOn != "" && gateRank > 0 && worst >= gateRank {
		return fmt.Errorf("image audit found findings at or above '%s' (worst: %s)", opts.FailOn, rankName(worst))
	}
	return nil
}

func runTrivy(ref, jsonPath string, hostTrivy bool, cli, outDir string) error {
	if hostTrivy {
		cmd := exec.Command("trivy", "image", "--quiet", "--scanners", "vuln",
			"--format", "json", "--output", jsonPath, ref)
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}
	// Containerised path, cross-platform: export the image to a tar with the
	// engine (works on Linux/macOS/Windows Docker Desktop and Podman), then scan
	// the tar inside a trivy container. This avoids mounting the daemon socket,
	// which is unreliable in the macOS/Windows VM backends.
	absOut, err := filepath.Abs(outDir)
	if err != nil {
		return err
	}
	// The trivy container runs inside the engine's VM and bind-mounts the work
	// dir. Docker Desktop/OrbStack share the whole host filesystem, so any path
	// works — but Lima only shares the user's home and /tmp/lima. A macOS temp
	// path like /var/folders/... is invisible inside the Lima VM, so the mounted
	// tar would be empty and the scan would silently find nothing. Under Lima,
	// stage the tar + report in a home-rooted dir Lima maps into the VM at the
	// same path, then copy the report back to where the caller expects it.
	workDir := absOut
	relocated := false
	if GetEngine().Type() == EngineLima {
		if home, herr := os.UserHomeDir(); herr == nil {
			shared := filepath.Join(home, ".cache", "rfswift", "trivy")
			if os.MkdirAll(shared, 0o755) == nil {
				workDir = shared
				relocated = true
			}
		}
	}
	tarBase := sanitizeRef(ref) + ".tar"
	tarPath := filepath.Join(workDir, tarBase)
	save := exec.Command(cli, "save", ref, "-o", tarPath)
	save.Stderr = os.Stderr
	if err := save.Run(); err != nil {
		// Not present locally: pull once, then retry the export.
		pull := exec.Command(cli, "pull", ref)
		pull.Stderr = os.Stderr
		if pull.Run() != nil {
			return fmt.Errorf("%s could not find or pull image %s", cli, ref)
		}
		save = exec.Command(cli, "save", ref, "-o", tarPath)
		save.Stderr = os.Stderr
		if err := save.Run(); err != nil {
			return fmt.Errorf("%s save %s failed: %w", cli, ref, err)
		}
	}
	defer os.Remove(tarPath)
	base := filepath.Base(jsonPath)
	cmd := exec.Command(cli, "run", "--rm",
		"-v", workDir+":/report",
		"aquasec/trivy:latest", "image", "--input", "/report/"+tarBase,
		"--quiet", "--scanners", "vuln",
		"--format", "json", "--output", "/report/"+base)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	// When staged in a Lima-shared dir, move the report to the caller's path.
	if relocated {
		produced := filepath.Join(workDir, base)
		if produced != jsonPath {
			defer os.Remove(produced)
			data, rerr := os.ReadFile(produced)
			if rerr != nil {
				return fmt.Errorf("trivy produced no report at %s: %w", produced, rerr)
			}
			if werr := os.WriteFile(jsonPath, data, 0o644); werr != nil {
				return werr
			}
		}
	}
	return nil
}

func summarizeTrivy(jsonPath, ref string) imgResult {
	res := imgResult{Ref: ref, Worst: "none", Report: jsonPath}
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		common.PrintWarningMessage("trivy produced no report (image not found, or scan failed).")
		res.Error = "no report"
		return res
	}
	var r trivyReport
	if err := json.Unmarshal(data, &r); err != nil {
		common.PrintWarningMessage("trivy report unreadable.")
		res.Error = "unreadable"
		return res
	}
	worst := 0
	for _, rr := range r.Results {
		for _, v := range rr.Vulnerabilities {
			switch strings.ToLower(v.Severity) {
			case "critical":
				res.Critical++
			case "high":
				res.High++
			case "medium":
				res.Medium++
			case "low":
				res.Low++
			}
			res.Total++
			if sevRank(v.Severity) > worst {
				worst = sevRank(v.Severity)
			}
		}
	}
	res.Worst = rankName(worst)
	if res.Total == 0 {
		fmt.Printf("  ✅ trivy: no known vulnerabilities\n")
		return res
	}
	icon := "⚠️"
	if worst >= 3 {
		icon = "❌"
	}
	fmt.Printf("  %s trivy: %d vuln(s) [critical=%d high=%d medium=%d low=%d] -> %s\n",
		icon, res.Total, res.Critical, res.High, res.Medium, res.Low, jsonPath)
	return res
}

func runHostGrype(ref, jsonPath string) {
	cmd := exec.Command("grype", ref, "-o", "json="+jsonPath, "-q")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err == nil {
		fmt.Printf("  ℹ️  grype: second-opinion report -> %s\n", jsonPath)
	}
}

func writeImageJSON(path, generated, worst string, ok bool, results []imgResult) {
	obj := map[string]interface{}{
		"tool":           "rfswift images audit",
		"generated":      generated,
		"worst_severity": worst,
		"ok":             ok,
		"images":         results,
	}
	b, _ := json.MarshalIndent(obj, "", "  ")
	_ = os.WriteFile(path, b, 0o644)
}

func writeImageHTML(path, generated, worst string, ok bool, results []imgResult) {
	var b strings.Builder
	b.WriteString(`<!doctype html><html><head><meta charset="utf-8"><title>RF Swift image audit</title>
<style>body{font-family:system-ui,Arial,sans-serif;margin:2rem;color:#1b1f24}
h1{font-size:1.5rem}.meta{color:#57606a;font-size:.9rem}
.banner{padding:.7rem 1rem;border-radius:8px;font-weight:600;margin:1rem 0}
.green{background:#d3f9d8;color:#0b6b2e}.red{background:#ffe3e3;color:#a4133c}.yellow{background:#fff3bf;color:#8a6d00}
table{border-collapse:collapse;width:100%;font-size:.92rem}th,td{border:1px solid #d0d7de;padding:.35rem .55rem;text-align:left}
th{background:#f6f8fa}td.n{text-align:right}.crit{color:#a4133c;font-weight:600}.hi{color:#c9184a}.ok{color:#0b6b2e}code{font-size:.85em}</style></head><body>`)
	b.WriteString("<h1>RF Swift container image audit</h1>")
	b.WriteString(fmt.Sprintf(`<p class="meta">generated: %s &nbsp;|&nbsp; scanner: trivy</p>`, html.EscapeString(generated)))
	if ok {
		b.WriteString(`<div class="banner green">&#9989; No known vulnerabilities across scanned images</div>`)
	} else if sevRank(worst) >= 3 {
		b.WriteString(fmt.Sprintf(`<div class="banner red">&#10060; Vulnerabilities found &mdash; worst severity: %s</div>`, worst))
	} else {
		b.WriteString(fmt.Sprintf(`<div class="banner yellow">&#9888;&#65039; Vulnerabilities found &mdash; worst severity: %s</div>`, worst))
	}
	b.WriteString(`<table><tr><th>Image</th><th>Critical</th><th>High</th><th>Medium</th><th>Low</th><th>Total</th><th>Worst</th></tr>`)
	for _, r := range results {
		if r.Error != "" {
			b.WriteString(fmt.Sprintf(`<tr><td><code>%s</code></td><td colspan="6" class="hi">scan error: %s</td></tr>`,
				html.EscapeString(r.Ref), html.EscapeString(r.Error)))
			continue
		}
		cls := "ok"
		if sevRank(r.Worst) >= 3 {
			cls = "crit"
		} else if r.Total > 0 {
			cls = "hi"
		}
		b.WriteString(fmt.Sprintf(`<tr><td><code>%s</code></td><td class="n">%d</td><td class="n">%d</td><td class="n">%d</td><td class="n">%d</td><td class="n">%d</td><td class="%s">%s</td></tr>`,
			html.EscapeString(r.Ref), r.Critical, r.High, r.Medium, r.Low, r.Total, cls, r.Worst))
	}
	b.WriteString("</table></body></html>")
	_ = os.WriteFile(path, []byte(b.String()), 0o644)
}

// renderPDF converts the HTML report to PDF using a host weasyprint or
// wkhtmltopdf. PDF stays best-effort so the audit never fails just for lacking a
// renderer (the HTML report is always available).
func renderPDF(htmlPath, pdfPath string) error {
	if binaryExists("weasyprint") {
		return exec.Command("weasyprint", htmlPath, pdfPath).Run()
	}
	if binaryExists("wkhtmltopdf") {
		return exec.Command("wkhtmltopdf", htmlPath, pdfPath).Run()
	}
	return fmt.Errorf("no PDF renderer found (install weasyprint or wkhtmltopdf)")
}

func sanitizeRef(ref string) string {
	return strings.NewReplacer("/", "_", ":", "_", "@", "_", " ", "_").Replace(ref)
}
