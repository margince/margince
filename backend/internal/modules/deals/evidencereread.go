// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/owndomain"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// dealMailSentFromPage selects live activities on a deal whose sender is an
// address at the domain or a subdomain. The sender index serves role 'from'.
var dealMailSentFromPage = `
	SELECT a.id
	  FROM activity a
	 WHERE a.id > $2 AND a.archived_at IS NULL
	   AND EXISTS (SELECT 1 FROM activity_link l
	                WHERE l.activity_id = a.id AND l.deal_id IS NOT NULL)
	   AND a.id IN (SELECT p.activity_id FROM activity_participant p
	                 WHERE p.role = 'from' AND p.user_id IS NULL
	                   AND p.address_domain_folded IS NOT NULL
	                   AND ` + owndomain.CoversIndexedSQL("p.address_domain_folded", "$1") + `)
	 ORDER BY a.id LIMIT $3`

// DealMailSentFrom answers one page of the activities a removed own domain's
// senders wrote, after the cursor. A reading refused while the domain counted
// as ours left no evidence row, so these are the texts to read again.
//
// It names activities and reads no text. Whether one may still be read is the
// reading's own check.
func DealMailSentFrom(ctx context.Context, tx pgx.Tx, domain string, after ids.UUID, limit int) ([]ids.UUID, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, err
	}
	folded := owndomain.Fold(domain)
	if folded == "" {
		return nil, nil
	}
	rows, err := tx.Query(ctx, dealMailSentFromPage, folded, after, limit)
	if err != nil {
		return nil, fmt.Errorf("deals: reading the mail a removed own domain sent: %w", err)
	}
	page, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, fmt.Errorf("deals: reading the mail a removed own domain sent: %w", err)
	}
	return page, nil
}
