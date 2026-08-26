package dock

import (
	"os"
	"path/filepath"
	"strings"
)

const containerXAuthority = "/tmp/.rfswift-xauthority"

// forwardedXAuthority returns the host cookie file needed by an SSH-forwarded
// X11 display. Local Unix-socket displays authenticate through xhost instead.
func forwardedXAuthority(display string) (string, bool) {
	display = strings.TrimPrefix(strings.TrimSpace(display), "DISPLAY=")
	lower := strings.ToLower(display)
	if !(strings.HasPrefix(lower, "localhost:") || strings.HasPrefix(lower, "127.0.0.1:") || strings.HasPrefix(lower, "[::1]:")) {
		return "", false
	}
	candidates := []string{strings.TrimSpace(os.Getenv("XAUTHORITY"))}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, ".Xauthority"))
	}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, true
		}
	}
	return "", false
}

func addForwardedXAuthority(display string, binds, env []string) ([]string, []string) {
	authority, ok := forwardedXAuthority(display)
	if !ok {
		return binds, env
	}
	binds = append(binds, authority+":"+containerXAuthority+":ro")
	env = append(env, "XAUTHORITY="+containerXAuthority)
	return binds, env
}
