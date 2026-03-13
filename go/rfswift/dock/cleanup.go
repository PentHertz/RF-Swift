/* This code is part of RF Switch by @Penthertz
 * Author(s): Sebastien Dudek (@FlUxIuS)
 */
package dock

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"

	common "penthertz/rfswift/common"
	"penthertz/rfswift/tui"
)

// parseDuration parses a human-readable duration string into a time.Duration.
// Accepted units are h (hours), d (days), m (months, 30-day), and y (years, 365-day).
//
//	in(1): string duration  the duration string to parse (e.g. "24h", "7d", "1m", "1y")
//	out: time.Duration      the parsed duration
//	out: error              non-nil if the format or unit is unrecognised
func parseDuration(duration string) (time.Duration, error) {
	if duration == "" {
		return 0, nil
	}

	var value int
	var unit string
	_, err := fmt.Sscanf(duration, "%d%s", &value, &unit)
	if err != nil {
		return 0, fmt.Errorf("invalid duration format: %s (use format like '24h', '7d', '1m', '1y')", duration)
	}

	switch unit {
	case "h":
		return time.Duration(value) * time.Hour, nil
	case "d":
		return time.Duration(value) * 24 * time.Hour, nil
	case "m":
		return time.Duration(value) * 30 * 24 * time.Hour, nil
	case "y":
		return time.Duration(value) * 365 * 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("invalid duration unit: %s (use h, d, m, or y)", unit)
	}
}

// formatAge converts a duration into a concise human-readable age string
// (e.g. "3d 5h", "2mo 10d", "1y 30d").
//
//	in(1): time.Duration d  the duration to format
//	out: string             human-readable representation of the duration
func formatAge(d time.Duration) string {
	days := int(d.Hours() / 24)
	hours := int(d.Hours()) % 24

	if days > 365 {
		years := days / 365
		remainingDays := days % 365
		if remainingDays > 0 {
			return fmt.Sprintf("%dy %dd", years, remainingDays)
		}
		return fmt.Sprintf("%dy", years)
	} else if days > 30 {
		months := days / 30
		remainingDays := days % 30
		if remainingDays > 0 {
			return fmt.Sprintf("%dmo %dd", months, remainingDays)
		}
		return fmt.Sprintf("%dmo", months)
	} else if days > 0 {
		if hours > 0 {
			return fmt.Sprintf("%dd %dh", days, hours)
		}
		return fmt.Sprintf("%dd", days)
	} else {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
}

// CleanupAll removes RF Swift containers and images in sequence, applying
// the same age filter and confirmation behaviour to both resource types.
//
//	in(1): string olderThan  only remove resources older than this duration (e.g. "7d"); empty means no age filter
//	in(2): bool force        skip interactive confirmation prompt when true
//	in(3): bool dryRun       list resources that would be removed without actually deleting them when true
//	out: error               non-nil if container or image cleanup fails
func CleanupAll(olderThan string, force bool, dryRun bool) error {
	common.PrintInfoMessage("Cleaning up containers and images...")

	if err := CleanupContainers(olderThan, force, dryRun, false); err != nil {
		return err
	}

	fmt.Println()

	if err := CleanupImages(olderThan, force, dryRun, false, true); err != nil {
		return err
	}

	return nil
}

// CleanupContainers lists and removes RF Swift containers that match the
// supplied age and state criteria, with optional dry-run and force modes.
//
//	in(1): string olderThan   only consider containers created before this duration ago; empty means all ages
//	in(2): bool force         skip interactive confirmation prompt when true
//	in(3): bool dryRun        report what would be removed without deleting anything when true
//	in(4): bool onlyStopped   skip running containers when true
//	out: error                non-nil if the engine client cannot be created, the container list fails, or removal fails
func CleanupContainers(olderThan string, force bool, dryRun bool, onlyStopped bool) error {
	ctx := context.Background()
	cli, err := NewEngineClient()
	if err != nil {
		return fmt.Errorf("failed to create Docker client: %v", err)
	}
	defer cli.Close()

	duration, err := parseDuration(olderThan)
	if err != nil {
		return err
	}

	cutoffTime := time.Now().Add(-duration)

	containerFilters := filters.NewArgs()
	containerFilters.Add("label", "org.container.project=rfswift")

	containers, err := cli.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: containerFilters,
	})
	if err != nil {
		return fmt.Errorf("failed to list containers: %v", err)
	}

	var toDelete []types.Container
	for _, cont := range containers {
		if onlyStopped && cont.State == "running" {
			continue
		}
		created := time.Unix(cont.Created, 0)
		if olderThan != "" && created.After(cutoffTime) {
			continue
		}
		toDelete = append(toDelete, cont)
	}

	if len(toDelete) == 0 {
		common.PrintInfoMessage("No containers to remove")
		return nil
	}

	cyan := "\033[36m"
	reset := "\033[0m"
	fmt.Printf("%s🗑️  Containers to remove: %d%s\n", cyan, len(toDelete), reset)

	for _, cont := range toDelete {
		age := time.Since(time.Unix(cont.Created, 0))
		containerName := ""
		if len(cont.Names) > 0 {
			containerName = cont.Names[0]
			if len(containerName) > 0 && containerName[0] == '/' {
				containerName = containerName[1:]
			}
		} else {
			containerName = cont.ID[:12]
		}

		status := cont.State
		if status == "running" {
			status = "\033[32m" + status + "\033[0m"
		} else {
			status = "\033[31m" + status + "\033[0m"
		}

		fmt.Printf("  • %s (%s) - Age: %s - Status: %s\n",
			containerName, cont.ID[:12], formatAge(age), status)
	}
	fmt.Println()

	if dryRun {
		common.PrintWarningMessage("DRY RUN: No containers were actually removed")
		return nil
	}

	if !force {
		if !tui.Confirm(fmt.Sprintf("Are you sure you want to remove %d container(s)?", len(toDelete))) {
			common.PrintInfoMessage("Cleanup cancelled")
			return nil
		}
	}

	removed := 0
	for _, cont := range toDelete {
		containerName := ""
		if len(cont.Names) > 0 {
			containerName = cont.Names[0]
			if len(containerName) > 0 && containerName[0] == '/' {
				containerName = containerName[1:]
			}
		} else {
			containerName = cont.ID[:12]
		}

		// Check for NAT network before removing container
		hasNAT := false
		if natLabel, ok := cont.Labels["org.rfswift.nat_network"]; ok && natLabel != "" {
			hasNAT = true
		}

		err := cli.ContainerRemove(ctx, cont.ID, container.RemoveOptions{Force: true})
		if err != nil {
			if strings.Contains(err.Error(), "No such container") {
				common.PrintWarningMessage(fmt.Sprintf("Skipped ghost container: %s (already removed from engine)", containerName))
			} else {
				common.PrintWarningMessage(fmt.Sprintf("Failed to remove %s: %v", containerName, err))
			}
		} else {
			common.PrintSuccessMessage(fmt.Sprintf("Removed container: %s", containerName))
			removed++

			// Clean up associated NAT network (skip shared networks that still have containers)
			if hasNAT {
				natNet := cont.Labels["org.rfswift.nat_network"]
				if natNet != "" && isSharedNATNetwork(ctx, cli, natNet) {
					if countContainersOnNetwork(ctx, cli, natNet) == 0 {
						removeNATNetworkByFullName(ctx, cli, natNet)
					}
				} else {
					removeNATNetwork(ctx, cli, containerName)
				}
			}
		}
	}

	common.PrintSuccessMessage(fmt.Sprintf("Cleanup complete: removed %d/%d container(s)", removed, len(toDelete)))
	return nil
}

// CleanupImages lists and removes RF Swift images that match the supplied age
// and dangling criteria, optionally recursing into descendant images before
// removing each parent.
//
//	in(1): string olderThan    only consider images created before this duration ago; empty means all ages
//	in(2): bool force          skip interactive confirmation prompt when true
//	in(3): bool dryRun         report what would be removed without deleting anything when true
//	in(4): bool onlyDangling   restrict the candidate set to untagged (dangling) images when true
//	in(5): bool pruneChildren  remove descendant images before removing each parent image when true
//	out: error                 non-nil if the engine client cannot be created, the image list fails, or removal fails
func CleanupImages(olderThan string, force bool, dryRun bool, onlyDangling bool, pruneChildren bool) error {
	ctx := context.Background()
	cli, err := NewEngineClient()
	if err != nil {
		return fmt.Errorf("failed to create Docker client: %v", err)
	}
	defer cli.Close()

	duration, err := parseDuration(olderThan)
	if err != nil {
		return err
	}

	cutoffTime := time.Now().Add(-duration)

	imageFilters := filters.NewArgs()
	imageFilters.Add("label", "org.container.project=rfswift")
	if onlyDangling {
		imageFilters.Add("dangling", "true")
	}

	images, err := cli.ImageList(ctx, image.ListOptions{
		All:     true,
		Filters: imageFilters,
	})
	if err != nil {
		return fmt.Errorf("failed to list images: %v", err)
	}

	var toDelete []image.Summary
	for _, img := range images {
		if !onlyDangling && len(img.RepoTags) == 0 {
			continue
		}
		created := time.Unix(img.Created, 0)
		if olderThan != "" && created.After(cutoffTime) {
			continue
		}
		toDelete = append(toDelete, img)
	}

	if len(toDelete) == 0 {
		common.PrintInfoMessage("No images to remove")
		return nil
	}

	magenta := "\033[35m"
	reset := "\033[0m"
	fmt.Printf("%s🗑️  Images to remove: %d%s\n", magenta, len(toDelete), reset)

	var hasChildren bool
	totalDescendants := 0

	for _, img := range toDelete {
		age := time.Since(time.Unix(img.Created, 0))
		size := float64(img.Size) / (1024 * 1024)

		var displayName string
		if len(img.RepoTags) > 0 {
			displayName = img.RepoTags[0]
		} else {
			displayName = fmt.Sprintf("<none> (%s)", img.ID[7:19])
		}

		descendants := getAllDescendants(ctx, cli, img.ID)
		if len(descendants) > 0 {
			hasChildren = true
			totalDescendants += len(descendants)
			fmt.Printf("  • %s - Age: %s - Size: %.2f MB - ⚠️  %d descendant(s)\n",
				displayName, formatAge(age), size, len(descendants))
		} else {
			fmt.Printf("  • %s - Age: %s - Size: %.2f MB\n",
				displayName, formatAge(age), size)
		}
	}
	fmt.Println()

	if hasChildren {
		if pruneChildren {
			common.PrintInfoMessage(fmt.Sprintf("Will remove %d total descendant images", totalDescendants))
		} else {
			common.PrintWarningMessage("Some images have dependent descendant images. Use --prune-children to remove them first.")
		}
	}

	if dryRun {
		common.PrintWarningMessage("DRY RUN: No images were actually removed")
		return nil
	}

	if !force {
		totalToRemove := len(toDelete)
		if pruneChildren {
			totalToRemove += totalDescendants
		}
		if !tui.Confirm(fmt.Sprintf("Are you sure you want to remove %d image(s) in total?", totalToRemove)) {
			common.PrintInfoMessage("Cleanup cancelled")
			return nil
		}
	}

	removed := 0
	skipped := 0

	for _, img := range toDelete {
		var displayName string
		if len(img.RepoTags) > 0 {
			displayName = img.RepoTags[0]
		} else {
			displayName = img.ID[7:19]
		}

		descendants := getAllDescendants(ctx, cli, img.ID)

		if len(descendants) > 0 && !pruneChildren {
			common.PrintWarningMessage(fmt.Sprintf("Skipped %s: has %d descendant(s) (use --prune-children)", displayName, len(descendants)))
			skipped++
			continue
		}

		if pruneChildren {
			descendantsRemoved, err := removeImageWithDescendants(ctx, cli, img.ID, displayName)
			if err != nil {
				common.PrintWarningMessage(fmt.Sprintf("Failed to remove %s: %v", displayName, err))
			} else {
				common.PrintSuccessMessage(fmt.Sprintf("Removed image: %s (+ %d descendants)", displayName, descendantsRemoved))
				removed++
			}
		} else {
			_, err := cli.ImageRemove(ctx, img.ID, image.RemoveOptions{Force: true})
			if err != nil {
				if strings.Contains(err.Error(), "No such image") {
					common.PrintSuccessMessage(fmt.Sprintf("Removed image: %s (cascaded)", displayName))
					removed++
				} else {
					common.PrintWarningMessage(fmt.Sprintf("Failed to remove %s: %v", displayName, err))
				}
			} else {
				common.PrintSuccessMessage(fmt.Sprintf("Removed image: %s", displayName))
				removed++
			}
		}
	}

	if skipped > 0 {
		common.PrintInfoMessage(fmt.Sprintf("Skipped %d image(s) with descendants", skipped))
	}
	common.PrintSuccessMessage(fmt.Sprintf("Cleanup complete: removed %d/%d parent image(s)", removed, len(toDelete)))
	return nil
}

// getChildImages returns all images whose ParentID matches parentID.
//
//	in(1): context.Context ctx      context used for the image list API call
//	in(2): *client.Client cli       Docker/Podman engine client
//	in(3): string parentID          image ID whose direct children are sought
//	out: []image.Summary            slice of direct child images; empty on error or when none exist
func getChildImages(ctx context.Context, cli *client.Client, parentID string) []image.Summary {
	var children []image.Summary

	allImages, err := cli.ImageList(ctx, image.ListOptions{All: true})
	if err != nil {
		return children
	}

	for _, img := range allImages {
		if img.ParentID == parentID {
			children = append(children, img)
		}
	}

	return children
}

// getAllDescendants recursively collects all descendant images of parentID,
// performing a depth-first traversal through child layers.
//
//	in(1): context.Context ctx   context forwarded to getChildImages for each level
//	in(2): *client.Client cli    Docker/Podman engine client
//	in(3): string parentID       image ID whose full descendant tree is sought
//	out: []image.Summary         flat slice of all descendant images in traversal order
func getAllDescendants(ctx context.Context, cli *client.Client, parentID string) []image.Summary {
	var descendants []image.Summary

	children := getChildImages(ctx, cli, parentID)

	for _, child := range children {
		descendants = append(descendants, child)
		grandchildren := getAllDescendants(ctx, cli, child.ID)
		descendants = append(descendants, grandchildren...)
	}

	return descendants
}

// removeImageWithDescendants removes all descendant images of imageID in
// reverse traversal order and then removes imageID itself. Already-removed
// images encountered during the walk are silently skipped.
//
//	in(1): context.Context ctx   context used for inspect and remove API calls
//	in(2): *client.Client cli    Docker/Podman engine client
//	in(3): string imageID        ID of the parent image to remove along with its descendants
//	in(4): string displayName    human-readable label for imageID used in log messages
//	out: int                     number of descendant images successfully removed (excludes the parent)
//	out: error                   non-nil if the final removal of imageID fails for a reason other than "no such image"
func removeImageWithDescendants(ctx context.Context, cli *client.Client, imageID string, displayName string) (int, error) {
	removedCount := 0

	descendants := getAllDescendants(ctx, cli, imageID)

	if len(descendants) > 0 {
		common.PrintInfoMessage(fmt.Sprintf("Removing %d descendant image(s) for %s...", len(descendants), displayName))

		for i := len(descendants) - 1; i >= 0; i-- {
			desc := descendants[i]

			_, _, err := cli.ImageInspectWithRaw(ctx, desc.ID)
			if err != nil {
				continue
			}

			var descName string
			if len(desc.RepoTags) > 0 {
				descName = desc.RepoTags[0]
			} else {
				descName = desc.ID[7:19]
			}

			_, err = cli.ImageRemove(ctx, desc.ID, image.RemoveOptions{Force: true, PruneChildren: true})
			if err != nil {
				if strings.Contains(err.Error(), "No such image") {
					continue
				}
				common.PrintWarningMessage(fmt.Sprintf("  Failed to remove descendant %s: %v", descName, err))
			} else {
				common.PrintSuccessMessage(fmt.Sprintf("  Removed descendant: %s", descName))
				removedCount++
			}
		}
	}

	_, _, err := cli.ImageInspectWithRaw(ctx, imageID)
	if err != nil {
		common.PrintSuccessMessage(fmt.Sprintf("Image %s was already removed (cascaded)", displayName))
		return removedCount, nil
	}

	_, err = cli.ImageRemove(ctx, imageID, image.RemoveOptions{Force: true, PruneChildren: true})
	if err != nil && strings.Contains(err.Error(), "No such image") {
		return removedCount, nil
	}

	return removedCount, err
}
