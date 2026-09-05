package remote

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCommandCancellation(t *testing.T) {
	c := protocolTestServer(t, ServerConfig{RunCommand: func(ctx context.Context, _ []string) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	}})
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := RunCommand(ctx, c, []string{"exec"}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cancellation lost: %v", err)
	}
}

func TestEncodedResponseLimits(t *testing.T) {
	var result any
	if err := decodeResponse(strings.NewReader(`{"output":"too large"}`), 8, &result); err == nil || !strings.Contains(err.Error(), "wire limit") {
		t.Fatalf("missing explicit oversize error: %v", err)
	}
	if maxCommandResponse < 6*MaxCommandOutput+1024 || maxControlResponse < 6*MaxTerminalOutput+1024 || maxControlResponse < 4*((MaxArtifactBytes+2)/3)+1024 {
		t.Fatal("wire limits do not cover worst-case encoding and envelopes")
	}
}

func protocolTestServer(t *testing.T, c ServerConfig) ClientConfig {
	t.Helper()
	s := httptest.NewUnstartedServer(authenticatedHandler(c))
	s.TLS = &tls.Config{MinVersion: tls.VersionTLS13}
	s.StartTLS()
	t.Cleanup(s.Close)
	return ClientConfig{Endpoint: s.URL, Fingerprint: Fingerprint(s.Certificate())}
}

func TestRegressionCommandOutputFitsClientLimit(t *testing.T) {
	want := strings.Repeat("x", 16<<20)
	c := protocolTestServer(t, ServerConfig{RunCommand: func(context.Context, []string) (string, error) { return want, nil }})
	got, err := RunCommand(context.Background(), c, []string{"version"})
	if err != nil {
		t.Fatalf("server's allowed 16 MiB output cannot be decoded: %v", err)
	}
	if got.Output != want {
		t.Fatal("output differs")
	}
}

func TestRegressionArtifactFitsControlLimit(t *testing.T) {
	data := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("x", MaxArtifactBytes)))
	c := protocolTestServer(t, ServerConfig{Control: func(context.Context, ControlRequest) (any, error) {
		return map[string]any{"Data": data, "Truncated": false}, nil
	}})
	var got json.RawMessage
	if err := Control(context.Background(), c, "artifacts.read", map[string]string{}, &got); err != nil {
		t.Fatalf("16 MiB artifact under the server's 16 MiB limit cannot be decoded: %v", err)
	}
}

func TestRegressionCommandRespectsCallerDeadline(t *testing.T) {
	c := protocolTestServer(t, ServerConfig{RunCommand: func(ctx context.Context, _ []string) (string, error) {
		select {
		case <-time.After(9 * time.Second):
			return "done", nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}})
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	if _, err := RunCommand(ctx, c, []string{"exec"}); err != nil {
		t.Fatalf("command canceled before caller deadline: %v", err)
	}
}
