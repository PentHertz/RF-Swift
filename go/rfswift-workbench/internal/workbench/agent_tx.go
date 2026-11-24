/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
 */

package workbench

import (
	"fmt"
	"regexp"
	"strings"
)

// RF transmission gate for MCP agents.
//
// Receiving is free; radiating is not. An agent command that transmits (SDR
// TX, replay, jamming, Wi-Fi/802.15.4/BLE injection, rogue access points,
// cellular base stations or UEs) is refused unless the operator enabled
// "Allow RF transmission" for the agent (--mcp-tx). With it enabled the agent
// must still declare the transmission (transmits: true) and describe it
// (frequency, power or gain, duration, target) in purpose, which is what the
// operator sees in the AI tool tab and the audit trail.
//
// Detection is a list of patterns for the transmit modes of the tools RF Swift
// ships. Text matching cannot see inside a script or a GNU Radio flowgraph, so
// the declaration is the primary signal and the patterns are the safety net;
// typed tool recipes carry an exact transmits flag.

type txRule struct {
	name    string
	pattern *regexp.Regexp
}

// cmdPos matches a program name in command position: at the start, after
// whitespace, a separator or a path, and followed by whitespace or the end,
// so "cat gnb.yaml" or "/etc/hostapd/hostapd.conf" do not match.
func cmdPos(names string) *regexp.Regexp {
	return regexp.MustCompile(`(^|[\s;&|(/])(` + names + `)(\s|$)`)
}

var txRules = []txRule{
	{"HackRF transmit or carrier (hackrf_transfer -t/-c)", regexp.MustCompile(`\bhackrf_transfer\b[^|;&\n]*\s-[a-zA-Z0-9]*[tc](\s|$)`)},
	{"UHD transmit example (tx_samples_from_file, tx_waveforms, ...)", cmdPos(`tx_samples_from_file|tx_waveforms|tx_bursts|tx_timed_samples|txrx_loopback_to_file`)},
	{"UHD benchmark with TX (benchmark_rate --tx_rate)", regexp.MustCompile(`\bbenchmark_rate\b[^|;&\n]*--tx_rate\b`)},
	{"bladeRF transmit (bladeRF-cli tx)", regexp.MustCompile(`(?i)\bbladerf-cli\b[^|;&\n]*\btx\s+(start|config)\b`)},
	{"rpitx suite", cmdPos(`rpitx|sendiq|pifmrds|pichirp|pisstv|piopera|pirtty`)},
	{"URH send/transmit", regexp.MustCompile(`\burh_cli\b[^|;&\n]*(\s-tx\b|--transmit\b)`)},
	{"rfcat transmit (RFxmit)", regexp.MustCompile(`\bRFxmit(Long)?\b`)},
	{"Wi-Fi injection (aireplay-ng)", cmdPos(`aireplay-ng`)},
	{"Wi-Fi flooding/deauthentication (mdk3/mdk4)", cmdPos(`mdk3|mdk4`)},
	{"Rogue or lab access point (airbase-ng, hostapd, eaphammer, wifiphisher, ...)", cmdPos(`airbase-ng|hostapd|hostapd-mana|hostapd-wpe|eaphammer|wifiphisher|wifipumpkin3?|fluxion|berate_ap|create_ap|wifite`)},
	{"hcxdumptool (active by default)", cmdPos(`hcxdumptool`)},
	{"bettercap injection module", regexp.MustCompile(`\b(wifi\.deauth|wifi\.assoc|wifi\.ap|ble\.write|hid\.inject)\b`)},
	{"BLE jamming or hijacking (btlejack -j/-t)", regexp.MustCompile(`\bbtlejack\b[^|;&\n]*(\s-[a-zA-Z]*[jt](\s|$)|--jam\b|--hijack\b)`)},
	{"802.15.4 injection (killerbee zbreplay, zbstumbler, flooders)", cmdPos(`zbreplay|zbstumbler|zbwardrive|zbassocflood|zbfakebeacon|zbpanidconflictflood`)},
	{"2.4 GHz keystroke injection (JackIt, mousejack)", cmdPos(`jackit|mousejack`)},
	{"Cellular base station or UE (srsRAN, OCUDU, OAI, osmocom, YateBTS, OpenBTS)", cmdPos(`srsenb|srsue|srsgnb|gnb|lte-softmodem|nr-softmodem|lte-uesoftmodem|nr-uesoftmodem|osmo-bts-trx|osmo-trx-[a-z0-9]+|osmo-bts-[a-z0-9]+|yatebts|openbts|OpenBTS|transceiver52M`)},
}

// detectTransmission returns the first rule the command matches, or "".
func detectTransmission(command string) string {
	for _, r := range txRules {
		if r.pattern.MatchString(command) {
			return r.name
		}
	}
	return ""
}

// checkTransmit decides whether a command may run. declared is the agent's
// transmits flag (or a recipe's); it returns whether the run is a
// transmission and an error that explains a refusal to the agent.
func (s *mcpServer) checkTransmit(command string, declared bool, purpose string) (bool, error) {
	rule := detectTransmission(command)
	if !declared && rule == "" {
		return false, nil
	}
	why := "the request declares an RF transmission"
	if rule != "" {
		why = "it matches the transmission rule \"" + rule + "\""
	}
	if !s.opts.AllowTransmit {
		return true, fmt.Errorf("refused: %s, and RF transmission is disabled for this agent. Receive-only work needs no permission. To transmit, ask the operator to enable \"Allow RF transmission\" in the Workbench Agent settings and to approve this specific transmission (frequency, power or gain, duration, target); then run it again with transmits: true and that description in purpose. Licensed bands (cellular, GNSS, aviation) are only ever transmitted into a shielded enclosure or over attenuated coax", why)
	}
	if !declared {
		return true, fmt.Errorf("refused: %s but was not declared. Run it again with transmits: true and a purpose stating the frequency, power or gain, duration and target, once the operator has explicitly approved this transmission", why)
	}
	if len(strings.TrimSpace(purpose)) < 12 {
		return true, fmt.Errorf("refused: a transmission needs a purpose stating the frequency, power or gain, duration and target the operator approved")
	}
	return true, nil
}
