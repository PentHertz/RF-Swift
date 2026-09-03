/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
*
*  WSLg's display client, seen from Windows.
*
*  Every window a WSL 2 GUI tool opens is painted on the Windows desktop by
*  WSLg's RDP client, msrdc.exe (C:\Program Files\WSL), which wslhost.exe
*  starts with the first distribution and reconnects by itself. When that
*  client's graphics decoder trips over a surface it cannot create or decode,
*  it stops painting: from then on a new window gets its taskbar entry and
*  nothing else, while the tool runs normally inside the distribution - SDR++
*  "shows only an icon in the bar". The client records every such failure in
*  the Microsoft-Windows-TerminalServices-RDPClient/Operational event channel
*  (events 226, 1033 and 1404) and recovers only when its RDP connection is
*  re-established, which it does on its own minutes later, or within seconds
*  when the process is restarted: wslhost relaunches it and Weston's RAIL
*  shell re-creates every open window for the new client.
*
*  WSLgDisplayStatus reads the process list and that channel and says whether
*  the client is in the failed state; ResetWSLgDisplay performs the restart
*  and waits for the new client to connect. Both only mean something on
*  Windows and report ErrWSLgDisplayNotWindows elsewhere; the parsing and the
*  decision are plain functions so they are unit-tested everywhere.
 */

package rfutils

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	// wslgClientExe is WSLg's RDP client. The Windows App / Azure Virtual
	// Desktop client carries the same name; wslgClientPathHint tells them
	// apart by where the executable lives.
	wslgClientExe      = "msrdc.exe"
	wslgClientPathHint = `\wsl\`
	// wslgClientEventLog is where the client logs its connections (1025) and
	// its graphics failures (226, 1033 with an RdpGfx* component, 1404).
	wslgClientEventLog = "Microsoft-Windows-TerminalServices-RDPClient/Operational"
	// wslgClientConnectedEvent is logged each time the client (re)connects to
	// WSLg's compositor; a graphics error after the last one means the client
	// is still in its failed state.
	wslgClientConnectedEvent = 1025
)

// ErrWSLgDisplayNotWindows is returned by the WSLg display helpers elsewhere.
var ErrWSLgDisplayNotWindows = errors.New("WSLg's display client only exists on Windows")

// ErrWSLgDisplayNotRunning is returned by ResetWSLgDisplay when there is no
// client to restart.
var ErrWSLgDisplayNotRunning = errors.New("WSLg's display client (msrdc.exe) is not running: it starts with the first WSL 2 GUI window, so there is nothing to reset")

// WSLgDisplayState is what the display client is doing.
type WSLgDisplayState struct {
	ClientRunning    bool      `json:"clientRunning"`
	ClientPID        int       `json:"clientPid,omitempty"`
	LastConnected    time.Time `json:"lastConnected,omitempty"`    // last connection of this client process to WSLg's compositor
	LastGfxError     time.Time `json:"lastGfxError,omitempty"`     // last graphics failure it logged
	LastGfxErrorText string    `json:"lastGfxErrorText,omitempty"` // that failure, in the client's words
	// Degraded: the client runs but logged a graphics failure after its last
	// connection, so new windows show only a taskbar icon until it reconnects.
	Degraded bool `json:"degraded"`
}

// Summary is the one-line description for status sheets and the doctor.
func (s WSLgDisplayState) Summary() string {
	switch {
	case s.Degraded:
		return fmt.Sprintf("stuck since %s: RDP graphics errors (%s), windows show only a taskbar icon", s.LastGfxError.Local().Format("15:04:05"), s.LastGfxErrorText)
	case !s.ClientRunning:
		return "not running (starts with the first GUI window)"
	case !s.LastConnected.IsZero():
		return fmt.Sprintf("connected since %s (msrdc.exe pid %d)", s.LastConnected.Local().Format("15:04:05"), s.ClientPID)
	default:
		return fmt.Sprintf("running (msrdc.exe pid %d)", s.ClientPID)
	}
}

// wslgClientEvent is one record of the client's event channel.
type wslgClientEvent struct {
	ID        int
	Time      time.Time
	ProcessID int
	Component string   // Data Name='Name' of a 1033 event (RdpGfxProtocolClientDecoder, slint, ...)
	Data      []string // the other data fields, in order
}

// gfxError reports whether the event is a graphics failure, as opposed to
// the connection bookkeeping the client logs with the same ids (1033 is also
// used for "SL::OnDisconnected").
func (e wslgClientEvent) gfxError() bool {
	switch e.ID {
	case 226, 1404:
		return true
	case 1033:
		c := strings.ToLower(e.Component)
		return strings.HasPrefix(c, "rdpgfx") || strings.HasPrefix(c, "offscreensurface")
	}
	return false
}

// describe reduces an event to the words a user can search for.
func (e wslgClientEvent) describe() string {
	parts := []string{}
	if e.Component != "" {
		parts = append(parts, e.Component)
	}
	for _, d := range e.Data {
		if d = strings.TrimSpace(d); d != "" {
			parts = append(parts, d)
		}
	}
	if len(parts) == 0 {
		return fmt.Sprintf("event %d", e.ID)
	}
	return strings.Join(parts, " ")
}

// wevtEvents is the shape of `wevtutil qe ... /f:xml` output: a sequence of
// <Event> elements (no document root), wrapped by the parser.
type wevtEvents struct {
	Events []struct {
		System struct {
			EventID     int `xml:"EventID"`
			TimeCreated struct {
				SystemTime string `xml:"SystemTime,attr"`
			} `xml:"TimeCreated"`
			Execution struct {
				ProcessID int `xml:"ProcessID,attr"`
			} `xml:"Execution"`
		} `xml:"System"`
		EventData struct {
			Data []struct {
				Name  string `xml:"Name,attr"`
				Value string `xml:",chardata"`
			} `xml:"Data"`
		} `xml:"EventData"`
	} `xml:"Event"`
}

// parseWSLgClientEvents decodes wevtutil's XML output. Malformed input
// yields no events rather than an error: the caller then simply knows
// nothing about the client, which is never treated as a failure.
func parseWSLgClientEvents(text string) []wslgClientEvent {
	text = strings.TrimSpace(strings.TrimPrefix(text, "\ufeff"))
	if text == "" {
		return nil
	}
	var doc wevtEvents
	if err := xml.Unmarshal([]byte("<Events>"+text+"</Events>"), &doc); err != nil {
		return nil
	}
	out := make([]wslgClientEvent, 0, len(doc.Events))
	for _, raw := range doc.Events {
		e := wslgClientEvent{ID: raw.System.EventID, ProcessID: raw.System.Execution.ProcessID}
		if t, err := time.Parse(time.RFC3339Nano, raw.System.TimeCreated.SystemTime); err == nil {
			e.Time = t
		}
		for _, d := range raw.EventData.Data {
			if d.Name == "Name" && e.Component == "" {
				e.Component = strings.TrimSpace(d.Value)
				continue
			}
			e.Data = append(e.Data, d.Value)
		}
		out = append(out, e)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// wslgDisplayStateFrom is the decision: with the client's process and its
// recent events, is it painting windows or not?
func wslgDisplayStateFrom(running bool, pid int, events []wslgClientEvent) WSLgDisplayState {
	st := WSLgDisplayState{ClientRunning: running, ClientPID: pid}
	if !running {
		return st
	}
	for _, e := range events {
		if pid != 0 && e.ProcessID != 0 && e.ProcessID != pid {
			continue // another Remote Desktop client, or a previous WSLg one
		}
		switch {
		case e.ID == wslgClientConnectedEvent:
			if e.Time.After(st.LastConnected) {
				st.LastConnected = e.Time
			}
		case e.gfxError():
			if e.Time.After(st.LastGfxError) {
				st.LastGfxError = e.Time
				st.LastGfxErrorText = e.describe()
			}
		}
	}
	st.Degraded = !st.LastGfxError.IsZero() && st.LastGfxError.After(st.LastConnected)
	return st
}

// wevtutilPath locates wevtutil.exe: PATH, then System32 (a GUI started with
// a minimal PATH still finds it).
func wevtutilPath() (string, error) {
	if p, err := exec.LookPath("wevtutil.exe"); err == nil {
		return p, nil
	}
	if root := os.Getenv("SystemRoot"); root != "" {
		p := filepath.Join(root, "System32", "wevtutil.exe")
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, nil
		}
	}
	return "", errors.New("wevtutil.exe not found")
}

// queryWSLgClientEvents reads the newest count events matching the XPath
// query from the client's channel, for one client process when pid is set.
func queryWSLgClientEvents(query string, pid, count int) ([]wslgClientEvent, error) {
	exe, err := wevtutilPath()
	if err != nil {
		return nil, err
	}
	if pid != 0 {
		query = fmt.Sprintf("*[System[%s and Execution[@ProcessID=%d]]]", query, pid)
	} else {
		query = fmt.Sprintf("*[System[%s]]", query)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, "qe", wslgClientEventLog, "/q:"+query, fmt.Sprintf("/c:%d", count), "/rd:true", "/f:xml")
	hideConsoleWindow(cmd)
	raw, err := cmd.Output()
	if err != nil {
		detail := ""
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			detail = firstLine(strings.TrimSpace(DecodeConsoleOutput(exit.Stderr)))
		}
		if detail == "" {
			detail = err.Error()
		}
		return nil, fmt.Errorf("reading the %s event log: %s", wslgClientEventLog, detail)
	}
	return parseWSLgClientEvents(DecodeConsoleOutput(raw)), nil
}

// wslgClientEvents gathers what the decision needs: the client's last
// connection and its most recent graphics failures.
func wslgClientEvents(pid int) ([]wslgClientEvent, error) {
	connected, err := queryWSLgClientEvents(fmt.Sprintf("(EventID=%d)", wslgClientConnectedEvent), pid, 1)
	if err != nil {
		return nil, err
	}
	failures, err := queryWSLgClientEvents("(EventID=226 or EventID=1033 or EventID=1404)", pid, 24)
	if err != nil {
		return nil, err
	}
	return append(connected, failures...), nil
}

// WSLgDisplayStatus reports whether WSLg's display client is painting windows.
//
//	out(1): WSLgDisplayState
//	out(2): error when this is not Windows, or the process list or the event
//	        log could not be read (the state then only says whether it runs)
func WSLgDisplayStatus() (WSLgDisplayState, error) {
	if runtime.GOOS != "windows" {
		return WSLgDisplayState{}, ErrWSLgDisplayNotWindows
	}
	pid, err := wslgClientPID()
	if err != nil {
		return WSLgDisplayState{}, err
	}
	if pid == 0 {
		return WSLgDisplayState{}, nil
	}
	events, err := wslgClientEvents(pid)
	if err != nil {
		return WSLgDisplayState{ClientRunning: true, ClientPID: pid}, err
	}
	return wslgDisplayStateFrom(true, pid, events), nil
}

// ResetWSLgDisplay restarts WSLg's display client and waits (up to wait) for
// the new one to connect: wslhost.exe relaunches msrdc.exe within seconds
// and WSLg re-creates every open window for it. This is the fix for GUI
// tools that show only a taskbar icon, without a `wsl --shutdown`.
//
//	in(1): time.Duration wait how long to wait for the new client
//	out(1): WSLgDisplayState of the new client
//	out(2): error when there is no client, it could not be stopped, or the
//	        new one did not connect in time
func ResetWSLgDisplay(wait time.Duration) (WSLgDisplayState, error) {
	if runtime.GOOS != "windows" {
		return WSLgDisplayState{}, ErrWSLgDisplayNotWindows
	}
	pid, err := wslgClientPID()
	if err != nil {
		return WSLgDisplayState{}, err
	}
	if pid == 0 {
		return WSLgDisplayState{}, ErrWSLgDisplayNotRunning
	}
	if err := terminateProcess(pid); err != nil {
		return WSLgDisplayState{ClientRunning: true, ClientPID: pid}, fmt.Errorf("could not stop msrdc.exe (pid %d): %w", pid, err)
	}
	if wait <= 0 {
		wait = 30 * time.Second
	}
	deadline := time.Now().Add(wait)
	var last WSLgDisplayState
	for {
		time.Sleep(time.Second)
		st, err := WSLgDisplayStatus()
		if err == nil && st.ClientRunning && st.ClientPID != pid && !st.LastConnected.IsZero() && !st.Degraded {
			return st, nil
		}
		if err == nil {
			last = st
		}
		if time.Now().After(deadline) {
			if err != nil {
				return last, err
			}
			return last, fmt.Errorf("WSLg's display client did not reconnect within %s; 'wsl --shutdown' restarts WSLg entirely", wait.Round(time.Second))
		}
	}
}
