package rfutils

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
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
}

const (
	orangeColor = "\033[38;5;208m"
	resetColor  = "\033[0m"
)

func printOrange(message string) {
	fmt.Printf("%s%s%s\n", orangeColor, message, resetColor)
}

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
		config.General.RepoTag = promptForValue("RepoTag", "penthertz/rfswift")
	}
	if config.Container.Shell == "[missing]" {
		printOrange("Shell is missing in the config file.")
		config.Container.Shell = promptForValue("Shell", "/bin/zsh")
	}
	if len(config.Container.Bindings) == 0 {
		printOrange("Bindings are missing in the config file.")
		bindings := promptForValue("Bindings (comma-separated)", "/dev/bus/usb:/dev/bus/usb,/run/dbus/system_bus_socket:/run/dbus/system_bus_socket,/dev/snd:/dev/snd,/dev/dri:/dev/dri,/dev/vhci:/dev/vhci")
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
		config.Container.Devices = promptForValue("Devices", "/dev/snd:/dev/snd,/dev/dri:/dev/dri,/dev/input:/dev/input")
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
		config.Container.Seccomp = promptForValue("Cgroups", "c *:* rmw")
	}

	return config, nil
}

func createDefaultConfig(filename string) error {
	content := `[general]
imagename = myrfswift:latest
repotag = penthertz/rfswift

[container]
shell = /bin/zsh
bindings = /run/dbus/system_bus_socket:/run/dbus/system_bus_socket,/var/run/dbus:/var/run/dbus,/dev/bus/usb:/dev/bus/usb
network = host
exposedports =
portbindings =
x11forward = /tmp/.X11-unix:/tmp/.X11-unix
xdisplay = "DISPLAY=:0"
extrahost = pluto.local:192.168.2.1
extraenv =
devices = /dev/bus/usb:/dev/bus/usb,/dev/snd:/dev/snd,/dev/dri:/dev/dri,/dev/input:/dev/input,/dev/vhci:/dev/vhci,/dev/console:/dev/console,/dev/vcsa:/dev/vcsa,/dev/tty:/dev/tty,/dev/tty0:/dev/tty0,/dev/tty1:/dev/tty1,/dev/tty2:/dev/tty2,/dev/uinput:/dev/uinput
privileged = true
caps = NET_ADMIN
seccomp =
cgroups = c *:* rmw

[audio]
pulse_server = tcp:localhost:34567
`

	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	return os.WriteFile(filename, []byte(content), 0644)
}

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
