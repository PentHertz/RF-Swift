/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
*
*  Official images published on Docker Hub for this host's architecture, the
*  list `rfswift images remote` shows, for image pickers: the Workbench's
*  create dialog (locally, or on the agent host through the
*  "images.published" control method). Listing a repository walks every tag
*  page on Docker Hub, so the result is cached on disk for a few hours.
 */

package dock

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	common "penthertz/rfswift/common"
)

// publishedImagesTTL is how long a fetched list is served from the cache.
// Images are rebuilt about once a day; a picker that misses a brand-new image
// can be refreshed, and the name can always be typed.
const publishedImagesTTL = 6 * time.Hour

// publishedImagesFetch lists a repository's tags; tests replace it.
var publishedImagesFetch = getLatestDockerHubTags

// publishedImagesMu keeps two pickers opened together from both walking
// Docker Hub.
var publishedImagesMu sync.Mutex

// validImageTag is Docker's tag grammar. Cached entries are checked against
// it before they reach a picker.
var validImageTag = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$`)

// PublishedImage is one official image, by its rolling tag.
type PublishedImage struct {
	Name string    `json:"name"` // toolset tag, e.g. sdr_full
	Ref  string    `json:"ref"`  // repo:name
	Date time.Time `json:"date"` // last push for this architecture
	Size int64     `json:"size"` // compressed size reported by Docker Hub
}

// PublishedImageList is what an image picker shows.
type PublishedImageList struct {
	Architecture string           `json:"architecture"`
	Fetched      time.Time        `json:"fetched"`
	Stale        bool             `json:"stale,omitempty"` // Docker Hub unreachable: the last list fetched
	Images       []PublishedImage `json:"images"`
}

// publishedImagesCache is the file kept in the cache directory. Key ties it
// to the repositories and architecture it was fetched for, so changing
// repotag does not serve the previous repository's images.
type publishedImagesCache struct {
	Key  string             `json:"key"`
	List PublishedImageList `json:"list"`
}

// PublishedImages returns the official images published for this host's
// architecture, sorted by name. A list fetched less than publishedImagesTTL
// ago comes from the cache unless refresh is set. When Docker Hub cannot be
// reached, the last cached list is returned with Stale set.
//
//	in(1): bool refresh fetch from Docker Hub even if the cache is fresh
//	out: PublishedImageList images for this architecture
//	out: error non-nil when nothing could be fetched and nothing is cached
func PublishedImages(refresh bool) (PublishedImageList, error) {
	arch := getArchitecture()
	if arch == "" {
		return PublishedImageList{}, fmt.Errorf("unsupported architecture %s", runtime.GOARCH)
	}
	repos := OfficialRepos()
	key := strings.Join(repos, ",") + "|" + arch
	path := publishedImagesCachePath()

	publishedImagesMu.Lock()
	defer publishedImagesMu.Unlock()

	cached, haveCache := readPublishedImagesCache(path, key)
	if haveCache && !refresh && time.Since(cached.Fetched) < publishedImagesTTL {
		return cached, nil
	}

	var images []PublishedImage
	var errs []string
	for _, repo := range repos {
		tags, err := publishedImagesFetch(repo, arch)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", repo, err))
			continue
		}
		images = append(images, publishedImagesFromTags(repo, tags)...)
	}
	if len(errs) == len(repos) {
		if haveCache {
			cached.Stale = true
			return cached, nil
		}
		return PublishedImageList{}, fmt.Errorf("cannot list the published images: %s", strings.Join(errs, "; "))
	}
	sort.SliceStable(images, func(i, j int) bool { return images[i].Name < images[j].Name })
	list := PublishedImageList{Architecture: arch, Fetched: time.Now().UTC(), Images: images}
	writePublishedImagesCache(path, publishedImagesCache{Key: key, List: list})
	return list, nil
}

// publishedImagesFromTags keeps what `rfswift images remote` lists: the
// rolling per-architecture tag of each toolset (sdr_full_amd64), not the
// multi-architecture index or the versioned releases.
func publishedImagesFromTags(repo string, tags []Tag) []PublishedImage {
	byName := map[string]PublishedImage{}
	for _, tag := range tags {
		if removeArchitectureSuffix(tag.Name) == tag.Name {
			continue
		}
		name, version := parseTagVersion(tag.Name)
		if version != "" || !validImageTag.MatchString(name) {
			continue
		}
		if seen, ok := byName[name]; ok && !tag.TagLastPushed.After(seen.Date) {
			continue
		}
		byName[name] = PublishedImage{Name: name, Ref: repo + ":" + name, Date: tag.TagLastPushed, Size: tag.FullSize}
	}
	out := make([]PublishedImage, 0, len(byName))
	for _, image := range byName {
		out = append(out, image)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// publishedImagesCachePath is the cache file, or "" when the platform has no
// cache directory (then every call fetches). Tests replace it.
var publishedImagesCachePath = func() string {
	dir := common.CacheDirByPlatform()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "published-images.json")
}

// readPublishedImagesCache loads the cached list when it was fetched for key.
// Entries that are not plain image tags are dropped.
func readPublishedImagesCache(path, key string) (PublishedImageList, bool) {
	if path == "" {
		return PublishedImageList{}, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return PublishedImageList{}, false
	}
	var cache publishedImagesCache
	if err := json.Unmarshal(data, &cache); err != nil || cache.Key != key || cache.List.Fetched.IsZero() {
		return PublishedImageList{}, false
	}
	images := cache.List.Images[:0]
	for _, image := range cache.List.Images {
		if validImageTag.MatchString(image.Name) {
			images = append(images, image)
		}
	}
	cache.List.Images = images
	return cache.List, len(images) > 0
}

// writePublishedImagesCache stores the list through a temporary file renamed
// over the old one, so a reader never sees half a file. Best effort: without
// a cache the next call fetches again. Under sudo the directory and file go
// back to the user, like the configuration.
func writePublishedImagesCache(path string, cache publishedImagesCache) {
	if path == "" {
		return
	}
	data, err := json.Marshal(cache)
	if err != nil {
		return
	}
	dir := filepath.Dir(path)
	if err := common.MkdirAllForInvokingUser(dir, 0o700); err != nil {
		return
	}
	tmp, err := os.CreateTemp(dir, "published-images-*.json")
	if err != nil {
		return
	}
	_, werr := tmp.Write(data)
	cerr := tmp.Close()
	if werr != nil || cerr != nil {
		os.Remove(tmp.Name())
		return
	}
	common.ChownToInvokingUser(tmp.Name())
	if err := os.Rename(tmp.Name(), path); err != nil {
		os.Remove(tmp.Name())
	}
}
