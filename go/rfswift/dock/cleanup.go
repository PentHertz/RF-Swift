/* This code is part of RF Switch by @Penthertz
 * Author(s): Sebastien Dudek (@FlUxIuS)
 *
 * Cleanup and pruning operations
 *
 * parseDuration              - in(1): string duration, out: time.Duration, error
 * formatAge                  - in(1): time.Duration d, out: string
 * CleanupAll                 - in(1): string olderThan, in(2): bool force, in(3): bool dryRun, out: error
 * CleanupContainers          - in(1): string olderThan, in(2): bool force, in(3): bool dryRun, in(4): bool onlyStopped, out: error
 * CleanupImages              - in(1): string olderThan, in(2): bool force, in(3): bool dryRun, in(4): bool onlyDangling, in(5): bool pruneChildren, out: error
 * getChildImages             - in(1): context.Context, in(2): *client.Client, in(3): string parentID, out: []image.Summary
 * getAllDescendants           - in(1): context.Context, in(2): *client.Client, in(3): string parentID, out: []image.Summary
 * removeImageWithDescendants - in(1): context.Context, in(2): *client.Client, in(3): string imageID, in(4): string displayName, out: int, error
 */
package dock

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"

	common "penthertz/rfswift/common"
)

// parseDuration parses duration strings like "24h", "7d", "1m", "1y".
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

// formatAge formats a duration into a human-readable string (e.g., "3d 5h", "2mo 10d").
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

// CleanupAll removes both old containers and images.
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

// CleanupContainers removes old RF Swift containers.
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
		reader := bufio.NewReader(os.Stdin)
		common.PrintWarningMessage(fmt.Sprintf("Are you sure you want to remove %d container(s)? (y/n): ", len(toDelete)))
		response, _ := reader.ReadString('\n')
		response = strings.ToLower(strings.TrimSpace(response))
		if response != "y" && response != "yes" {
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
		}
	}

	common.PrintSuccessMessage(fmt.Sprintf("Cleanup complete: removed %d/%d container(s)", removed, len(toDelete)))
	return nil
}

// CleanupImages removes old RF Swift images.
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
		reader := bufio.NewReader(os.Stdin)
		totalToRemove := len(toDelete)
		if pruneChildren {
			totalToRemove += totalDescendants
		}
		common.PrintWarningMessage(fmt.Sprintf("Are you sure you want to remove %d image(s) in total? (y/n): ", totalToRemove))
		response, _ := reader.ReadString('\n')
		response = strings.ToLower(strings.TrimSpace(response))
		if response != "y" && response != "yes" {
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
