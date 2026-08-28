// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// The open DSR cases in the order their clocks run out — the thin read a
// worklist lane needs, beside the case queue's own paged list. It answers
// less on purpose: the lane says "these are waiting and this one is due
// first"; working the case stays on the queue's screen and endpoints.

import (
	"context"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// OpenDSR is one case still owed an answer: what was asked, and by when.
type OpenDSR struct {
	ID    ids.UUID
	Kind  string
	DueAt time.Time
}

// openDSRLaneDefault mirrors the worklist's small-lane page on purpose: the
// lane exists to prompt, not to be the queue, and the caller passes its own
// bound anyway — this is only the answer for a caller that passed none.
const openDSRLaneDefault = 8

// unresolvedDSRStatuses derives "still owed an answer" from the status
// machine itself: a status is unresolved exactly while it has somewhere to
// go. A new state added to dsrTransitions reaches this lane by existing,
// instead of leaving it with an older idea of open.
func unresolvedDSRStatuses() []string {
	statuses := make([]string, 0, len(dsrTransitions))
	for status := range dsrTransitions {
		statuses = append(statuses, status)
	}
	sort.Strings(statuses)
	return statuses
}

// OpenDSRsDueSoonest lists the cases nobody has resolved, soonest deadline
// first. Gated exactly as the case queue is — requireDSRAdmin — so a caller
// the queue refuses is refused here too, and the lane above renders that
// refusal as a withheld lane rather than an empty one.
func (s *Store) OpenDSRsDueSoonest(ctx context.Context, limit int) ([]OpenDSR, error) {
	if err := requireDSRAdmin(ctx, principal.ActionRead); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = openDSRLaneDefault
	}
	var out []OpenDSR
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, kind, due_at FROM data_subject_request
			 WHERE status = ANY($1)
			 ORDER BY due_at, id
			 LIMIT $2`, unresolvedDSRStatuses(), limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var d OpenDSR
			if err := rows.Scan(&d.ID, &d.Kind, &d.DueAt); err != nil {
				return err
			}
			out = append(out, d)
		}
		return rows.Err()
	})
	return out, err
}
