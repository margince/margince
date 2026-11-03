// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// RefileCompaniesTx retains both ends of a company change despite later edits.
func (s *Store) RefileCompaniesTx(ctx context.Context, tx pgx.Tx, deal, audit ids.UUID) ([]ids.UUID, error) {
	return refileAssociationsTx(ctx, tx, deal, audit, fieldCompanyID)
}

// RefileProjectsTx retains project associations across later deal edits.
func (s *Store) RefileProjectsTx(ctx context.Context, tx pgx.Tx, deal, audit ids.UUID) ([]ids.UUID, error) {
	return refileAssociationsTx(ctx, tx, deal, audit, "project_id")
}

func refileAssociationsTx(ctx context.Context, tx pgx.Tx, deal, audit ids.UUID, field string) ([]ids.UUID, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, err
	}
	query := fmt.Sprintf(`SELECT DISTINCT association FROM (
 SELECT %s AS association FROM deal WHERE id=@refile_deal
 UNION ALL SELECT (before->>@field)::uuid FROM audit_log WHERE id=@audit AND entity_type='deal' AND entity_id=@refile_deal
 UNION ALL SELECT (after->>@field)::uuid FROM audit_log WHERE id=@audit AND entity_type='deal' AND entity_id=@refile_deal
 ) associations WHERE association IS NOT NULL ORDER BY association`, pgx.Identifier{field}.Sanitize())
	rows, err := tx.Query(ctx, query, pgx.NamedArgs{"refile_deal": deal, "audit": audit, "field": field})
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
}
