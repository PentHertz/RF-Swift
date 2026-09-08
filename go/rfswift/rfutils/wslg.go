/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
*
*  WSLg detection from the Windows side. WSLg (Windows 11, Windows 10 21H2+
*  with a current WSL) runs an X11 server and a PulseAudio server for every
*  WSL 2 distribution and exposes them under /mnt/wslg; containers on Docker
*  Desktop / Podman machine reach the same tree, which is how RF Swift gives
*  them a display and sound on Windows without installing anything.
 */

package rfutils

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// WSLgStatus reports what WSLg exposes inside the default WSL 2 distribution.
type WSLgStatus struct {
	Audio bool // /mnt/wslg/PulseServer socket present
	X11   bool // /mnt/wslg/.X11-unix present
}

// CheckWSLg asks the default WSL 2 distribution whether the WSLg sockets
// exist (starts it if needed).
//
//	out(1): WSLgStatus
//	out(2): error when wsl.exe is missing or does not answer
func CheckWSLg() (WSLgStatus, error) {
	var status WSLgStatus
	wsl, err := exec.LookPath("wsl.exe")
	if err != nil {
		return status, fmt.Errorf("wsl.exe not found: WSL 2 with WSLg is required for display and audio on Windows (wsl --install)")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, wsl, "-e", "sh", "-c", "test -S /mnt/wslg/PulseServer && echo wslg-audio; test -d /mnt/wslg/.X11-unix && echo wslg-x11; true")
	hideConsoleWindow(cmd)
	raw, err := cmd.CombinedOutput()
	text := DecodeConsoleOutput(raw)
	if err != nil && strings.TrimSpace(text) == "" {
		return status, fmt.Errorf("could not query WSL: %w", err)
	}
	status.Audio = strings.Contains(text, "wslg-audio")
	status.X11 = strings.Contains(text, "wslg-x11")
	return status, nil
}
