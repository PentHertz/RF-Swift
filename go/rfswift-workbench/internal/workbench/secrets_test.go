package workbench

import (
	"bytes"
	"errors"
	"os"
	"testing"
)

type memorySecretStore struct{ values map[string][]byte }

func (m *memorySecretStore) Set(ref string, value []byte) error {
	if m.values == nil {
		m.values = map[string][]byte{}
	}
	m.values[ref] = bytes.Clone(value)
	return nil
}
func (m *memorySecretStore) Get(ref string) ([]byte, error) {
	v, ok := m.values[ref]
	if !ok {
		return nil, errors.New("not found")
	}
	return bytes.Clone(v), nil
}
func (m *memorySecretStore) Delete(ref string) error { delete(m.values, ref); return nil }

func TestMissionSecretValueNeverStoredInWorkspace(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := store.CreateWorkspace("test"); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveMission("test", Mission{ID: "lab"}); err != nil {
		t.Fatal(err)
	}
	vault := &memorySecretStore{}
	item, err := saveMissionSecret(store, vault, "test", "lab", MissionSecret{Label: "Router admin", Kind: "password", Username: "admin", Source: "note.md:12"}, "correct-horse")
	if err != nil {
		t.Fatal(err)
	}
	data, err := readPrivateFile(store.secretsPath("test", "lab"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte("correct-horse")) {
		t.Fatal("secret value leaked into workspace metadata")
	}
	value, err := vault.Get(secretRef(store.Root, "test", "lab", item.ID))
	if err != nil {
		t.Fatal(err)
	}
	if string(value) != "correct-horse" {
		t.Fatalf("unexpected vault value %q", value)
	}
	items, err := store.LoadSecrets("test", "lab")
	if err != nil || len(items) != 1 || !items[0].HasValue {
		t.Fatalf("bad metadata: %#v, %v", items, err)
	}
}

func TestMCPCollectsSecretWithoutReturningItsValue(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := store.CreateWorkspace("test"); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveMission("test", Mission{ID: "lab"}); err != nil {
		t.Fatal(err)
	}
	vault := &memorySecretStore{}
	server := &mcpServer{store: store, eng: &fakeEngine{}, vault: vault, opts: MCPOptions{Workspace: "test", Mission: "lab", AllowWrite: true}}
	result, err := server.call("save_secret", map[string]any{"label": "Device API", "kind": "api-key", "value": "token-123", "source": "capture.log:8"})
	if err != nil {
		t.Fatal(err)
	}
	item, ok := result.(MissionSecret)
	if !ok || item.ID == "" {
		t.Fatalf("unexpected result %#v", result)
	}
	encoded := []byte(item.Label + item.Source + item.Note)
	if bytes.Contains(encoded, []byte("token-123")) {
		t.Fatal("MCP result returned the secret value")
	}
	listed, err := server.call("list_secrets", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.([]MissionSecret)) != 1 {
		t.Fatalf("unexpected metadata %#v", listed)
	}
}

func readPrivateFile(path string) ([]byte, error) { return os.ReadFile(path) }
