package rfutils

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type Config struct {
	General struct {
		ImageName string
		RepoTag   string
	}
	Container struct {
		Shell        string
		Bindings     []string
		Network      string
		ExposedPorts string
		PortBindings string
		X11Forward   string
		XDisplay     string
		ExtraHost    string
		ExtraEnv     string
		Devices      string
		Privileged   string
		Caps         string
		Seccomp      string
		Cgroups      string
	}
	Audio struct {
		PulseServer string
	}
	Desktop struct {
		Proto    string
		Host     string
		Password string
		Port     string
		SSL      string
	}
}

const (
	orangeColor = "\033[38;5;208m"
	resetColor  = "\033[0m"
)

// printOrange prints a message to stdout wrapped in the orange ANSI color code.
//
//	in(1): string message the text to display in orange
func printOrange(message string) {
	fmt.Printf("%s%s%s\n", orangeColor, message, resetColor)
}

// GetDefaultDevices returns a comma-separated list of default device mappings
// appropriate for the current operating system.
//
//	out: string platform-specific comma-separated device mapping string
func GetDefaultDevices() string {
	switch runtime.GOOS {
	case "darwin":
		// On macOS (Docker/OrbStack), /dev/snd and /dev/vhci do not exist.
		// USB devices are available via Lima VM: use 'rfswift macusb attach' to hot-plug.
		return "/dev/bus/usb:/dev/bus/usb,/dev/console:/dev/console"
	case "windows":
		// WSL2/Docker Desktop - fewer device mappings available
		return "/dev/bus/usb:/dev/bus/usb,/dev/snd:/dev/snd,/dev/console:/dev/console,/dev/vcsa:/dev/vcsa,/dev/tty:/dev/tty,/dev/tty0:/dev/tty0,/dev/tty1:/dev/tty1,/dev/tty2:/dev/tty2,/dev/uinput:/dev/uinput"
	default:
		// Linux - full device access
		return "/dev/bus/usb:/dev/bus/usb,/dev/snd:/dev/snd,/dev/dri:/dev/dri,/dev/input:/dev/input,/dev/vhci:/dev/vhci,/dev/console:/dev/console,/dev/vcsa:/dev/vcsa,/dev/tty:/dev/tty,/dev/tty0:/dev/tty0,/dev/tty1:/dev/tty1,/dev/tty2:/dev/tty2,/dev/uinput:/dev/uinput"
	}
}

// ReadOrCreateConfig reads an INI-style configuration file and returns a populated
// Config struct. If the file does not exist, the user is prompted to create one
// with default values. Any missing fields are interactively filled in via stdin.
//
//	in(1): string filename path to the configuration file to read or create
//	out: *Config pointer to the populated Config struct, or nil on error
//	out: error non-nil if the file could not be opened, created, or parsed
func ReadOrCreateConfig(filename string) (*Config, error) {
	config := &Config{}

	if _, err := os.Stat(filename); os.IsNotExist(err) {
		printOrange("Config file not found in your user profile. Would you like to create one with default values? (y/n)")
		reader := bufio.NewReader(os.Stdin)
		response, _ := reader.ReadString('\n')
		if strings.ToLower(strings.TrimSpace(response)) == "y" {
			if err := createDefaultConfig(filename); err != nil {
				return nil, fmt.Errorf("error creating default config: %v", err)
			}
			printOrange("Default config file created.")
		} else {
			return nil, fmt.Errorf("config file not found and user chose not to create one")
		}
	}

	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("error opening file: %v", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	currentSection := ""

	config.General.ImageName = "[missing]"
	config.Container.Shell = "[missing]"
	config.General.RepoTag = "[missing]"
	config.Container.Shell = "[missing]"
	config.Container.Network = "[missing]"
	config.Container.ExposedPorts = "[missing]"
	config.Container.PortBindings = "[missing]"
	config.Container.X11Forward = "[missing]"
	config.Container.XDisplay = "[missing]"
	config.Container.ExtraHost = "[missing]"
	config.Container.ExtraEnv = "[missing]"
	config.Container.Devices = "[missing]"
	config.Container.Privileged = "[missing]"
	config.Container.Caps = "[missing]"
	config.Container.Seccomp = "[missing]"
	config.Container.Cgroups = "[missing]"

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			currentSection = strings.ToLower(line[1 : len(line)-1])
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid config line: %s", line)
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch currentSection {
		case "general":
			switch key {
			case "imagename":
				config.General.ImageName = value
			case "repotag":
				config.General.RepoTag = value
			}
		case "container":
			switch key {
			case "shell":
				config.Container.Shell = value
			case "bindings":
				config.Container.Bindings = strings.Split(value, ",")
			case "network":
				config.Container.Network = value
			case "exposedports":
				config.Container.ExposedPorts = value
			case "portbindings":
				config.Container.PortBindings = value
			case "x11forward":
				config.Container.X11Forward = value
			case "xdisplay":
				config.Container.XDisplay = value
			case "extrahost":
				config.Container.ExtraHost = value
			case "extraenv":
				config.Container.ExtraEnv = value
			case "devices":
				config.Container.Devices = value
			case "privileged":
				config.Container.Privileged = value
			case "caps":
				config.Container.Caps = value
			case "seccomp":
				config.Container.Seccomp = value
			case "cgroups":
				config.Container.Cgroups = value
			}
		case "audio":
			if key == "pulse_server" {
				config.Audio.PulseServer = value
			}
		case "desktop":
			switch key {
			case "proto":
				config.Desktop.Proto = value
			case "host":
				config.Desktop.Host = value
			case "port":
				config.Desktop.Port = value
			case "password":
				config.Desktop.Password = value
			case "ssl":
				config.Desktop.SSL = value
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading file: %v", err)
	}

	// Check for missing values and prompt user
	if config.General.ImageName == "[missing]" {
		printOrange("Image name is missing in the config file.")
		config.General.ImageName = promptForValue("Image name", "myrfswift:latest")
	}
	if config.General.RepoTag == "[missing]" {
		printOrange("Repository tag is missing in the config file.")
		config.General.RepoTag = promptForValue("RepoTag", "penthertz/rfswift_noble")
	}
	if config.Container.Shell == "[missing]" {
		printOrange("Shell is missing in the config file.")
		config.Container.Shell = promptForValue("Shell", "/bin/zsh")
	}
	if len(config.Container.Bindings) == 0 {
		printOrange("Bindings are missing in the config file.")
		bindings := promptForValue("Bindings (comma-separated)", "")
		config.Container.Bindings = strings.Split(bindings, ",")
	}
	if config.Container.Network == "[missing]" {
		printOrange("Network is missing in the config file.")
		config.Container.Network = promptForValue("Network", "host")
	}
	if config.Container.ExposedPorts == "[missing]" {
		printOrange("ExposedPorts is missing in the config file.")
		config.Container.ExposedPorts = promptForValue("ExposedPorts", "")
	}
	if config.Container.PortBindings == "[missing]" {
		printOrange("PortBindings is missing in the config file.")
		config.Container.PortBindings = promptForValue("PortBindings", "")
	}
	if config.Container.X11Forward == "[missing]" {
		printOrange("X11 forwarding is missing in the config file.")
		config.Container.X11Forward = promptForValue("X11 forwarding", "/tmp/.X11-unix:/tmp/.X11-unix")
	}
	if config.Container.XDisplay == "[missing]" {
		printOrange("X Display is missing in the config file.")
		config.Container.XDisplay = promptForValue("X Display", "DISPLAY=:0")
	}
	if config.Container.ExtraHost == "[missing]" {
		printOrange("Extra host is missing in the config file.")
		config.Container.ExtraHost = promptForValue("Extra host", "pluto.local:192.168.2.1")
	}
	if config.Audio.PulseServer == "[missing]" {
		printOrange("PulseAudio server is missing in the config file.")
		config.Audio.PulseServer = promptForValue("PulseAudio server", "tcp:localhost:34567")
	}
	if config.Container.Devices == "[missing]" {
		printOrange("Devices field is missing in the config file.")
		config.Container.Devices = promptForValue("Devices", GetDefaultDevices())
	}
	if config.Container.Privileged == "[missing]" {
		printOrange("Privileged value is missing in the config file.")
		config.Container.Privileged = promptForValue("Privileged mode (true/false)", "true")
	}
	if config.Container.Caps == "[missing]" {
		printOrange("Capabilities are missing in the config file.")
		config.Container.Caps = promptForValue("Capabilities", "SYS_RAWIO,NET_ADMIN,SYS_TTY_CONFIG,SYS_ADMIN")
	}
	if config.Container.Seccomp == "[missing]" {
		printOrange("Seccomp field is missing in the config file.")
		config.Container.Seccomp = promptForValue("Seccomp", "unconfined")
	}
	if config.Container.Cgroups == "[missing]" {
		printOrange("Cgroup field is missing in the config file.")
		config.Container.Cgroups = promptForValue("Cgroups", "c 189:* rwm,c 166:* rwm,c 188:* rwm")
	}

	return config, nil
}

// createDefaultConfig writes a default INI-style configuration file to the given
// path, creating any necessary parent directories. Device defaults are chosen
// based on the current operating system.
//
//	in(1): string filename destination path for the new configuration file
//	out: error non-nil if the directory could not be created or the file could not be written
func createDefaultConfig(filename string) error {
	// Use platform-specific default devices
	defaultDevices := GetDefaultDevices()

	content := fmt.Sprintf(`[general]
imagename = myrfswift:latest
repotag = penthertz/rfswift_noble

[container]
shell = /bin/zsh
bindings =
network = host
exposedports =
portbindings =
x11forward = /tmp/.X11-unix:/tmp/.X11-unix
xdisplay = DISPLAY=:0
extrahost = pluto.local:192.168.2.1
extraenv =
devices = %s
privileged = false
caps =
seccomp =
cgroups = c 189:* rwm,c 166:* rwm,c 188:* rwm

[audio]
pulse_server = tcp:localhost:34567

[desktop]
proto =
host = 127.0.0.1
password =
port = 6080
ssl =
`, defaultDevices)

	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		if !errors.Is(err, os.ErrPermission) {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
		// Permission denied: try with elevated privileges
		if ok := promptForElevation("create config directory"); !ok {
			return fmt.Errorf("cannot create directory %s: permission denied", dir)
		}
		return writeFileElevated(filename, []byte(content), dir)
	}

	if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
		if !errors.Is(err, os.ErrPermission) {
			return err
		}
		if ok := promptForElevation("write config file"); !ok {
			return fmt.Errorf("cannot write %s: permission denied", filename)
		}
		return writeFileElevated(filename, []byte(content), "")
	}
	return nil
}

// promptForElevation asks the user whether to retry an operation with elevated
// privileges and returns true if the user agrees.
func promptForElevation(action string) bool {
	printOrange(fmt.Sprintf("Permission denied to %s. Retry with elevated privileges (sudo)? (y/n)", action))
	reader := bufio.NewReader(os.Stdin)
	response, _ := reader.ReadString('\n')
	return strings.ToLower(strings.TrimSpace(response)) == "y"
}

// writeFileElevated writes content to a file using sudo. If dir is non-empty,
// the directory is created first.
func writeFileElevated(filename string, content []byte, dir string) error {
	if runtime.GOOS == "windows" {
		return fmt.Errorf("elevated write not supported on Windows; please run as Administrator")
	}
	if dir != "" {
		cmd := exec.Command("sudo", "mkdir", "-p", dir)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to create directory with sudo: %w", err)
		}
	}
	cmd := exec.Command("sudo", "tee", filename)
	cmd.Stdin = strings.NewReader(string(content))
	cmd.Stdout = nil // suppress tee's stdout echo
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to write file with sudo: %w", err)
	}
	return nil
}

// promptForValue displays a colored prompt to the user and reads a single line
// from stdin. If the user enters an empty string, the provided default value is
// returned instead.
//
//	in(1): string prompt label text shown to the user before the input cursor
//	in(2): string defaultValue value returned when the user provides no input
//	out: string the user-supplied input, or defaultValue if input was empty
func promptForValue(prompt, defaultValue string) string {
	fmt.Printf("%s%s (default: %s):%s ", orangeColor, prompt, defaultValue, resetColor)
	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input == "" {
		return defaultValue
	}
	return input
}
