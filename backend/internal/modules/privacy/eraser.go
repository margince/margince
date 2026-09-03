// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// What an eraser IS, and what it can reach.
//
// Its own file because erasure.go had outgrown the size cap, and because these
// are one concept: an Eraser is the set of seams a destruction can travel
// along, and each option here is one more place the data actually lives. What
// erasing a person DOES is next door.

import (
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/database"
)

// Eraser executes the shared erase path both the DSR surface and the
// retention engine's 'erase' action ride.
type Eraser struct {
	// db binds the installation's workspace itself (ADR-0091 §9 step 3).
	db *database.DB
	// blob purges the subject's attachment objects (Art. 17 reaches the
	// bytes, not only the row). nil in a deployment with no object store —
	// where no upload path could have stored an object either.
	blob blobstore.Store
	// payloads destroys the one-time link material behind a controller
	// delivery. Like blob it is nil where no such material could exist, and
	// like blob the purge happens BEFORE the rows that reference it: the
	// reference lives in the row, so clearing the row first would orphan a live
	// confirmation link with no key left to find it.
	payloads PayloadPurger
	// purgeRawCaptures destroys the provider original behind an activity whose
	// text this eraser destroys.
	//
	// Unlike blob there is no "there is nothing to do" case, so nil is not a
	// deployment shape: an eraser without it can only destroy the PARSED copy
	// while the verbatim original stands, joined on the (source_system,
	// source_id) pair the erasure deliberately keeps, and an Art. 15 export
	// serves it back. purgeContentDerivedFrom refuses rather than doing half.
	purgeRawCaptures RawCapturePurger
}

// NewEraser binds the erasure pass to the installation's pool.
func NewEraser(db *database.DB) *Eraser { return &Eraser{db: db} }

// WithRawCapturePurger returns an eraser that also destroys the provider
// originals behind the activities it erases.
//
// Every path that erases an activity's CONTENT needs it — the retention sweep's
// erase action, the expiry of a statutory floor, and a controller's release —
// and each is reached without an Art. 17 request, so none can lean on the
// person-scoped purge the cascade does. compose supplies capture's, which owns
// the natural-key join those rows are found by.
func (e *Eraser) WithRawCapturePurger(purge RawCapturePurger) *Eraser {
	clone := *e
	clone.purgeRawCaptures = purge
	return &clone
}

// WithPayloadVault returns an eraser that also destroys the one-time link
// material behind the subject's controller deliveries.
func (e *Eraser) WithPayloadVault(p PayloadPurger) *Eraser {
	clone := *e
	clone.payloads = p
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
