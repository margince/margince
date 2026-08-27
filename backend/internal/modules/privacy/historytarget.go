// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// Whether one audit row is an entry of one record's history — asked by the
// reversal path, which addresses a row the record does not own.
//
// A record's history holds two kinds of row: its own, and the rows of the LINKS
// it is an end of. The second kind sits on ('relationship', edge_id), so the
// path's `{entity_type}/{id}` and the target row's own identity are different
// things, and a reversal that admitted a row by its id alone would let a caller
// holding an audit id reach a link whose other end they may not see.
//
// The admission is recordHistoryWindowSQL's, unchanged and unrestated:
// endpoint membership, the other end's visibility, the other end's erasure. A
// second spelling of it would answer the same question differently the first
// time either half moved.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// HistoryServesEntry reports whether the record's history page serves auditID.
//
// It takes the caller's transaction because the reversal path asks it beside the
// record's own row-scope gate, and the two must read one snapshot: admitting the
// entry against a state the write no longer sees is the gap If-Match exists to
// close, and it should not be widened here.
//
// The record's own erasure boundary is deliberately NOT applied. A row behind it
// is served by this admission and refused by the evaluator's own
// behind_erasure_boundary branch, which is the answer a person can act on;
// answering "no such entry" would say the row does not exist when it does.
//
// That division of labour holds for a LINK's row only because the evaluator asks
// the boundary of the records the link JOINS (EdgeBehindErasureBoundary) rather
// than of the link itself. Keyed on the row's own identity it would answer about
// ('relationship', edge_id), which no scrub verb is ever written against, and
// this admission would be handing the write a row nothing downstream bounds.
func HistoryServesEntry(ctx context.Context, tx pgx.Tx, entityType string, entityID, auditID ids.UUID) (bool, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	typePos, idPos := arg(entityType), arg(entityID)
	conds := []string{fmt.Sprintf("a.id = $%d", arg(auditID))}

	edgeCTE, err := edgeSubjectCTE(ctx, entityType, idPos, arg)
	// No grant on the edge object leaves the links ABSENT rather than refused,
	// exactly as the list read leaves them: a refusal is proof the record holds
	// links, and this path must not be the one that says so.
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		edgeCTE = ""
	} else if err != nil {
		return false, err
	}

	window := recordHistoryWindowSQL(typePos, idPos, arg(1), conds, edgeCTE)
	var served bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM (`+window+`) AS target)`, args...).Scan(&served); err != nil {
		return false, fmt.Errorf("privacy: whether %s history serves entry %s: %w", entityType, auditID, err)
	}
	return served, nil
}
