package remote

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A credential file's CA, certificate, endpoint and pins are what the
// importing side will trust. They must be bound to the passphrase, or anyone
// who can edit the file in transit could point a Workbench at their own
// agent, or make an agent accept their own clients, without knowing the
// passphrase: the encrypted key alone did not cover them.
func TestImportRefusesCredentialFilesModifiedInTransit(t *testing.T) {
	store := memoryStore{}
	bundle, err := GenerateCertificateBundle(filepath.Join(t.TempDir(), "lab"), "lab-agent", "127.0.0.1", store)
	if err != nil {
		t.Fatal(err)
	}
	pass := []byte(testPassphrase)
	client, err := IssueClientCredentials(bundle.Directory, "laptop", "", pass, store)
	if err != nil {
		t.Fatal(err)
	}
	server, err := ExportServerCredentials(bundle.Directory, pass, store)
	if err != nil {
		t.Fatal(err)
	}
	if client.MAC == "" || client.Salt == "" || server.MAC == "" {
		t.Fatal("issued files carry no integrity tag")
	}

	// An attacker's CA re-signing the file's own public key: the shape of the
	// swap that passed every certificate check before the tag existed.
	attackerKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 100))
	attackerCA := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: "Attacker CA"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	attackerCADER, _ := x509.CreateCertificate(rand.Reader, attackerCA, attackerCA, &attackerKey.PublicKey, attackerKey)
	attackerCA, _ = x509.ParseCertificate(attackerCADER)
	resign := func(certPEM string, eku x509.ExtKeyUsage) (string, string) {
		cert, _ := parseCertificatePEM(certPEM)
		serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 100))
		tpl := &x509.Certificate{SerialNumber: serial, Subject: cert.Subject, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{eku}}
		der, _ := x509.CreateCertificate(rand.Reader, tpl, attackerCA, cert.PublicKey, attackerKey)
		parsed, _ := x509.ParseCertificate(der)
		return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})), Fingerprint(parsed)
	}
	attackerCAPEM := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: attackerCADER}))

	cases := []struct {
		name string
		file RoleFile
	}{
		{"client endpoint", func() RoleFile { f := client; f.Endpoint = "https://attacker.example:8443"; return f }()},
		{"client pin", func() RoleFile { f := client; f.ServerFingerprint = strings.Repeat("AB", 32); return f }()},
		{"client CA and certificate", func() RoleFile {
			f := client
			f.CA = attackerCAPEM
			f.Certificate, f.ClientFingerprint = resign(client.Certificate, x509.ExtKeyUsageClientAuth)
			return f
		}()},
		{"server CA", func() RoleFile {
			f := server
			f.CA = attackerCAPEM
			f.Certificate, f.ServerFingerprint = resign(server.Certificate, x509.ExtKeyUsageServerAuth)
			return f
		}()},
		{"server agent name", func() RoleFile { f := server; f.Agent = "impostor"; return f }()},
		{"tag stripped but salt kept", func() RoleFile { f := client; f.MAC = ""; return f }()},
	}
	for _, tc := range cases {
		if _, err := ImportCredentials(tc.file, filepath.Join(t.TempDir(), "in"), pass, memoryStore{}); err == nil {
			t.Errorf("%s: modified credential file was imported", tc.name)
		} else if !strings.Contains(err.Error(), "modified") && !strings.Contains(err.Error(), "malformed") {
			t.Errorf("%s: unexpected error %v", tc.name, err)
		}
	}

	// Untouched files import, with no warning.
	for _, f := range []RoleFile{client, server} {
		imported, err := ImportCredentials(f, filepath.Join(t.TempDir(), f.Role), pass, memoryStore{})
		if err != nil {
			t.Fatalf("%s: genuine file refused: %v", f.Role, err)
		}
		if imported.Warning != "" {
			t.Errorf("%s: genuine file warned: %s", f.Role, imported.Warning)
		}
	}
	// A wrong passphrase is reported as such, before any certificate work.
	if _, err := ImportCredentials(client, filepath.Join(t.TempDir(), "wrong"), []byte("not the passphrase"), memoryStore{}); err == nil || !strings.Contains(err.Error(), "wrong passphrase") {
		t.Errorf("wrong passphrase: %v", err)
	}
}

// Files written before the tag existed still import, with a warning that
// names what to verify by hand, so a credential issued by an older agent is
// not locked out.
func TestImportAcceptsLegacyCredentialFileWithWarning(t *testing.T) {
	store := memoryStore{}
	bundle, err := GenerateCertificateBundle(filepath.Join(t.TempDir(), "lab"), "lab-agent", "127.0.0.1", store)
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := IssueClientCredentials(bundle.Directory, "laptop", "", []byte(testPassphrase), store)
	if err != nil {
		t.Fatal(err)
	}
	legacy.MAC, legacy.Salt = "", ""
	imported, err := ImportCredentials(legacy, filepath.Join(t.TempDir(), "legacy"), []byte(testPassphrase), memoryStore{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(imported.Warning, "fingerprint") {
		t.Fatalf("legacy file imported without the verification warning: %q", imported.Warning)
	}
}
