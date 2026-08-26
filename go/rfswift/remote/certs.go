package remote

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/youmark/pkcs8"
)

type CertificateBundle struct {
	Directory, CAFile, CAKey, ServerCert, ServerKey, ClientCert, ClientKey string
	CAKeyRef, ServerKeyRef, ClientKeyRef                                   string
	ServerFingerprint, ClientFingerprint                                   string
}

// GenerateCertificateBundle creates a private CA and mutually authenticated
// TLS leaf certificates. Private keys are encrypted on disk and their random
// passwords are stored only in the supplied secure store.
func GenerateCertificateBundle(dir, name, host string, store SecretStore) (CertificateBundle, error) {
	if store == nil {
		return CertificateBundle{}, fmt.Errorf("secure store is required")
	}
	if name == "" {
		name = "rfswift-agent"
	}
	if host == "" {
		host = "localhost"
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return CertificateBundle{}, err
	}
	if err = os.MkdirAll(abs, 0700); err != nil {
		return CertificateBundle{}, err
	}
	for _, existing := range []string{"ca.pem", "server.pem", "client.pem", "bundle.json"} {
		if _, statErr := os.Stat(filepath.Join(abs, existing)); statErr == nil {
			return CertificateBundle{}, fmt.Errorf("certificate directory already contains %s; choose a new directory or use an explicit rotation workflow", existing)
		} else if !os.IsNotExist(statErr) {
			return CertificateBundle{}, statErr
		}
	}
	now := time.Now().Add(-5 * time.Minute)
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return CertificateBundle{}, err
	}
	// Public certificates intentionally contain no product, vendor, user, or
	// friendly agent-name banner.
	caTpl := &x509.Certificate{SerialNumber: serial(), Subject: pkix.Name{CommonName: "Local Private CA"}, NotBefore: now, NotAfter: now.AddDate(10, 0, 0), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign}
	caDER, err := x509.CreateCertificate(rand.Reader, caTpl, caTpl, &caKey.PublicKey, caKey)
	if err != nil {
		return CertificateBundle{}, err
	}
	serverDER, serverKey, err := leaf(caTpl, caKey, host, host, true)
	if err != nil {
		return CertificateBundle{}, err
	}
	clientDER, clientKey, err := leaf(caTpl, caKey, "Authorized Client", "", false)
	if err != nil {
		return CertificateBundle{}, err
	}
	b := CertificateBundle{Directory: abs, CAFile: filepath.Join(abs, "ca.pem"), CAKey: filepath.Join(abs, "ca-key.pem"), ServerCert: filepath.Join(abs, "server.pem"), ServerKey: filepath.Join(abs, "server-key.pem"), ClientCert: filepath.Join(abs, "client.pem"), ClientKey: filepath.Join(abs, "client-key.pem")}
	id := base64.RawURLEncoding.EncodeToString([]byte(abs))
	b.CAKeyRef, b.ServerKeyRef, b.ClientKeyRef = "remote/"+id+"/ca-key", "remote/"+id+"/server-key", "remote/"+id+"/client-key"
	caPass, err := randomSecret()
	if err != nil {
		return CertificateBundle{}, err
	}
	defer wipe(caPass)
	serverPass, err := randomSecret()
	if err != nil {
		return CertificateBundle{}, err
	}
	defer wipe(serverPass)
	clientPass, err := randomSecret()
	if err != nil {
		return CertificateBundle{}, err
	}
	defer wipe(clientPass)
	if err = store.Set(b.CAKeyRef, caPass); err != nil {
		return CertificateBundle{}, fmt.Errorf("store CA key password: %w", err)
	}
	rollbackCA := true
	defer func() {
		if rollbackCA {
			_ = store.Delete(b.CAKeyRef)
		}
	}()
	if err = store.Set(b.ServerKeyRef, serverPass); err != nil {
		return CertificateBundle{}, fmt.Errorf("store server key password: %w", err)
	}
	rollbackServer := true
	defer func() {
		if rollbackServer {
			_ = store.Delete(b.ServerKeyRef)
		}
	}()
	if err = store.Set(b.ClientKeyRef, clientPass); err != nil {
		return CertificateBundle{}, fmt.Errorf("store client key password: %w", err)
	}
	rollbackClient := true
	defer func() {
		if rollbackClient {
			_ = store.Delete(b.ClientKeyRef)
		}
	}()
	if err = writePEM(b.CAFile, "CERTIFICATE", caDER, 0644); err != nil {
		return CertificateBundle{}, err
	}
	if err = writeEncryptedKey(b.CAKey, caKey, caPass); err != nil {
		return CertificateBundle{}, err
	}
	if err = writePEM(b.ServerCert, "CERTIFICATE", serverDER, 0644); err != nil {
		return CertificateBundle{}, err
	}
	if err = writeEncryptedKey(b.ServerKey, serverKey, serverPass); err != nil {
		return CertificateBundle{}, err
	}
	if err = writePEM(b.ClientCert, "CERTIFICATE", clientDER, 0644); err != nil {
		return CertificateBundle{}, err
	}
	if err = writeEncryptedKey(b.ClientKey, clientKey, clientPass); err != nil {
		return CertificateBundle{}, err
	}
	serverParsed, _ := x509.ParseCertificate(serverDER)
	clientParsed, _ := x509.ParseCertificate(clientDER)
	b.ServerFingerprint, b.ClientFingerprint = Fingerprint(serverParsed), Fingerprint(clientParsed)
	meta, _ := json.MarshalIndent(b, "", "  ")
	if err = os.WriteFile(filepath.Join(abs, "bundle.json"), append(meta, '\n'), 0600); err != nil {
		return CertificateBundle{}, err
	}
	rollbackCA, rollbackServer, rollbackClient = false, false, false
	return b, nil
}

func leaf(ca *x509.Certificate, caKey *ecdsa.PrivateKey, cn, host string, server bool) ([]byte, *ecdsa.PrivateKey, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	t := &x509.Certificate{SerialNumber: serial(), Subject: pkix.Name{CommonName: cn}, NotBefore: time.Now().Add(-5 * time.Minute), NotAfter: time.Now().AddDate(1, 0, 0), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}
	if server {
		t.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
		if ip := net.ParseIP(host); ip != nil {
			t.IPAddresses = []net.IP{ip}
		} else {
			t.DNSNames = []string{host}
		}
	}
	der, err := x509.CreateCertificate(rand.Reader, t, ca, &key.PublicKey, caKey)
	return der, key, err
}
func serial() *big.Int { n, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128)); return n }
func randomSecret() ([]byte, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	out := make([]byte, base64.RawURLEncoding.EncodedLen(len(b)))
	base64.RawURLEncoding.Encode(out, b)
	wipe(b)
	return out, nil
}
func writePEM(path, typ string, der []byte, mode os.FileMode) error {
	return os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: typ, Bytes: der}), mode)
}
func writeEncryptedKey(path string, key any, password []byte) error {
	der, err := pkcs8.MarshalPrivateKey(key, password, nil)
	if err != nil {
		return err
	}
	defer wipe(der)
	return writePEM(path, "ENCRYPTED PRIVATE KEY", der, 0600)
}

// ClientConfigFromDirectory resolves generated credential filenames and the
// deterministic vault reference, keeping frontend connection forms minimal.
func ClientConfigFromDirectory(endpoint, fingerprint, dir string) (ClientConfig, error) {
	if endpoint == "" || fingerprint == "" || dir == "" {
		return ClientConfig{}, fmt.Errorf("endpoint, server fingerprint, and client credential directory are required")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return ClientConfig{}, err
	}
	id := base64.RawURLEncoding.EncodeToString([]byte(abs))
	c := ClientConfig{Endpoint: endpoint, Fingerprint: fingerprint, CAFile: filepath.Join(abs, "ca.pem"), ClientCert: filepath.Join(abs, "client.pem"), ClientKey: filepath.Join(abs, "client-key.pem"), ClientKeyRef: "remote/" + id + "/client-key"}
	for _, path := range []string{c.CAFile, c.ClientCert, c.ClientKey} {
		if _, err := os.Stat(path); err != nil {
			return ClientConfig{}, fmt.Errorf("client credential %s: %w", path, err)
		}
	}
	caPEM, err := os.ReadFile(c.CAFile)
	if err != nil {
		return ClientConfig{}, err
	}
	clientPEM, err := os.ReadFile(c.ClientCert)
	if err != nil {
		return ClientConfig{}, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return ClientConfig{}, fmt.Errorf("client credential directory contains an invalid ca.pem")
	}
	block, _ := pem.Decode(clientPEM)
	if block == nil {
		return ClientConfig{}, fmt.Errorf("client credential directory contains an invalid client.pem")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return ClientConfig{}, fmt.Errorf("parse client.pem: %w", err)
	}
	if _, err = cert.Verify(x509.VerifyOptions{Roots: pool, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}); err != nil {
		return ClientConfig{}, fmt.Errorf("client.pem is not signed by the selected ca.pem: %w", err)
	}
	return c, nil
}
