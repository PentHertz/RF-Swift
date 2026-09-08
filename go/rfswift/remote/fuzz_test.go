package remote

import (
	"crypto/x509"
	"encoding/pem"
	"os"
	"testing"
)

func FuzzDecryptPrivateKeyPEM(f *testing.F) {
	store := memoryStore{}
	b, err := GenerateCertificateBundle(f.TempDir(), "fuzz-agent", "localhost", store)
	if err != nil {
		f.Fatal(err)
	}
	valid, err := os.ReadFile(b.ClientKey)
	if err != nil {
		f.Fatal(err)
	}
	f.Add(valid, store[b.ClientKeyRef])
	f.Add([]byte("not a PEM"), []byte("wrong"))
	f.Add(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte{1, 2, 3}}), []byte("secret"))
	f.Fuzz(func(t *testing.T, key, password []byte) {
		plain, err := decryptPrivateKeyPEM(key, password)
		if err == nil {
			block, rest := pem.Decode(plain)
			if block == nil || block.Type != "PRIVATE KEY" || len(rest) != 0 {
				t.Fatal("successful decode returned invalid PEM")
			}
			if _, err := x509.ParsePKCS8PrivateKey(block.Bytes); err != nil {
				t.Fatalf("successful decode returned invalid PKCS#8: %v", err)
			}
		}
	})
}

func FuzzCertificateFingerprint(f *testing.F) {
	f.Add([]byte{1, 2, 3})
	f.Fuzz(func(t *testing.T, der []byte) {
		cert, err := x509.ParseCertificate(der)
		if err != nil {
			return
		}
		got := Fingerprint(cert)
		if len(got) != 64 {
			t.Fatalf("unexpected SHA-256 fingerprint length %d", len(got))
		}
	})
}
