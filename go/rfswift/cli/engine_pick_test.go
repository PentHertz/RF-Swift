package cli

import (
	"testing"

	"github.com/spf13/cobra"
	common "penthertz/rfswift/common"
	"penthertz/rfswift/rfutils"
)

func engineTestCmd(t *testing.T) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "run"}
	cmd.Flags().String("engine", "auto", "")
	cmd.Flags().Bool("gpu", false, "")
	return cmd
}

func TestEngineChosen(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SUDO_USER", "")
	t.Setenv("RFSWIFT_ENGINE", "")

	if engineChosen(engineTestCmd(t)) {
		t.Fatal("nothing chosen, but engineChosen is true")
	}

	cmd := engineTestCmd(t)
	_ = cmd.Flags().Set("engine", "podman")
	if !engineChosen(cmd) {
		t.Error("--engine was ignored")
	}

	cmd = engineTestCmd(t)
	_ = cmd.Flags().Set("gpu", "true")
	if !engineChosen(cmd) {
		t.Error("--gpu (Lima GPU VM) was ignored")
	}

	t.Setenv("RFSWIFT_ENGINE", "docker")
	if !engineChosen(engineTestCmd(t)) {
		t.Error("RFSWIFT_ENGINE was ignored")
	}
	t.Setenv("RFSWIFT_ENGINE", "")

	cfg := common.ConfigFileByPlatform()
	if err := rfutils.SetConfigValue(cfg, "general", "engine", "auto"); err != nil {
		t.Fatal(err)
	}
	if engineChosen(engineTestCmd(t)) {
		t.Error("engine = auto in the config must still ask")
	}
	if err := rfutils.SetConfigValue(cfg, "general", "engine", "nix"); err != nil {
		t.Fatal(err)
	}
	if !engineChosen(engineTestCmd(t)) {
		t.Error("a configured default engine was ignored")
	}
}

func TestEngineSetDefault(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SUDO_USER", "")
	cfg := common.ConfigFileByPlatform()
	for _, v := range []string{"podman", "NIX", "auto"} {
		engineSetDefaultCmd.Run(engineSetDefaultCmd, []string{v})
		want := map[string]string{"podman": "podman", "NIX": "nix", "auto": "auto"}[v]
		if got := rfutils.ConfiguredEngine(cfg); got != want {
			t.Fatalf("engine set %s stored %q, want %q", v, got, want)
		}
	}
}
