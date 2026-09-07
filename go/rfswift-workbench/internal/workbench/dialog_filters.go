package workbench

import (
	"runtime"
	"strings"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// nativeDialogFilters adapts file-dialog filters to the native dialog backend
// before they reach Wails. Every OpenFileDialog/SaveFileDialog call that has
// filters must go through it.
//
// On macOS, Wails turns each pattern into a UTType with
// [UTType typeWithFilenameExtension:] and appends the result to an NSArray.
// That lookup returns nil for anything that is not a single extension
// ("rfenv.age", "tar.gz", "rfswift-workbench.zip"), and inserting nil into an
// NSArray raises NSInvalidArgumentException: the whole Workbench aborts the
// moment the export or import dialog opens. Cocoa has no all-files pattern
// either; "*.*" only produces a bogus type for the extension "*". So on macOS
// each pattern keeps its last extension only and the all-files fallback is
// dropped. GTK and the Windows shell accept the patterns as they are (and are
// the ones that show DisplayName), so they keep the originals.
func nativeDialogFilters(filters []wruntime.FileFilter) []wruntime.FileFilter {
	if runtime.GOOS != "darwin" {
		return filters
	}
	return cocoaDialogFilters(filters)
}

// cocoaDialogFilters reduces filters to what NSOpenPanel/NSSavePanel can take:
// one plain extension per pattern, no wildcard-only pattern, no duplicates
// (the panel merges every filter into a single allowed-types list anyway).
func cocoaDialogFilters(filters []wruntime.FileFilter) []wruntime.FileFilter {
	seen := map[string]bool{}
	out := make([]wruntime.FileFilter, 0, len(filters))
	for _, filter := range filters {
		var patterns []string
		for _, pattern := range strings.Split(filter.Pattern, ";") {
			ext := cocoaExtension(pattern)
			key := strings.ToLower(ext)
			if ext == "" || seen[key] {
				continue
			}
			seen[key] = true
			patterns = append(patterns, "*."+ext)
		}
		if len(patterns) == 0 {
			continue
		}
		out = append(out, wruntime.FileFilter{DisplayName: filter.DisplayName, Pattern: strings.Join(patterns, ";")})
	}
	return out
}

// cocoaExtension reduces one dialog pattern to the single extension a UTType
// can be made from: "*.tar.gz" -> "gz", "*.rfenv" -> "rfenv". It is "" for a
// pattern that matches everything ("*.*", "*") or carries no extension.
func cocoaExtension(pattern string) string {
	pattern = strings.TrimSpace(pattern)
	if i := strings.LastIndex(pattern, "."); i >= 0 {
		pattern = pattern[i+1:]
	}
	pattern = strings.TrimSpace(pattern)
	if pattern == "" || strings.ContainsAny(pattern, "*?/\\") {
		return ""
	}
	return pattern
}
