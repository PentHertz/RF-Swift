package hostsetup

import (
	"fmt"
	"reflect"
	"testing"
)

func TestLineWriterSplitsLinesAsTheyArrive(t *testing.T) {
	var lines []string
	w := &lineWriter{onLine: func(line string) { lines = append(lines, line) }}
	for _, chunk := range []string{"rfswift-config-stage:15:Stop", "ping\r\n[i] done\nlast without newline"} {
		fmt.Fprint(w, chunk)
	}
	if want := []string{"rfswift-config-stage:15:Stopping", "[i] done"}; !reflect.DeepEqual(lines, want) {
		t.Fatalf("lines before flush = %q, want %q", lines, want)
	}
	w.flush()
	if lines[len(lines)-1] != "last without newline" {
		t.Fatalf("flush lost the last line: %q", lines)
	}
	if w.out.String() != "rfswift-config-stage:15:Stopping\r\n[i] done\nlast without newline" {
		t.Fatalf("output kept = %q", w.out.String())
	}
}
