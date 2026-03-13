/* This code is part of RF Switch by @Penthertz
 * Author(s): Sebastien Dudek (@FlUxIuS)
 */

package dock

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/moby/term"

	common "penthertz/rfswift/common"
	rfutils "penthertz/rfswift/rfutils"
	"penthertz/rfswift/tui"
)

// getRemoteImageDigest fetches the digest for a specific tag from Docker Hub,
// normalizing the tag for the target architecture before querying the registry API.
//
//	in(1): string repo         Docker Hub repository path (e.g. "penthertz/rfswift_noble")
//	in(2): string tag          image tag to look up
//	in(3): string architecture target architecture string used to normalize the tag
//	out:   string              digest string for the matched tag, empty on failure
//	out:   error               non-nil if the tag was not found or the request failed
func getRemoteImageDigest(repo, tag, architecture string) (string, error) {
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

// checkIfImageIsUpToDate reports whether the given tag is listed among the
// latest Docker Hub tags for the current architecture of the host system.
//
//	in(1): string repo  Docker Hub repository path
//	in(2): string tag   image tag to check
//	out:   bool         true if the tag matches a current remote tag
//	out:   error        non-nil if the remote tag list could not be retrieved
func checkIfImageIsUpToDate(repo, tag string) (bool, error) {
	architecture := getArchitecture()
	tags, err := getLatestDockerHubTags(repo, architecture)
	if err != nil {
		return false, err
	}

	for _, latestTag := range tags {
		if latestTag.Name == tag {
			return true, nil
		}
	}

	return false, nil
}

// getLocalImageCreationDate returns the creation timestamp of a locally stored image
// by inspecting its metadata and parsing the RFC3339 Created field.
//
//	in(1): context.Context    request context
//	in(2): *client.Client     Docker/Podman engine client
//	in(3): string imageName   fully-qualified image name or ID to inspect
//	out:   time.Time          parsed creation time of the local image
//	out:   error              non-nil if the image does not exist locally or the timestamp is unparseable
func getLocalImageCreationDate(ctx context.Context, cli *client.Client, imageName string) (time.Time, error) {
	localImage, _, err := cli.ImageInspectWithRaw(ctx, imageName)
	if err != nil {
		return time.Time{}, err
	}
	localImageTime, err := time.Parse(time.RFC3339, localImage.Created)
	if err != nil {
		return time.Time{}, err
	}
	return localImageTime, nil
}

// digestMatches reports whether remoteDigest is present in the localDigests slice.
//
//	in(1): []string localDigests  list of digests extracted from a local image's RepoDigests
//	in(2): string remoteDigest    digest string retrieved from the remote registry
//	out:   bool                   true if remoteDigest is found in localDigests
func digestMatches(localDigests []string, remoteDigest string) bool {
	for _, d := range localDigests {
		if d == remoteDigest {
			return true
		}
	}
	return false
}

// checkImageStatusWithCache determines whether a locally stored image is up-to-date,
// obsolete, or custom by comparing its digest against a pre-fetched remote version map,
// avoiding repeated network calls when checking many images at once.
//
//	in(1): context.Context       request context
//	in(2): *client.Client        Docker/Podman engine client
//	in(3): string repo           Docker Hub repository path
//	in(4): string tag            image tag to evaluate
//	in(5): string architecture   host architecture string used for tag normalization
//	in(6): RepoVersionMap        pre-fetched map of remote versions keyed by repository and base name
//	out:   bool                  true if the local image digest matches the latest remote version
//	out:   bool                  true if the image is considered custom (not an official RF Swift image)
//	out:   error                 non-nil if the local image could not be inspected
func checkImageStatusWithCache(ctx context.Context, cli *client.Client, repo, tag string, architecture string, cachedVersionsByRepo RepoVersionMap) (bool, bool, error) {
	if common.Disconnected {
		return false, true, nil
	}

	// Check if this is an official image
	fullImageName := fmt.Sprintf("%s:%s", repo, tag)
	if !IsOfficialImage(fullImageName) {
		return false, true, nil // Custom image
	}

	// Get local image info
	localImage, _, err := cli.ImageInspectWithRaw(ctx, fullImageName)
	if err != nil {
		return false, true, err
	}

	// Get local digests (Podman may store multiple)
	var localDigests []string
	for _, repoDigest := range localImage.RepoDigests {
		if idx := strings.Index(repoDigest, "@"); idx != -1 {
			localDigests = append(localDigests, repoDigest[idx+1:])
		}
	}

	// Parse tag to check if it's a versioned tag (e.g., reversing_0.0.7)
	baseName, version := parseTagVersion(tag)

	// Get versions for THIS SPECIFIC REPO
	repoVersions, ok := cachedVersionsByRepo[repo]
	if !ok || len(repoVersions) == 0 {
		// Repo not found in cache - likely custom image
		return false, true, nil
	}

	versions, ok := repoVersions[baseName]
	if !ok || len(versions) == 0 {
		// Base name not found in this repo - likely custom image
		return false, true, nil
	}

	// Find the latest non-"latest" version (highest semver) for THIS REPO
	var latestVersion string
	var latestVersionDigest string
	for _, v := range versions {
		if v.Version != "latest" {
			if latestVersion == "" {
				// First non-latest version is the highest due to sorting
				latestVersion = v.Version
				latestVersionDigest = v.Digest
			}
			break
		}
	}

	// If it's a versioned tag (e.g., reversing_0.0.7)
	if version != "" {
		// Check if this version is the latest for this repo
		if latestVersion != "" && compareVersions(version, latestVersion) < 0 {
			// There's a newer version available in this repo = Obsolete
			return false, false, nil
		}

		// This is the latest version (or equal), check if digest matches
		for _, v := range versions {
			if v.Version == version {
				if len(localDigests) > 0 && digestMatches(localDigests, v.Digest) {
					return true, false, nil // Up-to-date
				}
				// Same version but different digest = Obsolete (rebuilt)
				return false, false, nil
			}
		}

		// Version not found in remote - custom local version
		return false, true, nil
	}

	// For non-versioned tags (like "sdr_light", "reversing"):
	// Find which version the local image matches by digest
	var matchedVersion string
	for _, v := range versions {
		if digestMatches(localDigests, v.Digest) {
			if v.Version != "latest" {
				matchedVersion = v.Version
			}
			break
		}
	}

	// If we found a matching version, check if it's the latest for this repo
	if matchedVersion != "" {
		if latestVersion != "" && compareVersions(matchedVersion, latestVersion) < 0 {
			// There's a newer version in this repo = Obsolete
			return false, false, nil
		}
		// Matched version is the latest for this repo = Up to date
		return true, false, nil
	}

	// No version match found by digest - compare with latest digest directly
	if latestVersionDigest != "" && len(localDigests) > 0 {
		if digestMatches(localDigests, latestVersionDigest) {
			return true, false, nil // Up-to-date
		}
		return false, false, nil // Obsolete
	}

	// Fallback: check "latest" tag digest
	for _, v := range versions {
		if v.Version == "latest" {
			if len(localDigests) > 0 && digestMatches(localDigests, v.Digest) {
				return true, false, nil
			}
			return false, false, nil
		}
	}

	// Could not determine - assume custom
	return false, true, nil
}

// checkImageStatus checks whether a local image is up-to-date compared to the
// remote registry by fetching the current remote version map and delegating to
// checkImageStatusWithCache.
//
//	in(1): context.Context  request context
//	in(2): *client.Client   Docker/Podman engine client
//	in(3): string repo      Docker Hub repository path
//	in(4): string tag       image tag to evaluate
//	out:   bool             true if the local image digest matches the latest remote version
//	out:   bool             true if the image is considered custom (not an official RF Swift image)
//	out:   error            non-nil if the local image could not be inspected
func checkImageStatus(ctx context.Context, cli *client.Client, repo, tag string) (bool, bool, error) {
	if common.Disconnected {
		return false, true, nil
	}
	architecture := getArchitecture()

	// Check if this is an official image
	fullImageName := fmt.Sprintf("%s:%s", repo, tag)
	if !IsOfficialImage(fullImageName) {
		return false, true, nil // Custom image
	}

	// Fetch versions by repo
	cachedVersionsByRepo := GetAllRemoteVersionsByRepo(architecture)

	return checkImageStatusWithCache(ctx, cli, repo, tag, architecture, cachedVersionsByRepo)
}

// getLocalImageDigest returns the first RepoDigest hash found for a locally
// stored image, stripping the "repo@" prefix so only the raw digest is returned.
//
//	in(1): context.Context  request context
//	in(2): *client.Client   Docker/Podman engine client
//	in(3): string imageName fully-qualified image name or ID to inspect
//	out:   string           digest string (e.g. "sha256:abc…"), empty string on failure
func getLocalImageDigest(ctx context.Context, cli *client.Client, imageName string) string {
	imageInspect, _, err := cli.ImageInspectWithRaw(ctx, imageName)
	if err != nil {
		return ""
	}

	for _, repoDigest := range imageInspect.RepoDigests {
		if idx := strings.Index(repoDigest, "@"); idx != -1 {
			return repoDigest[idx+1:]
		}
	}

	return ""
}

// getLocalImageDigests returns all RepoDigest hashes found for a locally stored
// image, stripping the "repo@" prefix from each entry so only raw digests are returned.
//
//	in(1): context.Context  request context
//	in(2): *client.Client   Docker/Podman engine client
//	in(3): string imageName fully-qualified image name or ID to inspect
//	out:   []string         slice of digest strings; nil if the image is not found or has no digests
func getLocalImageDigests(ctx context.Context, cli *client.Client, imageName string) []string {
	imageInspect, _, err := cli.ImageInspectWithRaw(ctx, imageName)
	if err != nil {
		return nil
	}
	var digests []string
	for _, repoDigest := range imageInspect.RepoDigests {
		if idx := strings.Index(repoDigest, "@"); idx != -1 {
			digests = append(digests, repoDigest[idx+1:])
		}
	}
	return digests
}

// ContainerPull pulls a Docker image from the remote registry and optionally
// retags it with a clean local name, handling architecture-specific tag
// normalization and prompting the user to preserve any differing local copy.
//
//	in(1): string imageref  fully-qualified image reference to pull (e.g. "repo/image:tag")
//	in(2): string imagetag  local tag to assign after pulling; if empty a clean name is derived
func ContainerPull(imageref string, imagetag string) {
	ctx := context.Background()
	cli, err := NewEngineClient()
	if err != nil {
		common.PrintErrorMessage(err)
		return
	}
	defer cli.Close()

	// Get current architecture
	architecture := getArchitecture()

	imageref = normalizeImageName(imageref)

	// Parse the image reference to get repo and tag
	parts := strings.Split(imageref, ":")
	repo := parts[0]
	tag := "latest"
	if len(parts) > 1 {
		tag = parts[1]
	}

	// Check if this is an official image that might need architecture suffix
	isOfficial := IsOfficialImage(imageref)

	// For official images, ALWAYS use architecture suffix
	actualPullRef := imageref
	if isOfficial && architecture != "" {
		// Check if tag already has an architecture suffix
		hasArchSuffix := strings.HasSuffix(tag, "_amd64") ||
			strings.HasSuffix(tag, "_arm64") ||
			strings.HasSuffix(tag, "_riscv64") ||
			strings.HasSuffix(tag, "_arm")

		if !hasArchSuffix {
			// Append architecture to the tag - this is required for official images
			actualPullRef = fmt.Sprintf("%s:%s_%s", repo, tag, architecture)
			common.PrintInfoMessage(fmt.Sprintf("Detected architecture: %s, pulling %s", architecture, actualPullRef))
		} else {
			common.PrintInfoMessage(fmt.Sprintf("Using architecture-specific tag: %s", actualPullRef))
		}
	} else if isOfficial && architecture == "" {
		common.PrintErrorMessage(fmt.Errorf("cannot determine system architecture for official image"))
		return
	}

	// Set the display tag (without architecture suffix for cleaner naming)
	if imagetag == "" {
		// Use clean tag name without architecture suffix
		imagetag = fmt.Sprintf("%s:%s", repo, tag)
	}

	// Check if the image exists locally
	localInspect, localInspectErr := ImageInspectCompat(ctx, cli, imagetag)
	localExists := localInspectErr == nil
	localDigest := ""
	if localExists {
		localDigest = localInspect.ID
	}

	// Pull the image from remote using the architecture-specific reference
	common.PrintInfoMessage(fmt.Sprintf("Pulling image from: %s", actualPullRef))
	out, err := cli.ImagePull(ctx, actualPullRef, image.PullOptions{})
	if err != nil {
		common.PrintErrorMessage(err)
		return
	}
	defer out.Close()

	// Process pull output with per-layer progress
	pullProgress := tui.NewLayerProgress()
	jsonDecoder := json.NewDecoder(out)
	for {
		var msg jsonmessage.JSONMessage
		if err := jsonDecoder.Decode(&msg); err == io.EOF {
			break
		} else if err != nil {
			common.PrintErrorMessage(err)
			return
		}
		if msg.ID != "" {
			var current, total int64
			if msg.Progress != nil {
				current = msg.Progress.Current
				total = msg.Progress.Total
			}
			pullProgress.Update(msg.ID, msg.Status, current, total)
			if pullProgress.Total() > 0 {
				fmt.Printf("\033[%dA\033[J", pullProgress.Total())
			}
			fmt.Print(pullProgress.Render())
		} else if msg.Status != "" {
			fmt.Printf("  %s\n", msg.Status)
		}
	}
	tui.PrintSuccess(fmt.Sprintf("Pull complete (%d/%d layers)", pullProgress.Done(), pullProgress.Total()))

	// Get information about the pulled image
	remoteInspect, _, err := cli.ImageInspectWithRaw(ctx, actualPullRef)
	if err != nil {
		common.PrintErrorMessage(err)
		return
	}

	// Compare local and remote images
	if localExists && localDigest != remoteInspect.ID {
		common.PrintInfoMessage("The pulled image is different from the local one.")
		if tui.Confirm("Do you want to rename the old image with a date tag?") {
			currentTime := time.Now()
			dateTag := fmt.Sprintf("%s-%02d%02d%d", imagetag, currentTime.Day(), currentTime.Month(), currentTime.Year())
			err = cli.ImageTag(ctx, localDigest, dateTag)
			if err != nil {
				common.PrintErrorMessage(err)
				return
			}
			common.PrintSuccessMessage(fmt.Sprintf("Old image '%s' retagged as '%s'", imagetag, dateTag))
		}
	}

	// Tag the pulled image with the clean name (without architecture suffix)
	if imagetag != actualPullRef {
		err = cli.ImageTag(ctx, remoteInspect.ID, imagetag)
		if err != nil {
			common.PrintErrorMessage(err)
			return
		}
		common.PrintSuccessMessage(fmt.Sprintf("Image tagged as '%s'", imagetag))

		// Remove the original architecture-suffixed tag to avoid duplicates in local listing
		if IsOfficialImage(actualPullRef) {
			_, err = cli.ImageRemove(ctx, actualPullRef, image.RemoveOptions{Force: false})
			if err != nil {
				// Only log if it's not a "tag not found" or "in use" error
				if !strings.Contains(err.Error(), "No such image") && !strings.Contains(err.Error(), "image is referenced") {
					log.Printf("Note: Could not remove architecture-suffixed tag %s: %v", actualPullRef, err)
				}
			} else {
				common.PrintInfoMessage(fmt.Sprintf("Removed architecture-suffixed tag: %s", actualPullRef))
			}
		}
	}

	// Warn about Podman root/rootless image store separation
	if GetEngine().Type() == EnginePodman {
		if os.Getuid() == 0 {
			common.PrintWarningMessage("Image pulled as root — it won't be visible in rootless mode.")
			common.PrintInfoMessage("To copy to rootless: podman image scp root@localhost::<image> <user>@localhost::")
		} else {
			common.PrintWarningMessage("Image pulled in rootless mode — it won't be visible with sudo.")
			common.PrintInfoMessage("To copy to root: sudo podman image scp <user>@localhost::<image> root@localhost::")
		}
	}

	common.PrintSuccessMessage(fmt.Sprintf("Image '%s' installed successfully", imagetag))
}

// ContainerTag renames a local image by tagging it with a new name and then
// removing the original tag reference (the underlying image layers are kept).
//
//	in(1): string imageref  source image reference to rename
//	in(2): string imagetag  new tag name to assign to the image
func ContainerTag(imageref string, imagetag string) {
	ctx := context.Background()
	cli, err := NewEngineClient()
	if err != nil {
		panic(err)
	}
	defer cli.Close()
	// Normalize source image reference
	imageref = normalizeImageName(imageref)
	err = cli.ImageTag(ctx, imageref, imagetag)
	if err != nil {
		panic(err)
	}

	// Remove old tag (only removes the reference, not the image layers)
	_, err = cli.ImageRemove(ctx, imageref, image.RemoveOptions{
		Force:         false,
		PruneChildren: false,
	})
	if err != nil {
		common.PrintWarningMessage(fmt.Sprintf("Retagged to '%s' but could not remove old tag '%s': %v", imagetag, imageref, err))
	} else {
		common.PrintSuccessMessage(fmt.Sprintf("Image renamed: '%s' → '%s'", imageref, imagetag))
	}
}

// ListImages returns all locally stored images that carry the given label
// key/value pair and have at least one RepoTag.
//
//	in(1): string labelKey    Docker label key to filter on
//	in(2): string labelValue  required value for the label key
//	out:   []image.Summary    slice of matching image summaries
//	out:   error              non-nil if the engine client could not be created or the list call failed
func ListImages(labelKey string, labelValue string) ([]image.Summary, error) {
	ctx := context.Background()
	cli, err := NewEngineClient()
	if err != nil {
		return nil, err
	}
	defer cli.Close()

	// Filter images by the specified image label
	imagesFilters := filters.NewArgs()
	imagesFilters.Add("label", fmt.Sprintf("%s=%s", labelKey, labelValue))

	images, err := cli.ImageList(ctx, image.ListOptions{
		All:     true,
		Filters: imagesFilters,
	})
	if err != nil {
		return nil, err
	}

	// Only display images with RepoTags
	var filteredImages []image.Summary
	for _, image := range images {
		if len(image.RepoTags) > 0 {
			filteredImages = append(filteredImages, image)
		}
	}

	return filteredImages, nil
}

// ListImageTags returns a flat list of all repo:tag strings for RF Swift images.
func ListImageTags(labelKey string, labelValue string) []string {
	images, err := ListImages(labelKey, labelValue)
	if err != nil {
		return nil
	}
	var tags []string
	for _, img := range images {
		tags = append(tags, img.RepoTags...)
	}
	return tags
}

// PrintImagesTable prints a formatted terminal table of RF Swift images filtered
// by a Docker label, optionally showing resolved version strings and restricting
// rows to tags that contain filterImage as a substring.
//
//	in(1): string labelKey    Docker label key used to select RF Swift images
//	in(2): string labelValue  required value for the label key
//	in(3): bool showVersions  when true, appends a Version column resolved from remote metadata
//	in(4): string filterImage substring filter applied to tag names; empty string disables filtering
func PrintImagesTable(labelKey string, labelValue string, showVersions bool, filterImage string) {
	ctx := context.Background()
	cli, err := NewEngineClient()
	if err != nil {
		log.Fatalf("Error creating Docker client: %v", err)
	}
	defer cli.Close()

	images, err := ListImages(labelKey, labelValue)
	if err != nil {
		log.Fatalf("Error listing images: %v", err)
	}

	rfutils.ClearScreen()

	// Fetch remote versions ONCE for all checks - BY REPO
	architecture := getArchitecture()
	var remoteVersionsByRepo RepoVersionMap
	if !common.Disconnected {
		remoteVersionsByRepo = GetAllRemoteVersionsByRepo(architecture)
	} else {
		remoteVersionsByRepo = make(RepoVersionMap)
	}

	// Prepare table data
	tableData := [][]string{}
	maxStatusLength := 0
	maxVersionLength := 0

	for _, image := range images {
		for _, repoTag := range image.RepoTags {
			repoTagParts := strings.Split(repoTag, ":")
			if len(repoTagParts) != 2 {
				continue
			}
			repository := repoTagParts[0]
			tag := repoTagParts[1]

			// Apply filter if specified
			if filterImage != "" && !strings.Contains(strings.ToLower(tag), strings.ToLower(filterImage)) {
				continue
			}

			// Check image status using cached versions BY REPO
			isUpToDate, isCustom, err := checkImageStatusWithCache(ctx, cli, repository, tag, architecture, remoteVersionsByRepo)
			var status string
			if err != nil {
				status = "Error"
			} else if isCustom {
				status = "Custom"
				if common.Disconnected {
					status = "No network"
				}
			} else if isUpToDate {
				status = "Up to date"
			} else {
				status = "Obsolete"
			}

			if len(status) > maxStatusLength {
				maxStatusLength = len(status)
			}

			created := time.Unix(image.Created, 0).Format(time.RFC3339)
			size := fmt.Sprintf("%.2f MB", float64(image.Size)/1024/1024)

			// Get version info from cached data FOR THIS REPO
			versionDisplay := ""
			if showVersions {
				baseName, existingVersion := parseTagVersion(tag)

				// If tag already has a version, display it
				if existingVersion != "" {
					versionDisplay = existingVersion
				} else {
					// Get local image digest and find matching version in THIS REPO
					localDigest := getLocalImageDigests(ctx, cli, repoTag)
					if len(localDigest) > 0 {
						if repoVersions, ok := remoteVersionsByRepo[repository]; ok {
							if versions, ok := repoVersions[baseName]; ok {
								matchedVersion := GetVersionForDigests(versions, localDigest)
								if matchedVersion != "" {
									versionDisplay = matchedVersion
								}
							}
						}
					}
				}

				if versionDisplay == "" {
					versionDisplay = "-"
				}

				if len(versionDisplay) > maxVersionLength {
					maxVersionLength = len(versionDisplay)
				}
			}

			row := []string{
				repository,
				tag,
				image.ID[7:19], // sha256: prefix removed, first 12 chars
				created,
				size,
				status,
			}

			if showVersions {
				row = append(row, versionDisplay)
			}

			tableData = append(tableData, row)
		}
	}

	// Build headers
	headers := []string{"Repository", "Tag", "Image ID", "Created", "Size", "Status"}
	if showVersions {
		headers = append(headers, "Version")
	}

	statusCol := 5
	versionCol := 6

	tui.RenderTable(tui.TableConfig{
		Title:      "📦 RF Swift Images",
		TitleColor: tui.ColorWarning,
		Headers:    headers,
		Rows:       tableData,
		ColorFunc:  tui.ImageTableColorFunc(statusCol, versionCol, showVersions),
	})
}

// DeleteImage removes a local image by ID or tag after interactively confirming
// with the user, stopping and removing any containers that reference the image first.
//
//	in(1): string imageIDOrTag  image ID (short or full) or repo:tag reference to delete
//	out:   error                non-nil if the image was not found, the user cancelled, or removal failed
func DeleteImage(imageIDOrTag string) error {
	ctx := context.Background()
	cli, err := NewEngineClient()
	if err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to create Docker client: %v", err))
		return err
	}
	defer cli.Close()

	// Normalize image reference (but not if it looks like an ID)
	if !strings.HasPrefix(imageIDOrTag, "sha256:") && len(imageIDOrTag) != 12 && len(imageIDOrTag) != 64 {
		imageIDOrTag = normalizeImageName(imageIDOrTag)
	}

	common.PrintInfoMessage(fmt.Sprintf("Attempting to delete image: %s", imageIDOrTag))

	// List all images
	images, err := cli.ImageList(ctx, image.ListOptions{All: true})
	if err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to list images: %v", err))
		return err
	}

	var imageToDelete image.Summary
	imageFound := false

	// Get current architecture for matching
	architecture := getArchitecture()

	for _, img := range images {
		// Check if the full image ID matches
		if img.ID == "sha256:"+imageIDOrTag || img.ID == imageIDOrTag {
			imageToDelete = img
			imageFound = true
			break
		}

		// Check if any RepoTags match
		for _, tag := range img.RepoTags {
			normalizedTag := tag

			// If the input doesn't contain ":", prepend the repo
			if !strings.Contains(imageIDOrTag, ":") {
				imageIDOrTag = fmt.Sprintf("%s:%s", containerCfg.repotag, imageIDOrTag)
			}

			// Check for exact match first
			if normalizedTag == imageIDOrTag {
				imageToDelete = img
				imageFound = true
				break
			}

			// For official images, also check with architecture suffix
			if IsOfficialImage(imageIDOrTag) {
				// Extract repo and tag from the search term
				parts := strings.Split(imageIDOrTag, ":")
				if len(parts) == 2 {
					repo := parts[0]
					searchTag := parts[1]

					// Try matching with architecture suffix
					tagWithArch := fmt.Sprintf("%s:%s_%s", repo, searchTag, architecture)
					if normalizedTag == tagWithArch {
						imageToDelete = img
						imageFound = true
						break
					}
				}
			}

			// Also check if the stored tag (which might have arch suffix) matches
			// when we strip the architecture suffix from it
			cleanTag := normalizedTag
			parts := strings.Split(cleanTag, ":")
			if len(parts) == 2 {
				tagPart := parts[1]
				cleanedTagPart := removeArchitectureSuffix(tagPart)
				cleanTag = fmt.Sprintf("%s:%s", parts[0], cleanedTagPart)

				if cleanTag == imageIDOrTag {
					imageToDelete = img
					imageFound = true
					break
				}
			}
		}

		// If image is found by tag, break the outer loop
		if imageFound {
			break
		}
	}

	if !imageFound {
		common.PrintErrorMessage(fmt.Errorf("image not found: %s", imageIDOrTag))
		common.PrintInfoMessage("Available images:")
		for _, img := range images {
			// Display tags with clean names
			displayTags := []string{}
			for _, tag := range img.RepoTags {
				parts := strings.Split(tag, ":")
				if len(parts) == 2 {
					cleanTagPart := removeArchitectureSuffix(parts[1])
					displayTags = append(displayTags, fmt.Sprintf("%s:%s", parts[0], cleanTagPart))
				} else {
					displayTags = append(displayTags, tag)
				}
			}
			common.PrintInfoMessage(fmt.Sprintf("ID: %s, Tags: %v", strings.TrimPrefix(img.ID, "sha256:"), displayTags))
		}
		return fmt.Errorf("image not found: %s", imageIDOrTag)
	}

	imageID := imageToDelete.ID

	// Display clean tag names in the confirmation
	displayTags := []string{}
	for _, tag := range imageToDelete.RepoTags {
		parts := strings.Split(tag, ":")
		if len(parts) == 2 {
			cleanTagPart := removeArchitectureSuffix(parts[1])
			displayTags = append(displayTags, fmt.Sprintf("%s:%s", parts[0], cleanTagPart))
		} else {
			displayTags = append(displayTags, tag)
		}
	}

	common.PrintInfoMessage(fmt.Sprintf("Found image to delete: ID: %s, Tags: %v", strings.TrimPrefix(imageID, "sha256:"), displayTags))

	// Ask for user confirmation
	if !tui.Confirm("Are you sure you want to delete this image?") {
		common.PrintInfoMessage("Image deletion cancelled by user.")
		return nil
	}

	// Find and remove containers using the image
	containers, err := cli.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to list containers: %v", err))
		return err
	}

	for _, icontainer := range containers {
		if icontainer.ImageID == imageID {
			common.PrintWarningMessage(fmt.Sprintf("Removing container: %s", icontainer.ID[:12]))
			if err := cli.ContainerRemove(ctx, icontainer.ID, container.RemoveOptions{Force: true}); err != nil {
				common.PrintWarningMessage(fmt.Sprintf("Failed to remove container %s: %v", icontainer.ID[:12], err))
			}
		}
	}

	// Attempt to delete the image
	_, err = cli.ImageRemove(ctx, imageID, image.RemoveOptions{Force: true, PruneChildren: true})
	if err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to delete image %s: %v", imageIDOrTag, err))
		return err
	}

	common.PrintSuccessMessage(fmt.Sprintf("Successfully deleted image: %s", imageIDOrTag))
	return nil
}

// SaveImageToFile exports a local image to a gzip-compressed tar archive,
// optionally pulling the latest version from the remote registry first.
//
//	in(1): string imageName   fully-qualified image name to export
//	in(2): string outputFile  destination file path for the .tar.gz archive
//	in(3): bool pullFirst     when true, always pull the latest image before saving
//	out:   error              non-nil if pulling, saving, or writing the archive failed
func SaveImageToFile(imageName string, outputFile string, pullFirst bool) error {
	ctx := context.Background()
	cli, err := NewEngineClient()
	if err != nil {
		return fmt.Errorf("failed to create Docker client: %v", err)
	}
	defer cli.Close()

	// Normalize image name
	imageName = normalizeImageName(imageName)

	// Check if image exists locally
	_, inspectErr := ImageInspectCompat(ctx, cli, imageName)
	imageExists := inspectErr == nil

	if !imageExists || pullFirst {
		// Need to pull the image
		if !imageExists {
			common.PrintInfoMessage(fmt.Sprintf("Image '%s' not found locally, pulling...", imageName))
		} else {
			common.PrintInfoMessage(fmt.Sprintf("Pulling latest version of '%s'...", imageName))
		}

		// Parse image name for architecture handling
		parts := strings.Split(imageName, ":")
		repo := parts[0]
		tag := "latest"
		if len(parts) > 1 {
			tag = parts[1]
		}

		// Check if this is an official image
		isOfficial := IsOfficialImage(imageName)
		architecture := getArchitecture()
		actualPullRef := imageName

		// Handle architecture suffix for official images
		if isOfficial && architecture != "" {
			hasArchSuffix := strings.HasSuffix(tag, "_amd64") ||
				strings.HasSuffix(tag, "_arm64") ||
				strings.HasSuffix(tag, "_riscv64") ||
				strings.HasSuffix(tag, "_arm")

			if !hasArchSuffix {
				actualPullRef = fmt.Sprintf("%s:%s_%s", repo, tag, architecture)
				common.PrintInfoMessage(fmt.Sprintf("Using architecture-specific tag: %s", actualPullRef))
			}
		}

		// Pull the image
		out, err := cli.ImagePull(ctx, actualPullRef, image.PullOptions{})
		if err != nil {
			return fmt.Errorf("failed to pull image: %v", err)
		}
		defer out.Close()

		// Show pull progress
		termFd, isTerm := term.GetFdInfo(os.Stdout)
		if isTerm {
			jsonmessage.DisplayJSONMessagesStream(out, os.Stdout, termFd, isTerm, nil)
		} else {
			// Read to completion even if not displaying
			io.Copy(io.Discard, out)
		}

		// Tag if needed (for official images with architecture suffix)
		if actualPullRef != imageName && isOfficial {
			remoteInspect, _, _ := cli.ImageInspectWithRaw(ctx, actualPullRef)
			if remoteInspect.ID != "" {
				cli.ImageTag(ctx, remoteInspect.ID, imageName)
				common.PrintSuccessMessage(fmt.Sprintf("Tagged as: %s", imageName))
			}
		}

		common.PrintSuccessMessage("Image pulled successfully")
	} else {
		common.PrintSuccessMessage(fmt.Sprintf("Using local image: %s", imageName))
	}

	// Now save the image to file
	common.PrintInfoMessage(fmt.Sprintf("Saving image '%s' to %s", imageName, outputFile))

	// Save image
	reader, err := cli.ImageSave(ctx, []string{imageName})
	if err != nil {
		return fmt.Errorf("failed to save image: %v", err)
	}
	defer reader.Close()

	// Create output file
	outFile, err := os.Create(outputFile)
	if err != nil {
		return fmt.Errorf("failed to create output file: %v", err)
	}
	defer outFile.Close()

	// Create gzip writer
	gzipWriter := gzip.NewWriter(outFile)
	defer gzipWriter.Close()

	// Copy with progress
	common.PrintInfoMessage("Compressing image data...")
	written, err := io.Copy(gzipWriter, reader)
	if err != nil {
		return fmt.Errorf("failed to write compressed data: %v", err)
	}

	common.PrintSuccessMessage(fmt.Sprintf("Image saved successfully: %s (%.2f MB)",
		outputFile, float64(written)/(1024*1024)))

	// Show file info
	fileInfo, _ := os.Stat(outputFile)
	if fileInfo != nil {
		common.PrintInfoMessage(fmt.Sprintf("Compressed file size: %.2f MB", float64(fileInfo.Size())/(1024*1024)))
	}

	return nil
}

// ContainerPullVersion pulls a specific versioned image from the remote registry
// by locating the requested version across all official repositories, then
// tagging the result with a clean local name that omits the architecture suffix.
//
//	in(1): string imageref  base image reference used to scope the version search
//	in(2): string version   semantic version string to look up (e.g. "0.0.7"); empty delegates to ContainerPull
//	in(3): string imagetag  local tag to assign after pulling; if empty a name is derived from repo and version
func ContainerPullVersion(imageref string, version string, imagetag string) {
	ctx := context.Background()
	cli, err := NewEngineClient()
	if err != nil {
		common.PrintErrorMessage(err)
		return
	}
	defer cli.Close()

	architecture := getArchitecture()
	if architecture == "" {
		common.PrintErrorMessage(fmt.Errorf("unsupported architecture"))
		return
	}

	// If no version specified, use regular pull
	if version == "" {
		ContainerPull(imageref, imagetag)
		return
	}

	common.PrintInfoMessage(fmt.Sprintf("Looking for version %s of %s...", version, imageref))

	// Find the version in remote repos
	repo, digest, baseName, err := FindVersionInRemote(imageref, version, architecture)
	if err != nil {
		common.PrintErrorMessage(err)
		common.PrintInfoMessage("Use 'rfswift images versions' to see available versions")
		return
	}

	common.PrintSuccessMessage(fmt.Sprintf("Found version %s in %s", version, repo))
	common.PrintInfoMessage(fmt.Sprintf("Digest: %s", digest[:min(32, len(digest))]))

	// Build the versioned tag name with architecture
	// Format: baseName_version_architecture (e.g., reversing_0.0.7_amd64)
	versionedTag := fmt.Sprintf("%s_%s_%s", baseName, version, architecture)
	pullRef := fmt.Sprintf("%s:%s", repo, versionedTag)

	// Set display tag - use underscore format without 'v' prefix
	// Format: repo:baseName_version (e.g., penthertz/rfswift_noble:reversing_0.0.7)
	if imagetag == "" {
		imagetag = fmt.Sprintf("%s:%s_%s", repo, baseName, version)
	}

	common.PrintInfoMessage(fmt.Sprintf("Pulling %s...", pullRef))

	out, err := cli.ImagePull(ctx, pullRef, image.PullOptions{})
	if err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to pull image: %v", err))
		return
	}
	defer out.Close()

	// Process pull output with per-layer progress
	pullProgress := tui.NewLayerProgress()
	jsonDecoder := json.NewDecoder(out)
	for {
		var msg jsonmessage.JSONMessage
		if err := jsonDecoder.Decode(&msg); err == io.EOF {
			break
		} else if err != nil {
			common.PrintErrorMessage(err)
			return
		}
		if msg.ID != "" {
			var current, total int64
			if msg.Progress != nil {
				current = msg.Progress.Current
				total = msg.Progress.Total
			}
			pullProgress.Update(msg.ID, msg.Status, current, total)
			if pullProgress.Total() > 0 {
				fmt.Printf("\033[%dA\033[J", pullProgress.Total())
			}
			fmt.Print(pullProgress.Render())
		} else if msg.Status != "" {
			fmt.Printf("  %s\n", msg.Status)
		}
	}
	tui.PrintSuccess(fmt.Sprintf("Pull complete (%d/%d layers)", pullProgress.Done(), pullProgress.Total()))

	// Get the pulled image info
	remoteInspect, _, err := cli.ImageInspectWithRaw(ctx, pullRef)
	if err != nil {
		common.PrintErrorMessage(err)
		return
	}

	// Tag with friendly name (without architecture suffix)
	err = cli.ImageTag(ctx, remoteInspect.ID, imagetag)
	if err != nil {
		common.PrintErrorMessage(err)
		return
	}

	common.PrintSuccessMessage(fmt.Sprintf("Image tagged as '%s'", imagetag))

	// Optionally remove the architecture-suffixed tag to keep things clean
	_, err = cli.ImageRemove(ctx, pullRef, image.RemoveOptions{Force: false})
	if err == nil {
		common.PrintInfoMessage(fmt.Sprintf("Removed architecture-suffixed tag: %s", pullRef))
	}

	common.PrintSuccessMessage(fmt.Sprintf("Version %s installed successfully!", version))
}
