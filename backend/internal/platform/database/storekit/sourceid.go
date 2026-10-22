// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// sourceKeyArbiter names the partial unique index a record table keeps on
// (source_system, source_id). An INSERT that loses to it writes nothing and
// leaves the transaction usable.
const sourceKeyArbiter = ` ON CONFLICT (source_system, source_id)
	WHERE source_system IS NOT NULL AND source_id IS NOT NULL DO NOTHING`

// ProvenanceInsertFragments appends a record's outside id and its source author
// to an INSERT's fixed args. It returns the column and placeholder fragments,
// numbered from the args so no placeholder is counted by hand. Chain it before
// InsertFragments.
func ProvenanceInsertFragments(sourceID *string, author SourceAuthorInput, base []any) (cols, placeholders string, args []any) {
	n := len(base)
	args = append(append(make([]any, 0, n+3), base...), sourceID, author.AuthorID, author.AuthorName)
	return ", source_id, source_author_id, source_author_name",
		fmt.Sprintf(", $%d, $%d, $%d", n+1, n+2, n+3), args
}

// SourceKeyClauses narrows a list by the outside system, the outside id, or
// both. Both name one record, the pair the table's unique index serves. Each is
// an equality match on its own, so neither given narrows nothing. A value the
// create wire would refuse is refused here too, by the same check.
func SourceKeyClauses(sourceSystem, sourceID *string, arg func(any) int) ([]string, error) {
	if err := provenance.CheckSourceValues(sourceSystem, sourceID); err != nil {
		return nil, err
	}
	var where []string
	if sourceSystem != nil {
		where = append(where, SQLf("source_system = $%d", arg(*sourceSystem)))
	}
	if sourceID != nil {
		where = append(where, SQLf("source_id = $%d", arg(*sourceID)))
	}
	return where, nil
}

// ExecSourceInsert runs a record's INSERT and answers a conflict when another
// record of the same kind already holds the (source_system, source_id) pair.
// record is the table name, a compile-time literal used only in the message.
//
// The answer is the same whether or not the caller may read the holder. It names,
// links and counts nothing, so it says only what the caller sent. Nothing is
// written when it loses.
func ExecSourceInsert(ctx context.Context, tx pgx.Tx, record, sql string, args []any) error {
	tag, err := tx.Exec(ctx, sql+sourceKeyArbiter, args...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("a %s with this source_system and source_id already exists: %w", record, apperrors.ErrConflict)
	}
	return nil
}
