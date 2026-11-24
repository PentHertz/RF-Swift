/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
 */

package workbench

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Mission RF scope: what an agent may transmit on and target.
//
// The operator writes it in the Workbench (Agent panel > RF scope); it is
// stored as scope.json in the mission directory, outside the notes, so an agent
// can read it (read_scope) but never change it. Once a mission has a scope, the
// RF transmission gate (agent_tx.go) also requires every transmission to state
// its frequency (tx_frequency_hz), which, like any frequency found on the
// command line of a known transmit tool, must lie in one of the allowed bands.
// Hardware addresses (Wi-Fi BSSIDs and clients, Bluetooth/BLE addresses) on a
// transmitting command must be among the in-scope targets when that list is
// not empty, and an excluded address is refused on any agent command.

type FreqBand struct {
	FromHz float64 `json:"fromHz"`
	ToHz   float64 `json:"toHz"`
	Note   string  `json:"note,omitempty"`
}

type MissionScope struct {
	TransmitBands []FreqBand `json:"transmitBands"`
	Targets       []string   `json:"targets"`
	Excluded      []string   `json:"excluded"`
	Note          string     `json:"note,omitempty"`
	Updated       string     `json:"updated,omitempty"`
	// Defined is false when the mission has no scope file (no extra checks).
	Defined bool `json:"defined"`
}

func (s *Store) scopePath(ws, mission string) string {
	return filepath.Join(s.missionDir(ws, mission), "scope.json")
}

func (s *Store) LoadMissionScope(ws, mission string) (MissionScope, error) {
	if err := validateMissionPath(ws, mission); err != nil {
		return MissionScope{}, err
	}
	var scope MissionScope
	err := readJSON(s.scopePath(ws, mission), &scope)
	if errors.Is(err, os.ErrNotExist) {
		return MissionScope{TransmitBands: []FreqBand{}, Targets: []string{}, Excluded: []string{}}, nil
	}
	if err != nil {
		return MissionScope{}, fmt.Errorf("mission scope: %w", err)
	}
	scope.Defined = true
	if scope.TransmitBands == nil {
		scope.TransmitBands = []FreqBand{}
	}
	if scope.Targets == nil {
		scope.Targets = []string{}
	}
	if scope.Excluded == nil {
		scope.Excluded = []string{}
	}
	return scope, nil
}

func (s *Store) SaveMissionScope(ws, mission string, scope MissionScope) error {
	if err := validateMissionPath(ws, mission); err != nil {
		return err
	}
	if err := os.MkdirAll(s.missionDir(ws, mission), 0o700); err != nil {
		return err
	}
	scope.Defined = true
	scope.Updated = time.Now().UTC().Format(time.RFC3339)
	return writeJSON(s.scopePath(ws, mission), scope)
}

func (s *Store) ClearMissionScope(ws, mission string) error {
	if err := validateMissionPath(ws, mission); err != nil {
		return err
	}
	err := os.Remove(s.scopePath(ws, mission))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

var (
	bandPattern = regexp.MustCompile(`^([0-9]+(?:\.[0-9]+)?)([kmg]?)(?:hz)?-([0-9]+(?:\.[0-9]+)?)([kmg]?)(?:hz)?$`)
	macPattern  = regexp.MustCompile(`(?i)(?:^|[^0-9a-f:])((?:[0-9a-f]{2}[:-]){5}[0-9a-f]{2})(?:$|[^0-9a-f:])`)
	macExact    = regexp.MustCompile(`^(?:[0-9a-f]{2}:){5}[0-9a-f]{2}$`)
)

func unitFactor(u string) float64 {
	switch u {
	case "k":
		return 1e3
	case "m":
		return 1e6
	case "g":
		return 1e9
	}
	return 1
}

// ParseFreqBand reads "433.05-434.79 MHz", "2400-2483.5MHz", "868M-868.6M" or
// "433050000-434790000" (Hz), with an optional "# note".
func ParseFreqBand(line string) (FreqBand, error) {
	text, note, _ := strings.Cut(line, "#")
	compact := strings.ToLower(strings.Join(strings.Fields(text), ""))
	m := bandPattern.FindStringSubmatch(compact)
	if m == nil {
		return FreqBand{}, fmt.Errorf("band %q: write it as <from>-<to> <unit>, for example 433.05-434.79 MHz", strings.TrimSpace(line))
	}
	fromUnit, toUnit := m[2], m[4]
	if fromUnit == "" {
		fromUnit = toUnit // "433.05-434.79 MHz": the unit applies to both
	}
	from, _ := strconv.ParseFloat(m[1], 64)
	to, _ := strconv.ParseFloat(m[3], 64)
	band := FreqBand{FromHz: from * unitFactor(fromUnit), ToHz: to * unitFactor(toUnit), Note: strings.TrimSpace(note)}
	if band.FromHz <= 0 || band.ToHz <= band.FromHz || band.ToHz > 300e9 {
		return FreqBand{}, fmt.Errorf("band %q: the upper edge must be above the lower one (and at most 300 GHz)", strings.TrimSpace(line))
	}
	return band, nil
}

// normalizeMAC returns aa:bb:cc:dd:ee:ff, or "" when s is not a 48-bit address.
func normalizeMAC(s string) string {
	s = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(s), "-", ":"))
	if macExact.MatchString(s) {
		return s
	}
	return ""
}

// BuildMissionScope validates the operator's lines into a scope.
func BuildMissionScope(bands, targets, excluded []string, note string) (MissionScope, error) {
	scope := MissionScope{TransmitBands: []FreqBand{}, Targets: []string{}, Excluded: []string{}, Note: strings.TrimSpace(note)}
	for _, line := range bands {
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		band, err := ParseFreqBand(line)
		if err != nil {
			return MissionScope{}, err
		}
		scope.TransmitBands = append(scope.TransmitBands, band)
	}
	sort.Slice(scope.TransmitBands, func(a, b int) bool { return scope.TransmitBands[a].FromHz < scope.TransmitBands[b].FromHz })
	for _, list := range []struct {
		in  []string
		out *[]string
	}{{targets, &scope.Targets}, {excluded, &scope.Excluded}} {
		seen := map[string]bool{}
		for _, line := range list.in {
			text, _, _ := strings.Cut(line, "#")
			if strings.TrimSpace(text) == "" {
				continue
			}
			mac := normalizeMAC(text)
			if mac == "" {
				return MissionScope{}, fmt.Errorf("%q is not a hardware address (aa:bb:cc:dd:ee:ff)", strings.TrimSpace(text))
			}
			if !seen[mac] {
				seen[mac] = true
				*list.out = append(*list.out, mac)
			}
		}
	}
	for _, mac := range scope.Excluded {
		for _, t := range scope.Targets {
			if t == mac {
				return MissionScope{}, fmt.Errorf("%s is both in scope and excluded", mac)
			}
		}
	}
	return scope, nil
}

func formatHz(hz float64) string {
	switch {
	case hz >= 1e9:
		return strconv.FormatFloat(hz/1e9, 'f', -1, 64) + " GHz"
	case hz >= 1e6:
		return strconv.FormatFloat(hz/1e6, 'f', -1, 64) + " MHz"
	case hz >= 1e3:
		return strconv.FormatFloat(hz/1e3, 'f', -1, 64) + " kHz"
	}
	return strconv.FormatFloat(hz, 'f', -1, 64) + " Hz"
}

func (b FreqBand) String() string {
	s := formatHz(b.FromHz) + " - " + formatHz(b.ToHz)
	if b.Note != "" {
		s += " (" + b.Note + ")"
	}
	return s
}

func (scope MissionScope) bandFor(hz float64) (FreqBand, bool) {
	for _, b := range scope.TransmitBands {
		if hz >= b.FromHz && hz <= b.ToHz {
			return b, true
		}
	}
	return FreqBand{}, false
}

func (scope MissionScope) bandList() string {
	if len(scope.TransmitBands) == 0 {
		return "none"
	}
	parts := make([]string, len(scope.TransmitBands))
	for i, b := range scope.TransmitBands {
		parts[i] = b.String()
	}
	return strings.Join(parts, "; ")
}

// Frequencies on the command line of known transmit tools, cross-checked
// against the scope (hackrf_transfer -f Hz, UHD examples --freq, urh_cli -f).
var txFreqPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\bhackrf_transfer\b[^|;&\n]*?\s-f\s*([0-9][0-9.eE+]*)`),
	regexp.MustCompile(`\b(?:tx_samples_from_file|tx_waveforms|tx_bursts|tx_timed_samples|benchmark_rate)\b[^|;&\n]*?--(?:tx[-_])?freq[=\s]+([0-9][0-9.eE+]*)`),
	regexp.MustCompile(`\burh_cli\b[^|;&\n]*?\s(?:-f|--frequency)\s+([0-9][0-9.eE+]*)`),
}

func commandFrequencies(command string) []float64 {
	var out []float64
	for _, re := range txFreqPatterns {
		for _, m := range re.FindAllStringSubmatch(command, -1) {
			if v, err := strconv.ParseFloat(m[1], 64); err == nil && v > 0 && !math.IsInf(v, 0) {
				out = append(out, v)
			}
		}
	}
	return out
}

func commandMACs(command string) []string {
	var out []string
	for _, m := range macPattern.FindAllStringSubmatch(command, -1) {
		if mac := normalizeMAC(m[1]); mac != "" {
			out = append(out, mac)
		}
	}
	return out
}

// checkScope applies the mission scope to an agent command. tx says whether
// it transmits; txFreqHz is the frequency the agent declared (0 if none).
func (s *mcpServer) checkScope(mission, command string, tx bool, txFreqHz float64) error {
	scope, err := s.store.LoadMissionScope(s.opts.Workspace, mission)
	if err != nil {
		return fmt.Errorf("refused: the mission scope cannot be read (%v); fix or clear it in the Workbench", err)
	}
	if !scope.Defined {
		return nil
	}
	macs := commandMACs(command)
	excluded := map[string]bool{}
	for _, m := range scope.Excluded {
		excluded[m] = true
	}
	for _, m := range macs {
		if excluded[m] {
			return fmt.Errorf("refused: %s is excluded from this mission's scope", m)
		}
	}
	if !tx {
		return nil
	}
	if len(scope.TransmitBands) == 0 {
		return errors.New("refused: this mission's scope allows no transmission (no transmit band). The operator can add the approved bands in the Workbench (Agent panel > RF scope)")
	}
	freqs := commandFrequencies(command)
	if txFreqHz > 0 {
		freqs = append(freqs, txFreqHz)
	}
	if txFreqHz <= 0 {
		return fmt.Errorf("refused: this mission has an RF scope, so a transmission must state its centre frequency in tx_frequency_hz. Allowed bands: %s", scope.bandList())
	}
	for _, f := range freqs {
		if _, ok := scope.bandFor(f); !ok {
			return fmt.Errorf("refused: %s is outside the mission's transmit bands (%s)", formatHz(f), scope.bandList())
		}
	}
	if len(scope.Targets) > 0 {
		allowed := map[string]bool{}
		for _, t := range scope.Targets {
			allowed[t] = true
		}
		for _, m := range macs {
			if !allowed[m] {
				return fmt.Errorf("refused: %s is not among the mission's in-scope targets", m)
			}
		}
	}
	return nil
}

// --- Workbench bindings and MCP -------------------------------------------

func (a *App) GetMissionScope(mission string) (MissionScope, error) {
	if err := a.requireMission(mission); err != nil {
		return MissionScope{}, err
	}
	return a.store.LoadMissionScope(a.workspace(), mission)
}

// SaveMissionScope stores the scope from the editor's lines (one band or
// address per line). With clear, the scope is removed instead.
func (a *App) SaveMissionScope(mission string, bands, targets, excluded []string, note string, clear bool) (MissionScope, error) {
	if err := a.requireMission(mission); err != nil {
		return MissionScope{}, err
	}
	if clear {
		if err := a.store.ClearMissionScope(a.workspace(), mission); err != nil {
			return MissionScope{}, err
		}
		return a.store.LoadMissionScope(a.workspace(), mission)
	}
	scope, err := BuildMissionScope(bands, targets, excluded, note)
	if err != nil {
		return MissionScope{}, err
	}
	if err := a.store.SaveMissionScope(a.workspace(), mission, scope); err != nil {
		return MissionScope{}, err
	}
	return a.store.LoadMissionScope(a.workspace(), mission)
}

func (s *mcpServer) callReadScope(mission string) (any, error) {
	scope, err := s.store.LoadMissionScope(s.opts.Workspace, mission)
	if err != nil {
		return nil, err
	}
	bands := make([]map[string]any, 0, len(scope.TransmitBands))
	for _, b := range scope.TransmitBands {
		bands = append(bands, map[string]any{"from_hz": b.FromHz, "to_hz": b.ToHz, "range": b.String()})
	}
	result := map[string]any{"defined": scope.Defined, "transmit_bands": bands, "targets": scope.Targets, "excluded": scope.Excluded}
	if scope.Note != "" {
		result["note"] = scope.Note
	}
	if scope.Defined {
		result["rules"] = "Transmissions need tx_frequency_hz inside a transmit band; addresses on a transmitting command must be in targets when targets is not empty; excluded addresses are refused on every command. Only the operator can change the scope."
	} else {
		result["rules"] = "No RF scope is defined for this mission: transmissions follow only the transmission permission and the operator's approval."
	}
	return result, nil
}
