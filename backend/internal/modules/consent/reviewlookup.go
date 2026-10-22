// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// Which review stands over a held message.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// fieldReviewID is the review's name on the wire, in an audit payload and in a
// refusal's field. One spelling, because a typo would name a field the caller
// did not send.
const fieldReviewID = "review_id"

// LiveReviewsForIntents answers which review, if any, stands over each of these
// held messages. It implements activities.ReviewLookup; compose binds it.
//
// The grant is activity:read, the check the scheduled-send list makes before it
// has any ids to ask about. The review's own grant would refuse the rep whose
// message it is.
//
// Scoped to the caller's own reviews on initiated_by, the rep the held message
// belongs to. The caller supplies the ids. Without the scope, anybody holding
// activity:read could learn that another rep's message carries a refusal.
// ReviewForReader hides that fact with a 404.
func (s *Store) LiveReviewsForIntents(
	ctx context.Context, intents []ids.UUID,
) (map[ids.UUID]ids.UUID, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return nil, err
	}
	// A principal with no seat initiated no review, so the answer is empty
	// without sending a zero uuid into the query.
	seat := initiatingSeat(ctx)
	out := make(map[ids.UUID]ids.UUID, len(intents))
	if len(intents) == 0 || seat.IsZero() {
		return out, nil
	}
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT delivery_intent_id, id
			  FROM communication_review
			 WHERE delivery_intent_id = ANY($1::uuid[])
			   AND resolved_at IS NULL
			   AND initiated_by = $2`, intents, seat)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var intent, review ids.UUID
			if err := rows.Scan(&intent, &review); err != nil {
				return err
			}
			out[intent] = review
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("consent: finding the reviews standing over these messages: %w", err)
	}
	return out, nil
}
