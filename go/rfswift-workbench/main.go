// RF Swift Workbench - a research and assessment GUI for RF Swift.
//
// Separate binary and module from the rfswift CLI (see go.mod). It is a client:
// it connects to a local or remote rfswift agent and drives the same engines
// (docker/podman/lima/nix) the CLI uses. See docs/workspace-gui.md and
// docs/remote-agent.md.
package main

import (
	"embed"
	"flag"
	"fmt"
	"os"

	"penthertz/rfswift-workbench/internal/workbench"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	mcpMode := flag.Bool("mcp", false, "run the optional RF Swift MCP server over stdio")
	mcpWorkspace := flag.String("workspace", "default", "Workbench workspace exposed in MCP mode")
	mcpMission := flag.String("mission", "", "restrict MCP access to one mission")
	mcpWrite := flag.Bool("mcp-write", false, "allow MCP clients to change notes and findings")
	mcpExec := flag.Bool("mcp-exec", false, "allow MCP clients to execute mission commands")
	flag.Parse()
	if *mcpMode {
		if err := workbench.RunMCPServer(os.Stdin, os.Stdout, workbench.MCPOptions{Workspace: *mcpWorkspace, Mission: *mcpMission, AllowWrite: *mcpWrite, AllowExec: *mcpExec}); err != nil {
			fmt.Fprintln(os.Stderr, "rfswift-workbench mcp:", err)
			os.Exit(1)
		}
		return
	}
	app := workbench.NewApp(assets)
	err := wails.Run(&options.App{
		Title:  "RF Swift Workbench",
		Width:  1320,
		Height: 860,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup:  app.Startup,
		OnShutdown: app.Shutdown,
		Bind: []interface{}{
			app,
		},
	})
	if err != nil {
		println("rfswift-workbench:", err.Error())
	}
}
