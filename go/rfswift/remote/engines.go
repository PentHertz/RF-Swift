package remote

// EngineReport is an agent's account of the engines on its own host (control
// method "engines.status"). The Workbench's engine doctor shows it in place
// of the machine the Workbench runs on while a remote connection is active,
// so an empty target list can be explained (daemon down, socket not
// accessible to the agent's user) instead of guessed at.
type EngineReport struct {
	Host    string        `json:"host"` // agent host name
	OS      string        `json:"os"`
	Engines []EngineState `json:"engines"`
	Nix     NixState      `json:"nix"`
}

// EngineState is one container engine as the agent sees it.
type EngineState struct {
	Name       string `json:"name"`  // docker|podman|lima
	Label      string `json:"label"` // display name
	Available  bool   `json:"available"`
	Running    bool   `json:"running"`
	State      string `json:"state"` // running|stopped|not installed|unreachable
	Active     bool   `json:"active"`
	Socket     string `json:"socket"`
	Containers int    `json:"containers"` // RF Swift containers the agent can list, -1 when it could not list
	Detail     string `json:"detail"`     // why the agent cannot use it, when it cannot
}

// NixState is the Nix engine as the agent sees it.
type NixState struct {
	Available bool   `json:"available"`
	Version   string `json:"version"`
	Detail    string `json:"detail"`
}
