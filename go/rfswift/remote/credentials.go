package remote

import (
	"crypto/tls"
	"encoding/pem"
	"errors"
	"fmt"
	"os"

	"github.com/youmark/pkcs8"
	keyring "github.com/zalando/go-keyring"
)

const credentialService = "org.penthertz.rfswift"

// SecretStore keeps secret values outside connection profiles and certificate
// directories. Implementations must fail closed when secure storage is absent.
type SecretStore interface {
	Set(ref string, secret []byte) error
	Get(ref string) ([]byte, error)
	Delete(ref string) error
}

type OSSecretStore struct{}

func (OSSecretStore) Set(ref string, secret []byte) error {
	if ref == "" || len(secret) == 0 {
		return errors.New("credential reference and secret are required")
	}
	return keyring.Set(credentialService, ref, string(secret))
}
func (OSSecretStore) Get(ref string) ([]byte, error) {
	if ref == "" {
		return nil, errors.New("credential reference is required")
	}
	v, err := keyring.Get(credentialService, ref)
	if err != nil {
		return nil, fmt.Errorf("secure store: %w", err)
	}
	return []byte(v), nil
}
func (OSSecretStore) Delete(ref string) error { return keyring.Delete(credentialService, ref) }

func loadEncryptedKeyPair(certFile, keyFile, secretRef string, store SecretStore) (tls.Certificate, error) {
	if secretRef == "" {
		return tls.Certificate{}, errors.New("encrypted private key requires a secure-store reference")
	}
	certPEM, err := os.ReadFile(certFile)
	if err != nil {
		return tls.Certificate{}, err
	}
	keyPEM, err := os.ReadFile(keyFile)
	if err != nil {
		return tls.Certificate{}, err
	}
	password, err := store.Get(secretRef)
	if err != nil {
		return tls.Certificate{}, err
	}
	defer wipe(password)
	plain, err := decryptPrivateKeyPEM(keyPEM, password)
	if err != nil {
		return tls.Certificate{}, err
	}
	defer wipe(plain)
	return tls.X509KeyPair(certPEM, plain)
}

func decryptPrivateKeyPEM(keyPEM, password []byte) ([]byte, error) {
	block, rest := pem.Decode(keyPEM)
	if block == nil || block.Type != "ENCRYPTED PRIVATE KEY" || len(rest) != 0 {
		return nil, errors.New("private key must be a single password-encrypted PKCS#8 PEM block")
	}
	key, err := pkcs8.ParsePKCS8PrivateKey(block.Bytes, password)
	if err != nil {
		return nil, errors.New("cannot decrypt private key with credential from secure store")
	}
	der, err := pkcs8.MarshalPrivateKey(key, nil, nil)
	if err != nil {
		return nil, err
	}
	defer wipe(der)
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), nil
}

func wipe(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
