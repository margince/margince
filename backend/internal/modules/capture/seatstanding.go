// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// Whether the seat behind a call already holds an activity — asked in one
// place, by everything that acts on a row it found by somebody else's key.
//
// Two doors ask it. A replay collides on a natural key and must decide whether
// this mailbox may record itself an importer of the message already there
// (replayClaimIsProvenTx, which ORs its own content arm on top). A calendar
// verb finds a meeting by the provider's event id and must decide whether this
// calendar is the one entitled to close or move it. Two spellings would drift,
// and the drift is silent: both doors go on answering, just differently, and
// the weaker one is the one an attacker picks.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// SeatHoldsActivityTx reports whether the seat behind this call already holds
// the activity, by any of the three standings that mean it is theirs: they
// captured the row, they filed an import row on it, or they sit on it as a
// participant.
//
// Exported because compose injects it into the module that owns `activity` —
// the answer lives here, in capture's own tables, and the decision lives there,
// under the row's lock. The same shape AddressProver travels on, for the same
// reason.
//
// A principal with no seat behind it holds NOTHING. That is the one direction
// this must not fail in: reading "I cannot tell who this is" as "the same seat"
// would open the case the predicate exists to close.
//
// auth.activityMembershipArm spells the same two of these three arms, but for
// content visibility, composed into wider clauses a caller who may only READ
// composes too. Sharing it here would hand those clauses the license to
// CANCEL or MOVE a row that this predicate grants, so it holds its own copy.
func SeatHoldsActivityTx(ctx context.Context, tx pgx.Tx, id ids.ActivityID) (bool, error) {
	seat := actorUserID(ctx)
	if seat == ids.Nil {
		return false, nil
	}
	var held bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM activity a
			 WHERE a.id = $1
			   AND (
			     a.captured_by LIKE $2
			     OR EXISTS (SELECT 1 FROM capture_import ci WHERE ci.activity_id = a.id AND ci.user_id = $3)
			     OR EXISTS (SELECT 1 FROM activity_participant ap WHERE ap.activity_id = a.id AND ap.user_id = $3)))`,
		id, "%:"+seat.String(), seat).Scan(&held); err != nil {
		return false, fmt.Errorf("capture: reading whether %s is this seat's to act on: %w", id, err)
	}
	return held, nil
}
