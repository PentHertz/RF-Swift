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
