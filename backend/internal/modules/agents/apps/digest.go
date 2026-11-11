// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package apps

// Each view document is pinned to the bytes this build produced.
//
// The web build writes dist/mcp-apps/digests.json, a SHA-256 per document, and
// the api's build links the same file into ViewDigests. A fetched document is
// served only when its digest matches. A document changed in transit or at the
// origin is refused, and so is any view the manifest does not name.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path"

	"github.com/margince/margince/backend/internal/shared/buildinfo"
)

// ViewDigests is the web build's digests.json, set at link time:
//
//	go build -ldflags "-X '<this package>.ViewDigests=<digests.json>'"
//
// It maps each document's file name to the hex SHA-256 of its bytes. A binary
// linked without it serves unpinned views only when it carries no release.
var ViewDigests string

// errNoDigest refuses a view the build pinned no digest for.
var errNoDigest = errors.New("crmapps: this build carries no digest for the view")

// viewPins is the parsed manifest. unpinned marks a development build with no
// manifest. refusal marks a release without one, or one that does not parse;
// then every view is refused.
type viewPins struct {
	byFile   map[string]string
	unpinned bool
	refusal  error
}

// pinsFrom parses a manifest for a binary of the given release.
func pinsFrom(manifest, release string) viewPins {
	if manifest == "" {
		if !buildinfo.Comparable(release) {
			return viewPins{unpinned: true}
		}
		return viewPins{refusal: fmt.Errorf("%w: release %s was linked without a view digest manifest",
			ErrPermanent, release)}
	}
	var byFile map[string]string
	if err := json.Unmarshal([]byte(manifest), &byFile); err != nil {
		return viewPins{refusal: fmt.Errorf("%w: the view digest manifest does not parse: %w", ErrPermanent, err)}
	}
	return viewPins{byFile: byFile}
}

// digestMismatch is a document whose bytes are not the ones this build made.
type digestMismatch struct {
	uri, want, got string
}

func (e *digestMismatch) Error() string {
	return fmt.Sprintf("crmapps: %s does not match this build (want sha256 %s, got %s)", e.uri, e.want, e.got)
}

func (e *digestMismatch) Unwrap() error { return ErrPermanent }

// check answers nil for a document this build produced.
func (p viewPins) check(uri, doc string) error {
	if p.refusal != nil {
		return p.refusal
	}
	if p.unpinned {
		return nil
	}
	want, pinned := p.byFile[path.Base(uri)]
	if !pinned {
		return fmt.Errorf("%w: %w: %s", ErrPermanent, errNoDigest, uri)
	}
	sum := sha256.Sum256([]byte(doc))
	if got := hex.EncodeToString(sum[:]); got != want {
		return &digestMismatch{uri: uri, want: want, got: got}
	}
	return nil
}
