package workbench

import (
	"context"
	"encoding/json"

	rfdock "penthertz/rfswift/dock"
)

// Data types shared by the store, the engine and the bound API. They are
// JSON-serialisable so Wails can pass them straight to the frontend.

// Port is one network port on a mission's target.
type Port struct {
	Port      string `json:"port"`      // e.g. "8073/tcp"
	Published string `json:"published"` // e.g. "0.0.0.0:8073" or "-"
	Service   string `json:"service"`
}

// Posture is a generic severity count. Environment-audit and mission-finding
// summaries are stored separately and must never be merged.
type Posture struct {
	Crit int `json:"crit"`
	High int `json:"high"`
	Med  int `json:"med"`
	Low  int `json:"low"`
}

type SecurityIssue struct {
	ID          string `json:"id"`
	Severity    string `json:"severity"`
	Title       string `json:"title"`
	Component   string `json:"component"`
	Installed   string `json:"installed"`
	Fixed       string `json:"fixed"`
	Source      string `json:"source"`
	Scope       string `json:"scope"`
	Score       string `json:"score"`
	Disposition string `json:"disposition"`
	Evidence    string `json:"evidence"`
}

type AuditResult struct {
	Posture Posture         `json:"posture"`
	Issues  []SecurityIssue `json:"issues"`
	Raw     json.RawMessage `json:"raw,omitempty"`
}

// Mission is one pentest against one target (a container or a Nix env).
type Mission struct {
	ID               string          `json:"id"`
	Title            string          `json:"title"`
	Engine           string          `json:"engine"` // docker|podman|lima|nix
	Env              string          `json:"env"`
	Image            string          `json:"image"`
	User             string          `json:"user"`
	Caps             []string        `json:"caps"`
	Cgroups          []string        `json:"cgroups"`
	Mounts           []string        `json:"mounts"`
	Net              string          `json:"net"`
	Ports            []Port          `json:"ports"`
	Status           string          `json:"status"` // stopped|starting|up (runtime, from the engine)
	EnvironmentAudit Posture         `json:"environmentAudit"`
	AuditIssues      []SecurityIssue `json:"auditIssues,omitempty"`
	FindingSummary   Posture         `json:"findingSummary"`
	Posture          Posture         `json:"posture,omitempty"` // legacy environment-audit migration only
	Notes            string          `json:"notes"`             // short config note
	DesktopURL       string          `json:"desktopURL"`
	FlakeRef         string          `json:"flakeRef"`
	// Lazy marks an on-demand Nix environment (tools build on first call). It has
	// no eager profile, so rebuild/rollback do not apply to it — only update.
	Lazy    bool `json:"lazy,omitempty"`
	Isolate bool `json:"isolate,omitempty"`
	// HostAudioOff records that this container mission should not load the
	// host audio server's TCP module when it starts (the CLI's `run` and, by
	// default, the Workbench do). Toggled from the target's context menu;
	// persisted in mission.json.
	HostAudioOff bool `json:"hostAudioOff,omitempty"`
	// Warnings lists the adjustments made while creating the target (for
	// example what rootless Podman could not honour). Returned to the caller
	// of CreateMission only; never persisted in mission.json.
	Warnings []string `json:"warnings,omitempty"`
	// Summary is the full container property sheet (what the CLI prints after
	// run/exec). Filled by Inspect for container engines; never persisted.
	Summary *rfdock.ContainerSummary `json:"summary,omitempty"`
}

// MissionCreate is the engine-neutral request accepted from the Workbench UI.
type MissionCreate struct {
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

type ContainerDefaults struct {
	Path         string   `json:"path"`
	Image        string   `json:"image"`
	Shell        string   `json:"shell"`
	Bindings     []string `json:"bindings"`
	Network      string   `json:"network"`
	ExposedPorts string   `json:"exposedPorts"`
	PortBindings string   `json:"portBindings"`
	ExtraHosts   []string `json:"extraHosts"`
	Environment  []string `json:"environment"`
	Devices      []string `json:"devices"`
	Privileged   bool     `json:"privileged"`
	Caps         []string `json:"caps"`
	Seccomp      string   `json:"seccomp"`
	CgroupRules  []string `json:"cgroupRules"`
	DesktopProto string   `json:"desktopProto"`
	DesktopHost  string   `json:"desktopHost"`
	DesktopPort  string   `json:"desktopPort"`
	DesktopPass  string   `json:"desktopPassword"`
	DesktopSSL   bool     `json:"desktopSSL"`
}

type MissionProfile struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	Image        string `json:"image"`
	Network      string `json:"network"`
	ExposedPorts string `json:"exposedPorts"`
	PortBindings string `json:"portBindings"`
	Bindings     string `json:"bindings"`
	Devices      string `json:"devices"`
	Caps         string `json:"caps"`
	Cgroups      string `json:"cgroups"`
	GPUs         string `json:"gpus"`
	Privileged   bool   `json:"privileged"`
	Realtime     bool   `json:"realtime"`
	Desktop      bool   `json:"desktop"`
	DesktopSSL   bool   `json:"desktopSSL"`
	NoX11        bool   `json:"noX11"`
}

type ContainerChange struct {
	Kind   string `json:"kind"`
	Value  string `json:"value"`
	Source string `json:"source"`
	Target string `json:"target"`
	Add    bool   `json:"add"`
}

type MissionTemplate struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"`
}

type ToolCandidate struct {
	Name   string `json:"name"`
	Detail string `json:"detail"`
	Source string `json:"source"`
}

// Note is one markdown document in a mission's notebook.
type Note struct {
	Name    string `json:"name"`
	Body    string `json:"body"`
	Updated string `json:"updated"`
}

// Finding is a security finding, modelled on a pwndoc vulnerability.
type Finding struct {
	ID                    string           `json:"id"`
	Sev                   string           `json:"sev"` // crit|high|med|low
	Title                 string           `json:"title"`
	Target                string           `json:"target"` // mission id / affected scope
	VulnType              string           `json:"vulnType"`
	CVSS                  string           `json:"cvss"` // CVSS v3 vector
	Status                string           `json:"status"`
	Description           string           `json:"description"`
	Observation           string           `json:"observation"`
	Remediation           string           `json:"remediation"`
	References            string           `json:"references"` // one per line
	PoC                   string           `json:"poc"`
	Src                   string           `json:"src"`
	Category              string           `json:"category"`
	Priority              int              `json:"priority"`              // pwndoc: 1 low .. 4 urgent
	RemediationComplexity int              `json:"remediationComplexity"` // pwndoc: 1 low .. 3 high
	CVSSv4                string           `json:"cvssv4"`
	RetestStatus          string           `json:"retestStatus"` // ok|ko|unknown|partial
	RetestDescription     string           `json:"retestDescription"`
	Paragraphs            []map[string]any `json:"paragraphs"`
	CustomFields          []map[string]any `json:"customFields"`
}

// Capture is one artifact an assessment produced (IQ, Flipper, Proxmark,
// pcap, binary, or a custom type), catalogued with metadata.
type Capture struct {
	Mission string            `json:"mission"`
	Type    string            `json:"type"`
	Name    string            `json:"name"`
	Tool    string            `json:"tool"`
	Meta    map[string]string `json:"meta"`
	Note    string            `json:"note"`
	Path    string            `json:"path"` // relative path inside the mission's captures/
}

// WorkspaceArtifact describes a file in the mission's live tool workspace.
// Listing never reads file contents or follows symbolic links.
type WorkspaceArtifact struct {
	Path       string `json:"path"`
	Name       string `json:"name"`
	Size       int64  `json:"size"`
	Modified   string `json:"modified"`
	Type       string `json:"type"`
	Registered bool   `json:"registered"`
	AIAllowed  bool   `json:"aiAllowed"`
}

type ArtifactPreview struct {
	Path      string `json:"path"`
	Kind      string `json:"kind"`
	Content   string `json:"content"`
	Truncated bool   `json:"truncated"`
}
