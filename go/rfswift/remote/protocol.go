// Package remote implements the transport shared by the RF Swift CLI, agent,
// and Workbench. It deliberately has no GUI dependencies.
package remote

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const Protocol = "rfswift-agent/v1"

// AuthPolicy is deliberately small: a verified client certificate is the sole
// network authentication mechanism. Its private key remains encrypted at rest.
type AuthPolicy struct {
	ClientCertificateRequired bool `json:"clientCertificateRequired"`
}

func (p AuthPolicy) Validate() error {
	if !p.ClientCertificateRequired {
		return errors.New("remote access requires a client certificate")
	}
	return nil
}

type Info struct {
	Protocol, Version, Name, Exposure string
	RateLimit, ClientCertRequired     bool
	Engines                           []string
	Authentication                    AuthPolicy `json:"authentication"`
}
type ClientConfig struct{ Endpoint, Fingerprint, CAFile, ClientCert, ClientKey, ClientKeyRef string }
type Probe struct {
	Info                     Info
	Fingerprint, TLS, Cipher string
	CertDays                 int
}
type ServerConfig struct {
	Bind, CertFile, KeyFile, KeySecretRef, ClientCA, Name, Version string
	SecretStore                                                    SecretStore
	Authentication                                                 AuthPolicy
	RunCommand                                                     func(context.Context, []string) (string, error)
	Control                                                        func(context.Context, ControlRequest) (any, error)
}

type CommandRequest struct {
	Args []string `json:"args"`
}
type CommandResult struct {
	Output string `json:"output"`
	Error  string `json:"error,omitempty"`
}

type ControlRequest struct {
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}
type ControlResult struct {
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

func Fingerprint(cert *x509.Certificate) string {
	sum := sha256.Sum256(cert.Raw)
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}

func NewClient(c ClientConfig, allowUnpinned bool) (*http.Client, error) {
	tc, err := newTLSConfig(c, allowUnpinned)
	if err != nil {
		return nil, err
	}
	return &http.Client{Timeout: 8 * time.Second, Transport: &http.Transport{TLSClientConfig: tc}}, nil
}

func newTLSConfig(c ClientConfig, allowUnpinned bool) (*tls.Config, error) {
	rawEndpoint := strings.Replace(c.Endpoint, "rfswifts://", "https://", 1)
	if !strings.Contains(rawEndpoint, "://") {
		rawEndpoint = "https://" + rawEndpoint
	}
	u, err := url.Parse(rawEndpoint)
	if err != nil || u.Hostname() == "" {
		return nil, errors.New("invalid agent endpoint")
	}
	host := u.Hostname()
	tc := &tls.Config{MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13, ServerName: host}
	if c.CAFile != "" {
		pem, e := os.ReadFile(c.CAFile)
		if e != nil {
			return nil, e
		}
		pool, _ := x509.SystemCertPool()
		if pool == nil {
			pool = x509.NewCertPool()
		}
		if !pool.AppendCertsFromPEM(pem) {
			return nil, errors.New("invalid agent CA")
		}
		tc.RootCAs = pool
	}
	if c.ClientCert != "" || c.ClientKey != "" {
		cert, e := loadEncryptedKeyPair(c.ClientCert, c.ClientKey, c.ClientKeyRef, OSSecretStore{})
		if e != nil {
			return nil, e
		}
		tc.Certificates = []tls.Certificate{cert}
	}
	if c.Fingerprint != "" || allowUnpinned {
		tc.InsecureSkipVerify = true
		tc.VerifyConnection = func(cs tls.ConnectionState) error {
			if len(cs.PeerCertificates) == 0 {
				return errors.New("agent sent no certificate")
			}
			got := Fingerprint(cs.PeerCertificates[0])
			if c.Fingerprint != "" && !strings.EqualFold(strings.ReplaceAll(c.Fingerprint, ":", ""), got) {
				return fmt.Errorf("agent certificate pin changed: got %s", got)
			}
			leaf := cs.PeerCertificates[0]
			now := time.Now()
			if now.Before(leaf.NotBefore) || now.After(leaf.NotAfter) {
				return errors.New("agent certificate is not currently valid")
			}
			if c.CAFile != "" {
				intermediates := x509.NewCertPool()
				for _, cert := range cs.PeerCertificates[1:] {
					intermediates.AddCert(cert)
				}
				if _, err := leaf.Verify(x509.VerifyOptions{DNSName: host, Roots: tc.RootCAs, Intermediates: intermediates}); err != nil {
					return fmt.Errorf("agent certificate verification failed: %w", err)
				}
			}
			return nil
		}
	}
	return tc, nil
}

// ProbeAgent performs only the mutually authenticated TLS handshake and derives
// certificate posture locally without relying on an application endpoint.
func ProbeAgent(ctx context.Context, c ClientConfig, allowUnpinned bool) (Probe, error) {
	tc, e := newTLSConfig(c, allowUnpinned)
	if e != nil {
		return Probe{}, e
	}
	raw := strings.Replace(c.Endpoint, "rfswifts://", "https://", 1)
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, e := url.Parse(raw)
	if e != nil {
		return Probe{}, e
	}
	address := u.Host
	if _, _, e = net.SplitHostPort(address); e != nil {
		address = net.JoinHostPort(u.Hostname(), "8443")
	}
	dialer := &tls.Dialer{Config: tc}
	conn, e := dialer.DialContext(ctx, "tcp", address)
	if e != nil {
		return Probe{}, e
	}
	defer conn.Close()
	state := conn.(*tls.Conn).ConnectionState()
	cert := state.PeerCertificates[0]
	probe := Probe{Info: Info{Name: u.Hostname(), Exposure: "unknown", ClientCertRequired: true}, Fingerprint: Fingerprint(cert), TLS: "1.3", Cipher: tls.CipherSuiteName(state.CipherSuite), CertDays: int(time.Until(cert.NotAfter).Hours() / 24)}
	client, e := NewClient(c, allowUnpinned)
	if e != nil {
		return Probe{}, e
	}
	endpoint := strings.TrimSuffix(strings.Replace(c.Endpoint, "rfswifts://", "https://", 1), "/")
	if !strings.Contains(endpoint, "://") {
		endpoint = "https://" + endpoint
	}
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"/v1/info", nil)
	if e != nil {
		return Probe{}, e
	}
	resp, e := client.Do(req)
	if e != nil {
		return Probe{}, e
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Probe{}, fmt.Errorf("authenticated agent returned %s", resp.Status)
	}
	if e = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&probe.Info); e != nil {
		return Probe{}, e
	}
	if probe.Info.Protocol != Protocol {
		return Probe{}, fmt.Errorf("incompatible protocol %q", probe.Info.Protocol)
	}
	return probe, nil
}

func Serve(c ServerConfig) error {
	if c.CertFile == "" || c.KeyFile == "" {
		return errors.New("TLS certificate and key are required")
	}
	if err := c.Authentication.Validate(); err != nil {
		return fmt.Errorf("unsafe authentication policy: %w", err)
	}
	if c.ClientCA == "" {
		return errors.New("client CA is required by the authentication policy")
	}
	if c.Bind == "" {
		c.Bind = "127.0.0.1:8443"
	}
	if c.Name == "" {
		c.Name = "RF Swift agent"
	}
	if c.Version == "" {
		c.Version = "development"
	}
	store := c.SecretStore
	if store == nil {
		store = OSSecretStore{}
	}
	cert, e := loadEncryptedKeyPair(c.CertFile, c.KeyFile, c.KeySecretRef, store)
	if e != nil {
		return e
	}
	tc := &tls.Config{MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13, Certificates: []tls.Certificate{cert}}
	if c.ClientCA != "" {
		pem, e := os.ReadFile(c.ClientCA)
		if e != nil {
			return e
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return errors.New("invalid client CA")
		}
		tc.ClientCAs = pool
		tc.ClientAuth = tls.RequireAndVerifyClientCert
	}
	// Reaching this handler already requires a CA-verified client certificate.
	mux := authenticatedHandler(c)
	// Disable HTTP/2 before authentication so the handler can always hijack and
	// close without emitting an HTTP response or protocol metadata.
	server := &http.Server{Addr: c.Bind, Handler: mux, TLSConfig: tc, TLSNextProto: make(map[string]func(*http.Server, *tls.Conn, http.Handler)), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second}
	ln, e := tls.Listen("tcp", c.Bind, tc)
	if e != nil {
		return e
	}
	return server.Serve(ln)
}

func authenticatedHandler(c ServerConfig) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/info", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			closeWithoutResponse(w)
			return
		}
		host, _, _ := net.SplitHostPort(c.Bind)
		exposure := "lan"
		if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
			exposure = "loopback"
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(Info{Protocol: Protocol, Version: c.Version, Name: c.Name, Exposure: exposure, RateLimit: false, ClientCertRequired: true, Authentication: c.Authentication, Engines: []string{"docker", "podman", "lima", "nix"}})
	})
	mux.HandleFunc("/v1/command", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || c.RunCommand == nil {
			closeWithoutResponse(w)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		var input CommandRequest
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil || len(input.Args) == 0 || len(input.Args) > 128 {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		for _, arg := range input.Args {
			if len(arg) > 8192 || strings.IndexByte(arg, 0) >= 0 {
				http.Error(w, "invalid request", http.StatusBadRequest)
				return
			}
		}
		out, err := c.RunCommand(r.Context(), input.Args)
		result := CommandResult{Output: out}
		if err != nil {
			result.Error = err.Error()
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)
	})
	mux.HandleFunc("/v1/control", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || c.Control == nil {
			closeWithoutResponse(w)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
		var input ControlRequest
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil || strings.TrimSpace(input.Method) == "" {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		value, err := c.Control(r.Context(), input)
		result := ControlResult{}
		if err != nil {
			result.Error = err.Error()
		} else if value != nil {
			result.Result, err = json.Marshal(value)
			if err != nil {
				result.Error = err.Error()
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) { closeWithoutResponse(w) })
	return mux
}

func Control(ctx context.Context, c ClientConfig, method string, params, result any) error {
	client, err := NewClient(c, false)
	if err != nil {
		return err
	}
	// Long operations (Nix builds and image pulls) are bounded by the caller's
	// context, not the short probe timeout used by NewClient.
	client.Timeout = 0
	raw, err := json.Marshal(params)
	if err != nil {
		return err
	}
	body, err := json.Marshal(ControlRequest{Method: method, Params: raw})
	if err != nil {
		return err
	}
	endpoint := strings.TrimSuffix(strings.Replace(c.Endpoint, "rfswifts://", "https://", 1), "/")
	if !strings.Contains(endpoint, "://") {
		endpoint = "https://" + endpoint
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+"/v1/control", strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("agent control returned %s", resp.Status)
	}
	var envelope ControlResult
	if err = json.NewDecoder(io.LimitReader(resp.Body, 18<<20)).Decode(&envelope); err != nil {
		return err
	}
	if envelope.Error != "" {
		return errors.New(envelope.Error)
	}
	if result != nil && len(envelope.Result) > 0 {
		return json.Unmarshal(envelope.Result, result)
	}
	return nil
}

func RunCommand(ctx context.Context, c ClientConfig, args []string) (CommandResult, error) {
	client, err := NewClient(c, false)
	if err != nil {
		return CommandResult{}, err
	}
	body, err := json.Marshal(CommandRequest{Args: args})
	if err != nil {
		return CommandResult{}, err
	}
	endpoint := strings.TrimSuffix(strings.Replace(c.Endpoint, "rfswifts://", "https://", 1), "/")
	if !strings.Contains(endpoint, "://") {
		endpoint = "https://" + endpoint
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+"/v1/command", strings.NewReader(string(body)))
	if err != nil {
		return CommandResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return CommandResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return CommandResult{}, fmt.Errorf("agent command returned %s: %s", resp.Status, strings.TrimSpace(string(b)))
	}
	var result CommandResult
	if err = json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(&result); err != nil {
		return CommandResult{}, err
	}
	return result, nil
}

func closeWithoutResponse(w http.ResponseWriter) {
	// Do not emit a status line, headers, Date, Server, body, redirect, or
	// route hint for unknown paths.
	h, ok := w.(http.Hijacker)
	if !ok {
		return
	}
	conn, _, err := h.Hijack()
	if err == nil {
		_ = conn.Close()
	}
}
