// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// The messages nobody has sent yet. A scheduled send (ADR-0104) holds a frozen
// payload — the addresses, the subject and the body — before any activity
// exists, which is exactly why the rest of this package cannot reach it: every
// other outbound scrub keys off the activity the message became, and a message
// that has not fired has none.
//
// Left alone, a subject who exercises Art. 17 the night before a scheduled send
// gets the message anyway, at nine the next morning, from a system that has
// just certified their data destroyed. Cancelled and held rows are worse: they
// have no timer at all, so their copy of the address and the body would sit
// there indefinitely with nothing that would ever look at them again.
//
// Kept in its own file for the reason deliveries.go is: both destructive
// engines call it, and it belongs to neither.

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// redactScheduledSends empties every scheduled message addressed to one of the
// subject's addresses, and stops the ones that have not fired.
//
// It selects on ADDRESSES rather than on activities, which is the opposite of
// redactDeliveries and forced by the shape of the thing: a scheduled send has
// no activity to inherit a decision from. That means it also inherits none of
// the activity engine's shields — but there is nothing for them to protect
// here. The statutory correspondence floor exists to keep SENT business
// letters; a message nobody has sent is not correspondence yet, and a
// Handelsbrief that was never sent is not one.
//
// Pending rows are CANCELLED rather than merely emptied. A scheduled row whose
// payload is blank still has a live timer, and the fire path would replay an
// empty message to an empty address list — a send that fails confusingly at
// best, and at worst one the gates cannot refuse because there is no longer
// anyone named to refuse on behalf of. Cancelling is what makes the timer a
// no-op, and it is terminal to every other transition.
//
// Released rows are emptied but keep their status: the message was sent, the
// activity and the delivery record that fact, and both are scrubbed by the
// engines that own them. Rewriting the status here would falsify what happened.
//
// A HELD ROW IS CANCELLED TOO, for the scheduled row's reason and one of its
// own. A refused send freezes its message into a held row so a human can decide
// what happens to it; once the subject is erased there is no decision left to
// take, and leaving the row resumable would let somebody fire an emptied
// message at an emptied address list.
//
// It is also what the state-shape CHECK requires. That constraint reserves
// held_reason for holds — a row still reading 'held' with the reason cleared
// violates it and aborts the whole erasure, which is a subject's Art. 17
// request failing on a message they asked to be rid of.
//
// No reason is written anywhere. A cancelled row carries none by design (the
// state-shape CHECK reserves held_reason for holds), and the delivery scrub's
// park reason has no analogue here: nothing was handed to a provider, so there
// is no operator waiting to be told why a transmission stopped.
func redactScheduledSends(ctx context.Context, tx pgx.Tx, reason string, emails []string) error {
	if len(emails) == 0 {
		return nil
	}
	// CASE-INSENSITIVELY, because the payload keeps the address the SENDER
	// TYPED and the erasure list carries the address as it was stored. A rep
	// who wrote Subject@example.com to a contact recorded as
	// subject@example.com would otherwise leave that message behind — holding
	// their name, their address and the words meant for them — after the
	// installation had certified their data destroyed. The domain half is
	// case-insensitive by RFC and every mailbox in practice treats the local
	// half that way too, which is why contact_email is matched on lower()
	// everywhere else in this engine.
	//
	// The payload keeps To, Cc and Bcc in one merged recipients list, so one
	// test would cover all three — a blind copy is not a hiding place — but
	// all three are read anyway, because a payload writer added later may not
	// merge them.
	//
	// THROUGH jsonb_path_query_array RATHER THAN A CONCATENATION, because
	// concatenating the three fields raises if any of them is not an array,
	// and a raise here aborts the whole erasure — a subject's Art. 17 request
	// failing on a malformed row. The path expression skips what it cannot
	// walk. Every payload this build writes holds arrays there; the shape is
	// not enforced by a constraint, and an erasure is the wrong place to
	// discover that.
	// The schedule's audit image carries the message SUBJECT verbatim, so
	// emptying the payload is only half the scrub: the other half is the
	// tombstone that stops the spine's readers before the image that quoted it.
	scrubbed, err := scrubbedIDs(ctx, tx, `
		UPDATE scheduled_send
		   SET payload = jsonb_build_object(
		         'recipients', '[]'::jsonb,
		         'subject', ''::text,
		         'body', ''::text,
		         'consent_purpose', payload->>'consent_purpose'),
		       status = CASE WHEN status IN ('scheduled', 'held') THEN 'cancelled' ELSE status END,
		       held_reason = NULL,
		       version = version + 1,
		       updated_at = now()
		 WHERE EXISTS (
		         SELECT 1
		           FROM jsonb_array_elements_text(
		                  jsonb_path_query_array(payload,
		                    '$.keyvalue() ? (@.key == "recipients" || @.key == "cc" ||
		                                     @.key == "bcc").value[*]')) AS addressed(address)
		          WHERE lower(addressed.address) = ANY($1))
		 RETURNING id`, loweredAddresses(emails))
	if err != nil {
		return fmt.Errorf("redacting the scheduled messages addressed to the subject: %w", err)
	}
	if err := closeReviewsForErasedMessages(ctx, tx, reason, scrubbed); err != nil {
		return err
	}
	return tombstoneCollateralScrubs(ctx, tx, "scheduled_send", scrubbed, reason, causeContactErasure)
}

// closeReviewsForErasedMessages ends the reviews standing over the messages the
// scrub above just cancelled.
//
// WHY THIS IS NOT ALREADY DONE by the erasure's own review scrub. That scrub
// (erasure_consent.go, clearRefusedSendReviews) empties the REFUSALS — the
// addresses and subject ids a review recorded — and deliberately leaves the row
// standing so a human's queue does not develop holes. What it does not touch is
// the review's LIVENESS, and a live review over an erased subject's cancelled
// message is work somebody can still act on: directing that send fires an
// emptied message at an emptied address list, from a system that has just
// certified the subject's data destroyed.
//
// Cancelled, since nothing was sent. The account matches the one consent's own
// closer writes for the same move, so two cancelled reviews read alike
// whichever engine ended them.
//
// The SQL is written here because privacy may not import consent. An erasure
// must not depend on a seam a deployment could leave unwired.
func closeReviewsForErasedMessages(ctx context.Context, tx pgx.Tx, reason string, intents []ids.UUID) error {
	if len(intents) == 0 {
		return nil
	}
	// RETURNING the ids, so each closure can be tombstoned below. An erasure
	// that ended somebody's work and recorded nothing is exactly the silent
	// half this engine writes tombstones to avoid.
	closed, err := scrubbedIDs(ctx, tx, `
		UPDATE communication_review
		   SET state = 'cancelled', resolved_at = now()
		 WHERE delivery_intent_id = ANY($1::uuid[]) AND resolved_at IS NULL
		RETURNING id`, intents)
	if err != nil {
		return fmt.Errorf("closing the reviews over the subject's cancelled messages: %w", err)
	}
	return tombstoneCollateralScrubs(ctx, tx, "communication_review", closed, reason, causeContactErasure)
}

// loweredAddresses folds the erasure's address list for the comparison above.
// The list arrives as stored, and what it is compared against is a sender's
// typing — see the query.
func loweredAddresses(emails []string) []string {
	out := make([]string, 0, len(emails))
	for _, e := range emails {
		out = append(out, strings.ToLower(e))
	}
	return out
}
