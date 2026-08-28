/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
*
*  Package ptyx starts a command on a pseudo-terminal on every platform RF
*  Swift supports: a Unix PTY through creack/pty, a Windows ConPTY through
*  UserExistsError/conpty. The remote agent (interactive shells served to the
*  Workbench) and the Workbench's own local terminals use it, so a Windows lab
*  machine can host the agent and a Windows Workbench can run coding-agent
*  terminals without a Unix PTY.
 */

package ptyx

import (
	"errors"
	"io"
	"os/exec"
)

// ErrUnsupported is returned where no pseudo-terminal implementation exists
// (for example Windows builds older than 10 1809, which lack ConPTY).
var ErrUnsupported = errors.New("pseudo-terminals are not supported on this platform")

// Terminal is a running command attached to a pseudo-terminal. Reads return
// the terminal output stream, writes are keyboard input. Close terminates the
// process and releases the PTY; afterwards Read fails, which is how readers
// learn the session ended.
type Terminal interface {
	io.ReadWriteCloser
	Resize(cols, rows int) error
	Wait() error
	Pid() int
}

// Start runs cmd on a new pseudo-terminal of the given size. cmd.Path,
// cmd.Args, cmd.Env and cmd.Dir are honoured on every platform; the process
// is started by this call (do not call cmd.Start yourself).
//
//	in(1): *exec.Cmd cmd the command to run
//	in(2): int cols terminal width (defaults to 80 when < 2)
//	in(3): int rows terminal height (defaults to 24 when < 2)
//	out(1): Terminal
//	out(2): error ErrUnsupported when the platform has no PTY
func Start(cmd *exec.Cmd, cols, rows int) (Terminal, error) {
	if cmd == nil {
		return nil, errors.New("command is required")
	}
	if cols < 2 {
		cols = 80
	}
	if rows < 2 {
		rows = 24
	}
	return start(cmd, cols, rows)
}
