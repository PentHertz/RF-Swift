package remote

import (
	"crypto/tls"
	"crypto/x509"
	"testing"
	"time"
)

func TestTLSConfigParsesIPv6Endpoint(t *testing.T) {
	config, err := newTLSConfig(ClientConfig{Endpoint: "https://[::1]:8443"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if config.ServerName != "::1" {
		t.Fatalf("IPv6 server name = %q", config.ServerName)
	}
}

// A cleartext endpoint must be refused, never silently used: http.Transport
// applies the pinned mTLS config only to https:// URLs, so an accepted
// http:// endpoint would send the whole control channel in plaintext.
func TestClearTextEndpointIsRejected(t *testing.T) {
	for _, ep := range []string{"http://agent.example:8443", "ws://agent.example:8443", "ftp://agent.example"} {
		if _, err := newTLSConfig(ClientConfig{Endpoint: ep}, false); err == nil {
			t.Fatalf("non-https endpoint %q was accepted", ep)
		}
	}
}

func TestEndpointSchemeNormalization(t *testing.T) {
	for _, ep := range []string{"agent.example:8443", "rfswifts://agent.example:8443", "https://agent.example:8443"} {
		u, err := normalizeEndpoint(ep)
		if err != nil {
			t.Fatalf("endpoint %q rejected: %v", ep, err)
		}
		if u.Scheme != "https" {
			t.Fatalf("endpoint %q normalized to scheme %q", ep, u.Scheme)
		}
	}
}

func TestEndpointRejectsNonOriginComponents(t *testing.T) {
	for _, ep := range []string{
		"https://user:pass@example.test",
		"https://example.test/prefix",
		"https://example.test?redirect=evil",
		"https://example.test/#fragment",
	} {
		if _, err := normalizeEndpoint(ep); err == nil {
			t.Fatalf("endpoint with non-origin component %q was accepted", ep)
		}
	}
}

func TestPinnedExpiredCertificateIsRejected(t *testing.T) {
	cert := &x509.Certificate{Raw: []byte("expired-agent"), NotBefore: time.Now().Add(-48 * time.Hour), NotAfter: time.Now().Add(-24 * time.Hour)}
	config, err := newTLSConfig(ClientConfig{Endpoint: "https://agent.example:8443", Fingerprint: Fingerprint(cert)}, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := config.VerifyConnection(tls.ConnectionState{PeerCertificates: []*x509.Certificate{cert}}); err == nil {
		t.Fatal("expired pinned certificate was accepted")
	}
}
