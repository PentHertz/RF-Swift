/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
*
*  Nix engine on Windows: WSLg's display client, the piece between a GUI
*  tool's window inside the distribution and the Windows desktop.
*
*  When that client (msrdc.exe) has tripped over an RDP graphics error it
*  keeps running but paints nothing any more: a tool started afterwards shows
*  only a taskbar icon, while its log inside the distribution looks perfectly
*  healthy (SDR++ loads its modules, opens its audio stream, saves its
*  config). Restarting the client fixes it within seconds - WSL relaunches it
*  and re-creates the open windows - so the delegated GUI-capable operations
*  (run, exec, shell, tool runs, the Workbench launches) check the Windows
*  event log first and do that restart by themselves; `rfswift nix wsl
*  display-reset` and the Workbench's engine doctor do it on demand. See
*  rfutils/wslgdisplay.go for the detection.
 */

package nix

import (
	"fmt"
	"os"
	"strings"
	"time"

	common "penthertz/rfswift/common"
	rfutils "penthertz/rfswift/rfutils"
)

// WSLDisplayAutoResetVar, set to 0/false/no/off, keeps RF Swift from
// restarting a stuck WSLg display client before a GUI-capable command.
const WSLDisplayAutoResetVar = "RFSWIFT_WSLG_AUTORESET"

// wslDisplayResetWait is how long a reset waits for the new client to connect.
const wslDisplayResetWait = 30 * time.Second

// WSLDisplayState reports what WSLg's display client is doing (Windows).
func WSLDisplayState() (rfutils.WSLgDisplayState, error) {
	if !useWSL() {
		return rfutils.WSLgDisplayState{}, errNotWindows
	}
	return rfutils.WSLgDisplayStatus()
}

// WSLDisplayReset restarts WSLg's display client and waits for it to
// reconnect; open windows come back within seconds (Windows).
func WSLDisplayReset() (rfutils.WSLgDisplayState, error) {
	if !useWSL() {
		return rfutils.WSLgDisplayState{}, errNotWindows
	}
	return rfutils.ResetWSLgDisplay(wslDisplayResetWait)
}

func wslDisplayAutoResetDisabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(WSLDisplayAutoResetVar))) {
	case "0", "false", "no", "off":
		return true
	}
	return false
}

// WSLDisplayPreflight, before a command that may open windows, restarts a
// WSLg display client that has stopped painting them. It never fails the
// command: when the state cannot be read, or the restart does not work, the
// command runs as before and the user is told what to do.
func WSLDisplayPreflight() {
	if !useWSL() || wslDisplayAutoResetDisabled() {
		return
	}
	st, err := rfutils.WSLgDisplayStatus()
	if err != nil || !st.Degraded {
		return
	}
	common.PrintWarningMessage(fmt.Sprintf("WSLg's display client stopped painting windows at %s (%s): GUI tools would show only a taskbar icon. Restarting it (%s=0 disables this)...",
		st.LastGfxError.Local().Format("15:04:05"), st.LastGfxErrorText, WSLDisplayAutoResetVar))
	if _, err := rfutils.ResetWSLgDisplay(wslDisplayResetWait); err != nil {
		common.PrintWarningMessage(fmt.Sprintf("WSLg's display client could not be restarted (%v). If a window stays invisible: rfswift nix wsl display-reset, or wsl --shutdown.", err))
		return
	}
	common.PrintInfoMessage("WSLg's display client is back; windows are painted again.")
}
