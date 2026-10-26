package cli

import (
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	rfdock "penthertz/rfswift/dock"
)

// setCommandFlags sets flags on a command as the command line would,
// including -d, which the bindings group declares for both subcommands.
func setCommandFlags(t *testing.T, cmd *cobra.Command, flags map[string]string) {
	t.Helper()
	cmd.InheritedFlags() // merges the parent's persistent flags into Flags()
	for name, value := range flags {
		flag := cmd.Flags().Lookup(name)
		if flag == nil {
			t.Fatalf("%s has no --%s flag", cmd.Name(), name)
		}
		old := flag.Value.String()
		if err := cmd.Flags().Set(name, value); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = cmd.Flags().Set(name, old) })
	}
}

// A binding the host cannot provide is refused before elevateForConfigEdit
// asks for a password, not by the root process after it.
func TestConfigPreflightChecksBindingsBeforeElevation(t *testing.T) {
	rfdock.SetPreferredEngine("docker")

	t.Run("absent device", func(t *testing.T) {
		setCommandFlags(t, BindingsAddCmd, map[string]string{"container": "lab", "target": "/dev/rfswift-no-such-node"})
		if err := configPreflight(BindingsAddCmd); err == nil || !strings.Contains(err.Error(), "not present") {
			t.Fatalf("expected the absent-device error, got %v", err)
		}
	})
	t.Run("no container", func(t *testing.T) {
		setCommandFlags(t, BindingsAddCmd, map[string]string{"container": "", "target": "/dev/null"})
		if err := configPreflight(BindingsAddCmd); err == nil {
			t.Fatal("a binding without a container must be refused")
		}
	})
	t.Run("usb tree as a device", func(t *testing.T) {
		if _, err := os.Stat("/dev/bus/usb"); err != nil {
			t.Skip("no /dev/bus/usb here")
		}
		setCommandFlags(t, BindingsAddCmd, map[string]string{"container": "lab", "target": "/dev/bus/usb", "devices": "true"})
		if err := configPreflight(BindingsAddCmd); err != nil {
			t.Fatalf("rfswift bindings add -d -t /dev/bus/usb must be accepted: %v", err)
		}
	})
	t.Run("removal", func(t *testing.T) {
		setCommandFlags(t, BindingsRmCmd, map[string]string{"container": "lab", "target": "/dev/rfswift-no-such-node", "devices": "true"})
		if err := configPreflight(BindingsRmCmd); err != nil {
			t.Fatalf("a removal never needs the path to exist: %v", err)
		}
	})
	t.Run("other settings", func(t *testing.T) {
		if err := configPreflight(CapabilitiesAddCmd); err != nil {
			t.Fatalf("only bindings are checked here: %v", err)
		}
	})
}
