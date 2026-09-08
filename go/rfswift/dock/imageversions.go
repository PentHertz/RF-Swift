/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
*
*  Published versions of an official image, for a version picker: the
*  Workbench's create dialog (locally, or on the agent host through the
*  "images.versions" control method) and any other client that wants to pin
*  a release instead of following the rolling tag. The CLI's equivalent is
*  `rfswift image versions` and `rfswift image pull -V`.
 */

package dock

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/moby/moby/client"
)

// ImageVersionEntry is one published version of an official image. Ref is
// the local tag a pull produces and a container is created from: repo:base
// for the rolling latest build, repo:base_version for a release (the
// registry tag carries the architecture suffix; PullImageContext adds it and
// retags without it, the way `rfswift image pull -V` does).
type ImageVersionEntry struct {
	Version string    `json:"version"` // "latest" or a release such as "0.1.1"
	Ref     string    `json:"ref"`
	Digest  string    `json:"digest"`
	Date    time.Time `json:"date"`
	Newest  bool      `json:"newest"` // the highest published release
}

// ImageVersionList is what a version picker shows for one image.
type ImageVersionList struct {
	Repo         string              `json:"repo"`
	Base         string              `json:"base"` // toolset name without repository, version or architecture
	Architecture string              `json:"architecture"`
	Versions     []ImageVersionEntry `json:"versions"`
}

// ImageVersions lists the versions Docker Hub publishes for an official image
// (a short name such as sdr_full, a full reference, or a versioned tag) for
// this host's architecture: the rolling latest first, then the releases,
// newest first. A custom image has no published versions.
func ImageVersions(imageName string) (ImageVersionList, error) {
	arch := getArchitecture()
	if arch == "" {
		return ImageVersionList{}, fmt.Errorf("unsupported architecture %s", runtime.GOARCH)
	}
	resolved := strings.TrimPrefix(strings.TrimSpace(imageName), "docker.io/")
	if resolved == "" {
		return ImageVersionList{}, fmt.Errorf("image name is required")
	}
	if !strings.Contains(resolved, ":") {
		resolved = OfficialRepos()[0] + ":" + resolved
	}
	repo, tag := parseImageName(resolved)
	if !IsOfficialImage(repo + ":" + tag) {
		return ImageVersionList{}, fmt.Errorf("%s is not an official RF Swift image: it has no published versions", imageName)
	}
	base, _ := parseTagVersion(tag)
	repoVersions, err := GetRemoteVersionsForRepo(repo, arch)
	if err != nil {
		return ImageVersionList{}, err
	}
	entries := imageVersionEntries(repo, base, repoVersions[base])
	if len(entries) == 0 {
		return ImageVersionList{}, fmt.Errorf("no published version of %s for %s in %s", base, arch, repo)
	}
	return ImageVersionList{Repo: repo, Base: base, Architecture: arch, Versions: entries}, nil
}

// imageVersionEntries orders what the registry reported (the rolling latest
// first, then releases newest first), drops duplicates, and gives every
// entry the local tag it pulls as.
func imageVersionEntries(repo, base string, versions []VersionInfo) []ImageVersionEntry {
	sorted := append([]VersionInfo(nil), versions...)
	sortVersionInfos(sorted)
	var out []ImageVersionEntry
	seen := map[string]bool{}
	newest := false
	for _, v := range sorted {
		if v.Version == "" || seen[v.Version] {
			continue
		}
		seen[v.Version] = true
		e := ImageVersionEntry{Version: v.Version, Digest: v.Digest, Date: v.Date, Ref: repo + ":" + base}
		if v.Version != "latest" {
			e.Ref = repo + ":" + base + "_" + v.Version
			if !newest {
				newest, e.Newest = true, true
			}
		}
		out = append(out, e)
	}
	return out
}

// localImageVersion says which release a local official image is, for the
// Workbench summary: the tag's version for a pinned release; for a rolling
// tag (repo:sdr_full is whatever release was newest when it was pulled) the
// release whose published digest the local image carries. latest is the
// newest release published for that toolset.
func localImageVersion(ctx context.Context, cli *client.Client, repo, tag string, versions ImageVersionMap) (version, latest string, pinned bool) {
	base, tagVersion := parseTagVersion(tag)
	published := versions[base]
	latest = newestPublished(published)
	if tagVersion != "" {
		return tagVersion, latest, true
	}
	img, err := inspectImage(ctx, cli, repo+":"+tag)
	if err != nil {
		return "", latest, false
	}
	var digests []string
	for _, repoDigest := range img.RepoDigests {
		if i := strings.Index(repoDigest, "@"); i != -1 {
			digests = append(digests, repoDigest[i+1:])
		}
	}
	return matchPublishedVersion(digests, published), latest, false
}

// newestPublished is the highest release in a published list.
func newestPublished(published []VersionInfo) string {
	sorted := append([]VersionInfo(nil), published...)
	sortVersionInfos(sorted)
	for _, v := range sorted {
		if v.Version != "" && v.Version != "latest" {
			return v.Version
		}
	}
	return ""
}

// matchPublishedVersion names the release whose published digest a local
// image carries, or "" when the image matches no listed release (a build
// published only under the rolling tag, or one no longer listed).
func matchPublishedVersion(localDigests []string, published []VersionInfo) string {
	for _, v := range published {
		if v.Version != "" && v.Version != "latest" && digestMatches(localDigests, v.Digest) {
			return v.Version
		}
	}
	return ""
}
