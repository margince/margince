// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// What one card does to the record it resolved to.
//
// Its own file for the reason the arms differ at all: one has an existing
// subject to hold and the other does not, and that difference decides the LOCK
// ORDER each has to keep. Reading them side by side is what makes the ordering
// checkable; buried inside the dedupe switch they read as two similar blocks.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// updateMatchedByCard applies a card to the contact it matched exactly.
//
// HOLD rather than probe, and the order of the two gates is load-bearing. This
// arm goes on to WRITE the person, and the source check below takes a lock of
// its own — the eraser is subject-first (privacy/erasure.go anonymizes the
// subject's rows before deleting what hangs off it), so this transaction must
// hold the subject before it holds anything else or the two deadlock and an
// Art. 17 fulfilment fails on a path an ordinary seat can drive.
func (s *Store) updateMatchedByCard(
	ctx context.Context, tx pgx.Tx, personID ids.PersonID,
	entry VCardEntry, source ids.UUID, result *VCardResult,
) error {
	// The card names somebody who exists, so the caller's authority over THAT
	// ROW decides whether this card may touch it — the person:create grant that
	// let them start the import says nothing about a record they cannot see.
	//
	// Reported as a skip rather than skipped silently: an import that quietly
	// leaves one card out is an import nobody can audit, and the reader can act
	// on "you may not write this one".
	if err := auth.HoldWritableLive(ctx, tx, entityPerson, personID.UUID); err != nil {
		if errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied) {
			// Deliberately says nothing about WHAT was matched. The two refusals
			// are one sentence because telling them apart would confirm that the
			// address on this card belongs to a real contact in this workspace,
			// to somebody who may not see that contact.
			result.Outcome = VCardSkipped
			result.Reason = "this card could not be written here"
			return nil
		}
		return fmt.Errorf("probing write authority over the matched person: %w", err)
	}
	narrowed, err := sourceIsNarrowed(ctx, tx, source)
	if err != nil {
		return err
	}
	if narrowed {
		result.Outcome = VCardSkipped
		result.Reason = vcardSourceNarrowed
		return nil
	}
	result.Outcome = VCardUpdated
	result.PersonID = &personID
	return s.fillFromVCard(ctx, tx, personID, entry)
}

// createFromCard turns a card nobody matched into a person.
//
// No existing subject to hold on this arm — the person does not exist yet — so
// the source check is what stands between a narrowed message and a new record,
// and it takes this transaction's first row lock.
func (s *Store) createFromCard(
	ctx context.Context, tx pgx.Tx, entry VCardEntry,
	source ids.UUID, result *VCardResult,
) error {
	narrowed, err := sourceIsNarrowed(ctx, tx, source)
	if err != nil {
		return err
	}
	if narrowed {
		result.Outcome = VCardSkipped
		result.Reason = vcardSourceNarrowed
		return nil
	}
	person, err := s.CreatePersonTx(ctx, tx, personFromVCard(entry))
	if err != nil {
		return err
	}
	created := ids.From[ids.PersonKind](ids.UUID(person.Id))
	result.Outcome = VCardCreated
	result.PersonID = &created
	return s.attachVCardEmployer(ctx, tx, created, entry)
}
