// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package knowledge

// Where a document's bytes come from, decided in ONE place.
//
// Two kinds of document live in these tables and they are stored differently.
// A document somebody uploaded is in object storage. A page of the operator
// handbook is in the binary — it has to be, because object storage is optional
// and the handbook is the corpus every installation has.
//
// Three call sites need a document's bytes: the download a citation points at,
// the ingest that turns a document into passages, and the cleanup that destroys
// a failed one. Spelling the choice at each of them would be three copies of
// one rule, and the third copy is the one that forgets the handbook exists and
// hands a reader a 500 for the only corpus their installation has.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/margince/margince/backend/internal/modules/knowledge/handbook"
)

// handbookKeyPrefix marks a storage key as naming an embedded handbook page
// rather than an object. A colon cannot appear in the keys blobstore mints
// (blobstore.WorkspaceKey builds them from a workspace id and a uuid), so the
// two spaces cannot collide.
const handbookKeyPrefix = "handbook:"

// handbookStorageKey is the storage key for one embedded page. One spelling,
// because the seeding writes these keys and the readers below parse them, and
// a prefix typed in two places is one typo from documents nothing can open.
func handbookStorageKey(filename string) string { return handbookKeyPrefix + filename }

// ErrHandbookPageGone is a document row naming a page this build does not ship.
//
// It is reachable, not theoretical: a release that withdraws a page leaves its
// rows behind until the boot reconciliation removes them, and a request can
// arrive in between. Distinct from a missing object because the repair is
// different — nobody should go looking in a bucket for it.
var ErrHandbookPageGone = errors.New("knowledge: this release does not ship that handbook page")

// openContent streams a document's bytes, whichever side of the split it is on.
func (s *Store) openContent(ctx context.Context, storageKey string) (io.ReadCloser, error) {
	if page, ok := strings.CutPrefix(storageKey, handbookKeyPrefix); ok {
		content, present := handbook.Open(page)
		if !present {
			return nil, fmt.Errorf("%q: %w", page, ErrHandbookPageGone)
		}
		return io.NopCloser(bytes.NewReader(content)), nil
	}
	if s.blob == nil {
		return nil, ErrBlobstoreUnconfigured
	}
	body, _, err := s.blob.Get(ctx, storageKey)
	if err != nil {
		return nil, fmt.Errorf("read the stored document: %w", err)
	}
	return body, nil
}

// discardContent destroys the bytes behind a document, for the paths that
// destroy the document.
//
// A handbook page is a no-op AND SAYS WHY: there is nothing to destroy, because
// the bytes are a segment of the running binary rather than an object anybody
// wrote. Returning an error instead would fail the ordinary cleanup of a failed
// handbook ingest for something that is not a fault; silently sharing the
// blobstore branch would eventually call Delete on a key that is not one.
//
// The nil-blobstore arm is the same judgement the ingest cleanup already
// documents: a role with no object store never wrote bytes, so there are none
// to remove and that is not a failure.
func (s *Store) discardContent(ctx context.Context, storageKey string) error {
	if strings.HasPrefix(storageKey, handbookKeyPrefix) {
		return nil
	}
	if s.blob == nil {
		return nil
	}
	if err := s.blob.Delete(ctx, storageKey); err != nil {
		return fmt.Errorf("delete the document's stored file: %w", err)
	}
	return nil
}
