package workbench

import (
	"errors"
	"sort"
	"strings"
	"unicode"

	rfnix "penthertz/rfswift/nix"
)

type ToolRecommendation struct {
	Environment string   `json:"environment"`
	Category    string   `json:"category"`
	Description string   `json:"description"`
	Tools       []string `json:"tools"`
	Examples    []string `json:"exampleCommands,omitempty"`
	Missing     []string `json:"knownMissing,omitempty"`
	Score       int      `json:"-"`
}

var toolAdviceAliases = map[string][]string{
	"iq": {"sdr", "signal", "spectrum", "inspectrum", "urh"}, "radio": {"sdr", "signal"},
	"rfid": {"nfc", "mifare", "proxmark", "badge", "card"}, "nfc": {"rfid", "mifare", "proxmark"},
	"firmware": {"reversing", "binary", "ghidra", "binwalk", "hardware"}, "binary": {"reversing", "ghidra", "rizin"},
	"bluetooth": {"ble", "bluez", "ubertooth"}, "ble": {"bluetooth", "ubertooth"},
	"wifi": {"wireless", "802.11", "aircrack", "kismet"}, "can": {"automotive", "can-utils", "savvycan"},
	"android": {"mobile", "apk", "jadx", "frida"}, "network": {"pcap", "nmap", "wireshark"},
	"cellular": {"telecom", "lte", "5g", "gsm"}, "osint": {"recon", "metadata"},
}

var toolAdviceExamples = map[string][]string{
	"rfid":       {"pm3 --list", "pm3 -p /dev/ttyACM0", "nfc-list"},
	"sdr_light":  {"gqrx", "sdrpp", "inspectrum capture.iq", "urh"},
	"sdr_full":   {"gnuradio-companion", "sdrangel", "satdump", "inspectrum capture.iq"},
	"reversing":  {"file firmware.bin", "binwalk firmware.bin", "ghidra", "rizin -A firmware.bin"},
	"hardware":   {"sigrok-cli --scan", "pulseview", "openocd", "flashrom --help"},
	"wifi":       {"iw dev", "airmon-ng", "airodump-ng <interface>", "kismet"},
	"bluetooth":  {"bluetoothctl", "ubertooth-scan", "btmon", "wireshark"},
	"automotive": {"ip link show", "candump can0", "cansend can0 123#DEADBEEF", "savvycan"},
	"network":    {"nmap -sV <authorized-target>", "tcpdump -i <interface>", "wireshark"},
	"android":    {"adb devices", "jadx <application.apk>", "frida-ps -U"},
}

func recommendRFSwiftTools(task, artifact string, limit int) ([]ToolRecommendation, error) {
	query := strings.TrimSpace(task + " " + artifact)
	if query == "" {
		return nil, errors.New("describe the task or artifact")
	}
	if limit < 1 || limit > 10 {
		limit = 4
	}
	terms := adviceTerms(query)
	cat, err := rfnix.LoadCatalog()
	if err != nil {
		return nil, err
	}
	out := make([]ToolRecommendation, 0, len(cat.Environments))
	for _, env := range cat.Environments {
		envText := strings.ToLower(env.Name + " " + env.Category + " " + env.Description)
		score := 0
		matchedTools := make([]string, 0)
		for _, term := range terms {
			if strings.Contains(envText, term) {
				score += 4
			}
		}
		for _, pkg := range env.Packages {
			pkgText := strings.ToLower(pkg)
			matched := false
			for _, term := range terms {
				if len(term) > 1 && strings.Contains(pkgText, term) {
					score += 2
					matched = true
				}
			}
			if matched {
				matchedTools = append(matchedTools, pkg)
			}
		}
		if score == 0 {
			continue
		}
		if len(matchedTools) == 0 {
			matchedTools = append(matchedTools, env.Packages[:min(8, len(env.Packages))]...)
		} else if len(matchedTools) > 12 {
			matchedTools = matchedTools[:12]
		}
		out = append(out, ToolRecommendation{Environment: env.Name, Category: env.Category, Description: env.Description, Tools: matchedTools, Examples: toolAdviceExamples[env.Name], Missing: env.Missing, Score: score})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func adviceTerms(query string) []string {
	base := strings.FieldsFunc(strings.ToLower(query), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '.' })
	seen := map[string]bool{}
	out := make([]string, 0, len(base)*2)
	for _, term := range base {
		if len(term) < 2 || seen[term] {
			continue
		}
		seen[term] = true
		out = append(out, term)
		for key, aliases := range toolAdviceAliases {
			if term != key && !containsString(aliases, term) {
				continue
			}
			for _, expanded := range append([]string{key}, aliases...) {
				if !seen[expanded] {
					seen[expanded] = true
					out = append(out, expanded)
				}
			}
		}
	}
	return out
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
