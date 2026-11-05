// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ArchiveScope names affected IDs; callers must authorize referenced records before disclosure.
type ArchiveScope struct {
	Relationships []ids.UUID
	Lists         []ids.UUID
	Tags          []ids.UUID
}

// PreviewArchiveScope accepts predicates owned by the archive writer. Callers
// authorize the named lists and tags before disclosing the scope.
func PreviewArchiveScope(ctx context.Context, tx pgx.Tx, id ids.UUID, edges, memberships string) (ArchiveScope, error) {
	var out ArchiveScope
	var err error
	out.Relationships, err = previewArchiveEdges(ctx, tx, id, edges)
	if err != nil {
		return ArchiveScope{}, err
	}
	for _, scope := range []struct {
		query string
		into  *[]ids.UUID
	}{
		{`SELECT list_id FROM list_member WHERE ` + memberships + ` ORDER BY list_id LIMIT 501`, &out.Lists},
		{`SELECT tag_id FROM taggable WHERE ` + memberships + ` ORDER BY tag_id LIMIT 501`, &out.Tags},
	} {
		rows, err := tx.Query(ctx, scope.query, pgx.StrictNamedArgs{"record": id})
		if err != nil {
			return ArchiveScope{}, err
		}
		*scope.into, err = ScanUUIDColumn(rows, "read archive scope")
		if err != nil {
			return ArchiveScope{}, err
		}
		if len(*scope.into) > 500 {
			return ArchiveScope{}, apperrors.ErrInvalidArgument
		}
	}
	return out, nil
}

func previewArchiveEdges(ctx context.Context, tx pgx.Tx, id ids.UUID, predicate string) ([]ids.UUID, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	predicate = strings.ReplaceAll(predicate, "@record", SQLf("$%d", arg(id)))
	bound, err := auth.EdgeReadScope(ctx, "r", arg)
	if err != nil {
		return nil, err
	}
	if bound == "" {
		bound = "true"
	}
	rows, err := tx.Query(ctx, SQLf("SELECT r.id, (%s) FROM relationship r WHERE %s ORDER BY r.id LIMIT 501", bound, predicate), args...)
	if err != nil {
		return nil, err
	}
	matches, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (ids.UUID, error) {
		var id ids.UUID
		var visible bool
		if err := row.Scan(&id, &visible); err != nil {
			return id, err
		}
		if !visible {
			return ids.UUID{}, apperrors.ErrNotFound
		}
		return id, nil
	})
	if err != nil {
		return nil, err
	}
	if len(matches) > 500 {
		return nil, apperrors.ErrInvalidArgument
	}
	return matches, nil
}
