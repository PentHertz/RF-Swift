package dock

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"runtime"
	"sort"
	"time"

	"github.com/olekukonko/tablewriter"
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

func getLatestDockerHubTags(repo string, architecture string) ([]Tag, error) {
	/*
	 *	Get latest Docker images details
	 *	in(1): remote repository string
	 *	in(2): architecture string
	 *	out: tuple status
	 */
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
	/*
	 *	Prints Latest tags for RF Swift
	 */
	repo := "penthertz/rfswift" // Change this to the repository you want to check
	architecture := getArchitecture()

	if architecture == "" {
		log.Fatalf("Unsupported architecture: %s", runtime.GOARCH)
	}

	tags, err := getLatestDockerHubTags(repo, architecture)
	if err != nil {
		log.Fatalf("Error getting tags: %v", err)
	}

	rfutils.ClearScreen()

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Tag", "Pushed Date", "Image", "Architecture", "Digest"})

	for _, tag := range tags {
		for _, image := range tag.Images {
			if image.Architecture == architecture {
				table.Append([]string{
					tag.Name,
					tag.TagLastPushed.Format(time.RFC3339),
					fmt.Sprintf("%s:%s", repo, tag.Name),
					image.Architecture,
					image.Digest,
				})
				break
			}
		}
	}

	table.Render()
}