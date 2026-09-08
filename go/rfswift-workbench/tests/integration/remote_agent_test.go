package integration_test

import (
	"testing"

	"penthertz/rfswift-workbench/internal/workbench"
)

func TestProbeRemoteAgentRequiresEndpoint(t *testing.T) {
	app := workbench.NewApp()
	if _, err := app.ProbeRemoteAgent(workbench.RemoteProbeRequest{}); err == nil {
		t.Fatal("empty remote endpoint accepted")
	}
}

func TestGenerateRemoteCertificatesRequiresDirectory(t *testing.T) {
	app := workbench.NewApp()
	if _, err := app.GenerateRemoteCertificates(workbench.RemoteCertificateRequest{}); err == nil {
		t.Fatal("empty certificate directory accepted")
	}
}
