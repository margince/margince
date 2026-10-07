// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ContactNamesAmong answers which of the given names belong to a live contact
// the caller may read — by full name, its first or last word, or the split
// first and last name, under the exact name key the dedupe ladder compares by.
// Like the label reads it is exactly as visible as the records: a contact
// outside the caller's scope names nobody.
func (s *Store) ContactNamesAmong(ctx context.Context, names []string) ([]string, error) {
	if err := auth.Require(ctx, entityContact, principal.ActionRead); err != nil {
		return nil, err
	}
	if len(names) == 0 {
		return nil, nil
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	namesPos := arg(names)
	scope, err := auth.ScopeClauseFor(ctx, entityContact, "r", arg)
	if err != nil {
		return nil, err
	}
	if scope == "" {
		scope = sqlAlwaysVisible
	}
	wanted := exactNameKeySQL("wanted.n")
	full := exactNameKeySQL("r.full_name")
	var found []string
	err = s.tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, fmt.Sprintf(`
			SELECT n
			  FROM unnest($%d::text[]) WITH ORDINALITY AS wanted(n, at)
			 WHERE EXISTS (
			       SELECT 1
			         FROM contact r
			        WHERE r.archived_at IS NULL AND (%s)
			          AND %s IN (%s, split_part(%s, ' ', 1), regexp_replace(%s, '^.* ', ''), %s, %s))
			 ORDER BY at`,
			namesPos, scope, wanted, full, full, full,
			exactNameKeySQL("r.first_name"), exactNameKeySQL("r.last_name")), args...)
		if err != nil {
			return err
		}
		found, err = pgx.CollectRows(rows, pgx.RowTo[string])
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("contacts: matching contact names: %w", err)
	}
	return found, nil
}
