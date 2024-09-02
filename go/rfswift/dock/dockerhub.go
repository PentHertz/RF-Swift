package dock

import (
	"encoding/json"
	"fmt"
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

func getArchitecture() string {
	switch runtime.GOARCH {
	case "amd64":
		return "amd64"
	case "arm64":
		return "arm64"
	case "arm":
		return "arm"
	default:
		return ""
	}
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

	var tagList TagList
	if err := json.NewDecoder(resp.Body).Decode(&tagList); err != nil {
		return time.Time{}, err
	}

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

	var tagList TagList
	if err := json.NewDecoder(resp.Body).Decode(&tagList); err != nil {
		return nil, err
	}

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

func ListDockerImagesRepo() {
	repo := "penthertz/rfswift"
	architecture := getArchitecture()
	if architecture == "" {
		log.Fatalf("Unsupported architecture: %s", runtime.GOARCH)
	}
	tags, err := getLatestDockerHubTags(repo, architecture)
	if err != nil {
		log.Fatalf("Error getting tags: %v", err)
	}

	rfutils.ClearScreen()

	headers := []string{"Tag", "Pushed Date", "Image", "Architecture"}
	tableData := [][]string{}

	for _, tag := range tags {
		for _, image := range tag.Images {
			if image.Architecture == architecture {
				tableData = append(tableData, []string{
					tag.Name,
					tag.TagLastPushed.Format(time.RFC3339),
					fmt.Sprintf("%s:%s", repo, tag.Name),
					image.Architecture,
				})
				break
			}
		}
	}

	width, _, err := terminal.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		width = 80 // default width if terminal size cannot be determined
	}

	columnWidths := make([]int, len(headers))
	for i, header := range headers {
		columnWidths[i] = len(header)
	}
	for _, row := range tableData {
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

	for i, row := range tableData {
		printRow(row, columnWidths, "│")
		if i < len(tableData)-1 {
			printHorizontalBorder(columnWidths, "├", "┼", "┤")
		}
	}

	printHorizontalBorder(columnWidths, "└", "┴", "┘")

	fmt.Print(reset)
	fmt.Println()
}
