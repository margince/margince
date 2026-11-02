// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// A disclosure that did not arrive leaves the duty owed.
//
// Sending the mail moves a notice case to `queued`, which is the honest word
// for "a message is on its way" — it deliberately stops short of claiming the
// subject was told. But `queued` was also where the case STAYED when the
// message bounced. The duty was not met, nobody was told, and the queue showed
// a case somebody had handled.
//
// That is the worst way for this control to fail. An open case is visible and
// gets worked; a queued case that silently failed looks better than an open one
// while being worse, so nobody looks at it again and the duty quietly stops
// being anybody's.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// MarkNoticeDeliveryFailedTx reopens every notice case this delivery was
// carrying, because the message did not arrive.
//
// Runs on the CALLER'S transaction — the one recording the bounce — so the
// failed delivery and the reopened duty commit together. A case reopened
// against a bounce that rolled back would put work back on the queue for a
// message that arrived perfectly well.
//
// ONLY FROM `queued`. A case somebody has since excused, or that a later
// disclosure completed, is not dragged back by a late report about an older
// message: the state moved on for a reason, and a bounce is evidence about one
// delivery rather than about the duty's whole history.
//
// The count records applied delivery outcomes, including cases returned to source review.
func MarkNoticeDeliveryFailedTx(ctx context.Context, tx pgx.Tx, deliveryID ids.UUID) (int, error) {
	if deliveryID.IsZero() {
		return 0, nil
	}
	// The self-join reads the prior state, because Postgres's RETURNING sees
	// only the new row and the write shape refuses an update audited with no
	// before-image. "It failed now" without "it was queued then" cannot tell a
	// reopening from a restatement of one.
	rows, err := tx.Query(ctx, `
		UPDATE privacy_notice_case c
		   SET state = @state, owner_user_id = `+noticeLiveOwnerSQL("c")+`, updated_at = now()
		  FROM privacy_notice_case prior
		 WHERE prior.id = c.id
		   AND c.delivery_id = @delivery
		   AND c.state = @prior_state
		RETURNING c.id, c.rule, prior.state, c.updated_at, prior.owner_user_id, c.owner_user_id`,
		pgx.NamedArgs{"delivery": deliveryID, "prior_state": string(NoticeQueued), fieldState: string(NoticeDeliveryFailed)})
	if err != nil {
		return 0, fmt.Errorf("consent: reopening the duties this message was carrying: %w", err)
	}
	type reopened struct {
		id              ids.UUID
		rule            string
		wasState        string
		at              time.Time
		wasOwner, owner *ids.UUID
	}
	// Drained and closed BEFORE the audit writes below: the audit runs further
	// queries on this same transaction, and a connection cannot carry an open
	// row set and a new query at once.
	var failed []reopened
	for rows.Next() {
		var r reopened
		if err := rows.Scan(&r.id, &r.rule, &r.wasState, &r.at, &r.wasOwner, &r.owner); err != nil {
			rows.Close()
			return 0, fmt.Errorf("consent: reading a reopened duty: %w", err)
		}
		failed = append(failed, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("consent: reopening the duties this message was carrying: %w", err)
	}
	for _, r := range failed {
		// Audited per case, not once for the batch: each is its own compliance
		// record, and "was this duty ever met, and what became of the attempt"
		// is asked of one case at a time.
		before := map[string]any{fieldState: r.wasState, fieldOwner: r.wasOwner}
		if _, err := storekit.Audit(ctx, tx, "update", "privacy_notice_case", r.id, before,
			map[string]any{
				fieldState: string(NoticeDeliveryFailed), fieldRule: r.rule, fieldOwner: r.owner,
			}); err != nil {
			return 0, fmt.Errorf("audit the reopened notice case: %w", err)
		}
	}
	if len(failed) > 0 {
		cases := make([]ids.UUID, 0, len(failed))
		for _, row := range failed {
			cases = append(cases, row.id)
		}
		for _, id := range cases {
			if _, err := ReviewUnverifiedNoticeTx(ctx, tx, id, failed[0].at); err != nil {
				return 0, err
			}
		}
		if err := settleFailedSenderDutiesTx(ctx, tx, cases, failed[0].at); err != nil {
			return 0, err
		}
	}
	return len(failed), nil
}
