package dock

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	rfutils "penthertz/rfswift/rfutils"
	common "penthertz/rfswift/common"
	"penthertz/rfswift/tui"
)

type RepoVersionMap map[string]ImageVersionMap

type Tag struct {
	Name          string    `json:"name"`
	Images        []Image   `json:"images"`
	TagLastPushed time.Time `json:"tag_last_pushed"`
	FullSize      int64     `json:"full_size"`
}

type Image struct {
	Architecture string `json:"architecture"`
	Digest       string `json:"digest"`
}

type TagList struct {
	Results []Tag `json:"results"`
}

type DockerHubTag struct {
	Name        string `json:"name"`
	LastUpdated string `json:"last_updated"`
	FullSize    int64  `json:"full_size"`
	MediaType   string `json:"media_type"`
	Digest      string `json:"digest"`
}

type DockerHubResponse struct {
	Count    int            `json:"count"`
	Next     string         `json:"next"`
	Previous string         `json:"previous"`
	Results  []DockerHubTag `json:"results"`
}

// VersionInfo holds version information for an image
type VersionInfo struct {
	Version string
	Digest  string
	Date    time.Time
}

// ImageVersionMap maps base image names to their versions
type ImageVersionMap map[string][]VersionInfo

// getArchitecture returns the current system architecture as a normalised string
// suitable for Docker Hub tag filtering.
//
//	out: string architecture identifier ("amd64", "arm64", "riscv64", "arm") or empty string if unsupported
func getArchitecture() string {
	switch runtime.GOARCH {
	case "amd64":
		return "amd64"
	case "arm64":
		return "arm64"
	case "riscv64":
		return "riscv64"
	case "arm":
		return "arm"
	default:
		return ""
	}
}

// GetAllRemoteVersionsByRepo fetches all remote image versions grouped by repository
// for the given architecture, logging a warning and skipping any repo that fails.
//
//	in(1): string architecture target architecture identifier (e.g. "amd64")
//	out: RepoVersionMap map of repository name to its ImageVersionMap
func GetAllRemoteVersionsByRepo(architecture string) RepoVersionMap {
	allVersions := make(RepoVersionMap)

	for _, repo := range OfficialRepos() {
		repoVersions, err := GetRemoteVersionsForRepo(repo, architecture)
		if err != nil {
			log.Printf("Warning: Error getting versions for %s: %v", repo, err)
			continue
		}
		allVersions[repo] = repoVersions
	}

	return allVersions
}

// FormatVersionsMultiLine formats a slice of VersionInfo into multiple display lines,
// deduplicating version strings and wrapping when the per-line count or character
// width limit is exceeded.
//
//	in(1): []VersionInfo versions list of version metadata to format
//	in(2): int maxPerLine maximum number of version strings per output line
//	in(3): int maxWidth maximum character width per line (0 disables width limiting)
//	out: []string list of formatted lines, or []string{"-"} when versions is empty
func FormatVersionsMultiLine(versions []VersionInfo, maxPerLine int, maxWidth int) []string {
	if len(versions) == 0 {
		return []string{"-"}
	}

	// Deduplicate version strings while preserving order
	seen := make(map[string]bool)
	var uniqueVersions []string

	for _, v := range versions {
		if !seen[v.Version] {
			seen[v.Version] = true
			uniqueVersions = append(uniqueVersions, v.Version)
		}
	}

	if len(uniqueVersions) == 0 {
		return []string{"-"}
	}

	var lines []string
	var currentLine strings.Builder
	countOnLine := 0

	for i, v := range uniqueVersions {
		separator := ""
		if countOnLine > 0 {
			separator = ", "
		}

		testLen := currentLine.Len() + len(separator) + len(v)

		// Start new line if we exceed count or width
		if countOnLine >= maxPerLine || (maxWidth > 0 && testLen > maxWidth && countOnLine > 0) {
			if currentLine.Len() > 0 {
				lines = append(lines, currentLine.String())
			}
			currentLine.Reset()
			countOnLine = 0
			separator = ""
		}

		if countOnLine > 0 {
			currentLine.WriteString(", ")
		}
		currentLine.WriteString(v)
		countOnLine++

		// Last item
		if i == len(uniqueVersions)-1 && currentLine.Len() > 0 {
			lines = append(lines, currentLine.String())
		}
	}

	if len(lines) == 0 {
		return []string{"-"}
	}

	return lines
}

// FindVersionInRemote searches all official repositories for a specific version of the
// named image and returns the repository, digest, clean tag name, and any error encountered.
//
//	in(1): string imageName image name, optionally prefixed with a repo and colon
//	in(2): string version semver version string to locate (e.g. "1.2.0")
//	in(3): string architecture target architecture identifier (e.g. "amd64")
//	out: string repo official repository that contains the version
//	out: string digest content-addressable digest of the matching tag
//	out: string tag cleaned base image name (architecture and repo prefix stripped)
//	out: error non-nil if the version cannot be found or a network error occurs
func FindVersionInRemote(imageName string, version string, architecture string) (repo string, digest string, tag string, err error) {
	allVersions := GetAllRemoteVersions(architecture)

	// Clean the image name (remove repo prefix if present)
	cleanImageName := imageName
	for _, officialRepo := range OfficialRepos() {
		if strings.HasPrefix(imageName, officialRepo+":") {
			cleanImageName = strings.TrimPrefix(imageName, officialRepo+":")
			break
		}
	}

	// Also try without architecture suffix
	cleanImageName = removeArchitectureSuffix(cleanImageName)

	// Search in the version map
	if versions, ok := allVersions[cleanImageName]; ok {
		for _, v := range versions {
			if v.Version == version {
				// Found the version, now find which repo has it
				for _, officialRepo := range OfficialRepos() {
					repoVersions, err := GetRemoteVersionsForRepo(officialRepo, architecture)
					if err != nil {
						continue
					}
					if repoVers, ok := repoVersions[cleanImageName]; ok {
						for _, rv := range repoVers {
							if rv.Version == version {
								return officialRepo, rv.Digest, cleanImageName, nil
							}
						}
					}
				}
			}
		}
	}

	return "", "", "", fmt.Errorf("version %s not found for image %s", version, imageName)
}

// ListAvailableVersions fetches and prints all available remote image versions for
// the current architecture, optionally narrowed to images whose name contains filterImage.
//
//	in(1): string filterImage case-insensitive substring filter; pass "" to list all images
func ListAvailableVersions(filterImage string) {
	architecture := getArchitecture()
	if architecture == "" {
		common.PrintErrorMessage(fmt.Errorf("unsupported architecture: %s", runtime.GOARCH))
		return
	}

	common.PrintInfoMessage(fmt.Sprintf("Fetching versions for architecture: %s", architecture))

	remoteVersions := GetAllRemoteVersions(architecture)

	if len(remoteVersions) == 0 {
		common.PrintInfoMessage("No version information available")
		return
	}

	// Sort image names for consistent output
	var imageNames []string
	for name := range remoteVersions {
		// Apply filter
		if filterImage != "" && !strings.Contains(strings.ToLower(name), strings.ToLower(filterImage)) {
			continue
		}
		imageNames = append(imageNames, name)
	}
	sort.Strings(imageNames)

	if len(imageNames) == 0 {
		common.PrintInfoMessage("No images found matching filter")
		return
	}

	// Print header
	cyan := "\033[36m"
	yellow := "\033[33m"
	green := "\033[32m"
	reset := "\033[0m"

	fmt.Printf("\n%s📋 Available Versions%s\n", cyan, reset)
	fmt.Printf("%s───────────────────────────────────────────────────────────%s\n\n", cyan, reset)

	for _, imageName := range imageNames {
		versions := remoteVersions[imageName]
		if len(versions) == 0 {
			continue
		}

		fmt.Printf("%s📦 %s%s\n", yellow, imageName, reset)

		for _, v := range versions {
			shortDigest := v.Digest
			if len(shortDigest) > 12 {
				shortDigest = shortDigest[:12] + "..."
			}

			dateStr := v.Date.Format("2006-01-02")

			if v.Version == "latest" {
				fmt.Printf("   %s• %-12s%s  (digest: %s, date: %s)\n",
					green, v.Version, reset, shortDigest, dateStr)
			} else {
				fmt.Printf("   • %-12s  (digest: %s, date: %s)\n",
					v.Version, shortDigest, dateStr)
			}
		}
		fmt.Println()
	}

	// Print usage hint
	fmt.Printf("%sUsage:%s\n", cyan, reset)
	fmt.Printf("  Pull specific version: rfswift images pull -i <image> -V <version>\n")
	fmt.Printf("  Example: rfswift images pull -i sdr_full -V 1.2.0\n\n")
}

// showLoadingIndicatorWithReturn runs commandFunc in a goroutine while displaying a
// spinning clock-emoji indicator labelled with stepName, then returns the error
// produced by commandFunc once it completes.
//
//	in(1): func() error commandFunc function to execute asynchronously
//	in(2): string stepName label displayed next to the loading indicator
//	out: error error returned by commandFunc, or nil on success
func showLoadingIndicatorWithReturn(commandFunc func() error, stepName string) error {
	spinner := tui.NewSpinner(stepName)
	spinner.Start()
	err := commandFunc()
	spinner.Stop()
	return err
}

// determineArchitectureFromTag infers the architecture from known tag suffixes
// (_amd64, _arm64, _riscv64). When no suffix matches it falls back to requestedArch,
// defaulting to "amd64" when requestedArch is empty.
//
//	in(1): string tagName Docker Hub tag name to inspect
//	in(2): string requestedArch fallback architecture when no suffix is present
//	out: string resolved architecture string
func determineArchitectureFromTag(tagName, requestedArch string) string {
	if strings.HasSuffix(tagName, "_amd64") {
		return "amd64"
	}
	if strings.HasSuffix(tagName, "_arm64") {
		return "arm64"
	}
	if strings.HasSuffix(tagName, "_riscv64") {
		return "riscv64"
	}

	if requestedArch == "amd64" || requestedArch == "" {
		return "amd64"
	}

	return requestedArch
}

// OfficialRepos returns the list of official RF-Swift Docker Hub repository names.
//
//	out: []string slice of fully-qualified repository names (e.g. "penthertz/rfswift_noble")
func OfficialRepos() []string {
	return []string{"penthertz/rfswift_noble"}
}

// IsOfficialImage reports whether imageName refers to one of the official RF-Swift
// repositories, stripping a leading "docker.io/" prefix before comparison.
//
//	in(1): string imageName fully-qualified or short image name to check
//	out: bool true if the image belongs to an official repository
func IsOfficialImage(imageName string) bool {
	// Podman uses fully qualified names (docker.io/penthertz/...)
	cleaned := strings.TrimPrefix(imageName, "docker.io/")
	for _, repo := range OfficialRepos() {
		if strings.HasPrefix(cleaned, repo+":") {
			return true
		}
	}
	return false
}

// normalizeTagForRemote ensures tag has an architecture suffix required for Docker Hub
// lookups. If the tag already ends with a known suffix it is returned unchanged;
// otherwise "_<architecture>" is appended.
//
//	in(1): string tag Docker Hub tag name to normalise
//	in(2): string architecture architecture identifier to append when missing
//	out: string tag name guaranteed to carry an architecture suffix
func normalizeTagForRemote(tag, architecture string) string {
	suffixes := []string{"_amd64", "_arm64", "_riscv64", "_arm"}
	for _, suffix := range suffixes {
		if strings.HasSuffix(tag, suffix) {
			return tag
		}
	}
	return fmt.Sprintf("%s_%s", tag, architecture)
}

// parseTagVersion extracts the base image name and optional semver version string
// from a Docker Hub tag name, stripping any architecture suffix first.
//
//	in(1): string tagName raw Docker Hub tag name (e.g. "sdr_full_1.2.0_amd64")
//	out: string baseName base image name with architecture and version removed
//	out: string version semver string (e.g. "1.2.0"), or "" when absent
func parseTagVersion(tagName string) (baseName string, version string) {
	// Remove architecture suffix first
	cleanTag := removeArchitectureSuffix(tagName)

	// Check for version pattern (semver-like)
	// Pattern: name_X.Y.Z or name_vX.Y.Z
	versionPattern := regexp.MustCompile(`^(.+?)_v?(\d+\.\d+\.\d+)$`)
	matches := versionPattern.FindStringSubmatch(cleanTag)

	if len(matches) == 3 {
		return matches[1], matches[2]
	}

	return cleanTag, ""
}

// compareVersions performs a numeric component-wise comparison of two semver strings.
//
//	in(1): string v1 first semver string (e.g. "1.2.3")
//	in(2): string v2 second semver string (e.g. "1.3.0")
//	out: int 1 if v1 > v2, -1 if v1 < v2, 0 if equal
func compareVersions(v1, v2 string) int {
	parts1 := strings.Split(v1, ".")
	parts2 := strings.Split(v2, ".")

	for i := 0; i < 3; i++ {
		var n1, n2 int
		if i < len(parts1) {
			fmt.Sscanf(parts1[i], "%d", &n1)
		}
		if i < len(parts2) {
			fmt.Sscanf(parts2[i], "%d", &n2)
		}

		if n1 > n2 {
			return 1
		}
		if n1 < n2 {
			return -1
		}
	}
	return 0
}

// sortVersionInfos sorts a slice of VersionInfo in-place, placing the "latest" entry
// first and ordering all remaining entries by semver descending.
//
//	in(1): []VersionInfo versions slice to sort in-place
func sortVersionInfos(versions []VersionInfo) {
	sort.Slice(versions, func(i, j int) bool {
		if versions[i].Version == "latest" {
			return true
		}
		if versions[j].Version == "latest" {
			return false
		}
		return compareVersions(versions[i].Version, versions[j].Version) > 0
	})
}

// GetVersionForDigest returns the non-"latest" version string whose digest matches
// the provided digest, or an empty string if no match is found.
//
//	in(1): []VersionInfo versions list of version metadata to search
//	in(2): string digest content-addressable digest to look up
//	out: string matching version string, or "" when not found
func GetVersionForDigest(versions []VersionInfo, digest string) string {
	for _, v := range versions {
		if v.Digest == digest && v.Version != "latest" {
			return v.Version
		}
	}
	return ""
}

// GetVersionForDigests returns the non-"latest" version string that matches any one
// of the provided digests, or an empty string if no match is found.
//
//	in(1): []VersionInfo versions list of version metadata to search
//	in(2): []string digests candidate digests to match against
//	out: string matching version string, or "" when not found
func GetVersionForDigests(versions []VersionInfo, digests []string) string {
	for _, v := range versions {
		if v.Version != "latest" && digestMatches(digests, v.Digest) {
			return v.Version
		}
	}
	return ""
}

// FormatVersionsString formats a slice of VersionInfo into a comma-separated string,
// deduplicating version labels and truncating to maxVersions entries with a "+N" suffix
// indicating how many additional versions were omitted.
//
//	in(1): []VersionInfo versions list of version metadata to format
//	in(2): int maxVersions maximum number of versions to include (0 means unlimited)
//	out: string comma-separated version string, or "-" when versions is empty
func FormatVersionsString(versions []VersionInfo, maxVersions int) string {
	if len(versions) == 0 {
		return "-"
	}

	// Deduplicate version strings while preserving order
	seen := make(map[string]bool)
	var uniqueVersions []string
	
	for _, v := range versions {
		if !seen[v.Version] {
			seen[v.Version] = true
			uniqueVersions = append(uniqueVersions, v.Version)
		}
	}

	// Apply maxVersions limit
	var versionStrs []string
	for i, v := range uniqueVersions {
		if maxVersions > 0 && i >= maxVersions {
			remaining := len(uniqueVersions) - maxVersions
			if remaining > 0 {
				versionStrs = append(versionStrs, fmt.Sprintf("+%d", remaining))
			}
			break
		}
		versionStrs = append(versionStrs, v)
	}

	return strings.Join(versionStrs, ", ")
}

// GetRemoteImageDigest fetches the content-addressable digest for a specific tag
// from Docker Hub, normalising the tag name with an architecture suffix before querying.
//
//	in(1): string repo Docker Hub repository (e.g. "penthertz/rfswift_noble")
//	in(2): string tag image tag name, with or without architecture suffix
//	in(3): string architecture target architecture used to normalise the tag
//	out: string digest of the matching tag, or "" when not found
//	out: error non-nil if the HTTP request fails or the tag cannot be located
func GetRemoteImageDigest(repo, tag, architecture string) (string, error) {
	var digest string

	// Normalize tag to include architecture suffix
	normalizedTag := normalizeTagForRemote(tag, architecture)

	err := showLoadingIndicatorWithReturn(func() error {
		url := fmt.Sprintf("https://hub.docker.com/v2/repositories/%s/tags/?page_size=100", repo)
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Get(url)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusNotFound {
			return fmt.Errorf("tag not found")
		} else if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("failed to get tags: %s", resp.Status)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}

		var response DockerHubResponse
		if err := json.Unmarshal(body, &response); err != nil {
			return err
		}

		for _, hubTag := range response.Results {
			if hubTag.Name == normalizedTag {
				if strings.HasPrefix(hubTag.Name, "cache_") {
					continue
				}
				if hubTag.MediaType != "application/vnd.oci.image.index.v1+json" {
					continue
				}

				digest = hubTag.Digest
				return nil
			}
		}

		return fmt.Errorf("tag not found")
	}, fmt.Sprintf("Checking Docker Hub for '%s' (%s)", tag, architecture))

	return digest, err
}

// GetRemoteVersionsForRepo fetches and parses all image tags from a single Docker Hub
// repository, returning a map from base image name to its sorted list of VersionInfo entries.
//
//	in(1): string repo Docker Hub repository to query (e.g. "penthertz/rfswift_noble")
//	in(2): string architecture target architecture to filter tags by
//	out: ImageVersionMap map of base image name to sorted []VersionInfo
//	out: error non-nil if the Docker Hub request fails
func GetRemoteVersionsForRepo(repo string, architecture string) (ImageVersionMap, error) {
	versions := make(ImageVersionMap)

	tags, err := getLatestDockerHubTags(repo, architecture)
	if err != nil {
		return nil, err
	}

	for _, tag := range tags {
		baseName, version := parseTagVersion(tag.Name)

		digest := ""
		if len(tag.Images) > 0 {
			digest = tag.Images[0].Digest
		}

		info := VersionInfo{
			Version: version,
			Digest:  digest,
			Date:    tag.TagLastPushed,
		}

		// If no version, it's the "latest" tag
		if version == "" {
			info.Version = "latest"
		}

		versions[baseName] = append(versions[baseName], info)
	}

	// Sort versions for each image
	for baseName := range versions {
		sortVersionInfos(versions[baseName])
	}

	return versions, nil
}

// GetAllRemoteVersions aggregates image versions from all official repositories,
// merging entries by digest+version to avoid duplicates, and returns the combined
// sorted ImageVersionMap.
//
//	in(1): string architecture target architecture identifier (e.g. "amd64")
//	out: ImageVersionMap merged map of base image name to sorted []VersionInfo
func GetAllRemoteVersions(architecture string) ImageVersionMap {
	allVersions := make(ImageVersionMap)

	for _, repo := range OfficialRepos() {
		repoVersions, err := GetRemoteVersionsForRepo(repo, architecture)
		if err != nil {
			log.Printf("Warning: Error getting versions for %s: %v", repo, err)
			continue
		}

		for baseName, versions := range repoVersions {
			// Merge versions, avoiding duplicates by digest
			existing := allVersions[baseName]
			for _, v := range versions {
				found := false
				for _, ev := range existing {
					if ev.Digest == v.Digest && ev.Version == v.Version {
						found = true
						break
					}
				}
				if !found {
					allVersions[baseName] = append(allVersions[baseName], v)
				}
			}
		}
	}

	// Sort all version lists
	for baseName := range allVersions {
		sortVersionInfos(allVersions[baseName])
	}

	return allVersions
}

// getRemoteImageCreationDate queries Docker Hub for the push date of the specified tag
// in repo, filtering by architecture, and returns the parsed timestamp.
//
//	in(1): string repo Docker Hub repository (e.g. "penthertz/rfswift_noble")
//	in(2): string tag image tag name whose creation date is requested
//	in(3): string architecture target architecture used to match the correct tag variant
//	out: time.Time UTC timestamp of when the tag was last pushed
//	out: error non-nil if the HTTP request fails or the tag cannot be found
func getRemoteImageCreationDate(repo, tag, architecture string) (time.Time, error) {
	var result time.Time

	err := showLoadingIndicatorWithReturn(func() error {
		url := fmt.Sprintf("https://hub.docker.com/v2/repositories/%s/tags/?page_size=100", repo)
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Get(url)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusNotFound {
			return fmt.Errorf("tag not found")
		} else if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("failed to get tags: %s", resp.Status)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}

		date, err := getRemoteImageCreationDateFallback(body, tag, architecture)
		if err != nil {
			return err
		}

		result = date
		return nil
	}, fmt.Sprintf("Checking Docker Hub for '%s' (%s)", tag, architecture))

	return result, err
}

// getRemoteImageCreationDateFallback parses a pre-fetched Docker Hub response body
// to locate the push timestamp for tag (or its architecture-normalised equivalent),
// skipping cache tags and non-OCI-index media types.
//
//	in(1): []byte body raw JSON body of a Docker Hub tags response
//	in(2): string tag image tag name to search for
//	in(3): string architecture target architecture used for fallback tag normalisation
//	out: time.Time UTC timestamp of when the matching tag was last pushed
//	out: error non-nil if JSON parsing fails, the date cannot be parsed, or tag is not found
func getRemoteImageCreationDateFallback(body []byte, tag, architecture string) (time.Time, error) {
	var response DockerHubResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return time.Time{}, err
	}

	// Try both the original tag and the architecture-normalized version
	tagsToCheck := []string{tag, normalizeTagForRemote(tag, architecture)}

	for _, hubTag := range response.Results {
		for _, checkTag := range tagsToCheck {
			if hubTag.Name == checkTag {
				if strings.HasPrefix(hubTag.Name, "cache_") {
					continue
				}

				if hubTag.MediaType != "application/vnd.oci.image.index.v1+json" {
					continue
				}

				tagArch := determineArchitectureFromTag(hubTag.Name, architecture)
				if tagArch == architecture {
					lastPushed, err := time.Parse(time.RFC3339, hubTag.LastUpdated)
					if err != nil {
						return time.Time{}, fmt.Errorf("could not parse date for tag %s: %v", hubTag.Name, err)
					}
					return lastPushed, nil
				}
			}
		}
	}

	return time.Time{}, fmt.Errorf("tag not found")
}

// getLatestDockerHubTags fetches all pages of tags from a Docker Hub repository,
// filters by architecture and OCI index media type, deduplicates by tag name,
// and returns the results sorted by push date descending.
//
//	in(1): string repo Docker Hub repository to paginate (e.g. "penthertz/rfswift_noble")
//	in(2): string architecture target architecture to filter tags by
//	out: []Tag deduplicated and sorted list of matching Tag entries
//	out: error non-nil if any HTTP request or JSON parsing step fails
func getLatestDockerHubTags(repo string, architecture string) ([]Tag, error) {
	var latestTags []Tag

	err := showLoadingIndicatorWithReturn(func() error {
		baseURL := fmt.Sprintf("https://hub.docker.com/v2/repositories/%s/tags/", repo)
		url := fmt.Sprintf("%s?page_size=100", baseURL)

		client := &http.Client{Timeout: 10 * time.Second}

		for url != "" {
			resp, err := client.Get(url)
			if err != nil {
				return err
			}

			body, err := io.ReadAll(resp.Body)
			resp.Body.Close() // Close immediately after reading, not deferred in loop
			if err != nil {
				return err
			}

			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("failed to get tags: %s", resp.Status)
			}

			var response DockerHubResponse
			if err := json.Unmarshal(body, &response); err != nil {
				return err
			}

			// Process tags from this page
			for _, hubTag := range response.Results {
				if strings.HasPrefix(hubTag.Name, "cache_") {
					continue
				}

				if hubTag.MediaType != "application/vnd.oci.image.index.v1+json" {
					continue
				}

				lastPushed, err := time.Parse(time.RFC3339, hubTag.LastUpdated)
				if err != nil {
					log.Printf("Warning: Could not parse date for tag %s: %v", hubTag.Name, err)
					continue
				}

				tagArch := determineArchitectureFromTag(hubTag.Name, architecture)
				if tagArch != architecture {
					continue
				}

				images := []Image{
					{
						Architecture: tagArch,
						Digest:       hubTag.Digest,
					},
				}

				latestTags = append(latestTags, Tag{
				Name:          hubTag.Name,
				TagLastPushed: lastPushed,
				Images:        images,
				FullSize:      hubTag.FullSize,  // ADD THIS
			})
			}

			// Get next page URL
			url = response.Next
		}

		return nil
	}, "Fetching available tags")

	if err != nil {
		return nil, err
	}

	// Sort and deduplicate
	sort.Slice(latestTags, func(i, j int) bool {
		return latestTags[i].TagLastPushed.After(latestTags[j].TagLastPushed)
	})

	uniqueTags := make(map[string]Tag)
	for _, tag := range latestTags {
		if _, exists := uniqueTags[tag.Name]; !exists {
			uniqueTags[tag.Name] = tag
		}
	}

	var result []Tag
	for _, tag := range uniqueTags {
		result = append(result, tag)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].TagLastPushed.After(result[j].TagLastPushed)
	})

	return result, nil
}

// getLatestDockerHubTagsFallback parses a pre-fetched Docker Hub tags response body,
// applies the same architecture and media-type filters as getLatestDockerHubTags,
// deduplicates by tag name, and returns results sorted by push date descending.
//
//	in(1): []byte body raw JSON body of a Docker Hub tags response
//	in(2): string architecture target architecture to filter tags by
//	out: []Tag deduplicated and sorted list of matching Tag entries
//	out: error non-nil if JSON parsing fails
func getLatestDockerHubTagsFallback(body []byte, architecture string) ([]Tag, error) {
	var response DockerHubResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}

	var filteredTags []Tag
	for _, hubTag := range response.Results {
		if strings.HasPrefix(hubTag.Name, "cache_") {
			continue
		}

		if hubTag.MediaType != "application/vnd.oci.image.index.v1+json" {
			continue
		}

		lastPushed, err := time.Parse(time.RFC3339, hubTag.LastUpdated)
		if err != nil {
			log.Printf("Warning: Could not parse date for tag %s: %v", hubTag.Name, err)
			continue
		}

		tagArch := determineArchitectureFromTag(hubTag.Name, architecture)
		if tagArch != architecture {
			continue
		}

		images := []Image{
			{
				Architecture: tagArch,
				Digest:       hubTag.Digest,
			},
		}

		filteredTags = append(filteredTags, Tag{
			Name:          hubTag.Name,
			TagLastPushed: lastPushed,
			Images:        images,
		})
	}

	sort.Slice(filteredTags, func(i, j int) bool {
		return filteredTags[i].TagLastPushed.After(filteredTags[j].TagLastPushed)
	})

	uniqueTags := make(map[string]Tag)
	for _, tag := range filteredTags {
		if _, exists := uniqueTags[tag.Name]; !exists {
			uniqueTags[tag.Name] = tag
		}
	}

	var latestTags []Tag
	for _, tag := range uniqueTags {
		latestTags = append(latestTags, tag)
	}

	sort.Slice(latestTags, func(i, j int) bool {
		return latestTags[i].TagLastPushed.After(latestTags[j].TagLastPushed)
	})

	return latestTags, nil
}

// ListDockerImagesRepo fetches tags from all official Docker Hub repositories and
// renders them as a bordered table in the terminal. When showVersions is true an
// extra column lists all available semver versions for each image. Results are
// optionally narrowed to images whose name contains filterImage.
//
//	in(1): bool showVersions when true, fetch and display the versions column
//	in(2): string filterImage case-insensitive substring filter; pass "" to show all images
func ListDockerImagesRepo(showVersions bool, filterImage string) {
	repos := OfficialRepos()
	architecture := getArchitecture()
	if architecture == "" {
		log.Fatalf("Unsupported architecture: %s", runtime.GOARCH)
	}

	rfutils.ClearScreen()

	// Build version map for all repos
	var allVersions ImageVersionMap
	if showVersions {
		allVersions = make(ImageVersionMap)
	}

	type tagInfo struct {
		cleanName  string
		pushedDate time.Time
		repo       string
		arch       string
		digest     string
		fullSize   int64
		versions   []VersionInfo
	}
	uniqueTags := make(map[string]tagInfo)

	// Process each repository
	for _, repo := range repos {
		tags, err := getLatestDockerHubTags(repo, architecture)
		if err != nil {
			log.Printf("Warning: Error getting tags for %s: %v", repo, err)
			continue
		}

		// Build version map for this repo
		repoVersions := make(ImageVersionMap)
		for _, tag := range tags {
			baseName, version := parseTagVersion(tag.Name)

			digest := ""
			if len(tag.Images) > 0 {
				digest = tag.Images[0].Digest
			}

			info := VersionInfo{
				Version: version,
				Digest:  digest,
				Date:    tag.TagLastPushed,
			}
			if version == "" {
				info.Version = "latest"
			}

			repoVersions[baseName] = append(repoVersions[baseName], info)
		}

		// Sort versions
		for baseName := range repoVersions {
			sortVersionInfos(repoVersions[baseName])
		}

		// Merge into allVersions
		if showVersions {
			for baseName, versions := range repoVersions {
				allVersions[baseName] = versions
			}
		}

		for _, tag := range tags {
			// Skip versioned tags in main listing (show only "latest" entries)
			_, version := parseTagVersion(tag.Name)
			if version != "" {
				continue
			}

			hasArchSuffix := strings.HasSuffix(tag.Name, "_amd64") ||
				strings.HasSuffix(tag.Name, "_arm64") ||
				strings.HasSuffix(tag.Name, "_riscv64") ||
				strings.HasSuffix(tag.Name, "_arm")

			if !hasArchSuffix {
				continue
			}

			cleanTagName := removeArchitectureSuffix(tag.Name)

			// Apply filter
			if filterImage != "" && !strings.Contains(strings.ToLower(cleanTagName), strings.ToLower(filterImage)) {
				continue
			}

			uniqueKey := fmt.Sprintf("%s:%s", repo, cleanTagName)

			for _, image := range tag.Images {
				if image.Architecture == architecture {
					var versions []VersionInfo
					if showVersions {
						versions = repoVersions[cleanTagName]
					}

					if existing, exists := uniqueTags[uniqueKey]; !exists {
						uniqueTags[uniqueKey] = tagInfo{
							cleanName:  cleanTagName,
							pushedDate: tag.TagLastPushed,
							repo:       repo,
							arch:       image.Architecture,
							digest:     image.Digest,
							fullSize:   tag.FullSize,
							versions:   versions,
						}
					} else if tag.TagLastPushed.After(existing.pushedDate) {
						uniqueTags[uniqueKey] = tagInfo{
							cleanName:  cleanTagName,
							pushedDate: tag.TagLastPushed,
							repo:       repo,
							arch:       image.Architecture,
							digest:     image.Digest,
							fullSize:   tag.FullSize,
							versions:   versions,
						}
					}
					break
				}
			}
		}
	}

	// Build headers
	headers := []string{"Tag", "Pushed Date", "Image", "Size"}
	if showVersions {
		headers = append(headers, "Versions")
	}

	// Get terminal width
	width := tui.TerminalWidth()

	// Calculate base column widths
	baseWidths := []int{20, 18, 35, 12}
	if showVersions {
		// Calculate remaining width for versions column
		usedWidth := 1 // left border
		for _, w := range baseWidths {
			usedWidth += w + 3 // content + borders/padding
		}
		versionWidth := width - usedWidth - 4
		if versionWidth < 25 {
			versionWidth = 25
		}
		if versionWidth > 60 {
			versionWidth = 60
		}
		baseWidths = append(baseWidths, versionWidth)
	}

	// Build table data
	var tableData [][]string

	// Sort by date first
	type sortableTag struct {
		key  string
		info tagInfo
	}
	var sortableTags []sortableTag
	for key, info := range uniqueTags {
		sortableTags = append(sortableTags, sortableTag{key: key, info: info})
	}
	sort.Slice(sortableTags, func(i, j int) bool {
		return sortableTags[i].info.pushedDate.After(sortableTags[j].info.pushedDate)
	})

	// Deduplicate
	seenTags := make(map[string]bool)
	for _, st := range sortableTags {
		tagKey := st.info.cleanName + "|" + st.info.repo
		if seenTags[tagKey] {
			continue
		}
		seenTags[tagKey] = true

		info := st.info

		// Format size
		sizeStr := "-"
		if info.fullSize > 0 {
			sizeStr = fmt.Sprintf("%.1f MB", float64(info.fullSize)/(1024*1024))
		}

		row := []string{
			info.cleanName,
			info.pushedDate.Format("2006-01-02 15:04"),
			fmt.Sprintf("%s:%s", info.repo, info.cleanName),
			sizeStr,
		}

		if showVersions {
			versionWidth := baseWidths[len(baseWidths)-1]
			versionLines := FormatVersionsMultiLine(info.versions, 4, versionWidth)
			row = append(row, strings.Join(versionLines, "\n"))
		}

		tableData = append(tableData, row)
	}

	versionCol := len(headers) - 1

	tui.RenderTable(tui.TableConfig{
		Title:      "💿 Official Images",
		TitleColor: tui.ColorPrimary,
		Headers:    headers,
		Rows:       tableData,
		BorderRow:  true,
		ColorFunc: func(row, col int, content string) lipgloss.Color {
			if showVersions && col == versionCol && content != "" && content != "-" {
				return tui.ColorCyan
			}
			return lipgloss.Color("")
		},
	})

	// Print summary and hints
	common.PrintInfoMessage(fmt.Sprintf("Found %d image(s) for %s architecture", len(tableData), architecture))
}

// removeArchitectureSuffix strips a known architecture suffix (_amd64, _arm64,
// _riscv64, _arm) from tagName and returns the result. Returns tagName unchanged
// when no known suffix is present.
//
//	in(1): string tagName Docker Hub tag name to strip
//	out: string tag name with the architecture suffix removed, or tagName if none matched
func removeArchitectureSuffix(tagName string) string {
	suffixes := []string{"_amd64", "_arm64", "_riscv64", "_arm"}

	for _, suffix := range suffixes {
		if strings.HasSuffix(tagName, suffix) {
			return strings.TrimSuffix(tagName, suffix)
		}
	}

	return tagName
}

