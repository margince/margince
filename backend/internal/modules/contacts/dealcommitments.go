// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// dealFiledSQL makes a claim the deal's: the message it was read from is filed
// under the deal. Working at the deal's company is not enough, because a
// company with several open deals would show every deal's promises on each.
func dealFiledSQL(dealArg int) string {
	return fmt.Sprintf(`EXISTS (SELECT 1 FROM activity_link dl
		 WHERE dl.activity_id = c.source_activity_id AND dl.entity_type = 'deal' AND dl.deal_id = $%d)`, dealArg)
}

// OpenTheirCommitmentsForDeal reads the open commitments the customer made in
// messages filed under one deal: what the deal page watches. Same gates,
// ranking and completeness count as the account reads.
func (s *Store) OpenTheirCommitmentsForDeal(
	ctx context.Context, tx pgx.Tx, dealID ids.UUID, limit int,
) ([]CompanyCommitment, bool, error) {
	if err := auth.Require(ctx, "contact", principal.ActionRead); err != nil {
		return nil, false, err
	}
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return nil, false, err
	}
	asOf := s.clock()
	filed := func(arg func(any) int) (string, error) { return dealFiledSQL(arg(dealID)), nil }
	out, admitted, err := rankedClaims(ctx, tx, filed, limit, asOf, theirPromiseOpen)
	if err != nil {
		return nil, false, err
	}
	total, err := countDealCommitments(ctx, tx, dealID, asOf)
	if err != nil {
		return nil, false, err
	}
	return out, admitted >= total, nil
}

// countDealCommitments is how many open customer promises the deal carries,
// asked without the caller's scope and answering only a number, for the reason
// countCompanyCommitments gives.
func countDealCommitments(ctx context.Context, tx pgx.Tx, dealID ids.UUID, asOf time.Time) (int, error) {
	if err := auth.Require(ctx, "contact", principal.ActionRead); err != nil {
		return 0, err
	}
	args := []any{dealID}
	arg := func(v any) int { args = append(args, v); return len(args) }
	var total int
	query := `
		SELECT count(*)
		  FROM conversation_claim c
		  JOIN activity a ON a.id = c.source_activity_id AND a.archived_at IS NULL
		  JOIN contact pr ON pr.id = c.contact_id AND pr.archived_at IS NULL
		 WHERE ` + theirPromiseOpen(asOf, arg) + `
		   AND c.archived_at IS NULL AND ` + dealFiledSQL(1)
	err := tx.QueryRow(ctx, query, args...).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("count the deal's open commitments: %w", err)
	}
	return total, nil
}
