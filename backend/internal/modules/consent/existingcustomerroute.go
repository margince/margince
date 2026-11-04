// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// The existing-customer route (UWG §7(3)): advertising to a customer without
// consent, when all four conditions hold.
//
//  1. The address came with a sale: a won deal with the sending entity names
//     the contact as a stakeholder.
//  2. The mail advertises goods like the ones bought: the declaration that
//     covers the sale names this marketing purpose.
//  3. The customer did not object: verdictFor refuses on an objection or a
//     stop before it reaches this file.
//  4. The customer was told at collection that they may object: an admin
//     declared the notice for the sending entity, for a period the sale
//     falls in. A later order confirmation does not count, so the declaration
//     must cover the day of the sale itself.
//
// The route asks all three of its own conditions whatever the pack names. The
// one pack that declares the exception names all four, and asking less than a
// pack would be the only way this could be wrong.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ExistingCustomerGround is the sale and the declaration a marketing send rests
// on when it goes out under the existing-customer route.
type ExistingCustomerGround struct {
	NoticeID ids.UUID
	DealID   ids.UUID
	SoldAt   time.Time
}

// existingCustomerGround finds the newest sale of the sending entity to this
// contact that a live declaration covers for this purpose. None found answers
// nil.
//
// The deal's seller is the entity of its accepted offer, else the default
// entity, the one an installation with a single entity trades as.
func existingCustomerGround(
	ctx context.Context, tx pgx.Tx, contactID, purposeID string, entity ids.UUID,
) (*ExistingCustomerGround, error) {
	var g ExistingCustomerGround
	err := tx.QueryRow(ctx, `
		SELECT n.id, d.id, d.closed_at
		  FROM relationship r
		  JOIN deal d ON d.id = r.deal_id
		  JOIN existing_customer_notice n
		    ON n.legal_entity_id = $3
		   AND n.withdrawn_at IS NULL
		   AND d.closed_at >= n.effective_from
		   AND (n.effective_until IS NULL OR d.closed_at < n.effective_until)
		  JOIN existing_customer_notice_purpose np
		    ON np.notice_id = n.id AND np.purpose_id = $2
		 WHERE r.kind = 'deal_stakeholder'
		   AND r.contact_id = $1
		   AND r.archived_at IS NULL
		   AND d.status = 'won'
		   AND d.archived_at IS NULL
		   AND coalesce(
		         (SELECT o.legal_entity_id FROM offer o
		           WHERE o.deal_id = d.id AND o.status = 'accepted'
		             AND o.legal_entity_id IS NOT NULL
		           ORDER BY o.accepted_at DESC LIMIT 1),
		         (SELECT le.id FROM legal_entity le WHERE le.is_default)) = $3
		 ORDER BY d.closed_at DESC, n.id
		 LIMIT 1`, contactID, purposeID, entity).Scan(&g.NoticeID, &g.DealID, &g.SoldAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("consent: reading the existing-customer route: %w", err)
	}
	return &g, nil
}

// marketingContextTx is the marketing context of a send by whoever acts in
// ctx: the pack's exception, and the entity a sale must be with. The entity is
// read only when the pack grants an exception.
func marketingContextTx(ctx context.Context, tx pgx.Tx, w packRules) (MarketingContext, error) {
	out := MarketingContext{Exception: w.marketingException}
	if out.Exception == nil {
		return out, nil
	}
	entity, err := sendingEntityTx(ctx, tx, senderOf(ctx))
	if err != nil {
		return MarketingContext{}, err
	}
	out.SendingEntity = entity
	return out, nil
}
