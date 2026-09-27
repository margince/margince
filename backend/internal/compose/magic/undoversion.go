// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package magic

// The version an undo is sent with.
//
// The restore route takes an If-Match, so a control that offers "undo" must
// know the record's current version or it cannot send anything. The worklist's
// applied-undo carries it for that reason; this is the same field for the
// receipt. Read in the page's own transaction, through the same row scope as
// every other read here.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// versionQueries reads one record's version, per type. Fixed strings rather than
// a table name spliced in, so no value from a row reaches the SQL as text.
var versionQueries = map[string]string{
	typeDeal:    `SELECT version FROM deal WHERE id = $1`,
	typeCompany: `SELECT version FROM company WHERE id = $1`,
	typeContact: `SELECT version FROM contact WHERE id = $1`,
	typeLead:    `SELECT version FROM lead WHERE id = $1`,
	typeProject: `SELECT version FROM project WHERE id = $1`,
	"activity":  `SELECT version FROM activity WHERE id = $1 AND restricted_at IS NULL AND archived_at IS NULL`,
}

// attachVersion adds the record's version to an undoable answer. An answer that
// is not undoable carries none: there is nothing to send it with. A record the
// read cannot find turns the answer into a refusal rather than an offer the
// write would refuse.
func attachVersion(ctx context.Context, tx pgx.Tx, undo *crmcontracts.MagicUndo, entityType string, id ids.UUID) error {
	if undo == nil || !undo.Undoable {
		return nil
	}
	q, ok := versionQueries[entityType]
	if !ok {
		*undo = *refusedUndo(unwiredUndoReason)
		return nil
	}
	var version int64
	if err := tx.QueryRow(ctx, q, id).Scan(&version); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			*undo = *refusedUndo(unwiredUndoReason)
			return nil
		}
		return fmt.Errorf("read the version an undo is sent with: %w", err)
	}
	undo.Version = &version
	return nil
}
