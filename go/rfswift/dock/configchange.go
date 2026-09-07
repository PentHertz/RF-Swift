/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
*
*  One description of a container configuration change, applied by the CLI,
*  the Workbench and the remote agent alike, and re-run as root when the
*  engine keeps its configuration in files only root can edit (Docker on
*  Linux: hostconfig.json and config.v2.json, then a daemon restart).
 */

package dock

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
)

// ErrNeedsRoot is returned before anything is touched when the change must
// be applied by a root process; callers elevate and retry.
var ErrNeedsRoot = errors.New("this change edits the engine's container files, which needs root")

// ConfigEditMode says how a container's configuration is changed on an
// engine that keeps it in files (Docker on Linux, Docker inside the Lima VM).
// The files are rewritten in place and the daemon restarted: no copy of the
// container, no extra disk, the container comes back as it was. On Linux
// that takes root, so a plain user gets one password prompt. The
// alternative, committing the container and re-creating it, needs no root
// but leaves a snapshot image per change; it is the way on Podman and an
// option elsewhere.
type ConfigEditMode int

const (
	// EditModeAuto: the file edit wherever the engine supports it.
	EditModeAuto ConfigEditMode = iota
	// EditModeDirect: the file edit, explicitly.
	EditModeDirect
	// EditModeRecreate: commit and re-create instead.
	EditModeRecreate
)

var configEditMode = EditModeAuto

// SetConfigEditMode selects how configuration changes are applied.
func SetConfigEditMode(mode ConfigEditMode) { configEditMode = mode }

// useDirectConfigEdit decides for the active engine and mode.
func useDirectConfigEdit() bool {
	if !EngineSupportsDirectConfigEdit() {
		return false
	}
	return configEditMode != EditModeRecreate
}

// ConfigChange is a container configuration change. Kind is one of volume,
// device-bind, device, capability, cgroup, gpu, exposed-port, published-port.
type ConfigChange struct {
	Container string `json:"container"`
	Kind      string `json:"kind"`
	Source    string `json:"source,omitempty"`
	Target    string `json:"target,omitempty"`
	Value     string `json:"value,omitempty"`
	Add       bool   `json:"add"`
	// Mode: "" or "auto", "direct" (edit Docker's files, as root), "recreate".
	Mode string `json:"mode,omitempty"`
	// Engine pins the container engine (docker, podman, lima) so a process
	// re-run as root, with a clean environment, drives the same daemon.
	Engine string `json:"engine,omitempty"`
}

// EditMode is the ConfigEditMode the change asks for.
func (c ConfigChange) EditMode() ConfigEditMode {
	switch strings.ToLower(strings.TrimSpace(c.Mode)) {
	case "direct", "files":
		return EditModeDirect
	case "recreate":
		return EditModeRecreate
	}
	return EditModeAuto
}

// NeedsRootForConfigEdit reports whether applying a change here requires a
// root process: the file edit on Docker/Linux (the files live under
// /var/lib/docker) from a process that is not root. Inside the Lima VM the
// edit goes through the VM's own passwordless sudo, so nothing is needed on
// the host.
func NeedsRootForConfigEdit() bool {
	return runtime.GOOS == "linux" && useDirectConfigEdit() && GetEngine().Type() == EngineDocker && os.Geteuid() != 0
}

// ValidateConfigChange checks a change without touching the container, the
// same way ApplyConfigChange will, so a front end can refuse it before asking
// for a password.
func ValidateConfigChange(c ConfigChange) error {
	if strings.TrimSpace(c.Container) == "" {
		return errors.New("container is required")
	}
	if c.Engine != "" {
		SetPreferredEngine(strings.ToLower(strings.TrimSpace(c.Engine)))
	}
	value := strings.TrimSpace(c.Value)
	switch c.Kind {
	case "volume", "device", "device-bind":
		if strings.TrimSpace(c.Target) == "" {
			return errors.New("both host source and container target are required")
		}
		_, _, err := resolveBindingKind(c.Kind, c.Source, c.Add)
		return err
	case "capability":
		if value == "" {
			return errors.New("capability is required")
		}
	case "cgroup":
		if value == "" {
			return errors.New("cgroup rule is required")
		}
	case "gpu":
		if value == "" {
			return errors.New("GPU selection is required")
		}
	case "exposed-port", "published-port":
		if value == "" {
			return errors.New("port is required")
		}
	case "serial-hotplug":
		return SerialHotplugUnavailableError(GetEngine())
	default:
		return fmt.Errorf("unsupported container setting %q", c.Kind)
	}
	return nil
}

// ApplyConfigChange applies a change with the active engine.
func ApplyConfigChange(c ConfigChange) error {
	if strings.TrimSpace(c.Container) == "" {
		return errors.New("container is required")
	}
	if c.Engine != "" {
		SetPreferredEngine(strings.ToLower(strings.TrimSpace(c.Engine)))
	}
	SetConfigEditMode(c.EditMode())
	value := strings.TrimSpace(c.Value)
	switch c.Kind {
	case "volume", "device", "device-bind":
		return UpdateBinding(c.Container, c.Kind, c.Source, c.Target, c.Add)
	case "capability":
		if value == "" {
			return errors.New("capability is required")
		}
		return UpdateCapability(c.Container, value, c.Add)
	case "cgroup":
		if value == "" {
			return errors.New("cgroup rule is required")
		}
		return UpdateCgroupRule(c.Container, value, c.Add)
	case "gpu":
		if value == "" {
			return errors.New("GPU selection is required")
		}
		return UpdateGPUs(c.Container, value, c.Add)
	case "exposed-port":
		if value == "" {
			return errors.New("port is required")
		}
		return UpdateExposedPort(c.Container, value, c.Add)
	case "published-port":
		if value == "" {
			return errors.New("port binding is required")
		}
		return UpdatePortBinding(c.Container, value, c.Add)
	case "serial-hotplug":
		return UpdateSerialHotplug(c.Container, c.Add)
	default:
		return fmt.Errorf("unsupported container setting %q", c.Kind)
	}
}

// ApplyConfigChangeJSON is the root-side entry point: the change as JSON,
// handed over by the elevating process.
func ApplyConfigChangeJSON(raw string) error {
	var c ConfigChange
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		return fmt.Errorf("invalid container change: %w", err)
	}
	return ApplyConfigChange(c)
}
