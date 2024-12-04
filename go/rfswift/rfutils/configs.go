package rfutils

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"path/filepath"
)

type Config struct {
	General struct {
		ImageName string
		RepoTag string
	}
	Container struct {
		Shell      string
		Bindings   []string
		Network    string
		X11Forward string
		XDisplay   string
		ExtraHost  string
		ExtraEnv   string
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
			case "x11forward":
				config.Container.X11Forward = value
			case "xdisplay":
				config.Container.XDisplay = value
			case "extrahost":
				config.Container.ExtraHost = value
			case "extraenv":
				config.Container.ExtraEnv = value
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
	if config.General.ImageName == "" {
		printOrange("Image name is missing in the config file.")
		config.General.ImageName = promptForValue("Image name", "myrfswift:latest")
	}
	if config.General.RepoTag == "" {
		printOrange("Repository tag is missing in the config file.")
		config.General.RepoTag = promptForValue("RepoTag", "penthertz/rfswift")
	}
	if config.Container.Shell == "" {
		printOrange("Shell is missing in the config file.")
		config.Container.Shell = promptForValue("Shell", "/bin/zsh")
	}
	if len(config.Container.Bindings) == 0 {
		printOrange("Bindings are missing in the config file.")
		bindings := promptForValue("Bindings (comma-separated)", "/dev/bus/usb:/dev/bus/usb,/run/dbus/system_bus_socket:/run/dbus/system_bus_socket,/dev/snd:/dev/snd,/dev/dri:/dev/dri")
		config.Container.Bindings = strings.Split(bindings, ",")
	}
	if config.Container.Network == "" {
		printOrange("Network is missing in the config file.")
		config.Container.Network = promptForValue("Network", "host")
	}
	if config.Container.X11Forward == "" {
		printOrange("X11 forwarding is missing in the config file.")
		config.Container.X11Forward = promptForValue("X11 forwarding", "/tmp/.X11-unix:/tmp/.X11-unix")
	}
	if config.Container.XDisplay == "" {
		printOrange("X Display is missing in the config file.")
		config.Container.XDisplay = promptForValue("X Display", "DISPLAY=:0")
	}
	if config.Container.ExtraHost == "" {
		printOrange("Extra host is missing in the config file.")
		config.Container.ExtraHost = promptForValue("Extra host", "pluto.local:192.168.2.1")
	}
	if config.Audio.PulseServer == "" {
		printOrange("PulseAudio server is missing in the config file.")
		config.Audio.PulseServer = promptForValue("PulseAudio server", "tcp:localhost:34567")
	}

	return config, nil
}

func createDefaultConfig(filename string) error {
	content := `[general]
imagename = myrfswift:latest
repotag = penthertz/rfswift

[container]
shell = /bin/zsh
bindings = /dev/bus/usb:/dev/bus/usb,/run/dbus/system_bus_socket:/run/dbus/system_bus_socket,/dev/snd:/dev/snd,/dev/dri:/dev/dri,/dev/input:/dev/input
network = host
x11forward = /tmp/.X11-unix:/tmp/.X11-unix
xdisplay = "DISPLAY=:0"
extrahost = pluto.local:192.168.2.1
extraenv = ""

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
