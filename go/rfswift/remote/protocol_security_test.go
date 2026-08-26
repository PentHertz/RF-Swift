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
