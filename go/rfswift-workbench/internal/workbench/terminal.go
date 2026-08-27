package workbench

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
	"github.com/moby/moby/client"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
	rfdock "penthertz/rfswift/dock"
	rfnix "penthertz/rfswift/nix"
)

// xterm.js answers OSC 10/11/12 foreground/background/cursor colour queries
// through onData. Some remote shells race while consuming that reply and leave
// its printable `11;rgb:...` tail in the command line. These are terminal-
// generated replies, not operator input, so do not forward them over RPC.
var terminalColorReply = regexp.MustCompile(`\x1b\](?:10|11|12);(?:rgb:)?[0-9A-Fa-f/]+(?:\x07|\x1b\\)`)

type terminalSession struct {
	id, mission, execID string
	shell               string
	cols, rows          int
	cli                 *client.Client
	conn                io.ReadWriteCloser
	reader              *bufio.Reader
	started             time.Time
	record              *os.File
	recordPath          string
	recordMu            sync.Mutex
	inputMu             sync.Mutex
	closeOnce           sync.Once
	localCmd            *exec.Cmd
	localPTY            *os.File
	remote              *RemoteEngine
	agent               bool
}

type TerminalStartResult struct {
	ID            string `json:"id"`
	RecordingPath string `json:"recordingPath"`
}

func (a *App) StartTerminal(missionID, shell string, record bool, recordingDir string, cols, rows int) (TerminalStartResult, error) {
	if err := a.requireMission(missionID); err != nil {
		return TerminalStartResult{}, err
	}
	if remoteEngine, ok := a.eng.(*RemoteEngine); ok {
		return a.startRemoteTerminal(remoteEngine, missionID, shell, record, recordingDir, cols, rows)
	}
	if isNixEnv(missionID) {
		return a.startNixTerminal(missionID, shell, record, recordingDir, cols, rows)
	}
	if cols < 2 {
		cols = 80
	}
	if rows < 2 {
		rows = 24
	}
	if shell == "" {
		shell = "/bin/zsh"
	}
	local, ok := a.eng.(*LocalEngine)
	if !ok {
		return TerminalStartResult{}, errors.New("interactive terminals need a local engine")
	}
	cli, engType, err := local.clientFor(missionID)
	if err != nil {
		return TerminalStartResult{}, err
	}
	ctx := context.Background()
	if _, err = cli.ContainerStart(ctx, missionID, client.ContainerStartOptions{}); err != nil {
		cli.Close()
		return TerminalStartResult{}, err
	}
	cmd := []string{shell}
	if shell == "/bin/zsh" {
		cmd = []string{"/bin/sh", "-c", "if [ -x /bin/zsh ]; then exec /bin/zsh -il; elif [ -x /bin/bash ]; then exec /bin/bash -il; else exec /bin/sh -i; fi"}
	} else if shell == "/bin/bash" {
		cmd = []string{shell, "-il"}
	} else if shell == "/bin/sh" {
		cmd = []string{shell, "-i"}
	}
	created, err := cli.ExecCreate(ctx, missionID, client.ExecCreateOptions{
		AttachStdin: true, AttachStdout: true, AttachStderr: true, TTY: true,
		Cmd: cmd, Env: []string{"TERM=xterm-256color", "COLORTERM=truecolor", "LANG=C.UTF-8", "LC_ALL=C.UTF-8"},
	})
	if err != nil {
		cli.Close()
		return TerminalStartResult{}, err
	}
	attached, err := cli.ExecAttach(ctx, created.ID, client.ExecAttachOptions{TTY: true})
	if err != nil {
		cli.Close()
		return TerminalStartResult{}, err
	}
	_, _ = cli.ExecResize(ctx, created.ID, client.ExecResizeOptions{Width: uint(cols), Height: uint(rows)})
	if engType != rfdock.EnginePodman {
		if _, err = cli.ExecStart(ctx, created.ID, client.ExecStartOptions{TTY: true}); err != nil {
			attached.Close()
			cli.Close()
			return TerminalStartResult{}, err
		}
	}
	s := &terminalSession{id: created.ID, mission: missionID, execID: created.ID, shell: shell, cols: cols, rows: rows, cli: cli, conn: attached.Conn, reader: attached.Reader, started: time.Now()}
	if record {
		dir := strings.TrimSpace(recordingDir)
		if dir == "" {
			dir = filepath.Join(a.store.missionDir(a.ws, missionID), "recordings")
		}
		dir, err = filepath.Abs(dir)
		if err != nil {
			attached.Close()
			cli.Close()
			return TerminalStartResult{}, fmt.Errorf("recording destination: %w", err)
		}
		if err := os.MkdirAll(dir, 0o700); err != nil {
			attached.Close()
			cli.Close()
			return TerminalStartResult{}, err
		}
		s.recordPath = terminalRecordingPath(dir, missionID, "container")
		s.record, err = os.OpenFile(s.recordPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			attached.Close()
			cli.Close()
			return TerminalStartResult{}, err
		}
		header := map[string]any{"version": 2, "width": cols, "height": rows, "timestamp": time.Now().Unix(), "env": map[string]string{"SHELL": shell, "TERM": "xterm-256color"}}
		b, _ := json.Marshal(header)
		_, _ = s.record.Write(append(b, '\n'))
	}
	a.termMu.Lock()
	a.terminals[s.id] = s
	a.termMu.Unlock()
	go a.streamTerminal(s)
	return TerminalStartResult{ID: s.id, RecordingPath: s.recordPath}, nil
}

func (a *App) startRemoteTerminal(engine *RemoteEngine, missionID, shell string, record bool, recordingDir string, cols, rows int) (TerminalStartResult, error) {
	if cols < 2 {
		cols = 80
	}
	if rows < 2 {
		rows = 24
	}
	if shell == "" {
		shell = "/bin/zsh"
	}
	var started struct {
		ID string `json:"id"`
	}
	if err := engine.call("terminal.start", map[string]any{"mission": missionID, "shell": shell, "cols": cols, "rows": rows}, &started); err != nil {
		return TerminalStartResult{}, err
	}
	s := &terminalSession{id: started.ID, mission: missionID, shell: shell, cols: cols, rows: rows, started: time.Now(), remote: engine}
	if record {
		dir := strings.TrimSpace(recordingDir)
		if dir == "" {
			dir = filepath.Join(a.store.missionDir(a.ws, missionID), "recordings")
		}
		var err error
		dir, err = filepath.Abs(dir)
		if err != nil {
			return TerminalStartResult{}, err
		}
		if err = os.MkdirAll(dir, 0700); err != nil {
			return TerminalStartResult{}, err
		}
		s.recordPath = terminalRecordingPath(dir, missionID, "remote")
		s.record, err = os.OpenFile(s.recordPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return TerminalStartResult{}, err
		}
		header, _ := json.Marshal(map[string]any{"version": 2, "width": cols, "height": rows, "timestamp": time.Now().Unix(), "env": map[string]string{"SHELL": shell, "TERM": "xterm-256color", "RFSWIFT_REMOTE": engine.Config.Endpoint}})
		_, _ = s.record.Write(append(header, '\n'))
	}
	a.termMu.Lock()
	a.terminals[s.id] = s
	a.termMu.Unlock()
	go a.pollRemoteTerminal(s)
	return TerminalStartResult{ID: s.id, RecordingPath: s.recordPath}, nil
}

func (a *App) pollRemoteTerminal(s *terminalSession) {
	defer func() {
		a.termMu.Lock()
		delete(a.terminals, s.id)
		a.termMu.Unlock()
		s.close()
		wruntime.EventsEmit(a.ctx, "rfswift:terminal:exit", map[string]any{"id": s.id, "recordingPath": s.recordPath})
	}()
	for {
		var out struct {
			Data   string `json:"data"`
			Closed bool   `json:"closed"`
		}
		err := s.remote.call("terminal.read", map[string]string{"id": s.id}, &out)
		if err != nil {
			wruntime.EventsEmit(a.ctx, "rfswift:terminal:error", map[string]any{"id": s.id, "error": err.Error()})
			return
		}
		if out.Data != "" {
			s.writeCast("o", out.Data)
			wruntime.EventsEmit(a.ctx, "rfswift:terminal:data", map[string]any{"id": s.id, "data": out.Data})
		}
		if out.Closed {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func (a *App) startNixTerminal(missionID, shell string, record bool, recordingDir string, cols, rows int) (TerminalStartResult, error) {
	if cols < 2 {
		cols = 80
	}
	if rows < 2 {
		rows = 24
	}
	cmd, err := rfnix.InteractiveCommand(missionID, shell)
	if err != nil {
		return TerminalStartResult{}, err
	}
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
	if err != nil {
		return TerminalStartResult{}, err
	}
	id := fmt.Sprintf("nix-%s-%x", missionID, time.Now().UnixNano())
	s := &terminalSession{id: id, mission: missionID, shell: shell, cols: cols, rows: rows, conn: ptmx, reader: bufio.NewReader(ptmx), started: time.Now(), localCmd: cmd, localPTY: ptmx}
	if record {
		dir := strings.TrimSpace(recordingDir)
		if dir == "" {
			dir = filepath.Join(a.store.missionDir(a.ws, missionID), "recordings")
		}
		dir, err = filepath.Abs(dir)
		if err != nil {
			s.close()
			return TerminalStartResult{}, fmt.Errorf("recording destination: %w", err)
		}
		if err := os.MkdirAll(dir, 0o700); err != nil {
			s.close()
			return TerminalStartResult{}, err
		}
		s.recordPath = terminalRecordingPath(dir, missionID, "nix")
		s.record, err = os.OpenFile(s.recordPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			s.close()
			return TerminalStartResult{}, err
		}
		header, _ := json.Marshal(map[string]any{"version": 2, "width": cols, "height": rows, "timestamp": time.Now().Unix(), "env": map[string]string{"SHELL": shell, "TERM": "xterm-256color", "RFSWIFT_NIX_ENV": missionID}})
		_, _ = s.record.Write(append(header, '\n'))
	}
	a.termMu.Lock()
	a.terminals[id] = s
	a.termMu.Unlock()
	go a.streamTerminal(s)
	return TerminalStartResult{ID: id, RecordingPath: s.recordPath}, nil
}

// StartTerminalRecording begins a new asciinema v2 segment without replacing
// or reconnecting the live terminal session.
func (a *App) StartTerminalRecording(id, recordingDir string, cols, rows int) (string, error) {
	s, err := a.terminal(id)
	if err != nil {
		return "", err
	}
	s.recordMu.Lock()
	defer s.recordMu.Unlock()
	if s.record != nil {
		return "", errors.New("terminal is already recording")
	}
	dir := strings.TrimSpace(recordingDir)
	if dir == "" {
		dir = filepath.Join(a.store.missionDir(a.ws, s.mission), "recordings")
	}
	dir, err = filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("recording destination: %w", err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	if cols < 2 {
		cols = s.cols
	}
	if rows < 2 {
		rows = s.rows
	}
	kind := "container"
	if s.localPTY != nil {
		kind = "nix"
	}
	path := terminalRecordingPath(dir, s.mission, kind)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", err
	}
	header, _ := json.Marshal(map[string]any{"version": 2, "width": cols, "height": rows, "timestamp": time.Now().Unix(), "env": map[string]string{"SHELL": s.shell, "TERM": "xterm-256color", "RFSWIFT_MISSION": s.mission}})
	if _, err := file.Write(append(header, '\n')); err != nil {
		_ = file.Close()
		return "", err
	}
	s.record = file
	s.recordPath = path
	s.started = time.Now()
	return path, nil
}

func terminalRecordingPath(dir, mission, kind string) string {
	safeMission := strings.NewReplacer("/", "-", "\\", "-", ":", "-", " ", "-").Replace(strings.TrimSpace(mission))
	if safeMission == "" {
		safeMission = "mission"
	}
	return filepath.Join(dir, "terminal-"+time.Now().Format("20060102-150405.000")+"-"+kind+"-"+safeMission+".cast")
}

// StopTerminalRecording closes the active evidence segment while leaving the
// terminal connected. The returned path remains available for notes/export.
func (a *App) StopTerminalRecording(id string) (string, error) {
	s, err := a.terminal(id)
	if err != nil {
		return "", err
	}
	s.recordMu.Lock()
	defer s.recordMu.Unlock()
	if s.record == nil {
		return s.recordPath, errors.New("terminal is not recording")
	}
	path := s.recordPath
	err = s.record.Close()
	s.record = nil
	return path, err
}

// StartAgentTerminal configures the selected client in a private mission
// workspace and starts its interactive TUI on the same PTY transport as the
// other Workbench terminals.
func (a *App) StartAgentTerminal(missionID, clientID string, cols, rows int) (AgentTerminalResult, error) {
	if err := a.requireMission(missionID); err != nil {
		return AgentTerminalResult{}, err
	}
	connected, command, args, err := a.agentLaunchSpec(missionID, clientID)
	if err != nil {
		return AgentTerminalResult{}, err
	}
	if cols < 2 {
		cols = 100
	}
	if rows < 2 {
		rows = 30
	}
	cmd := exec.Command(command, args...)
	cmd.Dir = connected.Workspace
	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "COLORTERM=truecolor")
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
	if err != nil {
		return AgentTerminalResult{}, err
	}
	id := fmt.Sprintf("agent-%s-%x", clientID, time.Now().UnixNano())
	s := &terminalSession{id: id, mission: missionID, conn: ptmx, reader: bufio.NewReader(ptmx), started: time.Now(), localCmd: cmd, localPTY: ptmx, agent: true}
	a.termMu.Lock()
	a.terminals[id] = s
	a.termMu.Unlock()
	go a.streamTerminal(s)
	return AgentTerminalResult{ID: id, Client: connected.Client, Workspace: connected.Workspace, ConfigPath: connected.ConfigPath, Verified: connected.Verified}, nil
}

func (a *App) streamTerminal(s *terminalSession) {
	buf := make([]byte, 32*1024)
	for {
		n, err := s.reader.Read(buf)
		if n > 0 {
			data := string(buf[:n])
			s.writeCast("o", data)
			wruntime.EventsEmit(a.ctx, "rfswift:terminal:data", map[string]any{"id": s.id, "data": data})
		}
		if err != nil {
			if err != io.EOF && !errors.Is(err, net.ErrClosed) {
				wruntime.EventsEmit(a.ctx, "rfswift:terminal:error", map[string]any{"id": s.id, "error": err.Error()})
			}
			break
		}
	}
	a.termMu.Lock()
	delete(a.terminals, s.id)
	a.termMu.Unlock()
	s.close()
	wruntime.EventsEmit(a.ctx, "rfswift:terminal:exit", map[string]any{"id": s.id, "recordingPath": s.recordPath})
}

func (a *App) TerminalInput(id, data string) error {
	s, err := a.terminal(id)
	if err != nil {
		return err
	}
	s.inputMu.Lock()
	defer s.inputMu.Unlock()
	if s.remote != nil {
		data = terminalColorReply.ReplaceAllString(data, "")
		if data == "" {
			return nil
		}
	}
	return a.writeTerminalInput(s, data)
}

func (a *App) writeTerminalInput(s *terminalSession, data string) error {
	s.writeCast("i", data)
	if s.remote != nil {
		return s.remote.call("terminal.input", map[string]any{"id": s.id, "data": data}, nil)
	}
	_, err := io.WriteString(s.conn, data)
	return err
}

// SubmitAgentPrompt pastes a complete prompt and submits it as two distinct PTY
// operations. Full-screen coding-agent TUIs can treat text+Enter from one write
// as an unfinished paste; bracketed paste plus a short processing boundary
// matches real terminal input and reliably activates the prompt.
func (a *App) SubmitAgentPrompt(id, prompt string) error {
	s, err := a.terminal(id)
	if err != nil {
		return err
	}
	if !s.agent {
		return errors.New("prompt submission is restricted to an agent terminal")
	}
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return errors.New("agent prompt is empty")
	}
	if len(prompt) > 1<<20 || strings.ContainsRune(prompt, '\x00') {
		return errors.New("agent prompt is invalid or too large")
	}
	s.inputMu.Lock()
	defer s.inputMu.Unlock()
	if err := a.writeTerminalInput(s, "\x1b[200~"+prompt+"\x1b[201~"); err != nil {
		return err
	}
	time.Sleep(60 * time.Millisecond)
	return a.writeTerminalInput(s, "\r")
}

func (a *App) ResizeTerminal(id string, cols, rows int) error {
	s, err := a.terminal(id)
	if err != nil {
		return err
	}
	if cols < 2 || rows < 2 {
		return errors.New("invalid terminal size")
	}
	if s.remote != nil {
		return s.remote.call("terminal.resize", map[string]any{"id": id, "cols": cols, "rows": rows}, nil)
	}
	if s.localPTY != nil {
		return pty.Setsize(s.localPTY, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
	}
	_, err = s.cli.ExecResize(context.Background(), s.execID, client.ExecResizeOptions{Width: uint(cols), Height: uint(rows)})
	return err
}

func (a *App) StopTerminal(id string) error {
	s, err := a.terminal(id)
	if err != nil {
		return err
	}
	if s.remote != nil {
		return s.remote.call("terminal.stop", map[string]string{"id": id}, nil)
	}
	// Interrupt a foreground program, ask the shell to exit, then close the
	// hijacked PTY. Closing is essential when the shell is busy or ignores exit.
	_, _ = io.WriteString(s.conn, "\x03exit\r\x04")
	time.Sleep(100 * time.Millisecond)
	s.close()
	return nil
}

func (a *App) terminal(id string) (*terminalSession, error) {
	a.termMu.Lock()
	defer a.termMu.Unlock()
	s := a.terminals[strings.TrimSpace(id)]
	if s == nil {
		return nil, fmt.Errorf("terminal session %q is not active", id)
	}
	return s, nil
}

func (s *terminalSession) writeCast(kind, data string) {
	if s.record == nil {
		return
	}
	s.recordMu.Lock()
	defer s.recordMu.Unlock()
	event := []any{time.Since(s.started).Seconds(), kind, data}
	b, _ := json.Marshal(event)
	_, _ = s.record.Write(append(b, '\n'))
}

func (s *terminalSession) close() {
	s.closeOnce.Do(func() {
		if s.conn != nil {
			_ = s.conn.Close()
		}
		if s.record != nil {
			_ = s.record.Close()
		}
		if s.cli != nil {
			_ = s.cli.Close()
		}
		if s.localCmd != nil && s.localCmd.Process != nil {
			_ = s.localCmd.Process.Kill()
			_, _ = s.localCmd.Process.Wait()
		}
	})
}
