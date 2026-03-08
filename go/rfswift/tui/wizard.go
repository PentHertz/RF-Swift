/* This code is part of RF Switch by @Penthertz
 * Author(s): Sebastien Dudek (@FlUxIuS)
 */
package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

// RunWizardResult holds the values collected by the interactive run wizard.
type RunWizardResult struct {
	Image       string
	Name        string
	Desktop     bool
	DesktopSSL  bool
	NoX11       bool
	Privileged  int
	Realtime    bool
	Confirmed   bool
}

// RunWizard launches an interactive form to configure a new container.
// images is a list of available image tags to select from.
// Returns the wizard result or an error if cancelled.
func RunWizard(images []string) (*RunWizardResult, error) {
	if !IsInteractive() {
		return nil, fmt.Errorf("interactive terminal required for wizard mode")
	}

	result := &RunWizardResult{}

	// Step 1: Image selection
	if len(images) == 0 {
		// No images available, ask for manual input
		err := huh.NewInput().
			Title("Image name (e.g., penthertz/rfswift:sdr_full)").
			Value(&result.Image).
			Run()
		if err != nil {
			return nil, err
		}
	} else {
		opts := make([]huh.Option[string], len(images))
		for i, img := range images {
			opts[i] = huh.NewOption(img, img)
		}
		err := huh.NewSelect[string]().
			Title("Select an image").
			Options(opts...).
			Value(&result.Image).
			Run()
		if err != nil {
			return nil, err
		}
	}

	// Step 2: Container name
	err := huh.NewInput().
		Title("Container name").
		Placeholder("my_sdr").
		Value(&result.Name).
		Validate(func(s string) error {
			if strings.TrimSpace(s) == "" {
				return fmt.Errorf("name is required")
			}
			return nil
		}).
		Run()
	if err != nil {
		return nil, err
	}

	// Step 3: Feature toggles
	var features []string
	err = huh.NewMultiSelect[string]().
		Title("Enable features").
		Options(
			huh.NewOption("Remote Desktop (VNC/noVNC)", "desktop"),
			huh.NewOption("Desktop SSL/TLS", "desktop-ssl"),
			huh.NewOption("Disable X11 forwarding", "no-x11"),
			huh.NewOption("Privileged mode", "privileged"),
			huh.NewOption("Realtime mode (audio/SDR)", "realtime"),
		).
		Value(&features).
		Run()
	if err != nil {
		return nil, err
	}

	for _, f := range features {
		switch f {
		case "desktop":
			result.Desktop = true
		case "desktop-ssl":
			result.DesktopSSL = true
		case "no-x11":
			result.NoX11 = true
		case "privileged":
			result.Privileged = 1
		case "realtime":
			result.Realtime = true
		}
	}

	// If desktop SSL is enabled without desktop, enable desktop
	if result.DesktopSSL && !result.Desktop {
		result.Desktop = true
	}

	// Step 4: Recap
	items := map[string]string{
		"Image":     result.Image,
		"Name":      result.Name,
		"Desktop":   boolStr(result.Desktop),
		"SSL/TLS":   boolStr(result.DesktopSSL),
		"X11":       boolStr(!result.NoX11),
		"Privileged": boolStr(result.Privileged == 1),
		"Realtime":  boolStr(result.Realtime),
	}
	keys := []string{"Image", "Name", "Desktop", "SSL/TLS", "X11", "Privileged", "Realtime"}
	PrintRecap("Container Configuration", items, keys)

	// Build equivalent CLI command
	cmd := buildCLICommand(result)
	PrintCLIEquivalent(cmd)

	// Confirm
	result.Confirmed = Confirm("Create this container?")
	return result, nil
}

func boolStr(b bool) string {
	if b {
		return lipgloss.NewStyle().Foreground(ColorSuccess).Render("enabled")
	}
	return lipgloss.NewStyle().Foreground(ColorMuted).Render("disabled")
}

func buildCLICommand(r *RunWizardResult) string {
	parts := []string{"rfswift run"}
	parts = append(parts, fmt.Sprintf("-i %s", r.Image))
	parts = append(parts, fmt.Sprintf("-n %s", r.Name))
	if r.Desktop {
		parts = append(parts, "--desktop")
	}
	if r.DesktopSSL {
		parts = append(parts, "--desktop-ssl")
	}
	if r.NoX11 {
		parts = append(parts, "--no-x11")
	}
	if r.Privileged == 1 {
		parts = append(parts, "-u 1")
	}
	if r.Realtime {
		parts = append(parts, "--realtime")
	}
	return strings.Join(parts, " ")
}
