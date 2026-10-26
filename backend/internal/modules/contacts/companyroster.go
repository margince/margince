// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// CompanyRosterSQL selects, as contact_id, the account's current employees
// this caller may read: the edge gate and the contact row scope both hold.
// company is a SQL expression, a bind or a correlated column. A caller who
// may not read contacts or edges is refused.
func CompanyRosterSQL(ctx context.Context, company string, arg func(any) int) (string, error) {
	if err := auth.Require(ctx, "contact", principal.ActionRead); err != nil {
		return "", err
	}
	// "Who works at this account" is the fact relationship.read governs.
	edgeBound, err := auth.EdgeReadScope(ctx, "r", arg)
	if err != nil {
		return "", err
	}
	if edgeBound == "" {
		edgeBound = "TRUE"
	}
	scope, err := contactScopePredicate(ctx, arg)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(`SELECT p.id AS contact_id FROM contact p
		JOIN relationship r ON r.contact_id = p.id
		WHERE r.kind = 'employment' AND r.company_id = %s
		  AND `+employment.IsCurrentSQL("r.ended_at")+` AND r.archived_at IS NULL
		  AND p.archived_at IS NULL AND (%s) AND (%s)`, company, edgeBound, scope), nil
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
