package workbench

import (
	"regexp"
	"strings"
	"testing"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// Cocoa builds one UTType per pattern and aborts the process on anything that
// is not a single extension, so every pattern handed to the macOS dialogs must
// be "*.<ext>" with no further dot and no wildcard.
func TestCocoaDialogFiltersUseSingleExtensions(t *testing.T) {
	single := regexp.MustCompile(`^\*\.[A-Za-z0-9_-]+$`)
	for name, filters := range map[string][]wruntime.FileFilter{
		"nix":       nixEnvironmentFilters(),
		"container": containerArchiveFilters(),
		"project":   []wruntime.FileFilter{{DisplayName: "RF Swift Workbench project", Pattern: "*.rfswift-workbench.zip;*.zip"}},
	} {
		for _, filter := range cocoaDialogFilters(filters) {
			for _, pattern := range strings.Split(filter.Pattern, ";") {
				if !single.MatchString(pattern) {
					t.Errorf("%s: pattern %q would crash the Cocoa dialog", name, pattern)
				}
			}
		}
	}
}

func TestCocoaDialogFiltersKeepLastExtensionAndDropAllFiles(t *testing.T) {
	tests := []struct {
		name string
		in   []wruntime.FileFilter
		want []string
	}{
		{"nix", nixEnvironmentFilters(), []string{"*.rfenv", "*.age"}},
		{"container", containerArchiveFilters(), []string{"*.gz", "*.age", "*.tgz", "*.tar"}},
		{"project import", []wruntime.FileFilter{{Pattern: "*.rfswift-workbench.zip;*.zip"}}, []string{"*.zip"}},
		{"all files only", []wruntime.FileFilter{{Pattern: "*.*"}, {Pattern: "*"}}, nil},
		{"spaces and case", []wruntime.FileFilter{{Pattern: " *.JSON ; *.json"}}, []string{"*.JSON"}},
	}
	for _, test := range tests {
		got := filterPatterns(cocoaDialogFilters(test.in))
		var flat []string
		for _, pattern := range got {
			flat = append(flat, strings.Split(pattern, ";")...)
		}
		if strings.Join(flat, " ") != strings.Join(test.want, " ") {
			t.Errorf("%s: cocoaDialogFilters = %v, want %v", test.name, flat, test.want)
		}
	}
	// Display names survive so the (merged) list still reads sensibly.
	got := cocoaDialogFilters(nixEnvironmentFilters())
	if len(got) == 0 || got[0].DisplayName != nixEnvironmentFilters()[0].DisplayName {
		t.Errorf("display name lost: %+v", got)
	}
}
