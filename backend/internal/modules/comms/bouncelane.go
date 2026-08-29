// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package comms

// The bounce-lane read: the caller's own sends whose delivery reports came
// back hard, inside a bounded window. It reads the ground truth RecordBounce
// stamps on the row — never the capture stream — so the lane and the
// timeline cannot disagree about whether a send arrived. Soft bounces stay a
// stamp on the row: the provider is still trying, and a card would ask a
// human to act on something that may yet deliver.

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

// HardBounce is one send of the caller's that did not arrive: what it was
// about, why the receiving side refused it, when the report landed, and the
// person the send's activity is filed under — zero when it is filed under
// none, and the card then names the send by its subject line alone.
type HardBounce struct {
	ID        ids.UUID
	Subject   string
	Reason    string
	BouncedAt time.Time
	PersonID  ids.UUID
}

// hardBouncesFilter is the send half of the statement; the person half is
// assembled per call because its visibility clause is the caller's own.
const hardBouncesFilter = `
 WHERE o.user_id = $1
   AND o.bounce_kind = 'hard'
   AND o.bounced_at >= $2
 ORDER BY o.bounced_at DESC, o.id DESC
 LIMIT $3`

// subjectLineBound caps the send's subject on the way to the wire, as the
// sibling lanes cap their free text: the column is unbounded and eight
// multi-megabyte headlines would be a self-inflicted flood.
const subjectLineBound = 300

// hardBouncesSQL joins each bounced send to the person its activity is filed
// under. activity_link belongs to the activities module; this read joins it
// directly rather than through a port for the same reason consent's verdict
// read and deals' health read do — the link row is shared metadata every
// module's row-level reads resolve in their own statement. The join carries
// auth.LinkTargetVisibleClause, the one spelling of the rule every
// activity_link projection asks: owning the send says nothing about the
// visibility of the people its activity touches, and a person this caller
// may not read must not reach the wire even as a bare id. LATERAL with
// LIMIT 1 rather than a plain join: an activity filed under several people
// must not put the same bounce on the lane twice.
func hardBouncesSQL(ctx context.Context, args *[]any) (string, error) {
	arg := func(v any) int { *args = append(*args, v); return len(*args) }
	visible, err := auth.LinkTargetVisibleClause(ctx, "al", arg)
	if err != nil {
		return "", err
	}
	if visible == "" {
		visible = "TRUE"
	}
	return `
SELECT o.id, left(COALESCE(o.subject, ''), ` + fmt.Sprint(subjectLineBound) + `), COALESCE(o.bounce_reason, ''), o.bounced_at, l.person_id
  FROM comms_outbound o
  LEFT JOIN LATERAL (
    SELECT al.person_id FROM activity_link al
     WHERE al.activity_id = o.activity_id AND al.entity_type = 'person' AND ` + visible + `
     ORDER BY al.person_id LIMIT 1
  ) l ON true` + hardBouncesFilter, nil
}

// HardBouncesFor answers the calling person's own hard-bounced sends since
// `since`, newest report first, bounded. The person comes from the bound
// principal and is not a parameter — another person's bounces cannot be
// expressed — and a caller with no person behind it is refused with the
// permission sentinel, which the attention feed renders as a withheld lane.
func (s *Store) HardBouncesFor(ctx context.Context, since time.Time, limit int) ([]HardBounce, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID.IsZero() {
		return nil, fmt.Errorf("comms: reading your bounced sends needs an authenticated person: %w", apperrors.ErrPermissionDenied)
	}
	args := []any{actor.UserID, since, limit}
	statement, err := hardBouncesSQL(ctx, &args)
	if err != nil {
		return nil, err
	}
	var bounced []HardBounce
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, txErr := tx.Query(ctx, statement, args...)
		if txErr != nil {
			return txErr
		}
		defer rows.Close()
		bounced = []HardBounce{}
		for rows.Next() {
			var bounce HardBounce
			var person *ids.UUID
			if scanErr := rows.Scan(&bounce.ID, &bounce.Subject, &bounce.Reason,
				&bounce.BouncedAt, &person); scanErr != nil {
				return scanErr
			}
			if person != nil {
				bounce.PersonID = *person
			}
			bounced = append(bounced, bounce)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("comms: listing bounced sends: %w", err)
	}
	return bounced, nil
}
