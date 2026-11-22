// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package finance

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// LedgerVisibleClause is the predicate over a company alias for the companies
// whose ledger the caller may read. An invoice can carry a salary,
// so reading the company is not enough. The caller must own it, hold a record
// grant on it, or read every row; a teammate of the owner reads no ledger.
// Empty means unbounded.
func LedgerVisibleClause(ctx context.Context, companyAlias string, arg func(any) int) (string, error) {
	var clauses []string
	for _, scope := range []func(context.Context, string, string, func(any) int) (string, error){
		auth.ScopeClauseFor, auth.OwnedOrSharedClause,
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

// ensureLedgerVisible answers ErrNotFound unless the company passes the
// company probe, live when live is set, and LedgerVisibleClause. Every read of one
// company's invoices or payments asks it. A ledger the caller may not read
// then looks the same as no ledger at all.
func ensureLedgerVisible(ctx context.Context, tx pgx.Tx, company ids.UUID, live bool) error {
	if !auth.ReadGranted(ctx, "company") {
		return auth.HideRow(ctx, "company")
	}
	var probe error
	if live {
		probe = auth.EnsureVisibleLive(ctx, tx, "company", company)
	} else {
		probe = auth.EnsureVisible(ctx, tx, "company", company)
	}
	if probe != nil {
		return probe
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	clause, err := LedgerVisibleClause(ctx, "ledger_company", arg)
	if err != nil {
		return err
	}
	if clause == "" {
		clause = "TRUE"
	}
	if live {
		clause += " AND ledger_company.archived_at IS NULL"
	}
	query := fmt.Sprintf(`SELECT EXISTS (SELECT 1 FROM company ledger_company WHERE ledger_company.id = $%d AND %s)`, arg(company), clause)
	var visible bool
	if err := tx.QueryRow(ctx, query, args...).Scan(&visible); err != nil {
		return fmt.Errorf("finance: checking who may read the ledger: %w", err)
	}
	if !visible {
		return auth.HideRow(ctx, "company")
	}
	return nil
}
