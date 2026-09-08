package integration_test

import (
	"strings"
	"testing"

	"penthertz/rfswift-workbench/internal/workbench"
)

func TestPwndocFindingFieldsRoundTrip(t *testing.T) {
	want := workbench.Finding{Title: "Replay attack", Target: "rf-lab", VulnType: "CWE-294", CVSS: "CVSS:3.1/AV:A/AC:L/PR:N/UI:N/S:U/C:L/I:H/A:N", CVSSv4: "CVSS:4.0/AV:A", Sev: "high", Description: "description", Observation: "observation", Remediation: "remediation", References: "https://example.test/advisory", PoC: "proof", Category: "Wireless", Priority: 4, RemediationComplexity: 3, RetestStatus: "partial", RetestDescription: "remaining issue", Paragraphs: []map[string]any{{"text": "extra context"}}, CustomFields: []map[string]any{{"customField": "field-id", "text": "value"}}}
	b, err := workbench.BuildPwndocJSON(workbench.Mission{ID: "rf-lab"}, []workbench.Finding{want})
	if err != nil {
		t.Fatal(err)
	}
	got, err := workbench.ParsePwndoc(b, "rf-lab")
	if err != nil || len(got) != 1 {
		t.Fatalf("ParsePwndoc() = %#v, %v", got, err)
	}
	g := got[0]
	if g.Category != want.Category || g.Priority != want.Priority || g.RemediationComplexity != want.RemediationComplexity || g.CVSSv4 != want.CVSSv4 || g.RetestStatus != want.RetestStatus || g.RetestDescription != want.RetestDescription || len(g.Paragraphs) != 1 || len(g.CustomFields) != 1 {
		t.Fatalf("pwndoc-only fields were not preserved: %#v", g)
	}
}

func TestPwndocExportConvertsMarkdownProseToHTML(t *testing.T) {
	b, err := workbench.BuildPwndocJSON(workbench.Mission{ID: "lab"}, []workbench.Finding{{Title: "RF issue", Description: "## Impact\n\n**Replay** is possible."}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `\u003ch2\u003eImpact\u003c/h2\u003e`) || !strings.Contains(string(b), `\u003cstrong\u003eReplay\u003c/strong\u003e`) {
		t.Fatalf("export did not contain PwnDoc HTML: %s", b)
	}
}

func TestReportNormalizesAndDerivesFindingSeverity(t *testing.T) {
	md := workbench.BuildReportMarkdown(workbench.Mission{ID: "iot-audit"}, []workbench.Finding{{Title: "Legacy critical", Sev: "critical", CVSS: "CVSS:3.1/AV:P/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:N"}, {Title: "Derived high", CVSS: "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:N/A:N"}}, "", nil)
	if strings.Contains(md, "### []") || !strings.Contains(md, "### [Critical] Legacy critical") || !strings.Contains(md, "### [High] Derived high") {
		t.Fatalf("report did not normalize CVSS severity:\n%s", md)
	}
}
