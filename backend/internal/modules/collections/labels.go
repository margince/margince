// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package collections

// Names for a set of tags or lists, one query per kind. Each runs under a
// single get's grant and list row scope, so an unreadable record has no name.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// TagLabels names the live tags in a set. A tag has no owner, so the object
// grant is the whole gate, as it is on GetTag.
func (s *Store) TagLabels(ctx context.Context, want []ids.UUID) (map[ids.UUID]string, error) {
	return s.labelsOf(ctx, "tag", want, false)
}

// ListLabels names the live lists in a set the caller may see.
func (s *Store) ListLabels(ctx context.Context, want []ids.UUID) (map[ids.UUID]string, error) {
	return s.labelsOf(ctx, listObject, want, true)
}

// predicateAlways stands in for an absent scope clause, because an empty string
// interpolated into a `WHERE` is a syntax error rather than "no restriction".
const predicateAlways = "true"

// labelsOf is safe to format because object is a constant from the callers
// above. The object name is also the table name for both kinds.
func (s *Store) labelsOf(ctx context.Context, object string, want []ids.UUID, rowScoped bool) (map[ids.UUID]string, error) {
	if err := auth.Require(ctx, object, principal.ActionRead); err != nil {
		return nil, err
	}
	labels := map[ids.UUID]string{}
	if len(want) == 0 {
		return labels, nil
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	idsPos := arg(want)
	scope := ""
	if rowScoped {
		var err error
		if scope, err = auth.ScopeClauseFor(ctx, object, "r", arg); err != nil {
			return nil, err
		}
	}
	if scope == "" {
		scope = predicateAlways
	}
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		found, err := storekit.LabelsByID(ctx, tx, fmt.Sprintf(`
			SELECT r.id, coalesce(r.name, '')
			  FROM %s r
			 WHERE r.id = ANY($%d) AND r.archived_at IS NULL AND (%s)`,
			object, idsPos, scope), args...)
		labels = found
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("collections: reading %s names: %w", object, err)
	}
	return labels, nil
}
