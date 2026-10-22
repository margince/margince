// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Pending-only writes keep late worker replays from overturning a human answer.
func settleDisposition(ctx context.Context, tx pgx.Tx, in ResolveDomainTriageInput, companyID *ids.CompanyID) error {
	var readID *ids.UUID
	if !in.ReadID.IsZero() {
		readID = &in.ReadID
	}
	if _, err := tx.Exec(ctx, `
		UPDATE company_domain_disposition
		   SET status = $2, source = $3, evidence = COALESCE(NULLIF($4, ''), evidence),
		       company_id = $5, site_read_id = COALESCE($6, site_read_id),
		       -- The question is answered, so it is no longer waiting on
		       -- evidence. Leaving the marker would keep a settled domain in
		       -- the "needs a human" list for ever.
		       pending_reason = NULL,
		       next_attempt_at = NULL, updated_at = now()
		 WHERE domain = $1 AND status = 'pending'`,
		in.Domain, in.Status, in.Source, in.Evidence, companyID, readID); err != nil {
		return fmt.Errorf("contacts: settling the disposition of %s: %w", in.Domain, err)
	}
	return nil
}
