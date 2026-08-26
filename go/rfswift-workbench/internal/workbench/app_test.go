package workbench

import (
	"os"
	"path/filepath"
	"testing"
)

type fakeEngine struct {
	targets []Mission
}

func (f *fakeEngine) Name() string                    { return "fake" }
func (f *fakeEngine) ListTargets() ([]Mission, error) { return f.targets, nil }
func (f *fakeEngine) Inspect(id string) (Mission, error) {
	for _, target := range f.targets {
		if target.ID == id {
			return target, nil
		}
	}
	return Mission{ID: id}, nil
}
func (f *fakeEngine) Start(string) error                  { return nil }
func (f *fakeEngine) Stop(string) error                   { return nil }
func (f *fakeEngine) Audit(string) (Posture, error)       { return Posture{}, nil }
func (f *fakeEngine) Exec(string, string) (string, error) { return "", nil }
func (f *fakeEngine) Create(req MissionCreate) (Mission, error) {
	return Mission{ID: req.Name, Title: req.Title, Engine: req.Engine}, nil
}

func testApp(t *testing.T, eng Engine) *App {
	t.Helper()
	a := &App{store: NewStore(t.TempDir()), eng: eng, ws: "default"}
	if err := a.store.CreateWorkspace(a.ws); err != nil {
		t.Fatal(err)
	}
	return a
}

func TestMissionsOverlayPersistedMetadata(t *testing.T) {
	a := testApp(t, &fakeEngine{targets: []Mission{{ID: "lab", Title: "lab", Engine: "nix", Status: "up"}}})
	want := Mission{ID: "lab", Title: "Door assessment", Notes: "authorized scope", Posture: Posture{High: 2}}
	if err := a.store.SaveMission(a.ws, want); err != nil {
		t.Fatal(err)
	}
	got, err := a.Missions()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Title != want.Title || got[0].Notes != want.Notes || got[0].EnvironmentAudit.High != 2 || got[0].Posture.High != 0 {
		t.Fatalf("persisted metadata not overlaid: %#v", got)
	}
	if got[0].Engine != "nix" || got[0].Status != "up" {
		t.Fatalf("live runtime fields were lost: %#v", got[0])
	}
}

func TestMissionFindingSummaryIsIndependentFromEnvironmentAudit(t *testing.T) {
	p := summarizeFindings([]Finding{{Sev: "critical"}, {Sev: "high"}, {Sev: "high"}})
	if p.Crit != 1 || p.High != 2 || p.Med != 0 || p.Low != 0 {
		t.Fatalf("finding summary = %#v", p)
	}
	environment := Posture{Crit: 9, Low: 4}
	if environment.Crit == p.Crit {
		t.Fatal("test fixture must keep environment and target counts distinct")
	}
}

func TestMissionsReloadsDetailedEnvironmentAudit(t *testing.T) {
	a := testApp(t, &fakeEngine{targets: []Mission{{ID: "lab", Engine: "nix", Status: "up"}}})
	dir := filepath.Join(a.store.missionDir(a.ws, "lab"), "environment-audits")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	report := `{"vulnerabilities":[{"cve":"CVE-2026-40468","severity":"critical","package":"gawk","installed_version":"5.3.2","fixed_version":"5.4.1","scope":"runtime"}]}`
	if err := os.WriteFile(filepath.Join(dir, "latest.json"), []byte(report), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := a.Missions()
	if err != nil || len(got) != 1 {
		t.Fatalf("Missions() = %#v, %v", got, err)
	}
	if len(got[0].AuditIssues) != 1 || got[0].AuditIssues[0].ID != "CVE-2026-40468" || got[0].AuditIssues[0].Component != "gawk" {
		t.Fatalf("detailed audit was not restored: %#v", got[0].AuditIssues)
	}
}

func TestMissionCreationCanBeCanceled(t *testing.T) {
	a := testApp(t, &fakeEngine{})
	if err := a.BeginMissionCreation("cancel-me"); err != nil {
		t.Fatal(err)
	}
	ctx := a.creationContext
	if !a.CancelMissionCreation("cancel-me") {
		t.Fatal("active creation was not canceled")
	}
	select {
	case <-ctx.Done():
	default:
		t.Fatal("creation context remains active")
	}
	a.FinishMissionCreation("cancel-me")
	if a.createCancel != nil || a.creationContext != nil || a.createName != "" {
		t.Fatal("creation state was not reset")
	}
}

func TestDeleteWorkspaceRejectsCurrentProject(t *testing.T) {
	a := testApp(t, &fakeEngine{})
	if err := a.DeleteWorkspace(a.ws); err == nil {
		t.Fatal("current project deletion should require closing or switching first")
	}
	if err := a.store.CreateWorkspace("old-project"); err != nil {
		t.Fatal(err)
	}
	if err := a.DeleteWorkspace("old-project"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(a.store.wsDir("old-project")); !os.IsNotExist(err) {
		t.Fatalf("deleted project still exists: %v", err)
	}
}

func TestImportCaptureCopiesArtifact(t *testing.T) {
	s := NewStore(t.TempDir())
	src := filepath.Join(t.TempDir(), "trace.pcap")
	if err := os.WriteFile(src, []byte("pcap fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := Capture{Name: "trace.pcap", Path: src, Mission: "wifi", Type: "pcap"}
	if err := s.ImportCapture("default", "wifi", &c); err != nil {
		t.Fatal(err)
	}
	if c.Path != filepath.Join("captures", "trace.pcap") {
		t.Fatalf("unexpected stored path %q", c.Path)
	}
	b, err := os.ReadFile(filepath.Join(s.capturesDir("default", "wifi"), "trace.pcap"))
	if err != nil || string(b) != "pcap fixture" {
		t.Fatalf("capture copy failed: %q, %v", b, err)
	}
	items, err := s.ListCaptures("default", "wifi")
	if err != nil || len(items) != 1 || items[0].Name != "trace.pcap" {
		t.Fatalf("capture metadata missing: %#v, %v", items, err)
	}
	if err := s.ImportCapture("default", "wifi", &Capture{Name: "trace.pcap", Path: src}); err == nil {
		t.Fatal("duplicate capture unexpectedly overwrote the existing artifact")
	}
}

func TestAgentConfigIsPrivate(t *testing.T) {
	s := NewStore(t.TempDir())
	if err := s.SaveAgentCfg(AgentCfg{Enabled: true, Client: "codex", AllowWrite: true, Yolo: map[string]bool{"codex": true}}); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(s.agentCfgPath())
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("agent config permissions = %o, want 600", st.Mode().Perm())
	}
	if cfg := s.LoadAgentCfg(); !cfg.Yolo["codex"] {
		t.Fatalf("per-client YOLO setting was not persisted: %#v", cfg)
	}
}

func TestUnsupportedConnectionIsRejected(t *testing.T) {
	a := testApp(t, &fakeEngine{})
	if err := a.SelectConnection("remote-demo"); err == nil {
		t.Fatal("unsupported remote connection unexpectedly accepted")
	}
	if err := a.SelectConnection("local"); err != nil {
		t.Fatal(err)
	}
}

func TestSaveMissionRequiresID(t *testing.T) {
	a := testApp(t, &fakeEngine{})
	if err := a.SaveMission(Mission{}); err == nil {
		t.Fatal("empty mission id unexpectedly accepted")
	}
}

func TestCreateMissionPersistsEngineTarget(t *testing.T) {
	a := testApp(t, &fakeEngine{})
	m, err := a.CreateMission(MissionCreate{Name: "nfc-lab", Title: "NFC assessment", Engine: "nix", Image: "rfid"})
	if err != nil {
		t.Fatal(err)
	}
	if m.ID != "nfc-lab" || m.Title != "NFC assessment" {
		t.Fatalf("unexpected created mission: %#v", m)
	}
	stored, err := a.store.ListMissions(a.ws)
	if err != nil || len(stored) != 1 || stored[0].ID != m.ID {
		t.Fatalf("created mission was not persisted: %#v, %v", stored, err)
	}
	if _, err := a.CreateMission(MissionCreate{Name: "../escape", Engine: "nix", Image: "rfid"}); err == nil {
		t.Fatal("unsafe mission name unexpectedly accepted")
	}
}

func TestWorkspaceNameCannotEscapeRoot(t *testing.T) {
	s := NewStore(t.TempDir())
	for _, name := range []string{"", ".", "..", "../outside", "nested/name"} {
		if err := s.CreateWorkspace(name); err == nil {
			t.Fatalf("unsafe workspace name %q accepted", name)
		}
	}
	if err := s.CreateWorkspace("assessment-01"); err != nil {
		t.Fatal(err)
	}
}

func TestCustomCaptureTypeRoundTrip(t *testing.T) {
	s := NewStore(t.TempDir())
	want := CaptureType{Key: "c_sweep", Label: "RF sweep", Exts: []string{"sweep", "s16"}, Icon: "📡"}
	if err := s.SaveCustomCaptureType("default", want); err != nil {
		t.Fatal(err)
	}
	got := s.LoadCustomCaptureTypes("default")
	if len(got) != 1 || got[0].Key != want.Key || got[0].Icon != want.Icon || len(got[0].Exts) != 2 {
		t.Fatalf("custom capture type did not round-trip: %#v", got)
	}
}
