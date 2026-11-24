/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
 */

package workbench

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	"github.com/moby/moby/client"
	rfdock "penthertz/rfswift/dock"
	rfnix "penthertz/rfswift/nix"
	"penthertz/rfswift/ptyx"
)

// agentProcess is one program an MCP agent started in its mission target: a
// background job (sh -lc COMMAND) or an interactive session (a login shell the
// agent types into). Output is a terminal stream; Read returns an error once
// the program is gone.
type agentProcess interface {
	io.Reader
	// Input writes keyboard input (sessions).
	Input(data []byte) error
	// Stop terminates the program and everything it started.
	Stop() error
	// Wait returns the exit code once the output stream has ended.
	Wait() (int, error)
	// Close releases the terminal and the engine connection.
	Close() error
}

// agentProcessStarter starts agent processes in a mission target. The local
// engine implements it for container and Nix missions; tests substitute it.
type agentProcessStarter interface {
	startAgentProcess(mission, command string, interactive bool) (agentProcess, error)
}

const (
	agentTermCols = 200
	agentTermRows = 50
)

// Environment shared by agent jobs and sessions. TERM keeps full-screen tools
// working; the output is cleaned before the agent reads it.
var agentProcessEnv = []string{"TERM=xterm-256color", "LANG=C.UTF-8", "LC_ALL=C.UTF-8", "RFSWIFT_AGENT=1"}

func (e *LocalEngine) startAgentProcess(mission, command string, interactive bool) (agentProcess, error) {
	if ev, err := rfnix.GetEnvironment(mission); err == nil {
		return startNixAgentProcess(ev, mission, command, interactive)
	}
	return e.startContainerAgentProcess(mission, command, interactive)
}

// --- Nix environments: a host process on a pseudo-terminal -----------------

type hostPTYProcess struct {
	term     ptyx.Terminal
	stopOnce sync.Once
	exited   atomic.Bool // set once Wait reaped the process: its PID may be reused
}

func startNixAgentProcess(ev *rfnix.Environment, mission, command string, interactive bool) (agentProcess, error) {
	var cmd *exec.Cmd
	var err error
	if interactive {
		shell := ""
		if _, statErr := os.Stat("/bin/bash"); statErr == nil {
			shell = "/bin/bash"
		}
		cmd, err = rfnix.InteractiveCommand(mission, shell)
	} else {
		cmd, err = nixEnvironmentCommand(ev, command)
	}
	if err != nil {
		return nil, err
	}
	if cmd.Env == nil {
		cmd.Env = os.Environ()
	}
	cmd.Env = append(cmd.Env, agentProcessEnv...)
	if ev.Workspace != "" {
		cmd.Env = append(cmd.Env, "RFSWIFT_WORKSPACE="+ev.Workspace)
	}
	term, err := ptyx.Start(cmd, agentTermCols, agentTermRows)
	if err != nil {
		return nil, err
	}
	return &hostPTYProcess{term: term}, nil
}

func (p *hostPTYProcess) Read(b []byte) (int, error) { return p.term.Read(b) }

func (p *hostPTYProcess) Input(data []byte) error {
	_, err := p.term.Write(data)
	return err
}

// Stop hangs up the program's process group (an interactive shell passes the
// hang-up on to its jobs), asks it to terminate, and kills what is left after
// a grace period. On Windows the console is closed, which ends the tree.
func (p *hostPTYProcess) Stop() error {
	p.stopOnce.Do(func() {
		pid := p.term.Pid()
		if pid <= 0 || !signalProcessGroup(pid, groupHangup) {
			_ = p.term.Close()
			return
		}
		signalProcessGroup(pid, groupTerminate)
		go func() {
			time.Sleep(3 * time.Second)
			if !p.exited.Load() {
				signalProcessGroup(pid, groupKill)
				_ = p.term.Close()
			}
		}()
	})
	return nil
}

func (p *hostPTYProcess) Wait() (int, error) {
	err := p.term.Wait()
	p.exited.Store(true)
	if err == nil {
		return 0, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), nil
	}
	return -1, err
}

func (p *hostPTYProcess) Close() error { return p.term.Close() }

// --- Containers: a docker/podman exec session on a TTY ---------------------

// The wrapper records the PID of the exec'd shell, which leads the exec's
// process group (a TTY exec runs in its own session), so Stop can signal the
// whole group from a second exec. $1 is the PID file and $2 the command.
const (
	containerJobWrapper     = `echo $$ > "$1"; sh -lc "$2"; rc=$?; rm -f "$1"; exit $rc`
	containerSessionWrapper = `echo $$ > "$1"; if command -v bash >/dev/null 2>&1; then exec bash -il; else exec sh -il; fi`
	containerStopScript     = `p=$(cat "$1" 2>/dev/null) || exit 0; [ -n "$p" ] || exit 0; kill -HUP "$p" 2>/dev/null; kill -TERM -- "-$p" 2>/dev/null; i=0; while kill -0 "$p" 2>/dev/null && [ "$i" -lt 30 ]; do sleep 0.1; i=$((i+1)); done; kill -KILL -- "-$p" 2>/dev/null; rm -f "$1"`
)

type containerAgentProcess struct {
	cli       *client.Client
	engType   rfdock.EngineType
	mission   string
	execID    string
	pidFile   string
	conn      io.ReadWriteCloser
	reader    *bufio.Reader
	inputMu   sync.Mutex
	stopOnce  sync.Once
	closeOnce sync.Once
}

func (e *LocalEngine) startContainerAgentProcess(mission, command string, interactive bool) (agentProcess, error) {
	cli, engType, err := e.clientFor(mission)
	if err != nil {
		return nil, err
	}
	ctx := context.Background()
	info, err := cli.ContainerInspect(ctx, mission, client.ContainerInspectOptions{})
	if err != nil {
		cli.Close()
		return nil, err
	}
	if info.Container.State == nil || !info.Container.State.Running {
		cli.Close()
		return nil, errors.New("the mission container is not running: start it from the Workbench first")
	}
	_, _ = rfdock.SyncSerialDevices(ctx, cli, mission)
	env := append([]string(nil), agentProcessEnv...)
	workdir := ""
	for _, mp := range info.Container.Mounts {
		if mp.Destination == "/workspace" {
			workdir = "/workspace"
			env = append(env, "RFSWIFT_WORKSPACE=/workspace")
			break
		}
	}
	pidFile := fmt.Sprintf("/tmp/.rfswift-agent-%x.pid", time.Now().UnixNano())
	cmd := []string{"sh", "-c", containerJobWrapper, "rfswift-agent", pidFile, command}
	if interactive {
		cmd = []string{"sh", "-c", containerSessionWrapper, "rfswift-agent", pidFile}
	}
	created, err := cli.ExecCreate(ctx, mission, client.ExecCreateOptions{
		Cmd: cmd, Env: env, WorkingDir: workdir, TTY: true,
		AttachStdin: interactive, AttachStdout: true, AttachStderr: true,
	})
	if err != nil {
		cli.Close()
		return nil, err
	}
	attached, err := cli.ExecAttach(ctx, created.ID, client.ExecAttachOptions{TTY: true})
	if err != nil {
		cli.Close()
		return nil, err
	}
	_, _ = cli.ExecResize(ctx, created.ID, client.ExecResizeOptions{Width: agentTermCols, Height: agentTermRows})
	// Docker attaches before starting the exec session; Podman's compatibility
	// API starts it as part of Attach and rejects a second start.
	if engType != rfdock.EnginePodman {
		if _, err := cli.ExecStart(ctx, created.ID, client.ExecStartOptions{TTY: true}); err != nil {
			attached.Close()
			cli.Close()
			return nil, fmt.Errorf("start container command: %w", err)
		}
	}
	return &containerAgentProcess{cli: cli, engType: engType, mission: mission, execID: created.ID, pidFile: pidFile, conn: attached.Conn, reader: attached.Reader}, nil
}

func (p *containerAgentProcess) Read(b []byte) (int, error) { return p.reader.Read(b) }

func (p *containerAgentProcess) Input(data []byte) error {
	p.inputMu.Lock()
	defer p.inputMu.Unlock()
	_, err := p.conn.Write(data)
	return err
}

// Stop signals the recorded process group from a second exec in the same
// container. If that cannot run, closing the attach connection is the
// fallback, which ends the stream the job pump is waiting on.
func (p *containerAgentProcess) Stop() error {
	var stopErr error
	p.stopOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		created, err := p.cli.ExecCreate(ctx, p.mission, client.ExecCreateOptions{
			Cmd:          []string{"sh", "-c", containerStopScript, "rfswift-stop", p.pidFile},
			AttachStdout: true, AttachStderr: true,
		})
		if err == nil {
			attached, attachErr := p.cli.ExecAttach(ctx, created.ID, client.ExecAttachOptions{})
			err = attachErr
			if err == nil {
				if p.engType != rfdock.EnginePodman {
					_, err = p.cli.ExecStart(ctx, created.ID, client.ExecStartOptions{})
				}
				if err == nil {
					_, _ = io.Copy(io.Discard, attached.Reader)
				}
				attached.Close()
			}
		}
		if err != nil {
			stopErr = err
			_ = p.conn.Close()
		}
	})
	return stopErr
}

// Wait reads the exit code; the exec can be reported running for a moment
// after its stream closed.
func (p *containerAgentProcess) Wait() (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for {
		status, err := p.cli.ExecInspect(ctx, p.execID, client.ExecInspectOptions{})
		if err != nil {
			return -1, fmt.Errorf("inspect container command: %w", err)
		}
		if !status.Running {
			return status.ExitCode, nil
		}
		select {
		case <-ctx.Done():
			return -1, errors.New("the container command did not report an exit status")
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func (p *containerAgentProcess) Close() error {
	p.closeOnce.Do(func() {
		_ = p.conn.Close()
		_ = p.cli.Close()
	})
	return nil
}
