package rfutils

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/cheggaaa/pb/v3"
	"github.com/go-resty/resty/v2"
	"golang.org/x/term"
)

type Release struct {
	TagName string `json:"tag_name"`
}

// GetLatestRelease queries the GitHub Releases API for the most recent release
// of the given repository and returns a Release value containing its tag name.
// It gives up after 2 seconds: it runs before every command (the version
// notice), which must not hang on a slow network.
//
//	in(1): string owner  GitHub organisation or user name that owns the repository
//	in(2): string repo   repository name
//	out: Release  the latest release metadata (currently only TagName is populated)
//	out: error    non-nil if the HTTP request fails or the response cannot be decoded
func GetLatestRelease(owner string, repo string) (Release, error) {
	return getLatestRelease(owner, repo, 2*time.Second)
}

// getLatestRelease is GetLatestRelease with a caller-chosen timeout
// (`rfswift update` waits longer than the startup notice).
func getLatestRelease(owner, repo string, timeout time.Duration) (Release, error) {
	client := resty.New().
		SetTimeout(timeout).
		SetRetryCount(0)

	resp, err := client.R().
		SetHeader("Accept", "application/vnd.github.v3+json").
		Get(fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", owner, repo))

	if err != nil {
		return Release{}, err
	}

	if resp.StatusCode() != http.StatusOK {
		return Release{}, fmt.Errorf("failed to get latest release: %s", resp.Status())
	}

	var release Release
	if err := json.Unmarshal(resp.Body(), &release); err != nil {
		return Release{}, err
	}

	return release, nil
}

// ConstructDownloadURL builds the GitHub release asset download URL from its
// constituent parts.
//
//	in(1): string owner     GitHub organisation or user name
//	in(2): string repo      repository name
//	in(3): string tag       release tag (e.g. "v1.2.3")
//	in(4): string fileName  asset file name within the release
//	out: string  fully-qualified HTTPS download URL for the asset
func ConstructDownloadURL(owner, repo, tag, fileName string) string {
	return fmt.Sprintf("https://github.com/%s/%s/releases/download/%s/%s", owner, repo, tag, fileName)
}

// DownloadFile fetches the file at url and writes it to the local path dest,
// displaying an animated progress bar in the terminal while the transfer is
// in progress.
//
//	in(1): string url   the HTTP/HTTPS URL of the remote file to download
//	in(2): string dest  local filesystem path where the file should be written
//	out: error  non-nil if the download, file creation, or copy fails
func DownloadFile(url, dest string) error {
	// No overall deadline (a large file on a slow link is fine), but give up
	// on a server that never answers.
	client := &http.Client{Transport: &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		TLSHandshakeTimeout:   30 * time.Second,
		ResponseHeaderTimeout: 60 * time.Second,
	}}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download file: %s", resp.Status)
	}

	// -1 when the server sends no Content-Length: the bar then counts bytes
	// without a total.
	size := resp.ContentLength

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	// No bar when stdout is not a terminal (logs, pipes): pb then writes
	// every refresh on the same line, without carriage returns.
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		_, err = io.Copy(out, resp.Body)
		return err
	}

	bar := pb.Full.Start64(size)
	bar.Set(pb.Bytes, true)
	go func() {
		colors := []string{"\033[31m", "\033[32m", "\033[33m", "\033[34m", "\033[35m", "\033[36m"}
		for i := 0; bar.IsStarted(); i++ {
			// {{percent}} already ends with "%"; reset the color afterwards
			// so it does not leak into the next line.
			bar.SetTemplateString(fmt.Sprintf("%s{{counters . }} {{bar . }} {{percent . }}\033[0m", colors[i%len(colors)]))
			time.Sleep(100 * time.Millisecond)
		}
	}()

	barReader := bar.NewProxyReader(resp.Body)
	_, err = io.Copy(out, barReader)
	bar.Finish()

	return err
}

// VersionCompare compares two semantic version strings and returns an integer
// indicating their relative order. Leading "v" prefixes are stripped before
// comparison. Pre-release suffixes (e.g. "-rc1") are handled so that a stable
// release is considered newer than its pre-release counterparts.
//
//	in(1): string v1  first version string (e.g. "1.2.3" or "v1.2.3-rc1")
//	in(2): string v2  second version string (e.g. "1.2.3" or "v1.2.3-rc2")
//	out: int  1 if v1 > v2, -1 if v1 < v2, 0 if v1 == v2
func VersionCompare(v1, v2 string) int {
	// Strip 'v' prefix if present
	v1 = strings.TrimPrefix(v1, "v")
	v2 = strings.TrimPrefix(v2, "v")

	// Split version and pre-release parts
	v1Parts := strings.SplitN(v1, "-", 2)
	v2Parts := strings.SplitN(v2, "-", 2)

	// Compare main version numbers
	parts1 := strings.Split(v1Parts[0], ".")
	parts2 := strings.Split(v2Parts[0], ".")

	for i := 0; i < len(parts1) && i < len(parts2); i++ {
		p1, _ := strconv.Atoi(parts1[i])
		p2, _ := strconv.Atoi(parts2[i])

		if p1 > p2 {
			return 1
		}
		if p1 < p2 {
			return -1
		}
	}

	if len(parts1) > len(parts2) {
		return 1
	}
	if len(parts1) < len(parts2) {
		return -1
	}

	// Main versions are equal, compare pre-release tags
	hasPrerelease1 := len(v1Parts) > 1
	hasPrerelease2 := len(v2Parts) > 1

	// If only one has pre-release, the one without is newer
	if !hasPrerelease1 && hasPrerelease2 {
		return 1 // v1 is newer (stable > pre-release)
	}
	if hasPrerelease1 && !hasPrerelease2 {
		return -1 // v2 is newer
	}

	// Both have pre-release tags, compare them
	if hasPrerelease1 && hasPrerelease2 {
		prerelease1 := v1Parts[1]
		prerelease2 := v2Parts[1]

		// Extract rc number if format is "rcX"
		if strings.HasPrefix(prerelease1, "rc") && strings.HasPrefix(prerelease2, "rc") {
			rc1Str := strings.TrimPrefix(prerelease1, "rc")
			rc2Str := strings.TrimPrefix(prerelease2, "rc")

			rc1, err1 := strconv.Atoi(rc1Str)
			rc2, err2 := strconv.Atoi(rc2Str)

			// If both are valid rc numbers, compare numerically
			if err1 == nil && err2 == nil {
				if rc1 > rc2 {
					return 1
				}
				if rc1 < rc2 {
					return -1
				}
				return 0
			}
		}

		// Fallback to string comparison for other pre-release formats
		if prerelease1 > prerelease2 {
			return 1
		}
		if prerelease1 < prerelease2 {
			return -1
		}
	}

	return 0
}
