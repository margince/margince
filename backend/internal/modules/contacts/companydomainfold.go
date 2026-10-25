// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/platform/owndomain"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// unfoldedCompanyDomains selects the domains with no folded form yet. A public
// suffix stays NULL, so a replay inspects it again and changes nothing.
const unfoldedCompanyDomains = `FROM company_domain WHERE domain_folded IS NULL`

type companyDomainRow struct {
	ID     ids.UUID
	Domain string
}

// FoldCompanyDomainsTx stores owndomain.StoredOwn on up to limit domains written
// before the column existed, after the cursor, and returns the rows it
// inspected. The column is derived from the domain alone, so the write is not
// audited, the same as the insert that fills it for new rows.
// The UPDATE runs the row's version trigger, so a form opened on the older
// version must reload before it saves.
func FoldCompanyDomainsTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) ([]ids.UUID, int, error) {
	rows, err := tx.Query(ctx, `SELECT id, domain `+unfoldedCompanyDomains+`
		   AND id > $1 ORDER BY id LIMIT $2 FOR UPDATE`, after, limit)
	if err != nil {
		return nil, 0, fmt.Errorf("contacts: reading company domains to fold: %w", err)
	}
	written, err := pgx.CollectRows(rows, pgx.RowToStructByPos[companyDomainRow])
	if err != nil {
		return nil, 0, fmt.Errorf("contacts: reading company domains to fold: %w", err)
	}
	inspected := make([]ids.UUID, 0, len(written))
	rowIDs := make([]ids.UUID, 0, len(written))
	folded := make([]*string, 0, len(written))
	for _, row := range written {
		inspected = append(inspected, row.ID)
		if stored := owndomain.StoredOwn(row.Domain); stored != nil {
			rowIDs = append(rowIDs, row.ID)
			folded = append(folded, stored)
		}
	}
	if len(rowIDs) == 0 {
		return inspected, 0, nil
	}
	tag, err := tx.Exec(ctx, `
		UPDATE company_domain d SET domain_folded = folded.domain
		  FROM unnest($1::uuid[], $2::text[]) AS folded(id, domain)
		 WHERE d.id = folded.id AND d.domain_folded IS NULL`, rowIDs, folded)
	if err != nil {
		return nil, 0, fmt.Errorf("contacts: storing the folded form of company domains: %w", err)
	}
	return inspected, int(tag.RowsAffected()), nil
}

// CountUnfoldedCompanyDomains counts the rows FoldCompanyDomainsTx has yet to
// inspect, reading at most maximum+1 of them.
func CountUnfoldedCompanyDomains(ctx context.Context, q datatask.Querier, maximum int64) (int64, error) {
	var count int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT 1 `+unfoldedCompanyDomains+` LIMIT $1) unfolded`, maximum+1).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("contacts: counting company domains to fold: %w", err)
	}
	return count, nil
}
