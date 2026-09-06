// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// WHO MAY SEE A CONTACT, and the three rules the column needs that no other
// field on the person patch does.
//
// visibility used to move one way only — POST /people/{id}/publish widened a
// contact and nothing narrowed one — on the reasoning that a colleague may
// already have acted on seeing it. That assumed a human made the disclosure.
// The sender classifier publishes a contact it judges a real counterparty with
// nobody approving it, so the common case was a machine making a decision no
// human could undo, the row's own owner included. It is an ordinary field now,
// writable both ways by anybody the write gate admits.
//
// Ordinary in who may write it; not ordinary in what a wrong write costs. A
// title written over a stale read is a wrong title. This column decides who
// may read the record, so it carries a staleness check every other field
// tolerates going without, refuses a result no human could read, and hands the
// correspondence across only for the owner.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// refuseStaleVisibility refuses a visibility write built on a row that has
// moved since it was read.
//
// UpdatePerson reads the person, decides admission, and only then locks the
// row — an unconditional patch takes its FOR UPDATE inside ApplyGuarded
// (storekit.LockRow). Every other field tolerates that window: last-write-wins
// over a title is a wrong title. This column is different. A colleague whose
// PATCH was admitted while the contact was public can land `workspace` after
// its owner has just made it private, RE-PUBLISHING a record somebody
// deliberately closed, and the audit records `workspace -> workspace` because
// the before-image came from the stale read.
//
// So this re-reads the column under the row lock and refuses when it no longer
// matches what the caller was shown. A conflict, not a permission error: the
// caller may write the row, they are just writing over an answer they never
// saw. If-Match makes the whole question moot — that path never reaches here —
// and the panel sends one; this is what protects every OTHER client.
func refuseStaleVisibility(
	ctx context.Context, tx pgx.Tx, id ids.PersonID, current crmcontracts.Person,
) error {
	var live string
	if err := tx.QueryRow(ctx,
		`SELECT visibility FROM person WHERE id = $1 FOR UPDATE`, id).Scan(&live); err != nil {
		return fmt.Errorf("people: re-reading visibility under the row lock: %w", err)
	}
	was := ""
	if current.Visibility != nil {
		was = string(*current.Visibility)
	}
	if live != was {
		return apperrors.ErrConflict
	}
	return nil
}

// refuseUnreadableResult refuses a patch whose result no human could read.
//
// The row-scope arm for a person reads the pair as
// `visibility <> 'owner' OR owner_id = me`, so a row that says 'owner' and
// names nobody satisfies neither side and is invisible to EVERY seat — its
// author, an admin, and any later attempt to repair it through this same
// endpoint. That is a record destroyed rather than a record made private, and
// it is reachable two ways: an unbounded caller narrowing a row that already
// has no owner (writescope.go admits them without the owner predicate), and
// any owner sending `{"visibility":"owner","owner_id":null}`, since owner_id
// is clearable.
//
// Refused rather than repaired by naming the caller. Whose contact it becomes
// is a decision, and quietly answering it for somebody would hand the row to
// whoever happened to press the button.
func refuseUnreadableResult(current crmcontracts.Person, in UpdatePersonInput) error {
	if in.Visibility == nil || *in.Visibility != visibilityOwner {
		return nil
	}
	named := current.OwnerId != nil
	if in.OwnerID != nil {
		named = true
	}
	for _, cleared := range in.Clear {
		if cleared == filterOwnerID {
			named = false
		}
	}
	if !named {
		return &RequiredFieldError{Field: filterOwnerID}
	}
	return nil
}

// carryHistoryIfPublished takes a contact's mail and meetings with it when this
// patch is what published the contact.
//
// The same thing POST /people/{id}/publish does at the end of its own write.
// Without it the two doors to one field disagree about what the field MEANS:
// the owner's verb carries the correspondence across, so a colleague opening
// the record finds the history, while a patch setting the same column left them
// a contact nobody has ever spoken to. Same fact, two answers, decided by which
// door the caller happened to use.
//
// WIDENING ONLY. The cohort pass links and re-derives — it is how a record
// OPENS — so running it on a narrowing would be reading it backwards. Making a
// contact private does not re-hold what was already shared; the activities keep
// their own audiences, which is what the contract says.
func (s *Store) carryHistoryIfPublished(
	ctx context.Context, tx pgx.Tx, id ids.PersonID,
	current crmcontracts.Person, in UpdatePersonInput,
) error {
	if in.Visibility == nil || *in.Visibility != visibilityWorkspace {
		return nil
	}
	if current.Visibility == nil || *current.Visibility != visibilityOwner {
		return nil
	}
	// THE OWNER'S correspondence, so the owner decides. A colleague holding a
	// write grant may move the visibility column — that is the field's rule —
	// but publishing somebody's captured mail and meetings is the stricter
	// question promoteown.go answers with ownership, and the answer does not
	// change because the caller came through a different door.
	//
	// The contact still becomes the workspace's. What stays behind is the
	// history, which the owner can carry across afterwards with their own verb.
	actor, ok := principal.Actor(ctx)
	if !ok || current.OwnerId == nil || ids.UUID(*current.OwnerId) != actor.UserID {
		return nil
	}
	_, err := s.PromotePersonCohortTx(ctx, tx, id)
	return err
}
