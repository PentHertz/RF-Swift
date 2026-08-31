package workbench

import (
	"context"
	"errors"
	"io/fs"
	"net"
	"net/http"
	"sync"

	"penthertz/rfswift/common"
	rfdock "penthertz/rfswift/dock"
	"penthertz/rfswift/remote"
)

// App is the Wails-bound application object. Every exported method on it is
// callable from the frontend as window.go.workbench.App.<Method>().
type App struct {
	ctx             context.Context
	store           *Store
	eng             Engine
	ws              string // current workspace
	termMu          sync.Mutex
	terminals       map[string]*terminalSession
	createMu        sync.Mutex
	createCancel    context.CancelFunc
	creationContext context.Context
	createName      string
	secretStore     remote.SecretStore
	chefMu          sync.Mutex
	chefServer      *http.Server
	chefListener    net.Listener
	chefBaseURL     string
	assets          fs.FS
	usbMu           sync.Mutex
	usbAttached     map[string]bool // QMP device IDs we forwarded into the Lima VM this session
}

// GetAppVersion exposes the CLI's canonical version to the Workbench so both
// binaries are changed from one declaration in common.Version.
func (a *App) GetAppVersion() string { return common.Version }

func (a *App) requireMission(id string) error {
	if !validWorkspaceName(id) {
		return errors.New("invalid mission scope")
	}
	missions, err := a.store.ListMissions(a.ws)
	if err != nil {
		return err
	}
	for _, mission := range missions {
		if mission.ID == id {
			return nil
		}
	}
	return errors.New("mission is outside the open Workbench project")
}

// NewApp wires the on-disk store and the local engine. Remote agents attach
// through a different Engine implementation (docs/remote-agent.md).
func NewApp(assetFS ...fs.FS) *App {
	app := &App{
		store:       NewStore(""),
		eng:         NewLocalEngine(),
		ws:          "default",
		terminals:   make(map[string]*terminalSession),
		secretStore: remote.OSSecretStore{},
	}
	if len(assetFS) > 0 {
		app.assets = assetFS[0]
	}
	return app
}

// Startup initialises the Wails application lifecycle.
func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
	// Ensure the default workspace exists so the UI has somewhere to write.
	_ = a.store.CreateWorkspace(a.ws)
	_ = a.store.SecurePermissions()
	// Surface container-engine lifecycle state (e.g. a Lima VM booting) in the
	// GUI. Without this, starting a stopped Lima VM during mission creation is
	// invisible — the dialog sits still for up to ~30s before any image pull
	// progress appears, and the user cannot tell whether anything is happening.
	rfdock.SetEngineStatusReporter(a.emitEngineStatus)
}

// emitEngineStatus forwards a container-engine lifecycle update to the GUI. When
// a mission is being created it rides that mission's progress channel so the
// state (e.g. "starting the Lima VM") shows on the creation dialog; otherwise it
// is emitted untargeted for any engine strip listening for it.
func (a *App) emitEngineStatus(stage string, running bool) {
	a.createMu.Lock()
	target := a.createName
	a.createMu.Unlock()
	percent := 12
	if running {
		percent = 25
	}
	a.emitOperationProgress("mission-create", target, percent, stage)
}

// Shutdown releases background services created by the Workbench.
func (a *App) Shutdown(ctx context.Context) {
	a.chefMu.Lock()
	server := a.chefServer
	a.chefServer = nil
	a.chefListener = nil
	a.chefBaseURL = ""
	a.chefMu.Unlock()
	if server != nil {
		_ = server.Shutdown(ctx)
	}
}
