package dock

import (
	"golang.org/x/crypto/ssh/terminal"
)

func getTerminalSize(fd int) (int, int, error) {
	return terminal.GetSize(fd)
}
