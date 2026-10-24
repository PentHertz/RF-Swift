package nix

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGithubRepositoryFromFlake(t *testing.T) {
	for _, ref := range []string{"github:PentHertz/RF-Swift-nix", "github:PentHertz/RF-Swift-nix/v1.2.3", "https://github.com/PentHertz/RF-Swift-nix.git", "git@github.com:PentHertz/RF-Swift-nix.git"} {
		o, r, err := githubRepositoryFromFlake(ref)
		if err != nil || o != "PentHertz" || r != "RF-Swift-nix" {
			t.Fatalf("%q => %q/%q, %v", ref, o, r, err)
		}
	}
}

func TestListRepositoryVersions(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/PentHertz/RF-Swift-nix", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"default_branch":"main"}`)) })
	mux.HandleFunc("/repos/PentHertz/RF-Swift-nix/commits/main", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"sha":"abcdef123456","commit":{"committer":{"date":"2026-08-29T10:00:00Z"}}}`))
	})
	mux.HandleFunc("/repos/PentHertz/RF-Swift-nix/tags", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"name":"v1.1.0","commit":{"sha":"older"}},{"name":"v1.2.0","commit":{"sha":"123456abcdef"}}]`))
	})
	mux.HandleFunc("/repos/PentHertz/RF-Swift-nix/releases/latest", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"tag_name":"v1.2.0"}`)) })
	server := httptest.NewServer(mux)
	defer server.Close()

	got, err := listRepositoryVersions(context.Background(), "github:PentHertz/RF-Swift-nix", server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if got.DefaultBranch != "main" || got.Nightly.Ref != "github:PentHertz/RF-Swift-nix?rev=abcdef123456" {
		t.Fatalf("unexpected nightly: %#v", got)
	}
	if got.Latest == nil || got.Latest.Ref != "github:PentHertz/RF-Swift-nix/v1.2.0" || len(got.Releases) != 1 || got.Releases[0].Name != "v1.1.0" {
		t.Fatalf("unexpected latest release: %#v", got)
	}
}

func TestListRepositoryVersionsWithoutTagsUsesNightly(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/PentHertz/RF-Swift-nix", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"default_branch":"main"}`)) })
	mux.HandleFunc("/repos/PentHertz/RF-Swift-nix/commits/main", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"sha":"nightlysha","commit":{"committer":{"date":"2026-08-29T10:00:00Z"}}}`))
	})
	mux.HandleFunc("/repos/PentHertz/RF-Swift-nix/tags", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`[]`)) })
	server := httptest.NewServer(mux)
	defer server.Close()

	got, err := listRepositoryVersions(context.Background(), "github:PentHertz/RF-Swift-nix", server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if got.Latest != nil || got.Nightly.Name != "Nightly" || got.Nightly.Commit != "nightlysha" {
		t.Fatalf("unexpected versions: %#v", got)
	}
}

func TestListRepositoryVersionsWithoutPublishedReleaseUsesNewestTag(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/PentHertz/RF-Swift-nix", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"default_branch":"main"}`)) })
	mux.HandleFunc("/repos/PentHertz/RF-Swift-nix/commits/main", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"sha":"nightlysha","commit":{"committer":{"date":"2026-10-24T10:00:00Z"}}}`))
	})
	// Tags only, as RF-Swift-nix publishes them: /releases/latest answers 404.
	mux.HandleFunc("/repos/PentHertz/RF-Swift-nix/tags", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"name":"v1.0.0-dev","commit":{"sha":"old"}},{"name":"snapshot","commit":{"sha":"snap"}},{"name":"v1.1.0-dev","commit":{"sha":"3fa632f"}}]`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	got, err := listRepositoryVersions(context.Background(), "github:PentHertz/RF-Swift-nix", server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if got.Latest == nil || got.Latest.Name != "v1.1.0-dev" || got.Latest.Kind != "latest" || got.Latest.Ref != "github:PentHertz/RF-Swift-nix/v1.1.0-dev" {
		t.Fatalf("newest tag not used as latest: %#v", got.Latest)
	}
	if len(got.Releases) != 2 {
		t.Fatalf("latest should leave the release list: %#v", got.Releases)
	}
}

func TestNewestVersionTag(t *testing.T) {
	tags := func(names ...string) []RepositoryVersion {
		var out []RepositoryVersion
		for _, n := range names {
			out = append(out, RepositoryVersion{Name: n})
		}
		return out
	}
	for _, c := range []struct {
		want  string
		names []string
	}{
		{"v1.1.0-dev", []string{"v1.0.0-dev", "v1.1.0-dev"}},
		{"v1.1.0", []string{"v1.1.0-dev", "v1.1.0", "v1.0.9"}},
		{"v1.10.0", []string{"v1.9.0", "v1.10.0"}},
		{"", []string{"snapshot", "main"}},
	} {
		if got := newestVersionTag(tags(c.names...)); got != c.want {
			t.Errorf("newestVersionTag(%v) = %q, want %q", c.names, got, c.want)
		}
	}
}
