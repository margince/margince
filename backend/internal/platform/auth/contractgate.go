// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ContractDealMatchesSQL keeps a contract on its own company when an older
// writer left its deal under another company. Aliases come from source code.
func ContractDealMatchesSQL(contract, deal string) string {
	return fmt.Sprintf("(%s.company_id IS NULL OR %s.company_id = %s.company_id)", deal, deal, contract)
}

// ContractAnchorSQL is the anchor projection for both reads and writes.
func ContractAnchorSQL(contract, deal string) (string, string) {
	condition := fmt.Sprintf("%s.id IS NOT NULL AND %s", deal, ContractDealMatchesSQL(contract, deal))
	return fmt.Sprintf("CASE WHEN %s THEN 'deal' ELSE 'company' END", condition),
		fmt.Sprintf("CASE WHEN %s THEN %s.id ELSE %s.company_id END", condition, deal, contract)
}

// ContractVisibleClause uses the deal for consistent links and the contract’s
// own company for legacy mismatches, so its account team can repair them.
func ContractVisibleClause(ctx context.Context, alias string, arg func(any) int) (string, error) {
	if alias == "" {
		alias = "contract"
	}
	dealScope, err := ScopeClauseFor(ctx, "deal", "d", arg)
	if err != nil {
		return "", err
	}
	companyScope, err := ScopeClauseFor(ctx, "company", "o", arg)
	if err != nil {
		return "", err
	}
	if dealScope == "" {
		dealScope = truePredicate
	}
	if companyScope == "" {
		companyScope = truePredicate
	}
	matches := ContractDealMatchesSQL(alias, "d")
	return fmt.Sprintf(`(
  (%[1]s.deal_id IS NOT NULL AND EXISTS (
   SELECT 1 FROM deal d WHERE d.id = %[1]s.deal_id AND d.archived_at IS NULL AND %[2]s AND %[3]s))
  OR ((%[1]s.deal_id IS NULL OR EXISTS (
   SELECT 1 FROM deal d WHERE d.id = %[1]s.deal_id AND NOT %[2]s)) AND EXISTS (
   SELECT 1 FROM company o WHERE o.id = %[1]s.company_id AND o.archived_at IS NULL AND %[4]s))
 )`, alias, matches, dealScope, companyScope), nil
}

// ContractAnchorTx shares the visible anchor between delivery and filing.
// Historical events include archived contracts; new filing does not.
func ContractAnchorTx(ctx context.Context, tx pgx.Tx, id ids.UUID, includeArchived bool) (string, ids.UUID, error) {
	if err := Require(ctx, "contract", principal.ActionRead); err != nil {
		return "", ids.Nil, err
	}
	var args []any
	arg := func(value any) int { args = append(args, value); return len(args) }
	clause, err := ContractVisibleClause(ctx, "c", arg)
	if err != nil {
		return "", ids.Nil, err
	}
	var anchor string
	var anchorID ids.UUID
	kindSQL, idSQL := ContractAnchorSQL("c", "d")
	live := ""
	if !includeArchived {
		live = " AND c.archived_at IS NULL"
	}
	query := fmt.Sprintf(`SELECT %s, %s
  FROM contract c LEFT JOIN deal d ON d.id = c.deal_id
  WHERE c.id = $%d%s AND %s`, kindSQL, idSQL, arg(id), live, clause)
	err = tx.QueryRow(ctx, query, args...).Scan(&anchor, &anchorID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ids.Nil, apperrors.ErrNotFound
	}
	return anchor, anchorID, err
}
