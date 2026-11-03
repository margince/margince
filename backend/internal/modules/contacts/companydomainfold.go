// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// domainFold is a data task's pass over one table's folded-domain column, on
// rows written before the column existed. The column and its fold come from
// storekit.FoldedDomains, the list every Patch write stamps from.
type domainFold struct{ table, noun string }

var (
	companyDomainFold      = domainFold{table: "company_domain", noun: "company domains"}
	contactEmailDomainFold = domainFold{table: tableContactEmail, noun: "contact addresses"}
	leadEmailDomainFold    = domainFold{table: "lead", noun: "lead addresses"}
)

func (f domainFold) column() (storekit.FoldedDomain, error) {
	column, ok := storekit.FoldedDomainOf(f.table)
	if !ok {
		return storekit.FoldedDomain{}, fmt.Errorf("contacts: %s has no folded-domain column in storekit.FoldedDomains", f.table)
	}
	return column, nil
}

// unfolded selects the rows with a value and no folded form yet. A value with
// no domain stays NULL, so a replay inspects it again and changes nothing.
func unfolded(c storekit.FoldedDomain) string {
	return `FROM ` + c.Table + ` WHERE ` + c.Folded + ` IS NULL AND ` + c.Source + ` IS NOT NULL`
}

type foldRow struct {
	ID    ids.UUID
	Value string
}

// run stores the folded form on up to limit rows after the cursor, and
// returns the rows it inspected. The column is derived from the row's own
// value, so the write is not audited, the same as the insert that fills it.
func (f domainFold) run(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) ([]ids.UUID, int, error) {
	c, err := f.column()
	if err != nil {
		return nil, 0, err
	}
	rows, err := tx.Query(ctx, `SELECT id, `+c.Source+` `+unfolded(c)+`
		   AND id > $1 ORDER BY id LIMIT $2 FOR UPDATE`, after, limit)
	if err != nil {
		return nil, 0, fmt.Errorf("contacts: reading %s to fold: %w", f.noun, err)
	}
	written, err := pgx.CollectRows(rows, pgx.RowToStructByPos[foldRow])
	if err != nil {
		return nil, 0, fmt.Errorf("contacts: reading %s to fold: %w", f.noun, err)
	}
	inspected := make([]ids.UUID, 0, len(written))
	rowIDs := make([]ids.UUID, 0, len(written))
	folded := make([]*string, 0, len(written))
	for _, row := range written {
		inspected = append(inspected, row.ID)
		if stored := c.Fold(row.Value); stored != nil {
			rowIDs = append(rowIDs, row.ID)
			folded = append(folded, stored)
		}
	}
	if len(rowIDs) == 0 {
		return inspected, 0, nil
	}
	tag, err := tx.Exec(ctx, `
		UPDATE `+c.Table+` t SET `+c.Folded+` = folded.domain
		  FROM unnest($1::uuid[], $2::text[]) AS folded(id, domain)
		 WHERE t.id = folded.id AND t.`+c.Folded+` IS NULL`, rowIDs, folded)
	if err != nil {
		return nil, 0, fmt.Errorf("contacts: storing the folded domain of %s: %w", f.noun, err)
	}
	return inspected, int(tag.RowsAffected()), nil
}

// count counts the rows run has yet to inspect, reading at most maximum+1.
func (f domainFold) count(ctx context.Context, q datatask.Querier, maximum int64) (int64, error) {
	c, err := f.column()
	if err != nil {
		return 0, err
	}
	var n int64
	err = q.QueryRow(ctx, `SELECT count(*) FROM (SELECT 1 `+unfolded(c)+` LIMIT $1) unfolded`, maximum+1).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("contacts: counting %s to fold: %w", f.noun, err)
	}
	return n, nil
}

// FoldCompanyDomainsTx stores owndomain.StoredOwn on company domains written
// before the column existed. The UPDATE runs the row's version trigger, so a
// form opened on the older version must reload before it saves.
func FoldCompanyDomainsTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) ([]ids.UUID, int, error) {
	return companyDomainFold.run(ctx, tx, after, limit)
}

// CountUnfoldedCompanyDomains counts what FoldCompanyDomainsTx has yet to inspect.
func CountUnfoldedCompanyDomains(ctx context.Context, q datatask.Querier, maximum int64) (int64, error) {
	return companyDomainFold.count(ctx, q, maximum)
}

// FoldContactEmailDomainsTx stores owndomain.StoredOfAddress on contact
// addresses written before the column existed. The UPDATE runs the address's
// version trigger, so a form opened on the older version must reload first.
func FoldContactEmailDomainsTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) ([]ids.UUID, int, error) {
	return contactEmailDomainFold.run(ctx, tx, after, limit)
}

// CountUnfoldedContactEmailDomains counts what FoldContactEmailDomainsTx has
// yet to inspect.
func CountUnfoldedContactEmailDomains(ctx context.Context, q datatask.Querier, maximum int64) (int64, error) {
	return contactEmailDomainFold.count(ctx, q, maximum)
}

// FoldLeadEmailDomainsTx stores owndomain.StoredOfAddress on lead addresses
// written before the column existed. The UPDATE runs the lead's version
// trigger, so a form opened on the older version must reload before it saves.
func FoldLeadEmailDomainsTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) ([]ids.UUID, int, error) {
	return leadEmailDomainFold.run(ctx, tx, after, limit)
}

// CountUnfoldedLeadEmailDomains counts what FoldLeadEmailDomainsTx has yet to
// inspect.
func CountUnfoldedLeadEmailDomains(ctx context.Context, q datatask.Querier, maximum int64) (int64, error) {
	return leadEmailDomainFold.count(ctx, q, maximum)
}
