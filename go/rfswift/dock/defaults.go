package dock

import (
	"strings"

	common "penthertz/rfswift/common"
	"penthertz/rfswift/rfutils"
)

// CreationDefaults is the platform-specific config.ini base used by the CLI.
type CreationDefaults struct {
	Path         string
	Image        string
	Shell        string
	Bindings     []string
	Network      string
	ExposedPorts string
	PortBindings string
	X11Forward   string
	XDisplay     string
	ExtraHosts   []string
	Environment  []string
	Devices      []string
	Privileged   bool
	Caps         []string
	Seccomp      string
	CgroupRules  []string
	DesktopProto string
	DesktopHost  string
	DesktopPort  string
	DesktopPass  string
	DesktopSSL   bool
}

func configList(value string) []string {
	var out []string
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

// LoadCreationDefaults reads the same platform config.ini used by rfswift.
func LoadCreationDefaults() (CreationDefaults, error) {
	path := common.ConfigFileByPlatform()
	if err := rfutils.EnsureDefaultConfig(path); err != nil {
		return CreationDefaults{}, err
	}
	cfg, err := rfutils.ReadOrCreateConfig(path)
	if err != nil {
		return CreationDefaults{}, err
	}
	bindings := configList(strings.Join(cfg.Container.Bindings, ","))
	env := configList(cfg.Container.ExtraEnv)
	return CreationDefaults{Path: path, Image: cfg.General.ImageName, Shell: cfg.Container.Shell,
		Bindings: bindings, Network: cfg.Container.Network, ExposedPorts: cfg.Container.ExposedPorts,
		PortBindings: cfg.Container.PortBindings, X11Forward: cfg.Container.X11Forward,
		XDisplay: cfg.Container.XDisplay, ExtraHosts: configList(cfg.Container.ExtraHost),
		Environment: env, Devices: configList(cfg.Container.Devices),
		Privileged: strings.EqualFold(cfg.Container.Privileged, "true"), Caps: configList(cfg.Container.Caps),
		Seccomp: cfg.Container.Seccomp, CgroupRules: configList(cfg.Container.Cgroups),
		DesktopProto: cfg.Desktop.Proto, DesktopHost: cfg.Desktop.Host, DesktopPort: cfg.Desktop.Port,
		DesktopPass: cfg.Desktop.Password, DesktopSSL: strings.EqualFold(cfg.Desktop.SSL, "true")}, nil
}
