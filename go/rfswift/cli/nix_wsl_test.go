package cli

import (
	"runtime"
	"testing"

	"github.com/spf13/cobra"
)

func TestNixToolsAndSearchCommandPaths(t *testing.T) {
	for _, path := range [][]string{{"nix", "tools"}, {"env", "tools"}, {"nix", "search"}, {"env", "search"}} {
		command, remaining, err := rootCmd.Find(path)
		if err != nil || command == nil || len(remaining) != 0 {
			t.Errorf("command path %v did not resolve: %v %v", path, remaining, err)
		}
	}
	for _, path := range [][]string{{"nix", "list"}, {"nix", "info"}, {"nix", "generations"}, {"nix", "gl"}, {"nix", "udev"}, {"nix", "tools"}, {"nix", "search"}} {
		command, _, err := rootCmd.Find(path)
		if err != nil {
			t.Fatal(err)
		}
		if command.Flags().Lookup("json") == nil {
			t.Errorf("%v has no --json flag", path)
		}
	}
	update, _, _ := rootCmd.Find([]string{"nix", "update"})
	if update.Flags().Lookup("tool") == nil {
		t.Error("nix update has no --tool flag")
	}
	run, _, _ := rootCmd.Find([]string{"run"})
	if run.Flags().Lookup("create-only") == nil {
		t.Error("run has no --create-only flag")
	}
	if rootCmd.Version == "" {
		t.Error("rfswift --version must be available (the WSL front end reads it)")
	}
}

func TestNixRunAcceptsToolOnlyWithFlake(t *testing.T) {
	run, _, err := rootCmd.Find([]string{"nix", "run"})
	if err != nil {
		t.Fatal(err)
	}
	if err := run.Args(run, []string{"gqrx"}); err != nil {
		t.Errorf("a single argument must be accepted (tool only with --flake): %v", err)
	}
	if err := run.Args(run, nil); err == nil {
		t.Error("no argument must be rejected")
	}
}

func TestNixWSLGroupOnlyOnWindows(t *testing.T) {
	command, remaining, err := rootCmd.Find([]string{"nix", "wsl", "status"})
	present := err == nil && command != nil && len(remaining) == 0 && command.Name() == "status"
	if runtime.GOOS == "windows" && !present {
		t.Fatalf("nix wsl status must exist on Windows: %v %v", remaining, err)
	}
	if runtime.GOOS != "windows" && present {
		t.Fatal("nix wsl is a Windows-only group")
	}
	if runtime.GOOS == "windows" {
		if envWSL, _, err := rootCmd.Find([]string{"env", "wsl", "setup"}); err != nil || envWSL.Name() != "setup" {
			t.Fatalf("the env clone must carry the wsl group too: %v", err)
		}
	}
}

func TestShouldBridgeNixToWSL(t *testing.T) {
	nixList, _, _ := rootCmd.Find([]string{"nix", "list"})
	envShell, _, _ := rootCmd.Find([]string{"env", "shell"})
	run, _, _ := rootCmd.Find([]string{"run"})
	exec, _, _ := rootCmd.Find([]string{"exec"})
	install, _, _ := rootCmd.Find([]string{"install"})
	images, _, _ := rootCmd.Find([]string{"images"})
	doctor, _, _ := rootCmd.Find([]string{"doctor"})
	for _, c := range []*cobra.Command{nixList, envShell, run, exec, install} {
		if !shouldBridgeNixToWSL(c) {
			t.Errorf("%s must be served by the Linux rfswift when the Nix engine is selected", c.CommandPath())
		}
	}
	for _, c := range []*cobra.Command{images, doctor, nil} {
		if shouldBridgeNixToWSL(c) {
			name := "nil"
			if c != nil {
				name = c.CommandPath()
			}
			t.Errorf("%s must stay on Windows", name)
		}
	}
	wsl := &cobra.Command{Use: "wsl"}
	status := &cobra.Command{Use: "status"}
	wsl.AddCommand(status)
	nixCmd.AddCommand(wsl)
	defer nixCmd.RemoveCommand(wsl)
	if shouldBridgeNixToWSL(status) || !isNixWSLCommand(status) {
		t.Error("nix wsl subcommands manage the distribution from Windows itself")
	}
}
