// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// readPairOwners fills each row's Owners with the live owners of its two
// records, left end first. The rows were listed because the caller sees both
// ends, so naming their owners discloses nothing the records do not.
func readPairOwners(ctx context.Context, tx pgx.Tx, rows []DedupeCandidateRow) error {
	for _, side := range dedupePairSides {
		var ends []ids.UUID
		for _, r := range rows {
			if r.EntityType == side.entity {
				ends = append(ends, r.LeftID, r.RightID)
			}
		}
		if len(ends) == 0 {
			continue
		}
		res, err := tx.Query(ctx, fmt.Sprintf(`
			SELECT o.id, o.owner_id FROM %s o LEFT JOIN app_user u ON u.id = o.owner_id
			 WHERE o.id = ANY($1) AND o.owner_id IS NOT NULL AND %s`, side.entity, liveCaseOwnerSQL), ends)
		if err != nil {
			return fmt.Errorf("contacts: reading who owns the pair ends: %w", err)
		}
		ownerOf := map[ids.UUID]ids.UUID{}
		for res.Next() {
			var id, owner ids.UUID
			if err := res.Scan(&id, &owner); err != nil {
				res.Close()
				return err
			}
			ownerOf[id] = owner
		}
		res.Close()
		if err := res.Err(); err != nil {
			return err
		}
		for i, r := range rows {
			if r.EntityType != side.entity {
				continue
			}
			for _, end := range []ids.UUID{r.LeftID, r.RightID} {
				if owner, ok := ownerOf[end]; ok && !slices.Contains(rows[i].Owners, owner) {
					rows[i].Owners = append(rows[i].Owners, owner)
				}
			}
		}
	}
	return nil
}

// queryCandidateRows runs one queue statement whose columns are the candidate
// fields followed by the two writable flags, and reads its rows.
func queryCandidateRows(ctx context.Context, tx pgx.Tx, query string, args []any) ([]DedupeCandidateRow, error) {
	res, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer res.Close()
	var rows []DedupeCandidateRow
	for res.Next() {
		var r DedupeCandidateRow
		var writers PairWriters
		if err := res.Scan(&r.ID, &r.EntityType, &r.LeftID, &r.RightID, &r.Confidence,
			&r.Evidence, &r.Disposition, &r.DisposedBy, &r.DisposedAt, &r.CreatedAt,
			&writers.Left, &writers.Right); err != nil {
			return nil, err
		}
		r.pairAuthority(writers)
		rows = append(rows, r)
	}
	return rows, res.Err()
}
