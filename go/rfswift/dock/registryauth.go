/* This code is part of RF Swift by @Penthertz
 * Author(s): Sebastien Dudek (@FlUxIuS)
 *
 * Explanation for registry credential rejections during image pulls.
 *
 * RF Swift never sends registry credentials: every pull goes through the
 * engine API with an empty auth header, from the CLI (`rfswift pull`) and the
 * Workbench alike. The engine then presents whatever login it has stored
 * (`podman login` / `docker login`, or ~/.docker/config.json) to Docker Hub,
 * which validates it even for public images. A revoked token or a changed
 * password therefore breaks pulls of public RF Swift images with
 * "unable to retrieve auth token: invalid username/password". The same text
 * also appears during Docker Hub token-service incidents, in which case a
 * retry succeeds.
 */

package dock

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// isRegistryAuthError reports whether err is a registry credential rejection.
func isRegistryAuthError(err error) bool {
	if err == nil {
		return false
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "unable to retrieve auth token") ||
		strings.Contains(lower, "incorrect username or password") ||
		strings.Contains(lower, "invalid username/password")
}

// registryAuthFiles lists the credential files the engine consults for this
// user (rootful Podman and Docker read root's when RF Swift runs with sudo).
func registryAuthFiles() []string {
	var paths []string
	if custom := os.Getenv("REGISTRY_AUTH_FILE"); custom != "" {
		paths = append(paths, custom)
	}
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		runtimeDir = fmt.Sprintf("/run/containers/%d", os.Getuid())
	} else {
		runtimeDir = filepath.Join(runtimeDir, "containers")
	}
	paths = append(paths, filepath.Join(runtimeDir, "auth.json"))
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths,
			filepath.Join(home, ".config", "containers", "auth.json"),
			filepath.Join(home, ".docker", "config.json"),
			filepath.Join(home, ".dockercfg"))
	}
	return paths
}

// fileHasDockerHubCredentials reports whether a Docker/Podman auth file holds
// a login for Docker Hub.
func fileHasDockerHubCredentials(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var parsed struct {
		Auths map[string]struct {
			Auth          string `json:"auth"`
			IdentityToken string `json:"identitytoken"`
		} `json:"auths"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return false
	}
	for registry, entry := range parsed.Auths {
		if strings.Contains(registry, "docker.io") && (entry.Auth != "" || entry.IdentityToken != "") {
			return true
		}
	}
	return false
}

// explainRegistryAuthError leaves unrelated errors untouched and appends,
// for credential rejections, where the stored login comes from and how to
// clear or renew it.
func explainRegistryAuthError(err error) error {
	if !isRegistryAuthError(err) {
		return err
	}
	hint := "Docker Hub rejected the login the container engine presented. RF Swift images are public and RF Swift sends no credentials itself: the engine used a stored login (podman login / docker login) that is no longer valid, or Docker Hub's token service had a temporary problem. Retry once; if it persists, remove or renew the stored login"
	var stored []string
	for _, path := range registryAuthFiles() {
		if fileHasDockerHubCredentials(path) {
			stored = append(stored, path)
		}
	}
	if len(stored) > 0 {
		hint += " (Docker Hub login found in: " + strings.Join(stored, ", ") + ")"
	}
	if engine := GetEngine(); engine != nil {
		switch engine.Type() {
		case EnginePodman:
			hint += ": podman logout docker.io, with sudo for rootful Podman."
		case EngineDocker:
			hint += ": docker logout."
		default:
			hint += "."
		}
	} else {
		hint += "."
	}
	return fmt.Errorf("%w\n%s", err, hint)
}
