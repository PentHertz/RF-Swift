package dock

import (
	"golang.org/x/crypto/ssh/terminal"
	"os"
	"syscall"
)

func getTerminalSize(fd int) (int, int, error) {
	return terminal.GetSize(fd)
}

func syscallsigwin() os.Signal {
	return syscall.SIGWINCH
}
