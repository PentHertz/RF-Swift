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

// RemoteImportRequest installs a credential file (remote.RoleFile) written on
// another machine. Directory empty: next to the file, named after it.
type RemoteImportRequest struct {
	File       string `json:"file"`
	Directory  string `json:"directory"`
	Passphrase string `json:"passphrase"`
}

// RemoteIssueRequest issues client credentials from a bundle for a machine
// elsewhere (Out empty: <bundle>/clients/<name>-client.json), or exports the
// bundle's server side when ClientName is empty and Server is set.
type RemoteIssueRequest struct {
	BundleDirectory string `json:"bundleDirectory"`
	ClientName      string `json:"clientName"`
	Endpoint        string `json:"endpoint"`
	Out             string `json:"out"`
	Passphrase      string `json:"passphrase"`
	Server          bool   `json:"server"`
}

func (a *App) SelectRemoteCertificateDirectory() (string, error) {
	return wruntime.OpenDirectoryDialog(a.ctx, wruntime.OpenDialogOptions{Title: "Select parent folder for the new certificate bundle"})
}

func (a *App) SelectRemoteBundle() (string, error) {
	return wruntime.OpenDirectoryDialog(a.ctx, wruntime.OpenDialogOptions{Title: "Select client credential directory"})
}

func (a *App) SelectRemoteCredentialFile() (string, error) {
	return wruntime.OpenFileDialog(a.ctx, wruntime.OpenDialogOptions{Title: "Select an RF Swift credential file", Filters: []wruntime.FileFilter{{DisplayName: "RF Swift credentials (*.json)", Pattern: "*.json"}}})
}

func (a *App) SelectRemoteCredentialSavePath(suggested string) (string, error) {
	return wruntime.SaveFileDialog(a.ctx, wruntime.SaveDialogOptions{Title: "Save the credential file", DefaultFilename: suggested, Filters: []wruntime.FileFilter{{DisplayName: "RF Swift credentials (*.json)", Pattern: "*.json"}}})
}

// ImportRemoteCredentials installs a credential file from another machine:
// the transfer passphrase (typed by the person, it is not a vault secret)
// unlocks the key, which is re-encrypted under a fresh random password in
// this user's vault. The result carries the endpoint, fingerprint and
// directory the connection form needs.
func (a *App) ImportRemoteCredentials(req RemoteImportRequest) (remote.ImportedCredentials, error) {
	if strings.TrimSpace(req.File) == "" {
		return remote.ImportedCredentials{}, errors.New("select a credential file")
	}
	file, err := remote.ReadRoleFile(req.File)
	if err != nil {
		return remote.ImportedCredentials{}, err
	}
	dir := strings.TrimSpace(req.Directory)
	if dir == "" {
		dir = strings.TrimSuffix(req.File, filepath.Ext(req.File))
	}
	return remote.ImportCredentials(file, dir, []byte(req.Passphrase), remote.OSSecretStore{})
}

// IssueRemoteCredentials writes a credential file for another machine from a
// bundle generated here: a newly signed client (default) or this agent's own
// server side (req.Server). Returns the file's path.
func (a *App) IssueRemoteCredentials(req RemoteIssueRequest) (string, error) {
	bundle := strings.TrimSpace(req.BundleDirectory)
	if bundle == "" {
		return "", errors.New("select the bundle folder written by Generate or by rfswift agent certs init")
	}
	var file remote.RoleFile
	var err error
	out := strings.TrimSpace(req.Out)
	if req.Server {
		file, err = remote.ExportServerCredentials(bundle, []byte(req.Passphrase), remote.OSSecretStore{})
		if out == "" {
			out = filepath.Join(bundle, "server-credentials.json")
		}
	} else {
		name := strings.TrimSpace(req.ClientName)
		if name == "" {
			name = "workbench"
		}
		file, err = remote.IssueClientCredentials(bundle, name, req.Endpoint, []byte(req.Passphrase), remote.OSSecretStore{})
		if out == "" {
			out = filepath.Join(bundle, "clients", slugify(name)+"-client.json")
		}
	}
	if err != nil {
		return "", err
	}
	if err := remote.WriteRoleFile(out, file); err != nil {
		return "", err
	}
	return out, nil
}

func slugify(name string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, name)
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
	a.setEngine(&RemoteEngine{Config: cfg})
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
	slug := slugify(name)
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
