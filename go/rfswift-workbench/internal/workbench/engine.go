package workbench

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
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/moby/moby/client"

	rfdock "penthertz/rfswift/dock"
	rfnix "penthertz/rfswift/nix"
	"penthertz/rfswift/remote"
)

// Engine abstracts the thing that actually runs targets. It is engine-agnostic:
// LocalEngine drives the engines on this machine through the rfswift packages
// (docker/podman/lima/nix); a remote implementation will speak to an
// `rfswift agent` over the authenticated protocol (docs/remote-agent.md). The
// GUI never cares which engine a target uses; it just calls these methods.
type Engine interface {
	Name() string
	ListTargets() ([]Mission, error)
	Inspect(id string) (Mission, error)
	Start(id string) error
	Stop(id string) error
	Audit(id string) (Posture, error)
	Exec(id, cmd string) (string, error)
	Create(req MissionCreate) (Mission, error)
}

// RemotePendingEngine prevents a selected or unreachable remote agent from
// silently falling back to local Docker/Nix operations.
type RemotePendingEngine struct{ Agent string }

func (e *RemotePendingEngine) unavailable() error {
	return fmt.Errorf("remote agent %s selected; mission synchronization and interactive terminals are not available yet", e.Agent)
}

// RemoteEngine forwards engine-neutral operations to the authenticated agent.
type RemoteEngine struct{ Config remote.ClientConfig }

func (e *RemoteEngine) Name() string { return "remote:" + e.Config.Endpoint }
func (e *RemoteEngine) call(method string, params, result any) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	return e.callContext(ctx, method, params, result)
}
func (e *RemoteEngine) callContext(ctx context.Context, method string, params, result any) error {
	return remote.Control(ctx, e.Config, method, params, result)
}
func (e *RemoteEngine) ListTargets() ([]Mission, error) {
	var out []Mission
	err := e.call("targets.list", map[string]any{}, &out)
	return out, err
}
func (e *RemoteEngine) Inspect(id string) (Mission, error) {
	var out Mission
	err := e.call("targets.inspect", map[string]string{"id": id}, &out)
	return out, err
}
func (e *RemoteEngine) Start(id string) error {
	return e.call("targets.start", map[string]string{"id": id}, nil)
}
func (e *RemoteEngine) Stop(id string) error {
	return e.call("targets.stop", map[string]string{"id": id}, nil)
}
func (e *RemoteEngine) Audit(id string) (Posture, error) {
	result, err := e.AuditDetailed(id)
	return result.Posture, err
}
func (e *RemoteEngine) AuditDetailed(id string) (AuditResult, error) {
	var payload struct {
		Data string `json:"data"`
	}
	if err := e.call("audit.run", map[string]string{"id": id}, &payload); err != nil {
		return AuditResult{}, err
	}
	data, err := base64.StdEncoding.DecodeString(payload.Data)
	if err != nil {
		return AuditResult{}, err
	}
	f, err := os.CreateTemp("", "rfswift-remote-audit-*.json")
	if err != nil {
		return AuditResult{}, err
	}
	path := f.Name()
	defer os.Remove(path)
	if _, err = f.Write(data); err != nil {
		f.Close()
		return AuditResult{}, err
	}
	if err = f.Close(); err != nil {
		return AuditResult{}, err
	}
	return parseAuditFile(path), nil
}
func (e *RemoteEngine) Exec(id, command string) (string, error) {
	r, err := remote.RunCommand(context.Background(), e.Config, []string{"exec", "-c", id, "-e", command})
	if err != nil {
		return "", err
	}
	if r.Error != "" {
		return r.Output, errors.New(r.Error)
	}
	return r.Output, nil
}
func (e *RemoteEngine) Create(req MissionCreate) (Mission, error) {
	var out Mission
	err := e.call("targets.create", req, &out)
	return out, err
}
func (e *RemoteEngine) Delete(id string, nix, clean bool) error {
	return e.call("targets.delete", map[string]any{"id": id, "nix": nix, "clean": clean}, nil)
}
func (e *RemoteEngine) Configure(id string, change ContainerChange) error {
	return e.call("targets.configure", map[string]any{"id": id, "kind": change.Kind, "value": change.Value, "source": change.Source, "target": change.Target, "add": change.Add}, nil)
}
func (e *RemotePendingEngine) Name() string                        { return "remote:" + e.Agent }
func (e *RemotePendingEngine) ListTargets() ([]Mission, error)     { return nil, e.unavailable() }
func (e *RemotePendingEngine) Inspect(string) (Mission, error)     { return Mission{}, e.unavailable() }
func (e *RemotePendingEngine) Start(string) error                  { return e.unavailable() }
func (e *RemotePendingEngine) Stop(string) error                   { return e.unavailable() }
func (e *RemotePendingEngine) Audit(string) (Posture, error)       { return Posture{}, e.unavailable() }
func (e *RemotePendingEngine) Exec(string, string) (string, error) { return "", e.unavailable() }
func (e *RemotePendingEngine) Create(MissionCreate) (Mission, error) {
	return Mission{}, e.unavailable()
}

// ErrNotWired is returned for operations that do not apply to a target type.
var ErrNotWired = errors.New("operation not supported for this target")

const rfLabelKey, rfLabelVal = "org.container.project", "rfswift"

// origDockerHost snapshots DOCKER_HOST at startup. rfdock.GetEngine() exports
// the Lima/Podman socket into DOCKER_HOST for the CLI's benefit, but in the
// long-lived GUI that poisons every later Docker call (Docker's client dials
// FromEnv and would silently talk to the Lima daemon). resetEngineEnv restores
// the user's original value before each engine-routing operation.
var origDockerHost, origDockerHostSet = func() (string, bool) {
	v, ok := os.LookupEnv("DOCKER_HOST")
	return v, ok
}()

func resetEngineEnv() {
	if origDockerHostSet {
		os.Setenv("DOCKER_HOST", origDockerHost)
	} else {
		os.Unsetenv("DOCKER_HOST")
	}
}

// containerEngines returns every distinct, installed container engine on this
// host, active engine first. Engines whose socket resolves to one already in
// the list are dropped (e.g. DOCKER_HOST pointing into the Lima VM would make
// Docker and Lima list the same daemon twice).
func containerEngines() []rfdock.ContainerEngine {
	resetEngineEnv()
	candidates := []rfdock.ContainerEngine{rfdock.GetEngine(), &rfdock.DockerEngine{}, &rfdock.PodmanEngine{}}
	if rfdock.IsLimaEngineCandidate() {
		candidates = append(candidates, &rfdock.LimaEngine{})
	}
	var out []rfdock.ContainerEngine
	seenType := map[rfdock.EngineType]bool{}
	seenSocket := map[string]bool{}
	for _, eng := range candidates {
		if eng == nil || seenType[eng.Type()] || !eng.IsAvailable() {
			continue
		}
		seenType[eng.Type()] = true
		if key := canonicalSocket(eng.GetSocketPath()); key != "" {
			if seenSocket[key] {
				continue
			}
			seenSocket[key] = true
		}
		out = append(out, eng)
	}
	return out
}

func canonicalSocket(socket string) string {
	socket = strings.TrimPrefix(socket, "unix://")
	if socket == "" {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(socket); err == nil {
		return resolved
	}
	return socket
}

// engineByType returns a fresh engine for a mission's recorded engine name.
func engineByType(t rfdock.EngineType) rfdock.ContainerEngine {
	switch t {
	case rfdock.EngineDocker:
		return &rfdock.DockerEngine{}
	case rfdock.EnginePodman:
		return &rfdock.PodmanEngine{}
	case rfdock.EngineLima:
		return &rfdock.LimaEngine{}
	}
	return nil
}

// LocalEngine drives docker/podman/lima containers and Nix environments on this
// host via the rfswift packages. Containers from every running engine are
// aggregated into one mission list; route remembers which engine hosts which
// container so lifecycle/exec/terminal operations reach the right daemon.
type LocalEngine struct {
	mu    sync.Mutex
	route map[string]rfdock.EngineType
}

// rememberRoute records which engine hosts a container.
func (e *LocalEngine) rememberRoute(id string, t rfdock.EngineType) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.route == nil {
		e.route = map[string]rfdock.EngineType{}
	}
	e.route[id] = t
}

// engineFor resolves the engine hosting a container, scanning every running
// engine when the container was not seen by a previous ListTargets.
func (e *LocalEngine) engineFor(id string) (rfdock.ContainerEngine, error) {
	e.mu.Lock()
	t, ok := e.route[id]
	e.mu.Unlock()
	if ok {
		if eng := engineByType(t); eng != nil {
			return eng, nil
		}
	}
	resetEngineEnv()
	for _, eng := range containerEngines() {
		if !eng.IsServiceRunning() {
			continue
		}
		cli, err := eng.GetClient()
		if err != nil {
			continue
		}
		_, err = cli.ContainerInspect(context.Background(), id, client.ContainerInspectOptions{})
		cli.Close()
		if err == nil {
			e.rememberRoute(id, eng.Type())
			return eng, nil
		}
	}
	return nil, fmt.Errorf("container %q not found on any running engine", id)
}

// clientFor returns a moby client connected to the engine hosting a container.
func (e *LocalEngine) clientFor(id string) (*client.Client, rfdock.EngineType, error) {
	eng, err := e.engineFor(id)
	if err != nil {
		return nil, "", err
	}
	cli, err := eng.GetClient()
	if err != nil {
		return nil, "", err
	}
	return cli, eng.Type(), nil
}

// RouteMission points the process-wide rfdock engine at the engine hosting
// this mission, for the rfdock helpers that still operate on the global
// engine (audit, config rebinding, install scripts, container removal).
func (e *LocalEngine) RouteMission(id string) {
	eng, err := e.engineFor(id)
	if err != nil {
		return
	}
	resetEngineEnv()
	if rfdock.GetEngine().Type() != eng.Type() {
		rfdock.SetPreferredEngine(string(eng.Type()))
	}
}

func (e *LocalEngine) Create(req MissionCreate) (Mission, error) {
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		return Mission{}, errors.New("mission name is required")
	}
	resetEngineEnv()
	switch req.Engine {
	case "nix":
		if err := rfnix.RunEnvironment(rfnix.RunOptions{Name: req.Name, Image: req.Image, Workspace: req.Workspace, FlakeRef: req.FlakeRef, Lazy: req.Lazy, Pure: req.Pure, CreateOnly: true}); err != nil {
			return Mission{}, err
		}
	case "docker", "podman", "lima":
		_, err := rfdock.CreateContainer(rfdock.CreateOptions{Context: req.Context, Engine: req.Engine, Name: req.Name, Image: req.Image, Workspace: req.Workspace, Network: req.Network, Shell: req.Shell, Caps: req.Caps, Bindings: req.Bindings, Devices: req.Devices, ExposedPorts: req.ExposedPorts, PortBindings: req.PortBindings, CgroupRules: req.CgroupRules, GPUs: req.GPUs, Seccomp: req.Seccomp, ExtraHosts: req.ExtraHosts, Environment: req.Environment, Realtime: req.Realtime, Desktop: req.Desktop, DesktopProto: req.DesktopProto, DesktopHost: req.DesktopHost, DesktopPort: req.DesktopPort, DesktopPassword: req.DesktopPassword, DesktopSSL: req.DesktopSSL, NoX11: req.NoX11, Privileged: req.Privileged, Start: req.Start})
		if err != nil {
			return Mission{}, err
		}
	default:
		return Mission{}, fmt.Errorf("unsupported mission engine %q", req.Engine)
	}
	m, err := e.Inspect(req.Name)
	if err != nil {
		return Mission{}, err
	}
	if req.Title != "" {
		m.Title = req.Title
	}
	return m, nil
}

// NewLocalEngine selects the preferred container engine (RFSWIFT_ENGINE env, or
// auto-detect) and returns the local engine.
func NewLocalEngine() *LocalEngine {
	pref := os.Getenv("RFSWIFT_ENGINE")
	if pref == "" {
		pref = "auto"
	}
	rfdock.SetPreferredEngine(pref)
	return &LocalEngine{route: map[string]rfdock.EngineType{}}
}

// Name reports the active container engine (docker/podman/lima).
func (e *LocalEngine) Name() string { return string(rfdock.GetEngine().Type()) }

// ListTargets returns RF Swift containers from every running engine (Docker,
// Podman, and the Docker daemon inside the Lima VM) plus Nix environments as
// one unified mission list. Engines whose service is down are skipped rather
// than auto-started — the Lima VM boots only on an explicit start.
func (e *LocalEngine) ListTargets() ([]Mission, error) {
	var out []Mission
	route := map[string]rfdock.EngineType{}
	seen := map[string]bool{}
	for _, eng := range containerEngines() {
		if !eng.IsServiceRunning() {
			continue
		}
		for _, c := range listEngineContainers(eng) {
			name := strings.TrimPrefix(c.Name, "/")
			// A container of the same name on a second engine stays hidden
			// until the first one is gone — mission IDs are names.
			if seen[name] {
				continue
			}
			seen[name] = true
			route[name] = eng.Type()
			out = append(out, Mission{
				ID:     name,
				Title:  name,
				Engine: string(eng.Type()),
				Image:  c.Image,
				Status: mapState(c.State),
			})
		}
	}
	e.mu.Lock()
	e.route = route
	e.mu.Unlock()
	if envs, err := rfnix.ListEnvironments(); err == nil {
		for _, ev := range envs {
			st := "stopped"
			if ev.Realised() {
				st = "up"
			}
			out = append(out, Mission{
				ID:       ev.Name,
				Title:    ev.Name,
				Engine:   "nix",
				Env:      ev.Image,
				Image:    "nix env",
				Status:   st,
				FlakeRef: ev.FlakeRef,
				Lazy:     ev.Lazy,
			})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

type engineContainer struct {
	Name  string
	Image string
	State string
}

// listEngineContainers lists RF Swift-labelled containers on one specific
// engine's daemon (rfdock.ListContainers only sees the global active engine).
func listEngineContainers(eng rfdock.ContainerEngine) []engineContainer {
	cli, err := eng.GetClient()
	if err != nil {
		return nil
	}
	defer cli.Close()
	filters := make(client.Filters)
	filters.Add("label", rfLabelKey+"="+rfLabelVal)
	res, err := cli.ContainerList(context.Background(), client.ContainerListOptions{
		All:     true,
		Filters: filters,
	})
	if err != nil {
		return nil
	}
	var out []engineContainer
	for _, c := range res.Items {
		name := c.ID
		if len(name) > 12 {
			name = name[:12]
		}
		if len(c.Names) > 0 {
			name = c.Names[0]
		}
		out = append(out, engineContainer{Name: name, Image: c.Image, State: string(c.State)})
	}
	return out
}

// Inspect fills a mission's configuration/network from a live container (via the
// routed moby client) or a Nix environment.
func (e *LocalEngine) Inspect(id string) (Mission, error) {
	if ev, err := rfnix.GetEnvironment(id); err == nil {
		return Mission{
			ID: ev.Name, Title: ev.Name, Engine: "nix", Env: ev.Image,
			Image: "nix env (" + ev.FlakeRef + ")", User: "you",
			Net: "host (native)", Status: boolState(ev.Realised()),
			Mounts:   []string{ev.Workspace},
			FlakeRef: ev.FlakeRef,
		}, nil
	}
	cli, engType, err := e.clientFor(id)
	if err != nil {
		return Mission{}, err
	}
	defer cli.Close()
	res, err := cli.ContainerInspect(context.Background(), id, client.ContainerInspectOptions{})
	if err != nil {
		return Mission{}, err
	}
	info := res.Container
	m := Mission{
		ID:     strings.TrimPrefix(info.Name, "/"),
		Title:  strings.TrimPrefix(info.Name, "/"),
		Engine: string(engType),
		User:   "root",
		Status: "stopped",
	}
	if info.Config != nil {
		m.Image = info.Config.Image
		if info.Config.User != "" {
			m.User = info.Config.User
		}
		var proto, host, port string
		ssl := false
		for _, item := range info.Config.Env {
			switch {
			case strings.HasPrefix(item, "RFSWIFT_DESKTOP_PROTO="):
				proto = strings.TrimPrefix(item, "RFSWIFT_DESKTOP_PROTO=")
			case strings.HasPrefix(item, "RFSWIFT_DESKTOP_PORT="):
				port = strings.TrimPrefix(item, "RFSWIFT_DESKTOP_PORT=")
			case strings.HasPrefix(item, "RFSWIFT_DESKTOP_SSL="):
				ssl = strings.TrimPrefix(item, "RFSWIFT_DESKTOP_SSL=") == "1"
			}
		}
		if proto != "" && port != "" {
			host = "127.0.0.1"
			if proto == "http" && ssl {
				proto = "https"
			}
			if proto == "vnc" && ssl {
				proto = "vncs"
			}
			m.DesktopURL = proto + "://" + host + ":" + port
		}
	}
	if info.State != nil && info.State.Running {
		m.Status = "up"
	}
	if info.HostConfig != nil {
		m.Net = string(info.HostConfig.NetworkMode)
		m.Caps = append([]string(nil), info.HostConfig.CapAdd...)
		m.Cgroups = append([]string(nil), info.HostConfig.DeviceCgroupRules...)
		if len(m.Cgroups) == 0 && info.Config != nil && info.Config.Labels != nil {
			m.Cgroups = splitNonEmpty(info.Config.Labels["org.rfswift.cgroup_rules"])
		}
		if info.HostConfig.Privileged {
			m.Caps = append(m.Caps, "PRIVILEGED")
		}
	}
	for _, mp := range info.Mounts {
		rw := "ro"
		if mp.RW {
			rw = "rw"
		}
		m.Mounts = append(m.Mounts, mp.Source+" -> "+mp.Destination+" ("+rw+")")
	}
	if info.NetworkSettings != nil {
		for port, binds := range info.NetworkSettings.Ports {
			pub := "-"
			if len(binds) > 0 {
				pub = binds[0].HostIP.String() + ":" + binds[0].HostPort
			}
			m.Ports = append(m.Ports, Port{Port: port.String(), Published: pub, Service: ""})
		}
	}
	return m, nil
}

func splitNonEmpty(value string) []string {
	var out []string
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

// Start starts a stopped container. Nix environments are not long-running, so
// starting one is a no-op.
func (e *LocalEngine) Start(id string) error {
	if isNixEnv(id) {
		return fmt.Errorf("%w: Nix environments have no start/stop lifecycle", ErrNotWired)
	}
	cli, _, err := e.clientFor(id)
	if err != nil {
		return err
	}
	defer cli.Close()
	_, err = cli.ContainerStart(context.Background(), id, client.ContainerStartOptions{})
	return err
}

// Stop stops a running container. No-op for Nix environments.
func (e *LocalEngine) Stop(id string) error {
	if isNixEnv(id) {
		return fmt.Errorf("%w: Nix environments have no start/stop lifecycle", ErrNotWired)
	}
	cli, _, err := e.clientFor(id)
	if err != nil {
		return err
	}
	defer cli.Close()
	_, err = cli.ContainerStop(context.Background(), id, client.ContainerStopOptions{})
	return err
}

// Audit runs the appropriate audit (Nix env or container), writing report.json,
// then parses it into a Posture.
func (e *LocalEngine) Audit(id string) (Posture, error) {
	result, err := e.AuditDetailed(id)
	return result.Posture, err
}

func (e *LocalEngine) AuditDetailed(id string) (AuditResult, error) {
	if isNixEnv(id) {
		out := rfnix.EnvReportDir(id)
		env, envErr := rfnix.GetEnvironment(id)
		if envErr != nil {
			return AuditResult{}, envErr
		}
		auditErr := rfnix.RunAudit(env.FlakeRef, []string{"--env", env.Image, "--format", "json", "--out", out})
		report := filepath.Join(out, "report.json")
		if _, err := os.Stat(report); err != nil {
			if auditErr != nil {
				return AuditResult{}, auditErr
			}
			return AuditResult{}, fmt.Errorf("Nix audit did not produce %s: %w", report, err)
		}
		return parseAuditFile(report), nil
	}
	out, err := os.MkdirTemp("", "rfswift-audit-")
	if err != nil {
		return AuditResult{}, err
	}
	defer os.RemoveAll(out)
	// The audit helper operates on rfdock's global engine; point it at the
	// engine hosting this container first (it may live inside the Lima VM).
	e.RouteMission(id)
	auditErr := rfdock.AuditContainer(id, rfdock.ContainerAuditOptions{OutDir: out, Formats: "json"})
	report := filepath.Join(out, "report.json")
	if _, statErr := os.Stat(report); statErr != nil {
		if auditErr != nil {
			return AuditResult{}, auditErr
		}
		return AuditResult{}, fmt.Errorf("container audit did not produce %s: %w", report, statErr)
	}
	return parseAuditFile(report), nil
}

func parseAuditFile(path string) AuditResult {
	b, err := os.ReadFile(path)
	if err != nil {
		return AuditResult{}
	}
	result := AuditResult{Posture: parsePostureFile(path), Raw: append(json.RawMessage(nil), b...)}
	var root any
	if json.Unmarshal(b, &root) == nil {
		collectSecurityIssues(root, "", &result.Issues)
		appendReportLevelIssues(root, &result.Issues)
		appendImageCVESummary(root, &result.Issues)
	}
	return result
}

// appendImageCVESummary turns a container report's `image_cve` block — Trivy's
// per-severity CVE totals for the container image — into one summary record per
// non-empty severity. Those CVEs feed the posture counts (via the numeric
// critical/high/medium/low fields) but Trivy's full per-CVE list is not inlined,
// so without this the detail list shows only container config findings while the
// posture reports thousands of image CVEs. One row per severity keeps the list
// readable while explaining the numbers; the full CVE list stays in the linked
// Trivy report.
func appendImageCVESummary(root any, out *[]SecurityIssue) {
	m, ok := root.(map[string]any)
	if !ok {
		return
	}
	cve, ok := m["image_cve"].(map[string]any)
	if !ok {
		return
	}
	img := strings.TrimSpace(toStr(cve["image"]))
	if img == "" {
		img = strings.TrimSpace(toStr(m["image"]))
	}
	report := strings.TrimSpace(toStr(cve["report"]))
	for _, sev := range []string{"critical", "high", "medium", "low"} {
		n, ok := asInt(cve[sev])
		if !ok || n <= 0 {
			continue
		}
		evidence := "Per-CVE detail is in the Trivy report."
		if report != "" {
			evidence = "Full CVE list: " + report
		}
		*out = append(*out, SecurityIssue{
			Severity:  sev,
			Title:     fmt.Sprintf("%d %s CVE(s) in the container image", n, sev),
			Component: img,
			Source:    "trivy",
			Scope:     "image",
			Evidence:  evidence,
		})
	}
}

// appendReportLevelIssues surfaces the nix audit's top-level `issues` array —
// human summary strings like "pkgs/ contains N placeholder hashes" or
// "env:X: could not realise closure" — as detail records. parsePostureFile
// counts these strings toward the posture (they set the "N medium" summary), but
// collectSecurityIssues skips them because they are strings, not structured CVE
// objects. Without this the summary shows counts the detail list cannot explain.
func appendReportLevelIssues(root any, out *[]SecurityIssue) {
	m, ok := root.(map[string]any)
	if !ok {
		return
	}
	arr, ok := m["issues"].([]any)
	if !ok {
		return
	}
	// The report gives no per-string severity, only an overall worst_severity;
	// use it so the records match the posture bucketing exactly.
	worst := strings.ToLower(strings.TrimSpace(toStr(m["worst_severity"])))
	if worst == "" {
		worst = "medium"
	}
	for _, item := range arr {
		s, ok := item.(string)
		if !ok {
			continue
		}
		if s = strings.TrimSpace(s); s == "" {
			continue
		}
		*out = append(*out, SecurityIssue{
			Severity: worst,
			Title:    s,
			Source:   "rfswift audit",
			Scope:    "environment",
		})
	}
}

func collectSecurityIssues(value any, inheritedSeverity string, out *[]SecurityIssue) {
	switch v := value.(type) {
	case []any:
		for _, item := range v {
			collectSecurityIssues(item, inheritedSeverity, out)
		}
	case map[string]any:
		severity := firstString(v, "severity", "Severity", "worst_severity")
		if severity == "" {
			severity = inheritedSeverity
		}
		id := firstString(v, "cve", "CVE", "id", "vulnerability", "vulnerability_id")
		title := firstString(v, "title", "message", "name", "description", "summary")
		component := firstString(v, "package", "component", "target", "artifact", "path")
		installed := firstString(v, "installed", "installed_version", "version")
		fixed := firstString(v, "fixed", "fixed_version", "fix_version")
		source := firstString(v, "scanner", "source")
		scope := firstString(v, "scope")
		score := firstString(v, "cvss_score", "score")
		if score == "" {
			for _, key := range []string{"cvss_score", "score"} {
				if number, ok := v[key].(float64); ok {
					score = strconv.FormatFloat(number, 'f', -1, 64)
					break
				}
			}
		}
		disposition := firstString(v, "disposition")
		if severity != "" && (id != "" || title != "" || component != "") {
			evidence, _ := json.Marshal(v)
			*out = append(*out, SecurityIssue{ID: id, Severity: strings.ToLower(severity), Title: title, Component: component, Installed: installed, Fixed: fixed, Source: source, Scope: scope, Score: score, Disposition: disposition, Evidence: string(evidence)})
			return
		}
		for _, child := range v {
			collectSecurityIssues(child, severity, out)
		}
	}
}

func firstString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := m[key]; ok {
			if text := strings.TrimSpace(toStr(value)); text != "" && text != "<nil>" {
				return text
			}
		}
	}
	return ""
}

// Exec runs a non-interactive command inside a container or a native Nix
// environment and returns its combined output.
func (e *LocalEngine) Exec(id, cmd string) (string, error) {
	return e.ExecStream(id, cmd, io.Discard)
}

// ExecStream runs a command and mirrors output to live while retaining the
// complete output for the MCP/tool response.
func (e *LocalEngine) ExecStream(id, cmd string, live io.Writer) (string, error) {
	if live == nil {
		live = io.Discard
	}
	if ev, err := rfnix.GetEnvironment(id); err == nil {
		return execNixEnvironmentStream(ev, cmd, live)
	}
	cli, engType, err := e.clientFor(id)
	if err != nil {
		return "", err
	}
	defer cli.Close()
	ctx := context.Background()
	cr, err := cli.ExecCreate(ctx, id, client.ExecCreateOptions{
		Cmd:          []string{"sh", "-lc", cmd},
		AttachStdout: true,
		AttachStderr: true,
		TTY:          true,
	})
	if err != nil {
		return "", err
	}
	att, err := cli.ExecAttach(ctx, cr.ID, client.ExecAttachOptions{TTY: true})
	if err != nil {
		return "", err
	}
	defer att.Close()
	// Docker's API attaches before starting the exec session. Podman's Docker
	// compatibility API starts it as part of Attach and rejects a second start.
	// Without this distinction Docker returns an immediate empty stream, which
	// made every GUI command appear as "(no output)".
	if engType != rfdock.EnginePodman {
		if _, err := cli.ExecStart(ctx, cr.ID, client.ExecStartOptions{TTY: true}); err != nil {
			return "", fmt.Errorf("start container command: %w", err)
		}
	}
	var captured strings.Builder
	_, readErr := io.Copy(io.MultiWriter(&captured, live), att.Reader)
	if readErr != nil {
		return captured.String(), readErr
	}
	status, err := cli.ExecInspect(ctx, cr.ID, client.ExecInspectOptions{})
	if err != nil {
		return captured.String(), fmt.Errorf("inspect container command: %w", err)
	}
	if status.ExitCode != 0 {
		return captured.String(), fmt.Errorf("command exited with status %d", status.ExitCode)
	}
	return captured.String(), nil
}

func execNixEnvironment(ev *rfnix.Environment, command string) (string, error) {
	return execNixEnvironmentStream(ev, command, io.Discard)
}

func execNixEnvironmentStream(ev *rfnix.Environment, command string, live io.Writer) (string, error) {
	if strings.TrimSpace(command) == "" {
		return "", errors.New("command is required")
	}
	workdir := ev.Workspace
	if workdir == "" {
		workdir, _ = os.Getwd()
	}
	shell, err := exec.LookPath("sh")
	if err != nil {
		return "", err
	}
	var c *exec.Cmd
	if ev.ProfilePath == "" && !ev.Lazy {
		args := []string{"--extra-experimental-features", "nix-command flakes", "develop",
			ev.FlakeRef + "#" + ev.Image, "--ignore-environment", "--command", shell, "-lc", command}
		c = exec.Command(rfnix.NixBinary(), args...)
	} else {
		binDir := filepath.Join(ev.ProfilePath, "bin")
		if ev.Lazy {
			binDir = filepath.Join(rfnix.EnvDir(ev.Name), "bin")
		}
		paths := []string{binDir}
		for _, profile := range []string{rfnix.EnvExtrasProfile(ev.Name), rfnix.SharedExtrasProfile()} {
			if st, statErr := os.Stat(profile); statErr == nil && st.IsDir() {
				paths = append(paths, filepath.Join(profile, "bin"))
			}
		}
		paths = append(paths, os.Getenv("PATH"))
		c = exec.Command(shell, "-lc", command)
		c.Env = append(os.Environ(),
			"PATH="+strings.Join(paths, string(os.PathListSeparator)),
			"RFSWIFT_NIX_ENV="+ev.Name,
			"RFSWIFT_ENGINE=nix",
		)
	}
	c.Dir = workdir
	var captured strings.Builder
	combined := io.MultiWriter(&captured, live)
	c.Stdout, c.Stderr = combined, combined
	runErr := c.Run()
	if runErr != nil {
		return captured.String(), fmt.Errorf("command failed: %w", runErr)
	}
	return captured.String(), nil
}

// --- helpers ---

func mapState(state string) string {
	if strings.EqualFold(state, "running") {
		return "up"
	}
	return "stopped"
}

func boolState(up bool) string {
	if up {
		return "up"
	}
	return "stopped"
}

func isNixEnv(id string) bool {
	_, err := rfnix.GetEnvironment(id)
	return err == nil
}

// parsePostureFile reads an audit report.json and derives crit/high/med/low
// counts. It handles both shapes: dock reports carry numeric
// critical/high/medium/low fields (summed recursively); nix reports carry a
// worst_severity + issues list (bucketed into the worst severity).
func parsePostureFile(path string) Posture {
	b, err := os.ReadFile(path)
	if err != nil {
		return Posture{}
	}
	var root any
	if err := json.Unmarshal(b, &root); err != nil {
		return Posture{}
	}
	var p Posture
	sumSeverityCounts(root, &p)
	if p.Crit+p.High+p.Med+p.Low == 0 {
		// nix-style report: worst_severity + issues
		if m, ok := root.(map[string]any); ok {
			n := 0
			if issues, ok := m["issues"].([]any); ok {
				n = len(issues)
			}
			if n == 0 {
				return p
			}
			switch strings.ToLower(toStr(m["worst_severity"])) {
			case "critical", "crit":
				p.Crit = n
			case "high":
				p.High = n
			case "medium", "med":
				p.Med = n
			default:
				p.Low = n
			}
		}
	}
	return p
}

func sumSeverityCounts(v any, p *Posture) {
	switch t := v.(type) {
	case map[string]any:
		if strings.EqualFold(toStr(t["scope"]), "build-time") || t["runtime_closure"] == false {
			return
		}
		for k, val := range t {
			if strings.EqualFold(k, "severity") {
				switch strings.ToLower(toStr(val)) {
				case "critical", "crit":
					p.Crit++
				case "high":
					p.High++
				case "medium", "med":
					p.Med++
				case "low":
					p.Low++
				}
				continue
			}
			if n, ok := asInt(val); ok {
				switch strings.ToLower(k) {
				case "critical", "crit":
					p.Crit += n
				case "high":
					p.High += n
				case "medium", "med":
					p.Med += n
				case "low":
					p.Low += n
				}
			} else {
				sumSeverityCounts(val, p)
			}
		}
	case []any:
		for _, e := range t {
			sumSeverityCounts(e, p)
		}
	}
}

func asInt(v any) (int, bool) {
	if f, ok := v.(float64); ok {
		return int(f), true
	}
	return 0, false
}

func toStr(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
