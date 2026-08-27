package workbench

import (
	"slices"
	"testing"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

func TestPortableTargetInputsAreRejectedBeforeDialogs(t *testing.T) {
	a := NewApp()
	if _, err := a.ExportTarget("../outside", "nix", ""); err == nil {
		t.Fatal("ExportTarget accepted a path traversal target")
	}
	if _, err := a.ImportNixEnvironment("../outside", ""); err == nil {
		t.Fatal("ImportNixEnvironment accepted a path traversal name")
	}
	if _, err := a.ImportContainerArchive("unknown", "image:tag", ""); err == nil {
		t.Fatal("ImportContainerArchive accepted an unknown engine")
	}
	if _, err := a.ImportContainerArchive("docker", "bad image:tag", ""); err == nil {
		t.Fatal("ImportContainerArchive accepted whitespace in an image name")
	}
}

func TestNixImportCannotReuseContainerMissionName(t *testing.T) {
	a := testApp(t, &fakeEngine{targets: []Mission{{ID: "radio-lab", Engine: "docker"}}})
	if err := a.ensureNixImportNameAvailable("radio-lab"); err == nil {
		t.Fatal("Nix import accepted a live container mission name")
	}
	if err := a.ensureNixImportNameAvailable("another-radio-lab"); err != nil {
		t.Fatalf("unused mission name was rejected: %v", err)
	}
}

func TestPortableDialogFiltersAlwaysIncludeAllFiles(t *testing.T) {
	for name, filters := range map[string][]string{
		"nix":       filterPatterns(nixEnvironmentFilters()),
		"container": filterPatterns(containerArchiveFilters()),
	} {
		if !slices.Contains(filters, "*.*") {
			t.Fatalf("%s filters do not include an all-files fallback: %v", name, filters)
		}
	}
	wantContainer := []string{"*.tar.gz", "*.tgz", "*.tar"}
	gotContainer := filterPatterns(containerArchiveFilters())
	for _, pattern := range wantContainer {
		if !slices.Contains(gotContainer, pattern) {
			t.Errorf("container filters are missing %q: %v", pattern, gotContainer)
		}
	}
}

func TestEnsureArchiveExtension(t *testing.T) {
	tests := []struct {
		path string
		ext  string
		want string
	}{
		{"assessment", ".rfenv", "assessment.rfenv"},
		{"assessment.zip", ".rfenv", "assessment.zip.rfenv"},
		{"assessment.RFENV", ".rfenv", "assessment.RFENV"},
		{"assessment", ".rfenv.age", "assessment.rfenv.age"},
		{"radio", ".tar.gz", "radio.tar.gz"},
		{"radio.tar", ".tar.gz", "radio.tar.tar.gz"},
		{"radio.TAR.GZ", ".tar.gz", "radio.TAR.GZ"},
	}
	for _, test := range tests {
		if got := ensureArchiveExtension(test.path, test.ext); got != test.want {
			t.Errorf("ensureArchiveExtension(%q, %q) = %q, want %q", test.path, test.ext, got, test.want)
		}
	}
}

func filterPatterns(filters []wruntime.FileFilter) []string {
	out := make([]string, 0, len(filters))
	for _, filter := range filters {
		out = append(out, filter.Pattern)
	}
	return out
}
