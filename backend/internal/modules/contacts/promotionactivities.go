// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const promotionExistingContactLink = `EXISTS (
	SELECT 1 FROM activity_link b WHERE b.activity_id = a.activity_id
	AND b.entity_type = 'contact' AND b.contact_id = @contact)`

// LeadPromotionActivity preserves activity lineage when a lead becomes a contact.
type LeadPromotionActivity struct {
	ID               ids.UUID
	AlreadyOnContact bool
}

func previewPromotionActivities(ctx context.Context, tx pgx.Tx, lead ids.LeadID, contact ids.ContactID) ([]LeadPromotionActivity, error) {
	rows, err := tx.Query(ctx, `SELECT a.activity_id, `+promotionExistingContactLink+`
		FROM activity_link a WHERE a.lead_id = @lead ORDER BY a.activity_id`,
		pgx.NamedArgs{leadEntity: lead, contactEntity: contact})
	if err != nil {
		return nil, fmt.Errorf("read promotion activity links: %w", err)
	}
	links, err := pgx.CollectRows(rows, pgx.RowToStructByPos[LeadPromotionActivity])
	if err != nil {
		return nil, err
	}
	if len(links) == 0 {
		return links, nil
	}
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return nil, err
	}
	for _, link := range links {
		if err := auth.EnsureActivityContentVisible(ctx, tx, link.ID); err != nil {
			return nil, err
		}
	}
	return links, nil
}

// Only moved links belong in lead.promoted; a duplicate already belonged to
// the contact and must not trigger downstream work a second time.
func carryLeadActivities(ctx context.Context, tx pgx.Tx, lead ids.LeadID, contact ids.ContactID) ([]ids.UUID, error) {
	args := pgx.NamedArgs{leadEntity: lead, contactEntity: contact}
	if _, err := tx.Exec(ctx, `DELETE FROM activity_link a
		WHERE a.lead_id = @lead AND `+promotionExistingContactLink, args); err != nil {
		return nil, fmt.Errorf("drop already-linked lead activities: %w", err)
	}
	rows, err := tx.Query(ctx, `UPDATE activity_link
		SET entity_type = 'contact', contact_id = @contact, lead_id = NULL
		WHERE lead_id = @lead RETURNING activity_id`, args)
	if err != nil {
		return nil, fmt.Errorf("carry lead activities: %w", err)
	}
	carried, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, fmt.Errorf("carry lead activities: %w", err)
	}
	return carried, nil
}
