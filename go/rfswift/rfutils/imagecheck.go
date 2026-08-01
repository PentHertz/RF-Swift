/* This code is part of RF Swift by @Penthertz
*  Detects when the configured/used container image is an official RF-Swift
*  image built on an older Ubuntu base than the current one, and nudges the user.
 */

package rfutils

import (
	"fmt"
	"strings"
	"sync"

	common "penthertz/rfswift/common"
)

// officialImageCodename returns the Ubuntu-base codename of an official RF-Swift
// image reference and whether the reference is an official Penthertz RF-Swift
// image at all. It recognises the release images (penthertz/rfswift_<codename>)
// and the dev images (penthertz/rfswiftdev_<codename>). Any tag or digest is
// ignored, e.g. "penthertz/rfswift_noble:sdr_light" -> ("noble", true).
func officialImageCodename(image string) (codename string, official bool) {
	repo := strings.TrimSpace(image)

	// Drop a digest ("...@sha256:...").
	if i := strings.Index(repo, "@"); i >= 0 {
		repo = repo[:i]
	}
	// Drop a tag: a ':' that appears after the last '/' (so a registry host:port
	// is not mistaken for a tag separator).
	if colon := strings.LastIndex(repo, ":"); colon > strings.LastIndex(repo, "/") {
		repo = repo[:colon]
	}
	repo = strings.ToLower(repo)

	// Look at the namespace/name components so registry-qualified references
	// (e.g. "docker.io/penthertz/rfswift_noble") are recognised too.
	parts := strings.Split(repo, "/")
	if len(parts) < 2 || parts[len(parts)-2] != "penthertz" {
		return "", false
	}
	name := parts[len(parts)-1]
	// Longest prefix first so "rfswiftdev_" wins over "rfswift_".
	for _, prefix := range []string{"rfswiftdev_", "rfswift_"} {
		if strings.HasPrefix(name, prefix) {
			if cn := strings.TrimPrefix(name, prefix); cn != "" {
				return cn, true
			}
		}
	}
	return "", false
}

// IsOutdatedOfficialImage reports whether image is an official Penthertz
// RF-Swift image built on an Ubuntu base older than the current one
// (common.CurrentImageCodename). Custom or third-party images return false.
func IsOutdatedOfficialImage(image string) bool {
	cn, official := officialImageCodename(image)
	return official && !strings.EqualFold(cn, common.CurrentImageCodename)
}

var outdatedImageNotifyOnce sync.Once

// NotifyIfOutdatedImage prints a one-time warning when image is an official
// RF-Swift image built on an Ubuntu base older than the current one. The notice
// fires at most once per run regardless of how many times it is called (the
// config default plus an explicit -i image both route here).
func NotifyIfOutdatedImage(image string) {
	if !IsOutdatedOfficialImage(image) {
		return
	}
	outdatedImageNotifyOnce.Do(func() {
		DisplayNotification(
			"Outdated image",
			fmt.Sprintf("'%s' is an official RF-Swift image, but not the current one.\n"+
				"The current base is '%s' (penthertz/rfswift_%s). Consider switching to it.",
				image, common.CurrentImageCodename, common.CurrentImageCodename),
			"warning",
		)
	})
}
