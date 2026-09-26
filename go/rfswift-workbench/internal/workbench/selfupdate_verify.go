/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
 */

package workbench

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/sigstore/sigstore-go/pkg/bundle"
	"github.com/sigstore/sigstore-go/pkg/root"
	"github.com/sigstore/sigstore-go/pkg/tuf"
	"github.com/sigstore/sigstore-go/pkg/verify"
)

// Update provenance. Every release asset is attested by the GitHub Actions
// workflow that built it (actions/attest-build-provenance in release.yml,
// macos-dmg.yml and windows-installer.yml), through the public Sigstore
// instance. An update is installed only when the SHA-256 of the downloaded
// file has such an attestation, signed for PentHertz/RF-Swift by the workflow
// expected for that asset, on the tag being installed. This is what
// `gh attestation verify --repo PentHertz/RF-Swift` checks, without needing
// gh or a GitHub token: the attestation API is public for public repositories.

const (
	updateOwner      = "PentHertz"
	updateRepo       = "RF-Swift"
	githubOIDCIssuer = "https://token.actions.githubusercontent.com"
	slsaProvenanceV1 = "https://slsa.dev/provenance/v1"
	githubAPI        = "https://api.github.com"
)

// provenanceWorkflow names the workflow whose attestation an asset must carry.
func provenanceWorkflow(asset string) string {
	switch {
	case strings.HasSuffix(asset, ".dmg"):
		return "macos-dmg.yml"
	case strings.HasSuffix(asset, ".msi"), strings.HasSuffix(asset, ".exe"):
		return "windows-installer.yml"
	default:
		return "release.yml"
	}
}

// provenanceSAN is the certificate identity the attestation must be signed
// with: the workflow file at the release tag.
func provenanceSAN(asset, tag string) string {
	return fmt.Sprintf("https://github.com/%s/%s/.github/workflows/%s@refs/tags/%s", updateOwner, updateRepo, provenanceWorkflow(asset), tag)
}

// sigstoreTrustedRoot fetches the public Sigstore trust root through TUF,
// starting from the root embedded in sigstore-go, with its cache kept under
// the user cache directory.
func sigstoreTrustedRoot() (root.TrustedMaterial, error) {
	opts := tuf.DefaultOptions()
	if dir, err := os.UserCacheDir(); err == nil {
		opts = opts.WithCachePath(filepath.Join(dir, "rfswift-workbench", "sigstore-tuf"))
	}
	tr, err := root.FetchTrustedRootWithOptions(opts)
	if err != nil {
		return nil, fmt.Errorf("could not load the Sigstore trust root: %w", err)
	}
	return tr, nil
}

// fetchAttestations returns the Sigstore bundles GitHub stores for a digest.
func fetchAttestations(ctx context.Context, hc *http.Client, digest []byte) ([][]byte, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/attestations/sha256:%s", githubAPI, updateOwner, updateRepo, hex.EncodeToString(digest))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("could not fetch the release attestations: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, errors.New("no attestation exists for this file: it was not built by the RF Swift release workflow")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("could not fetch the release attestations: %s", resp.Status)
	}
	var body struct {
		Attestations []struct {
			Bundle json.RawMessage `json:"bundle"`
		} `json:"attestations"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(&body); err != nil {
		return nil, fmt.Errorf("unreadable attestation response: %w", err)
	}
	var out [][]byte
	for _, a := range body.Attestations {
		if len(a.Bundle) > 0 {
			out = append(out, a.Bundle)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("no attestation exists for this file: it was not built by the RF Swift release workflow")
	}
	return out, nil
}

// verifyProvenance succeeds when one of the bundles is a valid Sigstore
// attestation of digest, signed by the expected workflow at tag, with SLSA
// provenance as its predicate.
func verifyProvenance(tm root.TrustedMaterial, bundles [][]byte, digest []byte, asset, tag string) error {
	v, err := verify.NewVerifier(tm,
		verify.WithSignedCertificateTimestamps(1),
		verify.WithTransparencyLog(1),
		verify.WithObserverTimestamps(1))
	if err != nil {
		return err
	}
	id, err := verify.NewShortCertificateIdentity(githubOIDCIssuer, "", provenanceSAN(asset, tag), "")
	if err != nil {
		return err
	}
	policy := verify.NewPolicy(verify.WithArtifactDigest("sha256", digest), verify.WithCertificateIdentity(id))
	var lastErr error
	for _, raw := range bundles {
		var b bundle.Bundle
		if err := b.UnmarshalJSON(raw); err != nil {
			lastErr = err
			continue
		}
		res, err := v.Verify(&b, policy)
		if err != nil {
			lastErr = err
			continue
		}
		if res.Statement == nil || res.Statement.PredicateType != slsaProvenanceV1 {
			lastErr = errors.New("the attestation is not SLSA build provenance")
			continue
		}
		return nil
	}
	return fmt.Errorf("the download is not attested by %s for %s: %v", provenanceWorkflow(asset), tag, lastErr)
}
