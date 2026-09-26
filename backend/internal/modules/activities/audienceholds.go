// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Letting go of a hold this module put on an activity's audience.
//
// Both are scoped by the reason VALUE and not by id alone: `audience_reason`
// has several writers and each may clear only its own, which is why these are
// two writers rather than one un-hold.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ClearCounterpartyHoldTx removes a row-level counterparty hold from the
// activities a seat's widening pass re-opened, and only from those.
//
// It lives here because activities owns the activity table; capture reaches it
// through the same function-typed seam it already uses for the recompute, which
// compose wires. A copy of this UPDATE inside capture is what the
// table-ownership gate refuses, and refuses for the reason this function's
// predicate demonstrates: the column has more than one writer and each one has
// to know which values are its own to clear.
//
// Scoped by VALUE, not by id alone. audience_reason also records a workspace
// floor, a sender's confidential marker, and a human's manual narrowing through
// SetAudience — clearing it unconditionally would republish messages nobody
// asked about. The predicate is what makes this a counterparty-only widening
// rather than a general un-hold.
func ClearCounterpartyHoldTx(ctx context.Context, tx pgx.Tx, activityIDs []ids.ActivityID) error {
	if len(activityIDs) == 0 {
		return nil
	}
	if _, err := tx.Exec(ctx, `
		UPDATE activity
		   SET audience_reason = NULL
		 WHERE id = ANY($1) AND audience_reason = $2`,
		activityIDs, ReasonCounterparty); err != nil {
		return fmt.Errorf("activities: clearing the counterparty hold from re-opened rows: %w", err)
	}
	return nil
}

// ClearConfidentialityVerdictHoldTx removes the one row-carried hold an owner
// is entitled to lift: a CLASSIFIER's confidentiality answer.
//
// Its sibling above clears a counterparty hold; this exists for the same reason
// and answers a different question. A row-carried hold outranks an opening
// contribution, which is right for the holds a recipient may not lift and wrong
// for the one they may. The owner pressed Share, the ledger recorded
// `shared_by_owner`, the derivation ran, read the row's reason and left the
// message held — the share worked and the hold ignored it.
//
// The predicate is the subtle part. `explicitly_confidential` on a row means
// two things that cannot be told apart by the word:
//
//   - the SENDER marked their own subject line. Not a recipient's to lift.
//   - a CLASSIFIER concluded the text asks for confidence. A judgement, and the
//     owner may disagree with it.
//
// So the CALLER decides which rows qualify and this writes them. capture owns
// the thread ledger and can see that a row's hold came from a classifier — a
// verdict recorded a `kind`, a sink marking never does — while this module owns
// `activity` and is the only one entitled to write the column. Reading the
// ledger here would be a module reaching into a sibling's table, which the
// ownership gate refuses and which would put the same rule in two places.
//
// Newer rows do not need any of this: capture's rowReasonForKind now sends a
// classifier's answer to the row as the generic `verdict`, which is not
// row-carried at all. The rows judged before that landed still carry the word,
// and they are the ones a rep cannot share today.
//
// Every other reason is left where it is: `counterparty` is this seat's standing
// decision about a CONTACT rather than this message, `workspace_floor` is an
// admin's decision one seat cannot overrule, and `no_record` / `no_counterparty`
// are the capture ladder's filing facts rather than judgements about
// confidentiality. The value predicate stays in the UPDATE even though the
// caller has already chosen the ids, for the reason its sibling states: the
// column has more than one writer, and each has to say which values are its own
// to clear.
func ClearConfidentialityVerdictHoldTx(
	ctx context.Context, tx pgx.Tx, activityIDs []ids.ActivityID,
) error {
	if len(activityIDs) == 0 {
		return nil
	}
	if _, err := tx.Exec(ctx, `
		UPDATE activity
		   SET audience_reason = NULL
		 WHERE id = ANY($1) AND audience_reason = $2`,
		activityIDs, ReasonConfidentialMarker); err != nil {
		return fmt.Errorf("activities: clearing a confidentiality verdict the owner has shared past: %w", err)
	}
	return nil
}
