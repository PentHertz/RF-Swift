package remote

import (
	"bufio"
	"bytes"
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type memoryStore map[string][]byte

func (m memoryStore) Set(ref string, value []byte) error {
	m[ref] = append([]byte(nil), value...)
	return nil
}

func TestPreAuthRoutesDiscloseNothing(t *testing.T) {
	for _, path := range []string{"/", "/health", "/metrics", "/openapi.json", "/does-not-exist"} {
		req := httptest.NewRequest("GET", "https://agent.invalid"+path, nil)
		res := &silentResponseWriter{}
		authenticatedHandler(ServerConfig{}).ServeHTTP(res, req)
		if !res.closed {
			t.Fatalf("%s connection was not closed", path)
		}
		if res.buf.Len() != 0 {
			t.Fatalf("%s disclosed %q", path, res.buf.String())
		}
	}
}

func TestAuthenticatedInfoEndpoint(t *testing.T) {
	req := httptest.NewRequest("GET", "https://agent.invalid/v1/info", nil)
	res := httptest.NewRecorder()
	authenticatedHandler(ServerConfig{Name: "lab", Version: "test", Bind: "127.0.0.1:8443", Authentication: AuthPolicy{ClientCertificateRequired: true}}).ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d", res.Code)
	}
	if !strings.Contains(res.Body.String(), `"clientCertificateRequired":true`) {
		t.Fatalf("unexpected info: %s", res.Body.String())
	}
}

func TestAuthenticatedControlEndpoint(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "https://agent.invalid/v1/control", strings.NewReader(`{"method":"targets.list","params":{}}`))
	res := httptest.NewRecorder()
	authenticatedHandler(ServerConfig{Control: func(_ context.Context, request ControlRequest) (any, error) {
		if request.Method != "targets.list" {
			t.Fatalf("unexpected method %q", request.Method)
		}
		return []map[string]string{{"id": "lab", "engine": "nix"}}, nil
	}}).ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d", res.Code)
	}
	if !strings.Contains(res.Body.String(), `"id":"lab"`) {
		t.Fatalf("unexpected response: %s", res.Body.String())
	}
}

type silentResponseWriter struct {
	header http.Header
	buf    bytes.Buffer
	closed bool
}

func (w *silentResponseWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}
func (w *silentResponseWriter) Write(p []byte) (int, error) { return w.buf.Write(p) }
func (w *silentResponseWriter) WriteHeader(int)             {}
func (w *silentResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return &closeMarker{w: w}, bufio.NewReadWriter(bufio.NewReader(bytes.NewReader(nil)), bufio.NewWriter(&w.buf)), nil
}

type closeMarker struct {
	net.Conn
	w *silentResponseWriter
}

func (c *closeMarker) Close() error { c.w.closed = true; return nil }
func (m memoryStore) Get(ref string) ([]byte, error) {
	v, ok := m[ref]
	if !ok {
		return nil, errors.New("not found")
	}
	return append([]byte(nil), v...), nil
}
func (m memoryStore) Delete(ref string) error { delete(m, ref); return nil }

func TestGenerateCertificateBundleEncryptedAndLoadable(t *testing.T) {
	store := memoryStore{}
	b, err := GenerateCertificateBundle(t.TempDir(), "test-agent", "127.0.0.1", store)
	if err != nil {
		t.Fatal(err)
	}
	for _, keyFile := range []string{b.CAKey, b.ServerKey, b.ClientKey} {
		raw, err := os.ReadFile(keyFile)
		if err != nil {
			t.Fatal(err)
		}
		block, rest := pem.Decode(raw)
		if block == nil || block.Type != "ENCRYPTED PRIVATE KEY" || len(rest) != 0 {
			t.Fatalf("%s is not encrypted PKCS#8", keyFile)
		}
		if bytes.Contains(raw, []byte("EC PRIVATE KEY")) {
			t.Fatalf("%s leaked a plaintext key", keyFile)
		}
		info, _ := os.Stat(keyFile)
		if info.Mode().Perm() != 0600 {
			t.Fatalf("%s mode is %o", keyFile, info.Mode().Perm())
		}
	}
	if _, err = loadEncryptedKeyPair(b.ServerCert, b.ServerKey, b.ServerKeyRef, store); err != nil {
		t.Fatalf("server key: %v", err)
	}
	if _, err = loadEncryptedKeyPair(b.ClientCert, b.ClientKey, b.ClientKeyRef, store); err != nil {
		t.Fatalf("client key: %v", err)
	}
	if _, err = os.Stat(filepath.Join(b.Directory, "bundle.json")); err != nil {
		t.Fatal(err)
	}
	if _, err = ClientConfigFromDirectory("https://127.0.0.1:8443", b.ServerFingerprint, b.Directory); err != nil {
		t.Fatalf("valid client directory rejected: %v", err)
	}
	if _, err = GenerateCertificateBundle(b.Directory, "replacement", "127.0.0.1", store); err == nil {
		t.Fatal("existing certificate bundle was silently overwritten")
	}
	for _, certFile := range []string{b.CAFile, b.ServerCert, b.ClientCert} {
		raw, err := os.ReadFile(certFile)
		if err != nil {
			t.Fatal(err)
		}
		block, _ := pem.Decode(raw)
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			t.Fatal(err)
		}
		publicName := strings.ToLower(cert.Subject.String() + " " + cert.Issuer.String())
		if strings.Contains(publicName, "rfswift") || strings.Contains(publicName, "rf swift") || strings.Contains(publicName, "penthertz") || strings.Contains(publicName, "test-agent") {
			t.Fatalf("%s leaks product or friendly name in certificate: %s", certFile, publicName)
		}
	}
}

func TestEncryptedKeyRequiresStoreReference(t *testing.T) {
	_, err := loadEncryptedKeyPair("missing", "missing", "", memoryStore{})
	if err == nil {
		t.Fatal("expected missing reference to fail closed")
	}
}

func TestRemoteAuthenticationPolicyFailsClosed(t *testing.T) {
	valid := AuthPolicy{ClientCertificateRequired: true}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid combined policy rejected: %v", err)
	}
	unsafe := []AuthPolicy{{}}
	for i, p := range unsafe {
		if err := p.Validate(); err == nil {
			t.Fatalf("unsafe policy %d accepted", i)
		}
	}
}
