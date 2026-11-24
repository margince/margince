// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// What an eraser IS, and what it can reach.
//
// Its own file because erasure.go had outgrown the size cap, and because these
// are one concept: an Eraser is the set of seams a destruction can travel
// along, and each option here is one more place the data actually lives. What
// erasing a contact DOES is next door.

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/extprivacy"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Eraser executes the shared erase path both the DSR surface and the
// retention engine's 'erase' action ride.
type Eraser struct {
	invalidateEdges EdgeInvalidator
	// db binds the installation's workspace itself (ADR-0091 §9 step 3).
	db *database.DB
	// blob is the object store the erased attachments' bytes live in. The
	// erasure queues their keys for the stored-object reap and deletes no
	// object itself. nil in a deployment with no object store.
	blob blobstore.Store
	// journal is a separate store for the suppression journal, so a restore of
	// blob never rolls it back. nil keeps the journal in blob.
	journal blobstore.Store
	// purgeRawCaptures destroys the provider original behind an activity whose
	// text this eraser destroys.
	//
	// Unlike blob there is no "there is nothing to do" case, so nil is not a
	// deployment shape: an eraser without it can only destroy the PARSED copy
	// while the verbatim original stands, joined on the (source_system,
	// source_id) pair the erasure deliberately keeps, and an Art. 15 export
	// serves it back. purgeContentDerivedFrom refuses rather than doing half.
	purgeRawCaptures RawCapturePurger
	// country selects the jurisdiction whose correspondence floor binds.
	country CountryReader
	// extensions holds the personal-data declarations of the extension tables
	// this eraser clears. The zero value declares none.
	extensions extprivacy.Registry
}

// WithExtensions returns an eraser that also clears the extension tables the
// registry declares. Every eraser built outside a test carries it;
// TestEveryProductionEraserCarriesTheExtensionRegistry holds that.
func (e *Eraser) WithExtensions(r extprivacy.Registry) *Eraser {
	clone := *e
	clone.extensions = r
	return &clone
}

// NewEraser binds the erasure pass to the installation's pool.
func NewEraser(db *database.DB) *Eraser { return &Eraser{db: db} }

// WithRawCapturePurger returns an eraser that also destroys the provider
// originals behind the activities it erases.
//
// Every path that erases an activity's CONTENT needs it — the retention sweep's
// erase action, the expiry of a statutory floor, and a controller's release —
// and each is reached without an Art. 17 request, so none can lean on the
// contact-scoped purge the cascade does. compose supplies capture's, which owns
// the natural-key join those rows are found by.
func (e *Eraser) WithRawCapturePurger(purge RawCapturePurger) *Eraser {
	clone := *e
	if rawCapturePurgerMissing(purge) {
		purge = nil
	}
	clone.purgeRawCaptures = purge
	return &clone
}

// WithBlobstore returns an eraser that also purges attachment objects.
// Compose passes the object store so erasure reaches the bytes behind the
// attachment rows it deletes.
func (e *Eraser) WithBlobstore(blob blobstore.Store) *Eraser {
	clone := *e
	clone.blob = blob
	return &clone
}

// WithJournalStore returns an eraser that keeps the suppression journal in
// journal instead of the attachment store. A nil journal changes nothing.
func (e *Eraser) WithJournalStore(journal blobstore.Store) *Eraser {
	clone := *e
	clone.journal = journal
	return &clone
}

// journalStore is where the export writes and the replay reads the journal.
// Every journal access goes through it, so the two never disagree.
func (e *Eraser) journalStore() blobstore.Store {
	if e.journal != nil {
		return e.journal
	}
	return e.blob
}

func requireErasableContact(ctx context.Context, tx pgx.Tx, subject ids.ContactID) error {
	if err := auth.EnsureWritableForSubjectRights(ctx, tx, "contact", subject.UUID); err != nil {
		return err
	}
	return refuseContactUnderLegalHold(ctx, tx, subject)
}
