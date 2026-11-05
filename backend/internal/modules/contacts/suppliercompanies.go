// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// SupplierCompanies returns visible supplier-only companies, excluding customer relationships.
func (s *Store) SupplierCompanies(ctx context.Context, companyIDs []ids.UUID) (map[ids.UUID]bool, error) {
	if err := auth.Require(ctx, companyEntity, principal.ActionRead); err != nil {
		return nil, err
	}
	suppliers := make(map[ids.UUID]bool)
	if len(companyIDs) == 0 {
		return suppliers, nil
	}
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var args []any
		arg := func(v any) int { args = append(args, v); return len(args) }
		companies := arg(companyIDs)
		visible, err := visibleCompanySQL(ctx, "c", arg)
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx, storekit.SQLf(`SELECT c.id
   FROM company c JOIN company_relationship_type role ON role.company_id = c.id
   WHERE c.id = ANY($%d) AND %s AND role.archived_at IS NULL
   GROUP BY c.id
   HAVING bool_or(role.relationship_type = 'supplier')
      AND NOT bool_or(role.relationship_type = 'customer')`, companies, visible), args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id ids.UUID
			if err := rows.Scan(&id); err != nil {
				return err
			}
			suppliers[id] = true
		}
		return rows.Err()
	})
	return suppliers, err
}
