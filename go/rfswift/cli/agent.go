package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sync"

	"github.com/spf13/cobra"
	"penthertz/rfswift/remote"
)

const maxAgentCommandOutput = 16 << 20

type cappedAgentOutput struct {
	mu        sync.Mutex
	buf       bytes.Buffer
	truncated bool
}

func (w *cappedAgentOutput) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	remaining := maxAgentCommandOutput - w.buf.Len()
	if remaining > 0 {
		chunk := p
		if len(chunk) > remaining {
			chunk = chunk[:remaining]
		}
		_, _ = w.buf.Write(chunk)
	}
	if len(p) > remaining {
		w.truncated = true
	}
	return len(p), nil
}

func (w *cappedAgentOutput) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := w.buf.String()
	if w.truncated {
		out += "\n[RF Swift: remote command output truncated at 16 MiB]\n"
	}
	return out
}

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Serve RF Swift engines to authenticated remote clients",
	Long:  "Run the headless RF Swift remote agent over TLS 1.3. It binds to loopback by default; use an SSH or VPN tunnel for remote access.",
	RunE: func(cmd *cobra.Command, _ []string) error {
		bind, _ := cmd.Flags().GetString("bind")
		cert, _ := cmd.Flags().GetString("cert")
		key, _ := cmd.Flags().GetString("key")
		clientCA, _ := cmd.Flags().GetString("client-ca")
		keyRef, _ := cmd.Flags().GetString("key-ref")
		name, _ := cmd.Flags().GetString("name")
		fmt.Printf("RF Swift agent listening on %s (TLS 1.3)\n", bind)
		return remote.Serve(remote.ServerConfig{Bind: bind, CertFile: cert, KeyFile: key, KeySecretRef: keyRef, ClientCA: clientCA, Name: name, Authentication: remote.AuthPolicy{ClientCertificateRequired: true}, RunCommand: runAgentCommand, Control: agentControl})
	},
}

func runAgentCommand(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 || args[0] == "agent" {
		return "", fmt.Errorf("nested agent command is not allowed")
	}
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, exe, args...)
	out := &cappedAgentOutput{}
	cmd.Stdout, cmd.Stderr = out, out
	err = cmd.Run()
	return out.String(), err
}

var agentCertsInitCmd = &cobra.Command{
	Use: "init", Short: "Generate a CA and encrypted server/client certificates",
	RunE: func(cmd *cobra.Command, _ []string) error {
		dir, _ := cmd.Flags().GetString("dir")
		name, _ := cmd.Flags().GetString("name")
		host, _ := cmd.Flags().GetString("host")
		bundle, err := remote.GenerateCertificateBundle(dir, name, host, remote.OSSecretStore{})
		if err != nil {
			return err
		}
		out, _ := json.MarshalIndent(bundle, "", "  ")
		fmt.Fprintln(os.Stdout, string(out))
		return nil
	},
}

func registerAgentCommand() {
	agentCmd.Flags().String("bind", "127.0.0.1:8443", "listen address (loopback recommended; expose through VPN/SSH)")
	agentCmd.Flags().String("cert", "", "TLS server certificate PEM (required)")
	agentCmd.Flags().String("key", "", "TLS server private key PEM (required)")
	agentCmd.Flags().String("key-ref", "", "secure-store reference for the encrypted server key (required)")
	agentCmd.Flags().String("client-ca", "", "CA PEM used to require and verify mTLS clients")
	agentCmd.Flags().String("name", "RF Swift agent", "agent display name")
	agentCertsCmd := &cobra.Command{Use: "certs", Short: "Manage remote-agent certificates"}
	agentCertsInitCmd.Flags().String("dir", "./rfswift-agent-certs", "output directory")
	agentCertsInitCmd.Flags().String("name", "rfswift-agent", "certificate name")
	agentCertsInitCmd.Flags().String("host", "localhost", "server DNS name or IP address")
	agentCertsCmd.AddCommand(agentCertsInitCmd)
	agentCmd.AddCommand(agentCertsCmd)
	rootCmd.AddCommand(agentCmd)
}
