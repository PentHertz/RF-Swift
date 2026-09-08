package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"penthertz/rfswift/remote"
)

// writeFakeBundle lays out what `rfswift agent certs init` writes, without
// touching a keyring: the files and a bundle.json recording their vault
// references.
func writeFakeBundle(t *testing.T, dir string) remote.CertificateBundle {
	t.Helper()
	b := remote.CertificateBundle{
		Directory:  dir,
		CAFile:     filepath.Join(dir, "ca.pem"),
		CAKey:      filepath.Join(dir, "ca-key.pem"),
		ServerCert: filepath.Join(dir, "server.pem"),
		ServerKey:  filepath.Join(dir, "server-key.pem"),
		ClientCert: filepath.Join(dir, "client.pem"),
		ClientKey:  filepath.Join(dir, "client-key.pem"),
		CAKeyRef:   "remote/abc/ca-key", ServerKeyRef: "remote/abc/server-key", ClientKeyRef: "remote/abc/client-key",
	}
	for _, f := range []string{b.CAFile, b.CAKey, b.ServerCert, b.ServerKey, b.ClientCert, b.ClientKey} {
		if err := os.WriteFile(f, []byte("pem"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	meta, _ := json.MarshalIndent(b, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "bundle.json"), meta, 0o600); err != nil {
		t.Fatal(err)
	}
	return b
}

func TestResolveAgentCredentialsFromBundle(t *testing.T) {
	dir := t.TempDir()
	b := writeFakeBundle(t, dir)

	// --bundle alone supplies everything.
	c, err := resolveAgentCredentials(dir, "", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if c.cert != b.ServerCert || c.key != b.ServerKey || c.keyRef != b.ServerKeyRef || c.clientCA != b.CAFile {
		t.Errorf("from bundle: %+v", c)
	}

	// --key alone: the bundle.json next to it supplies the reference and CA,
	// which is the documented explicit form without the jq step.
	c, err = resolveAgentCredentials("", b.ServerCert, b.ServerKey, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if c.keyRef != b.ServerKeyRef || c.clientCA != b.CAFile {
		t.Errorf("from --key: %+v", c)
	}

	// Explicit flags win over the bundle.
	c, err = resolveAgentCredentials(dir, "", "", "remote/other/server-key", "/elsewhere/ca.pem")
	if err != nil {
		t.Fatal(err)
	}
	if c.keyRef != "remote/other/server-key" || c.clientCA != "/elsewhere/ca.pem" || c.cert != b.ServerCert {
		t.Errorf("overrides: %+v", c)
	}

	// Another key in the bundle directory must not borrow the bundle's
	// reference: it would decrypt nothing, with a confusing vault error.
	other := filepath.Join(dir, "rotated-key.pem")
	if err := os.WriteFile(other, []byte("pem"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = resolveAgentCredentials("", b.ServerCert, other, "", "")
	if err == nil || !strings.Contains(err.Error(), "--key-ref") {
		t.Errorf("a foreign key must require its own reference, got %v", err)
	}
}

func TestResolveAgentCredentialsWithoutBundle(t *testing.T) {
	dir := t.TempDir()
	key := filepath.Join(dir, "server-key.pem")
	if err := os.WriteFile(key, []byte("pem"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := resolveAgentCredentials("", filepath.Join(dir, "server.pem"), key, "", "")
	if err == nil {
		t.Fatal("expected an error without a bundle.json or explicit reference")
	}
	for _, want := range []string{"--key-ref", "--client-ca", "--bundle"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %s", err, want)
		}
	}
	if strings.Contains(err.Error(), "--cert,") {
		t.Errorf("error %q lists --cert although it was given", err)
	}

	// A wrong --bundle directory is reported as such, not as missing flags.
	_, err = resolveAgentCredentials(filepath.Join(dir, "nope"), "", "", "", "")
	if err == nil || !strings.Contains(err.Error(), "certs init") {
		t.Errorf("bad bundle dir: %v", err)
	}

	// Everything explicit needs no bundle.
	c, err := resolveAgentCredentials("", "s.pem", key, "remote/x/server-key", "ca.pem")
	if err != nil || c.keyRef != "remote/x/server-key" || c.clientCA != "ca.pem" {
		t.Errorf("explicit: %+v, %v", c, err)
	}
}

func TestAgentCommandHasBundleFlagAndExample(t *testing.T) {
	if agentCmd.Flags().Lookup("bundle") == nil {
		t.Fatal("agent has no --bundle flag")
	}
	for _, name := range []string{"cert", "key", "key-ref", "client-ca"} {
		f := agentCmd.Flags().Lookup(name)
		if f == nil {
			t.Fatalf("agent has no --%s flag", name)
		}
		if strings.Contains(f.Usage, "(required)") {
			t.Errorf("--%s is still documented as required; --bundle supplies it", name)
		}
	}
	if !strings.Contains(agentCmd.Example, "--bundle") || !strings.Contains(agentCmd.Example, "--key-ref") {
		t.Errorf("the example should show both the bundle and the explicit form:\n%s", agentCmd.Example)
	}
}
