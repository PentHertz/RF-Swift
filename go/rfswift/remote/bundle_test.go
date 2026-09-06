package remote

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadCertificateBundleFollowsMovedDirectory(t *testing.T) {
	store := memoryStore{}
	root := t.TempDir()
	first := filepath.Join(root, "first")
	b, err := GenerateCertificateBundle(first, "test-agent", "127.0.0.1", store)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadCertificateBundle(first)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ServerKey != b.ServerKey || loaded.ServerKeyRef != b.ServerKeyRef || loaded.CAFile != b.CAFile {
		t.Errorf("loaded bundle differs from the generated one: %+v", loaded)
	}

	// Moving the directory keeps the vault references (the keys were
	// encrypted under them) and rebases the file paths.
	moved := filepath.Join(root, "moved")
	if err := os.Rename(first, moved); err != nil {
		t.Fatal(err)
	}
	loaded, err = LoadCertificateBundle(moved)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ServerKeyRef != b.ServerKeyRef || loaded.ClientKeyRef != b.ClientKeyRef {
		t.Errorf("vault references changed after a move: %+v", loaded)
	}
	for _, p := range []string{loaded.CAFile, loaded.ServerCert, loaded.ServerKey, loaded.ClientCert, loaded.ClientKey} {
		if filepath.Dir(p) != moved {
			t.Errorf("%s was not rebased onto %s", p, moved)
		}
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s: %v", p, err)
		}
	}
	if loaded.Directory != moved {
		t.Errorf("directory = %s, want %s", loaded.Directory, moved)
	}

	// The moved bundle still serves: the key decrypts with the recorded
	// reference from the same store.
	if _, err := loadEncryptedKeyPair(loaded.ServerCert, loaded.ServerKey, loaded.ServerKeyRef, store); err != nil {
		t.Errorf("moved bundle does not load: %v", err)
	}

	if _, err := LoadCertificateBundle(filepath.Join(root, "nothing-here")); err == nil || !strings.Contains(err.Error(), "certs init") {
		t.Errorf("a directory without bundle.json should point at certs init, got %v", err)
	}
}

func TestServeDoesNotReportListeningBeforeCredentialsLoad(t *testing.T) {
	store := memoryStore{}
	b, err := GenerateCertificateBundle(t.TempDir(), "test-agent", "127.0.0.1", store)
	if err != nil {
		t.Fatal(err)
	}
	listened := false
	onListen := func(string) { listened = true }
	cases := map[string]ServerConfig{
		"missing reference":                    {Bind: "127.0.0.1:0", CertFile: b.ServerCert, KeyFile: b.ServerKey, ClientCA: b.CAFile, SecretStore: store, Authentication: AuthPolicy{ClientCertificateRequired: true}, OnListen: onListen},
		"unknown reference":                    {Bind: "127.0.0.1:0", CertFile: b.ServerCert, KeyFile: b.ServerKey, KeySecretRef: "remote/x/server-key", ClientCA: b.CAFile, SecretStore: store, Authentication: AuthPolicy{ClientCertificateRequired: true}, OnListen: onListen},
		"client CA is a reference, not a file": {Bind: "127.0.0.1:0", CertFile: b.ServerCert, KeyFile: b.ServerKey, KeySecretRef: b.ServerKeyRef, ClientCA: b.ClientKeyRef, SecretStore: store, Authentication: AuthPolicy{ClientCertificateRequired: true}, OnListen: onListen},
	}
	wantText := map[string]string{
		"missing reference":                    "secure-store reference",
		"unknown reference":                    b.ServerKey,
		"client CA is a reference, not a file": "ca.pem",
	}
	for name, cfg := range cases {
		err := Serve(cfg)
		if err == nil {
			t.Fatalf("%s: Serve succeeded", name)
		}
		if !strings.Contains(err.Error(), wantText[name]) {
			t.Errorf("%s: error %q should mention %q", name, err, wantText[name])
		}
		if listened {
			t.Fatalf("%s: OnListen fired although the agent could not start", name)
		}
	}
}
