package workbench

import (
	"errors"
	"path/filepath"
	"strings"
)

// CaptureType is a built-in capture type and the file extensions that map to it.
// The UI can also define custom types; the classifier here covers the built-ins.
type CaptureType struct {
	Key   string   `json:"key"`
	Label string   `json:"label"`
	Exts  []string `json:"exts"`
	Icon  string   `json:"icon,omitempty"`
}

// BuiltinCaptureTypes are the default types files auto-classify into.
func BuiltinCaptureTypes() []CaptureType {
	return []CaptureType{
		{Key: "iq", Label: "IQ recording", Exts: []string{"iq", "cf32", "cfile", "cs8", "cs16", "sigmf-data", "complex"}},
		{Key: "flipper", Label: "Flipper", Exts: []string{"sub", "nfc", "ir", "u2f"}},
		{Key: "proxmark", Label: "Proxmark", Exts: []string{"eml", "mfd", "dump", "rfid", "json"}},
		{Key: "binary", Label: "Binary / firmware", Exts: []string{"bin", "elf", "hex", "img", "fw", "rom"}},
		{Key: "pcap", Label: "Network capture", Exts: []string{"pcap", "pcapng", "cap"}},
		{Key: "doc", Label: "Document", Exts: []string{"md", "txt", "pdf", "csv", "log"}},
		{Key: "terminal", Label: "Terminal recording", Exts: []string{"cast"}},
	}
}

func (s *Store) customCaptureTypesPath(ws string) string {
	return filepath.Join(s.wsDir(ws), "capture-types.json")
}

func (s *Store) LoadCustomCaptureTypes(ws string) []CaptureType {
	var types []CaptureType
	if err := readJSON(s.customCaptureTypesPath(ws), &types); err != nil {
		return []CaptureType{}
	}
	return types
}

func (s *Store) SaveCustomCaptureType(ws string, captureType CaptureType) error {
	if strings.TrimSpace(captureType.Key) == "" || strings.TrimSpace(captureType.Label) == "" || len(captureType.Exts) == 0 {
		return errors.New("capture type requires a key, label and at least one extension")
	}
	types := s.LoadCustomCaptureTypes(ws)
	found := false
	for i := range types {
		if types[i].Key == captureType.Key {
			types[i] = captureType
			found = true
			break
		}
	}
	if !found {
		types = append(types, captureType)
	}
	return writeJSON(s.customCaptureTypesPath(ws), types)
}

// ClassifyCapture returns the capture type key for a filename, by extension.
func ClassifyCapture(filename string) string {
	i := strings.LastIndex(filename, ".")
	if i < 0 {
		return "file"
	}
	ext := strings.ToLower(filename[i+1:])
	for _, t := range BuiltinCaptureTypes() {
		for _, e := range t.Exts {
			if e == ext {
				return t.Key
			}
		}
	}
	return "file"
}
