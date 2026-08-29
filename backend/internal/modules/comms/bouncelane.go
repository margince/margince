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

// hardBouncesSQL joins each bounced send to the person its activity is filed
// under through activity_link, the same table every activity surface resolves
// links through. LATERAL with LIMIT 1 rather than a plain join: an activity
// filed under several people must not put the same bounce on the lane twice.
const hardBouncesSQL = `
SELECT o.id, COALESCE(o.subject, ''), COALESCE(o.bounce_reason, ''), o.bounced_at,
       COALESCE(l.person_id, '00000000-0000-0000-0000-000000000000'::uuid)
  FROM comms_outbound o
  LEFT JOIN LATERAL (
    SELECT person_id FROM activity_link
     WHERE activity_id = o.activity_id AND entity_type = 'person'
     ORDER BY person_id LIMIT 1
  ) l ON true
 WHERE o.user_id = $1
   AND o.bounce_kind = 'hard'
   AND o.bounced_at >= $2
 ORDER BY o.bounced_at DESC, o.id DESC
 LIMIT $3`

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
	var bounced []HardBounce
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, txErr := tx.Query(ctx, hardBouncesSQL, actor.UserID, since, limit)
		if txErr != nil {
			return txErr
		}
		defer rows.Close()
		bounced = []HardBounce{}
		for rows.Next() {
			var bounce HardBounce
			if scanErr := rows.Scan(&bounce.ID, &bounce.Subject, &bounce.Reason,
				&bounce.BouncedAt, &bounce.PersonID); scanErr != nil {
				return scanErr
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
