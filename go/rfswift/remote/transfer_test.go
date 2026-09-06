package remote

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const testPassphrase = "correct horse battery staple"

// The whole journey: a bundle on the agent host, a client file issued from
// it, imported on a second machine (its own vault and directory), and an
// mTLS handshake with the agent that pins the server fingerprint the file
// carried.
func TestClientCredentialFileRoundTrip(t *testing.T) {
	agentStore := memoryStore{}
	bundle, err := GenerateCertificateBundle(filepath.Join(t.TempDir(), "lab"), "lab-agent", "127.0.0.1", agentStore)
	if err != nil {
		t.Fatal(err)
	}
	file, err := IssueClientCredentials(bundle.Directory, "laptop", "", []byte(testPassphrase), agentStore)
	if err != nil {
		t.Fatal(err)
	}
	if file.Role != "client" || file.Agent != "lab-agent" || file.Endpoint != "https://127.0.0.1:8443" || file.ServerFingerprint != bundle.ServerFingerprint {
		t.Errorf("client file header: %+v", file)
	}
	if file.ClientFingerprint == bundle.ClientFingerprint {
		t.Error("the issued client must get its own certificate, not the bundle's initial one")
	}
	path := filepath.Join(t.TempDir(), "laptop-client.json")
	if err := WriteRoleFile(path, file); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	if strings.Contains(string(raw), "\n-----BEGIN PRIVATE KEY") || strings.Contains(string(raw), "EC PRIVATE KEY") {
		t.Fatal("credential file carries a plaintext key")
	}
	// Windows keeps permissions in ACLs, not mode bits (Go reports 666 there).
	if info, _ := os.Stat(path); runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Errorf("credential file mode = %o, want 600", info.Mode().Perm())
	}
	if err := WriteRoleFile(path, file); err == nil {
		t.Error("overwriting an existing credential file must be refused")
	}

	// The second machine: its own vault, nothing from the agent host.
	laptopStore := memoryStore{}
	read, err := ReadRoleFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ImportCredentials(read, filepath.Join(t.TempDir(), "x"), []byte("wrong passphrase!"), laptopStore); err == nil || !strings.Contains(err.Error(), "passphrase") {
		t.Fatalf("wrong passphrase: %v", err)
	}
	if len(laptopStore) != 0 {
		t.Fatal("a failed import left a vault entry behind")
	}
	dir := filepath.Join(t.TempDir(), "lab-client")
	imported, err := ImportCredentials(read, dir, []byte(testPassphrase), laptopStore)
	if err != nil {
		t.Fatal(err)
	}
	if imported.Endpoint != file.Endpoint || imported.ServerFingerprint != bundle.ServerFingerprint || imported.Directory != dir {
		t.Errorf("imported = %+v", imported)
	}
	cfg, err := ClientConfigFromDirectory(imported.Endpoint, imported.ServerFingerprint, dir)
	if err != nil {
		t.Fatalf("the imported directory is not a client credential directory: %v", err)
	}
	if cfg.ClientKeyRef != imported.KeyRef {
		t.Errorf("vault reference %s differs from the deterministic one %s", imported.KeyRef, cfg.ClientKeyRef)
	}
	clientCert, err := loadEncryptedKeyPair(cfg.ClientCert, cfg.ClientKey, cfg.ClientKeyRef, laptopStore)
	if err != nil {
		t.Fatalf("imported key does not load from the laptop vault: %v", err)
	}
	if _, err := ImportCredentials(read, dir, []byte(testPassphrase), laptopStore); err == nil {
		t.Error("importing over existing files must be refused")
	}

	// Handshake with the agent, the way the Workbench does it (pin + CA + mTLS).
	addr := make(chan string, 1)
	go func() {
		_ = Serve(ServerConfig{Bind: "127.0.0.1:0", CertFile: bundle.ServerCert, KeyFile: bundle.ServerKey, KeySecretRef: bundle.ServerKeyRef, ClientCA: bundle.CAFile, SecretStore: agentStore,
			Authentication: AuthPolicy{ClientCertificateRequired: true}, OnListen: func(a string) { addr <- a }})
	}()
	var bound string
	select {
	case bound = <-addr:
	case <-time.After(10 * time.Second):
		t.Fatal("agent did not start")
	}
	tc, err := newTLSConfig(ClientConfig{Endpoint: "https://" + bound, Fingerprint: imported.ServerFingerprint, CAFile: cfg.CAFile}, false)
	if err != nil {
		t.Fatal(err)
	}
	tc.Certificates = []tls.Certificate{clientCert}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := (&tls.Dialer{Config: tc}).DialContext(ctx, "tcp", bound)
	if err != nil {
		t.Fatalf("mTLS handshake with the issued client failed: %v", err)
	}
	state := conn.(*tls.Conn).ConnectionState()
	conn.Close()
	if Fingerprint(state.PeerCertificates[0]) != imported.ServerFingerprint {
		t.Error("pinned fingerprint does not match the agent's certificate")
	}

	// A client file from another bundle pins another server: the mismatch the
	// user hit, now explained by the error.
	other, err := GenerateCertificateBundle(filepath.Join(t.TempDir(), "other"), "other", "127.0.0.1", agentStore)
	if err != nil {
		t.Fatal(err)
	}
	tc2, _ := newTLSConfig(ClientConfig{Endpoint: "https://" + bound, Fingerprint: other.ServerFingerprint, CAFile: other.CAFile}, false)
	tc2.Certificates = []tls.Certificate{clientCert}
	_, err = (&tls.Dialer{Config: tc2}).DialContext(ctx, "tcp", bound)
	if err == nil || !strings.Contains(err.Error(), "pin changed") || !strings.Contains(err.Error(), "another machine") {
		t.Errorf("a foreign pin must fail with guidance, got %v", err)
	}
}

func TestServerCredentialFileRoundTrip(t *testing.T) {
	workbenchStore := memoryStore{}
	bundle, err := GenerateCertificateBundle(filepath.Join(t.TempDir(), "lab"), "lab-agent", "lab.internal", workbenchStore)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ExportServerCredentials(bundle.Directory, []byte("short"), workbenchStore); err == nil {
		t.Error("a short passphrase must be refused")
	}
	file, err := ExportServerCredentials(bundle.Directory, []byte(testPassphrase), workbenchStore)
	if err != nil {
		t.Fatal(err)
	}
	if file.Role != "server" || file.Host != "lab.internal" || file.ServerFingerprint != bundle.ServerFingerprint {
		t.Errorf("server file header: %+v", file)
	}
	// The agent machine.
	agentStore := memoryStore{}
	dir := filepath.Join(t.TempDir(), "agent")
	imported, err := ImportCredentials(file, dir, []byte(testPassphrase), agentStore)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadCertificateBundle(dir)
	if err != nil {
		t.Fatalf("the imported directory is not usable with --bundle: %v", err)
	}
	if loaded.ServerKeyRef != imported.KeyRef || loaded.CAFile != imported.CAFile || loaded.Name != "lab-agent" || loaded.Host != "lab.internal" {
		t.Errorf("loaded = %+v", loaded)
	}
	if _, err := loadEncryptedKeyPair(loaded.ServerCert, loaded.ServerKey, loaded.ServerKeyRef, agentStore); err != nil {
		t.Errorf("imported server key does not load from the agent vault: %v", err)
	}
	if _, err := IssueClientCredentials(dir, "x", "", []byte(testPassphrase), agentStore); err == nil || !strings.Contains(err.Error(), "CA key") {
		t.Errorf("an imported server directory has no CA key and must say so, got %v", err)
	}
}

func TestReadRoleFileRejectsForeignAndTamperedFiles(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bundle.json")
	os.WriteFile(bad, []byte(`{"Directory":"/x","ServerKeyRef":"remote/x/server-key"}`), 0o600)
	if _, err := ReadRoleFile(bad); err == nil || !strings.Contains(err.Error(), "not an RF Swift credential file") {
		t.Errorf("bundle.json accepted as a credential file: %v", err)
	}
	store := memoryStore{}
	bundle, err := GenerateCertificateBundle(filepath.Join(dir, "lab"), "lab", "127.0.0.1", store)
	if err != nil {
		t.Fatal(err)
	}
	file, err := IssueClientCredentials(bundle.Directory, "laptop", "", []byte(testPassphrase), store)
	if err != nil {
		t.Fatal(err)
	}
	// A certificate swapped for one from another CA must not import.
	other, _ := GenerateCertificateBundle(filepath.Join(dir, "other"), "other", "127.0.0.1", store)
	otherCert, _ := os.ReadFile(other.ClientCert)
	tampered := file
	tampered.Certificate = string(otherCert)
	tampered.ClientFingerprint = ""
	if _, err := ImportCredentials(tampered, filepath.Join(dir, "t1"), []byte(testPassphrase), memoryStore{}); err == nil || !strings.Contains(err.Error(), "not signed by its CA") {
		t.Errorf("tampered certificate accepted: %v", err)
	}
	tampered = file
	tampered.ServerFingerprint = strings.Repeat("A", 64)
	tampered.Role = "server"
	if _, err := ImportCredentials(tampered, filepath.Join(dir, "t2"), []byte(testPassphrase), memoryStore{}); err == nil {
		t.Error("server file whose fingerprint does not match its certificate accepted")
	}
	var generic map[string]any
	data, _ := json.Marshal(file)
	json.Unmarshal(data, &generic)
	for _, key := range []string{"format", "role", "agent", "endpoint", "ca", "certificate", "encryptedKey", "serverFingerprint", "clientFingerprint", "expires"} {
		if _, ok := generic[key]; !ok {
			t.Errorf("credential file lacks %q", key)
		}
	}
}
