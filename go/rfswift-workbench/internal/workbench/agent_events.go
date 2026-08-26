package workbench

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const agentTerminalEventsFile = "terminal-events.jsonl"

type AgentTerminalEvent struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Mission string `json:"mission"`
	Command string `json:"command,omitempty"`
	Data    string `json:"data,omitempty"`
	Error   string `json:"error,omitempty"`
	At      int64  `json:"at"`
}

type AgentTerminalEvents struct {
	Events []AgentTerminalEvent `json:"events"`
	Cursor int64                `json:"cursor"`
}

type agentEventWriter struct {
	mu      sync.Mutex
	file    *os.File
	id      string
	mission string
}

func newAgentEventWriter(store *Store, workspace, mission, command string) (*agentEventWriter, error) {
	dir := filepath.Join(store.missionDir(workspace, mission), "agent-workspace")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(filepath.Join(dir, agentTerminalEventsFile), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	w := &agentEventWriter{file: file, id: fmt.Sprintf("cmd-%x", time.Now().UnixNano()), mission: mission}
	if err := w.event(AgentTerminalEvent{Type: "start", Command: command}); err != nil {
		_ = file.Close()
		return nil, err
	}
	return w, nil
}

func (w *agentEventWriter) Write(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, nil
	}
	if err := w.event(AgentTerminalEvent{Type: "output", Data: string(data)}); err != nil {
		return 0, err
	}
	return len(data), nil
}

func (w *agentEventWriter) finish(runErr error) {
	event := AgentTerminalEvent{Type: "exit"}
	if runErr != nil {
		event.Error = runErr.Error()
	}
	_ = w.event(event)
	_ = w.file.Close()
}

func (w *agentEventWriter) event(event AgentTerminalEvent) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	event.ID, event.Mission, event.At = w.id, w.mission, time.Now().UnixMilli()
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = w.file.Write(append(data, '\n'))
	return err
}

func (a *App) ReadAgentTerminalEvents(mission string, cursor int64) (AgentTerminalEvents, error) {
	if !validWorkspaceName(mission) || cursor < 0 {
		return AgentTerminalEvents{}, errors.New("invalid agent terminal event request")
	}
	path := filepath.Join(a.store.missionDir(a.ws, mission), "agent-workspace", agentTerminalEventsFile)
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return AgentTerminalEvents{Events: []AgentTerminalEvent{}, Cursor: 0}, nil
	}
	if err != nil {
		return AgentTerminalEvents{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return AgentTerminalEvents{}, err
	}
	if cursor > info.Size() { // file was reset for a new agent session
		cursor = 0
	}
	if _, err := file.Seek(cursor, io.SeekStart); err != nil {
		return AgentTerminalEvents{}, err
	}
	result := AgentTerminalEvents{Events: []AgentTerminalEvent{}, Cursor: cursor}
	reader := bufio.NewReader(io.LimitReader(file, 1<<20))
	for {
		line, readErr := reader.ReadString('\n')
		if readErr != nil {
			if readErr == io.EOF {
				break // leave a partial final line for the next poll
			}
			return result, readErr
		}
		result.Cursor += int64(len(line))
		var event AgentTerminalEvent
		if err := json.Unmarshal([]byte(strings.TrimSuffix(line, "\n")), &event); err != nil {
			return result, fmt.Errorf("invalid agent terminal event: %w", err)
		}
		result.Events = append(result.Events, event)
	}
	return result, nil
}
