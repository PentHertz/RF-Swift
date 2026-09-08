package remote

// Credential files that travel between machines.
//
// GenerateCertificateBundle protects every private key with a random password
// that lives only in the vault of the OS user who ran it, so its files only
// work on that machine. A RoleFile is the transferable form of one side of a
// connection: every certificate that side needs, the fingerprint it verifies
// the other side with, and its private key encrypted under a passphrase the
// person chose. On the destination, ImportCredentials checks the passphrase,
// re-encrypts the key under a fresh random password in that machine's vault
// and lays the files out the way the agent (`rfswift agent --bundle`) and the
// Workbench (ClientConfigFromDirectory) expect. The passphrase matters only
// for the transfer.

import (
	"crypto/ecdsa"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/youmark/pkcs8"
	"golang.org/x/crypto/scrypt"
)

// RoleFileFormat identifies the JSON layout; bump it on incompatible changes.
const RoleFileFormat = "rfswift-agent-credentials/1"

// MinPassphraseLength is the shortest transfer passphrase accepted.
const MinPassphraseLength = 12

// RoleFile is one side's complete credential set. Every PEM field is the file
// content, so the JSON is self-contained.
type RoleFile struct {
	Format   string    `json:"format"`
	Role     string    `json:"role"`               // "server" (the agent) or "client" (a Workbench)
	Agent    string    `json:"agent"`              // agent display name
	Host     string    `json:"host"`               // DNS name or IP the server certificate is issued for
	Endpoint string    `json:"endpoint,omitempty"` // https://host:port the client should dial
	Created  time.Time `json:"created"`
	Expires  time.Time `json:"expires"` // this side's certificate expiry

	CA           string `json:"ca"`           // PEM: the private CA, verifies the other side
	Certificate  string `json:"certificate"`  // PEM: this side's certificate
	EncryptedKey string `json:"encryptedKey"` // PEM: this side's private key, PKCS#8 encrypted with the passphrase (scrypt, AES-256-GCM)

	ServerFingerprint string `json:"serverFingerprint"`           // SHA-256 of the server certificate: the client pins it
	ClientFingerprint string `json:"clientFingerprint,omitempty"` // SHA-256 of the client certificate: lets the agent operator recognise it

	// Salt and MAC bind every field above to the passphrase: the CA that
	// verifies the other side, the certificate, the endpoint and the pins are
	// what a client or an agent will trust, and the encrypted key alone did
	// not protect them. A file written before they existed carries neither
	// and is imported with a warning (see ImportedCredentials.Warning).
	Salt string `json:"salt,omitempty"` // base64: KDF salt of the MAC key
	MAC  string `json:"mac,omitempty"`  // base64: HMAC-SHA256 of the fields under a key derived from the passphrase
}

// ImportedCredentials says where ImportCredentials put a RoleFile's content.
type ImportedCredentials struct {
	// Warning is set when the file could not be authenticated against the
	// passphrase (an older file without an integrity tag): the operator must
	// compare the endpoint and fingerprints with the values printed where the
	// file was issued.
	Warning           string `json:"warning,omitempty"`
	Role              string `json:"role"`
	Directory         string `json:"directory"`
	Agent             string `json:"agent"`
	Endpoint          string `json:"endpoint"`
	ServerFingerprint string `json:"serverFingerprint"`
	CAFile            string `json:"caFile"`
	CertFile          string `json:"certFile"`
	KeyFile           string `json:"keyFile"`
	KeyRef            string `json:"keyRef"`
}

// ValidatePassphrase enforces the minimum length of a transfer passphrase.
func ValidatePassphrase(p []byte) error {
	if len(p) < MinPassphraseLength {
		return fmt.Errorf("the passphrase must be at least %d characters", MinPassphraseLength)
	}
	return nil
}

// passphraseKeyOpts protects a key under a human passphrase: scrypt (N=2^17,
// r=8, p=1) stretches it before AES-256-GCM, unlike the vault-protected keys
// whose passwords are already random. Standard PKCS#8, so openssl reads it.
var passphraseKeyOpts = &pkcs8.Opts{Cipher: pkcs8.AES256GCM, KDFOpts: pkcs8.ScryptOpts{SaltSize: 16, CostParameter: 1 << 17, BlockSize: 8, ParallelizationParameter: 1}}

func encryptKeyWithPassphrase(key any, passphrase []byte) (string, error) {
	der, err := pkcs8.MarshalPrivateKey(key, passphrase, passphraseKeyOpts)
	if err != nil {
		return "", err
	}
	defer wipe(der)
	return string(pem.EncodeToMemory(&pem.Block{Type: "ENCRYPTED PRIVATE KEY", Bytes: der})), nil
}

func decryptKeyWithPassphrase(keyPEM string, passphrase []byte) (any, error) {
	block, rest := pem.Decode([]byte(keyPEM))
	if block == nil || block.Type != "ENCRYPTED PRIVATE KEY" || len(rest) != 0 {
		return nil, errors.New("the credential file's key is not a single encrypted PKCS#8 PEM block")
	}
	key, err := pkcs8.ParsePKCS8PrivateKey(block.Bytes, passphrase)
	if err != nil {
		return nil, errors.New("wrong passphrase, or the credential file is damaged")
	}
	return key, nil
}

// roleFileMACParams are the scrypt parameters of the MAC key: the same cost
// as the key's own encryption, so the tag is no cheaper an oracle for
// guessing the passphrase than the key already is.
const roleFileMACCost = 1 << 17

// roleFileMAC authenticates every field a client or an agent will trust with
// a key derived from the passphrase and salt. Fields are length-prefixed so
// no two field sequences share an encoding.
func roleFileMAC(f RoleFile, passphrase, salt []byte) ([]byte, error) {
	key, err := scrypt.Key(passphrase, salt, roleFileMACCost, 8, 1, 32)
	if err != nil {
		return nil, err
	}
	defer wipe(key)
	mac := hmac.New(sha256.New, key)
	for _, field := range []string{f.Format, f.Role, f.Agent, f.Host, f.Endpoint, f.Created.UTC().Format(time.RFC3339), f.Expires.UTC().Format(time.RFC3339), f.CA, f.Certificate, f.EncryptedKey, f.ServerFingerprint, f.ClientFingerprint} {
		var n [8]byte
		binary.BigEndian.PutUint64(n[:], uint64(len(field)))
		mac.Write(n[:])
		mac.Write([]byte(field))
	}
	return mac.Sum(nil), nil
}

// sealRoleFile computes the file's integrity tag. Call it last, once every
// other field is final.
func sealRoleFile(f *RoleFile, passphrase []byte) error {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return err
	}
	mac, err := roleFileMAC(*f, passphrase, salt)
	if err != nil {
		return err
	}
	f.Salt = base64.StdEncoding.EncodeToString(salt)
	f.MAC = base64.StdEncoding.EncodeToString(mac)
	return nil
}

// verifyRoleFile checks the integrity tag under the passphrase. A file
// without one (written by an older RF Swift) is accepted, and the returned
// warning tells the operator what to verify by hand.
func verifyRoleFile(f RoleFile, passphrase []byte) (string, error) {
	if f.MAC == "" && f.Salt == "" {
		return "the credential file carries no integrity tag (it was issued by an older RF Swift): before using it, compare the agent endpoint and the server fingerprint with the values printed where it was issued", nil
	}
	salt, err := base64.StdEncoding.DecodeString(f.Salt)
	want, err2 := base64.StdEncoding.DecodeString(f.MAC)
	if err != nil || err2 != nil || len(salt) < 8 || len(want) != sha256.Size {
		return "", errors.New("the credential file's integrity tag is malformed")
	}
	got, err := roleFileMAC(f, passphrase, salt)
	if err != nil {
		return "", err
	}
	if subtle.ConstantTimeCompare(got, want) != 1 {
		return "", errors.New("wrong passphrase, or the credential file was modified after it was issued")
	}
	return "", nil
}

// vaultKey decrypts a bundle's private key with the password in the vault and
// returns the key object; the plaintext never touches the disk.
func vaultKey(keyFile, ref string, store SecretStore) (any, error) {
	keyPEM, err := os.ReadFile(keyFile)
	if err != nil {
		return nil, err
	}
	password, err := store.Get(ref)
	if err != nil {
		return nil, fmt.Errorf("%w (reference %s for %s)", err, ref, keyFile)
	}
	defer wipe(password)
	plain, err := decryptPrivateKeyPEM(keyPEM, password)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", keyFile, err)
	}
	defer wipe(plain)
	block, _ := pem.Decode(plain)
	if block == nil {
		return nil, fmt.Errorf("%s: decrypted key is not PEM", keyFile)
	}
	return x509.ParsePKCS8PrivateKey(block.Bytes)
}

func readCertificatePEM(path string) (*x509.Certificate, string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	cert, err := parseCertificatePEM(string(raw))
	if err != nil {
		return nil, "", fmt.Errorf("%s: %w", path, err)
	}
	return cert, string(raw), nil
}

func parseCertificatePEM(text string) (*x509.Certificate, error) {
	block, _ := pem.Decode([]byte(text))
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, errors.New("not a PEM certificate")
	}
	return x509.ParseCertificate(block.Bytes)
}

// serverHost is the name the server certificate was issued for.
func serverHost(cert *x509.Certificate) string {
	if len(cert.DNSNames) > 0 {
		return cert.DNSNames[0]
	}
	if len(cert.IPAddresses) > 0 {
		return cert.IPAddresses[0].String()
	}
	return cert.Subject.CommonName
}

// DefaultEndpoint is the address a client dials for a server certificate
// issued for host, on the agent's default port.
func DefaultEndpoint(host string) string {
	if host == "" {
		host = "localhost"
	}
	if ip := net.ParseIP(host); ip != nil && ip.To4() == nil {
		host = "[" + host + "]"
	}
	return "https://" + host + ":8443"
}

// IssueClientCredentials signs a new client certificate with the bundle's CA
// (its key is decrypted from the vault for the signature only) and returns
// the client's RoleFile: CA, certificate, the new key encrypted under
// passphrase, and the server fingerprint to pin. Each machine gets its own
// certificate this way; the bundle's initial client certificate stays local.
func IssueClientCredentials(bundleDir, clientName, endpoint string, passphrase []byte, store SecretStore) (RoleFile, error) {
	if err := ValidatePassphrase(passphrase); err != nil {
		return RoleFile{}, err
	}
	if store == nil {
		return RoleFile{}, errors.New("secure store is required")
	}
	clientName = strings.TrimSpace(clientName)
	if clientName == "" {
		clientName = "Authorized Client"
	}
	b, err := LoadCertificateBundle(bundleDir)
	if err != nil {
		return RoleFile{}, err
	}
	if b.CAKey == "" || b.CAKeyRef == "" {
		return RoleFile{}, fmt.Errorf("%s holds no CA key; only the directory `certs init` wrote can issue client credentials", b.Directory)
	}
	caCert, caPEM, err := readCertificatePEM(b.CAFile)
	if err != nil {
		return RoleFile{}, err
	}
	serverCert, _, err := readCertificatePEM(b.ServerCert)
	if err != nil {
		return RoleFile{}, err
	}
	caKeyAny, err := vaultKey(b.CAKey, b.CAKeyRef, store)
	if err != nil {
		return RoleFile{}, fmt.Errorf("unlock CA key: %w", err)
	}
	caKey, ok := caKeyAny.(*ecdsa.PrivateKey)
	if !ok {
		return RoleFile{}, errors.New("the CA key is not an ECDSA key")
	}
	defer caKey.D.SetInt64(0)
	clientDER, clientKey, err := leaf(caCert, caKey, clientName, "", false)
	if err != nil {
		return RoleFile{}, err
	}
	defer clientKey.D.SetInt64(0)
	clientCert, err := x509.ParseCertificate(clientDER)
	if err != nil {
		return RoleFile{}, err
	}
	encrypted, err := encryptKeyWithPassphrase(clientKey, passphrase)
	if err != nil {
		return RoleFile{}, err
	}
	host := b.Host
	if host == "" {
		host = serverHost(serverCert)
	}
	if strings.TrimSpace(endpoint) == "" {
		endpoint = DefaultEndpoint(host)
	}
	f := RoleFile{
		Format: RoleFileFormat, Role: "client", Agent: b.Name, Host: host, Endpoint: strings.TrimSpace(endpoint),
		Created: time.Now().UTC().Truncate(time.Second), Expires: clientCert.NotAfter,
		CA: caPEM, Certificate: string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: clientDER})), EncryptedKey: encrypted,
		ServerFingerprint: Fingerprint(serverCert), ClientFingerprint: Fingerprint(clientCert),
	}
	if err := sealRoleFile(&f, passphrase); err != nil {
		return RoleFile{}, err
	}
	return f, nil
}

// ExportServerCredentials packs the agent's own side: CA (to verify clients),
// server certificate and its key re-encrypted under passphrase, for running
// the agent on a machine other than the one the bundle was generated on.
func ExportServerCredentials(bundleDir string, passphrase []byte, store SecretStore) (RoleFile, error) {
	if err := ValidatePassphrase(passphrase); err != nil {
		return RoleFile{}, err
	}
	if store == nil {
		return RoleFile{}, errors.New("secure store is required")
	}
	b, err := LoadCertificateBundle(bundleDir)
	if err != nil {
		return RoleFile{}, err
	}
	_, caPEM, err := readCertificatePEM(b.CAFile)
	if err != nil {
		return RoleFile{}, err
	}
	serverCert, serverPEM, err := readCertificatePEM(b.ServerCert)
	if err != nil {
		return RoleFile{}, err
	}
	key, err := vaultKey(b.ServerKey, b.ServerKeyRef, store)
	if err != nil {
		return RoleFile{}, fmt.Errorf("unlock server key: %w", err)
	}
	if k, ok := key.(*ecdsa.PrivateKey); ok {
		defer k.D.SetInt64(0)
	}
	encrypted, err := encryptKeyWithPassphrase(key, passphrase)
	if err != nil {
		return RoleFile{}, err
	}
	host := b.Host
	if host == "" {
		host = serverHost(serverCert)
	}
	f := RoleFile{
		Format: RoleFileFormat, Role: "server", Agent: b.Name, Host: host, Endpoint: DefaultEndpoint(host),
		Created: time.Now().UTC().Truncate(time.Second), Expires: serverCert.NotAfter,
		CA: caPEM, Certificate: serverPEM, EncryptedKey: encrypted, ServerFingerprint: Fingerprint(serverCert),
	}
	if err := sealRoleFile(&f, passphrase); err != nil {
		return RoleFile{}, err
	}
	return f, nil
}

// WriteRoleFile writes f as JSON, readable by its owner only, refusing to
// overwrite an existing file.
func WriteRoleFile(path string, f RoleFile) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("write credential file: %w", err)
	}
	_, werr := out.Write(append(data, '\n'))
	if cerr := out.Close(); werr == nil {
		werr = cerr
	}
	return werr
}

// ReadRoleFile reads and checks the shape of a credential file.
func ReadRoleFile(path string) (RoleFile, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return RoleFile{}, err
	}
	var f RoleFile
	if err := json.Unmarshal(raw, &f); err != nil {
		return RoleFile{}, fmt.Errorf("%s is not an RF Swift credential file: %w", path, err)
	}
	if f.Format != RoleFileFormat {
		return RoleFile{}, fmt.Errorf("%s is not an RF Swift credential file (format %q)", path, f.Format)
	}
	if f.Role != "server" && f.Role != "client" {
		return RoleFile{}, fmt.Errorf("%s has unknown role %q", path, f.Role)
	}
	if f.CA == "" || f.Certificate == "" || f.EncryptedKey == "" {
		return RoleFile{}, fmt.Errorf("%s is incomplete", path)
	}
	return f, nil
}

// ImportCredentials installs a RoleFile into dir for this machine: the
// certificate is checked against the CA and the fingerprint, the key is
// decrypted with passphrase and re-encrypted under a fresh random password
// stored in this user's vault. A client lands as ca.pem, client.pem,
// client-key.pem and connection.json (what ClientConfigFromDirectory and the
// Workbench read); a server lands as ca.pem, server.pem, server-key.pem and
// bundle.json (what `rfswift agent --bundle` reads).
func ImportCredentials(f RoleFile, dir string, passphrase []byte, store SecretStore) (ImportedCredentials, error) {
	if store == nil {
		return ImportedCredentials{}, errors.New("secure store is required")
	}
	if f.Format != RoleFileFormat || (f.Role != "server" && f.Role != "client") {
		return ImportedCredentials{}, errors.New("not an RF Swift credential file")
	}
	// Before trusting the CA, the certificate, the endpoint or the pins,
	// make sure they are the ones the passphrase holder issued.
	warning, err := verifyRoleFile(f, passphrase)
	if err != nil {
		return ImportedCredentials{}, err
	}
	caCert, err := parseCertificatePEM(f.CA)
	if err != nil {
		return ImportedCredentials{}, fmt.Errorf("credential file CA: %w", err)
	}
	cert, err := parseCertificatePEM(f.Certificate)
	if err != nil {
		return ImportedCredentials{}, fmt.Errorf("credential file certificate: %w", err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(caCert)
	usage := x509.ExtKeyUsageClientAuth
	if f.Role == "server" {
		usage = x509.ExtKeyUsageServerAuth
		if !strings.EqualFold(f.ServerFingerprint, Fingerprint(cert)) {
			return ImportedCredentials{}, errors.New("the credential file's server fingerprint does not match its certificate")
		}
	} else if f.ClientFingerprint != "" && !strings.EqualFold(f.ClientFingerprint, Fingerprint(cert)) {
		return ImportedCredentials{}, errors.New("the credential file's client fingerprint does not match its certificate")
	}
	if _, err := cert.Verify(x509.VerifyOptions{Roots: pool, KeyUsages: []x509.ExtKeyUsage{usage}}); err != nil {
		return ImportedCredentials{}, fmt.Errorf("the credential file's certificate is not signed by its CA: %w", err)
	}
	key, err := decryptKeyWithPassphrase(f.EncryptedKey, passphrase)
	if err != nil {
		return ImportedCredentials{}, err
	}
	if k, ok := key.(*ecdsa.PrivateKey); ok {
		defer k.D.SetInt64(0)
		if pub, ok := cert.PublicKey.(*ecdsa.PublicKey); !ok || !pub.Equal(&k.PublicKey) {
			return ImportedCredentials{}, errors.New("the credential file's key does not belong to its certificate")
		}
	}

	abs, err := filepath.Abs(dir)
	if err != nil {
		return ImportedCredentials{}, err
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return ImportedCredentials{}, err
	}
	side := "client"
	if f.Role == "server" {
		side = "server"
	}
	out := ImportedCredentials{Warning: warning, Role: f.Role, Directory: abs, Agent: f.Agent, Endpoint: f.Endpoint, ServerFingerprint: f.ServerFingerprint,
		CAFile: filepath.Join(abs, "ca.pem"), CertFile: filepath.Join(abs, side+".pem"), KeyFile: filepath.Join(abs, side+"-key.pem")}
	for _, existing := range []string{out.CAFile, out.CertFile, out.KeyFile} {
		if _, statErr := os.Stat(existing); statErr == nil {
			return ImportedCredentials{}, fmt.Errorf("%s already exists; import into an empty directory", existing)
		} else if !os.IsNotExist(statErr) {
			return ImportedCredentials{}, statErr
		}
	}
	// The same deterministic reference GenerateCertificateBundle uses, so
	// ClientConfigFromDirectory and LoadCertificateBundle find the password.
	out.KeyRef = "remote/" + base64.RawURLEncoding.EncodeToString([]byte(abs)) + "/" + side + "-key"
	password, err := randomSecret()
	if err != nil {
		return ImportedCredentials{}, err
	}
	defer wipe(password)
	if err := store.Set(out.KeyRef, password); err != nil {
		return ImportedCredentials{}, fmt.Errorf("store key password: %w", err)
	}
	keep := false
	defer func() {
		if !keep {
			_ = store.Delete(out.KeyRef)
			for _, p := range []string{out.CAFile, out.CertFile, out.KeyFile} {
				_ = os.Remove(p)
			}
		}
	}()
	if err := os.WriteFile(out.CAFile, []byte(f.CA), 0o644); err != nil {
		return ImportedCredentials{}, err
	}
	if err := os.WriteFile(out.CertFile, []byte(f.Certificate), 0o644); err != nil {
		return ImportedCredentials{}, err
	}
	if err := writeEncryptedKey(out.KeyFile, key, password); err != nil {
		return ImportedCredentials{}, err
	}
	var meta []byte
	metaPath := filepath.Join(abs, "connection.json")
	if f.Role == "server" {
		metaPath = filepath.Join(abs, "bundle.json")
		meta, _ = json.MarshalIndent(CertificateBundle{Directory: abs, Name: f.Agent, Host: f.Host, CAFile: out.CAFile, ServerCert: out.CertFile, ServerKey: out.KeyFile, ServerKeyRef: out.KeyRef, ServerFingerprint: f.ServerFingerprint}, "", "  ")
	} else {
		meta, _ = json.MarshalIndent(map[string]any{"agent": f.Agent, "endpoint": f.Endpoint, "serverFingerprint": f.ServerFingerprint, "clientFingerprint": f.ClientFingerprint, "expires": f.Expires, "imported": time.Now().UTC().Truncate(time.Second)}, "", "  ")
	}
	if err := os.WriteFile(metaPath, append(meta, '\n'), 0o600); err != nil {
		return ImportedCredentials{}, err
	}
	keep = true
	return out, nil
}
