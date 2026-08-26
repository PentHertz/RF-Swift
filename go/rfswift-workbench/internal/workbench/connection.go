package workbench

// Connection describes an rfswift agent the Workbench can attach to (local or
// remote). Security is first-class: the client audits the live connection before
// any work runs. See docs/remote-agent.md.
type Connection struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Host      string   `json:"host"`
	Kind      string   `json:"kind"` // local|remote
	TLS       string   `json:"tls"`  // "1.3" or ""
	Cipher    string   `json:"cipher"`
	Cert      string   `json:"cert"`     // pinned fingerprint
	CertDays  int      `json:"certDays"` // days until expiry (<0 = n/a)
	CertPin   bool     `json:"certPin"`
	Auth      []string `json:"auth"` // e.g. ["mTLS client certificate"]
	Bind      string   `json:"bind"` // loopback|vpn|lan|public
	RateLimit bool     `json:"rateLimit"`
	Version   string   `json:"version"` // up-to-date|outdated
}

// Check is one line of the connection security audit.
type Check struct {
	Severity string `json:"severity"` // ok|warn|crit
	Label    string `json:"label"`
	Detail   string `json:"detail"`
	Rec      string `json:"rec"` // recommendation when not ok
}

// ConnAudit is the result of auditing a connection.
type ConnAudit struct {
	Posture string  `json:"posture"` // ok|warn|crit
	Checks  []Check `json:"checks"`
}

// AuditConnection runs the connection security checks (transport, auth, cert,
// exposure, brute-force protection, agent version). AI data-egress is added by
// the caller since it depends on the AI config.
func AuditConnection(c Connection) ConnAudit {
	var ch []Check
	strong := false
	for _, a := range c.Auth {
		if containsAny(a, "mTLS", "cert") {
			strong = true
		}
	}
	switch {
	case c.Kind == "local":
		ch = append(ch, Check{"ok", "Transport", "Process-local Wails bridge and MCP stdio pipes; no RF Swift network listener.", ""})
	case c.TLS == "1.3":
		ch = append(ch, Check{"ok", "Transport encryption", "TLS 1.3, " + orDefault(c.Cipher, "AEAD cipher") + ".", ""})
	case c.TLS != "":
		ch = append(ch, Check{"warn", "Transport encryption", "TLS " + c.TLS + ".", "Require TLS 1.3, disable legacy ciphers."})
	default:
		ch = append(ch, Check{"crit", "Transport encryption", "No TLS.", "Never expose the agent without TLS 1.3."})
	}
	switch {
	case c.Kind == "local":
		ch = append(ch, Check{"ok", "Authentication", "Local OS user session.", ""})
	case strong:
		ch = append(ch, Check{"ok", "Authentication", "Phishing-resistant: " + join(c.Auth, " + ") + ".", ""})
	default:
		ch = append(ch, Check{"crit", "Authentication", "No verified client certificate.", "Require a CA-verified mTLS client certificate."})
	}
	if c.Kind != "local" {
		if c.CertPin {
			ch = append(ch, Check{"ok", "Server identity", "Server certificate pinned (" + c.Cert + ").", ""})
		} else {
			ch = append(ch, Check{"warn", "Server identity", "Server certificate not pinned.", "Pin the agent certificate (TOFU on first connect, or your own CA)."})
		}
		if c.CertDays >= 0 {
			sev := "ok"
			if c.CertDays <= 30 {
				sev = "warn"
			}
			if c.CertDays == 0 {
				sev = "crit"
			}
			rec := ""
			if sev != "ok" {
				rec = "Rotate the certificate before it expires."
			}
			ch = append(ch, Check{sev, "Certificate validity", "Expires in " + itoa(c.CertDays) + " days.", rec})
		}
	}
	switch c.Bind {
	case "loopback":
		ch = append(ch, Check{"ok", "Network exposure", "Bound to loopback only.", ""})
	case "vpn":
		ch = append(ch, Check{"ok", "Network exposure", "Reachable only over the VPN/WireGuard tunnel.", ""})
	case "lan":
		ch = append(ch, Check{"warn", "Network exposure", "Reachable on the local network.", "Bind to loopback and reach it over a VPN/SSH tunnel, or lock the firewall to known peers."})
	case "public":
		ch = append(ch, Check{"crit", "Network exposure", "Exposed to the public internet.", "Bind to loopback and reach it over a VPN/SSH tunnel, or lock the firewall to known peers."})
	default:
		ch = append(ch, Check{"warn", "Network exposure", "Unknown network exposure.", "Restrict the agent to loopback + VPN."})
	}
	if c.RateLimit {
		ch = append(ch, Check{"ok", "Brute-force protection", "Auth rate limiting / lockout enabled.", ""})
	} else {
		ch = append(ch, Check{"warn", "Brute-force protection", "No rate limiting.", "Enable auth rate limiting and account lockout."})
	}
	if c.Version == "up-to-date" {
		ch = append(ch, Check{"ok", "Agent version", "Agent is up to date.", ""})
	} else {
		ch = append(ch, Check{"warn", "Agent version", "Agent is outdated.", "Update the RF Swift agent to pick up security fixes."})
	}
	return ConnAudit{Posture: worstOf(ch), Checks: ch}
}

func worstOf(ch []Check) string {
	rank := map[string]int{"crit": 0, "warn": 1, "ok": 2}
	worst := "ok"
	for _, c := range ch {
		if rank[c.Severity] < rank[worst] {
			worst = c.Severity
		}
	}
	return worst
}
