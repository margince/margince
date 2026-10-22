// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"
	"maps"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// originMerge is the origin a merge's per-edge audit rows carry as evidence.
// The edge's history can then say why it moved.
const originMerge = "merge"

var mergeEdgeEvidence = map[string]any{auditKeyOrigin: originMerge}

// edgeIDsKey is the named argument that carries the edges a merge write locked.
const edgeIDsKey = "edges"

// mergeEdgeWrite runs `UPDATE relationship a SET <set>` over the edges `where`
// picks, and gives each changed edge its own audit row and event. The edges are
// read FOR UPDATE first, so the before image is the state the UPDATE changes.
// flipCurrentPrimary derives its before image from the one flag it moves; a
// merge moves more columns, so the two do not share the read. A moved edge's
// image carries its endpoints, because moving the edge is the change.
func mergeEdgeWrite(ctx context.Context, tx pgx.Tx, set, where string, args pgx.NamedArgs) (int64, error) {
	rows, err := tx.Query(ctx, `SELECT `+relationshipColumns+` FROM relationship a WHERE `+where+` FOR UPDATE`, args)
	if err != nil {
		return 0, fmt.Errorf("contacts: locking the edges a merge moves: %w", err)
	}
	locked, err := scanRelationships(rows)
	rows.Close()
	if err != nil {
		return 0, fmt.Errorf("contacts: locking the edges a merge moves: %w", err)
	}
	if len(locked) == 0 {
		return 0, nil
	}
	before := make(map[ids.UUID]relationshipRow, len(locked))
	edgeIDs := make([]ids.UUID, 0, len(locked))
	for _, rel := range locked {
		before[rel.ID] = rel
		edgeIDs = append(edgeIDs, rel.ID)
	}
	writeArgs := pgx.NamedArgs{edgeIDsKey: edgeIDs}
	maps.Copy(writeArgs, args)
	rows, err = tx.Query(ctx, `UPDATE relationship a SET `+set+` WHERE a.id = ANY(@`+edgeIDsKey+`) RETURNING `+relationshipColumns, writeArgs)
	if err != nil {
		return 0, err
	}
	written, err := scanRelationships(rows)
	rows.Close()
	if err != nil {
		return 0, err
	}
	for _, rel := range written {
		if err := emitEdgeWrite(ctx, tx, before[rel.ID], rel); err != nil {
			return 0, err
		}
	}
	return int64(len(written)), nil
}

func emitEdgeWrite(ctx context.Context, tx pgx.Tx, before, after relationshipRow) error {
	if before.ArchivedAt == nil && after.ArchivedAt != nil {
		return emitRelationshipChangeWithEvidence(ctx, tx, "archive", nil, after, mergeEdgeEvidence)
	}
	from, to := storekit.ChangedColumns(relationshipMoveImage(before), relationshipMoveImage(after))
	if len(to) == 0 {
		return nil
	}
	return emitRelationshipNarrowedImagePairs(ctx, tx, "update", from, to, after, mergeEdgeEvidence)
}
