package remote

import "context"

// CreateRequest is the shared target-creation contract. Context is local only;
// the agent uses the authenticated HTTP request context for cancellation.
type CreateRequest struct {
	Context         context.Context `json:"-"`
	Name            string          `json:"name"`
	Title           string          `json:"title"`
	Engine          string          `json:"engine"` // nix or container
	Image           string          `json:"image"`
	FlakeRef        string          `json:"flakeRef"`
	Workspace       string          `json:"workspace"`
	Network         string          `json:"network"`
	Caps            []string        `json:"caps"`
	Bindings        []string        `json:"bindings"`
	Devices         []string        `json:"devices"`
	ExposedPorts    string          `json:"exposedPorts"`
	PortBindings    string          `json:"portBindings"`
	CgroupRules     []string        `json:"cgroupRules"`
	GPUs            string          `json:"gpus"`
	Seccomp         string          `json:"seccomp"`
	ExtraHosts      []string        `json:"extraHosts"`
	Environment     []string        `json:"environment"`
	Shell           string          `json:"shell"`
	Realtime        bool            `json:"realtime"`
	Desktop         bool            `json:"desktop"`
	DesktopProto    string          `json:"desktopProto"`
	DesktopHost     string          `json:"desktopHost"`
	DesktopPort     string          `json:"desktopPort"`
	DesktopPassword string          `json:"desktopPassword"`
	DesktopSSL      bool            `json:"desktopSSL"`
	NoX11           bool            `json:"noX11"`
	NoAudio         bool            `json:"noAudio"` // do not enable the host audio server for this container
	Privileged      bool            `json:"privileged"`
	Start           bool            `json:"start"`
	Lazy            bool            `json:"lazy"`
	Pure            bool            `json:"pure"`
	Isolate         bool            `json:"isolate"`
}
