package workbench

import "testing"

func TestPortableTargetInputsAreRejectedBeforeDialogs(t *testing.T) {
	a := NewApp()
	if _, err := a.ExportTarget("../outside", "nix"); err == nil {
		t.Fatal("ExportTarget accepted a path traversal target")
	}
	if _, err := a.ImportNixEnvironment("../outside"); err == nil {
		t.Fatal("ImportNixEnvironment accepted a path traversal name")
	}
	if _, err := a.ImportContainerArchive("unknown", "image:tag"); err == nil {
		t.Fatal("ImportContainerArchive accepted an unknown engine")
	}
	if _, err := a.ImportContainerArchive("docker", "bad image:tag"); err == nil {
		t.Fatal("ImportContainerArchive accepted whitespace in an image name")
	}
}
