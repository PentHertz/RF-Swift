package workbench

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"
)

type emitRecorder struct {
	mu     sync.Mutex
	events []string
}

func (r *emitRecorder) emit(s string) {
	r.mu.Lock()
	r.events = append(r.events, s)
	r.mu.Unlock()
}

func (r *emitRecorder) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.events...)
}

func TestOutputCoalescerSendsIdleOutputAtOnce(t *testing.T) {
	var r emitRecorder
	c := newOutputCoalescer(r.emit)
	c.Write([]byte("$ "))
	if got := r.snapshot(); len(got) != 1 || got[0] != "$ " {
		t.Fatalf("prompt after a quiet period must be sent immediately, got %q", got)
	}
}

func TestOutputCoalescerKeepsOrderAndWholeCharacters(t *testing.T) {
	input := []byte(strings.Repeat("⚡ root@rytr ~ gnuradio-companion ✘ é\r\n", 200))
	for chunk := 1; chunk <= 7; chunk++ {
		var r emitRecorder
		c := newOutputCoalescer(r.emit)
		for i := 0; i < len(input); i += chunk {
			c.Write(input[i:min(i+chunk, len(input))])
		}
		c.Close()
		events := r.snapshot()
		if joined := strings.Join(events, ""); joined != string(input) {
			t.Fatalf("chunk %d: output changed or reordered", chunk)
		}
		for _, e := range events {
			if !utf8.ValidString(e) {
				t.Fatalf("chunk %d: event splits a UTF-8 character: %q", chunk, e)
			}
		}
	}
}

func TestOutputCoalescerMergesBursts(t *testing.T) {
	var r emitRecorder
	c := newOutputCoalescer(r.emit)
	const writes = 1000
	for i := 0; i < writes; i++ {
		c.Write([]byte(fmt.Sprintf("line %d\r\n", i)))
	}
	time.Sleep(3 * outputFlushInterval)
	events := r.snapshot()
	if len(events) > writes/10 {
		t.Fatalf("a burst of %d writes produced %d events", writes, len(events))
	}
	if !strings.HasSuffix(strings.Join(events, ""), fmt.Sprintf("line %d\r\n", writes-1)) {
		t.Fatal("the end of the burst was not flushed by the timer")
	}
}

// TestOutputCoalescerFloodingConsoles reproduces several consoles running a
// line-oriented tool at full speed: each line is one read, which used to be
// one Wails event handled on the UI thread.
func TestOutputCoalescerFloodingConsoles(t *testing.T) {
	const consoles, lines = 10, 20000
	line := strings.Repeat("x", 78) + "\r\n"
	var wg sync.WaitGroup
	reads := make([]int, consoles)
	recs := make([]emitRecorder, consoles)
	start := time.Now()
	for k := 0; k < consoles; k++ {
		pr, pw := io.Pipe()
		go func() {
			for i := 0; i < lines; i++ {
				_, _ = pw.Write([]byte(line))
			}
			_ = pw.Close()
		}()
		wg.Add(1)
		go func(k int) {
			defer wg.Done()
			out := newOutputCoalescer(recs[k].emit)
			buf := make([]byte, 32*1024) // same loop as streamTerminal
			for {
				n, err := pr.Read(buf)
				if n > 0 {
					reads[k]++
					out.Write(buf[:n])
				}
				if err != nil {
					out.Close()
					return
				}
			}
		}(k)
	}
	wg.Wait()
	elapsed := time.Since(start)
	before, after := 0, 0
	for k := 0; k < consoles; k++ {
		events := recs[k].snapshot()
		if got := len(strings.Join(events, "")); got != lines*len(line) {
			t.Fatalf("console %d: %d bytes delivered, want %d", k, got, lines*len(line))
		}
		before += reads[k]
		after += len(events)
	}
	t.Logf("%d consoles x %d lines in %v: %d events before (one per read), %d after (%.0fx fewer)",
		consoles, lines, elapsed.Round(time.Millisecond), before, after, float64(before)/float64(after))
	if after*20 > before {
		t.Fatalf("coalescing too weak: %d events for %d reads", after, before)
	}
}
