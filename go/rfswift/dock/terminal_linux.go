package dock

import (
	"syscall"
	"os"
	"golang.org/x/crypto/ssh/terminal"
)

func getTerminalSize(fd int) (int, int, error) {
	return terminal.GetSize(fd)
}

func syscallsigwin() (os.Signal) {
	return syscall.SIGWINCH
}