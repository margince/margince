// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/platform/owndomain"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// unfoldedLoginDomains selects the seats whose login has no folded domain yet. A
// login without a domain stays NULL, so a replay inspects it again and changes
// nothing.
const unfoldedLoginDomains = `FROM app_user WHERE email_domain_folded IS NULL`

type loginAddress struct {
	ID    ids.UUID
	Email string
}

// FoldLoginDomainsTx stores owndomain.StoredOfAddress on up to limit seats
// written before the column existed, after the cursor, and returns the seats
// it inspected. The column is derived from the login alone, so the write is not
// audited, the same as the insert that fills it for new seats.
func FoldLoginDomainsTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) ([]ids.UUID, int, error) {
	rows, err := tx.Query(ctx, `SELECT id, email `+unfoldedLoginDomains+`
		   AND id > $1 ORDER BY id LIMIT $2 FOR UPDATE`, after, limit)
	if err != nil {
		return nil, 0, fmt.Errorf("identity: reading login addresses to fold: %w", err)
	}
	seats, err := pgx.CollectRows(rows, pgx.RowToStructByPos[loginAddress])
	if err != nil {
		return nil, 0, fmt.Errorf("identity: reading login addresses to fold: %w", err)
	}
	inspected := make([]ids.UUID, 0, len(seats))
	seatIDs := make([]ids.UUID, 0, len(seats))
	folded := make([]*string, 0, len(seats))
	for _, seat := range seats {
		inspected = append(inspected, seat.ID)
		if stored := owndomain.StoredOfAddress(seat.Email); stored != nil {
			seatIDs = append(seatIDs, seat.ID)
			folded = append(folded, stored)
		}
	}
	if len(seatIDs) == 0 {
		return inspected, 0, nil
	}
	tag, err := tx.Exec(ctx, `
		UPDATE app_user u SET email_domain_folded = folded.domain
		  FROM unnest($1::uuid[], $2::text[]) AS folded(id, domain)
		 WHERE u.id = folded.id AND u.email_domain_folded IS NULL`, seatIDs, folded)
	if err != nil {
		return nil, 0, fmt.Errorf("identity: storing the folded domain of login addresses: %w", err)
	}
	return inspected, int(tag.RowsAffected()), nil
}

// CountUnfoldedLoginDomains counts the seats FoldLoginDomainsTx has yet to
// inspect, reading at most maximum+1 of them.
func CountUnfoldedLoginDomains(ctx context.Context, q datatask.Querier, maximum int64) (int64, error) {
	var count int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT 1 `+unfoldedLoginDomains+` LIMIT $1) unfolded`, maximum+1).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("identity: counting login addresses to fold: %w", err)
	}
	return count, nil
}
