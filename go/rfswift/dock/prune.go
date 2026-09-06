/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
*
*  Reclaim space on a container engine. Shared by the Workbench (local engine
*  doctor) and the remote agent (the same action for a connected Workbench).
 */

package dock

import (
	"context"
	"fmt"
	"strings"

	"github.com/moby/moby/client"
)

// PruneOptions selects what a reclaim pass removes.
type PruneOptions struct {
	Images       bool `json:"images"`       // dangling images only (untagged layers, always safe)
	UnusedImages bool `json:"unusedImages"` // every image no container references, tagged ones included
	BuildCache   bool `json:"buildCache"`
	Volumes      bool `json:"volumes"`  // unused volumes only
	Networks     bool `json:"networks"` // unused networks only
}

// Any reports whether at least one target is selected.
func (o PruneOptions) Any() bool {
	return o.Images || o.UnusedImages || o.BuildCache || o.Volumes || o.Networks
}

// PruneSummary reports what a reclaim pass freed.
type PruneSummary struct {
	Reclaimed    uint64 `json:"reclaimed"` // bytes
	ReclaimedStr string `json:"reclaimedStr"`
	Images       int    `json:"images"`
	CacheEntries int    `json:"cacheEntries"`
	Volumes      int    `json:"volumes"`
	Networks     int    `json:"networks"`
	Detail       string `json:"detail"`
}

// EngineByType returns the engine driver for a type name, nil for unknown.
func EngineByType(t EngineType) ContainerEngine {
	switch t {
	case EngineDocker:
		return &DockerEngine{}
	case EnginePodman:
		return &PodmanEngine{}
	case EngineLima:
		return &LimaEngine{}
	}
	return nil
}

// PruneEngine reclaims space on one container engine (docker/podman, or the
// Docker daemon inside the Lima VM). Images prunes dangling-only; UnusedImages
// prunes every image no container references (tagged images left by deleted
// containers must be pulled again later). Build cache is pruned fully,
// volumes and networks only when unused. Each target is best-effort: a daemon
// that does not support one (Podman has no build-cache endpoint) does not
// fail the whole pass, it is reported in Detail.
func PruneEngine(eng ContainerEngine, opts PruneOptions) (PruneSummary, error) {
	var s PruneSummary
	if eng == nil {
		return s, fmt.Errorf("unknown engine")
	}
	if !opts.Any() {
		return s, fmt.Errorf("select at least one target")
	}
	if !eng.IsServiceRunning() {
		return s, fmt.Errorf("%s is not running", eng.Name())
	}
	cli, err := eng.GetClient()
	if err != nil {
		return s, err
	}
	defer cli.Close()
	ctx := context.Background()
	var notes []string

	if opts.UnusedImages || opts.Images {
		o := client.ImagePruneOptions{}
		if opts.UnusedImages {
			// dangling=false widens the prune to every image not used by a
			// container (includes the dangling set).
			f := make(client.Filters)
			f.Add("dangling", "false")
			o.Filters = f
		}
		if r, e := cli.ImagePrune(ctx, o); e == nil {
			s.Reclaimed += r.Report.SpaceReclaimed
			s.Images += len(r.Report.ImagesDeleted)
		} else {
			notes = append(notes, "images: "+cleanPruneErr(e))
		}
	}
	if opts.BuildCache {
		if r, e := cli.BuildCachePrune(ctx, client.BuildCachePruneOptions{All: true}); e == nil {
			s.Reclaimed += r.Report.SpaceReclaimed
			s.CacheEntries += len(r.Report.CachesDeleted)
		} else {
			notes = append(notes, "build cache: "+cleanPruneErr(e))
		}
	}
	if opts.Volumes {
		if r, e := cli.VolumePrune(ctx, client.VolumePruneOptions{}); e == nil {
			s.Reclaimed += r.Report.SpaceReclaimed
			s.Volumes += len(r.Report.VolumesDeleted)
		} else {
			notes = append(notes, "volumes: "+cleanPruneErr(e))
		}
	}
	if opts.Networks {
		if r, e := cli.NetworkPrune(ctx, client.NetworkPruneOptions{}); e == nil {
			s.Networks += len(r.Report.NetworksDeleted)
		} else {
			notes = append(notes, "networks: "+cleanPruneErr(e))
		}
	}
	s.ReclaimedStr = HumanBytes(s.Reclaimed)
	parts := []string{fmt.Sprintf("reclaimed %s", s.ReclaimedStr)}
	if s.Images > 0 {
		parts = append(parts, fmt.Sprintf("%d image(s)", s.Images))
	}
	if s.CacheEntries > 0 {
		parts = append(parts, fmt.Sprintf("%d cache entr(ies)", s.CacheEntries))
	}
	if s.Volumes > 0 {
		parts = append(parts, fmt.Sprintf("%d volume(s)", s.Volumes))
	}
	if s.Networks > 0 {
		parts = append(parts, fmt.Sprintf("%d network(s)", s.Networks))
	}
	s.Detail = strings.Join(parts, ", ")
	if len(notes) > 0 {
		s.Detail += " (skipped: " + strings.Join(notes, "; ") + ")"
	}
	return s, nil
}

// cleanPruneErr turns "not supported" daemon errors (Podman has no build-cache
// prune endpoint, so it answers Not Found) into a readable note.
func cleanPruneErr(e error) string {
	msg := e.Error()
	low := strings.ToLower(msg)
	if strings.Contains(low, "not found") || strings.Contains(low, "not implemented") || strings.Contains(low, "404") {
		return "not supported by this engine"
	}
	return msg
}

// HumanBytes formats a byte count with binary units.
func HumanBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}
