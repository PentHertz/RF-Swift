/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
 */

package rfutils

import (
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/FlUxIuS/pulseaudio_2"
)

// PulseTCPModule is the PulseAudio / PipeWire (pipewire-pulse) module that
// accepts container audio over TCP. `rfswift run`, `rfswift host audio
// enable` and the Workbench load it on the host; PULSE_SERVER inside the
// containers points at it.
const PulseTCPModule = "module-native-protocol-tcp"

// pulseModule is one module loaded in the running audio server.
type pulseModule struct {
	Index string
	Name  string
	Args  string
}

// pactlInstallHint names what provides pactl on this host. The native Linux
// packages (deb/rpm/pacman) depend on it; tarball installs may lack it.
func pactlInstallHint() string {
	if runtime.GOOS == "darwin" {
		return "install PulseAudio with: brew install pulseaudio"
	}
	return "install pulseaudio-utils (Debian/Ubuntu/Fedora/openSUSE) or libpulse (Arch), or the rfswift native package which depends on it"
}

// pactlCommand builds a pactl invocation with the environment the host needs
// (Homebrew PulseAudio on macOS looks for its socket through it).
func pactlCommand(args ...string) (*exec.Cmd, error) {
	pactl, err := exec.LookPath("pactl")
	if err != nil {
		return nil, fmt.Errorf("pactl not found: %s", pactlInstallHint())
	}
	cmd := exec.Command(pactl, args...)
	if runtime.GOOS == "darwin" {
		cmd.Env = macOSPulseEnv()
	}
	return cmd, nil
}

// listPulseModules returns the modules loaded in the running audio server:
// through pactl when it is installed (PulseAudio and PipeWire alike), through
// the native PulseAudio protocol otherwise.
//
//	out(1): []pulseModule loaded modules
//	out(2): error when neither path can talk to the server
func listPulseModules() ([]pulseModule, error) {
	if cmd, err := pactlCommand("list", "short", "modules"); err == nil {
		out, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("pactl list short modules: %w", err)
		}
		var mods []pulseModule
		for _, line := range strings.Split(string(out), "\n") {
			fields := strings.SplitN(strings.TrimSpace(line), "\t", 3)
			if len(fields) < 2 || fields[0] == "" {
				continue
			}
			m := pulseModule{Index: fields[0], Name: fields[1]}
			if len(fields) == 3 {
				m.Args = fields[2]
			}
			mods = append(mods, m)
		}
		return mods, nil
	}
	client, err := pulseaudio.NewClient()
	if err != nil {
		return nil, fmt.Errorf("pactl not found and the native PulseAudio connection failed (%v); %s", err, pactlInstallHint())
	}
	defer client.Close()
	list, err := client.ModuleList()
	if err != nil {
		return nil, fmt.Errorf("listing PulseAudio modules: %w", err)
	}
	mods := make([]pulseModule, 0, len(list))
	for _, m := range list {
		mods = append(mods, pulseModule{Index: strconv.FormatUint(uint64(m.Index), 10), Name: m.Name, Args: m.Argument})
	}
	return mods, nil
}

// pulseTCPModules returns the loaded instances of module-native-protocol-tcp.
func pulseTCPModules() ([]pulseModule, error) {
	mods, err := listPulseModules()
	if err != nil {
		return nil, err
	}
	var tcp []pulseModule
	for _, m := range mods {
		if m.Name == PulseTCPModule {
			tcp = append(tcp, m)
		}
	}
	return tcp, nil
}

// pulseModuleListensOn reports whether a module-native-protocol-tcp instance
// binds the given port (the module's own default is 4713 when unspecified).
func pulseModuleListensOn(m pulseModule, port string) bool {
	if port == "" {
		return true
	}
	for _, kv := range strings.Fields(m.Args) {
		if strings.HasPrefix(kv, "port=") {
			return strings.TrimPrefix(kv, "port=") == port
		}
	}
	return port == "4713"
}

// unloadPulseModule unloads one module by index, through pactl or the native
// PulseAudio protocol.
func unloadPulseModule(index string) error {
	if cmd, err := pactlCommand("unload-module", index); err == nil {
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to unload module #%s: %w\nOutput: %s", index, err, strings.TrimSpace(string(out)))
		}
		return nil
	}
	client, err := pulseaudio.NewClient()
	if err != nil {
		return fmt.Errorf("pactl not found and the native PulseAudio connection failed (%v); %s", err, pactlInstallHint())
	}
	defer client.Close()
	idx, err := strconv.ParseUint(index, 10, 32)
	if err != nil {
		return fmt.Errorf("invalid module index %q", index)
	}
	return client.UnloadModule(uint32(idx))
}

// audioSystemName is the human name of the running audio server.
func audioSystemName() string {
	switch detectAudioSystem() {
	case AudioSystemPipeWire:
		return "PipeWire"
	case AudioSystemPulse:
		return "PulseAudio"
	}
	return "audio system"
}

// HostAudioStatus describes the host side of container audio: which audio
// server runs, whether the TCP module the containers connect to is loaded on
// the configured port, and whether that endpoint answers. The Workbench shows
// it (engine doctor, target context menu); `rfswift doctor` runs the
// equivalent checks.
type HostAudioStatus struct {
	Supported  bool   `json:"supported"`  // false where nothing is to be loaded (Windows: WSLg socket)
	System     string `json:"system"`     // pipewire|pulseaudio|wslg|none
	Running    bool   `json:"running"`    // an audio server answers
	PactlFound bool   `json:"pactlFound"` // pactl on PATH (pulseaudio-utils / libpulse)
	Server     string `json:"server"`     // configured PULSE_SERVER, e.g. tcp:localhost:34567
	Port       string `json:"port"`       // TCP port from Server ("" when not a tcp: target)
	Enabled    bool   `json:"enabled"`    // module-native-protocol-tcp loaded on Port
	Reachable  bool   `json:"reachable"`  // a TCP connection to Server succeeded
	Detail     string `json:"detail"`     // one-line summary for humans
}

// GetHostAudioStatus inspects the host audio server for the given container
// PULSE_SERVER target. It never starts or changes anything.
//
//	in(1): string pulseServer configured target (tcp:<host>:<port>, or unix: on WSLg)
//	out: HostAudioStatus
func GetHostAudioStatus(pulseServer string) HostAudioStatus {
	st := HostAudioStatus{Server: pulseServer}
	if runtime.GOOS == "windows" {
		st.System = "wslg"
		if w, err := CheckWSLg(); err == nil && w.Audio {
			st.Running, st.Enabled, st.Reachable = true, true, true
			st.Detail = "WSLg PulseAudio socket: containers use it directly, nothing to enable"
		} else {
			st.Detail = "WSLg PulseAudio socket not found (run wsl --update, then wsl --shutdown)"
		}
		return st
	}
	st.Supported = true
	_, perr := exec.LookPath("pactl")
	st.PactlFound = perr == nil
	switch detectAudioSystem() {
	case AudioSystemPipeWire:
		st.System, st.Running = "pipewire", true
	case AudioSystemPulse:
		st.System, st.Running = "pulseaudio", true
	default:
		st.System = "none"
	}
	parts := strings.Split(pulseServer, ":")
	if len(parts) != 3 || parts[0] != "tcp" {
		st.Detail = fmt.Sprintf("PULSE_SERVER %q is not a tcp:<host>:<port> target; nothing to load on the host", pulseServer)
		return st
	}
	host, port := parts[1], parts[2]
	st.Port = port
	if st.Running {
		if mods, err := pulseTCPModules(); err == nil {
			for _, m := range mods {
				if pulseModuleListensOn(m, port) {
					st.Enabled = true
					break
				}
			}
		}
	}
	if conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 1500*time.Millisecond); err == nil {
		conn.Close()
		st.Reachable = true
	}
	name := audioSystemName()
	switch {
	case !st.Running:
		st.Detail = "no PulseAudio or PipeWire server is running; containers get no sound"
	case st.Enabled:
		st.Detail = fmt.Sprintf("%s: %s loaded, containers reach it on port %s", name, PulseTCPModule, port)
	case st.Reachable:
		st.Detail = fmt.Sprintf("port %s answers, but %s is not loaded in %s (another process owns the port?)", port, PulseTCPModule, name)
	default:
		st.Detail = fmt.Sprintf("%s running, %s not loaded: containers get no sound until it is enabled", name, PulseTCPModule)
	}
	if !st.PactlFound {
		st.Detail += "; pactl is missing (" + pactlInstallHint() + ")"
	}
	return st
}
