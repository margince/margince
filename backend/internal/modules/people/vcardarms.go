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
// The source is checked AFTER the person is minted, and that ordering is the
// deadlock rule rather than a preference. This arm has no prior subject to hold
// — the person does not exist yet — so it cannot open with a subject lock the
// way updateMatchedByCard does. What it can do is take its rows in the same
// DIRECTION the eraser does: privacy/erasure.go deletes person_email (its
// `email = ANY(...)` arm reaches an address held by an archived record) and only
// then locks the subject's activities. Taking the activity lock first would put
// this transaction holding the message and waiting on an email the eraser holds
// while that eraser waits on the message — and the loser of a 40P01 there can be
// the Art. 17 fulfilment, which fails on a path ordinary inbound mail drives.
//
// The guarantee is unchanged by the move: the check and the write are still one
// transaction, so a narrowing cannot commit between them. Only the order in
// which this transaction takes its two locks changes, and the rollback that
// follows a refusal undoes the person as if it had never been minted.
func (s *Store) createFromCard(
	ctx context.Context, tx pgx.Tx, entry VCardEntry,
	source ids.UUID, result *VCardResult,
) error {
	person, err := s.CreatePersonTx(ctx, tx, personFromVCard(entry))
	if err != nil {
		return err
	}
	narrowed, err := sourceIsNarrowed(ctx, tx, source)
	if err != nil {
		return err
	}
	if narrowed {
		// The person minted above goes with the transaction. Reported as a skip
		// so the caller sees a refusal rather than a create it cannot find.
		result.Outcome = VCardSkipped
		result.Reason = vcardSourceNarrowed
		return errCardSourceNarrowed
	}
	created := ids.From[ids.PersonKind](ids.UUID(person.Id))
	result.Outcome = VCardCreated
	result.PersonID = &created
	return s.attachVCardEmployer(ctx, tx, created, entry)
}

// errCardSourceNarrowed rolls back the person a refused card had already minted.
//
// It never leaves importOneVCard: the create arm has to write before it may take
// the source lock (see above), so the only way to undo that write is to fail the
// transaction. The caller recognises this one error and reports the card's own
// outcome, which the arm has already filled in.
var errCardSourceNarrowed = errors.New("people: the message this card arrived on is no longer readable here")
