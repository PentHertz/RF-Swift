/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
 */

package workbench

import (
	"sync"
	"time"
	"unicode/utf8"
)

// Terminal output is sent to the frontend as Wails events, each handled on
// the WebView's UI thread. Forwarding every read gives one event per line for
// line-oriented tools (rtl_433, logs), so a few busy consoles flood the UI.
// outputCoalescer batches a session's output: data arriving after a quiet
// period goes out at once, so typing and prompts keep their latency, and
// anything within outputFlushInterval of the previous event is merged into
// the next one (or sent early once outputMaxBatch bytes are waiting).
const (
	outputFlushInterval = 16 * time.Millisecond // matches the frontend's write batching
	outputMaxBatch      = 64 * 1024
)

type outputCoalescer struct {
	mu    sync.Mutex
	emit  func(string)
	buf   []byte
	timer *time.Timer
	last  time.Time
}

func newOutputCoalescer(emit func(string)) *outputCoalescer {
	return &outputCoalescer{emit: emit}
}

// Write queues p. It never blocks on the frontend.
func (c *outputCoalescer) Write(p []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.buf = append(c.buf, p...)
	switch {
	case len(c.buf) >= outputMaxBatch:
		c.flushLocked(false)
	case c.timer != nil:
		// merged into the pending flush
	case time.Since(c.last) >= outputFlushInterval:
		c.flushLocked(false)
	default:
		c.timer = time.AfterFunc(outputFlushInterval-time.Since(c.last), c.flush)
	}
}

func (c *outputCoalescer) flush() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.flushLocked(false)
}

// Close sends whatever is left, including an incomplete trailing character.
func (c *outputCoalescer) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.flushLocked(true)
}

// flushLocked emits the buffered output. A UTF-8 sequence split across reads
// is held back until its remaining bytes arrive: sent as is, JSON encoding
// would turn each half into U+FFFD. Emitting under the lock keeps order.
func (c *outputCoalescer) flushLocked(all bool) {
	if c.timer != nil {
		c.timer.Stop()
		c.timer = nil
	}
	n := len(c.buf)
	if !all {
		n = completeUTF8Prefix(c.buf)
	}
	if n == 0 {
		return
	}
	data := string(c.buf[:n])
	c.buf = append(c.buf[:0], c.buf[n:]...)
	c.last = time.Now()
	c.emit(data)
}

// completeUTF8Prefix returns the length of b without a trailing incomplete
// UTF-8 sequence.
func completeUTF8Prefix(b []byte) int {
	for i := len(b) - 1; i >= 0 && i >= len(b)-utf8.UTFMax; i-- {
		if utf8.RuneStart(b[i]) {
			if !utf8.FullRune(b[i:]) {
				return i
			}
			break
		}
	}
	return len(b)
}
