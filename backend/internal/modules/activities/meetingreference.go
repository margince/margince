// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Whether an id names a meeting the caller may wait for.
//
// A snooze that waits for a meeting stores the meeting's id, and two set-aside
// systems store one — the waiting lane here and the brief's own queue. Both ask
// this, because the question is about the ACTIVITY table and this module owns
// it; a copy in the other would be a second answer to "is this a meeting",
// drifting the first time either was edited.
//
// Three separate failures, one guard:
//
//   - An id naming no row is a snooze nothing will ever lift, so the rep loses
//     the work with no screen anywhere saying why.
//   - An id naming something that is not a meeting turns any old email into the
//     lift condition. Its occurred_at is already past, so the snooze would end
//     the instant it was set.
//   - An id the caller cannot read would let them learn, from whether their own
//     item came back, when somebody else's meeting happened.
//
// The unreadable case answers not-found rather than a distinct error, so a
// caller cannot tell "not a meeting" from "not yours" — the same
// existence-hiding every row-scope miss takes.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// KindMeeting is what a snooze reference must be, and what both lift predicates
// filter on. Exported so the brief's queue names the same kind this module's
// SQL does rather than spelling its own.
const KindMeeting = "meeting"

// EnsureMeetingReference refuses a reopen_ref that does not name a meeting this
// caller may read. It takes a transaction because both callers check inside
// their own write, so the meeting cannot be archived between the check and the
// row that depends on it.
func EnsureMeetingReference(ctx context.Context, tx pgx.Tx, ref ids.UUID) error {
	// The OBJECT gate first, because this reaches a table the caller may have
	// no grant on at all. The disposition writer already holds activity:read
	// through judgeMessage; the brief's writer holds only deal:read, so
	// without this a rep could name activity ids and learn from the refusals
	// which ones are meetings they may see.
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return err
	}
	// Then the ROW gate — the DISCOVER one, not the content one. Waiting for a meeting to be over
	// needs only to know it exists and when it ends; it reads no subject and no
	// body. Demanding content authority would refuse a rep who can see a
	// meeting is on the calendar but not what was said in it, which is exactly
	// the person this snooze is for.
	if err := auth.EnsureActivityVisible(ctx, tx, ref); err != nil {
		return err
	}
	var kind string
	if err := tx.QueryRow(ctx, `SELECT kind FROM activity WHERE id = $1`, ref).Scan(&kind); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		}
		return fmt.Errorf("activities: reading the meeting a snooze waits for: %w", err)
	}
	if kind != KindMeeting {
		return &values.ParseError{
			Field: "reopen_ref", Code: "not_a_meeting",
			Message: "a snooze waiting for a meeting names a meeting",
		}
	}
	return nil
}
