package workbench

import "testing"

func TestAuditDoesNotAskForRateLimitingWithClientCertificates(t *testing.T) {
	find := func(a ConnAudit, label string) *Check {
		for i := range a.Checks {
			if a.Checks[i].Label == label {
				return &a.Checks[i]
			}
		}
		return nil
	}
	mtls := Connection{ID: "remote-lab", Kind: "remote", TLS: "1.3", Cert: "AB", CertPin: true, CertDays: 300, Auth: []string{"mTLS client certificate"}, Bind: "loopback", RateLimit: false, Version: "up-to-date"}
	a := AuditConnection(mtls)
	c := find(a, "Brute-force protection")
	if c == nil || c.Severity != "ok" {
		t.Fatalf("a certificate-authenticated agent must not be told to add rate limiting: %+v", c)
	}
	if a.Posture != "ok" {
		t.Errorf("posture = %s; checks %+v", a.Posture, a.Checks)
	}
	// Anything guessable still gets the advice.
	weak := mtls
	weak.Auth = []string{"password"}
	c = find(AuditConnection(weak), "Brute-force protection")
	if c == nil || c.Severity != "warn" {
		t.Errorf("password auth without rate limiting must warn: %+v", c)
	}
}
