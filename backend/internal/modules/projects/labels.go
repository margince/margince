// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package projects

// Display names for a SET of projects, in one query.
//
// The attention feed names every subject on the page; read one at a time that
// is a round trip per card. It carries the object grant and the row-scope
// clause GetProject carries, so a name is exactly as visible as the project,
// and a project the caller cannot see is absent rather than refused.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ProjectLabels answers each named project's own name, under the caller's
// grants.
func (s *Store) ProjectLabels(ctx context.Context, want []ids.UUID) (map[ids.UUID]string, error) {
	if err := auth.Require(ctx, projectObject, principal.ActionRead); err != nil {
		return nil, err
	}
	labels := map[ids.UUID]string{}
	if len(want) == 0 {
		return labels, nil
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	idsPos := arg(want)
	scope, err := auth.ScopeClauseFor(ctx, projectObject, "p", arg)
	if err != nil {
		return nil, err
	}
	if scope == "" {
		scope = "true"
	}
	err = s.Tx(ctx, func(tx pgx.Tx) error {
		found, err := storekit.LabelsByID(ctx, tx, fmt.Sprintf(`
			SELECT p.id, coalesce(p.name, '')
			  FROM project p
			 WHERE p.id = ANY($%d) AND p.archived_at IS NULL AND (%s)`,
			idsPos, scope), args...)
		labels = found
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("projects: reading project names: %w", err)
	}
	return labels, nil
}
