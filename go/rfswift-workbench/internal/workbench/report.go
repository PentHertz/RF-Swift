package workbench

import (
	"encoding/json"
	"fmt"
	"html"
	"math"
	"regexp"
	"sort"
	"strings"
)

// Branding for generated reports (Penthertz). The GUI shows "powered by
// Penthertz"; the report carries the full company/author/services block so it
// doubles as a client-facing deliverable.
var Brand = struct {
	Company, Site, Email, Author, Tagline, Blurb string
	Services                                     []string
}{
	Company: "Penthertz",
	Site:    "penthertz.com",
	Email:   "contact@penthertz.com",
	Author:  "Sebastien Dudek (@FlUxIuS)",
	Tagline: "Wireless and hardware security",
	Blurb:   "Penthertz helps organizations find and fix vulnerabilities across their wireless and hardware attack surface, from RF and radio protocols to embedded devices.",
	Services: []string{
		"RF and SDR security assessments",
		"Telecom security (2G to 5G)",
		"Wi-Fi, Bluetooth and BLE",
		"RFID / NFC and access control",
		"Automotive and CAN bus",
		"Hardware, embedded and firmware reverse engineering",
		"Trainings and workshops",
	},
}

var cvssBySev = map[string]string{
	"crit": "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H",
	"high": "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:N/A:N",
	"med":  "CVSS:3.1/AV:N/AC:L/PR:L/UI:N/S:U/C:L/I:L/A:N",
	"low":  "CVSS:3.1/AV:L/AC:H/PR:H/UI:R/S:U/C:L/I:N/A:N",
}
var sevName = map[string]string{"crit": "Critical", "high": "High", "med": "Medium", "low": "Low"}
var pwSev = map[string]string{"crit": "Critical", "high": "High", "med": "Medium", "low": "Low"}
var sevOrder = map[string]int{"crit": 0, "high": 1, "med": 2, "low": 3}

// BuildReportMarkdown builds a per-mission, Penthertz-branded markdown report.
func BuildReportMarkdown(m Mission, findings []Finding, note string, captures []Capture) string {
	counts := map[string]int{"crit": 0, "high": 0, "med": 0, "low": 0}
	findings = append([]Finding(nil), findings...)
	for i := range findings {
		findings[i].Sev = reportSeverity(findings[i])
	}
	for _, f := range findings {
		counts[f.Sev]++
	}
	var b strings.Builder
	title := m.Title
	if title == "" {
		title = m.ID
	}
	b.WriteString("# " + title + "\n\n")
	b.WriteString("**" + Brand.Company + "** - " + Brand.Tagline + "\n" + Brand.Site + " | Author: " + Brand.Author + " | Mission: " + m.ID + " (" + engineName(m.Engine) + ")\n\n")
	b.WriteString("> Prepared by " + Brand.Company + " (" + Brand.Site + "). Generated with the RF Swift Workbench.\n\n")
	b.WriteString("## Summary\n\n| Severity | Count |\n|---|---|\n")
	for _, k := range []string{"crit", "high", "med", "low"} {
		b.WriteString("| " + sevName[k] + " | " + itoa(counts[k]) + " |\n")
	}
	b.WriteString("\n## Findings\n\n")
	sorted := append([]Finding(nil), findings...)
	sort.SliceStable(sorted, func(i, j int) bool { return sevOrder[sorted[i].Sev] < sevOrder[sorted[j].Sev] })
	if len(sorted) == 0 {
		b.WriteString("_No findings recorded for this mission._\n\n")
	}
	for _, f := range sorted {
		b.WriteString("### [" + sevName[f.Sev] + "] " + f.Title + "\n\n")
		b.WriteString("- Target: `" + f.Target + "`")
		if f.VulnType != "" {
			b.WriteString("\n- Type: " + f.VulnType)
		}
		b.WriteString("\n- Status: " + f.Status + "\n- CVSS v3: `" + orDefault(f.CVSS, cvssBySev[f.Sev]) + "`\n\n")
		if f.Description != "" {
			b.WriteString(f.Description + "\n\n")
		}
		if f.Observation != "" {
			b.WriteString("**Observation:** " + f.Observation + "\n\n")
		}
		if f.Remediation != "" {
			b.WriteString("**Remediation:** " + f.Remediation + "\n\n")
		}
		if f.PoC != "" {
			b.WriteString("**Proof of concept:**\n\n" + f.PoC + "\n\n")
		}
		if refs := splitLines(f.References); len(refs) > 0 {
			b.WriteString("**References:** " + strings.Join(refs, ", ") + "\n\n")
		}
	}
	b.WriteString("## Notes\n\n" + orDefault(note, "_No notes recorded._") + "\n\n")
	b.WriteString("## Captures\n\n")
	if len(captures) == 0 {
		b.WriteString("_No captures recorded._\n")
	}
	for _, c := range captures {
		b.WriteString("- **" + c.Name + "** (" + c.Type + ", " + c.Tool + "): " + c.Note + "\n")
	}
	b.WriteString("\n## About " + Brand.Company + "\n\n" + Brand.Blurb + "\n\n### Services\n\n")
	for _, s := range Brand.Services {
		b.WriteString("- " + s + "\n")
	}
	b.WriteString("\nContact: " + Brand.Site + " - " + Brand.Email + "\n")
	return b.String()
}

func reportSeverity(f Finding) string {
	switch strings.ToLower(strings.TrimSpace(f.Sev)) {
	case "crit", "critical":
		return "crit"
	case "high":
		return "high"
	case "med", "medium", "moderate":
		return "med"
	case "low", "info", "informational":
		return "low"
	}
	score := cvss3BaseScore(f.CVSS)
	switch {
	case score >= 9:
		return "crit"
	case score >= 7:
		return "high"
	case score >= 4:
		return "med"
	default:
		return "low"
	}
}

func cvss3BaseScore(vector string) float64 {
	m := map[string]string{}
	for _, part := range strings.Split(vector, "/") {
		if pair := strings.SplitN(part, ":", 2); len(pair) == 2 {
			m[pair[0]] = pair[1]
		}
	}
	av := map[string]float64{"N": .85, "A": .62, "L": .55, "P": .2}[m["AV"]]
	ac := map[string]float64{"L": .77, "H": .44}[m["AC"]]
	ui := map[string]float64{"N": .85, "R": .62}[m["UI"]]
	prMap := map[string]float64{"N": .85, "L": .62, "H": .27}
	changed := m["S"] == "C"
	if changed {
		prMap = map[string]float64{"N": .85, "L": .68, "H": .5}
	}
	impactWeight := map[string]float64{"N": 0, "L": .22, "H": .56}
	isc := 1 - (1-impactWeight[m["C"]])*(1-impactWeight[m["I"]])*(1-impactWeight[m["A"]])
	impact := 6.42 * isc
	if changed {
		impact = 7.52*(isc-.029) - 3.25*math.Pow(isc-.02, 15)
	}
	if impact <= 0 || av == 0 || ac == 0 || ui == 0 || prMap[m["PR"]] == 0 {
		return 0
	}
	base := impact + 8.22*av*ac*prMap[m["PR"]]*ui
	if changed {
		base *= 1.08
	}
	return math.Ceil((math.Min(base, 10)-1e-10)*10) / 10
}

// --- pwndoc interoperability ---

type pwndocFinding struct {
	Title                 string           `json:"title"`
	VulnType              string           `json:"vulnType"`
	Description           string           `json:"description"`
	Observation           string           `json:"observation"`
	Remediation           string           `json:"remediation"`
	References            []string         `json:"references"`
	CVSSv3                string           `json:"cvssv3"`
	CVSSv4                string           `json:"cvssv4,omitempty"`
	CVSSSeverity          string           `json:"cvssSeverity"`
	Priority              int              `json:"priority"`
	RemediationComplexity int              `json:"remediationComplexity"`
	PoC                   string           `json:"poc"`
	Scope                 string           `json:"scope"`
	Affected              string           `json:"affected"`
	Status                int              `json:"status"`
	Category              string           `json:"category"`
	CustomFields          []any            `json:"customFields"`
	Paragraphs            []map[string]any `json:"paragraphs,omitempty"`
	RetestStatus          string           `json:"retestStatus,omitempty"`
	RetestDescription     string           `json:"retestDescription,omitempty"`
}

type pwndocDoc struct {
	Name        string          `json:"name"`
	Mission     string          `json:"mission"`
	Company     string          `json:"company"`
	Author      string          `json:"author"`
	GeneratedBy string          `json:"generatedBy"`
	Findings    []pwndocFinding `json:"findings"`
	Audits      []struct {
		Findings []pwndocFinding `json:"findings"`
	} `json:"audits"`
}

var priorityBySev = map[string]int{"crit": 4, "high": 3, "med": 2, "low": 1}

var (
	mdImage = regexp.MustCompile(`!\[([^]]*)\]\(([^)]+)\)`)
	mdLink  = regexp.MustCompile(`\[([^]]+)\]\(([^)]+)\)`)
	mdBold  = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	mdCode  = regexp.MustCompile("`([^`]+)`")
)

// PwnDoc finding prose is HTML. Workbench deliberately stores Markdown so its
// source mode remains portable; conversion happens only at the boundary.
func markdownToPwndocHTML(src string) string {
	if strings.TrimSpace(src) == "" {
		return ""
	}
	if regexp.MustCompile(`(?i)<(p|ul|ol|pre|h[1-6]|img|blockquote)\b`).MatchString(src) {
		return src
	}
	inline := func(s string) string {
		s = html.EscapeString(s)
		s = mdImage.ReplaceAllString(s, `<img alt="$1" src="$2">`)
		s = mdLink.ReplaceAllString(s, `<a href="$2">$1</a>`)
		s = mdBold.ReplaceAllString(s, `<strong>$1</strong>`)
		return mdCode.ReplaceAllString(s, `<code>$1</code>`)
	}
	var out strings.Builder
	lines := strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n")
	for i := 0; i < len(lines); {
		line := lines[i]
		if strings.HasPrefix(line, "```") {
			i++
			var code []string
			for i < len(lines) && !strings.HasPrefix(lines[i], "```") {
				code = append(code, lines[i])
				i++
			}
			if i < len(lines) {
				i++
			}
			out.WriteString("<pre><code>" + html.EscapeString(strings.Join(code, "\n")) + "</code></pre>")
			continue
		}
		if strings.TrimSpace(line) == "" {
			i++
			continue
		}
		if strings.HasPrefix(line, "#") {
			n := 0
			for n < len(line) && n < 6 && line[n] == '#' {
				n++
			}
			if n < len(line) && line[n] == ' ' {
				out.WriteString(fmt.Sprintf("<h%d>%s</h%d>", n, inline(line[n+1:]), n))
				i++
				continue
			}
		}
		if strings.HasPrefix(strings.TrimSpace(line), "- ") {
			out.WriteString("<ul>")
			for i < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i]), "- ") {
				out.WriteString("<li>" + inline(strings.TrimSpace(lines[i])[2:]) + "</li>")
				i++
			}
			out.WriteString("</ul>")
			continue
		}
		var para []string
		for i < len(lines) && strings.TrimSpace(lines[i]) != "" {
			para = append(para, lines[i])
			i++
		}
		out.WriteString("<p>" + inline(strings.Join(para, "\n")) + "</p>")
	}
	return out.String()
}

func pwndocHTMLToMarkdown(src string) string {
	if !strings.Contains(src, "<") {
		return src
	}
	r := strings.NewReplacer("<strong>", "**", "</strong>", "**", "<b>", "**", "</b>", "**", "<code>", "`", "</code>", "`", "<p>", "", "</p>", "\n\n", "<br>", "\n", "<br/>", "\n", "<ul>", "", "</ul>", "\n", "<li>", "- ", "</li>", "\n")
	out := r.Replace(src)
	out = regexp.MustCompile(`<a href="([^"]+)">([^<]+)</a>`).ReplaceAllString(out, `[$2]($1)`)
	out = regexp.MustCompile(`<img[^>]*alt="([^"]*)"[^>]*src="([^"]+)"[^>]*>`).ReplaceAllString(out, `![$1]($2)`)
	out = regexp.MustCompile(`<[^>]+>`).ReplaceAllString(out, "")
	return strings.TrimSpace(html.UnescapeString(out))
}

// BuildPwndocJSON maps a mission's findings to a pwndoc-compatible document.
func BuildPwndocJSON(m Mission, findings []Finding) ([]byte, error) {
	doc := pwndocDoc{
		Name:        orDefault(m.Title, m.ID),
		Mission:     m.ID,
		Company:     Brand.Company,
		Author:      Brand.Author,
		GeneratedBy: "RF Swift Workbench",
	}
	for _, f := range findings {
		refs := splitLines(f.References)
		if len(refs) == 0 && f.Src != "" {
			refs = []string{f.Src}
		}
		doc.Findings = append(doc.Findings, pwndocFinding{
			Title:                 f.Title,
			VulnType:              orDefault(f.VulnType, "RF / hardware"),
			Description:           markdownToPwndocHTML(orDefault(f.Description, f.Title)),
			Observation:           markdownToPwndocHTML(f.Observation),
			Remediation:           markdownToPwndocHTML(f.Remediation),
			References:            refs,
			CVSSv3:                orDefault(f.CVSS, cvssBySev[f.Sev]),
			CVSSSeverity:          pwSev[f.Sev],
			CVSSv4:                f.CVSSv4,
			Priority:              intDefault(f.Priority, priorityBySev[f.Sev]),
			RemediationComplexity: intDefault(f.RemediationComplexity, 2),
			PoC:                   markdownToPwndocHTML(f.PoC),
			Scope:                 f.Target,
			Status:                0,
			Category:              orDefault(f.Category, "RF Swift"),
			CustomFields:          mapsToAny(f.CustomFields),
			Paragraphs:            f.Paragraphs,
			RetestStatus:          f.RetestStatus,
			RetestDescription:     f.RetestDescription,
		})
	}
	return json.MarshalIndent(doc, "", "  ")
}

// ParsePwndoc reads a pwndoc export (a document, a bare array, or a backup with
// audits) into RF Swift findings.
func ParsePwndoc(data []byte, missionID string) ([]Finding, error) {
	trimmed := strings.TrimSpace(string(data))
	var pfs []pwndocFinding
	if strings.HasPrefix(trimmed, "[") {
		if err := json.Unmarshal(data, &pfs); err != nil {
			return nil, err
		}
	} else {
		var doc pwndocDoc
		if err := json.Unmarshal(data, &doc); err != nil {
			return nil, err
		}
		pfs = doc.Findings
		for _, a := range doc.Audits {
			pfs = append(pfs, a.Findings...)
		}
	}
	var out []Finding
	for _, p := range pfs {
		if p.Title == "" {
			continue
		}
		out = append(out, Finding{
			Sev:                   sevFromPwndoc(p),
			Title:                 p.Title,
			Target:                firstNonEmpty(p.Scope, p.Affected, missionID),
			VulnType:              p.VulnType,
			CVSS:                  p.CVSSv3,
			CVSSv4:                p.CVSSv4,
			Status:                "open",
			Description:           pwndocHTMLToMarkdown(p.Description),
			Observation:           pwndocHTMLToMarkdown(p.Observation),
			Remediation:           pwndocHTMLToMarkdown(p.Remediation),
			References:            strings.Join(p.References, "\n"),
			PoC:                   pwndocHTMLToMarkdown(p.PoC),
			Category:              p.Category,
			Priority:              p.Priority,
			RemediationComplexity: p.RemediationComplexity,
			RetestStatus:          p.RetestStatus,
			RetestDescription:     p.RetestDescription,
			Paragraphs:            p.Paragraphs,
			CustomFields:          anyToMaps(p.CustomFields),
			Src:                   "pwndoc import",
		})
	}
	return out, nil
}

func intDefault(v, fallback int) int {
	if v != 0 {
		return v
	}
	return fallback
}
func mapsToAny(in []map[string]any) []any {
	out := make([]any, len(in))
	for i := range in {
		out[i] = in[i]
	}
	return out
}
func anyToMaps(in []any) []map[string]any {
	out := make([]map[string]any, 0, len(in))
	for _, v := range in {
		if m, ok := v.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func sevFromPwndoc(p pwndocFinding) string {
	s := strings.ToLower(p.CVSSSeverity)
	switch {
	case strings.HasPrefix(s, "crit"):
		return "crit"
	case strings.HasPrefix(s, "high"):
		return "high"
	case strings.HasPrefix(s, "med"):
		return "med"
	case strings.HasPrefix(s, "low"):
		return "low"
	}
	return "med"
}

func splitLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if t := strings.TrimSpace(line); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return "-"
}

func engineName(e string) string {
	switch e {
	case "docker":
		return "Docker"
	case "podman":
		return "Podman"
	case "lima":
		return "Lima VM"
	case "nix":
		return "Nix"
	}
	return e
}
