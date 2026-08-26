package workbench

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"penthertz/rfswift/remote"
)

type RemoteCertificateRequest struct {
	Directory string `json:"directory"`
	Name      string `json:"name"`
	Host      string `json:"host"`
}

type RemoteProbeRequest struct {
	Endpoint     string `json:"endpoint"`
	Fingerprint  string `json:"fingerprint"`
	CAFile       string `json:"caFile"`
	ClientCert   string `json:"clientCert"`
	ClientKey    string `json:"clientKey"`
	ClientKeyRef string `json:"clientKeyRef"`
}

type RemoteConnectRequest struct{ Endpoint, Fingerprint, CredentialDirectory string }
type RemoteCommandRequest struct {
	Endpoint, Fingerprint, CredentialDirectory string
	Args                                       []string
}

func (a *App) SelectRemoteCertificateDirectory() (string, error) {
	return wruntime.OpenDirectoryDialog(a.ctx, wruntime.OpenDialogOptions{Title: "Select parent folder for the new certificate bundle"})
}

func (a *App) SelectRemoteBundle() (string, error) {
	return wruntime.OpenDirectoryDialog(a.ctx, wruntime.OpenDialogOptions{Title: "Select client credential directory"})
}

func (a *App) ConnectRemoteAgent(req RemoteConnectRequest) (Connection, error) {
	cfg, err := remote.ClientConfigFromDirectory(req.Endpoint, req.Fingerprint, req.CredentialDirectory)
	if err != nil {
		return Connection{}, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 10*time.Second)
	defer cancel()
	p, err := remote.ProbeAgent(ctx, cfg, false)
	if err != nil {
		return Connection{}, err
	}
	a.eng = &RemoteEngine{Config: cfg}
	return Connection{ID: "remote-" + strings.ToLower(strings.ReplaceAll(p.Info.Name, " ", "-")), Name: p.Info.Name, Host: req.Endpoint, Kind: "remote", TLS: p.TLS, Cipher: p.Cipher, Cert: p.Fingerprint, CertDays: p.CertDays, CertPin: true, Auth: []string{"mTLS client certificate"}, Bind: p.Info.Exposure, RateLimit: p.Info.RateLimit, Version: "up-to-date"}, nil
}

// PingRemoteAgent checks the authenticated remote session without changing the
// selected engine. The GUI uses it as a fail-closed liveness heartbeat.
func (a *App) PingRemoteAgent(req RemoteConnectRequest) error {
	cfg, err := remote.ClientConfigFromDirectory(req.Endpoint, req.Fingerprint, req.CredentialDirectory)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 3*time.Second)
	defer cancel()
	_, err = remote.ProbeAgent(ctx, cfg, false)
	return err
}

func (a *App) RunRemoteRFSwift(req RemoteCommandRequest) (remote.CommandResult, error) {
	cfg, err := remote.ClientConfigFromDirectory(req.Endpoint, req.Fingerprint, req.CredentialDirectory)
	if err != nil {
		return remote.CommandResult{}, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 5*time.Minute)
	defer cancel()
	return remote.RunCommand(ctx, cfg, req.Args)
}

// GenerateRemoteCertificates uses the same core as the rfswift CLI. Secrets go
// directly to the current user's native OS vault and never cross the Wails
// JavaScript bridge.
func (a *App) GenerateRemoteCertificates(req RemoteCertificateRequest) (remote.CertificateBundle, error) {
	if strings.TrimSpace(req.Directory) == "" {
		return remote.CertificateBundle{}, errors.New("select a certificate directory")
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = "rfswift-agent"
	}
	slug := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, name)
	destination := filepath.Join(req.Directory, slug)
	if _, err := os.Stat(destination); err == nil {
		destination = filepath.Join(req.Directory, fmt.Sprintf("%s-%s", slug, time.Now().Format("20060102-150405")))
	} else if !os.IsNotExist(err) {
		return remote.CertificateBundle{}, err
	}
	return remote.GenerateCertificateBundle(destination, name, strings.TrimSpace(req.Host), remote.OSSecretStore{})
}

// ProbeRemoteAgent verifies TLS, the server pin/CA, and the encrypted mTLS
// client key. The TLS handshake is the authentication boundary; remote engine
// streaming is implemented separately.
func (a *App) ProbeRemoteAgent(req RemoteProbeRequest) (Connection, error) {
	if strings.TrimSpace(req.Endpoint) == "" {
		return Connection{}, errors.New("agent endpoint is required")
	}
	if strings.TrimSpace(req.Fingerprint) == "" {
		return Connection{}, errors.New("a pinned server fingerprint is required")
	}
	if strings.TrimSpace(req.CAFile) == "" || strings.TrimSpace(req.ClientCert) == "" || strings.TrimSpace(req.ClientKey) == "" || strings.TrimSpace(req.ClientKeyRef) == "" {
		return Connection{}, errors.New("CA, client certificate, encrypted client key, and vault reference are required")
	}
	cfg := remote.ClientConfig{Endpoint: req.Endpoint, Fingerprint: req.Fingerprint, CAFile: req.CAFile, ClientCert: req.ClientCert, ClientKey: req.ClientKey, ClientKeyRef: req.ClientKeyRef}
	ctx, cancel := context.WithTimeout(a.ctx, 10*time.Second)
	defer cancel()
	p, err := remote.ProbeAgent(ctx, cfg, false)
	if err != nil {
		return Connection{}, err
	}
	return Connection{ID: "remote-" + strings.ToLower(strings.ReplaceAll(p.Info.Name, " ", "-")), Name: p.Info.Name, Host: req.Endpoint, Kind: "remote", TLS: p.TLS, Cipher: p.Cipher, Cert: p.Fingerprint, CertDays: p.CertDays, CertPin: true, Auth: []string{"mTLS client certificate"}, Bind: p.Info.Exposure, RateLimit: p.Info.RateLimit, Version: "up-to-date"}, nil
}
