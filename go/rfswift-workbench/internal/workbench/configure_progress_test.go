package workbench

import (
	"strings"
	"testing"

	rfdock "penthertz/rfswift/dock"
)

// The root helper prints its steps among its messages; an error shown to the
// user keeps the messages and drops the step lines.
func TestStripANSILinesDropsConfigStages(t *testing.T) {
	out := strings.Join([]string{
		rfdock.ConfigStageLine("Stopping the container", 15),
		"[i] Container engine: Docker (explicit)",
		"\x1b[31mfailed to restart Docker service: exit status 1\x1b[0m",
		rfdock.ConfigStageLine("Restarting the Docker service", 55),
	}, "\n")
	if got := stripANSILines(out); got != "failed to restart Docker service: exit status 1" {
		t.Fatalf("stripANSILines = %q", got)
	}
}
