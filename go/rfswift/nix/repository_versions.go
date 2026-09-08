package nix

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// RepositoryVersion is a selectable, reproducible RF-Swift-nix revision.
type RepositoryVersion struct {
	Name   string `json:"name"`
	Ref    string `json:"ref"`
	Commit string `json:"commit"`
	Date   string `json:"date,omitempty"`
	Kind   string `json:"kind"`
}

// RepositoryVersions describes the tip of the default branch followed by the
// repository's published tags.
type RepositoryVersions struct {
	Repository    string              `json:"repository"`
	DefaultBranch string              `json:"defaultBranch"`
	Latest        *RepositoryVersion  `json:"latest,omitempty"`
	Nightly       RepositoryVersion   `json:"nightly"`
	Releases      []RepositoryVersion `json:"releases"`
}

type githubRepository struct {
	DefaultBranch string `json:"default_branch"`
}

type githubCommit struct {
	SHA    string `json:"sha"`
	Commit struct {
		Committer struct {
			Date time.Time `json:"date"`
		} `json:"committer"`
	} `json:"commit"`
}

type githubTag struct {
	Name   string `json:"name"`
	Commit struct {
		SHA string `json:"sha"`
	} `json:"commit"`
}

type githubRelease struct {
	TagName string `json:"tag_name"`
}

// ListRepositoryVersions lists the latest release tag, remaining tags, and the
// current default-branch commit for a GitHub-backed flake. Nightly is returned
// as an immutable rev ref so missions do not silently change after creation.
func ListRepositoryVersions(ctx context.Context, flakeRef string) (RepositoryVersions, error) {
	return listRepositoryVersions(ctx, flakeRef, "https://api.github.com", &http.Client{Timeout: 20 * time.Second})
}

func listRepositoryVersions(ctx context.Context, flakeRef, apiBase string, client *http.Client) (RepositoryVersions, error) {
	owner, repo, err := githubRepositoryFromFlake(flakeRef)
	if err != nil {
		return RepositoryVersions{}, err
	}
	result := RepositoryVersions{Repository: owner + "/" + repo, Releases: []RepositoryVersion{}}
	var metadata githubRepository
	if err := githubGet(ctx, client, apiBase+"/repos/"+owner+"/"+repo, &metadata); err != nil {
		return RepositoryVersions{}, err
	}
	if metadata.DefaultBranch == "" {
		return RepositoryVersions{}, fmt.Errorf("GitHub repository response has no default branch")
	}
	result.DefaultBranch = metadata.DefaultBranch
	var commit githubCommit
	if err := githubGet(ctx, client, apiBase+"/repos/"+owner+"/"+repo+"/commits/"+url.PathEscape(result.DefaultBranch), &commit); err != nil {
		return RepositoryVersions{}, err
	}
	result.Nightly = RepositoryVersion{Name: "Nightly", Ref: "github:" + owner + "/" + repo + "?rev=" + commit.SHA, Commit: commit.SHA, Date: commit.Commit.Committer.Date.Format(time.RFC3339), Kind: "nightly"}
	for page := 1; ; page++ {
		var tags []githubTag
		endpoint := fmt.Sprintf("%s/repos/%s/%s/tags?per_page=100&page=%d", apiBase, owner, repo, page)
		if err := githubGet(ctx, client, endpoint, &tags); err != nil {
			return RepositoryVersions{}, err
		}
		for _, tag := range tags {
			result.Releases = append(result.Releases, RepositoryVersion{Name: tag.Name, Ref: "github:" + owner + "/" + repo + "/" + tag.Name, Commit: tag.Commit.SHA, Kind: "release"})
		}
		if len(tags) < 100 {
			break
		}
	}
	var published githubRelease
	found, err := githubGetOptional(ctx, client, apiBase+"/repos/"+owner+"/"+repo+"/releases/latest", &published)
	if err != nil {
		return RepositoryVersions{}, err
	}
	if found {
		for index, release := range result.Releases {
			if release.Name != published.TagName {
				continue
			}
			release.Kind = "latest"
			result.Latest = &release
			result.Releases = append(result.Releases[:index], result.Releases[index+1:]...)
			break
		}
	}
	return result, nil
}

func githubGetOptional(ctx context.Context, client *http.Client, endpoint string, dst any) (bool, error) {
	req, err := githubRequest(ctx, endpoint)
	if err != nil {
		return false, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return false, fmt.Errorf("query GitHub releases: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, fmt.Errorf("query GitHub releases: %s", resp.Status)
	}
	if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
		return false, fmt.Errorf("decode GitHub response: %w", err)
	}
	return true, nil
}

func githubGet(ctx context.Context, client *http.Client, endpoint string, dst any) error {
	req, err := githubRequest(ctx, endpoint)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("query GitHub releases: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("query GitHub releases: %s", resp.Status)
	}
	if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
		return fmt.Errorf("decode GitHub response: %w", err)
	}
	return nil
}

func githubRequest(ctx context.Context, endpoint string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "RF-Swift")
	if token := firstNonEmpty(os.Getenv("GITHUB_TOKEN"), os.Getenv("GH_TOKEN")); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func githubRepositoryFromFlake(flakeRef string) (string, string, error) {
	ref := strings.TrimSpace(flakeRef)
	if ref == "" {
		ref = DefaultFlakeRef
	}
	if strings.HasPrefix(ref, "path:") {
		ref = strings.TrimPrefix(ref, "path:")
	}
	if st, err := os.Stat(ref); err == nil && st.IsDir() {
		remote, err := exec.Command("git", "-C", filepath.Clean(ref), "remote", "get-url", "origin").Output()
		if err != nil {
			return "", "", fmt.Errorf("local flake has no readable GitHub origin")
		}
		ref = strings.TrimSpace(string(remote))
	}
	ref = strings.TrimPrefix(ref, "github:")
	ref = strings.TrimPrefix(ref, "https://github.com/")
	ref = strings.TrimPrefix(ref, "http://github.com/")
	ref = strings.TrimPrefix(ref, "git@github.com:")
	ref = strings.TrimSuffix(strings.SplitN(ref, "?", 2)[0], ".git")
	parts := strings.Split(strings.Trim(ref, "/"), "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("flake %q is not backed by a GitHub owner/repository", flakeRef)
	}
	return parts[0], parts[1], nil
}
