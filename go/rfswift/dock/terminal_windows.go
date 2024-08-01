package dock

import (
	"github.com/Azure/go-ansiterm/winterm"
)

func getTerminalSize(fd int) (int, int, error) {
	var info winterm.CONSOLE_SCREEN_BUFFER_INFO
	handle := winterm.GetStdHandle(winterm.STD_OUTPUT_HANDLE)
	err := winterm.GetConsoleScreenBufferInfo(handle, &info)
	if err != nil {
		return 0, 0, err
	}
	width := int(info.Window.Right - info.Window.Left + 1)
	height := int(info.Window.Bottom - info.Window.Top + 1)
	return width, height, nil
}