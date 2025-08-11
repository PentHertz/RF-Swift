package dock

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"runtime"
	"sort"
	"strings"
	"time"

	"golang.org/x/crypto/ssh/terminal"
	rfutils "penthertz/rfswift/rfutils"
)

type Tag struct {
	Name          string    `json:"name"`
	Images        []Image   `json:"images"`
	TagLastPushed time.Time `json:"tag_last_pushed"`
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

func determineArchitectureFromTag(tagName, requestedArch string) string {
	// Check for explicit architecture suffixes
	if strings.HasSuffix(tagName, "_amd64") {
		return "amd64"
	}
	if strings.HasSuffix(tagName, "_arm64") {
		return "arm64"
	}
	if strings.HasSuffix(tagName, "_riscv64") {
		return "riscv64"
	}

	// For tags without explicit architecture suffix, consider them as amd64 by default
	// (as you specified) or match the requested architecture
	if requestedArch == "amd64" || requestedArch == "" {
		return "amd64"
	}

	// For other architectures, only return if it's the requested one
	return requestedArch
}

func OfficialRepos() []string {
	return []string{"penthertz/rfswift", "penthertz/rfswift_noble"}
}

// IsOfficialImage checks if an image belongs to official repositories
func IsOfficialImage(imageName string) bool {
	for _, repo := range OfficialRepos() {
		if strings.HasPrefix(imageName, repo+":") {
			return true
		}
	}
	return false
}

func getRemoteImageCreationDate(repo, tag, architecture string) (time.Time, error) {
	url := fmt.Sprintf("https://hub.docker.com/v2/repositories/%s/tags/?page_size=100", repo)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return time.Time{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return time.Time{}, fmt.Errorf("tag not found")
	} else if resp.StatusCode != http.StatusOK {
		return time.Time{}, fmt.Errorf("failed to get tags: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return time.Time{}, err
	}

	// First, try to parse with the original structure
	var tagList TagList
	if err := json.Unmarshal(body, &tagList); err != nil {
		return time.Time{}, err
	}

	// Check if images arrays are empty (new API issue)
	hasEmptyImages := len(tagList.Results) > 0 && len(tagList.Results[0].Images) == 0

	if hasEmptyImages {
		log.Printf("Warning: Docker Hub API returned empty images arrays for getRemoteImageCreationDate, using fallback method")
		return getRemoteImageCreationDateFallback(body, tag, architecture)
	}

	// Original logic when images arrays are populated
	for _, t := range tagList.Results {
		if t.Name == tag {
			for _, image := range t.Images {
				if image.Architecture == architecture {
					return t.TagLastPushed, nil
				}
			}
		}
	}

	return time.Time{}, fmt.Errorf("tag not found")
}

func getRemoteImageCreationDateFallback(body []byte, tag, architecture string) (time.Time, error) {
	var response DockerHubResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return time.Time{}, err
	}

	for _, hubTag := range response.Results {
		if hubTag.Name == tag {
			// Skip cache tags and other non-standard tags
			if strings.HasPrefix(hubTag.Name, "cache_") {
				continue
			}

			// Only process actual container images, not cache configs
			if hubTag.MediaType != "application/vnd.oci.image.index.v1+json" {
				continue
			}

			// Determine architecture from tag name
			tagArch := determineArchitectureFromTag(hubTag.Name, architecture)
			if tagArch == architecture {
				// Parse the last pushed date
				lastPushed, err := time.Parse(time.RFC3339, hubTag.LastUpdated)
				if err != nil {
					return time.Time{}, fmt.Errorf("could not parse date for tag %s: %v", hubTag.Name, err)
				}
				return lastPushed, nil
			}
		}
	}

	return time.Time{}, fmt.Errorf("tag not found")
}

func getLatestDockerHubTags(repo string, architecture string) ([]Tag, error) {
	url := fmt.Sprintf("https://hub.docker.com/v2/repositories/%s/tags/?page_size=100", repo)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get tags: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// First, try to parse with the original structure
	var tagList TagList
	if err := json.Unmarshal(body, &tagList); err != nil {
		return nil, err
	}

	// Check if images arrays are empty (new API issue)
	hasEmptyImages := len(tagList.Results) > 0 && len(tagList.Results[0].Images) == 0

	if hasEmptyImages {
		log.Printf("Warning: Docker Hub API returned empty images arrays, using fallback method")
		return getLatestDockerHubTagsFallback(body, architecture)
	}

	// Original logic when images arrays are populated
	var filteredTags []Tag
	for _, tag := range tagList.Results {
		for _, image := range tag.Images {
			if image.Architecture == architecture {
				filteredTags = append(filteredTags, tag)
				break
			}
		}
	}

	// Sort tags by pushed date from latest to oldest
	sort.Slice(filteredTags, func(i, j int) bool {
		return filteredTags[i].TagLastPushed.After(filteredTags[j].TagLastPushed)
	})

	// Remove duplicate tags, keeping only the latest
	uniqueTags := make(map[string]Tag)
	for _, tag := range filteredTags {
		if _, exists := uniqueTags[tag.Name]; !exists {
			uniqueTags[tag.Name] = tag
		}
	}

	// Convert map to slice
	var latestTags []Tag
	for _, tag := range uniqueTags {
		latestTags = append(latestTags, tag)
	}

	// Sort the tags again to ensure they are in the correct order after deduplication
	sort.Slice(latestTags, func(i, j int) bool {
		return latestTags[i].TagLastPushed.After(latestTags[j].TagLastPushed)
	})

	return latestTags, nil
}

func getLatestDockerHubTagsFallback(body []byte, architecture string) ([]Tag, error) {
	var response DockerHubResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}

	var filteredTags []Tag
	for _, hubTag := range response.Results {
		// Skip cache tags and other non-standard tags
		if strings.HasPrefix(hubTag.Name, "cache_") {
			continue
		}

		// Only process actual container images, not cache configs
		if hubTag.MediaType != "application/vnd.oci.image.index.v1+json" {
			continue
		}

		// Parse the last pushed date
		lastPushed, err := time.Parse(time.RFC3339, hubTag.LastUpdated)
		if err != nil {
			log.Printf("Warning: Could not parse date for tag %s: %v", hubTag.Name, err)
			continue
		}

		// Determine architecture from tag name
		tagArch := determineArchitectureFromTag(hubTag.Name, architecture)
		if tagArch != architecture {
			continue // Skip if architecture doesn't match
		}

		// Create synthetic image entry
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

	// Sort tags by pushed date from latest to oldest
	sort.Slice(filteredTags, func(i, j int) bool {
		return filteredTags[i].TagLastPushed.After(filteredTags[j].TagLastPushed)
	})

	// Remove duplicate tags, keeping only the latest
	uniqueTags := make(map[string]Tag)
	for _, tag := range filteredTags {
		if _, exists := uniqueTags[tag.Name]; !exists {
			uniqueTags[tag.Name] = tag
		}
	}

	// Convert map to slice
	var latestTags []Tag
	for _, tag := range uniqueTags {
		latestTags = append(latestTags, tag)
	}

	// Sort the tags again to ensure they are in the correct order after deduplication
	sort.Slice(latestTags, func(i, j int) bool {
		return latestTags[i].TagLastPushed.After(latestTags[j].TagLastPushed)
	})

	return latestTags, nil
}

func ListDockerImagesRepo() {
	repos := OfficialRepos() // Use multiple repositories now
	architecture := getArchitecture()
	if architecture == "" {
		log.Fatalf("Unsupported architecture: %s", runtime.GOARCH)
	}

	rfutils.ClearScreen()

	headers := []string{"Tag", "Pushed Date", "Image", "Architecture"}
	allTableData := [][]string{}

	// Process each repository
	for _, repo := range repos {
		tags, err := getLatestDockerHubTags(repo, architecture)
		if err != nil {
			log.Printf("Warning: Error getting tags for %s: %v", repo, err)
			continue // Skip this repo and continue with others
		}

		for _, tag := range tags {
			for _, image := range tag.Images {
				if image.Architecture == architecture {
					allTableData = append(allTableData, []string{
						tag.Name,
						tag.TagLastPushed.Format(time.RFC3339),
						fmt.Sprintf("%s:%s", repo, tag.Name),
						image.Architecture,
					})
					break
				}
			}
		}
	}

	// Sort all data by pushed date (newest first)
	sort.Slice(allTableData, func(i, j int) bool {
		dateI, errI := time.Parse(time.RFC3339, allTableData[i][1])
		dateJ, errJ := time.Parse(time.RFC3339, allTableData[j][1])
		if errI != nil || errJ != nil {
			return false
		}
		return dateI.After(dateJ)
	})

	width, _, err := terminal.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		width = 80 // default width if terminal size cannot be determined
	}

	columnWidths := make([]int, len(headers))
	for i, header := range headers {
		columnWidths[i] = len(header)
	}
	for _, row := range allTableData {
		for i, col := range row {
			if len(col) > columnWidths[i] {
				columnWidths[i] = len(col)
			}
		}
	}

	// Adjust column widths to fit the terminal width
	totalWidth := len(headers) + 1 // Adding 1 for the left border
	for _, w := range columnWidths {
		totalWidth += w + 2 // Adding 2 for padding
	}

	if totalWidth > width {
		excess := totalWidth - width
		for i := range columnWidths {
			reduction := excess / len(columnWidths)
			if columnWidths[i] > reduction {
				columnWidths[i] -= reduction
				excess -= reduction
			}
		}
		totalWidth = width
	}

	blue := "\033[34m"
	white := "\033[37m"
	reset := "\033[0m"
	title := "💿 Official Images"

	fmt.Printf("%s%s%s%s%s\n", blue, strings.Repeat(" ", 2), title, strings.Repeat(" ", totalWidth-2-len(title)), reset)
	fmt.Print(white)

	printHorizontalBorder(columnWidths, "┌", "┬", "┐")
	printRow(headers, columnWidths, "│")
	printHorizontalBorder(columnWidths, "├", "┼", "┤")

	for i, row := range allTableData {
		printRow(row, columnWidths, "│")
		if i < len(allTableData)-1 {
			printHorizontalBorder(columnWidths, "├", "┼", "┤")
		}
	}

	printHorizontalBorder(columnWidths, "└", "┴", "┘")

	fmt.Print(reset)
	fmt.Println()
}
