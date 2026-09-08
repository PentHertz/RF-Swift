package dock

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExplainRegistryAuthErrorLeavesOtherErrorsAlone(t *testing.T) {
	err := errors.New("Error response from daemon: no such image")
	if got := explainRegistryAuthError(err); got != err {
		t.Fatalf("unrelated error was rewritten: %v", got)
	}
	if explainRegistryAuthError(nil) != nil {
		t.Fatal("nil must stay nil")
	}
}

func TestExplainRegistryAuthErrorAddsGuidance(t *testing.T) {
	err := errors.New(`Error response from daemon: {"message":"unable to retrieve auth token: invalid username/password: unauthorized: incorrect username or password"}`)
	got := explainRegistryAuthError(err)
	if !errors.Is(got, err) {
		t.Fatal("the original error must be wrapped")
	}
	if !strings.Contains(got.Error(), "stored login") || !strings.Contains(got.Error(), "Retry once") {
		t.Fatalf("message = %q", got.Error())
	}
}

func TestFileHasDockerHubCredentials(t *testing.T) {
	dir := t.TempDir()
	hub := filepath.Join(dir, "hub.json")
	other := filepath.Join(dir, "other.json")
	empty := filepath.Join(dir, "empty.json")
	os.WriteFile(hub, []byte(`{"auths":{"https://index.docker.io/v1/":{"auth":"dXNlcjpwYXNz"}}}`), 0o600)
	os.WriteFile(other, []byte(`{"auths":{"ghcr.io":{"auth":"dXNlcjpwYXNz"}}}`), 0o600)
	os.WriteFile(empty, []byte(`{"auths":{"docker.io":{}}}`), 0o600)
	if !fileHasDockerHubCredentials(hub) {
		t.Fatal("Docker Hub login not detected")
	}
	if fileHasDockerHubCredentials(other) || fileHasDockerHubCredentials(empty) || fileHasDockerHubCredentials(filepath.Join(dir, "missing.json")) {
		t.Fatal("false positive")
	}
}
