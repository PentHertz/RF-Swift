//go:build windows

package ptyx

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/UserExistsError/conpty"
)

type windowsTerminal struct {
	pty      *conpty.ConPty
	waitOnce sync.Once
	waitErr  error
	closed   chan struct{}
	closeOne sync.Once
}

// start runs the command under a Windows pseudo console (ConPTY, Windows 10
// 1809+). ConPTY takes a command line rather than an argument vector, so the
// arguments are quoted with the same rules Go uses for exec on Windows.
func start(cmd *exec.Cmd, cols, rows int) (Terminal, error) {
	if !conpty.IsConPtyAvailable() {
		return nil, ErrUnsupported
	}
	path := cmd.Path
	if !filepath.IsAbs(path) {
		if resolved, err := exec.LookPath(path); err == nil {
			path = resolved
		}
	}
	args := cmd.Args
	if len(args) == 0 {
		args = []string{path}
	}
	parts := make([]string, 0, len(args))
	parts = append(parts, syscall.EscapeArg(path))
	for _, a := range args[1:] {
		parts = append(parts, syscall.EscapeArg(a))
	}
	env := cmd.Env
	if env == nil {
		env = os.Environ()
	}
	p, err := conpty.Start(strings.Join(parts, " "), conpty.ConPtyDimensions(cols, rows), conpty.ConPtyWorkDir(cmd.Dir), conpty.ConPtyEnv(env))
	if err != nil {
		return nil, err
	}
	t := &windowsTerminal{pty: p, closed: make(chan struct{})}
	// A ConPTY output pipe never reports EOF by itself: readers would block
	// forever after the shell exits. Watch the process and release the console
	// shortly after it ends (leaving time to drain the last output), which
	// turns pending reads into errors exactly like a hung-up Unix PTY.
	go func() {
		_ = t.Wait()
		select {
		case <-t.closed:
		case <-time.After(300 * time.Millisecond):
			_ = t.Close()
		}
	}()
	return t, nil
}

func (t *windowsTerminal) Read(p []byte) (int, error)  { return t.pty.Read(p) }
func (t *windowsTerminal) Write(p []byte) (int, error) { return t.pty.Write(p) }
func (t *windowsTerminal) Resize(cols, rows int) error { return t.pty.Resize(cols, rows) }
func (t *windowsTerminal) Pid() int                    { return t.pty.Pid() }

// Wait blocks until the process exits; a non-zero exit code is an error.
func (t *windowsTerminal) Wait() error {
	t.waitOnce.Do(func() {
		code, err := t.pty.Wait(context.Background())
		if err != nil {
			t.waitErr = err
		} else if code != 0 {
			t.waitErr = fmt.Errorf("exit status %d", code)
		}
	})
	return t.waitErr
}

// Close destroys the pseudo console, which terminates the attached process.
func (t *windowsTerminal) Close() error {
	var err error
	t.closeOne.Do(func() {
		close(t.closed)
		err = t.pty.Close()
	})
	return err
}
