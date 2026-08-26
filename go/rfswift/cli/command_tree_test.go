package cli

import (
	"reflect"
	"sort"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func TestCommandPathsRemainCompatible(t *testing.T) {
	paths := [][]string{
		{"container", "create"}, {"container", "shell"}, {"container", "stop"}, {"container", "rm"}, {"container", "install"},
		{"image", "local"}, {"image", "pull"}, {"image", "rm"}, {"image", "build"}, {"image", "tag"}, {"image", "export", "container"},
		{"env", "list"}, {"env", "shell"}, {"env", "enter"}, {"env", "install"}, {"env", "remove"}, {"env", "audit"}, {"env", "gc"},
		{"env", "update"}, {"env", "rebuild"}, {"env", "rollback"}, {"env", "generations"},
		{"run"}, {"create"}, {"exec"}, {"shell"}, {"stop"}, {"halt"},
		{"remove"}, {"rm"}, {"images"}, {"image"}, {"nix"}, {"env"},
		{"agent"}, {"remote"}, {"config", "ports", "bind"}, {"ports", "bind"},
		{"system", "doctor"}, {"doctor"}, {"system", "cleanup", "images"}, {"cleanup", "images"},
	}
	for _, path := range paths {
		command, remaining, err := rootCmd.Find(path)
		if err != nil || command == nil || len(remaining) != 0 {
			t.Errorf("command path %v did not resolve: command=%v remaining=%v err=%v", path, command, remaining, err)
		}
	}
}

func TestLegacyCommandsHaveDeprecationNotice(t *testing.T) {
	for _, path := range [][]string{{"run"}, {"exec"}, {"remove"}, {"stop"}, {"images"}, {"images", "pull"}, {"images", "audit"}, {"nix"}, {"nix", "install"}, {"nix", "audit"}, {"delete"}} {
		command, _, err := rootCmd.Find(path)
		if err != nil {
			t.Fatal(err)
		}
		if command.Deprecated == "" {
			t.Errorf("legacy command %v has no deprecation notice", path)
		}
	}
}

func TestCanonicalAndLegacyFlagsMatch(t *testing.T) {
	pairs := [][][]string{{{"container", "create"}, {"run"}}, {{"container", "shell"}, {"exec"}}, {{"container", "rm"}, {"remove"}}, {{"image", "pull"}, {"images", "pull"}}, {{"env", "install"}, {"nix", "install"}}, {{"env", "audit"}, {"nix", "audit"}}}
	for _, pair := range pairs {
		canonical, _, e1 := rootCmd.Find(pair[0])
		legacy, _, e2 := rootCmd.Find(pair[1])
		if e1 != nil || e2 != nil {
			t.Fatalf("resolve %v: %v %v", pair, e1, e2)
		}
		if got, want := flagNames(canonical), flagNames(legacy); !reflect.DeepEqual(got, want) {
			t.Errorf("flags differ for %v and %v: %v != %v", pair[0], pair[1], got, want)
		}
	}
}

func TestNixUpdateAllowsWizardWithoutName(t *testing.T) {
	for _, path := range [][]string{{"env", "update"}, {"nix", "update"}} {
		command, _, err := rootCmd.Find(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := command.Args(command, nil); err != nil {
			t.Errorf("%v should allow zero args for wizard: %v", path, err)
		}
		if err := command.Args(command, []string{"radio"}); err != nil {
			t.Errorf("%v should allow a named update: %v", path, err)
		}
		if err := command.Args(command, []string{"one", "two"}); err == nil {
			t.Errorf("%v should reject multiple names", path)
		}
	}
}

func flagNames(command *cobra.Command) []string {
	var names []string
	command.LocalNonPersistentFlags().VisitAll(func(flag *pflag.Flag) { names = append(names, flag.Name) })
	sort.Strings(names)
	return names
}
