// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// OpenDuplicateCompaniesTx answers the companies an open duplicate candidate
// pairs with this one: records a user may yet merge into it. A pair already
// merged is one record, a pair a human answered as not a duplicate is two
// businesses, and a pair with an archived end is closed, as the dedupe queue
// treats it; none is returned. System-only: it reads past row scope.
func OpenDuplicateCompaniesTx(ctx context.Context, tx pgx.Tx, company ids.UUID) ([]ids.UUID, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
		SELECT CASE WHEN left_company_id = $1 THEN right_company_id ELSE left_company_id END
		  FROM dedupe_candidate
		 WHERE entity_type = $2 AND disposition = 'open' AND archived_at IS NULL
		   AND (left_company_id = $1 OR right_company_id = $1)
		   AND `+bothSidesServable(entityCompany, "twin", "left_company_id", "right_company_id", "", true),
		company, entityCompany)
	if err != nil {
		return nil, fmt.Errorf("contacts: reading a company's open duplicate candidates: %w", err)
	}
	twins, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, fmt.Errorf("contacts: reading a company's open duplicate candidates: %w", err)
	}
	return twins, nil
}
