// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// A website lead is not worked until its Art. 14 notice has gone out.
//
// A lead read off a company's website is deleted when its notice bounces. A
// promotion before then would copy their details onto a record the deletion
// does not reach. A claim would start work on somebody not yet told.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// websiteLeadBounceWindow is how long after its notice was sent a website lead
// stays held. A hard bounce arrives within minutes to hours; after this window
// the notice counts as delivered.
const websiteLeadBounceWindow = "72 hours"

// ErrWebsiteLeadNotYetTold refuses promoting or claiming a website lead whose
// privacy notice may still bounce.
var ErrWebsiteLeadNotYetTold = fmt.Errorf("%w: this lead was read off a website and its privacy "+
	"notice is still pending; it can be promoted or claimed once the notice has been sent for "+
	websiteLeadBounceWindow+" without bouncing", apperrors.ErrConflict)

// refuseWebsiteLeadNotYetTold refuses when any of these leads is a website
// lead whose notice may still bounce. Every path that promotes, claims,
// assigns or merges a lead asks it; TestEveryLeadHandoverAsksTheNoticeHold
// holds the list.
func refuseWebsiteLeadNotYetTold(ctx context.Context, tx pgx.Tx, leadIDs ...ids.UUID) error {
	pending, err := websiteLeadNoticePending(ctx, tx, leadIDs...)
	if err != nil {
		return err
	}
	if pending {
		return ErrWebsiteLeadNotYetTold
	}
	return nil
}

// websiteLeadNoticePending reports whether any of these leads is a website
// lead not yet known to be told.
//
// A website lead is one the accept captured under the reserved website source,
// or one a notice row names; no request can write either.
//
// It fails closed. A website lead counts as told only when its queued notice
// was sent, carries no bounce, and has been out for the whole bounce window.
// No row, a failed row, an unsent or a bounced mail all hold the lead.
//
// Nothing stores "delivered". It is read off the delivery each time, so a late
// bounce is never hidden behind an earlier verdict.
func websiteLeadNoticePending(ctx context.Context, tx pgx.Tx, leadIDs ...ids.UUID) (bool, error) {
	var pending bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
		    SELECT 1 FROM lead l
		     WHERE l.id = ANY($1)
		       AND (l.source_system = $3
		            OR EXISTS (SELECT 1 FROM lead_privacy_notice w WHERE w.lead_id = l.id))
		       AND NOT EXISTS (
		           SELECT 1 FROM lead_privacy_notice n
		             JOIN comms_outbound o ON o.id = n.delivery_id
		            WHERE n.lead_id = l.id AND n.state = 'queued'
		              AND o.status = 'sent' AND o.bounced_at IS NULL
		              AND o.sent_at <= now() - $2::interval))`,
		leadIDs, websiteLeadBounceWindow, provenance.WebsiteLeadSource).Scan(&pending); err != nil {
		return false, fmt.Errorf("contacts: reading whether the lead's privacy notice went out: %w", err)
	}
	return pending, nil
}
