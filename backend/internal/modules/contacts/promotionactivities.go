// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// promotionExistingContactLink is the one test, for the preview and the carry,
// that a lead's link duplicates one the contact already holds.
func promotionExistingContactLink(link, contact string) string {
	return `EXISTS (SELECT 1 FROM activity_link b WHERE b.activity_id = ` + link + `.activity_id
	AND b.entity_type = 'contact' AND b.contact_id = ` + contact + `)`
}

// LeadPromotionActivity preserves activity lineage when a lead becomes a contact.
type LeadPromotionActivity struct {
	ID               ids.UUID
	AlreadyOnContact bool
}

// previewPromotionActivities answers the lead's activities the reader may read,
// and how many more move with the lead that the reader may not. Every link
// moves either way; the reader is shown only what its audience admits them to.
func previewPromotionActivities(ctx context.Context, tx pgx.Tx, lead ids.LeadID, contact ids.ContactID) ([]LeadPromotionActivity, int, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	leadPos, contactPos := arg(lead), arg(contact)
	readable, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return nil, 0, err
	}
	rows, err := tx.Query(ctx, storekit.SQLf(`SELECT l.activity_id, %s,
		EXISTS (SELECT 1 FROM activity a WHERE a.id = l.activity_id AND %s)
		FROM activity_link l WHERE l.lead_id = $%d ORDER BY l.activity_id`,
		promotionExistingContactLink("l", storekit.SQLf("$%d", contactPos)), readable, leadPos), args...)
	if err != nil {
		return nil, 0, fmt.Errorf("read promotion activity links: %w", err)
	}
	type promotionLink struct {
		LeadPromotionActivity
		Readable bool
	}
	links, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (promotionLink, error) {
		var link promotionLink
		err := row.Scan(&link.ID, &link.AlreadyOnContact, &link.Readable)
		return link, err
	})
	if err != nil {
		return nil, 0, fmt.Errorf("read promotion activity links: %w", err)
	}
	shown := []LeadPromotionActivity{}
	if len(links) == 0 {
		return shown, 0, nil
	}
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return nil, 0, err
	}
	withheld := 0
	for _, link := range links {
		if !link.Readable {
			withheld++
			continue
		}
		shown = append(shown, link.LeadPromotionActivity)
	}
	return shown, withheld, nil
}

// Only moved links belong in lead.promoted; a duplicate already belonged to
// the contact and must not trigger downstream work a second time.
func carryLeadActivities(ctx context.Context, tx pgx.Tx, lead ids.LeadID, contact ids.ContactID) ([]ids.UUID, error) {
	args := pgx.NamedArgs{leadEntity: lead, contactEntity: contact}
	if _, err := tx.Exec(ctx, `DELETE FROM activity_link a
		WHERE a.lead_id = @lead AND `+promotionExistingContactLink("a", "@"+contactEntity), args); err != nil {
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
