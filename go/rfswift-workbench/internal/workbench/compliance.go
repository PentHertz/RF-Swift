/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
 */

package workbench

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// Compliance mapping for RF and IoT findings.
//
// A finding is linked to one or more weakness classes (replay, default
// credentials, exposed debug interface, ...), either explicitly (its weakness
// field) or inferred from its type, category and title. Each class maps to the
// requirements it breaks in the standards RF/IoT clients are assessed against:
// EN 18031 (the EU Radio Equipment Directive cybersecurity standards), ETSI EN
// 303 645, the OWASP IoT Top 10 and IEC 62443-4-2. The tables are in
// compliance/rf-iot-compliance.json; they are a starting point for the report,
// not a conformity assessment.

//go:embed compliance/rf-iot-compliance.json
var complianceJSON []byte

type ComplianceFramework struct {
	ID           string            `json:"id"`
	Short        string            `json:"short"`
	Name         string            `json:"name"`
	Note         string            `json:"note"`
	Requirements map[string]string `json:"requirements"`
}

type WeaknessClass struct {
	ID       string              `json:"id"`
	Label    string              `json:"label"`
	Keywords []string            `json:"keywords"`
	Map      map[string][]string `json:"map"`
	patterns []*regexp.Regexp
}

type ComplianceCatalog struct {
	Note       string                `json:"note"`
	Frameworks []ComplianceFramework `json:"frameworks"`
	Classes    []WeaknessClass       `json:"classes"`
}

// ComplianceRef is one requirement a finding relates to.
type ComplianceRef struct {
	Framework string `json:"framework"` // framework id
	Standard  string `json:"standard"`  // short display name
	ID        string `json:"id"`
	Title     string `json:"title"`
}

// FindingCompliance is the mapping of one finding.
type FindingCompliance struct {
	Classes  []string        `json:"classes"`
	Labels   []string        `json:"labels"`
	Inferred bool            `json:"inferred"`
	Refs     []ComplianceRef `json:"refs"`
	Unknown  []string        `json:"unknown,omitempty"` // explicit weakness ids not in the catalog
}

var (
	complianceOnce sync.Once
	complianceCat  *ComplianceCatalog
	complianceErr  error
)

// keywordPattern matches a keyword as a whole word or phrase; a trailing "*"
// makes it a prefix ("clon*" matches clone, cloned, cloning).
func keywordPattern(kw string) (*regexp.Regexp, error) {
	kw = strings.ToLower(strings.TrimSpace(kw))
	prefix := strings.HasSuffix(kw, "*")
	kw = strings.TrimSuffix(kw, "*")
	if kw == "" {
		return nil, fmt.Errorf("empty keyword")
	}
	expr := `(^|[^a-z0-9])` + regexp.QuoteMeta(kw)
	if !prefix {
		expr += `($|[^a-z0-9])`
	}
	return regexp.Compile(expr)
}

func loadCompliance() (*ComplianceCatalog, error) {
	complianceOnce.Do(func() {
		var cat ComplianceCatalog
		if err := json.Unmarshal(complianceJSON, &cat); err != nil {
			complianceErr = fmt.Errorf("compliance catalog: %w", err)
			return
		}
		frameworks := map[string]*ComplianceFramework{}
		for i := range cat.Frameworks {
			frameworks[cat.Frameworks[i].ID] = &cat.Frameworks[i]
		}
		seen := map[string]bool{}
		for i := range cat.Classes {
			c := &cat.Classes[i]
			if c.ID == "" || seen[c.ID] {
				complianceErr = fmt.Errorf("compliance class %d: missing or duplicate id", i)
				return
			}
			seen[c.ID] = true
			for fw, ids := range c.Map {
				f := frameworks[fw]
				if f == nil {
					complianceErr = fmt.Errorf("compliance class %s: unknown framework %s", c.ID, fw)
					return
				}
				for _, id := range ids {
					if _, ok := f.Requirements[id]; !ok {
						complianceErr = fmt.Errorf("compliance class %s: %s has no requirement %s", c.ID, fw, id)
						return
					}
				}
			}
			for _, kw := range c.Keywords {
				re, err := keywordPattern(kw)
				if err != nil {
					complianceErr = fmt.Errorf("compliance class %s: %w", c.ID, err)
					return
				}
				c.patterns = append(c.patterns, re)
			}
		}
		complianceCat = &cat
	})
	return complianceCat, complianceErr
}

// splitWeakness reads the weakness field: class ids separated by commas or spaces.
func splitWeakness(s string) []string {
	var out []string
	for _, part := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return r == ',' || r == ';' || r == ' ' }) {
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

// MapFindingCompliance returns the weakness classes of a finding and the
// requirements they relate to. An explicit weakness wins; otherwise classes
// are inferred from the type and category, and from the title when those say
// nothing.
func MapFindingCompliance(f Finding) FindingCompliance {
	cat, err := loadCompliance()
	out := FindingCompliance{Classes: []string{}, Labels: []string{}, Refs: []ComplianceRef{}}
	if err != nil {
		return out
	}
	byID := map[string]*WeaknessClass{}
	for i := range cat.Classes {
		byID[cat.Classes[i].ID] = &cat.Classes[i]
	}
	var classes []*WeaknessClass
	if explicit := splitWeakness(f.Weakness); len(explicit) > 0 {
		for _, id := range explicit {
			if c := byID[id]; c != nil {
				classes = append(classes, c)
			} else {
				out.Unknown = append(out.Unknown, id)
			}
		}
	} else {
		out.Inferred = true
		classes = inferWeaknessClasses(cat, strings.ToLower(f.VulnType+" \n "+f.Category))
		if len(classes) == 0 {
			classes = inferWeaknessClasses(cat, strings.ToLower(f.Title))
		}
	}
	type key struct{ fw, id string }
	seen := map[key]bool{}
	for _, c := range classes {
		out.Classes = append(out.Classes, c.ID)
		out.Labels = append(out.Labels, c.Label)
		for _, fw := range cat.Frameworks {
			for _, id := range c.Map[fw.ID] {
				k := key{fw.ID, id}
				if seen[k] {
					continue
				}
				seen[k] = true
				out.Refs = append(out.Refs, ComplianceRef{Framework: fw.ID, Standard: fw.Short, ID: id, Title: fw.Requirements[id]})
			}
		}
	}
	return out
}

func inferWeaknessClasses(cat *ComplianceCatalog, text string) []*WeaknessClass {
	var out []*WeaknessClass
	if strings.TrimSpace(text) == "" {
		return out
	}
	for i := range cat.Classes {
		for _, re := range cat.Classes[i].patterns {
			if re.MatchString(text) {
				out = append(out, &cat.Classes[i])
				break
			}
		}
	}
	return out
}

// complianceLine renders a finding's requirements for the report, grouped by
// standard: "EN 18031: [AUM] Authentication mechanism; ETSI EN 303 645: 5.1 ...".
func complianceLine(fc FindingCompliance) string {
	if len(fc.Refs) == 0 {
		return ""
	}
	var parts []string
	var current string
	var items []string
	flush := func() {
		if current != "" {
			parts = append(parts, current+": "+strings.Join(items, "; "))
		}
	}
	for _, r := range fc.Refs {
		if r.Standard != current {
			flush()
			current, items = r.Standard, nil
		}
		id := r.ID
		if r.Framework == "en18031" {
			id = "[" + id + "]"
		}
		items = append(items, id+" "+r.Title)
	}
	flush()
	return strings.Join(parts, " | ")
}

// complianceSummary renders the report appendix: for each standard and
// requirement, the findings that relate to it.
func complianceSummary(findings []Finding) string {
	cat, err := loadCompliance()
	if err != nil {
		return ""
	}
	type entry struct {
		ref      ComplianceRef
		findings []string
	}
	byKey := map[string]*entry{}
	order := map[string]int{}
	for i, fw := range cat.Frameworks {
		order[fw.ID] = i
	}
	for _, f := range findings {
		for _, r := range MapFindingCompliance(f).Refs {
			k := r.Framework + "\x00" + r.ID
			e := byKey[k]
			if e == nil {
				e = &entry{ref: r}
				byKey[k] = e
			}
			label := f.Title
			if f.ID != "" {
				label = f.ID + " " + f.Title
			}
			e.findings = append(e.findings, label)
		}
	}
	if len(byKey) == 0 {
		return ""
	}
	entries := make([]*entry, 0, len(byKey))
	for _, e := range byKey {
		entries = append(entries, e)
	}
	sort.Slice(entries, func(a, b int) bool {
		if order[entries[a].ref.Framework] != order[entries[b].ref.Framework] {
			return order[entries[a].ref.Framework] < order[entries[b].ref.Framework]
		}
		return naturalLess(entries[a].ref.ID, entries[b].ref.ID)
	})
	var b strings.Builder
	b.WriteString("## Compliance mapping\n\n")
	b.WriteString("Requirements of the standards below that the findings relate to. " + cat.Note + "\n\n")
	b.WriteString("| Standard | Requirement | Findings |\n|---|---|---|\n")
	for _, e := range entries {
		id := e.ref.ID
		if e.ref.Framework == "en18031" {
			id = "[" + id + "]"
		}
		b.WriteString("| " + e.ref.Standard + " | " + id + " " + e.ref.Title + " | " + strings.ReplaceAll(strings.Join(e.findings, "; "), "|", "/") + " |\n")
	}
	b.WriteString("\n")
	for _, fw := range cat.Frameworks {
		b.WriteString("- **" + fw.Name + "**: " + fw.Note + "\n")
	}
	b.WriteString("\n")
	return b.String()
}

// naturalLess orders requirement ids with their numbers compared as numbers,
// so I7 comes before I10 and 5.5 before 5.10.
func naturalLess(a, b string) bool {
	for a != "" && b != "" {
		da, db := leadingDigits(a), leadingDigits(b)
		if da != "" && db != "" {
			na, _ := strconv.Atoi(da)
			nb, _ := strconv.Atoi(db)
			if na != nb {
				return na < nb
			}
			a, b = a[len(da):], b[len(db):]
			continue
		}
		if a[0] != b[0] {
			return a[0] < b[0]
		}
		a, b = a[1:], b[1:]
	}
	return len(a) < len(b)
}

func leadingDigits(s string) string {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	return s[:i]
}

// --- Workbench bindings and MCP -------------------------------------------

// ComplianceCatalog returns the frameworks and weakness classes (finding editor).
func (a *App) ComplianceCatalog() (*ComplianceCatalog, error) {
	return loadCompliance()
}

// FindingCompliance maps one finding (finding editor preview).
func (a *App) FindingCompliance(f Finding) FindingCompliance {
	return MapFindingCompliance(f)
}

func (s *mcpServer) callMapCompliance(args map[string]any) (any, error) {
	cat, err := loadCompliance()
	if err != nil {
		return nil, err
	}
	f := Finding{VulnType: argString(args, "vuln_type"), Category: argString(args, "category"), Title: argString(args, "title"), Weakness: argString(args, "weakness")}
	if f.VulnType == "" && f.Category == "" && f.Title == "" && f.Weakness == "" {
		classes := make([]map[string]string, 0, len(cat.Classes))
		for _, c := range cat.Classes {
			classes = append(classes, map[string]string{"id": c.ID, "label": c.Label})
		}
		return map[string]any{"classes": classes, "note": cat.Note, "usage": "Give vuln_type, category, title or weakness (class ids) to map a finding; set the finding's weakness field to the class ids you choose."}, nil
	}
	return map[string]any{"mapping": MapFindingCompliance(f), "note": cat.Note}, nil
}
