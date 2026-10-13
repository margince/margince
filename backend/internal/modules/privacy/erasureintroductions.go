// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// redactIntroductionRequests clears what colleagues wrote about the subject while
// asking each other for an introduction, in the single erasure transaction.
//
// Reachable on BOTH sides of the route: a request names the subject as the contact to be
// introduced to (contact_id) and, where the path runs through somebody, as that path
// (through_contact_id). Either way the row carries four free-text columns describing
// them: the note written to be forwarded, what the requester claims the introduction is
// worth to the subject, the requester's reason for wanting it, and the introducer's
// reason for deciding as they did. Every other text column on the table is an enum or
// the capturing principal.
//
// Nothing else in this cascade reaches them. contact_id references contact ON DELETE
// CASCADE, which never fires in an erasure: the contact is ANONYMIZED rather than
// deleted, so the prose outlives the name it describes.
//
// The row stays: that an introduction was asked for and what came of it is the
// colleagues' own record of their work, and only who it was about stops being
// readable from it. An OPEN one closes, though, because a scrub a later decision can
// undo is not a scrub: Decide writes a fresh decision_reason, and May refuses every
// transition out of cancelled. The version bump settles the same race for a decision
// already in flight, which moves the row only on the version it read.
func redactIntroductionRequests(ctx context.Context, tx pgx.Tx, subject ids.ContactID) error {
	// The contact is held before this scrub reads anything, and to the commit: a
	// request CREATED after the UPDATE took its snapshot would carry the subject's
	// prose past the act that reported them erased, and introductions.Store.Create
	// probes the contact for liveness without holding it. The erasure's own cascade
	// already holds the subject by every key, and LockRow re-takes idempotently, so
	// this is the ANONYMIZE path's lock as much as a second one here.
	//
	// Archived rows lock too: the retention sweep anonymizes a contact that is
	// already archived, and a live-only lock would refuse it.
	if _, err := storekit.LockRow(ctx, tx, "contact", subject.UUID, storekit.IncludeArchived); err != nil {
		return err
	}
	// internal_reason takes the tombstone rather than a blank: intro_request_reason_present
	// refuses an empty one, and a requester's record may still say a reason was given.
	//
	// Only an OPEN ask is closed. closed_at is NULL for the completed outcomes too —
	// introduced, name_dropped, replied — so closing on that column would rewrite what
	// actually happened into "cancelled" and destroy the record this scrub spares.
	if _, err := tx.Exec(ctx, `
		UPDATE intro_request
		   SET forwardable_note = '',
		       value_for_target = '',
		       internal_reason  = $2,
		       decision_reason  = CASE WHEN decision_reason IS NULL THEN NULL ELSE $2 END,
		       status           = CASE WHEN status = ANY($3) THEN 'cancelled' ELSE status END,
		       closed_at        = CASE WHEN status = ANY($3) THEN now() ELSE closed_at END,
		       version          = version + 1
		 WHERE contact_id = $1
		    OR through_contact_id = $1`, subject, erasedMention, OpenIntroStatuses()); err != nil {
		return fmt.Errorf("redact introduction requests of an erased subject: %w", err)
	}
	return nil
}

// OpenIntroStatuses are the statuses an introduction can still be acted from, and the
// ones this erasure closes. Exported for the gate that holds it against the owner.
//
// The owner of the question is introductions.Open; this is a declared mirror of it
// because a module may not import a sibling, and the mirror is held in both directions
// by TestTheErasureClosesExactlyTheOpenIntroductions (backend/gates/openintrostatuses_test.go).
//
// It matters which three: Decide writes a fresh decision_reason from requested, and
// Cancel writes one from any of the three, so an ask left open is an ask whose prose
// can be rewritten after this scrub cleared it.
func OpenIntroStatuses() []string {
	return []string{
		string(crmcontracts.IntroRequestStatusIntroRequestStatusRequested),
		string(crmcontracts.IntroRequestStatusIntroRequestStatusAccepted),
		string(crmcontracts.IntroRequestStatusIntroRequestStatusNameDropApproved),
	}
}
