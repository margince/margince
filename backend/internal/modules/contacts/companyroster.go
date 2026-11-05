// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// CompanyRosterSQL selects, as contact_id, the account's current employees
// this caller may read: the edge gate and the contact row scope both hold.
// company is a SQL expression, a bind or a correlated column. A caller who
// may not read contacts or edges is refused.
func CompanyRosterSQL(ctx context.Context, company string, arg func(any) int) (string, error) {
	edges, err := CompanyRosterEdgesSQL(ctx, company, arg)
	if err != nil {
		return "", err
	}
	return "SELECT DISTINCT contact_id FROM (" + edges + ") roster", nil
}

// CompanyRosterTx reads CompanyRosterSQL for one account, in id order.
func CompanyRosterTx(ctx context.Context, tx pgx.Tx, companyID ids.CompanyID) ([]ids.ContactID, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	roster, err := CompanyRosterSQL(ctx, fmt.Sprintf("$%d", arg(companyID)), arg)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, roster+` ORDER BY contact_id`, args...)
	if err != nil {
		return nil, fmt.Errorf("contacts: reading the account's roster: %w", err)
	}
	contacts, err := pgx.CollectRows(rows, pgx.RowTo[ids.ContactID])
	if err != nil {
		return nil, fmt.Errorf("contacts: reading the account's roster: %w", err)
	}
	return contacts, nil
}

// CompanyRosterEdgesSQL retains edge identities for the roster's employment details.
func CompanyRosterEdgesSQL(ctx context.Context, company string, arg func(any) int) (string, error) {
	from, err := countedEmploymentFrom(ctx, "rel.company_id = "+company, arg)
	if err != nil {
		return "", err
	}
	return "SELECT p.id AS contact_id, rel.id AS relationship_id" + from, nil
}
