// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package comms

// The undelivered-lane read: the caller's own sends that were given up on,
// inside a bounded window.
//
// The sibling lane beside it (bouncelane.go) carries mail that ARRIVED and was
// refused. This one carries mail that never left: the ladder ran out, the
// mailbox would not transmit, the provider refused outright. To the sender the
// two look identical from the outside — a thread that goes quiet — but only
// this one leaves the message still unsent, which is why it says so in the
// present tense rather than reporting a report.
//
// It reads parked_at, not the parked STATUS: the status is also worn by a send
// parked after its message went out, and by one an erasure or a restriction
// stopped. Neither is a failure the sender must answer for, and both would put
// a card on a queue that promises everything on it needs a person.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ParkedSend is one send of the caller's that was given up on: what it was
// about, why it was abandoned in the dispatcher's own words, when that was
// decided, and the person the send's activity is filed under — zero when it is
// filed under none, and the card then names the send by its subject line
// alone.
type ParkedSend struct {
	ID       ids.UUID
	Subject  string
	Reason   string
	ParkedAt time.Time
	PersonID ids.UUID
}

// parkedSendsSQL joins each abandoned send to the person its activity is filed
// under, through the same clause and for the same reasons the bounce read
// documents: owning the send says nothing about who its activity touches, and
// a person this caller may not read must not reach the wire even as a bare id.
//
// The IS NOT NULL is not redundant beside the window, though the window would
// exclude an unstamped row on its own: it is the predicate the lane's partial
// index is built on, and a query that does not state it cannot be proved to
// imply it, so the planner would fall back to a scan of everything this sender
// ever sent.
func parkedSendsSQL(ctx context.Context, userID ids.UUID, since time.Time, limit int, args *[]any) (string, error) {
	arg := func(v any) int { *args = append(*args, v); return len(*args) }
	visible, err := auth.LinkTargetVisibleClause(ctx, "al", arg)
	if err != nil {
		return "", err
	}
	if visible == "" {
		visible = "TRUE"
	}
	return fmt.Sprintf(`
SELECT o.id, left(COALESCE(o.subject, ''), %d), COALESCE(o.reason, ''), o.parked_at, l.person_id
  FROM comms_outbound o
  LEFT JOIN LATERAL (
    SELECT al.person_id FROM activity_link al
     WHERE al.activity_id = o.activity_id AND al.entity_type = 'person' AND `+visible+`
     ORDER BY al.person_id LIMIT 1
  ) l ON true
 WHERE o.user_id = $%d
   AND o.parked_at IS NOT NULL
   AND o.parked_at >= $%d
 ORDER BY o.parked_at DESC, o.id DESC
 LIMIT $%d`, subjectLineBound, arg(userID), arg(since), arg(limit)), nil
}

// ParkedSendsFor answers the calling person's own abandoned sends since
// `since`, newest first, bounded. The person comes from the bound principal
// and is not a parameter — another person's failures cannot be expressed — and
// a caller with no person behind it is refused with the permission sentinel,
// which the attention feed renders as a withheld lane.
func (s *Store) ParkedSendsFor(ctx context.Context, since time.Time, limit int) ([]ParkedSend, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID.IsZero() {
		return nil, fmt.Errorf("comms: reading your undelivered sends needs an authenticated person: %w", apperrors.ErrPermissionDenied)
	}
	// A send is an activity, and reading one back — subject line included —
	// carries the activity read grant like every other timeline read.
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return nil, err
	}
	var args []any
	statement, err := parkedSendsSQL(ctx, actor.UserID, since, limit, &args)
	if err != nil {
		return nil, err
	}
	var parked []ParkedSend
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, txErr := tx.Query(ctx, statement, args...)
		if txErr != nil {
			return txErr
		}
		defer rows.Close()
		parked = []ParkedSend{}
		for rows.Next() {
			var send ParkedSend
			var person *ids.UUID
			if scanErr := rows.Scan(&send.ID, &send.Subject, &send.Reason,
				&send.ParkedAt, &person); scanErr != nil {
				return scanErr
			}
			if person != nil {
				send.PersonID = *person
			}
			parked = append(parked, send)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("comms: listing undelivered sends: %w", err)
	}
	return parked, nil
}
