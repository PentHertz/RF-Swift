package dock

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func publishedNames(list []PublishedImage) string {
	names := make([]string, 0, len(list))
	for _, image := range list {
		names = append(names, image.Name)
	}
	return strings.Join(names, " ")
}

func TestPublishedImagesFromTagsKeepsRollingArchitectureTags(t *testing.T) {
	day := func(d int) time.Time { return time.Date(2026, 10, d, 0, 0, 0, 0, time.UTC) }
	tags := []Tag{
		{Name: "sdr_full_amd64", TagLastPushed: day(23), FullSize: 20},
		{Name: "sdr_full_1.3.0_amd64", TagLastPushed: day(20)}, // release
		{Name: "sdr_full", TagLastPushed: day(23)},             // multi-architecture index
		{Name: "telecom_5G_oai_amd64", TagLastPushed: day(22)},
		{Name: "deeptempest_amd64", TagLastPushed: day(21)},
		{Name: "corebuild_amd64", TagLastPushed: day(19)},
		{Name: "corebuild_amd64", TagLastPushed: day(18)}, // older duplicate
	}
	got := publishedImagesFromTags("penthertz/rfswift_resolute", tags)
	if names := publishedNames(got); names != "corebuild deeptempest sdr_full telecom_5G_oai" {
		t.Fatalf("names = %q", names)
	}
	if got[2].Ref != "penthertz/rfswift_resolute:sdr_full" || got[2].Size != 20 || !got[2].Date.Equal(day(23)) {
		t.Fatalf("sdr_full = %+v", got[2])
	}
	if !got[0].Date.Equal(day(19)) {
		t.Fatalf("the newest push of a duplicated tag must win: %+v", got[0])
	}
}

// usePublishedImagesFixture points the cache at a temporary file and the
// Docker Hub listing at fetch, and counts the calls.
func usePublishedImagesFixture(t *testing.T, fetch func(repo, arch string) ([]Tag, error)) (*int, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "rfswift", "published-images.json")
	calls := 0
	oldFetch, oldPath := publishedImagesFetch, publishedImagesCachePath
	publishedImagesFetch = func(repo, arch string) ([]Tag, error) { calls++; return fetch(repo, arch) }
	publishedImagesCachePath = func() string { return path }
	t.Cleanup(func() { publishedImagesFetch, publishedImagesCachePath = oldFetch, oldPath })
	return &calls, path
}

func TestPublishedImagesCachesUntilRefresh(t *testing.T) {
	images := []string{"sdr_light_amd64"}
	calls, path := usePublishedImagesFixture(t, func(repo, arch string) ([]Tag, error) {
		var tags []Tag
		for _, name := range images {
			tags = append(tags, Tag{Name: strings.Replace(name, "_amd64", "_"+arch, 1), TagLastPushed: time.Now()})
		}
		return tags, nil
	})

	first, err := PublishedImages(false)
	if err != nil || publishedNames(first.Images) != "sdr_light" || first.Stale {
		t.Fatalf("first = %+v, %v", first, err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("cache not written: %v", err)
	}

	images = []string{"sdr_light_amd64", "sdr_hacks_amd64"}
	second, err := PublishedImages(false)
	if err != nil || *calls != 1 || publishedNames(second.Images) != "sdr_light" {
		t.Fatalf("a fresh cache must be served without asking Docker Hub: calls=%d %+v %v", *calls, second, err)
	}

	refreshed, err := PublishedImages(true)
	if err != nil || *calls != 2 || publishedNames(refreshed.Images) != "sdr_hacks sdr_light" {
		t.Fatalf("refresh = calls=%d %+v %v", *calls, refreshed, err)
	}
}

func TestPublishedImagesExpiredCacheIsFetchedAgain(t *testing.T) {
	calls, path := usePublishedImagesFixture(t, func(repo, arch string) ([]Tag, error) {
		return []Tag{{Name: "rfid_" + arch, TagLastPushed: time.Now()}}, nil
	})
	if _, err := PublishedImages(false); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-publishedImagesTTL - time.Minute)
	cache, ok := readPublishedImagesCache(path, strings.Join(OfficialRepos(), ",")+"|"+getArchitecture())
	if !ok {
		t.Fatal("cache unreadable")
	}
	cache.Fetched = old
	writePublishedImagesCache(path, publishedImagesCache{Key: strings.Join(OfficialRepos(), ",") + "|" + getArchitecture(), List: cache})
	if _, err := PublishedImages(false); err != nil || *calls != 2 {
		t.Fatalf("expired cache must be refetched: calls=%d %v", *calls, err)
	}
}

func TestPublishedImagesOfflineServesStaleCache(t *testing.T) {
	offline := false
	usePublishedImagesFixture(t, func(repo, arch string) ([]Tag, error) {
		if offline {
			return nil, errors.New("no network")
		}
		return []Tag{{Name: "wifi_" + arch, TagLastPushed: time.Now()}}, nil
	})
	if _, err := PublishedImages(false); err != nil {
		t.Fatal(err)
	}
	offline = true
	list, err := PublishedImages(true)
	if err != nil || !list.Stale || publishedNames(list.Images) != "wifi" {
		t.Fatalf("offline refresh must return the cached list marked stale: %+v %v", list, err)
	}
}

func TestPublishedImagesOfflineWithoutCacheFails(t *testing.T) {
	usePublishedImagesFixture(t, func(repo, arch string) ([]Tag, error) { return nil, errors.New("no network") })
	if _, err := PublishedImages(false); err == nil || !strings.Contains(err.Error(), "no network") {
		t.Fatalf("expected the Docker Hub error, got %v", err)
	}
}

func TestPublishedImagesCacheIgnoresOtherRepositoryAndBadEntries(t *testing.T) {
	calls, path := usePublishedImagesFixture(t, func(repo, arch string) ([]Tag, error) {
		return []Tag{{Name: "android_" + arch, TagLastPushed: time.Now()}}, nil
	})
	key := strings.Join(OfficialRepos(), ",") + "|" + getArchitecture()
	writePublishedImagesCache(path, publishedImagesCache{Key: "example/not-rfswift|" + getArchitecture(),
		List: PublishedImageList{Fetched: time.Now(), Images: []PublishedImage{{Name: "sdr_full"}}}})
	if list, err := PublishedImages(false); err != nil || *calls != 1 || publishedNames(list.Images) != "android" {
		t.Fatalf("a cache for another repository must not be served: calls=%d %+v %v", *calls, list, err)
	}

	writePublishedImagesCache(path, publishedImagesCache{Key: key, List: PublishedImageList{Fetched: time.Now(),
		Images: []PublishedImage{{Name: `x"><img src=x>`}, {Name: "rfid"}}}})
	list, ok := readPublishedImagesCache(path, key)
	if !ok || publishedNames(list.Images) != "rfid" {
		t.Fatalf("invalid tag names must be dropped from the cache: %+v", list)
	}
}
