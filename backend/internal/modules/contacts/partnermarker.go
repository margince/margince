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

// LivePartnerCompaniesBatch answers which of these companies carry a live
// partner programme, for THIS caller, inside the caller's own transaction. It
// takes the whole page's companies at once: a per-hit read would cost a round
// trip per hit, which a search page of 200 turns into 200 statements.
//
// The COMPANY's own read gate is re-derived here rather than taken on the
// caller's word — its object grant, and its row-scope clause joined to a live
// company row — so the marker never says more about an account than the
// caller may already see of it, whoever hands it the ids. This is exported,
// and an exported reader that trusts its caller is one call site away from
// being the door a partner programme leaks through. partnerListWhere spells
// the same company bound as an EXISTS because its keyset cursor forbids the
// join, so a change to what a visible company means moves both.
//
// Absent from the map means no live programme this caller may see; a caller
// who may not read both partner programmes and companies is refused instead.
func LivePartnerCompaniesBatch(ctx context.Context, tx pgx.Tx, companyIDs []ids.CompanyID) (map[ids.CompanyID]bool, error) {
	if err := auth.Require(ctx, "partner", principal.ActionRead); err != nil {
		return nil, err
	}
	if err := auth.Require(ctx, "company", principal.ActionRead); err != nil {
		return nil, err
	}
	live := make(map[ids.CompanyID]bool, len(companyIDs))
	if len(companyIDs) == 0 {
		return live, nil
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	idsPos := arg(companyIDs)
	scope, err := scopeOrAllRows(ctx, "company", "c", arg)
	if err != nil {
		return nil, fmt.Errorf("contacts: scoping live partner programmes: %w", err)
	}
	query := fmt.Sprintf(`
		SELECT p.company_id
		  FROM partner p
		  JOIN company c ON c.id = p.company_id
		 WHERE p.company_id = ANY($%d)
		   AND %s
		   AND c.archived_at IS NULL
		   AND (%s)`, idsPos, livePartnerSQL("p"), scope)
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("contacts: reading live partner programmes: %w", err)
	}
	partners, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (ids.CompanyID, error) {
		var companyID ids.CompanyID
		err := row.Scan(&companyID)
		return companyID, err
	})
	if err != nil {
		return nil, fmt.Errorf("contacts: collecting live partner programmes: %w", err)
	}
	for _, companyID := range partners {
		live[companyID] = true
	}
	return live, nil
}
