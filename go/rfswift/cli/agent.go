package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/spf13/cobra"
	"golang.org/x/term"
	"penthertz/rfswift/remote"
)

const maxAgentCommandOutput = remote.MaxCommandOutput

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
		name, _ := cmd.Flags().GetString("name")
		bundleDir, _ := cmd.Flags().GetString("bundle")
		cert, _ := cmd.Flags().GetString("cert")
		key, _ := cmd.Flags().GetString("key")
		keyRef, _ := cmd.Flags().GetString("key-ref")
		clientCA, _ := cmd.Flags().GetString("client-ca")
		creds, err := resolveAgentCredentials(bundleDir, cert, key, keyRef, clientCA)
		if err != nil {
			return err
		}
		return remote.Serve(remote.ServerConfig{Bind: bind, CertFile: creds.cert, KeyFile: creds.key, KeySecretRef: creds.keyRef, ClientCA: creds.clientCA, Name: name,
			Authentication: remote.AuthPolicy{ClientCertificateRequired: true}, RunCommand: runAgentCommand, Control: agentControl,
			// Printed only once the key is decrypted and the socket is bound:
			// a "listening" line above an error misled people.
			OnListen: func(addr string) { fmt.Printf("RF Swift agent listening on %s (TLS 1.3)\n", addr) }})
	},
}

type agentCredentials struct{ cert, key, keyRef, clientCA string }

// resolveAgentCredentials turns the agent's flags into files and a vault
// reference. A bundle directory (`rfswift agent certs init`, described by its
// bundle.json) supplies everything; explicit flags override each item. With
// only --key given, the bundle.json next to that key supplies the vault
// reference and the client CA, so nobody has to copy the reference out of the
// file by hand. Without a bundle, every item must be given.
func resolveAgentCredentials(bundleDir, cert, key, keyRef, clientCA string) (agentCredentials, error) {
	c := agentCredentials{cert: cert, key: key, keyRef: keyRef, clientCA: clientCA}
	dir := bundleDir
	if dir == "" && key != "" {
		dir = filepath.Dir(key)
	}
	if dir != "" {
		b, err := remote.LoadCertificateBundle(dir)
		if err != nil && bundleDir != "" {
			return c, err
		}
		if err == nil {
			if c.cert == "" {
				c.cert = b.ServerCert
			}
			if c.key == "" {
				c.key = b.ServerKey
			}
			// The recorded reference decrypts the bundle's own server key;
			// another key in the same directory needs its own --key-ref.
			if c.keyRef == "" && sameFile(c.key, b.ServerKey) {
				c.keyRef = b.ServerKeyRef
			}
			if c.clientCA == "" {
				c.clientCA = b.CAFile
			}
		}
	}
	var missing []string
	for _, item := range []struct{ flag, value string }{{"--cert", c.cert}, {"--key", c.key}, {"--key-ref", c.keyRef}, {"--client-ca", c.clientCA}} {
		if item.value == "" {
			missing = append(missing, item.flag)
		}
	}
	if len(missing) > 0 {
		return c, fmt.Errorf("missing %s: pass --bundle DIR (the directory `rfswift agent certs init` wrote, it holds bundle.json) or every flag explicitly", strings.Join(missing, ", "))
	}
	return c, nil
}

func sameFile(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	absA, errA := filepath.Abs(a)
	absB, errB := filepath.Abs(b)
	return errA == nil && errB == nil && filepath.Clean(absA) == filepath.Clean(absB)
}

func runAgentCommand(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 || args[0] == "agent" {
		return "", fmt.Errorf("nested agent command is not allowed")
	}
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	// -q keeps the banner and the release check out of output the Workbench
	// shows or parses (mission exec, tool search and install).
	cmd := exec.CommandContext(ctx, exe, append([]string{"-q"}, args...)...)
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
		fmt.Fprintf(os.Stderr, "Bundle written to %s (bundle.json holds the vault references).\n"+
			"Start the agent here with:            rfswift agent --bundle %q\n"+
			"Credentials for a Workbench elsewhere: rfswift agent certs client --bundle %q --name <machine>\n"+
			"(the keys in this directory only open with this user's vault; do not copy them, issue a client file instead)\n",
			bundle.Directory, bundle.Directory, bundle.Directory)
		return nil
	},
}

// readPassphrase takes the transfer passphrase from --passphrase-file, else
// from the terminal without echo (twice when confirm is set), else from a
// line on standard input.
func readPassphrase(file string, confirm bool) ([]byte, error) {
	if file != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		return bytes.TrimRight(data, "\r\n"), nil
	}
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && line == "" {
			return nil, errors.New("no passphrase on standard input (use --passphrase-file or a terminal)")
		}
		return []byte(strings.TrimRight(line, "\r\n")), nil
	}
	fmt.Fprint(os.Stderr, "Passphrase for the credential file: ")
	first, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return nil, err
	}
	if !confirm {
		return first, nil
	}
	fmt.Fprint(os.Stderr, "Repeat the passphrase: ")
	second, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(first, second) {
		return nil, errors.New("the passphrases differ")
	}
	return first, nil
}

// wipeBytes zeroes a passphrase once it has served; the transfer passphrase
// is never written anywhere.
func wipeBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

func fileSlug(name string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, strings.TrimSpace(name))
}

var agentCertsClientCmd = &cobra.Command{
	Use:   "client",
	Short: "Issue credentials for a Workbench on another machine, as one passphrase-protected JSON file",
	Long: "Sign a new client certificate with the bundle's CA and write one JSON file holding everything that client needs: the CA, its certificate, its private key encrypted with a passphrase you choose, the agent's address and the server fingerprint to pin. " +
		"Move the file to the other machine and import it there (Workbench: Connection & security > Add agent > Import client credentials; CLI: rfswift agent certs import). " +
		"Each machine gets its own certificate; the keys of the bundle directory stay where they were generated.",
	RunE: func(cmd *cobra.Command, _ []string) error {
		bundleDir, _ := cmd.Flags().GetString("bundle")
		name, _ := cmd.Flags().GetString("name")
		endpoint, _ := cmd.Flags().GetString("endpoint")
		out, _ := cmd.Flags().GetString("out")
		passFile, _ := cmd.Flags().GetString("passphrase-file")
		if bundleDir == "" {
			return errors.New("--bundle DIR is required (the directory `rfswift agent certs init` wrote)")
		}
		if out == "" {
			out = filepath.Join(bundleDir, "clients", fileSlug(name)+"-client.json")
		}
		passphrase, err := readPassphrase(passFile, true)
		if err != nil {
			return err
		}
		defer wipeBytes(passphrase)
		file, err := remote.IssueClientCredentials(bundleDir, name, endpoint, passphrase, remote.OSSecretStore{})
		if err != nil {
			return err
		}
		if err := remote.WriteRoleFile(out, file); err != nil {
			return err
		}
		fmt.Fprintf(os.Stdout, "Client credentials for %q written to %s\n  agent endpoint:     %s\n  server fingerprint: %s\n  client fingerprint: %s\n  valid until:        %s\n",
			name, out, file.Endpoint, file.ServerFingerprint, file.ClientFingerprint, file.Expires.Format("2006-01-02"))
		fmt.Fprintln(os.Stdout, "Move the file to the client machine and import it there: Workbench > Connection & security > Add agent > Import client credentials, or: rfswift agent certs import FILE --dir DIR")
		return nil
	},
}

var agentCertsExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export the agent's own credentials as one passphrase-protected JSON file, to run the agent on another machine",
	RunE: func(cmd *cobra.Command, _ []string) error {
		bundleDir, _ := cmd.Flags().GetString("bundle")
		out, _ := cmd.Flags().GetString("out")
		passFile, _ := cmd.Flags().GetString("passphrase-file")
		if bundleDir == "" {
			return errors.New("--bundle DIR is required (the directory `rfswift agent certs init` wrote)")
		}
		if out == "" {
			out = filepath.Join(bundleDir, "server-credentials.json")
		}
		passphrase, err := readPassphrase(passFile, true)
		if err != nil {
			return err
		}
		defer wipeBytes(passphrase)
		file, err := remote.ExportServerCredentials(bundleDir, passphrase, remote.OSSecretStore{})
		if err != nil {
			return err
		}
		if err := remote.WriteRoleFile(out, file); err != nil {
			return err
		}
		fmt.Fprintf(os.Stdout, "Server credentials written to %s (fingerprint %s).\nOn the agent machine: rfswift agent certs import %s --dir DIR, then rfswift agent --bundle DIR\n", out, file.ServerFingerprint, filepath.Base(out))
		return nil
	},
}

var agentCertsImportCmd = &cobra.Command{
	Use:   "import FILE",
	Short: "Install a credential file on this machine, re-protecting its key with this user's vault",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, _ := cmd.Flags().GetString("dir")
		passFile, _ := cmd.Flags().GetString("passphrase-file")
		file, err := remote.ReadRoleFile(args[0])
		if err != nil {
			return err
		}
		if dir == "" {
			dir = strings.TrimSuffix(args[0], filepath.Ext(args[0]))
		}
		passphrase, err := readPassphrase(passFile, false)
		if err != nil {
			return err
		}
		defer wipeBytes(passphrase)
		imported, err := remote.ImportCredentials(file, dir, passphrase, remote.OSSecretStore{})
		if err != nil {
			return err
		}
		switch imported.Role {
		case "server":
			fmt.Fprintf(os.Stdout, "Server credentials installed in %s.\nStart the agent with:  rfswift agent --bundle %q\n", imported.Directory, imported.Directory)
		default:
			fmt.Fprintf(os.Stdout, "Client credentials installed in %s.\n  agent endpoint:     %s\n  server fingerprint: %s\nIn the Workbench, enter the endpoint, the fingerprint and this directory as the client secrets location.\n", imported.Directory, imported.Endpoint, imported.ServerFingerprint)
		}
		return nil
	},
}

func registerAgentCommand() {
	agentCmd.Flags().String("bind", "127.0.0.1:8443", "listen address (loopback recommended; expose through VPN/SSH)")
	agentCmd.Flags().String("bundle", "", "directory written by 'rfswift agent certs init'; supplies --cert, --key, --key-ref and --client-ca")
	agentCmd.Flags().String("cert", "", "TLS server certificate PEM (default: server.pem of --bundle)")
	agentCmd.Flags().String("key", "", "TLS server private key PEM (default: server-key.pem of --bundle)")
	agentCmd.Flags().String("key-ref", "", "secure-store reference of the encrypted server key (default: ServerKeyRef of the bundle.json next to --key)")
	agentCmd.Flags().String("client-ca", "", "CA PEM used to require and verify mTLS clients (default: ca.pem of --bundle)")
	agentCmd.Flags().String("name", "RF Swift agent", "agent display name")
	agentCertsCmd := &cobra.Command{Use: "certs", Short: "Manage remote-agent certificates"}
	agentCertsInitCmd.Flags().String("dir", "./rfswift-agent-certs", "output directory")
	agentCertsInitCmd.Flags().String("name", "rfswift-agent", "certificate name")
	agentCertsInitCmd.Flags().String("host", "localhost", "server DNS name or IP address")
	agentCertsClientCmd.Flags().String("bundle", "", "directory written by 'rfswift agent certs init' (holds the CA key)")
	agentCertsClientCmd.Flags().String("name", "workbench", "name of the client machine or person (certificate subject)")
	agentCertsClientCmd.Flags().String("endpoint", "", "address the client dials (default: https://<host of the server certificate>:8443)")
	agentCertsClientCmd.Flags().String("out", "", "output file (default: <bundle>/clients/<name>-client.json)")
	agentCertsClientCmd.Flags().String("passphrase-file", "", "read the transfer passphrase from this file instead of the terminal")
	agentCertsExportCmd.Flags().String("bundle", "", "directory written by 'rfswift agent certs init'")
	agentCertsExportCmd.Flags().String("out", "", "output file (default: <bundle>/server-credentials.json)")
	agentCertsExportCmd.Flags().String("passphrase-file", "", "read the transfer passphrase from this file instead of the terminal")
	agentCertsImportCmd.Flags().String("dir", "", "directory to install into (default: the file's name without .json)")
	agentCertsImportCmd.Flags().String("passphrase-file", "", "read the transfer passphrase from this file instead of the terminal")
	agentCertsCmd.AddCommand(agentCertsInitCmd, agentCertsClientCmd, agentCertsExportCmd, agentCertsImportCmd)
	agentCmd.AddCommand(agentCertsCmd)
	rootCmd.AddCommand(agentCmd)
}
