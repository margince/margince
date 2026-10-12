// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// What the journal knows, kept beside it.
//
// The journal ends at the newest erasure any export carried. A replay after a
// restore covers nothing erased later, so the operator has to be told when
// that was. A worker booted against the restored database exports the older
// list too, so the manifest only ever moves forward.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// suppressionJournalManifestKey holds what the journal knows. No kind is
// named "manifest", so it shares no key with an entry.
const suppressionJournalManifestKey = suppressionJournalPrefix + "manifest"

// JournalManifest says what the journal knows. An erasure after NewestErasure
// is in no journal, so a replay cannot cover it.
type JournalManifest struct {
	NewestErasure time.Time
	Entries       int
}

func (m JournalManifest) encode() string {
	return fmt.Sprintf("newest_erasure %s\nentries %d\n", m.NewestErasure.Format(time.RFC3339Nano), m.Entries)
}

// ErrNoJournalManifest reports a store that holds no complete export.
var ErrNoJournalManifest = errors.New("privacy: the object store holds no complete suppression journal export")

// ReadJournalManifest reports what the journal knows, or ErrNoJournalManifest
// when no export ever finished.
func (e *Eraser) ReadJournalManifest(ctx context.Context) (JournalManifest, error) {
	if err := auth.Require(ctx, "contact", principal.ActionDelete); err != nil {
		return JournalManifest{}, err
	}
	if e.blob == nil {
		return JournalManifest{}, ErrNoJournalManifest
	}
	return e.loadJournalManifest(ctx)
}

// advanceJournalManifest records an export, never moving the manifest back.
//
// Entries are only ever added to the journal, so neither its newest erasure
// nor its size can shrink. An export from a restored, shorter list would
// otherwise tell the replay it covers erasures the list never held.
func (e *Eraser) advanceJournalManifest(ctx context.Context, export JournalManifest) error {
	recorded, err := e.loadJournalManifest(ctx)
	switch {
	case errors.Is(err, ErrNoJournalManifest):
		recorded = export
	case err != nil:
		return err
	}
	if export.NewestErasure.After(recorded.NewestErasure) {
		recorded.NewestErasure = export.NewestErasure
	}
	recorded.Entries = max(recorded.Entries, export.Entries)
	body := []byte(recorded.encode())
	if err := e.blob.Put(ctx, suppressionJournalManifestKey,
		bytes.NewReader(body), int64(len(body)), "text/plain"); err != nil {
		return fmt.Errorf("privacy: writing the suppression journal manifest: %w", err)
	}
	return nil
}

func (e *Eraser) loadJournalManifest(ctx context.Context) (JournalManifest, error) {
	reader, _, err := e.blob.Get(ctx, suppressionJournalManifestKey)
	if errors.Is(err, blobstore.ErrNotFound) {
		return JournalManifest{}, ErrNoJournalManifest
	}
	if err != nil {
		return JournalManifest{}, fmt.Errorf("privacy: reading the suppression journal manifest: %w", err)
	}
	body, readErr := io.ReadAll(io.LimitReader(reader, 1024))
	if err := reader.Close(); err != nil && readErr == nil {
		readErr = err
	}
	if readErr != nil {
		return JournalManifest{}, fmt.Errorf("privacy: reading the suppression journal manifest: %w", readErr)
	}
	var stamp string
	var manifest JournalManifest
	if _, err := fmt.Sscanf(string(body), "newest_erasure %s\nentries %d\n", &stamp, &manifest.Entries); err != nil {
		return JournalManifest{}, fmt.Errorf("privacy: the suppression journal manifest is malformed: %w", err)
	}
	if manifest.NewestErasure, err = time.Parse(time.RFC3339Nano, stamp); err != nil {
		return JournalManifest{}, fmt.Errorf("privacy: the suppression journal manifest is malformed: %w", err)
	}
	return manifest, nil
}
