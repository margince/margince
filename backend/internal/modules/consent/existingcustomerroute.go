// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// The existing-customer route (UWG §7(3)): advertising to a customer without
// consent, when all four conditions hold.
//
//  1. The address came with a sale: an offer the sending entity issued was
//     accepted, and the contact was its buyer before the acceptance. The
//     buyer is the deal's economic buyer, or a billing contact of the
//     offer's buyer company. A stakeholder who only advised is not the
//     customer. The sale is the accepted offer, not the deal's own status or
//     close date, which a rep may edit at will. An offer that names no
//     issuing entity carries no sale for any entity.
//  2. The mail advertises goods like the ones bought: the declaration that
//     covers the sale names this marketing purpose.
//  3. The customer did not object: verdictFor refuses on an objection or a
//     stop before it reaches this file.
//  4. The customer was told at collection that they may object: an admin
//     declared the notice for the sending entity, for a period the acceptance
//     falls in. A later order confirmation does not count, so the declaration
//     must cover the moment of the sale itself.
//
// The route asks all three of its own conditions whatever the pack names. The
// one pack that declares the exception names all four, and asking less than a
// pack would be the only way this could be wrong.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/dealrole"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// ExistingCustomerGround is the sale and the declaration a marketing send rests
// on when it goes out under the existing-customer route.
type ExistingCustomerGround = commsauthz.ExistingCustomerGround

// noticeCoversFromSQL is the first moment a sale counts under declaration n.
// That is its start, but no earlier than its proof's date. Without proof it is
// no earlier than the day it was declared. The route and the list both read it.
const noticeCoversFromSQL = `greatest(n.effective_from, coalesce(n.evidence_dated_at, n.created_at))`

// buyerRoles are the deal roles that make a stakeholder the one who bought.
var buyerRoles = []string{dealrole.EconomicBuyer, dealrole.LegacyDecisionMaker}

// existingCustomerGround finds the newest sale of the sending entity to this
// contact that a live declaration covers for this purpose. None found, or no
// known entity, answers nil.
//
// A declaration covers sales from its effective start, but no earlier than
// the date of the evidence it names. Without evidence it covers nothing before
// the day it was declared. A period back-dated on the admin's word alone covers
// nothing before that day.
func existingCustomerGround(
	ctx context.Context, tx pgx.Tx, contactID, purposeID string, entity ids.UUID,
) (*ExistingCustomerGround, error) {
	if entity.IsZero() {
		return nil, nil
	}
	var g ExistingCustomerGround
	err := tx.QueryRow(ctx, `
		SELECT n.id, o.id, o.accepted_at
		  FROM offer o
		  JOIN existing_customer_notice n
		    ON n.legal_entity_id = o.legal_entity_id
		   AND n.withdrawn_at IS NULL
		   AND o.accepted_at >= `+noticeCoversFromSQL+`
		   AND (n.effective_until IS NULL OR o.accepted_at < n.effective_until)
		  JOIN existing_customer_notice_purpose np
		    ON np.notice_id = n.id AND np.purpose_id = $2
		 WHERE o.status = 'accepted'
		   AND o.legal_entity_id = $3
		   AND (EXISTS (
		         SELECT 1 FROM relationship r
		          WHERE r.kind = 'deal_stakeholder' AND r.deal_id = o.deal_id
		            AND r.contact_id = $1 AND r.archived_at IS NULL
		            AND r.role = ANY($4) AND r.created_at <= o.accepted_at)
		     OR EXISTS (
		         SELECT 1 FROM relationship b
		          WHERE b.kind = 'billing_contact' AND b.company_id = o.buyer_company_id
		            AND b.contact_id = $1 AND b.archived_at IS NULL
		            AND b.created_at <= o.accepted_at))
		 ORDER BY o.accepted_at DESC, n.id
		 LIMIT 1`, contactID, purposeID, entity, buyerRoles).Scan(&g.NoticeID, &g.OfferID, &g.SoldAt)
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
	entity, err := sendingEntityOf(ctx, tx)
	if err != nil {
		return MarketingContext{}, err
	}
	out.SendingEntity = entity
	return out, nil
}
