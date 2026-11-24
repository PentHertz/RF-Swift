package workbench

import (
	"strings"
	"testing"
)

func refIDs(fc FindingCompliance, framework string) []string {
	var out []string
	for _, r := range fc.Refs {
		if r.Framework == framework {
			out = append(out, r.ID)
		}
	}
	return out
}

func TestComplianceCatalogLoadsAndIsConsistent(t *testing.T) {
	cat, err := loadCompliance()
	if err != nil {
		t.Fatal(err)
	}
	if len(cat.Frameworks) != 4 || len(cat.Classes) < 15 {
		t.Fatalf("unexpected catalog size: %d frameworks, %d classes", len(cat.Frameworks), len(cat.Classes))
	}
	for _, c := range cat.Classes {
		if len(c.Map) == 0 || len(c.patterns) == 0 {
			t.Errorf("class %s has no mapping or no keywords", c.ID)
		}
	}
}

func TestMapFindingComplianceInfersAndHonoursExplicitClasses(t *testing.T) {
	replay := MapFindingCompliance(Finding{Title: "Gate remote", VulnType: "Replay / fixed code"})
	if !replay.Inferred || len(replay.Classes) != 1 || replay.Classes[0] != "replay" {
		t.Fatalf("replay not inferred: %#v", replay)
	}
	if got := strings.Join(refIDs(replay, "en18031"), ","); got != "SCM,AUM" {
		t.Fatalf("EN 18031 mechanisms for replay: %q", got)
	}
	creds := MapFindingCompliance(Finding{Title: "Firmware ships a hardcoded maintenance password"})
	if len(creds.Classes) == 0 || creds.Classes[0] != "default-credentials" || strings.Join(refIDs(creds, "etsi303645"), ",") != "5.1" {
		t.Fatalf("title fallback did not map default credentials: %#v", creds)
	}
	explicit := MapFindingCompliance(Finding{VulnType: "Replay", Weakness: "debug-interface, nonsense"})
	if explicit.Inferred || len(explicit.Classes) != 1 || explicit.Classes[0] != "debug-interface" || len(explicit.Unknown) != 1 {
		t.Fatalf("explicit weakness not honoured: %#v", explicit)
	}
	if none := MapFindingCompliance(Finding{Title: "Something unrelated"}); len(none.Refs) != 0 {
		t.Fatalf("unrelated finding mapped: %#v", none)
	}
	// Whole-word matching: "dos" must not match inside another word.
	if fc := MapFindingCompliance(Finding{VulnType: "Windows endoscope"}); len(fc.Classes) != 0 {
		t.Fatalf("substring matched a keyword: %#v", fc)
	}
	if fc := MapFindingCompliance(Finding{VulnType: "Badge can be cloned"}); len(fc.Classes) == 0 || fc.Classes[0] != "cloneable-credential" {
		t.Fatalf("prefix keyword did not match: %#v", fc)
	}
}

func TestReportIncludesComplianceMapping(t *testing.T) {
	report := BuildReportMarkdown(Mission{ID: "lab"}, []Finding{{ID: "F-1", Title: "Gate remote can be replayed", VulnType: "Replay", Sev: "high"}}, "", nil)
	for _, want := range []string{"**Requirements:** EN 18031: [SCM] Secure communication mechanism", "## Compliance mapping", "| ETSI EN 303 645 | 5.5 Communicate securely | F-1 Gate remote can be replayed |"} {
		if !strings.Contains(report, want) {
			t.Errorf("report lacks %q", want)
		}
	}
}

func TestParseFreqBandForms(t *testing.T) {
	cases := map[string][2]float64{
		"433.05-434.79 MHz":        {433.05e6, 434.79e6},
		"2400-2483.5MHz # ISM":     {2400e6, 2483.5e6},
		"868M-868.6M":              {868e6, 868.6e6},
		"433050000-434790000":      {433050000, 434790000},
		"5.15 - 5.35 GHz":          {5.15e9, 5.35e9},
		"125-134 kHz # LF readers": {125e3, 134e3},
	}
	for line, want := range cases {
		b, err := ParseFreqBand(line)
		if err != nil {
			t.Errorf("%s: %v", line, err)
			continue
		}
		if diff := b.FromHz - want[0]; diff > 1 || diff < -1 || b.ToHz-want[1] > 1 || b.ToHz-want[1] < -1 {
			t.Errorf("%s: got %v-%v", line, b.FromHz, b.ToHz)
		}
	}
	for _, bad := range []string{"434-433 MHz", "433 MHz", "abc", "0-1"} {
		if _, err := ParseFreqBand(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	if _, err := BuildMissionScope(nil, []string{"AA-BB-CC-DD-EE-FF"}, []string{"aa:bb:cc:dd:ee:ff"}, ""); err == nil {
		t.Error("an address both in scope and excluded was accepted")
	}
	if _, err := BuildMissionScope(nil, []string{"not-a-mac"}, nil, ""); err == nil {
		t.Error("an invalid address was accepted")
	}
}

func TestCheckScopeEnforcesBandsAndAddresses(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := store.SaveMission("default", Mission{ID: "lab"}); err != nil {
		t.Fatal(err)
	}
	s := &mcpServer{store: store, eng: &fakeEngine{}, opts: MCPOptions{Workspace: "default", Mission: "lab", AllowExec: true, AllowTransmit: true}}
	tx := "hackrf_transfer -t captures/gate.cs8 -f 433920000 -s 2000000"
	if err := s.checkScope("lab", tx, true, 0); err != nil {
		t.Fatalf("no scope should mean no extra checks: %v", err)
	}
	scope, err := BuildMissionScope([]string{"433.05-434.79 MHz"}, []string{"aa:bb:cc:dd:ee:01"}, []string{"aa:bb:cc:dd:ee:99"}, "client gate")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveMissionScope("default", "lab", scope); err != nil {
		t.Fatal(err)
	}
	if err := s.checkScope("lab", tx, true, 0); err == nil || !strings.Contains(err.Error(), "tx_frequency_hz") {
		t.Fatalf("transmission without a declared frequency accepted: %v", err)
	}
	if err := s.checkScope("lab", tx, true, 433.92e6); err != nil {
		t.Fatalf("in-band transmission refused: %v", err)
	}
	if err := s.checkScope("lab", strings.Replace(tx, "433920000", "868300000", 1), true, 433.92e6); err == nil || !strings.Contains(err.Error(), "868.3 MHz") {
		t.Fatalf("command frequency outside the band accepted: %v", err)
	}
	if err := s.checkScope("lab", "aireplay-ng --deauth 3 -a AA:BB:CC:DD:EE:02 wlan0mon", true, 2.437e9); err == nil {
		t.Fatal("transmission outside the bands accepted")
	}
	wifi, _ := BuildMissionScope([]string{"2400-2483.5 MHz"}, []string{"aa:bb:cc:dd:ee:01"}, []string{"aa:bb:cc:dd:ee:99"}, "")
	if err := store.SaveMissionScope("default", "lab", wifi); err != nil {
		t.Fatal(err)
	}
	if err := s.checkScope("lab", "aireplay-ng --deauth 3 -a AA:BB:CC:DD:EE:02 wlan0mon", true, 2.437e9); err == nil || !strings.Contains(err.Error(), "in-scope targets") {
		t.Fatalf("out-of-scope BSSID accepted: %v", err)
	}
	if err := s.checkScope("lab", "aireplay-ng --deauth 3 -a aa:bb:cc:dd:ee:01 wlan0mon", true, 2.437e9); err != nil {
		t.Fatalf("in-scope BSSID refused: %v", err)
	}
	if err := s.checkScope("lab", "airodump-ng --bssid aa-bb-cc-dd-ee-99 -w captures/x wlan0mon", false, 0); err == nil || !strings.Contains(err.Error(), "excluded") {
		t.Fatalf("excluded address accepted on a receive-only command: %v", err)
	}
	if err := s.checkScope("lab", "airodump-ng --bssid aa:bb:cc:dd:ee:02 -w captures/x wlan0mon", false, 0); err != nil {
		t.Fatalf("receive-only command refused for an address outside targets: %v", err)
	}
	empty, _ := BuildMissionScope(nil, nil, nil, "receive only")
	if err := store.SaveMissionScope("default", "lab", empty); err != nil {
		t.Fatal(err)
	}
	if err := s.checkScope("lab", tx, true, 433.92e6); err == nil || !strings.Contains(err.Error(), "allows no transmission") {
		t.Fatalf("transmission accepted with no transmit band: %v", err)
	}
	got, err := s.call("read_scope", map[string]any{})
	if err != nil || got.(map[string]any)["defined"] != true {
		t.Fatalf("read_scope: %v %#v", err, got)
	}
}

func TestNaturalLessOrdersRequirementIDs(t *testing.T) {
	for _, pair := range [][2]string{{"I7", "I10"}, {"5.5", "5.10"}, {"5.13", "6"}, {"CR 1.7", "CR 3.1"}, {"CR 7.1", "CR 7.7"}, {"ACM", "AUM"}} {
		if !naturalLess(pair[0], pair[1]) || naturalLess(pair[1], pair[0]) {
			t.Errorf("%s should sort before %s", pair[0], pair[1])
		}
	}
}
