//go:build !windows

package ptyx

import (
	"errors"
	"os"
	"os/exec"
	"sync"

	"github.com/creack/pty"
)

type unixTerminal struct {
	file     *os.File
	cmd      *exec.Cmd
	waitOnce sync.Once
	waitErr  error
}

func start(cmd *exec.Cmd, cols, rows int) (Terminal, error) {
	f, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
	if err != nil {
		if errors.Is(err, pty.ErrUnsupported) {
			return nil, ErrUnsupported
		}
		return nil, err
	}
	return &unixTerminal{file: f, cmd: cmd}, nil
}

func (t *unixTerminal) Read(p []byte) (int, error)  { return t.file.Read(p) }
func (t *unixTerminal) Write(p []byte) (int, error) { return t.file.Write(p) }
func (t *unixTerminal) Resize(cols, rows int) error {
	return pty.Setsize(t.file, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
}
func (t *unixTerminal) Pid() int {
	if t.cmd.Process == nil {
		return 0
	}
	return t.cmd.Process.Pid
}

// Wait reaps the process once; safe to call after Close.
func (t *unixTerminal) Wait() error {
	t.waitOnce.Do(func() { t.waitErr = t.cmd.Wait() })
	return t.waitErr
}

// Close hangs up the PTY and makes sure the process is gone, then reaps it.
func (t *unixTerminal) Close() error {
	err := t.file.Close()
	if t.cmd.Process != nil {
		_ = t.cmd.Process.Kill()
	}
	_ = t.Wait()
	return err
}
