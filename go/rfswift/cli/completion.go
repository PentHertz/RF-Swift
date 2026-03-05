/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
 */

package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
	common "penthertz/rfswift/common"
)

var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish|powershell]",
	Short: "Generate and install completion script",
	Long: `Generate and install completion script for rfswift.
To load completions:

Bash:
  $ rfswift completion bash > /etc/bash_completion.d/rfswift
  # or
  $ rfswift completion bash > ~/.bash_completion

Zsh:
  $ rfswift completion zsh > "${fpath[1]}/_rfswift"
  # or
  $ rfswift completion zsh > ~/.zsh/completion/_rfswift

Fish:
  $ rfswift completion fish > ~/.config/fish/completions/rfswift.fish

PowerShell:
  PS> rfswift completion powershell > rfswift.ps1
`,
	ValidArgs: []string{"bash", "zsh", "fish", "powershell"},
	Args:      cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		var shell string
		if len(args) > 0 {
			shell = args[0]
		} else {
			shell = detectShell()
			common.PrintInfoMessage(fmt.Sprintf("Detected shell: %s", shell))
		}

		installCompletion(shell)
	},
}

// detectShell determines the current user's shell by inspecting the SHELL environment
// variable, falling back to platform-appropriate defaults when the variable is unset.
//
//	out: string the detected shell name ("bash", "zsh", "fish", or "powershell")
func detectShell() string {
	shell := os.Getenv("SHELL")
	if shell == "" {
		if runtime.GOOS == "windows" {
			// Default to PowerShell on Windows
			return "powershell"
		}
		// Default to bash
		return "bash"
	}

	// Extract the shell name from the path
	shell = filepath.Base(shell)
	switch shell {
	case "bash", "zsh", "fish":
		return shell
	default:
		return "bash" // Default to bash
	}
}

// installCompletion generates and installs a shell completion script for rfswift into
// the appropriate system or user completion directory for the given shell.
//
//	in(1): string shell the target shell name ("bash", "zsh", "fish", or "powershell")
//	out: none
func installCompletion(shell string) {
	var err error
	var dir string
	var filename string

	fmt.Println("🔍 Finding appropriate completion directory for " + shell + "...")

	switch shell {
	case "bash":
		// Try common bash completion directories
		if runtime.GOOS == "darwin" {
			// macOS often uses homebrew's bash completion
			if _, err := os.Stat("/usr/local/etc/bash_completion.d"); err == nil {
				dir = "/usr/local/etc/bash_completion.d"
			} else {
				// Fallback to user's home directory
				dir = filepath.Join(os.Getenv("HOME"), ".bash_completion.d")
				os.MkdirAll(dir, 0755)
			}
		} else {
			// Linux
			if _, err := os.Stat("/etc/bash_completion.d"); err == nil {
				dir = "/etc/bash_completion.d"
			} else {
				// Fallback to user's home directory
				dir = filepath.Join(os.Getenv("HOME"), ".bash_completion.d")
				os.MkdirAll(dir, 0755)
			}
		}
		filename = "rfswift"

	case "zsh":
		// Try common zsh completion directories
		var zshCompletionDirs []string
		homeDir := os.Getenv("HOME")

		// Check fpath directories
		fpathCmd := exec.Command("zsh", "-c", "echo ${fpath[1]}")
		fpathOutput, err := fpathCmd.Output()
		if err == nil && len(fpathOutput) > 0 {
			zshCompletionDirs = append(zshCompletionDirs, strings.TrimSpace(string(fpathOutput)))
		}

		// Common locations
		zshCompletionDirs = append(zshCompletionDirs,
			filepath.Join(homeDir, ".zsh/completion"),
			filepath.Join(homeDir, ".oh-my-zsh/completions"),
			"/usr/local/share/zsh/site-functions",
			"/usr/share/zsh/vendor-completions",
		)

		// Find first existing directory
		for _, d := range zshCompletionDirs {
			if _, err := os.Stat(d); err == nil {
				dir = d
				common.PrintInfoMessage(fmt.Sprintf("Found existing completion directory: %s", dir))
				break
			}
		}

		// If no directory exists, create one
		if dir == "" {
			dir = filepath.Join(homeDir, ".zsh/completion")
			common.PrintInfoMessage(fmt.Sprintf("Creating completion directory: %s", dir))
			os.MkdirAll(dir, 0755)
		}
		filename = "_rfswift"

	case "fish":
		// Fish completion directory
		dir = filepath.Join(os.Getenv("HOME"), ".config/fish/completions")
		os.MkdirAll(dir, 0755)
		filename = "rfswift.fish"

	case "powershell":
		// PowerShell profile directory
		output, err := exec.Command("powershell", "-Command", "echo $PROFILE").Output()
		if err == nil {
			profileDir := filepath.Dir(strings.TrimSpace(string(output)))
			dir = filepath.Join(profileDir, "CompletionScripts")
		} else {
			dir = filepath.Join(os.Getenv("USERPROFILE"), "Documents", "WindowsPowerShell", "CompletionScripts")
		}
		os.MkdirAll(dir, 0755)
		filename = "rfswift.ps1"

	default:
		common.PrintErrorMessage(fmt.Errorf("Unsupported shell: %s", shell))
		os.Exit(1)
	}

	filepath := filepath.Join(dir, filename)
	fmt.Println("📝 Installing completion script to " + filepath)

	file, err := os.Create(filepath)
	if err != nil {
		if os.IsPermission(err) {
			common.PrintErrorMessage(fmt.Errorf("Permission denied when writing to %s", filepath))
			common.PrintWarningMessage("Try running with sudo or choose a different directory.")
		} else {
			common.PrintErrorMessage(fmt.Errorf("Error creating file: %v", err))
		}
		os.Exit(1)
	}
	defer file.Close()

	// Generate completion script
	common.PrintInfoMessage(fmt.Sprintf("Generating completion script for %s...", shell))

	switch shell {
	case "bash":
		rootCmd.GenBashCompletion(file)
	case "zsh":
		rootCmd.GenZshCompletion(file)
		// Add compdef line at the beginning
		content, err := os.ReadFile(filepath)
		if err == nil {
			newContent := []byte("#compdef rfswift\n" + string(content))
			os.WriteFile(filepath, newContent, 0644)
		}
	case "fish":
		rootCmd.GenFishCompletion(file, true)
	case "powershell":
		rootCmd.GenPowerShellCompletion(file)
	}

	os.Chmod(filepath, 0644)
	common.PrintSuccessMessage(fmt.Sprintf("Completion script installed successfully to %s", filepath))

	// Instructions for shell configuration
	fmt.Println("\n📋 Configuration Instructions:")

	switch shell {
	case "zsh":
		common.PrintInfoMessage("To enable completions, add the following to your ~/.zshrc:")
		fmt.Println("fpath=(" + dir + " $fpath)")
		fmt.Println("autoload -Uz compinit")
		fmt.Println("compinit")
		common.PrintInfoMessage("Then restart your shell or run: source ~/.zshrc")
	case "bash":
		common.PrintInfoMessage("To enable completions, add the following to your ~/.bashrc:")
		fmt.Printf("[[ -f %s ]] && source %s\n", filepath, filepath)
		common.PrintInfoMessage("Then restart your shell or run: source ~/.bashrc")
	case "fish":
		common.PrintSuccessMessage("Completions should be automatically loaded by fish.")
	case "powershell":
		common.PrintInfoMessage("To enable completions, add the following to your PowerShell profile:")
		fmt.Printf(". '%s'\n", filepath)
	}

	fmt.Println("\n🚀 Happy tab-completing with rfswift!")
}

func registerCompletionCommands() {
	rootCmd.AddCommand(completionCmd)
}
