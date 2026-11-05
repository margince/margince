// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/platform/owndomain"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// participantAddress is one written participant row and the address it holds.
type participantAddress struct {
	ID      ids.UUID
	Address string
}

// unfoldedParticipants selects the rows whose address has no folded domain yet.
// An address without a domain stays NULL, so a replay inspects it again and
// changes nothing.
const unfoldedParticipants = `FROM activity_participant
	 WHERE address IS NOT NULL AND address_domain_folded IS NULL`

// FoldParticipantDomainsTx stores the folded domain on up to limit rows written
// before the column existed, after the cursor. It returns the rows it inspected.
// The column is derived from the address alone, so the write is not audited,
// the same as the insert that fills it for new rows.
func FoldParticipantDomainsTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) ([]ids.UUID, int, error) {
	rows, err := tx.Query(ctx, `SELECT id, address `+unfoldedParticipants+`
		   AND id > $1 ORDER BY id LIMIT $2 FOR UPDATE`, after, limit)
	if err != nil {
		return nil, 0, fmt.Errorf("activities: reading participant addresses to fold: %w", err)
	}
	written, err := pgx.CollectRows(rows, pgx.RowToStructByPos[participantAddress])
	if err != nil {
		return nil, 0, fmt.Errorf("activities: reading participant addresses to fold: %w", err)
	}
	inspected := make([]ids.UUID, 0, len(written))
	for _, row := range written {
		inspected = append(inspected, row.ID)
	}
	changed, err := storeParticipantDomains(ctx, tx, written)
	return inspected, changed, err
}

// CountUnfoldedParticipants counts the rows FoldParticipantDomainsTx has yet to
// inspect, reading at most maximum+1 of them.
func CountUnfoldedParticipants(ctx context.Context, q datatask.Querier, maximum int64) (int64, error) {
	var count int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT 1 `+unfoldedParticipants+` LIMIT $1) unfolded`, maximum+1).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("activities: counting participant addresses to fold: %w", err)
	}
	return count, nil
}

// foldInsertedParticipants stores the folded domain on the (id, address) rows
// an INSERT returned. A writer that derives the address in SQL needs it, since
// SQL cannot fold IDNA. It returns how many rows the INSERT wrote.
func foldInsertedParticipants(ctx context.Context, tx pgx.Tx, rows pgx.Rows) (int, error) {
	written, err := pgx.CollectRows(rows, pgx.RowToStructByPos[participantAddress])
	if err != nil {
		return 0, fmt.Errorf("activities: reading the participants just written: %w", err)
	}
	if _, err := storeParticipantDomains(ctx, tx, written); err != nil {
		return 0, err
	}
	return len(written), nil
}

// storeParticipantDomains writes owndomain.StoredOfAddress for rows a writer
// could not fold in its own statement, and returns how many it changed.
func storeParticipantDomains(ctx context.Context, tx pgx.Tx, written []participantAddress) (int, error) {
	rowIDs := make([]ids.UUID, 0, len(written))
	domains := make([]*string, 0, len(written))
	for _, row := range written {
		if folded := owndomain.StoredOfAddress(row.Address); folded != nil {
			rowIDs = append(rowIDs, row.ID)
			domains = append(domains, folded)
		}
	}
	if len(rowIDs) == 0 {
		return 0, nil
	}
	tag, err := tx.Exec(ctx, `
		UPDATE activity_participant ap SET address_domain_folded = folded.domain
		  FROM unnest($1::uuid[], $2::text[]) AS folded(id, domain)
		 WHERE ap.id = folded.id AND ap.address_domain_folded IS DISTINCT FROM folded.domain`, rowIDs, domains)
	if err != nil {
		return 0, fmt.Errorf("activities: storing the folded domain of participant addresses: %w", err)
	}
	return int(tag.RowsAffected()), nil
}
