package cli

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/moby/moby/client"
	rfdock "penthertz/rfswift/dock"
	rfnix "penthertz/rfswift/nix"
	"penthertz/rfswift/ptyx"
	"penthertz/rfswift/remote"
)

type agentTarget struct {
	ID, Title, Engine, Env, Image, User, Net, Status, DesktopURL, FlakeRef string
	Caps, Cgroups, Mounts                                                  []string
}
type agentPort struct{ Port, Published, Service string }
type agentCreate struct {
	Name, Title, Engine, Image, FlakeRef, Workspace, Network, ExposedPorts, PortBindings, GPUs, Seccomp, Shell, DesktopProto, DesktopHost, DesktopPort, DesktopPassword string
	Caps, Bindings, Devices, CgroupRules, ExtraHosts, Environment                                                                                                       []string
	Realtime, Desktop, DesktopSSL, NoX11, Privileged, Start, Lazy, Pure                                                                                                 bool
}
type agentChange struct {
	ID, Kind, Value, Source, Target string
	Add                             bool
}
type agentTerminalRequest struct {
	ID, Mission, Shell, Data string
	Cols, Rows               int
}
type agentArtifactRequest struct{ Mission, Path string }
type agentArtifact struct {
	Path, Name, Modified, Type string
	Size                       int64
}
type agentArtifactData struct {
	Path, Data string
	Truncated  bool
}

// remotePTY is one interactive shell served to a remote client. The terminal
// comes from ptyx, so it is a Unix PTY on Linux/macOS agents and a ConPTY on
// Windows agents.
type remotePTY struct {
	term    ptyx.Terminal
	cmdDone chan struct{}
	mu      sync.Mutex
	output  []byte
	closed  bool
}

var remotePTYs = struct {
	sync.Mutex
	sessions map[string]*remotePTY
}{sessions: map[string]*remotePTY{}}

func agentControl(ctx context.Context, req remote.ControlRequest) (any, error) {
	decode := func(v any) error { return json.Unmarshal(req.Params, v) }
	switch req.Method {
	case "targets.list":
		return agentListTargets()
	case "targets.inspect":
		var p struct{ ID string }
		if err := decode(&p); err != nil {
			return nil, err
		}
		return agentInspect(p.ID)
	case "targets.start", "targets.stop":
		var p struct{ ID string }
		if err := decode(&p); err != nil {
			return nil, err
		}
		return nil, agentLifecycle(ctx, p.ID, req.Method == "targets.start")
	case "targets.create":
		var p agentCreate
		if err := decode(&p); err != nil {
			return nil, err
		}
		return agentCreateTarget(p)
	case "targets.delete":
		var p struct {
			ID         string
			Nix, Clean bool
		}
		if err := decode(&p); err != nil {
			return nil, err
		}
		return nil, agentDelete(p.ID, p.Nix, p.Clean)
	case "targets.configure":
		var p agentChange
		if err := decode(&p); err != nil {
			return nil, err
		}
		return nil, agentConfigure(p)
	case "images.check":
		var p struct{ Engine, Image string }
		if err := decode(&p); err != nil {
			return nil, err
		}
		return rfdock.CheckImage(p.Engine, p.Image)
	case "images.pull":
		var p struct{ Engine, Image string }
		if err := decode(&p); err != nil {
			return nil, err
		}
		return rfdock.PullImageContext(ctx, p.Engine, p.Image, nil)
	case "profiles.defaults":
		d, err := rfdock.LoadCreationDefaults()
		if err != nil {
			return nil, err
		}
		return map[string]any{"path": d.Path, "image": d.Image, "shell": d.Shell, "bindings": d.Bindings, "network": d.Network, "exposedPorts": d.ExposedPorts, "portBindings": d.PortBindings, "extraHosts": d.ExtraHosts, "environment": d.Environment, "devices": d.Devices, "privileged": d.Privileged, "caps": d.Caps, "seccomp": d.Seccomp, "cgroupRules": d.CgroupRules, "desktopProto": d.DesktopProto, "desktopHost": d.DesktopHost, "desktopPort": d.DesktopPort, "desktopPassword": d.DesktopPass, "desktopSSL": d.DesktopSSL}, nil
	case "profiles.list":
		rfdock.InitDefaultProfiles(false)
		return rfdock.GetAllProfiles(), nil
	case "audit.run":
		var p struct{ ID string }
		if err := decode(&p); err != nil {
			return nil, err
		}
		return agentAudit(p.ID)
	case "tools.search":
		var p struct {
			Mission, Query string
			AllNixpkgs     bool
		}
		if err := decode(&p); err != nil {
			return nil, err
		}
		return agentSearchTools(p.Mission, p.Query, p.AllNixpkgs)
	case "tools.install":
		var p struct{ Mission, Name string }
		if err := decode(&p); err != nil {
			return nil, err
		}
		return nil, agentInstallTool(p.Mission, p.Name)
	case "terminal.start":
		var p agentTerminalRequest
		if err := decode(&p); err != nil {
			return nil, err
		}
		return agentTerminalStart(p)
	case "terminal.input":
		var p agentTerminalRequest
		if err := decode(&p); err != nil {
			return nil, err
		}
		return nil, agentTerminalInput(p.ID, p.Data)
	case "terminal.resize":
		var p agentTerminalRequest
		if err := decode(&p); err != nil {
			return nil, err
		}
		return nil, agentTerminalResize(p.ID, p.Cols, p.Rows)
	case "terminal.read":
		var p agentTerminalRequest
		if err := decode(&p); err != nil {
			return nil, err
		}
		return agentTerminalRead(p.ID)
	case "terminal.stop":
		var p agentTerminalRequest
		if err := decode(&p); err != nil {
			return nil, err
		}
		return nil, agentTerminalStop(p.ID)
	case "artifacts.list":
		var p agentArtifactRequest
		if err := decode(&p); err != nil {
			return nil, err
		}
		return agentArtifacts(p.Mission)
	case "artifacts.read":
		var p agentArtifactRequest
		if err := decode(&p); err != nil {
			return nil, err
		}
		return agentArtifactRead(p.Mission, p.Path)
	default:
		return nil, errors.New("unsupported control method")
	}
}

type agentTool struct{ Name, Detail, Source string }

func agentSearchTools(mission, query string, all bool) ([]agentTool, error) {
	t, err := agentInspect(mission)
	if err != nil {
		return nil, err
	}
	query = strings.ToLower(strings.TrimSpace(query))
	var out []agentTool
	if t.Engine != "nix" {
		for _, name := range rfdock.ListInstallFunctions(mission) {
			if query == "" || strings.Contains(strings.ToLower(name), query) {
				out = append(out, agentTool{Name: name, Detail: strings.TrimSuffix(name, "_install"), Source: "container script"})
			}
		}
		return out, nil
	}
	env, err := rfnix.GetEnvironment(mission)
	if err != nil {
		return nil, err
	}
	if all {
		hits, err := rfnix.SearchNixpkgs(env.FlakeRef, query)
		if err != nil {
			return nil, err
		}
		for name, detail := range hits {
			out = append(out, agentTool{Name: name, Detail: detail, Source: "nixpkgs"})
		}
		sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
		return out, nil
	}
	for _, hit := range rfnix.SearchPackages(query) {
		out = append(out, agentTool{Name: hit.Name, Detail: strings.Join(hit.Envs, ", "), Source: "RF Swift"})
	}
	return out, nil
}
func agentInstallTool(mission, name string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("tool name is required")
	}
	if env, err := rfnix.GetEnvironment(mission); err == nil {
		return rfnix.InstallPackages(env.FlakeRef, []string{name}, mission)
	}
	return rfdock.ContainerInstallScript(mission, "entrypoint.sh", name)
}

func agentAudit(id string) (map[string]string, error) {
	out, err := os.MkdirTemp("", "rfswift-remote-audit-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(out)
	if env, envErr := rfnix.GetEnvironment(id); envErr == nil {
		err = rfnix.RunAudit(env.FlakeRef, []string{"--env", env.Image, "--format", "json", "--out", out})
	} else {
		err = rfdock.AuditContainer(id, rfdock.ContainerAuditOptions{OutDir: out, Formats: "json"})
	}
	data, readErr := os.ReadFile(filepath.Join(out, "report.json"))
	if readErr != nil {
		if err != nil {
			return nil, err
		}
		return nil, readErr
	}
	return map[string]string{"data": base64.StdEncoding.EncodeToString(data)}, nil
}

func agentListTargets() ([]agentTarget, error) {
	var out []agentTarget
	eng := string(rfdock.GetEngine().Type())
	for _, c := range rfdock.ListContainers("org.container.project", "rfswift") {
		n := strings.TrimPrefix(c.Name, "/")
		out = append(out, agentTarget{ID: n, Title: n, Engine: eng, Image: c.Image, Status: agentState(c.State)})
	}
	envs, err := rfnix.ListEnvironments()
	if err == nil {
		for _, e := range envs {
			st := "stopped"
			if e.Realised() {
				st = "up"
			}
			out = append(out, agentTarget{ID: e.Name, Title: e.Name, Engine: "nix", Env: e.Image, Image: "nix env", Status: st, FlakeRef: e.FlakeRef, Mounts: []string{e.Workspace}})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
func agentState(s string) string {
	s = strings.ToLower(s)
	if strings.Contains(s, "running") || s == "up" {
		return "up"
	}
	return "stopped"
}
func agentInspect(id string) (agentTarget, error) {
	if e, err := rfnix.GetEnvironment(id); err == nil {
		return agentTarget{ID: e.Name, Title: e.Name, Engine: "nix", Env: e.Image, Image: "nix env (" + e.FlakeRef + ")", User: "you", Net: "host (native)", Status: map[bool]string{true: "up", false: "stopped"}[e.Realised()], Mounts: []string{e.Workspace}, FlakeRef: e.FlakeRef}, nil
	}
	c, err := rfdock.NewEngineClient()
	if err != nil {
		return agentTarget{}, err
	}
	defer c.Close()
	res, err := c.ContainerInspect(context.Background(), id, client.ContainerInspectOptions{})
	if err != nil {
		return agentTarget{}, err
	}
	i := res.Container
	t := agentTarget{ID: strings.TrimPrefix(i.Name, "/"), Title: strings.TrimPrefix(i.Name, "/"), Engine: string(rfdock.GetEngine().Type()), User: "root", Status: "stopped"}
	if i.Config != nil {
		t.Image = i.Config.Image
		if i.Config.User != "" {
			t.User = i.Config.User
		}
	}
	if i.State != nil && i.State.Running {
		t.Status = "up"
	}
	if i.HostConfig != nil {
		t.Net = string(i.HostConfig.NetworkMode)
		t.Caps = append([]string{}, i.HostConfig.CapAdd...)
		t.Cgroups = append([]string{}, i.HostConfig.DeviceCgroupRules...)
	}
	for _, m := range i.Mounts {
		rw := "ro"
		if m.RW {
			rw = "rw"
		}
		t.Mounts = append(t.Mounts, m.Source+" -> "+m.Destination+" ("+rw+")")
	}
	return t, nil
}
func agentLifecycle(ctx context.Context, id string, start bool) error {
	if _, e := rfnix.GetEnvironment(id); e == nil {
		return errors.New("Nix environments have no start/stop lifecycle")
	}
	c, e := rfdock.NewEngineClient()
	if e != nil {
		return e
	}
	defer c.Close()
	if start {
		_, e = c.ContainerStart(ctx, id, client.ContainerStartOptions{})
	} else {
		_, e = c.ContainerStop(ctx, id, client.ContainerStopOptions{})
	}
	return e
}
func agentCreateTarget(p agentCreate) (agentTarget, error) {
	if p.Engine == "nix" {
		e := rfnix.RunEnvironment(rfnix.RunOptions{Name: p.Name, Image: p.Image, Workspace: p.Workspace, FlakeRef: p.FlakeRef, Lazy: p.Lazy, Pure: p.Pure, CreateOnly: true})
		if e != nil {
			return agentTarget{}, e
		}
	} else {
		_, e := rfdock.CreateContainer(rfdock.CreateOptions{Engine: p.Engine, Name: p.Name, Image: p.Image, Workspace: p.Workspace, Network: p.Network, Shell: p.Shell, Caps: p.Caps, Bindings: p.Bindings, Devices: p.Devices, ExposedPorts: p.ExposedPorts, PortBindings: p.PortBindings, CgroupRules: p.CgroupRules, GPUs: p.GPUs, Seccomp: p.Seccomp, ExtraHosts: p.ExtraHosts, Environment: p.Environment, Realtime: p.Realtime, Desktop: p.Desktop, DesktopProto: p.DesktopProto, DesktopHost: p.DesktopHost, DesktopPort: p.DesktopPort, DesktopPassword: p.DesktopPassword, DesktopSSL: p.DesktopSSL, NoX11: p.NoX11, Privileged: p.Privileged, Start: p.Start})
		if e != nil {
			return agentTarget{}, e
		}
	}
	return agentInspect(p.Name)
}
func agentDelete(id string, nix, clean bool) error {
	if nix {
		if e := rfnix.RemoveEnvironment(id); e != nil {
			return e
		}
		if clean {
			return rfnix.GarbageCollect(rfnix.GCOptions{})
		}
		return nil
	}
	return rfdock.RemoveContainer(id)
}
func agentConfigure(p agentChange) error {
	switch p.Kind {
	case "volume", "device":
		return rfdock.UpdateBinding(p.ID, p.Kind, p.Source, p.Target, p.Add)
	case "device-bind":
		return rfdock.UpdateBinding(p.ID, "volume", p.Source, p.Target, p.Add)
	case "capability":
		return rfdock.UpdateCapability(p.ID, p.Value, p.Add)
	case "cgroup":
		return rfdock.UpdateCgroupRule(p.ID, p.Value, p.Add)
	case "gpu":
		return rfdock.UpdateGPUs(p.ID, p.Value, p.Add)
	case "exposed-port":
		return rfdock.UpdateExposedPort(p.ID, p.Value, p.Add)
	case "published-port":
		return rfdock.UpdatePortBinding(p.ID, p.Value, p.Add)
	}
	return errors.New("unsupported container setting")
}

func agentTerminalStart(p agentTerminalRequest) (map[string]string, error) {
	if p.Cols < 2 {
		p.Cols = 80
	}
	if p.Rows < 2 {
		p.Rows = 24
	}
	if p.Shell == "" {
		p.Shell = "/bin/zsh"
	}
	t, e := agentInspect(p.Mission)
	if e != nil {
		return nil, e
	}
	exe, e := os.Executable()
	if e != nil {
		return nil, e
	}
	args := []string{"--engine", t.Engine, "exec", "-c", p.Mission, "-e", p.Shell}
	cmd := exec.Command(exe, args...)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "COLORTERM=truecolor")
	f, e := ptyx.Start(cmd, p.Cols, p.Rows)
	if e != nil {
		return nil, e
	}
	id := fmt.Sprintf("remote-%d", time.Now().UnixNano())
	s := &remotePTY{term: f, cmdDone: make(chan struct{})}
	remotePTYs.Lock()
	remotePTYs.sessions[id] = s
	remotePTYs.Unlock()
	go func() {
		buf := make([]byte, 32<<10)
		for {
			n, e := f.Read(buf)
			if n > 0 {
				s.mu.Lock()
				if len(s.output) < 4<<20 {
					s.output = append(s.output, buf[:n]...)
				}
				s.mu.Unlock()
			}
			if e != nil {
				break
			}
		}
		s.mu.Lock()
		s.closed = true
		s.mu.Unlock()
		close(s.cmdDone)
	}()
	return map[string]string{"id": id}, nil
}

func agentTerminalInput(id, data string) error {
	remotePTYs.Lock()
	s := remotePTYs.sessions[id]
	remotePTYs.Unlock()
	if s == nil {
		return errors.New("terminal session is not active")
	}
	_, e := io.WriteString(s.term, data)
	return e
}
func agentTerminalResize(id string, cols, rows int) error {
	remotePTYs.Lock()
	s := remotePTYs.sessions[id]
	remotePTYs.Unlock()
	if s == nil {
		return errors.New("terminal session is not active")
	}
	return s.term.Resize(cols, rows)
}
func agentTerminalRead(id string) (map[string]any, error) {
	remotePTYs.Lock()
	s := remotePTYs.sessions[id]
	remotePTYs.Unlock()
	if s == nil {
		return map[string]any{"closed": true}, nil
	}
	s.mu.Lock()
	b := append([]byte{}, s.output...)
	s.output = nil
	closed := s.closed
	s.mu.Unlock()
	return map[string]any{"data": string(b), "closed": closed}, nil
}
func agentTerminalStop(id string) error {
	remotePTYs.Lock()
	s := remotePTYs.sessions[id]
	delete(remotePTYs.sessions, id)
	remotePTYs.Unlock()
	if s == nil {
		return nil
	}
	_, _ = io.WriteString(s.term, "\x03exit\r\x04")
	return s.term.Close()
}

func agentWorkspace(mission string) (string, error) {
	t, e := agentInspect(mission)
	if e != nil {
		return "", e
	}
	for _, m := range t.Mounts {
		if strings.Contains(m, " -> /workspace ") {
			return strings.TrimSpace(strings.SplitN(m, " -> ", 2)[0]), nil
		}
	}
	if t.Engine == "nix" && len(t.Mounts) > 0 && t.Mounts[0] != "" && t.Mounts[0] != "none" {
		return t.Mounts[0], nil
	}
	h, e := os.UserHomeDir()
	if e != nil {
		return "", e
	}
	return filepath.Join(h, "rfswift-workspace", mission), nil
}
func agentSafeArtifact(mission, rel string) (string, error) {
	root, e := agentWorkspace(mission)
	if e != nil {
		return "", e
	}
	root, e = filepath.Abs(root)
	if e != nil {
		return "", e
	}
	rel = filepath.Clean(filepath.FromSlash(rel))
	if rel == "." || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("artifact path escapes workspace")
	}
	p := filepath.Join(root, rel)
	resolved, e := filepath.EvalSymlinks(p)
	if e != nil {
		return "", e
	}
	if resolved != root && !strings.HasPrefix(resolved, root+string(filepath.Separator)) {
		return "", errors.New("artifact symlink escapes workspace")
	}
	return resolved, nil
}
func agentArtifacts(mission string) ([]agentArtifact, error) {
	root, e := agentWorkspace(mission)
	if e != nil {
		return nil, e
	}
	var out []agentArtifact
	e = filepath.WalkDir(root, func(path string, d os.DirEntry, walk error) error {
		if walk != nil {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if d.IsDir() {
			if path != root && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if len(out) >= 5000 || !d.Type().IsRegular() {
			return nil
		}
		i, e := d.Info()
		if e != nil {
			return nil
		}
		rel, e := filepath.Rel(root, path)
		if e == nil {
			out = append(out, agentArtifact{Path: filepath.ToSlash(rel), Name: d.Name(), Size: i.Size(), Modified: i.ModTime().UTC().Format(time.RFC3339)})
		}
		return nil
	})
	if os.IsNotExist(e) {
		return []agentArtifact{}, nil
	}
	return out, e
}
func agentArtifactRead(mission, rel string) (agentArtifactData, error) {
	p, e := agentSafeArtifact(mission, rel)
	if e != nil {
		return agentArtifactData{}, e
	}
	f, e := os.Open(p)
	if e != nil {
		return agentArtifactData{}, e
	}
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, 16<<20+1))
	if e != nil {
		return agentArtifactData{}, e
	}
	tr := len(b) > 16<<20
	if tr {
		b = b[:16<<20]
	}
	return agentArtifactData{Path: rel, Data: base64.StdEncoding.EncodeToString(b), Truncated: tr}, nil
}
