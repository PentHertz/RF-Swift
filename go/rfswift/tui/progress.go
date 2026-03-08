/* This code is part of RF Switch by @Penthertz
 * Author(s): Sebastien Dudek (@FlUxIuS)
 */
package tui

import (
	"fmt"
	"strings"
	"sync"

	"github.com/charmbracelet/lipgloss"
)

// LayerProgress tracks per-layer progress for image pulls.
type LayerProgress struct {
	mu     sync.Mutex
	layers map[string]*layerState
	order  []string
}

type layerState struct {
	id      string
	status  string
	current int64
	total   int64
}

// NewLayerProgress creates a new layer progress tracker.
func NewLayerProgress() *LayerProgress {
	return &LayerProgress{
		layers: make(map[string]*layerState),
	}
}

// Update updates the progress of a specific layer.
func (lp *LayerProgress) Update(id, status string, current, total int64) {
	lp.mu.Lock()
	defer lp.mu.Unlock()

	if _, ok := lp.layers[id]; !ok {
		lp.layers[id] = &layerState{id: id}
		lp.order = append(lp.order, id)
	}

	l := lp.layers[id]
	l.status = status
	l.current = current
	l.total = total
}

// Render returns a string representation of all layer progress.
func (lp *LayerProgress) Render() string {
	lp.mu.Lock()
	defer lp.mu.Unlock()

	var sb strings.Builder
	barWidth := 30

	for _, id := range lp.order {
		l := lp.layers[id]
		shortID := id
		if len(shortID) > 12 {
			shortID = shortID[:12]
		}

		var line string
		switch l.status {
		case "Downloading", "Extracting":
			pct := float64(0)
			if l.total > 0 {
				pct = float64(l.current) / float64(l.total)
			}
			filled := int(pct * float64(barWidth))
			bar := lipgloss.NewStyle().Foreground(ColorPrimary).Render(strings.Repeat("█", filled)) +
				lipgloss.NewStyle().Foreground(ColorMuted).Render(strings.Repeat("░", barWidth-filled))
			sizeMB := float64(l.current) / 1024 / 1024
			totalMB := float64(l.total) / 1024 / 1024
			line = fmt.Sprintf("  %s  %s %s %.1f/%.1fMB",
				shortID, bar, l.status, sizeMB, totalMB)
		case "Pull complete", "Already exists":
			check := lipgloss.NewStyle().Foreground(ColorSuccess).Render("✓")
			line = fmt.Sprintf("  %s  %s %s", shortID, check, l.status)
		case "Waiting":
			dot := lipgloss.NewStyle().Foreground(ColorMuted).Render("⋯")
			line = fmt.Sprintf("  %s  %s %s", shortID, dot, l.status)
		default:
			line = fmt.Sprintf("  %s  %s", shortID, l.status)
		}

		sb.WriteString(line + "\n")
	}

	return sb.String()
}

// Done returns the number of completed layers.
func (lp *LayerProgress) Done() int {
	lp.mu.Lock()
	defer lp.mu.Unlock()
	count := 0
	for _, l := range lp.layers {
		if l.status == "Pull complete" || l.status == "Already exists" {
			count++
		}
	}
	return count
}

// Total returns the total number of layers being tracked.
func (lp *LayerProgress) Total() int {
	lp.mu.Lock()
	defer lp.mu.Unlock()
	return len(lp.layers)
}
