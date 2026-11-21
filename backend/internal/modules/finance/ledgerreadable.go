// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package finance

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
)

// LedgerVisibleClause is the predicate over a company alias for the companies
// whose ledger the caller may read. Payment authority is narrower than
// workspace-readable company identity, so the owner scope joins the read
// scope. Empty means the caller is unbounded.
func LedgerVisibleClause(ctx context.Context, companyAlias string, arg func(any) int) (string, error) {
	var clauses []string
	for _, scope := range []func(context.Context, string, string, func(any) int) (string, error){
		auth.ScopeClauseFor, auth.OwnerScopeClauseFor,
	} {
		clause, err := scope(ctx, "company", companyAlias, arg)
		if err != nil {
			return "", err
		}
		if clause != "" {
			clauses = append(clauses, "("+clause+")")
		}
	}
	return strings.Join(clauses, " AND "), nil
}

// LedgerRowReadableSQL is LedgerVisibleClause over a finance_invoice or
// finance_payment row alias. A reader without the company grant reads none.
// alias is a compile-time literal.
func LedgerRowReadableSQL(ctx context.Context, alias string, arg func(any) int) (string, error) {
	if !auth.ReadGranted(ctx, "company") {
		return "FALSE", nil
	}
	clause, err := LedgerVisibleClause(ctx, "ledger_company", arg)
	if err != nil || clause == "" {
		return "TRUE", err
	}
	return fmt.Sprintf(`EXISTS (SELECT 1 FROM company ledger_company WHERE ledger_company.id = %s.company_id AND %s)`,
		pgx.Identifier{alias}.Sanitize(), clause), nil
}
