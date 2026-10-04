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

// Both reads below lock the pairs they return FOR SHARE: the caller acts on
// "these two are one business" later in its transaction, and a reviewer
// answering "not a duplicate" in between waits for it rather than being
// overtaken by a decision taken on the old answer.

// openCompanyPairClause admits a company dedupe pair that is still open: a
// pair already merged is one record, a pair a human answered as not a
// duplicate is two businesses, and a pair with an archived end is closed, as
// the dedupe queue treats it.
func openCompanyPairClause() string {
	return `entity_type = '` + entityCompany + `' AND disposition = 'open' AND archived_at IS NULL
	   AND ` + bothSidesServable(entityCompany, "twin", "left_company_id", "right_company_id", "", true)
}

// OpenDuplicateCompaniesTx answers the companies an open duplicate candidate
// pairs with this one: records a user may yet merge into it. System-only: it
// reads past row scope.
func OpenDuplicateCompaniesTx(ctx context.Context, tx pgx.Tx, company ids.UUID) ([]ids.UUID, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
		SELECT CASE WHEN left_company_id = $1 THEN right_company_id ELSE left_company_id END
		  FROM dedupe_candidate
		 WHERE (left_company_id = $1 OR right_company_id = $1)
		   AND `+openCompanyPairClause()+`
		   FOR SHARE OF dedupe_candidate`,
		company)
	if err != nil {
		return nil, fmt.Errorf("contacts: reading a company's open duplicate candidates: %w", err)
	}
	twins, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, fmt.Errorf("contacts: reading a company's open duplicate candidates: %w", err)
	}
	return twins, nil
}

// OpenDuplicateCompanyPairsTx answers every open company duplicate pair in the
// workspace, each as its two company ids. System-only: it reads past row scope.
func OpenDuplicateCompanyPairsTx(ctx context.Context, tx pgx.Tx) ([][2]ids.UUID, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
		SELECT left_company_id, right_company_id FROM dedupe_candidate
		 WHERE `+openCompanyPairClause()+`
		   FOR SHARE OF dedupe_candidate`)
	if err != nil {
		return nil, fmt.Errorf("contacts: reading the open company duplicate pairs: %w", err)
	}
	pairs, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) ([2]ids.UUID, error) {
		var pair [2]ids.UUID
		err := row.Scan(&pair[0], &pair[1])
		return pair, err
	})
	if err != nil {
		return nil, fmt.Errorf("contacts: reading the open company duplicate pairs: %w", err)
	}
	return pairs, nil
}
